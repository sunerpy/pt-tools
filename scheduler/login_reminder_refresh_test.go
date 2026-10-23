package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 发封号提醒前先补探测一次（2026-10-24 用户：「网页已经打开过了还是提示要封禁」）：
// 存下的状态是最多 6 小时前的探测结果，用户在这期间用浏览器登录过，站点的最近访问已经前进。

type countingReminderSite struct {
	fakeReminderSite
	calls int
}

func (s *countingReminderSite) GetUserInfo(ctx context.Context) (v2.UserInfo, error) {
	s.calls++
	return s.fakeReminderSite.GetUserInfo(ctx)
}

func newRefreshMonitor(t *testing.T, now time.Time, site *countingReminderSite) (*LoginReminderMonitor, *captureSender) {
	t.Helper()
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "rousipro", Enabled: true, AuthMethod: "passkey", Passkey: "k"}).Error)
	m := NewLoginReminderMonitor(LoginReminderConfig{
		DB:        db,
		Resolver:  &fakeReminderResolver{def: &v2.SiteDefinition{ID: "rousipro", Schema: v2.SchemaRousi}, site: site},
		Decryptor: fakeDecryptor{},
		Clock:     sitelogin.NewFakeClock(now),
		Logger:    zap.NewNop().Sugar(),
		DefinitionLookup: func(name string) (*v2.SiteDefinition, bool) {
			return &v2.SiteDefinition{ID: name, Schema: v2.SchemaRousi}, true
		},
	})
	return m, attachCaptureNotifier(t, m, db)
}

// 存下的状态是 42 天没活跃、上次探测在 5 小时前：到了提醒时刻先探测，站点说今天访问过，就不发提醒。
func TestReminderRefreshesBeforeSending(t *testing.T) {
	now := time.Date(2026, 10, 23, 2, 0, 0, 0, time.UTC) // 北京时间 10:00，提醒 cron 的时刻
	visited := now.Add(-3 * time.Hour)
	site := &countingReminderSite{fakeReminderSite: fakeReminderSite{info: v2.UserInfo{Username: "u", LastAccess: visited.Unix()}}}
	m, sender := newRefreshMonitor(t, now, site)

	stale := time.Date(2026, 8, 11, 15, 59, 59, 0, time.UTC)
	probedAt := now.Add(-5 * time.Hour)
	require.NoError(t, m.db.Create(&models.SiteLoginState{
		SiteName: "rousipro", BanThresholdDays: 30, RemindBeforeDays: 10, ReminderCron: "0 2,14 * * *",
		LastReminderTier: tierImminent, LastAccessAt: &stale, LastProbeAt: &probedAt, LastProbeStatus: "OK",
	}).Error)

	m.RunReminderOnce(context.Background())
	m.notifier.DeliverDue(context.Background())

	assert.Equal(t, 1, site.calls, "提醒前探测了一次")
	assert.Zero(t, sender.count.Load(), "探测后已经不危险：不发提醒")
	var st models.SiteLoginState
	require.NoError(t, m.db.Where("site_name = ?", "rousipro").First(&st).Error)
	require.NotNil(t, st.LastAccessAt)
	assert.True(t, visited.Equal(*st.LastAccessAt), "最近访问按这次探测更新")
	assert.Nil(t, st.LastReminderSentAt, "没有记成发过提醒")
}

