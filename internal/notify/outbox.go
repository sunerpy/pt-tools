package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

const (
	outboxStatusPending = "pending"
	outboxStatusSent    = "sent"
	outboxStatusDead    = "dead"
	defaultInterval     = 10 * time.Second
	maxErrorMsgLen      = 1024
)

var (
	backoffSchedule = []time.Duration{10 * time.Second, 60 * time.Second, 300 * time.Second}
	nowFn           = time.Now
)

// OutboxWorker scans pending notification_outbox rows and retries delivery with
// bounded exponential backoff.
type OutboxWorker struct {
	db       *gorm.DB
	registry *Registry
	interval time.Duration
	// live 经运行中的通道实例投递（生产环境注入）。有状态通道不能另起实例：QQ 会抢监听端口，
	// Telegram 会多开一个长轮询和正在运行的实例抢更新；ConfigJSON 在库里也是密文。
	live LiveSender
	// decrypt 解密 NotificationConf.ConfigJSON，只在没有 live 时的回退路径使用。
	decrypt func(string) (string, error)

	startOnce sync.Once
	mu        sync.Mutex
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// LiveSender 经运行中的通道实例投递一条通知。
type LiveSender interface {
	Send(ctx context.Context, confID uint, n Notification) error
}

// SetLiveSender 让重试经运行中的通道投递，不再按配置另建实例。须在 Start 之前调用。
func (w *OutboxWorker) SetLiveSender(s LiveSender) { w.live = s }

// SetConfigDecrypter 设置回退路径解密 ConfigJSON 的函数。须在 Start 之前调用。
func (w *OutboxWorker) SetConfigDecrypter(fn func(string) (string, error)) { w.decrypt = fn }

// NewOutboxWorker creates a worker. interval defaults to 10s when <= 0.
func NewOutboxWorker(db *gorm.DB, registry *Registry, interval time.Duration) *OutboxWorker {
	if interval <= 0 {
		interval = defaultInterval
	}
	if registry == nil {
		registry = DefaultRegistry()
	}
	return &OutboxWorker{db: db, registry: registry, interval: interval}
}

// Start launches one ticker goroutine. Repeated calls do not start additional
// workers.
func (w *OutboxWorker) Start(ctx context.Context) {
	if w == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	w.startOnce.Do(func() {
		workerCtx, cancel := context.WithCancel(ctx)
		w.mu.Lock()
		w.cancel = cancel
		w.mu.Unlock()

		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			tick := time.NewTicker(w.interval)
			defer tick.Stop()

			for {
				select {
				case <-workerCtx.Done():
					return
				case <-tick.C:
					_ = w.Tick(workerCtx)
				}
			}
		}()
	})
}

// Stop cancels the worker and waits up to 1s for the goroutine to exit.
func (w *OutboxWorker) Stop() {
	if w == nil {
		return
	}

	w.mu.Lock()
	cancel := w.cancel
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}

	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
	}
}

// Tick performs one scan of due pending rows and attempts delivery.
func (w *OutboxWorker) Tick(ctx context.Context) error {
	if w == nil || w.db == nil {
		return errors.New("outbox worker db is nil")
	}
	if w.registry == nil {
		return errors.New("outbox worker registry is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	now := nowFn()
	var rows []models.NotificationOutbox
	if err := w.db.WithContext(ctx).
		Where("status = ? AND next_retry_at <= ?", outboxStatusPending, now).
		Order("id ASC").
		Find(&rows).Error; err != nil {
		return fmt.Errorf("查询通知 outbox 失败: %w", err)
	}

	for i := range rows {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := w.deliverOne(ctx, rows[i], now); err != nil {
			return err
		}
	}
	return nil
}

func (w *OutboxWorker) deliverOne(ctx context.Context, row models.NotificationOutbox, now time.Time) error {
	var conf models.NotificationConf
	if err := w.db.WithContext(ctx).First(&conf, row.NotificationConfID).Error; err != nil {
		return w.markFailure(ctx, row, now, fmt.Errorf("加载通知通道配置失败: %w", err))
	}

	var notification Notification
	if err := json.Unmarshal([]byte(row.PayloadJSON), &notification); err != nil {
		return w.markFailure(ctx, row, now, fmt.Errorf("解析通知 payload 失败: %w", err))
	}
	if notification.ChannelType == "" {
		notification.ChannelType = conf.ChannelType
	}
	if notification.SourceConfID == 0 {
		notification.SourceConfID = conf.ID
	}

	if w.live != nil {
		if err := w.live.Send(ctx, conf.ID, notification); err != nil {
			return w.markFailure(ctx, row, now, err)
		}
	} else if err := w.sendViaNewChannel(ctx, conf, notification); err != nil {
		return w.markFailure(ctx, row, now, err)
	}

	return w.db.WithContext(ctx).Model(&models.NotificationOutbox{}).
		Where("id = ? AND status = ?", row.ID, outboxStatusPending).
		Updates(map[string]any{
			"status":    outboxStatusSent,
			"sent_at":   now,
			"error_msg": "",
		}).Error
}

// sendViaNewChannel 是没有注入 LiveSender 时的回退：按配置新建一个通道实例发送，用完关闭。
// 只适合无状态通道（webhook、企业微信），有状态通道在生产环境走 LiveSender。
func (w *OutboxWorker) sendViaNewChannel(ctx context.Context, conf models.NotificationConf, n Notification) error {
	if w.decrypt != nil && conf.ConfigJSON != "" {
		plain, err := w.decrypt(conf.ConfigJSON)
		if err != nil {
			return fmt.Errorf("解密通知通道配置失败: %w", err)
		}
		conf.ConfigJSON = plain
	}
	ch, err := w.registry.Make(conf.ChannelType)
	if err != nil {
		return err
	}
	if err := ch.Init(ctx, &conf); err != nil {
		return err
	}
	defer func() { _ = ch.Close(ctx) }()
	return ch.Send(ctx, n)
}

func (w *OutboxWorker) markFailure(ctx context.Context, row models.NotificationOutbox, now time.Time, cause error) error {
	errorMsg := truncateError(cause)
	if row.RetryCount >= len(backoffSchedule) {
		return w.db.WithContext(ctx).Model(&models.NotificationOutbox{}).
			Where("id = ? AND status = ?", row.ID, outboxStatusPending).
			Updates(map[string]any{
				"status":    outboxStatusDead,
				"error_msg": errorMsg,
			}).Error
	}

	nextRetry := row.RetryCount + 1
	delay := backoffSchedule[row.RetryCount]
	return w.db.WithContext(ctx).Model(&models.NotificationOutbox{}).
		Where("id = ? AND status = ?", row.ID, outboxStatusPending).
		Updates(map[string]any{
			"status":        outboxStatusPending,
			"retry_count":   nextRetry,
			"next_retry_at": now.Add(delay),
			"error_msg":     errorMsg,
		}).Error
}

func truncateError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) <= maxErrorMsgLen {
		return msg
	}
	return msg[:maxErrorMsgLen]
}
