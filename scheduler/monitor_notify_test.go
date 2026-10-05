package scheduler

import (
	"context"
	"errors"
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
