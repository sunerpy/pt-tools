package app

import (
	"context"
	"errors"
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

// logRecorder 收集注入的日志函数输出，供断言写回失败是否被记录。
type logRecorder struct {
	mu   sync.Mutex
	logs []string
}

func (l *logRecorder) logf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs = append(l.logs, fmt.Sprintf(format, args...))
}

func (l *logRecorder) joined() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.logs, "\n")
}

func seedPendingRSSLog(t *testing.T, db *gorm.DB, torrentID, payload string) models.RSSNotificationLog {
	t.Helper()
	past := time.Now().Add(-time.Minute)
	row := models.RSSNotificationLog{
		RSSID: 1, SiteName: "s", TorrentID: torrentID, NotifyKind: "all", NotificationConfID: 7,
		Result: "pending", PayloadJSON: payload, NextRetryAt: &past,
	}
	require.NoError(t, db.Create(&row).Error)
	return row
}

// 交给摘要前推迟重试时间失败时不放进摘要：这行保持到期的 pending，由重试 worker 逐条补发一次。
func TestRSSNotifier_DigestHoldFailureFallsBackToRetry(t *testing.T) {
	db := setupRSSNotifierDB(t)
	push := &capturePushService{}
	notifier, buf := newDigestNotifier(t, db, push)
	require.NoError(t, db.Exec(`CREATE TRIGGER fail_hold BEFORE UPDATE OF next_retry_at ON rss_notification_log
		BEGIN SELECT RAISE(ABORT, 'disk I/O error'); END`).Error)

	rss := &models.RSSConfig{ID: 1, NotifyMode: "all", NotifyConfIDs: "[7]"}
	require.NoError(t, notifier.NotifyNewItem(context.Background(), RSSItemEvent{
		RSS: rss, FeedItem: newFeedItem(), SiteName: "example", TorrentID: "1",
	}))
	buf.FlushAll()
	assert.Zero(t, push.callCount(), "没有放进摘要")

	require.NoError(t, db.Exec(`DROP TRIGGER fail_hold`).Error)
	require.NoError(t, NewRSSRetryWorker(db, push).drainOnce(context.Background()))
	assert.Equal(t, 1, push.callCount(), "重试 worker 补发一次")
}

// 查不到待发送记录（查询失败或都已不是 pending）时不发送。
func TestRSSDigestFlush_SkipsWhenNothingPending(t *testing.T) {
	t.Run("query failure", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		push := &capturePushService{}
		rec := &logRecorder{}
		flush := NewRSSDigestFlush(db, push, rec.logf)

		flush(context.Background(), 7, []notify.DigestItem{{LogID: 1, Title: "T", Text: "B"}})

		assert.Zero(t, push.callCount())
		assert.Contains(t, rec.joined(), "查询待发送记录失败")
	})
	t.Run("no pending rows", func(t *testing.T) {
		db := setupRSSNotifierDB(t)
		push := &capturePushService{}
		rec := &logRecorder{}
		flush := NewRSSDigestFlush(db, push, rec.logf)

		flush(context.Background(), 7, []notify.DigestItem{{LogID: 99, Title: "T", Text: "B"}})

		assert.Zero(t, push.callCount())
		assert.Empty(t, rec.joined())
	})
}

// 摘要投递失败时这些行 5 秒后交回重试 worker：仍是 pending，attempts 加一，记下错误。
func TestRSSDigestFlush_PushFailureHandsRowsBackToRetry(t *testing.T) {
	db := setupRSSNotifierDB(t)
	push := &capturePushService{err: errors.New("通道未运行")}
	rec := &logRecorder{}
	a := seedPendingRSSLog(t, db, "1", `{"title":"A","text":"a"}`)
	b := seedPendingRSSLog(t, db, "2", `{"title":"B","text":"b"}`)
	flush := NewRSSDigestFlush(db, push, rec.logf)

	before := time.Now()
	flush(context.Background(), 7, []notify.DigestItem{
		{LogID: a.ID, Title: "A", Text: "a"},
		{LogID: b.ID, Title: "B", Text: "b"},
	})

	require.Equal(t, 1, push.callCount(), "两条合并成一条摘要")
	assert.Empty(t, push.calls[0].Buttons, "多条合并的摘要不带单条按钮")
	var rows []models.RSSNotificationLog
	require.NoError(t, db.Order("id").Find(&rows).Error)
	require.Len(t, rows, 2)
	for _, row := range rows {
		assert.Equal(t, "pending", row.Result)
		assert.Equal(t, 1, row.Attempts)
		assert.Contains(t, row.LastError, "通道未运行")
		require.NotNil(t, row.NextRetryAt)
		assert.False(t, row.NextRetryAt.Before(before.Add(5*time.Second)), "5 秒后才交回重试")
	}
	assert.Contains(t, rec.joined(), "RSS digest 投递失败")
}

