package app

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

func setupRetryWorkerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.RSSNotificationLog{}))
	return db
}

type fakePushSvc struct {
	calls int32
	err   error
	hook  func(n Notification) error
}

func (f *fakePushSvc) Push(_ context.Context, n Notification) error {
	atomic.AddInt32(&f.calls, 1)
	if f.hook != nil {
		return f.hook(n)
	}
	return f.err
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

func TestRSSRetryWorker_PendingPastRetry_PushSucceeds(t *testing.T) {
	db := setupRetryWorkerDB(t)
	past := time.Now().Add(-1 * time.Minute)
	row := models.RSSNotificationLog{
		RSSID: 1, SiteName: "s", TorrentID: "t1", NotifyKind: "all",
		NotificationConfID: 1, Result: "pending", Attempts: 0,
		NextRetryAt: &past,
		PayloadJSON: mustJSON(t, renderedNotice{Title: "T", Text: "Body"}),
		CreatedAt:   time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&row).Error)

	svc := &fakePushSvc{}
	w := NewRSSRetryWorker(db, svc)
	require.NoError(t, w.drainOnce(context.Background()))
	assert.Equal(t, int32(1), atomic.LoadInt32(&svc.calls))

	var got models.RSSNotificationLog
	require.NoError(t, db.First(&got, row.ID).Error)
	assert.Equal(t, "sent", got.Result)
	assert.NotNil(t, got.DeliveredAt)
	assert.Equal(t, 1, got.Attempts)
}

func TestRSSRetryWorker_PushFails_BackoffScheduled(t *testing.T) {
	db := setupRetryWorkerDB(t)
	past := time.Now().Add(-1 * time.Minute)
	row := models.RSSNotificationLog{
		RSSID: 1, SiteName: "s", TorrentID: "t2", NotifyKind: "all",
		NotificationConfID: 1, Result: "pending", Attempts: 0,
		NextRetryAt: &past,
		PayloadJSON: mustJSON(t, renderedNotice{Title: "T", Text: "Body"}),
		CreatedAt:   time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&row).Error)

	svc := &fakePushSvc{err: errors.New("boom")}
	w := NewRSSRetryWorker(db, svc)
	w.now = func() time.Time { return time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC) }
	require.NoError(t, w.drainOnce(context.Background()))

	var got models.RSSNotificationLog
	require.NoError(t, db.First(&got, row.ID).Error)
	assert.Equal(t, "pending", got.Result)
	assert.Equal(t, 1, got.Attempts)
	assert.Contains(t, got.LastError, "boom")
	require.NotNil(t, got.NextRetryAt)
	expected := time.Date(2026, 5, 16, 12, 0, 10, 0, time.UTC)
	assert.True(t, got.NextRetryAt.Equal(expected),
		"expected next_retry_at=%v got %v", expected, *got.NextRetryAt)
}

func TestRSSRetryWorker_FifthFailureMarksFailed(t *testing.T) {
	db := setupRetryWorkerDB(t)
	past := time.Now().Add(-1 * time.Minute)
	row := models.RSSNotificationLog{
		RSSID: 1, SiteName: "s", TorrentID: "t3", NotifyKind: "all",
		NotificationConfID: 1, Result: "pending", Attempts: 4,
		NextRetryAt: &past,
		PayloadJSON: mustJSON(t, renderedNotice{Title: "T", Text: "B"}),
		CreatedAt:   time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&row).Error)

	svc := &fakePushSvc{err: errors.New("permafail")}
	w := NewRSSRetryWorker(db, svc)
	require.NoError(t, w.drainOnce(context.Background()))

	var got models.RSSNotificationLog
	require.NoError(t, db.First(&got, row.ID).Error)
	assert.Equal(t, "failed", got.Result)
	assert.Equal(t, 5, got.Attempts)
	assert.Contains(t, got.LastError, "permafail")
}

func TestRSSRetryWorker_FutureNextRetry_NotPickedUp(t *testing.T) {
	db := setupRetryWorkerDB(t)
	future := time.Now().Add(1 * time.Hour)
	row := models.RSSNotificationLog{
		RSSID: 1, SiteName: "s", TorrentID: "t4", NotifyKind: "all",
		NotificationConfID: 1, Result: "pending", Attempts: 1,
		NextRetryAt: &future,
		PayloadJSON: mustJSON(t, renderedNotice{Title: "T", Text: "B"}),
		CreatedAt:   time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&row).Error)

	svc := &fakePushSvc{}
	w := NewRSSRetryWorker(db, svc)
	require.NoError(t, w.drainOnce(context.Background()))
	assert.Equal(t, int32(0), atomic.LoadInt32(&svc.calls))

	var got models.RSSNotificationLog
	require.NoError(t, db.First(&got, row.ID).Error)
	assert.Equal(t, "pending", got.Result)
	assert.Equal(t, 1, got.Attempts)
}

func TestRSSRetryWorker_MissingPayload_MarkedFailed(t *testing.T) {
	db := setupRetryWorkerDB(t)
	past := time.Now().Add(-1 * time.Minute)
	row := models.RSSNotificationLog{
		RSSID: 1, SiteName: "s", TorrentID: "t5", NotifyKind: "all",
		NotificationConfID: 1, Result: "pending", Attempts: 0,
		NextRetryAt: &past,
		PayloadJSON: "",
		CreatedAt:   time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&row).Error)

	svc := &fakePushSvc{}
	w := NewRSSRetryWorker(db, svc)
	require.NoError(t, w.drainOnce(context.Background()))
	assert.Equal(t, int32(0), atomic.LoadInt32(&svc.calls))

	var got models.RSSNotificationLog
	require.NoError(t, db.First(&got, row.ID).Error)
	assert.Equal(t, "failed", got.Result)
	assert.Contains(t, got.LastError, "payload")
}
