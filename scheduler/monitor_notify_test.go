package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
)

func newNotifierForTest(t *testing.T, now time.Time, confs ...models.NotificationConf) (*MonitorNotifier, *captureSender, *sitelogin.FakeClock, *gorm.DB) {
	t.Helper()
	db := newReminderTestDB(t)
	for i := range confs {
		require.NoError(t, db.Create(&confs[i]).Error)
	}
	clock := sitelogin.NewFakeClock(now)
	sender := &captureSender{}
	return NewMonitorNotifier(db, sender, clock, zap.NewNop().Sugar()), sender, clock, db
}

func sampleEntry() MonitorNotifyEntry {
	return MonitorNotifyEntry{Source: "login", Subject: "hdsky", Kind: "tier", EventKey: "tier:7d:1", Title: "t", Text: "x"}
}

func notifyRows(t *testing.T, db *gorm.DB) []models.MonitorNotificationLog {
	t.Helper()
	var rows []models.MonitorNotificationLog
	require.NoError(t, db.Order("notification_conf_id").Find(&rows).Error)
	return rows
}

func TestMonitorNotifier_EnqueueTargetsAndDedupe(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	disabled := quietConf(3, "", "")
	n, _, _, db := newNotifierForTest(t, now, quietConf(1, "", ""), quietConf(2, "", ""), disabled)
	require.NoError(t, db.Model(&models.NotificationConf{}).Where("id = ?", 3).Update("enabled", false).Error)

	inserted, err := n.Enqueue(context.Background(), sampleEntry())
	require.NoError(t, err)
	assert.Equal(t, 2, inserted, "empty ConfIDs → every enabled channel, disabled ones skipped")

	again, err := n.Enqueue(context.Background(), sampleEntry())
	require.NoError(t, err)
	assert.Equal(t, 0, again, "the same decision written twice is not queued twice")

	only := sampleEntry()
	only.EventKey = "tier:7d:2"
	only.ConfIDs = []uint{2, 3}
	inserted, err = n.Enqueue(context.Background(), only)
	require.NoError(t, err)
	assert.Equal(t, 1, inserted, "explicit ConfIDs are intersected with enabled channels")
	assert.Len(t, notifyRows(t, db), 3)
}

func TestMonitorNotifier_QuietAtEnqueueDefersOnlyThatChannel(t *testing.T) {
	now := time.Date(2026, 5, 18, 3, 0, 0, 0, time.UTC)
	n, sender, _, db := newNotifierForTest(t, now, quietConf(1, "", ""), quietConf(2, "00:00", "07:00"))
	_, err := n.Enqueue(context.Background(), sampleEntry())
	require.NoError(t, err)

	rows := notifyRows(t, db)
	require.Len(t, rows, 2)
	assert.True(t, rows[0].NextRetryAt.Equal(now), "channel without quiet hours is due now")
	assert.True(t, rows[1].NextRetryAt.Equal(time.Date(2026, 5, 18, 7, 0, 0, 0, time.UTC)), "quiet channel waits for its window to end")

	assert.Equal(t, 1, n.DeliverDue(context.Background()))
	assert.Equal(t, int32(1), sender.count.Load())
	assert.Equal(t, uint(1), sender.lastConf)
}

func TestMonitorNotifier_QuietRecheckedAtDelivery(t *testing.T) {
	// 行在静默开始前写入（立即到期），轮到投递时通道已进入静默 → 顺延且不计尝试。
	start := time.Date(2026, 5, 18, 22, 59, 0, 0, time.UTC)
	n, sender, clock, db := newNotifierForTest(t, start, quietConf(1, "23:00", "08:00"))
	_, err := n.Enqueue(context.Background(), sampleEntry())
	require.NoError(t, err)

	clock.Advance(2 * time.Minute) // 23:01
	assert.Equal(t, 0, n.DeliverDue(context.Background()))
	assert.Equal(t, int32(0), sender.count.Load())
	row := notifyRows(t, db)[0]
	assert.Equal(t, models.MonitorNotifyPending, row.Result)
	assert.Equal(t, 0, row.Attempts, "a quiet deferral is not an attempt")
	assert.True(t, row.NextRetryAt.Equal(time.Date(2026, 5, 19, 8, 0, 0, 0, time.UTC)))

	clock.Advance(9 * time.Hour) // 08:01 next day
	assert.Equal(t, 1, n.DeliverDue(context.Background()))
	assert.Equal(t, models.MonitorNotifySent, notifyRows(t, db)[0].Result)
}

