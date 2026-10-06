package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

type RSSItemEvent struct {
	RSS       *models.RSSConfig
	FeedItem  *gofeed.Item
	SiteName  string
	TorrentID string
}

type RSSFilteredEvent struct {
	RSS       *models.RSSConfig
	Torrent   *v2.TorrentItem
	Rule      *models.FilterRule
	SiteName  string
	TorrentID string
}

type RSSNotifier interface {
	NotifyNewItem(ctx context.Context, ev RSSItemEvent) error
	NotifyFilteredItem(ctx context.Context, ev RSSFilteredEvent) error
}

// QuietLookupFunc 返回指定 NotificationConf 的 quiet_hours_start / quiet_hours_end。
// 用于在 tryDispatch 中按通道判断是否处于静默窗口。
type QuietLookupFunc func(confID uint) (start, end string, err error)

type NotificationServiceForRSS interface {
	Push(ctx context.Context, n Notification) error
}

type rssNotifier struct {
	db        *gorm.DB
	notifySvc NotificationServiceForRSS
	now       func() time.Time
	digestBuf *notify.DigestBuffer
	quietFn   QuietLookupFunc
	// quotaMu 把「数配额 → 写日志行」串成一个临界区：RSS 管线多个 worker 并发处理不同种子时，
	// 分开做会都读到未超限，然后一起写入，突破每小时上限。只在设置了上限时加锁。
	quotaMu sync.Mutex
}

// rssDigestHold 是交给 DigestBuffer 的行暂不参与重试的时长：比合并窗口长，
// 窗口内由摘要统一发送；进程在刷写前退出时，过了这段时间重试 worker 会逐条补发。
const rssDigestHold = notify.DigestWindow + time.Minute

func NewRSSNotifier(db *gorm.DB, notifySvc NotificationServiceForRSS) RSSNotifier {
	return &rssNotifier{db: db, notifySvc: notifySvc, now: time.Now}
}

// SetDigestBuffer 注入 DigestBuffer，启用异步合并发送路径。
// 未注入时回退到 S1 同步直发，保持已有单测兼容。
func (r *rssNotifier) SetDigestBuffer(b *notify.DigestBuffer) { r.digestBuf = b }

// SetQuietFn 注入 quiet_hours 查询函数。未注入时跳过静默判断。
func (r *rssNotifier) SetQuietFn(fn QuietLookupFunc) { r.quietFn = fn }

func (r *rssNotifier) NotifyNewItem(ctx context.Context, ev RSSItemEvent) error {
	if ev.RSS == nil {
		return errors.New("rss is nil")
	}
	if ev.FeedItem == nil {
		return errors.New("feed item is nil")
	}
	mode := ev.RSS.NotifyMode
	if mode != "all" && mode != "both" {
		return nil
	}
	confIDs, err := parseConfIDs(ev.RSS.NotifyConfIDs)
	if err != nil {
		return fmt.Errorf("解析 notify_conf_ids: %w", err)
	}
	if len(confIDs) == 0 {
		return nil
	}
	if ev.RSS.MaxNotificationsPerHour > 0 {
		r.quotaMu.Lock()
		defer r.quotaMu.Unlock()
	}
	if exceeded, qerr := r.exceededHourlyQuota(ctx, ev.RSS); qerr != nil {
		return qerr
	} else if exceeded {
		return r.recordThrottled(ctx, ev.RSS.ID, ev.SiteName, ev.TorrentID, "all", confIDs[0])
	}
	payload := renderAllPayload(ev)
	payloadJSON, _ := json.Marshal(payload)
	for _, cid := range confIDs {
		_ = r.tryDispatch(ctx, dispatchSpec{
			RSSID: ev.RSS.ID, SiteName: ev.SiteName, TorrentID: ev.TorrentID,
			Kind: "all", ConfID: cid,
			PayloadJSON: string(payloadJSON),
			Title:       payload.Title, Text: payload.Text,
		})
	}
	return nil
}

func (r *rssNotifier) NotifyFilteredItem(ctx context.Context, ev RSSFilteredEvent) error {
	if ev.RSS == nil {
		return errors.New("rss is nil")
	}
	if ev.Torrent == nil {
		return errors.New("torrent is nil")
	}
	mode := ev.RSS.NotifyMode
	if mode != "filtered" && mode != "both" {
		return nil
	}
	confIDs, err := parseConfIDs(ev.RSS.NotifyConfIDs)
	if err != nil {
		return err
	}
	if len(confIDs) == 0 {
		return nil
	}
	if ev.RSS.MaxNotificationsPerHour > 0 {
		r.quotaMu.Lock()
		defer r.quotaMu.Unlock()
	}
	if exceeded, qerr := r.exceededHourlyQuota(ctx, ev.RSS); qerr != nil {
		return qerr
	} else if exceeded {
		return r.recordThrottled(ctx, ev.RSS.ID, ev.SiteName, ev.TorrentID, "filtered", confIDs[0])
	}
	_ = r.db.WithContext(ctx).
		Model(&models.RSSNotificationLog{}).
		Where("rss_id = ? AND site_name = ? AND torrent_id = ? AND notify_kind = ? AND result = ?",
			ev.RSS.ID, ev.SiteName, ev.TorrentID, "all", "pending").
		Update("result", "suppressed").Error

	payload := renderFilteredPayload(ev)
	payloadJSON, _ := json.Marshal(payload)
	var ruleID *uint
	if ev.Rule != nil {
		rid := ev.Rule.ID
		ruleID = &rid
	}
	for _, cid := range confIDs {
		_ = r.tryDispatch(ctx, dispatchSpec{
			RSSID: ev.RSS.ID, SiteName: ev.SiteName, TorrentID: ev.TorrentID,
			Kind: "filtered", ConfID: cid, MatchedRuleID: ruleID,
			PayloadJSON: string(payloadJSON),
			Title:       payload.Title, Text: payload.Text,
		})
	}
	return nil
}

