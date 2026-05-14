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

	startOnce sync.Once
	mu        sync.Mutex
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

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

	ch, err := w.registry.Make(conf.ChannelType)
	if err != nil {
		return w.markFailure(ctx, row, now, err)
	}
	if err := ch.Init(ctx, &conf); err != nil {
		return w.markFailure(ctx, row, now, err)
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

	if err := ch.Send(ctx, notification); err != nil {
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
