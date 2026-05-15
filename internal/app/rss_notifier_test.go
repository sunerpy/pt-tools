package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func setupRSSNotifierDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.RSSSubscription{},
		&models.RSSNotificationLog{},
		&models.NotificationConf{},
		&models.NotificationOutbox{},
	))
	return db
}

type capturePushService struct {
	mu    sync.Mutex
	calls []Notification
	err   error
}

func (c *capturePushService) Push(_ context.Context, n Notification) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, n)
	return c.err
}

func (c *capturePushService) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

func newFeedItem() *gofeed.Item {
	pub := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	return &gofeed.Item{
		Title:           "Test.Movie.2026.1080p",
		Link:            "https://example.com/details.php?id=12345",
		GUID:            "guid-12345",
		Published:       pub.Format(time.RFC1123Z),
		PublishedParsed: &pub,
	}
}

func TestRSSNotifier_NotifyMode_Empty_NoOp(t *testing.T) {
	db := setupRSSNotifierDB(t)
	push := &capturePushService{}
	notifier := NewRSSNotifier(db, push)

	rss := &models.RSSConfig{ID: 1, NotifyMode: "", NotifyConfIDs: "[1]"}
	require.NoError(t, notifier.NotifyNewItem(context.Background(), RSSItemEvent{
		RSS: rss, FeedItem: newFeedItem(), SiteName: "example", TorrentID: "12345",
	}))

	var cnt int64
	require.NoError(t, db.Model(&models.RSSNotificationLog{}).Count(&cnt).Error)
	assert.EqualValues(t, 0, cnt)
	assert.Equal(t, 0, push.callCount())
}

func TestRSSNotifier_NotifyAll_EmptyConfIDs_NoOp(t *testing.T) {
	db := setupRSSNotifierDB(t)
	push := &capturePushService{}
	notifier := NewRSSNotifier(db, push)

	rss := &models.RSSConfig{ID: 1, NotifyMode: "all", NotifyConfIDs: "[]"}
	require.NoError(t, notifier.NotifyNewItem(context.Background(), RSSItemEvent{
		RSS: rss, FeedItem: newFeedItem(), SiteName: "example", TorrentID: "12345",
	}))

	var cnt int64
	require.NoError(t, db.Model(&models.RSSNotificationLog{}).Count(&cnt).Error)
	assert.EqualValues(t, 0, cnt)
	assert.Equal(t, 0, push.callCount())
}

func TestRSSNotifier_NotifyAll_FreshItem_Sent(t *testing.T) {
	db := setupRSSNotifierDB(t)
	push := &capturePushService{}
	notifier := NewRSSNotifier(db, push)

	rss := &models.RSSConfig{ID: 1, NotifyMode: "all", NotifyConfIDs: "[7]", MaxNotificationsPerHour: 100}
	require.NoError(t, notifier.NotifyNewItem(context.Background(), RSSItemEvent{
		RSS: rss, FeedItem: newFeedItem(), SiteName: "example", TorrentID: "12345",
	}))

	var rows []models.RSSNotificationLog
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, "sent", rows[0].Result)
	assert.Equal(t, "all", rows[0].NotifyKind)
	assert.EqualValues(t, 7, rows[0].NotificationConfID)
	assert.NotNil(t, rows[0].DeliveredAt)
	assert.Equal(t, 1, push.callCount())
}

func TestRSSNotifier_NotifyAll_DuplicateItem_NoSecondPush(t *testing.T) {
	db := setupRSSNotifierDB(t)
	push := &capturePushService{}
	notifier := NewRSSNotifier(db, push)

	rss := &models.RSSConfig{ID: 1, NotifyMode: "all", NotifyConfIDs: "[7]", MaxNotificationsPerHour: 100}
	ev := RSSItemEvent{RSS: rss, FeedItem: newFeedItem(), SiteName: "example", TorrentID: "12345"}

	require.NoError(t, notifier.NotifyNewItem(context.Background(), ev))
	require.NoError(t, notifier.NotifyNewItem(context.Background(), ev))

	var cnt int64
	require.NoError(t, db.Model(&models.RSSNotificationLog{}).Count(&cnt).Error)
	assert.EqualValues(t, 1, cnt)
	assert.Equal(t, 1, push.callCount())
}