type dispatchSpec struct {
	RSSID         uint
	SiteName      string
	TorrentID     string
	Kind          string
	ConfID        uint
	MatchedRuleID *uint
	PayloadJSON   string
	Title         string
	Text          string
}

func (r *rssNotifier) tryDispatch(ctx context.Context, sp dispatchSpec) error {
	now := r.now()
	row := models.RSSNotificationLog{
		RSSID: sp.RSSID, SiteName: sp.SiteName, TorrentID: sp.TorrentID,
		NotifyKind: sp.Kind, NotificationConfID: sp.ConfID,
		MatchedFilterRuleID: sp.MatchedRuleID,
		Result:              "pending", Attempts: 0,
		PayloadJSON: sp.PayloadJSON,
		NextRetryAt: &now,
		CreatedAt:   now, UpdatedAt: now,
	}
	res := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return nil
	}

	if r.quietFn != nil {
		if start, end, qerr := r.quietFn(sp.ConfID); qerr == nil && notify.IsQuietNow(now, start, end) {
			next := notify.NextQuietEnd(now, end)
			return r.db.WithContext(ctx).Model(&models.RSSNotificationLog{}).
				Where("id = ?", row.ID).
				Updates(map[string]any{
					"next_retry_at": next,
					"updated_at":    r.now(),
				}).Error
		}
	}

	if r.digestBuf != nil {
		// 交给摘要之前先把重试时间推到合并窗口之后：否则这行仍是到期的 pending，
		// 每 10 秒一轮的重试 worker 会在窗口内先逐条发一遍，摘要刷写时再发一遍。
		if err := r.db.WithContext(ctx).Model(&models.RSSNotificationLog{}).
			Where("id = ?", row.ID).
			Updates(map[string]any{"next_retry_at": now.Add(rssDigestHold), "updated_at": r.now()}).Error; err != nil {
			return err
		}
		r.digestBuf.Add(sp.ConfID, notify.DigestItem{
			LogID: row.ID,
			Title: sp.Title,
			Text:  sp.Text,
		})
		return nil
	}

	err := r.notifySvc.Push(ctx, Notification{
		Title: sp.Title, Text: sp.Text,
		SourceConfID: sp.ConfID,
		Buttons:      rssItemButtons(row.ID),
	})
	upd := map[string]any{"updated_at": r.now(), "attempts": 1}
	if err != nil {
		upd["result"] = "failed"
		upd["last_error"] = err.Error()
	} else {
		upd["result"] = "sent"
		upd["delivered_at"] = r.now()
	}
	return r.db.WithContext(ctx).Model(&models.RSSNotificationLog{}).
		Where("id = ?", row.ID).Updates(upd).Error
}

// rssItemButtons 是单条 RSS 通知附带的「立即下载 / 忽略」按钮（Telegram 回调处理）。
func rssItemButtons(logID uint) [][]notify.Button {
	return [][]notify.Button{{
		{Text: "立即下载", CallbackData: fmt.Sprintf("dl:%d", logID)},
		{Text: "忽略", CallbackData: fmt.Sprintf("ig:%d", logID)},
	}}
}

