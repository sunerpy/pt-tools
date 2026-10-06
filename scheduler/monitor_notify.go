package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
)

// MonitorSender 经 live 通道同步发送一条通知到指定通道，失败或超时直接返回错误，不写 outbox。
//
// 生产实现包装 app.NotificationService.PushSync（scheduler 不能依赖 internal/app，后者依赖 scheduler）。
// 不能换成 Push：Push 在 live 发送失败后转写 outbox 并返回 nil，而 outbox 用 registry.Make 新建通道实例，
// 发不了 QQ / Telegram 这类有状态通道，日志行也会被误记为已送达。
type MonitorSender interface {
	Send(ctx context.Context, confID uint, title, text string) error
}

// MonitorSenderFunc 让普通函数实现 MonitorSender。
type MonitorSenderFunc func(ctx context.Context, confID uint, title, text string) error

func (f MonitorSenderFunc) Send(ctx context.Context, confID uint, title, text string) error {
	return f(ctx, confID, title, text)
}

// MonitorNotifyEntry 是一次「决定发送」。同一个 (Source, Subject, Kind, EventKey) 重复写入不会重复发送。
type MonitorNotifyEntry struct {
	Source   string
	Subject  string
	Kind     string
	EventKey string
	Title    string
	Text     string
	// ConfIDs 为空时发给决策当时所有启用的通道。
	ConfIDs []uint
	// BypassQuiet 为 true 时不受通道静默时段限制（封号前最后一天、测试提醒）。
	BypassQuiet bool
}

type monitorNotifyPayload struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

const (
	monitorNotifyTick      = time.Minute
	monitorNotifyBatch     = 50
	monitorNotifyMaxErrLen = 1024
)

// monitorNotifyBackoff 是第 1、2、3 次投递失败后的等待时间；第 4 次失败记为 failed。
var monitorNotifyBackoff = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute}

// MonitorNotifier 负责后台监控通知的入队与投递，是这些通知唯一的重试者。
type MonitorNotifier struct {
	db     *gorm.DB
	sender MonitorSender
	clock  sitelogin.Clock
	logger *zap.SugaredLogger
	tick   time.Duration

	// deliverMu 串行化所有投递（后台循环与测试提醒的即时投递），同一行不会被发两次。
	deliverMu sync.Mutex

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewMonitorNotifier 构造投递器。sender 为 nil 时不写日志行（没有可用的通知服务）。
func NewMonitorNotifier(db *gorm.DB, sender MonitorSender, clock sitelogin.Clock, logger *zap.SugaredLogger) *MonitorNotifier {
	if clock == nil {
		clock = sitelogin.NewRealClock()
	}
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}
	return &MonitorNotifier{db: db, sender: sender, clock: clock, logger: logger, tick: monitorNotifyTick}
}

// Enabled 报告投递器是否可用（有数据库也有发送方）。
func (n *MonitorNotifier) Enabled() bool {
	return n != nil && n.db != nil && n.sender != nil
}

// Enqueue 按目标通道各写一行。通道此刻处于自己的静默时段时，该行顺延到静默结束。
// 返回新写入的行数；已存在的行（同一决策重做）不计入。
func (n *MonitorNotifier) Enqueue(ctx context.Context, e MonitorNotifyEntry) (int, error) {
	if !n.Enabled() {
		return 0, nil
	}
	confs, err := n.targetConfs(ctx, e.ConfIDs)
	if err != nil {
		return 0, err
	}
	payload, err := json.Marshal(monitorNotifyPayload{Title: e.Title, Text: e.Text})
	if err != nil {
		return 0, fmt.Errorf("序列化通知内容失败: %w", err)
	}
	now := n.clock.Now()
	inserted := 0
	for _, conf := range confs {
		next := now
		if !e.BypassQuiet && notify.IsQuietNow(now, conf.QuietHoursStart, conf.QuietHoursEnd) {
			next = notify.NextQuietEnd(now, conf.QuietHoursEnd)
		}
		row := models.MonitorNotificationLog{
			Source:             e.Source,
			Subject:            e.Subject,
			Kind:               e.Kind,
			EventKey:           e.EventKey,
			NotificationConfID: conf.ID,
			BypassQuiet:        e.BypassQuiet,
			Result:             models.MonitorNotifyPending,
			NextRetryAt:        next.UTC(),
			PayloadJSON:        string(payload),
		}
		res := n.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if res.Error != nil {
			return inserted, fmt.Errorf("写入通知日志失败: %w", res.Error)
		}
		inserted += int(res.RowsAffected)
	}
	return inserted, nil
}

