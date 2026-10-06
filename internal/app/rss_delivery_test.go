package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
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

// newDigestNotifier 返回接了摘要缓冲（窗口足够长、阈值足够高，测试里手动 FlushAll）的 notifier。
func newDigestNotifier(t *testing.T, db *gorm.DB, push *capturePushService) (*rssNotifier, *notify.DigestBuffer) {
	t.Helper()
	n := NewRSSNotifier(db, push).(*rssNotifier)
	buf := notify.NewDigestBufferWithWindow(context.Background(), NewRSSDigestFlush(db, push, nil), time.Hour, 100)
	n.SetDigestBuffer(buf)
	return n, buf
}

// 交给摘要的行在合并窗口内不会被重试 worker 逐条发出，摘要刷写时只发一次。
func TestRSSNotifier_DigestRowsAreNotRetriedDuringWindow(t *testing.T) {
	db := setupRSSNotifierDB(t)
	push := &capturePushService{}
	notifier, buf := newDigestNotifier(t, db, push)

	rss := &models.RSSConfig{ID: 1, NotifyMode: "all", NotifyConfIDs: "[7]"}
	require.NoError(t, notifier.NotifyNewItem(context.Background(), RSSItemEvent{
		RSS: rss, FeedItem: newFeedItem(), SiteName: "example", TorrentID: "1",
	}))

	worker := NewRSSRetryWorker(db, push)
	require.NoError(t, worker.drainOnce(context.Background()))
	assert.Zero(t, push.callCount(), "窗口内重试 worker 不发送摘要里的行")

	buf.FlushAll()
	require.Equal(t, 1, push.callCount(), "摘要刷写只发一次")
	var row models.RSSNotificationLog
	require.NoError(t, db.First(&row).Error)
	assert.Equal(t, "sent", row.Result)

	require.NoError(t, worker.drainOnce(context.Background()))
	assert.Equal(t, 1, push.callCount(), "已发出的行不会再被重试")
}

// both 模式下命中过滤规则后，摘要里那条 all 通知被抑制，刷写时不再发出。
func TestRSSDigestFlush_SkipsSuppressedRows(t *testing.T) {
	db := setupRSSNotifierDB(t)
	push := &capturePushService{}
	notifier, buf := newDigestNotifier(t, db, push)

	rss := &models.RSSConfig{ID: 1, NotifyMode: "both", NotifyConfIDs: "[7]"}
	require.NoError(t, notifier.NotifyNewItem(context.Background(), RSSItemEvent{
		RSS: rss, FeedItem: newFeedItem(), SiteName: "example", TorrentID: "1",
	}))
	require.NoError(t, notifier.NotifyFilteredItem(context.Background(), RSSFilteredEvent{
		RSS: rss, Torrent: &v2.TorrentItem{ID: "1", Title: "Test.Movie.2026.1080p", URL: "https://example.com/details.php?id=1"},
		Rule:     &models.FilterRule{ID: 3, Name: "r"},
		SiteName: "example", TorrentID: "1",
	}))

	buf.FlushAll()
	require.Equal(t, 1, push.callCount())
	sent := push.calls[0]
	assert.Contains(t, sent.Text, "🎯", "发出的是 filtered 通知")
	assert.NotContains(t, sent.Text, "🆕", "被抑制的 all 通知不出现在发送内容里")

	var results []string
	require.NoError(t, db.Model(&models.RSSNotificationLog{}).Order("id").Pluck("result", &results).Error)
	assert.Equal(t, []string{"suppressed", "sent"}, results)
}