// NewRSSDigestFlush 返回 DigestBuffer 的刷写函数。
//   - 只发送仍是 pending 的行：被 filtered 通知抑制（suppressed）或已由其他路径发出的行跳过，
//     DigestBuffer 里的内存条目不会再把它们发出去
//   - 只剩一条时按单条通知发送并保留「立即下载 / 忽略」按钮
//   - 成功后把这些行条件更新为 sent；失败时 5 秒后交回重试 worker 逐条重试
func NewRSSDigestFlush(db *gorm.DB, notifySvc NotificationServiceForRSS, logf func(format string, args ...any)) notify.DigestFlushFunc {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return func(ctx context.Context, confID uint, items []notify.DigestItem) {
		ids := make([]uint, 0, len(items))
		for _, it := range items {
			ids = append(ids, it.LogID)
		}
		var live []uint
		if err := db.WithContext(ctx).Model(&models.RSSNotificationLog{}).
			Where("id IN ? AND result = ?", ids, "pending").
			Pluck("id", &live).Error; err != nil {
			logf("RSS digest 查询待发送记录失败 conf_id=%d: %v", confID, err)
			return
		}
		keep := make(map[uint]bool, len(live))
		for _, id := range live {
			keep[id] = true
		}
		eligible := make([]notify.DigestItem, 0, len(live))
		for _, it := range items {
			if keep[it.LogID] {
				eligible = append(eligible, it)
			}
		}
		if len(eligible) == 0 {
			return
		}

		n := Notification{SourceConfID: confID}
		if len(eligible) == 1 {
			n.Title, n.Text = eligible[0].Title, eligible[0].Text
			n.Buttons = rssItemButtons(eligible[0].LogID)
		} else {
			n.Title, n.Text = notify.CombineDigest(eligible)
		}
		err := notifySvc.Push(ctx, n)
		now := time.Now()
		if err == nil {
			if uerr := db.WithContext(ctx).Model(&models.RSSNotificationLog{}).
				Where("id IN ? AND result = ?", live, "pending").
				Updates(map[string]any{
					"result":       "sent",
					"delivered_at": now,
					"updated_at":   now,
					"attempts":     gorm.Expr("attempts + 1"),
				}).Error; uerr != nil {
				logf("RSS digest 已投递但更新记录失败 conf_id=%d items=%d: %v", confID, len(eligible), uerr)
				return
			}
			logf("RSS digest 已投递 conf_id=%d items=%d", confID, len(eligible))
			return
		}
		if uerr := db.WithContext(ctx).Model(&models.RSSNotificationLog{}).
			Where("id IN ? AND result = ?", live, "pending").
			Updates(map[string]any{
				"attempts":      gorm.Expr("attempts + 1"),
				"next_retry_at": now.Add(5 * time.Second),
				"last_error":    err.Error(),
				"updated_at":    now,
			}).Error; uerr != nil {
			logf("RSS digest 投递失败且更新记录失败 conf_id=%d: %v / %v", confID, err, uerr)
			return
		}
		logf("RSS digest 投递失败 conf_id=%d items=%d err=%v", confID, len(eligible), err)
	}
}

func (r *rssNotifier) exceededHourlyQuota(ctx context.Context, rss *models.RSSConfig) (bool, error) {
	if rss.MaxNotificationsPerHour <= 0 {
		return false, nil
	}
	cutoff := r.now().Add(-1 * time.Hour)
	var cnt int64
	err := r.db.WithContext(ctx).
		Model(&models.RSSNotificationLog{}).
		Where("rss_id = ? AND created_at > ? AND result IN ?",
			rss.ID, cutoff, []string{"sent", "failed", "pending"}).
		Count(&cnt).Error
	if err != nil {
		return false, err
	}
	return cnt >= int64(rss.MaxNotificationsPerHour), nil
}

func (r *rssNotifier) recordThrottled(ctx context.Context, rssID uint, site, tid, kind string, confID uint) error {
	now := r.now()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&models.RSSNotificationLog{
		RSSID: rssID, SiteName: site, TorrentID: tid,
		NotifyKind: kind, NotificationConfID: confID,
		Result:    "throttled",
		CreatedAt: now, UpdatedAt: now,
	}).Error
}

func parseConfIDs(s string) ([]uint, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "[]" {
		return nil, nil
	}
	var ids []uint
	if err := json.Unmarshal([]byte(s), &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

type renderedNotice struct {
	Title string
	Text  string
}

func renderAllPayload(ev RSSItemEvent) renderedNotice {
	title := ev.FeedItem.Title
	if title == "" {
		title = "(无标题)"
	}
	pubStr := "未知时间"
	if ev.FeedItem.PublishedParsed != nil {
		pubStr = ev.FeedItem.PublishedParsed.Format("2006-01-02 15:04")
	} else if ev.FeedItem.Published != "" {
		pubStr = ev.FeedItem.Published
	}
	text := fmt.Sprintf(
		"🆕 [%s] %s\n\n📅 %s\n🔗 %s",
		ev.SiteName, title, pubStr, ev.FeedItem.Link,
	)
	return renderedNotice{Title: title, Text: text}
}

func renderFilteredPayload(ev RSSFilteredEvent) renderedNotice {
	t := ev.Torrent
	title := t.Title
	if title == "" {
		title = "(无标题)"
	}
	var ruleLine string
	if ev.Rule != nil {
		ruleLine = fmt.Sprintf("\n📌 匹配规则：%s", ev.Rule.Name)
	}
	var freeLine string
	if t.IsFree() {
		if end := t.GetFreeEndTime(); end != nil {
			freeLine = fmt.Sprintf("\n🆓 免费 (剩余 %s)", formatRemaining(*end))
		} else {
			freeLine = "\n🆓 免费"
		}
	}
	text := fmt.Sprintf(
		"🎯 [%s] %s\n\n📦 %s%s%s\n🔗 %s",
		ev.SiteName, title,
		formatBytesRSS(t.SizeBytes), freeLine, ruleLine, t.URL,
	)
	return renderedNotice{Title: title, Text: text}
}

func formatBytesRSS(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatRemaining(end time.Time) string {
	d := time.Until(end)
	if d <= 0 {
		return "已结束"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dmin", h, m)
	}
	return fmt.Sprintf("%dmin", m)
}
