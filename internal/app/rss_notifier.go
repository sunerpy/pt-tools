package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
}

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
		Buttons: [][]notify.Button{{
			{Text: "立即下载", CallbackData: fmt.Sprintf("dl:%d", row.ID)},
			{Text: "忽略", CallbackData: fmt.Sprintf("ig:%d", row.ID)},
		}},
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
