// MIT License
// Copyright (c) 2025 pt-tools

// Extra coverage for the RSSNotifier: SetQuietFn quiet-hours deferral,
// hourly-quota throttling, formatRemaining rendering, and the free-torrent
// filtered payload with a discount end time.

package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestRSSNotifier_QuietHoursDefersDelivery(t *testing.T) {
	db := setupRSSNotifierDB(t)
	push := &capturePushService{}
	n := NewRSSNotifier(db, push).(*rssNotifier)
	n.SetQuietFn(func(_ uint) (string, string, error) { return "00:00", "23:59", nil })

	rss := &models.RSSConfig{ID: 1, NotifyMode: "all", NotifyConfIDs: "[7]", MaxNotificationsPerHour: 100}
	require.NoError(t, n.NotifyNewItem(context.Background(), RSSItemEvent{
		RSS: rss, FeedItem: newFeedItem(), SiteName: "example", TorrentID: "quiet-1",
	}))

	// Quiet window -> row stays pending, no immediate push.
	var row models.RSSNotificationLog
	require.NoError(t, db.Where("torrent_id = ?", "quiet-1").First(&row).Error)
	assert.Equal(t, "pending", row.Result)
	require.NotNil(t, row.NextRetryAt)
	assert.Equal(t, 0, push.callCount())
}

func TestRSSNotifier_HourlyQuotaThrottles(t *testing.T) {
	db := setupRSSNotifierDB(t)
	push := &capturePushService{}
	n := NewRSSNotifier(db, push)

	now := time.Now()
	// Seed one recent row so the quota (max=1) is already reached.
	require.NoError(t, db.Create(&models.RSSNotificationLog{
		RSSID: 1, SiteName: "example", TorrentID: "old", NotifyKind: "all",
		NotificationConfID: 7, Result: "sent", CreatedAt: now, UpdatedAt: now,
	}).Error)

	rss := &models.RSSConfig{ID: 1, NotifyMode: "all", NotifyConfIDs: "[7]", MaxNotificationsPerHour: 1}
	require.NoError(t, n.NotifyNewItem(context.Background(), RSSItemEvent{
		RSS: rss, FeedItem: newFeedItem(), SiteName: "example", TorrentID: "new",
	}))

	var throttled models.RSSNotificationLog
	require.NoError(t, db.Where("torrent_id = ?", "new").First(&throttled).Error)
	assert.Equal(t, "throttled", throttled.Result)
	assert.Equal(t, 0, push.callCount())
}

func TestRSSNotifier_FilteredFreeTorrentRendersRemaining(t *testing.T) {
	db := setupRSSNotifierDB(t)
	push := &capturePushService{}
	n := NewRSSNotifier(db, push)

	end := time.Now().Add(3 * time.Hour)
	torrent := &v2.TorrentItem{
		ID: "f1", Title: "FreeMovie", URL: "https://x/details.php?id=1",
		SizeBytes: 2 * 1024 * 1024 * 1024, DiscountLevel: v2.DiscountFree, DiscountEndTime: end,
	}
	rss := &models.RSSConfig{ID: 1, NotifyMode: "filtered", NotifyConfIDs: "[7]", MaxNotificationsPerHour: 100}
	require.NoError(t, n.NotifyFilteredItem(context.Background(), RSSFilteredEvent{
		RSS: rss, Torrent: torrent, SiteName: "example", TorrentID: "f1",
	}))

	require.Equal(t, 1, push.callCount())
	push.mu.Lock()
	text := push.calls[0].Text
	push.mu.Unlock()
	assert.Contains(t, text, "免费")
	assert.Contains(t, text, "剩余")
}

func TestFormatRemaining(t *testing.T) {
	assert.Equal(t, "已结束", formatRemaining(time.Now().Add(-time.Hour)))
	assert.Contains(t, formatRemaining(time.Now().Add(90*time.Minute)), "h")
	assert.Contains(t, formatRemaining(time.Now().Add(30*time.Minute)), "min")
}

func TestRSSNotifier_NilGuards(t *testing.T) {
	db := setupRSSNotifierDB(t)
	n := NewRSSNotifier(db, &capturePushService{})

	require.Error(t, n.NotifyNewItem(context.Background(), RSSItemEvent{}))
	require.Error(t, n.NotifyNewItem(context.Background(), RSSItemEvent{RSS: &models.RSSConfig{}}))
	require.Error(t, n.NotifyFilteredItem(context.Background(), RSSFilteredEvent{}))
	require.Error(t, n.NotifyFilteredItem(context.Background(), RSSFilteredEvent{RSS: &models.RSSConfig{}}))
}