func TestMonitorNotifier_RetryBackoffThenFailed(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	n, sender, clock, db := newNotifierForTest(t, now, quietConf(1, "", ""))
	sender.err = errors.New("napcat not connected")
	_, err := n.Enqueue(context.Background(), sampleEntry())
	require.NoError(t, err)

	for i, wait := range []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute} {
		assert.Equal(t, 0, n.DeliverDue(context.Background()))
		row := notifyRows(t, db)[0]
		assert.Equal(t, i+1, row.Attempts)
		assert.Equal(t, models.MonitorNotifyPending, row.Result)
		assert.Equal(t, wait, row.NextRetryAt.Sub(clock.Now()), "attempt %d", i+1)
		assert.Contains(t, row.LastError, "napcat")
		clock.Advance(wait)
	}
	n.DeliverDue(context.Background())
	row := notifyRows(t, db)[0]
	assert.Equal(t, 4, row.Attempts)
	assert.Equal(t, models.MonitorNotifyFailed, row.Result, "the 4th failure gives up")
}

func TestMonitorNotifier_RetryLandingInQuietIsDeferred(t *testing.T) {
	now := time.Date(2026, 5, 18, 6, 59, 30, 0, time.UTC)
	n, sender, clock, db := newNotifierForTest(t, now, quietConf(1, "07:00", "08:00"))
	sender.err = errors.New("boom")
	_, err := n.Enqueue(context.Background(), sampleEntry())
	require.NoError(t, err)
	n.DeliverDue(context.Background()) // fails at 06:59:30, retry at 07:00:30
	clock.Advance(time.Minute)
	sender.err = nil
	assert.Equal(t, 0, n.DeliverDue(context.Background()), "the retry falls in the quiet window and waits")
	row := notifyRows(t, db)[0]
	assert.Equal(t, 1, row.Attempts)
	assert.True(t, row.NextRetryAt.Equal(time.Date(2026, 5, 18, 8, 0, 0, 0, time.UTC)))
}

func TestMonitorNotifier_BypassQuietSendsImmediately(t *testing.T) {
	now := time.Date(2026, 5, 18, 3, 0, 0, 0, time.UTC)
	n, sender, _, _ := newNotifierForTest(t, now, quietConf(1, "00:00", "07:00"))
	e := sampleEntry()
	e.BypassQuiet = true
	_, err := n.Enqueue(context.Background(), e)
	require.NoError(t, err)
	assert.Equal(t, 1, n.DeliverDue(context.Background()))
	assert.Equal(t, int32(1), sender.count.Load())
}

func TestMonitorNotifier_DisabledChannelAtDeliveryFails(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	n, sender, _, db := newNotifierForTest(t, now, quietConf(1, "", ""))
	_, err := n.Enqueue(context.Background(), sampleEntry())
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.NotificationConf{}).Where("id = ?", 1).Update("enabled", false).Error)
	n.DeliverDue(context.Background())
	assert.Equal(t, int32(0), sender.count.Load())
	assert.Equal(t, models.MonitorNotifyFailed, notifyRows(t, db)[0].Result)
}

func TestMonitorNotifier_DeliverNow(t *testing.T) {
	now := time.Date(2026, 5, 18, 3, 0, 0, 0, time.UTC)
	n, sender, _, _ := newNotifierForTest(t, now, quietConf(1, "", ""), quietConf(2, "", ""))
	e := sampleEntry()
	e.BypassQuiet = true
	_, err := n.Enqueue(context.Background(), e)
	require.NoError(t, err)
	require.NoError(t, n.DeliverNow(context.Background(), e))
	assert.Equal(t, int32(2), sender.count.Load())
	assert.Error(t, n.DeliverNow(context.Background(), e), "nothing pending any more")

	sender.err = errors.New("down")
	e2 := e
	e2.EventKey = "test:2"
	_, err = n.Enqueue(context.Background(), e2)
	require.NoError(t, err)
	assert.ErrorContains(t, n.DeliverNow(context.Background(), e2), "down")
}