// 摘要发送后写回记录失败时记日志，不静默吞掉。
func TestRSSDigestFlush_LogsWriteBackFailures(t *testing.T) {
	cases := []struct {
		name, trigger, want string
		pushErr             error
	}{
		{
			name: "sent write-back",
			trigger: `CREATE TRIGGER fail_write BEFORE UPDATE OF result ON rss_notification_log
				WHEN NEW.result = 'sent' BEGIN SELECT RAISE(ABORT, 'disk I/O error'); END`,
			want: "已投递但更新记录失败",
		},
		{
			name: "retry write-back",
			trigger: `CREATE TRIGGER fail_write BEFORE UPDATE OF last_error ON rss_notification_log
				BEGIN SELECT RAISE(ABORT, 'disk I/O error'); END`,
			want:    "投递失败且更新记录失败",
			pushErr: errors.New("通道未运行"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupRSSNotifierDB(t)
			push := &capturePushService{err: tc.pushErr}
			rec := &logRecorder{}
			row := seedPendingRSSLog(t, db, "1", `{"title":"A","text":"a"}`)
			require.NoError(t, db.Exec(tc.trigger).Error)

			NewRSSDigestFlush(db, push, rec.logf)(context.Background(), 7, []notify.DigestItem{{LogID: row.ID, Title: "A", Text: "a"}})

			require.Equal(t, 1, push.callCount())
			assert.Contains(t, rec.joined(), tc.want)
		})
	}
}

// 重试 worker 的各处写回失败都记日志：占用失败时不发送，避免在没占住的情况下重复投递。
func TestRSSRetryWorker_LogsDBWriteFailures(t *testing.T) {
	cases := []struct {
		name, payload, trigger, want string
		pushErr                      error
		wantCalls                    int
	}{
		{
			name:    "claim",
			payload: `{"title":"T","text":"B"}`,
			trigger: `CREATE TRIGGER fail_write BEFORE UPDATE OF next_retry_at ON rss_notification_log
				BEGIN SELECT RAISE(ABORT, 'disk I/O error'); END`,
			want: "占用记录失败",
		},
		{
			name:    "retry result",
			payload: `{"title":"T","text":"B"}`,
			trigger: `CREATE TRIGGER fail_write BEFORE UPDATE OF last_error ON rss_notification_log
				BEGIN SELECT RAISE(ABORT, 'disk I/O error'); END`,
			want:      "重试结果写回失败",
			pushErr:   errors.New("通道未运行"),
			wantCalls: 1,
		},
		{
			name:    "mark failed",
			payload: "{not valid json",
			trigger: `CREATE TRIGGER fail_write BEFORE UPDATE OF result ON rss_notification_log
				WHEN NEW.result = 'failed' BEGIN SELECT RAISE(ABORT, 'disk I/O error'); END`,
			want: "标记失败时写回出错",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupRSSNotifierDB(t)
			push := &capturePushService{err: tc.pushErr}
			rec := &logRecorder{}
			seedPendingRSSLog(t, db, "1", tc.payload)
			require.NoError(t, db.Exec(tc.trigger).Error)
			worker := NewRSSRetryWorker(db, push)
			worker.SetLogf(rec.logf)

			require.NoError(t, worker.drainOnce(context.Background()))

			assert.Equal(t, tc.wantCalls, push.callCount())
			assert.Contains(t, rec.joined(), tc.want)
		})
	}
}

// 周期扫描查询失败时记日志，循环继续运行，ctx 取消后退出。
func TestRSSRetryWorker_RunLogsScanFailure(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	worker := NewRSSRetryWorker(db, &capturePushService{})
	worker.interval = 5 * time.Millisecond
	logged := make(chan string, 1)
	worker.SetLogf(func(format string, args ...any) {
		select {
		case logged <- fmt.Sprintf(format, args...):
		default:
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Run(ctx)
	}()

	select {
	case msg := <-logged:
		assert.Contains(t, msg, "重试扫描失败")
	case <-time.After(5 * time.Second):
		t.Fatal("扫描失败没有记录日志")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ctx 取消后 Run 没有退出")
	}
}