// 摘要里只剩一条时按单条通知发送，保留「立即下载 / 忽略」按钮。
func TestRSSDigestFlush_SingleItemKeepsButtons(t *testing.T) {
	db := setupRSSNotifierDB(t)
	push := &capturePushService{}
	notifier, buf := newDigestNotifier(t, db, push)

	rss := &models.RSSConfig{ID: 1, NotifyMode: "all", NotifyConfIDs: "[7]"}
	require.NoError(t, notifier.NotifyNewItem(context.Background(), RSSItemEvent{
		RSS: rss, FeedItem: newFeedItem(), SiteName: "example", TorrentID: "1",
	}))
	buf.FlushAll()

	require.Equal(t, 1, push.callCount())
	require.Len(t, push.calls[0].Buttons, 1)
	var row models.RSSNotificationLog
	require.NoError(t, db.First(&row).Error)
	assert.Equal(t, fmt.Sprintf("dl:%d", row.ID), push.calls[0].Buttons[0][0].CallbackData)
}

// 重试 worker 拿着过期的快照再处理一次已经发出的行，不会重复发送。
func TestRSSRetryWorker_StaleSnapshotDoesNotResend(t *testing.T) {
	db := setupRSSNotifierDB(t)
	push := &capturePushService{}
	past := time.Now().Add(-time.Minute)
	row := models.RSSNotificationLog{
		RSSID: 1, SiteName: "s", TorrentID: "1", NotifyKind: "all", NotificationConfID: 7,
		Result: "pending", PayloadJSON: `{"title":"T","text":"B"}`, NextRetryAt: &past,
	}
	require.NoError(t, db.Create(&row).Error)
	snapshot := row

	worker := NewRSSRetryWorker(db, push)
	worker.attemptOne(context.Background(), &row)
	worker.attemptOne(context.Background(), &snapshot)

	assert.Equal(t, 1, push.callCount())
	assert.Len(t, push.calls[0].Buttons, 1, "重试发出的单条通知同样带按钮")
}

// 发送成功后写回 sent 失败时记录错误；行被租期占住，不会在下一轮马上重发。
func TestRSSRetryWorker_LogsWriteBackFailureAndHoldsLease(t *testing.T) {
	db := setupRSSNotifierDB(t)
	push := &capturePushService{}
	past := time.Now().Add(-time.Minute)
	row := models.RSSNotificationLog{
		RSSID: 1, SiteName: "s", TorrentID: "1", NotifyKind: "all", NotificationConfID: 7,
		Result: "pending", PayloadJSON: `{"title":"T","text":"B"}`, NextRetryAt: &past,
	}
	require.NoError(t, db.Create(&row).Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER fail_sent BEFORE UPDATE OF result ON rss_notification_log
		WHEN NEW.result = 'sent' BEGIN SELECT RAISE(ABORT, 'disk I/O error'); END`).Error)

	var logs []string
	var mu sync.Mutex
	worker := NewRSSRetryWorker(db, push)
	worker.SetLogf(func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		logs = append(logs, fmt.Sprintf(format, args...))
	})

	require.NoError(t, worker.drainOnce(context.Background()))
	require.NoError(t, worker.drainOnce(context.Background()))
	assert.Equal(t, 1, push.callCount(), "租期内不重发")
	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, logs)
	assert.True(t, strings.Contains(logs[0], "写回 sent 失败"), logs[0])
}

// 每小时配额：多个 RSS worker 并发处理不同种子时，合计不超过上限。
func TestRSSNotifier_HourlyQuotaHoldsUnderConcurrency(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=busy_timeout(5000)", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&models.RSSNotificationLog{}, &models.NotificationConf{}))

	push := &capturePushService{}
	notifier, buf := newDigestNotifier(t, db, push)
	rss := &models.RSSConfig{ID: 1, NotifyMode: "all", NotifyConfIDs: "[7]", MaxNotificationsPerHour: 2}

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			_ = notifier.NotifyNewItem(context.Background(), RSSItemEvent{
				RSS: rss, FeedItem: newFeedItem(), SiteName: "example", TorrentID: fmt.Sprintf("t%d", i),
			})
		})
	}
	wg.Wait()
	buf.FlushAll()

	var delivered int64
	require.NoError(t, db.Model(&models.RSSNotificationLog{}).Where("result IN ?", []string{"pending", "sent"}).Count(&delivered).Error)
	assert.EqualValues(t, 2, delivered, "并发时也不突破每小时上限")
}
