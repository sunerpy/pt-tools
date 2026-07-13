// MIT License
// Copyright (c) 2025 pt-tools

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

func TestNotifyNewItem_WrongModeSkips(t *testing.T) {
	db := setupRSSNotifierDB(t)
	n := NewRSSNotifier(db, &fakePushSvc{})
	rss := &models.RSSConfig{ID: 1, NotifyMode: "filtered", NotifyConfIDs: "[1]"}
	require.NoError(t, n.NotifyNewItem(context.Background(), RSSItemEvent{
		RSS: rss, SiteName: "s", TorrentID: "t", FeedItem: newFeedItem(),
	}))
	var cnt int64
	require.NoError(t, db.Model(&models.RSSNotificationLog{}).Count(&cnt).Error)
	assert.EqualValues(t, 0, cnt)
}

func TestNotifyNewItem_InvalidConfIDs(t *testing.T) {
	db := setupRSSNotifierDB(t)
	n := NewRSSNotifier(db, &fakePushSvc{})
	rss := &models.RSSConfig{ID: 1, NotifyMode: "all", NotifyConfIDs: "{bad"}
	err := n.NotifyNewItem(context.Background(), RSSItemEvent{
		RSS: rss, SiteName: "s", TorrentID: "t", FeedItem: newFeedItem(),
	})
	require.Error(t, err)
}

func TestExceededHourlyQuota_TriggersThrottle(t *testing.T) {
	db := setupRSSNotifierDB(t)
	n := NewRSSNotifier(db, &fakePushSvc{})
	rss := &models.RSSConfig{ID: 1, NotifyMode: "all", NotifyConfIDs: "[1]", MaxNotificationsPerHour: 1}
	// Pre-seed one recent log so the quota is already met.
	now := time.Now()
	require.NoError(t, db.Create(&models.RSSNotificationLog{
		RSSID: 1, SiteName: "s", TorrentID: "old", NotifyKind: "all",
		NotificationConfID: 1, Result: "sent", CreatedAt: now, UpdatedAt: now,
	}).Error)

	require.NoError(t, n.NotifyNewItem(context.Background(), RSSItemEvent{
		RSS: rss, SiteName: "s", TorrentID: "t2", FeedItem: newFeedItem(),
	}))
	var throttled int64
	require.NoError(t, db.Model(&models.RSSNotificationLog{}).
		Where("result = ?", "throttled").Count(&throttled).Error)
	assert.EqualValues(t, 1, throttled)
}

func TestNotifyFilteredItem_NilGuards(t *testing.T) {
	db := setupRSSNotifierDB(t)
	n := NewRSSNotifier(db, &fakePushSvc{})
	require.Error(t, n.NotifyFilteredItem(context.Background(), RSSFilteredEvent{RSS: nil}))
	rss := &models.RSSConfig{ID: 1, NotifyMode: "filtered", NotifyConfIDs: "[1]"}
	require.Error(t, n.NotifyFilteredItem(context.Background(), RSSFilteredEvent{RSS: rss, Torrent: nil}))
}

func TestRenderAllPayload_TitleAndTimeFallbacks(t *testing.T) {
	ev := RSSItemEvent{
		SiteName: "s",
		FeedItem: &gofeed.Item{Title: "", Published: "Mon, 01 Jan 2026", Link: "http://x"},
	}
	got := renderAllPayload(ev)
	assert.Equal(t, "(无标题)", got.Title)
	assert.Contains(t, got.Text, "Mon, 01 Jan 2026")

	ev2 := RSSItemEvent{SiteName: "s", FeedItem: &gofeed.Item{Title: "T", Link: "http://y"}}
	got2 := renderAllPayload(ev2)
	assert.Contains(t, got2.Text, "未知时间")
}

func TestRenderFilteredPayload_FreeAndRuleBranches(t *testing.T) {
	end := time.Now().Add(3 * time.Hour)
	tItem := &v2.TorrentItem{
		Title: "", URL: "http://z", SizeBytes: 2 * 1024 * 1024 * 1024,
		DiscountLevel: v2.DiscountFree, DiscountEndTime: end,
	}
	rule := &models.FilterRule{ID: 1, Name: "r1"}
	got := renderFilteredPayload(RSSFilteredEvent{SiteName: "s", Torrent: tItem, Rule: rule})
	assert.Equal(t, "(无标题)", got.Title)
	assert.Contains(t, got.Text, "免费")
	assert.Contains(t, got.Text, "剩余")
	assert.Contains(t, got.Text, "匹配规则")

	tItem2 := &v2.TorrentItem{Title: "X", URL: "http://z", DiscountLevel: v2.DiscountFree}
	got2 := renderFilteredPayload(RSSFilteredEvent{SiteName: "s", Torrent: tItem2})
	assert.Contains(t, got2.Text, "免费")
}

func TestFormatBytesRSS_And_FormatRemaining(t *testing.T) {
	assert.Equal(t, "512 B", formatBytesRSS(512))
	assert.Contains(t, formatBytesRSS(1024*1024), "MB")

	assert.Equal(t, "已结束", formatRemaining(time.Now().Add(-time.Hour)))
	assert.Contains(t, formatRemaining(time.Now().Add(90*time.Minute)), "h")
	assert.Contains(t, formatRemaining(time.Now().Add(30*time.Minute)), "min")
}

func TestExceededHourlyQuota_CountError(t *testing.T) {
	db := setupClosedForRSS(t)
	n := NewRSSNotifier(db, &fakePushSvc{}).(*rssNotifier)
	rss := &models.RSSConfig{ID: 1, MaxNotificationsPerHour: 5}
	_, err := n.exceededHourlyQuota(context.Background(), rss)
	require.Error(t, err)
}

func setupClosedForRSS(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.RSSNotificationLog{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	return db
}

func TestNotifyFilteredItem_QuotaThrottles(t *testing.T) {
	db := setupRSSNotifierDB(t)
	n := NewRSSNotifier(db, &fakePushSvc{})
	rss := &models.RSSConfig{ID: 1, NotifyMode: "filtered", NotifyConfIDs: "[1]", MaxNotificationsPerHour: 1}
	require.NoError(t, db.Create(&models.RSSNotificationLog{
		RSSID: 1, SiteName: "s", TorrentID: "old", NotifyKind: "filtered",
		NotificationConfID: 1, Result: "sent",
	}).Error)
	tItem := &v2.TorrentItem{ID: "t2", Title: "X", URL: "http://z"}
	require.NoError(t, n.NotifyFilteredItem(context.Background(), RSSFilteredEvent{
		RSS: rss, Torrent: tItem, SiteName: "s", TorrentID: "t2",
	}))
	var throttled int64
	require.NoError(t, db.Model(&models.RSSNotificationLog{}).
		Where("result = ?", "throttled").Count(&throttled).Error)
	assert.EqualValues(t, 1, throttled)
}

func TestNotifyFilteredItem_InvalidConfIDs(t *testing.T) {
	db := setupRSSNotifierDB(t)
	n := NewRSSNotifier(db, &fakePushSvc{})
	rss := &models.RSSConfig{ID: 1, NotifyMode: "filtered", NotifyConfIDs: "{bad"}
	tItem := &v2.TorrentItem{ID: "t", Title: "X"}
	require.Error(t, n.NotifyFilteredItem(context.Background(), RSSFilteredEvent{
		RSS: rss, Torrent: tItem, SiteName: "s", TorrentID: "t",
	}))
}

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