// 探测后仍然危险（站点的最近访问没动）：照常提醒，正文按新的探测结果写。
func TestReminderStillSentWhenProbeAgrees(t *testing.T) {
	now := time.Date(2026, 10, 23, 2, 0, 0, 0, time.UTC)
	stale := time.Date(2026, 8, 11, 15, 59, 59, 0, time.UTC)
	site := &countingReminderSite{fakeReminderSite: fakeReminderSite{info: v2.UserInfo{Username: "u", LastAccess: stale.Unix()}}}
	m, sender := newRefreshMonitor(t, now, site)
	probedAt := now.Add(-5 * time.Hour)
	require.NoError(t, m.db.Create(&models.SiteLoginState{
		SiteName: "rousipro", BanThresholdDays: 30, RemindBeforeDays: 10, ReminderCron: "0 2,14 * * *",
		LastReminderTier: tierImminent, LastAccessAt: &stale, LastProbeAt: &probedAt, LastProbeStatus: "OK",
	}).Error)

	m.RunReminderOnce(context.Background())
	m.notifier.DeliverDue(context.Background())

	assert.Equal(t, 1, site.calls)
	assert.Equal(t, int32(1), sender.count.Load(), "还是危险：照常提醒")
	var st models.SiteLoginState
	require.NoError(t, m.db.Where("site_name = ?", "rousipro").First(&st).Error)
	require.NotNil(t, st.LastProbeAt)
	assert.True(t, now.Equal(*st.LastProbeAt), "这次提醒前探测过")
}

// 不补探测的情况：刚探测过（30 分钟内）、不是自动探测的站点、单站锁被占用（正在探测或签到）。
func TestReminderRefreshSkipped(t *testing.T) {
	now := time.Date(2026, 10, 23, 2, 0, 0, 0, time.UTC)
	stale := time.Date(2026, 8, 11, 15, 59, 59, 0, time.UTC)
	cases := []struct {
		name     string
		probedAt time.Time
		mode     string
		lock     bool
	}{
		{name: "刚探测过", probedAt: now.Add(-10 * time.Minute), mode: ProbeModeAuto},
		{name: "手动探测的站点", probedAt: now.Add(-5 * time.Hour), mode: ProbeModeManual},
		{name: "不探测的站点", probedAt: now.Add(-5 * time.Hour), mode: ProbeModeDisabled},
		{name: "单站锁被占用", probedAt: now.Add(-5 * time.Hour), mode: ProbeModeAuto, lock: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			site := &countingReminderSite{fakeReminderSite: fakeReminderSite{info: v2.UserInfo{Username: "u", LastAccess: now.Unix()}}}
			m, sender := newRefreshMonitor(t, now, site)
			probedAt := c.probedAt
			require.NoError(t, m.db.Create(&models.SiteLoginState{
				SiteName: "rousipro", BanThresholdDays: 30, RemindBeforeDays: 10, ReminderCron: "0 2,14 * * *",
				LastReminderTier: tierImminent, LastAccessAt: &stale, LastProbeAt: &probedAt, LastProbeStatus: "OK", ProbeMode: c.mode,
			}).Error)
			if c.lock {
				release, ok := m.TryAcquireProbeLock("rousipro")
				require.True(t, ok)
				defer release()
			}

			m.RunReminderOnce(context.Background())
			m.notifier.DeliverDue(context.Background())

			assert.Zero(t, site.calls, "不补探测")
			assert.Equal(t, int32(1), sender.count.Load(), "按存下的状态照常提醒")
		})
	}
}

// 不到提醒时刻、也没有升档时不探测：补探测只发生在真要发提醒的那一刻，不会每分钟都请求站点。
func TestReminderRefreshOnlyWhenDue(t *testing.T) {
	now := time.Date(2026, 10, 23, 3, 17, 0, 0, time.UTC)
	stale := time.Date(2026, 8, 11, 15, 59, 59, 0, time.UTC)
	site := &countingReminderSite{fakeReminderSite: fakeReminderSite{info: v2.UserInfo{Username: "u", LastAccess: now.Unix()}}}
	m, sender := newRefreshMonitor(t, now, site)
	probedAt := now.Add(-5 * time.Hour)
	require.NoError(t, m.db.Create(&models.SiteLoginState{
		SiteName: "rousipro", BanThresholdDays: 30, RemindBeforeDays: 10, ReminderCron: "0 2,14 * * *",
		LastReminderTier: tierImminent, LastAccessAt: &stale, LastProbeAt: &probedAt, LastProbeStatus: "OK",
	}).Error)

	m.RunReminderOnce(context.Background())
	m.notifier.DeliverDue(context.Background())

	assert.Zero(t, site.calls)
	assert.Zero(t, sender.count.Load())
}