// Logged 报告同一决策（Source、Subject、Kind、EventKey）是否已经写过投递行，任何通道都算。
func (n *MonitorNotifier) Logged(ctx context.Context, source, subject, kind, eventKey string) bool {
	if !n.Enabled() {
		return false
	}
	var count int64
	err := n.db.WithContext(ctx).Model(&models.MonitorNotificationLog{}).
		Where("source = ? AND subject = ? AND kind = ? AND event_key = ?", source, subject, kind, eventKey).
		Count(&count).Error
	return err == nil && count > 0
}

func (n *MonitorNotifier) targetConfs(ctx context.Context, ids []uint) ([]models.NotificationConf, error) {
	var confs []models.NotificationConf
	q := n.db.WithContext(ctx).
		Select("id", "enabled", "quiet_hours_start", "quiet_hours_end").
		Where("enabled = ?", true)
	if len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	}
	if err := q.Order("id").Find(&confs).Error; err != nil {
		return nil, fmt.Errorf("读取通知通道失败: %w", err)
	}
	return confs, nil
}

// DeliverDue 投递所有到期的 pending 行，返回成功发送的条数。
func (n *MonitorNotifier) DeliverDue(ctx context.Context) int {
	if !n.Enabled() {
		return 0
	}
	n.deliverMu.Lock()
	defer n.deliverMu.Unlock()

	now := n.clock.Now()
	var rows []models.MonitorNotificationLog
	if err := n.db.WithContext(ctx).
		Where("result = ? AND next_retry_at <= ?", models.MonitorNotifyPending, now.UTC()).
		Order("next_retry_at").Limit(monitorNotifyBatch).
		Find(&rows).Error; err != nil {
		n.logger.Warnw("monitor_notify_load_failed", "err", err)
		return 0
	}
	sent := 0
	for _, row := range rows {
		if ctx.Err() != nil {
			break
		}
		// 每行按此刻的时间判断静默：前面的发送可能耗时，批次开始时的时间会过时。
		if n.deliverRow(ctx, row, n.clock.Now()) == nil {
			sent++
		}
	}
	return sent
}

