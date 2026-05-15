package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

// rssRetryBackoff 是 result='pending' 行的指数退避调度（秒）。
// 第 N 次失败后调度到 rssRetryBackoff[N]，索引超界后取最后一项。
var rssRetryBackoff = []time.Duration{
	5 * time.Second,
	10 * time.Second,
	20 * time.Second,
	40 * time.Second,
	80 * time.Second,
}

const rssRetryMaxAttempts = 5

// RSSRetryWorker 周期性扫描 rss_notification_log 中 result='pending' 且 next_retry_at <= now
// 的行，复用 payload_json 重新尝试投递；连续失败超过 rssRetryMaxAttempts 后标记 'failed'。
type RSSRetryWorker struct {
	db        *gorm.DB
	notifySvc NotificationServiceForRSS
	interval  time.Duration
	now       func() time.Time
}

func NewRSSRetryWorker(db *gorm.DB, notifySvc NotificationServiceForRSS) *RSSRetryWorker {
	return &RSSRetryWorker{
		db:        db,
		notifySvc: notifySvc,
		interval:  10 * time.Second,
		now:       time.Now,
	}
}

// Run 阻塞直到 ctx 取消，应作为 goroutine 启动。
func (w *RSSRetryWorker) Run(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = w.drainOnce(ctx)
		}
	}
}

func (w *RSSRetryWorker) drainOnce(ctx context.Context) error {
	now := w.now()
	var rows []models.RSSNotificationLog
	err := w.db.WithContext(ctx).
		Where("result = ? AND (next_retry_at IS NULL OR next_retry_at <= ?)", "pending", now).
		Order("created_at ASC").
		Limit(100).
		Find(&rows).Error
	if err != nil {
		return err
	}
	for i := range rows {
		w.attemptOne(ctx, &rows[i])
	}
	return nil
}

func (w *RSSRetryWorker) attemptOne(ctx context.Context, row *models.RSSNotificationLog) {
	if row.PayloadJSON == "" {
		w.markFailed(ctx, row, errors.New("missing payload_json"))
		return
	}
	var payload renderedNotice
	if err := json.Unmarshal([]byte(row.PayloadJSON), &payload); err != nil {
		w.markFailed(ctx, row, err)
		return
	}
	err := w.notifySvc.Push(ctx, Notification{
		Title:        payload.Title,
		Text:         payload.Text,
		SourceConfID: row.NotificationConfID,
	})
	now := w.now()
	if err == nil {
		w.db.WithContext(ctx).Model(&models.RSSNotificationLog{}).
			Where("id = ?", row.ID).
			Updates(map[string]any{
				"result":       "sent",
				"delivered_at": now,
				"updated_at":   now,
				"attempts":     row.Attempts + 1,
			})
		return
	}
	nextAttempt := row.Attempts + 1
	if nextAttempt >= rssRetryMaxAttempts {
		w.markFailed(ctx, row, err)
		return
	}
	backoffIdx := nextAttempt
	if backoffIdx >= len(rssRetryBackoff) {
		backoffIdx = len(rssRetryBackoff) - 1
	}
	nextRetry := now.Add(rssRetryBackoff[backoffIdx])
	w.db.WithContext(ctx).Model(&models.RSSNotificationLog{}).
		Where("id = ?", row.ID).
		Updates(map[string]any{
			"attempts":      nextAttempt,
			"next_retry_at": nextRetry,
			"last_error":    err.Error(),
			"updated_at":    now,
		})
}

func (w *RSSRetryWorker) markFailed(ctx context.Context, row *models.RSSNotificationLog, err error) {
	now := w.now()
	w.db.WithContext(ctx).Model(&models.RSSNotificationLog{}).
		Where("id = ?", row.ID).
		Updates(map[string]any{
			"result":     "failed",
			"last_error": err.Error(),
			"attempts":   row.Attempts + 1,
			"updated_at": now,
		})
}