func TestMonitorNotifier_NilSenderIsInert(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.NotificationConf{ID: 1, ChannelType: "x", Name: "x", Enabled: true}).Error)
	n := NewMonitorNotifier(db, nil, nil, nil)
	assert.False(t, n.Enabled())
	inserted, err := n.Enqueue(context.Background(), sampleEntry())
	require.NoError(t, err)
	assert.Equal(t, 0, inserted)
	assert.Equal(t, 0, n.DeliverDue(context.Background()))
	assert.Error(t, n.DeliverNow(context.Background(), sampleEntry()))
	n.Start()
	n.Stop()

	var nilNotifier *MonitorNotifier
	assert.False(t, nilNotifier.Enabled())
	nilNotifier.Start()
	nilNotifier.Stop()
}

func TestMonitorNotifier_StartStopIdempotent(t *testing.T) {
	n, _, _, _ := newNotifierForTest(t, time.Now(), quietConf(1, "", ""))
	n.tick = 10 * time.Millisecond
	n.Start()
	n.Start()
	time.Sleep(30 * time.Millisecond)
	n.Stop()
	n.Stop()
}

// slowSender 模拟耗时的 PushSync：每发一条，时钟前进 step。
type slowSender struct {
	clock *sitelogin.FakeClock
	step  time.Duration
	sent  []uint
}

func (s *slowSender) Send(_ context.Context, confID uint, _ MonitorMessage) error {
	s.sent = append(s.sent, confID)
	s.clock.Advance(s.step)
	return nil
}

// 同一批里前几条发送耗时，让后面的行轮到时已进入静默时段：按此刻的时间判断，顺延到静默结束且不计尝试。
func TestMonitorNotifier_QuietRecheckedPerRowWithinBatch(t *testing.T) {
	start := time.Date(2026, 5, 18, 22, 58, 0, 0, time.UTC)
	db := newReminderTestDB(t)
	conf := quietConf(1, "23:00", "08:00")
	require.NoError(t, db.Create(&conf).Error)
	clock := sitelogin.NewFakeClock(start)
	sender := &slowSender{clock: clock, step: 3 * time.Minute}
	n := NewMonitorNotifier(db, sender, clock, zap.NewNop().Sugar())

	first := sampleEntry()
	second := sampleEntry()
	second.EventKey = "tier:7d:2"
	for _, e := range []MonitorNotifyEntry{first, second} {
		_, err := n.Enqueue(context.Background(), e)
		require.NoError(t, err)
	}

	assert.Equal(t, 1, n.DeliverDue(context.Background()), "the second row comes up at 23:01, inside the quiet window")
	assert.Equal(t, []uint{1}, sender.sent)
	var deferred models.MonitorNotificationLog
	require.NoError(t, db.Where("event_key = ?", second.EventKey).First(&deferred).Error)
	assert.Equal(t, models.MonitorNotifyPending, deferred.Result)
	assert.Equal(t, 0, deferred.Attempts)
	assert.True(t, deferred.NextRetryAt.Equal(time.Date(2026, 5, 19, 8, 0, 0, 0, time.UTC)), "got %v", deferred.NextRetryAt)
}

// captureAuditor 记下交给审计的每一次投递。
type captureAuditor struct {
	mu   sync.Mutex
	list []MonitorDelivery
}

func (a *captureAuditor) RecordDelivery(_ context.Context, d MonitorDelivery) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.list = append(a.list, d)
}

func (a *captureAuditor) all() []MonitorDelivery {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]MonitorDelivery(nil), a.list...)
}