func TestRSSNotifier_NotifyAll_QuotaExceeded_Throttled(t *testing.T) {
	db := setupRSSNotifierDB(t)
	push := &capturePushService{}
	notifier := NewRSSNotifier(db, push)

	now := time.Now()
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Create(&models.RSSNotificationLog{
			RSSID: 1, SiteName: "example", TorrentID: "old-" + string(rune('A'+i)),
			NotifyKind: "all", NotificationConfID: 7,
			Result: "sent", CreatedAt: now, UpdatedAt: now,
		}).Error)
	}

	rss := &models.RSSConfig{ID: 1, NotifyMode: "all", NotifyConfIDs: "[7]", MaxNotificationsPerHour: 2}
	require.NoError(t, notifier.NotifyNewItem(context.Background(), RSSItemEvent{
		RSS: rss, FeedItem: newFeedItem(), SiteName: "example", TorrentID: "new-1",
	}))

	var rows []models.RSSNotificationLog
	require.NoError(t, db.Where("torrent_id = ?", "new-1").Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, "throttled", rows[0].Result)
	assert.Equal(t, 0, push.callCount())
}

func TestRSSNotifier_NotifyBoth_FilteredSuppressesPendingAll(t *testing.T) {
	db := setupRSSNotifierDB(t)
	push := &capturePushService{}
	notifier := NewRSSNotifier(db, push)

	now := time.Now()
	require.NoError(t, db.Create(&models.RSSNotificationLog{
		RSSID: 1, SiteName: "example", TorrentID: "12345",
		NotifyKind: "all", NotificationConfID: 7,
		Result: "pending", CreatedAt: now, UpdatedAt: now,
	}).Error)

	rss := &models.RSSConfig{ID: 1, NotifyMode: "both", NotifyConfIDs: "[7]", MaxNotificationsPerHour: 100}
	torrent := &v2.TorrentItem{
		ID:        "12345",
		Title:     "Test.Movie.2026.1080p",
		URL:       "https://example.com/details.php?id=12345",
		SizeBytes: 1024 * 1024 * 1024,
	}
	rule := &models.FilterRule{ID: 9, Name: "tv-rule"}
	require.NoError(t, notifier.NotifyFilteredItem(context.Background(), RSSFilteredEvent{
		RSS: rss, Torrent: torrent, Rule: rule, SiteName: "example", TorrentID: "12345",
	}))

	var allRow models.RSSNotificationLog
	require.NoError(t, db.Where("notify_kind = ?", "all").First(&allRow).Error)
	assert.Equal(t, "suppressed", allRow.Result)

	var filteredRow models.RSSNotificationLog
	require.NoError(t, db.Where("notify_kind = ?", "filtered").First(&filteredRow).Error)
	assert.Equal(t, "sent", filteredRow.Result)
	require.NotNil(t, filteredRow.MatchedFilterRuleID)
	assert.EqualValues(t, 9, *filteredRow.MatchedFilterRuleID)
	assert.Equal(t, 1, push.callCount())
}

func TestRSSNotifier_NotifyFilteredOnly_NoSuppression(t *testing.T) {
	db := setupRSSNotifierDB(t)
	push := &capturePushService{}
	notifier := NewRSSNotifier(db, push)

	rss := &models.RSSConfig{ID: 1, NotifyMode: "filtered", NotifyConfIDs: "[7]", MaxNotificationsPerHour: 100}
	torrent := &v2.TorrentItem{
		ID:        "12345",
		Title:     "Movie",
		URL:       "https://example.com/details.php?id=12345",
		SizeBytes: 500 * 1024 * 1024,
	}
	require.NoError(t, notifier.NotifyFilteredItem(context.Background(), RSSFilteredEvent{
		RSS: rss, Torrent: torrent, SiteName: "example", TorrentID: "12345",
	}))

	var rows []models.RSSNotificationLog
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, "filtered", rows[0].NotifyKind)
	assert.Equal(t, "sent", rows[0].Result)
	assert.Equal(t, 1, push.callCount())
}
