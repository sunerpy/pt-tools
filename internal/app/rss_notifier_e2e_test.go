// MIT License
// Copyright (c) 2025 pt-tools

package app

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func setupE2EDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.RSSSubscription{},
		&models.RSSNotificationLog{},
		&models.NotificationConf{},
		&models.NotificationOutbox{},
		&models.FilterRule{},
		&models.DownloaderSetting{},
	))
	return db
}

func TestE2E_RSSPipeline_AllMode_DigestSendsAndMarksSent(t *testing.T) {
	db := setupE2EDB(t)
	push := &capturePushService{}
	notifier := NewRSSNotifier(db, push).(*rssNotifier)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	flushed := make(chan struct{}, 1)
	flushFn := func(fctx context.Context, confID uint, items []notify.DigestItem) {
		title, text := notify.CombineDigest(items)
		err := push.Push(fctx, Notification{Title: title, Text: text, SourceConfID: confID})
		now := time.Now()
		ids := make([]uint, len(items))
		for i, it := range items {
			ids[i] = it.LogID
		}
		upd := map[string]any{"updated_at": now, "attempts": gorm.Expr("attempts + 1")}
		if err == nil {
			upd["result"] = "sent"
			upd["delivered_at"] = now
		} else {
			upd["last_error"] = err.Error()
		}
		require.NoError(t, db.Model(&models.RSSNotificationLog{}).
			Where("id IN ?", ids).Updates(upd).Error)
		select {
		case flushed <- struct{}{}:
		default:
		}
	}
	digest := notify.NewDigestBufferWithWindow(ctx, flushFn, 50*time.Millisecond, 5)
	notifier.SetDigestBuffer(digest)

	rss := &models.RSSConfig{ID: 1, NotifyMode: "all", NotifyConfIDs: "[7]", MaxNotificationsPerHour: 100}
	require.NoError(t, notifier.NotifyNewItem(ctx, RSSItemEvent{
		RSS: rss, FeedItem: newFeedItem(), SiteName: "example", TorrentID: "T-1",
	}))

	select {
	case <-flushed:
	case <-time.After(2 * time.Second):
		t.Fatal("digest 未在 2s 内 flush")
	}

	var rows []models.RSSNotificationLog
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, "sent", rows[0].Result)
	assert.Equal(t, "all", rows[0].NotifyKind)
}

func TestE2E_RSSPipeline_BothMode_FilteredSuppressesAll(t *testing.T) {
	db := setupE2EDB(t)
	push := &capturePushService{}
	notifier := NewRSSNotifier(db, push)

	rss := &models.RSSConfig{ID: 1, NotifyMode: "both", NotifyConfIDs: "[7]", MaxNotificationsPerHour: 100}
	require.NoError(t, notifier.NotifyNewItem(context.Background(), RSSItemEvent{
		RSS: rss, FeedItem: newFeedItem(), SiteName: "example", TorrentID: "T-FX",
	}))

	var allRow models.RSSNotificationLog
	require.NoError(t, db.Where("notify_kind = ?", "all").First(&allRow).Error)
	assert.Equal(t, "sent", allRow.Result)

	rule := &models.FilterRule{ID: 9, Name: "tv-rule"}
	torrent := &v2.TorrentItem{
		ID: "T-FX", Title: "Test.Movie.2026.1080p",
		URL: "https://example.com/details.php?id=T-FX", SizeBytes: 1024 * 1024 * 1024,
	}
	require.NoError(t, notifier.NotifyFilteredItem(context.Background(), RSSFilteredEvent{
		RSS: rss, Torrent: torrent, Rule: rule, SiteName: "example", TorrentID: "T-FX",
	}))

	var refreshed models.RSSNotificationLog
	require.NoError(t, db.Where("id = ?", allRow.ID).First(&refreshed).Error)
	assert.Equal(t, "sent", refreshed.Result, "completed all rows are not retroactively suppressed")

	var filteredRow models.RSSNotificationLog
	require.NoError(t, db.Where("notify_kind = ?", "filtered").First(&filteredRow).Error)
	assert.Equal(t, "sent", filteredRow.Result)
	require.NotNil(t, filteredRow.MatchedFilterRuleID)
	assert.EqualValues(t, 9, *filteredRow.MatchedFilterRuleID)
	assert.Equal(t, 2, push.callCount())
}