// 2026-10-28 用户：签到、追剧、每日战报这些通知不在操作审计里，每日战报发送失败了也不知道为什么。
// 每次投递都交给审计：送达、失败（原因、第几次、下一次重试）与放弃；静默顺延不是投递，不报。
func TestMonitorNotifier_AuditsEveryDelivery(t *testing.T) {
	now := time.Date(2026, 10, 28, 22, 0, 0, 0, time.UTC)
	qq := quietConf(1, "", "")
	qq.ChannelType = "qq_onebot"
	n, sender, clock, _ := newNotifierForTest(t, now, qq)
	aud := &captureAuditor{}
	n.SetAuditor(aud)
	sender.err = errors.New("QQ 适配器: 缺少目标 chat_id")
	_, err := n.Enqueue(context.Background(), MonitorNotifyEntry{
		Source: "daily_report", Subject: "*", Kind: "daily", EventKey: "2026-10-28", Title: "每日战报 2026-10-28", Text: "x",
	})
	require.NoError(t, err)

	assert.Equal(t, 0, n.DeliverDue(context.Background()))
	got := aud.all()
	require.Len(t, got, 1)
	first := got[0]
	assert.Equal(t, "daily_report", first.Source)
	assert.Equal(t, "daily", first.Kind)
	assert.Equal(t, uint(1), first.ConfID)
	assert.Equal(t, "qq_onebot", first.ChannelType)
	assert.Equal(t, "每日战报 2026-10-28", first.Title)
	assert.Equal(t, 1, first.Attempt)
	require.Error(t, first.Err)
	assert.Contains(t, first.Err.Error(), "缺少目标 chat_id")
	assert.False(t, first.Final)
	assert.True(t, first.NextRetryAt.Equal(now.Add(time.Minute)), "第一次失败 1 分钟后重试")

	clock.Advance(time.Minute)
	sender.err = nil
	assert.Equal(t, 1, n.DeliverDue(context.Background()))
	got = aud.all()
	require.Len(t, got, 2)
	assert.NoError(t, got[1].Err)
	assert.Equal(t, 2, got[1].Attempt)
	assert.False(t, got[1].Final)
}

func TestMonitorNotifier_AuditsGiveUpAndBrokenRows(t *testing.T) {
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	n, sender, clock, db := newNotifierForTest(t, now, quietConf(1, "", ""), quietConf(2, "", ""))
	aud := &captureAuditor{}
	n.SetAuditor(aud)
	sender.err = errors.New("boom")
	e := sampleEntry()
	e.ConfIDs = []uint{1}
	_, err := n.Enqueue(context.Background(), e)
	require.NoError(t, err)
	for _, wait := range []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 0} {
		n.DeliverDue(context.Background())
		clock.Advance(wait)
	}
	got := aud.all()
	require.Len(t, got, 4)
	assert.True(t, got[3].Final, "第 4 次失败后放弃")
	assert.True(t, got[3].NextRetryAt.IsZero())
	assert.Equal(t, 4, got[3].Attempt)

	// 通道在投递前被停用：放弃并说明原因
	e2 := sampleEntry()
	e2.EventKey, e2.ConfIDs = "tier:7d:9", []uint{2}
	_, err = n.Enqueue(context.Background(), e2)
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.NotificationConf{}).Where("id = ?", 2).Update("enabled", false).Error)
	n.DeliverDue(context.Background())
	got = aud.all()
	require.Len(t, got, 5)
	assert.True(t, got[4].Final)
	assert.EqualError(t, got[4].Err, "通知通道不存在或已停用")
	assert.Zero(t, got[4].Latency)
}

func TestMonitorNotifier_QuietDeferralIsNotAudited(t *testing.T) {
	start := time.Date(2026, 5, 18, 22, 59, 0, 0, time.UTC)
	n, _, clock, _ := newNotifierForTest(t, start, quietConf(1, "23:00", "08:00"))
	aud := &captureAuditor{}
	n.SetAuditor(aud)
	_, err := n.Enqueue(context.Background(), sampleEntry())
	require.NoError(t, err)
	clock.Advance(2 * time.Minute)
	assert.Equal(t, 0, n.DeliverDue(context.Background()))
	assert.Empty(t, aud.all())
}