// DeliverNow 立即投递某个决策的全部 pending 行（测试提醒用），返回首个失败的错误。
func (n *MonitorNotifier) DeliverNow(ctx context.Context, e MonitorNotifyEntry) error {
	if !n.Enabled() {
		return errors.New("通知服务未初始化")
	}
	n.deliverMu.Lock()
	defer n.deliverMu.Unlock()

	var rows []models.MonitorNotificationLog
	if err := n.db.WithContext(ctx).
		Where("source = ? AND subject = ? AND kind = ? AND event_key = ? AND result = ?",
			e.Source, e.Subject, e.Kind, e.EventKey, models.MonitorNotifyPending).
		Order("id").Find(&rows).Error; err != nil {
		return fmt.Errorf("读取通知日志失败: %w", err)
	}
	if len(rows) == 0 {
		return errors.New("没有可用的通知通道")
	}
	var firstErr error
	for _, row := range rows {
		if err := n.deliverRow(ctx, row, n.clock.Now()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// errDeferredQuiet 表示该行因通道静默被顺延，没有真正发送。
var errDeferredQuiet = errors.New("通道处于静默时段，已顺延")

func (n *MonitorNotifier) deliverRow(ctx context.Context, row models.MonitorNotificationLog, now time.Time) error {
	var conf models.NotificationConf
	err := n.db.WithContext(ctx).
		Select("id", "enabled", "quiet_hours_start", "quiet_hours_end").
		First(&conf, row.NotificationConfID).Error
	if err != nil || !conf.Enabled {
		reason := "通知通道不存在或已停用"
		n.finishRow(ctx, row.ID, map[string]any{
			"result":     models.MonitorNotifyFailed,
			"last_error": reason,
		})
		return errors.New(reason)
	}
	// 实际发送前再判断一次静默：静默开始前写入、静默期间才轮到的行，以及重试落进静默时段的行都在这里顺延，不计尝试。
	if !row.BypassQuiet && notify.IsQuietNow(now, conf.QuietHoursStart, conf.QuietHoursEnd) {
		n.finishRow(ctx, row.ID, map[string]any{
			"next_retry_at": notify.NextQuietEnd(now, conf.QuietHoursEnd).UTC(),
		})
		return errDeferredQuiet
	}

	var payload monitorNotifyPayload
	if err := json.Unmarshal([]byte(row.PayloadJSON), &payload); err != nil {
		n.finishRow(ctx, row.ID, map[string]any{
			"result":     models.MonitorNotifyFailed,
			"last_error": "通知内容损坏",
		})
		return fmt.Errorf("解析通知内容失败: %w", err)
	}

	sendErr := n.sender.Send(ctx, conf.ID, payload.Title, payload.Text)
	attempts := row.Attempts + 1
	if sendErr == nil {
		n.finishRow(ctx, row.ID, map[string]any{
			"result":       models.MonitorNotifySent,
			"attempts":     attempts,
			"last_error":   "",
			"delivered_at": now.UTC(),
		})
		return nil
	}

	updates := map[string]any{
		"attempts":   attempts,
		"last_error": truncateNotifyError(sendErr.Error()),
	}
	if attempts > len(monitorNotifyBackoff) {
		updates["result"] = models.MonitorNotifyFailed
	} else {
		updates["next_retry_at"] = now.Add(monitorNotifyBackoff[attempts-1]).UTC()
	}
	n.finishRow(ctx, row.ID, updates)
	n.logger.Warnw("monitor_notify_send_failed",
		"source", row.Source, "subject", row.Subject, "kind", row.Kind,
		"conf_id", row.NotificationConfID, "attempts", attempts, "err", sendErr)
	return sendErr
}

func (n *MonitorNotifier) finishRow(ctx context.Context, id uint, updates map[string]any) {
	if err := n.db.WithContext(context.WithoutCancel(ctx)).
		Model(&models.MonitorNotificationLog{}).
		Where("id = ? AND result = ?", id, models.MonitorNotifyPending).
		Updates(updates).Error; err != nil {
		n.logger.Warnw("monitor_notify_update_failed", "id", id, "err", err)
	}
}

func truncateNotifyError(s string) string {
	if len(s) <= monitorNotifyMaxErrLen {
		return s
	}
	return s[:monitorNotifyMaxErrLen]
}

// Start 启动每分钟一次的投递循环；重复调用无副作用。
func (n *MonitorNotifier) Start() {
	if !n.Enabled() {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	n.cancel = cancel
	n.running = true
	tick := n.tick
	if tick <= 0 {
		tick = monitorNotifyTick
	}
	n.wg.Go(func() {
		ticker := time.NewTicker(tick)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n.DeliverDue(ctx)
			}
		}
	})
}

// Stop 停止投递循环并等待当前一轮结束。
func (n *MonitorNotifier) Stop() {
	if n == nil {
		return
	}
	n.mu.Lock()
	if !n.running {
		n.mu.Unlock()
		return
	}
	n.running = false
	cancel := n.cancel
	n.mu.Unlock()
	cancel()
	n.wg.Wait()
}