func TestE2E_RSSPipeline_BothMode_FilteredSuppressesPendingAll(t *testing.T) {
	db := setupE2EDB(t)
	push := &capturePushService{}
	notifier := NewRSSNotifier(db, push)

	now := time.Now()
	require.NoError(t, db.Create(&models.RSSNotificationLog{
		RSSID: 1, SiteName: "example", TorrentID: "T-PEND",
		NotifyKind: "all", NotificationConfID: 7,
		Result: "pending", CreatedAt: now, UpdatedAt: now,
	}).Error)

	rss := &models.RSSConfig{ID: 1, NotifyMode: "both", NotifyConfIDs: "[7]", MaxNotificationsPerHour: 100}
	torrent := &v2.TorrentItem{
		ID: "T-PEND", Title: "Movie",
		URL: "https://example.com/details.php?id=T-PEND", SizeBytes: 100,
	}
	require.NoError(t, notifier.NotifyFilteredItem(context.Background(), RSSFilteredEvent{
		RSS: rss, Torrent: torrent, SiteName: "example", TorrentID: "T-PEND",
	}))

	var allRow models.RSSNotificationLog
	require.NoError(t, db.Where("notify_kind = ?", "all").First(&allRow).Error)
	assert.Equal(t, "suppressed", allRow.Result)
}

func TestE2E_RSSPipeline_Throttle_OverQuotaMarksThrottled(t *testing.T) {
	db := setupE2EDB(t)
	push := &capturePushService{}
	notifier := NewRSSNotifier(db, push)

	rss := &models.RSSConfig{ID: 42, NotifyMode: "all", NotifyConfIDs: "[7]", MaxNotificationsPerHour: 2}
	for i := 0; i < 3; i++ {
		require.NoError(t, notifier.NotifyNewItem(context.Background(), RSSItemEvent{
			RSS: rss, FeedItem: newFeedItem(), SiteName: "example",
			TorrentID: fmt.Sprintf("T-Q%d", i),
		}))
	}

	var sentCnt, throttledCnt int64
	require.NoError(t, db.Model(&models.RSSNotificationLog{}).Where("result = ?", "sent").Count(&sentCnt).Error)
	require.NoError(t, db.Model(&models.RSSNotificationLog{}).Where("result = ?", "throttled").Count(&throttledCnt).Error)
	assert.EqualValues(t, 2, sentCnt)
	assert.EqualValues(t, 1, throttledCnt)
}

func TestE2E_RSSPipeline_Retry_FiveFailuresMarksFailed(t *testing.T) {
	db := setupE2EDB(t)
	failingPush := &fakePushSvc{err: errors.New("always fails")}

	now := time.Now()
	row := models.RSSNotificationLog{
		RSSID: 1, SiteName: "example", TorrentID: "T-RETRY",
		NotifyKind: "all", NotificationConfID: 7,
		Result: "pending", Attempts: 0,
		NextRetryAt: &now,
		PayloadJSON: mustJSON(t, renderedNotice{Title: "T", Text: "Body"}),
		CreatedAt:   now, UpdatedAt: now,
	}
	require.NoError(t, db.Create(&row).Error)

	w := NewRSSRetryWorker(db, failingPush)
	clockTick := now
	w.now = func() time.Time { return clockTick }

	for i := 0; i < 6; i++ {
		clockTick = clockTick.Add(2 * time.Minute)
		require.NoError(t, w.drainOnce(context.Background()))
	}

	var got models.RSSNotificationLog
	require.NoError(t, db.First(&got, row.ID).Error)
	assert.Equal(t, "failed", got.Result, "after rssRetryMaxAttempts the row must be marked failed")
	assert.GreaterOrEqual(t, got.Attempts, rssRetryMaxAttempts)
	assert.NotEmpty(t, got.LastError)
	assert.GreaterOrEqual(t, atomic.LoadInt32(&failingPush.calls), int32(rssRetryMaxAttempts))
}
