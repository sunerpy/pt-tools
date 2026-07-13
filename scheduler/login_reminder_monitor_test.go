// MIT License
// Copyright (c) 2025 pt-tools

package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestTryAcquireProbeLock_SingleFlight(t *testing.T) {
	db := newReminderTestDB(t)
	m := newReminderMonitorForTest(db, time.Now())

	release, ok := m.TryAcquireProbeLock("HDSKY")
	require.True(t, ok)
	_, ok2 := m.TryAcquireProbeLock("HDSKY")
	assert.False(t, ok2, "second acquire on held slot must fail")
	release()
	release3, ok3 := m.TryAcquireProbeLock("HDSKY")
	require.True(t, ok3, "reacquire after release must succeed")
	release3()
}

func TestGetProbeSlot_ReusesSameSlot(t *testing.T) {
	db := newReminderTestDB(t)
	m := newReminderMonitorForTest(db, time.Now())
	m.probeLocks = nil // force lazy init
	s1 := m.getProbeSlot("A")
	s2 := m.getProbeSlot("A")
	assert.Same(t, s1, s2)
	assert.NotSame(t, s1, m.getProbeSlot("B"))
}

func TestRunProbeOnceForSite_HappyAndBusy(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true, Cookie: "c=1"}).Error)
	m := newReminderMonitorForTest(db, time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))

	ok := m.RunProbeOnceForSite(context.Background(), "HDSKY")
	assert.True(t, ok)

	// Hold the lock → RunProbeOnceForSite returns false.
	release, held := m.TryAcquireProbeLock("HDSKY")
	require.True(t, held)
	assert.False(t, m.RunProbeOnceForSite(context.Background(), "HDSKY"))
	release()
}

func TestRunProbeOnceForSite_NilDB(t *testing.T) {
	m := &LoginReminderMonitor{}
	assert.False(t, m.RunProbeOnceForSite(context.Background(), "X"))
}

func TestRunProbeOnceForSiteLocked_MissingSite(t *testing.T) {
	db := newReminderTestDB(t)
	m := newReminderMonitorForTest(db, time.Now())
	require.NotPanics(t, func() { m.RunProbeOnceForSiteLocked(context.Background(), "nope") })
}

func TestRunReminderOnce_NilDBAndListPath(t *testing.T) {
	nilM := &LoginReminderMonitor{}
	require.NotPanics(t, func() { nilM.RunReminderOnce(context.Background()) })

	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)
	m := newReminderMonitorForTest(db, time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))
	require.NotPanics(t, func() { m.RunReminderOnce(context.Background()) })
}

func TestRunReminderOnce_CtxCancelled(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)
	m := newReminderMonitorForTest(db, time.Now())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.NotPanics(t, func() { m.RunReminderOnce(ctx) })
}

func TestSendTestReminder_ErrorPaths(t *testing.T) {
	nilM := &LoginReminderMonitor{}
	require.Error(t, nilM.SendTestReminder(context.Background(), "X"))

	db := newReminderTestDB(t)
	m := newReminderMonitorForTest(db, time.Now())
	// Site missing → error.
	require.Error(t, m.SendTestReminder(context.Background(), "nope"))

	// Site present but router nil → error.
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)
	err := m.SendTestReminder(context.Background(), "HDSKY")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "通知路由")
}

func TestEffectiveSourceForLog_AllBranches(t *testing.T) {
	assert.Equal(t, "none", effectiveSourceForLog(nil, time.Now()))
	assert.Equal(t, "none", effectiveSourceForLog(&models.SiteLoginState{}, time.Time{}))

	now := time.Now()
	la := now
	st := &models.SiteLoginState{LastAccessAt: &la}
	assert.Equal(t, "last_access", effectiveSourceForLog(st, now))

	api := now.Add(time.Hour)
	st2 := &models.SiteLoginState{ApiLastLoginAt: &api}
	assert.Equal(t, "api_last_login", effectiveSourceForLog(st2, api))

	ck := now.Add(2 * time.Hour)
	st3 := &models.SiteLoginState{CookieLastLoginAt: &ck}
	assert.Equal(t, "cookie_last_login", effectiveSourceForLog(st3, ck))

	ll := now.Add(3 * time.Hour)
	st4 := &models.SiteLoginState{LastLoginAt: &ll}
	assert.Equal(t, "last_login", effectiveSourceForLog(st4, ll))

	lv := now.Add(4 * time.Hour)
	st5 := &models.SiteLoginState{LastVisitAt: &lv}
	assert.Equal(t, "last_visit", effectiveSourceForLog(st5, lv))

	other := now.Add(99 * time.Hour)
	assert.Equal(t, "unknown", effectiveSourceForLog(&models.SiteLoginState{}, other))
}

func TestMaybeNotifyProbeFailure_Branches(t *testing.T) {
	db := newReminderTestDB(t)
	m := newReminderMonitorForTest(db, time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))
	ctx := context.Background()
	setting := models.SiteSetting{Name: "HDSKY"}

	// OK status → no-op (router nil, no panic).
	m.maybeNotifyProbeFailure(ctx, setting, &models.SiteLoginState{}, &sitelogin.ProbeResult{Status: sitelogin.OK})

	// Non-session-expired with nil LastProbeAt → returns early.
	m.maybeNotifyProbeFailure(ctx, setting, &models.SiteLoginState{}, &sitelogin.ProbeResult{Status: sitelogin.NETWORK_ERROR})

	// Non-session-expired but recent probe (< warn threshold) → early.
	recent := m.clock.Now().Add(-time.Minute)
	m.maybeNotifyProbeFailure(ctx, setting, &models.SiteLoginState{LastProbeAt: &recent},
		&sitelogin.ProbeResult{Status: sitelogin.NETWORK_ERROR})

	// SESSION_EXPIRED with router set → routes.
	rec := &captureRouter{}
	m.router = newCapturingRouter(rec)
	state := &models.SiteLoginState{NotificationChannelIDs: "[1]"}
	m.maybeNotifyProbeFailure(ctx, setting, state, &sitelogin.ProbeResult{Status: sitelogin.SESSION_EXPIRED})
	assert.Equal(t, int32(1), rec.count.Load())
}

func TestDaysRemaining_DefaultThreshold(t *testing.T) {
	now := time.Now()
	effective := now.Add(-10 * 24 * time.Hour)
	// Threshold 0 → defaults to 30.
	got := DaysRemaining(&models.SiteLoginState{}, effective, now)
	assert.Equal(t, 20, got)
}

func TestComputeTier_DefaultPreWarnAndBands(t *testing.T) {
	now := time.Now()
	mk := func(daysAgo, ban int) (*models.SiteLoginState, time.Time) {
		st := &models.SiteLoginState{BanThresholdDays: ban}
		return st, now.Add(-time.Duration(daysAgo) * 24 * time.Hour)
	}
	// ban=30, active 5 days ago → 25 remaining, preWarn default 10 → none.
	st, eff := mk(5, 30)
	assert.Equal(t, tierNone, ComputeTier(st, eff, now))

	// 29 days ago, ban 30 → 1 remaining → imminent.
	st, eff = mk(29, 30)
	assert.Equal(t, tierImminent, ComputeTier(st, eff, now))

	// 28 days ago → ~2 remaining → 3d band.
	st, eff = mk(28, 30)
	assert.Equal(t, tier3d, ComputeTier(st, eff, now))

	// 25 days ago → 5 remaining → 7d band.
	st, eff = mk(25, 30)
	assert.Equal(t, tier7d, ComputeTier(st, eff, now))

	// 20 days ago → 10 remaining, preWarn default 10 (remaining>preWarn is false) → 14d band.
	st, eff = mk(20, 30)
	assert.Equal(t, tier14d, ComputeTier(st, eff, now))

	// 15 days ago → 15 remaining, preWarn 20 → 30d band.
	st, eff = mk(15, 30)
	st.RemindBeforeDays = 20
	assert.Equal(t, tier30d, ComputeTier(st, eff, now))
}

func TestNewest_AllBranches(t *testing.T) {
	assert.True(t, newest(nil, nil).IsZero())
	a := time.Now()
	b := a.Add(time.Hour)
	assert.Equal(t, a, newest(&a, nil))
	assert.Equal(t, b, newest(nil, &b))
	assert.Equal(t, b, newest(&a, &b))
	assert.Equal(t, b, newest(&b, &a))
}

func TestLoadOrInitState_CreatesFresh(t *testing.T) {
	db := newReminderTestDB(t)
	m := newReminderMonitorForTest(db, time.Now())
	st, err := m.loadOrInitState("HDSKY")
	require.NoError(t, err)
	assert.Equal(t, "HDSKY", st.SiteName)
	assert.Equal(t, "auto", st.ProbeMode)

	// Second call loads the existing row.
	st2, err := m.loadOrInitState("HDSKY")
	require.NoError(t, err)
	assert.Equal(t, st.SiteName, st2.SiteName)
}

func TestRunProbeOnce_ProbesEnabledSites(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true, Cookie: "c=1"}).Error)
	m := newReminderMonitorForTest(db, time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))
	m.RunProbeOnce(context.Background())

	var state models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&state).Error)
	assert.NotNil(t, state.LastProbeAt)
}

func TestProbeSiteInternal_UnknownModeSkips(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName: "HDSKY", BanThresholdDays: 30, RemindBeforeDays: 10, ProbeMode: "weird-mode",
	}).Error)
	m := newReminderMonitorForTest(db, time.Now())
	m.probeSiteInternal(context.Background(), models.SiteSetting{Name: "HDSKY"}, false)

	var state models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&state).Error)
	assert.Nil(t, state.LastProbeAt, "unknown mode must skip probe")
}

func TestEvaluateReminder_FullRouteAndSave(t *testing.T) {
	db := newReminderTestDB(t)
	recorder := &captureRouter{}
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)
	access := now.Add(-29 * 24 * time.Hour) // 1 day remaining → imminent
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName: "HDSKY", BanThresholdDays: 30, RemindBeforeDays: 10,
		ReminderCron: "0 10,22 * * *", NotificationChannelIDs: "[1]",
		LastReminderTier: tierNone, LastAccessAt: &access,
	}).Error)

	m := newReminderMonitorForTest(db, now)
	m.router = newCapturingRouter(recorder)
	m.evaluateReminder(context.Background(), models.SiteSetting{Name: "HDSKY"}, now)

	assert.Equal(t, int32(1), recorder.count.Load())
	var state models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&state).Error)
	require.NotNil(t, state.LastReminderSentAt)
	assert.NotEqual(t, tierNone, state.LastReminderTier)
}

func TestEvaluateReminder_InvalidCronSkips(t *testing.T) {
	db := newReminderTestDB(t)
	now := time.Now()
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)
	access := now.Add(-29 * 24 * time.Hour)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName: "HDSKY", BanThresholdDays: 30, RemindBeforeDays: 10,
		ReminderCron: "not a cron", LastReminderTier: tierNone, LastAccessAt: &access,
	}).Error)
	m := newReminderMonitorForTest(db, now)
	require.NotPanics(t, func() { m.evaluateReminder(context.Background(), models.SiteSetting{Name: "HDSKY"}, now) })
}

func TestEvaluateReminder_ZeroEffectiveSkips(t *testing.T) {
	db := newReminderTestDB(t)
	now := time.Now()
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName: "HDSKY", BanThresholdDays: 30, RemindBeforeDays: 10, ReminderCron: "0 10,22 * * *",
	}).Error)
	m := newReminderMonitorForTest(db, now)
	require.NotPanics(t, func() { m.evaluateReminder(context.Background(), models.SiteSetting{Name: "HDSKY"}, now) })
}

func TestEvaluateReminder_TierNoneSkips(t *testing.T) {
	db := newReminderTestDB(t)
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)
	access := now.Add(-2 * 24 * time.Hour) // 28 days remaining, preWarn 10 → none
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName: "HDSKY", BanThresholdDays: 30, RemindBeforeDays: 10,
		ReminderCron: "0 10,22 * * *", LastAccessAt: &access,
	}).Error)
	m := newReminderMonitorForTest(db, now)
	recorder := &captureRouter{}
	m.router = newCapturingRouter(recorder)
	m.evaluateReminder(context.Background(), models.SiteSetting{Name: "HDSKY"}, now)
	assert.Equal(t, int32(0), recorder.count.Load())
}

func TestProbeSiteInternal_OKWithAccessAndLogin(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true, Cookie: "c=1"}).Error)

	access := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC).Unix()
	login := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC).Unix()
	m := newReminderMonitorForTest(db, time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))
	m.resolver = &fakeReminderResolver{
		def:  &v2.SiteDefinition{ID: "hdsky", Schema: v2.SchemaNexusPHP},
		site: &fakeReminderSite{info: v2.UserInfo{LastAccess: access, LastLogin: login}},
	}
	m.probeSiteInternal(context.Background(), models.SiteSetting{Name: "HDSKY"}, false)

	var state models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&state).Error)
	require.NotNil(t, state.LastAccessAt)
	assert.Equal(t, "OK", state.LastProbeStatus)
	assert.Equal(t, 0, state.ConsecutiveProbeFailures)
}

func TestProbeSiteInternal_ManualBypassesDisabledMode(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true, Cookie: "c=1"}).Error)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName: "HDSKY", BanThresholdDays: 30, RemindBeforeDays: 10, ProbeMode: ProbeModeDisabled,
	}).Error)

	access := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC).Unix()
	m := newReminderMonitorForTest(db, time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))
	m.resolver = &fakeReminderResolver{
		def:  &v2.SiteDefinition{ID: "hdsky", Schema: v2.SchemaNexusPHP},
		site: &fakeReminderSite{info: v2.UserInfo{LastAccess: access}},
	}
	m.probeSiteInternal(context.Background(), models.SiteSetting{Name: "HDSKY"}, true)

	var state models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&state).Error)
	require.NotNil(t, state.LastProbeAt)
}

func TestProbeSiteInternal_ResolveFailureSkips(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)
	m := newReminderMonitorForTest(db, time.Now())
	m.resolver = &fakeReminderResolver{err: errSendBoom}
	m.probeSiteInternal(context.Background(), models.SiteSetting{Name: "HDSKY"}, false)

	var state models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&state).Error)
	assert.Nil(t, state.LastProbeAt, "resolve failure must skip before persisting probe result")
}

func TestUpdateAllMonitoredProgress_UpdatesProgress(t *testing.T) {
	db := setupTestDB(t)
	srv := createMockQBitServer(t, 0.5, false)
	defer srv.Close()
	dlMgr := setupTestDownloaderManager(t, srv)
	monitor := NewFreeEndMonitor(db.DB, dlMgr)

	future := time.Now().Add(time.Hour)
	pushed := true
	ti := &models.TorrentInfo{
		SiteName: "s", TorrentID: "up", Title: "Up", PauseOnFreeEnd: true,
		FreeEndTime: &future, DownloaderTaskID: "test-hash-123", DownloaderName: "test-qbit", IsPushed: &pushed,
	}
	require.NoError(t, db.DB.Create(ti).Error)

	monitor.updateAllMonitoredProgress()

	var got models.TorrentInfo
	require.NoError(t, db.DB.First(&got, ti.ID).Error)
	assert.InDelta(t, 50.0, got.Progress, 0.5)
}

func TestSendTestReminder_HappyPath(t *testing.T) {
	db := newReminderTestDB(t)
	recorder := &captureRouter{}
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	access := now.Add(-5 * 24 * time.Hour)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName: "HDSKY", BanThresholdDays: 30, RemindBeforeDays: 10,
		NotificationChannelIDs: "[1]", LastAccessAt: &access,
	}).Error)
	m := newReminderMonitorForTest(db, now)
	m.router = newCapturingRouter(recorder)
	require.NoError(t, m.SendTestReminder(context.Background(), "HDSKY"))
	assert.Equal(t, int32(1), recorder.count.Load())
}

func TestRunProbeOnce_SingleflightBusySkips(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true, Cookie: "c=1"}).Error)
	m := newReminderMonitorForTest(db, time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))
	m.resolver = &fakeReminderResolver{
		def:  &v2.SiteDefinition{ID: "hdsky", Schema: v2.SchemaNexusPHP},
		site: &fakeReminderSite{info: v2.UserInfo{LastAccess: time.Now().Unix()}},
	}
	// Hold the site's probe lock so RunProbeOnce skips it via singleflight.
	release, ok := m.TryAcquireProbeLock("HDSKY")
	require.True(t, ok)
	m.RunProbeOnce(context.Background())
	release()

	var state models.SiteLoginState
	err := db.Where("site_name = ?", "HDSKY").First(&state).Error
	// Skipped → no state row created.
	assert.Error(t, err)
}

func TestRunProbeOnceForSiteLocked_HappyPath(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true, Cookie: "c=1"}).Error)
	m := newReminderMonitorForTest(db, time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))
	m.resolver = &fakeReminderResolver{
		def:  &v2.SiteDefinition{ID: "hdsky", Schema: v2.SchemaNexusPHP},
		site: &fakeReminderSite{info: v2.UserInfo{LastAccess: time.Now().Unix()}},
	}
	m.RunProbeOnceForSiteLocked(context.Background(), "HDSKY")
	var state models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&state).Error)
	assert.NotNil(t, state.LastProbeAt)
}

func TestEvaluateReminder_QuietHoursSkipsNonImminent(t *testing.T) {
	db := newReminderTestDB(t)
	now := time.Date(2026, 5, 18, 3, 0, 0, 0, time.UTC) // 03:00 quiet window
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)
	access := now.Add(-25 * 24 * time.Hour) // 5 days remaining → tier7d (not imminent)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName: "HDSKY", BanThresholdDays: 30, RemindBeforeDays: 10,
		ReminderCron: "0 3 * * *", LastReminderTier: tierNone, LastAccessAt: &access,
	}).Error)
	m := newReminderMonitorForTest(db, now)
	m.quietHours = QuietHours{Start: "00:00", End: "07:00"}
	recorder := &captureRouter{}
	m.router = newCapturingRouter(recorder)
	m.evaluateReminder(context.Background(), models.SiteSetting{Name: "HDSKY"}, now)
	assert.Equal(t, int32(0), recorder.count.Load(), "quiet-hours must suppress non-imminent tier")
}

func TestEvaluateReminder_WindowAlreadySentSkips(t *testing.T) {
	db := newReminderTestDB(t)
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)
	access := now.Add(-25 * 24 * time.Hour) // tier7d
	sent := now                             // already sent at this window (10:00)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName: "HDSKY", BanThresholdDays: 30, RemindBeforeDays: 10,
		ReminderCron: "0 10,22 * * *", LastReminderTier: tier7d,
		LastReminderSentAt: &sent, LastAccessAt: &access,
	}).Error)
	m := newReminderMonitorForTest(db, now)
	recorder := &captureRouter{}
	m.router = newCapturingRouter(recorder)
	m.evaluateReminder(context.Background(), models.SiteSetting{Name: "HDSKY"}, now)
	assert.Equal(t, int32(0), recorder.count.Load(), "already sent in window must skip")
}

func TestEvaluateReminder_RouterNilProceeds(t *testing.T) {
	db := newReminderTestDB(t)
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)
	access := now.Add(-29 * 24 * time.Hour) // imminent
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName: "HDSKY", BanThresholdDays: 30, RemindBeforeDays: 10,
		ReminderCron: "0 10,22 * * *", LastReminderTier: tierNone, LastAccessAt: &access,
	}).Error)
	m := newReminderMonitorForTest(db, now) // router nil
	m.evaluateReminder(context.Background(), models.SiteSetting{Name: "HDSKY"}, now)

	var state models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&state).Error)
	require.NotNil(t, state.LastReminderSentAt, "state saved even without router")
}

func TestLoadOrInitState_ReturnsExisting(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName: "HDSKY", BanThresholdDays: 45, RemindBeforeDays: 12, ProbeMode: "manual",
	}).Error)
	m := newReminderMonitorForTest(db, time.Now())
	st, err := m.loadOrInitState("HDSKY")
	require.NoError(t, err)
	assert.Equal(t, 45, st.BanThresholdDays)
	assert.Equal(t, "manual", st.ProbeMode)
}

func newReminderTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}, &models.SiteLoginState{}, &models.MigrationState{}))
	return db
}

type fakeReminderSite struct {
	info v2.UserInfo
	err  error
}

func (f *fakeReminderSite) ID() string { return "fake" }

func (f *fakeReminderSite) Name() string { return "Fake" }

func (f *fakeReminderSite) Kind() v2.SiteKind { return v2.SiteNexusPHP }

func (f *fakeReminderSite) Login(context.Context, v2.Credentials) error { return nil }

func (f *fakeReminderSite) Search(context.Context, v2.SearchQuery) ([]v2.TorrentItem, error) {
	return nil, nil
}

func (f *fakeReminderSite) GetUserInfo(context.Context) (v2.UserInfo, error) { return f.info, f.err }

func (f *fakeReminderSite) Download(context.Context, string) ([]byte, error) { return nil, nil }

func (f *fakeReminderSite) Close() error { return nil }

type fakeReminderResolver struct {
	def  *v2.SiteDefinition
	site v2.Site
	err  error
}

func (f *fakeReminderResolver) Resolve(setting models.SiteSetting) (*v2.SiteDefinition, v2.Site, error) {
	return f.def, f.site, f.err
}

type fakeDecryptor struct{}

func (fakeDecryptor) Decrypt(setting models.SiteSetting) (string, error) {
	return setting.Cookie, nil
}

type captureRouter struct {
	count atomic.Int32
	last  notify.Notification
}

func (c *captureRouter) ListNotificationConfs(context.Context) ([]models.NotificationConf, error) {
	return []models.NotificationConf{{ID: 1, ChannelType: "capture", Name: "capture", Enabled: true}}, nil
}

type captureChannel struct {
	sink *captureRouter
}

func (c *captureChannel) Type() string { return "capture" }

func (c *captureChannel) Init(context.Context, *models.NotificationConf) error { return nil }

func (c *captureChannel) SupportsInbound() bool { return false }

func (c *captureChannel) Send(_ context.Context, n notify.Notification) error {
	c.sink.last = n
	c.sink.count.Add(1)
	return nil
}

func (c *captureChannel) OnInbound(notify.InboundHandler) {}

func (c *captureChannel) Close(context.Context) error { return nil }

func (c *captureChannel) Healthy() bool { return true }

func newCapturingRouter(c *captureRouter) *notify.Router {
	registry := notify.NewRegistry()
	registry.Register("capture", func() notify.Channel { return &captureChannel{sink: c} })
	return notify.NewRouter(registry, nil, c)
}

type spyMonitor struct {
	*LoginReminderMonitor
	calls atomic.Int32
}

func TestComputeTier(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name     string
		banDays  int
		remind   int
		ageDays  int
		expected string
	}{
		{"none beyond pre-warn", 30, 10, 5, tierNone},
		{"30d band at edge of pre-warn", 30, 30, 5, tier30d},
		{"14d band edge", 30, 30, 17, tier14d},
		{"7d band", 30, 10, 25, tier7d},
		{"3d band", 30, 10, 28, tier3d},
		{"imminent at 1d", 30, 10, 29, tierImminent},
		{"imminent past deadline", 30, 10, 31, tierImminent},
		{"custom threshold 60 within pre-warn", 60, 14, 50, tier14d},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lastActive := now.Add(-time.Duration(tc.ageDays) * 24 * time.Hour)
			state := &models.SiteLoginState{
				BanThresholdDays: tc.banDays,
				RemindBeforeDays: tc.remind,
				LastAccessAt:     &lastActive,
			}
			effective := EffectiveLastActive(state, now)
			tier := ComputeTier(state, effective, now)
			assert.Equal(t, tc.expected, tier, "remaining=%d", DaysRemaining(state, effective, now))
		})
	}
}

func TestEffectiveLastActiveProbeWins(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	probe := now.Add(-3 * 24 * time.Hour)
	visit := now.Add(-1 * 24 * time.Hour)

	state := &models.SiteLoginState{
		LastAccessAt:             &probe,
		LastVisitAt:              &visit,
		ConsecutiveProbeFailures: 0,
	}
	got := EffectiveLastActive(state, now)
	assert.Equal(t, probe, got, "fresh probe should win even though visit is newer")
}

func TestEffectiveLastActiveFallbackToVisit(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	probe := now.Add(-30 * 24 * time.Hour)
	visit := now.Add(-2 * 24 * time.Hour)
	staleProbeAt := now.Add(-20 * time.Hour)

	state := &models.SiteLoginState{
		LastAccessAt:             &probe,
		LastVisitAt:              &visit,
		LastProbeAt:              &staleProbeAt,
		ConsecutiveProbeFailures: 5,
	}
	got := EffectiveLastActive(state, now)
	assert.Equal(t, probe, got, "last_access should win over extension visit even after stale probe failures")
}

func TestEffectiveLastActiveFallbackOnlyAfterStaleness(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	probe := now.Add(-30 * 24 * time.Hour)
	visit := now.Add(-2 * 24 * time.Hour)
	freshProbeAt := now.Add(-1 * time.Hour)

	state := &models.SiteLoginState{
		LastAccessAt:             &probe,
		LastVisitAt:              &visit,
		LastProbeAt:              &freshProbeAt,
		ConsecutiveProbeFailures: 1,
	}
	got := EffectiveLastActive(state, now)
	assert.Equal(t, probe, got, "1h failure shouldn't trigger fallback (need 12h+)")
}

func TestRunReminderOnceCronWindowDedup(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	monitor := newReminderMonitorForTest(db, time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))

	state := &models.SiteLoginState{
		SiteName:         "HDSKY",
		BanThresholdDays: 30,
		RemindBeforeDays: 10,
		ReminderCron:     "0 10,22 * * *",
		LastReminderTier: tierNone,
	}
	expired := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	state.LastAccessAt = &expired
	require.NoError(t, db.Create(state).Error)

	monitor.RunReminderOnce(context.Background())

	var first models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&first).Error)
	require.NotNil(t, first.LastReminderSentAt, "first run within cron window should fire")
	firstSentAt := *first.LastReminderSentAt

	advanceClock(monitor, 30*time.Minute)
	monitor.RunReminderOnce(context.Background())

	var second models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&second).Error)
	require.NotNil(t, second.LastReminderSentAt)
	assert.Equal(t, firstSentAt, *second.LastReminderSentAt, "same cron window should NOT re-fire (dedup)")
}

func TestRunReminderOnceTierEscalationFiresImmediately(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	monitor := newReminderMonitorForTest(db, time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))

	stateAccess := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName:         "HDSKY",
		BanThresholdDays: 30,
		RemindBeforeDays: 10,
		ReminderCron:     "0 10,22 * * *",
		LastReminderTier: tier7d,
		LastAccessAt:     &stateAccess,
	}).Error)

	advanceClock(monitor, 30*time.Minute)

	advanceClock(monitor, 4*24*time.Hour)
	monitor.RunReminderOnce(context.Background())

	var after models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&after).Error)
	require.NotNil(t, after.LastReminderSentAt, "tier escalation 7d→3d should fire even off cron tick")
	assert.Equal(t, tier3d, after.LastReminderTier)
}

func TestRunReminderOnceQuietHoursOverride(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	imminent := time.Date(2026, 5, 18, 3, 0, 0, 0, time.UTC)
	monitor := newReminderMonitorForTest(db, imminent)
	monitor.quietHours = QuietHours{Start: "23:00", End: "08:00"}

	stateAccess := time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName:         "HDSKY",
		BanThresholdDays: 30,
		RemindBeforeDays: 10,
		ReminderCron:     "* * * * *",
		LastReminderTier: tierNone,
		LastAccessAt:     &stateAccess,
	}).Error)

	monitor.RunReminderOnce(context.Background())

	var imminentRow models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&imminentRow).Error)
	require.NotNil(t, imminentRow.LastReminderSentAt, "banned-imminent must override quiet hours")
	assert.Equal(t, tierImminent, imminentRow.LastReminderTier)
}

func TestRunReminderOnceQuietHoursNonImminent(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	atQuiet := time.Date(2026, 5, 18, 3, 0, 0, 0, time.UTC)
	monitor := newReminderMonitorForTest(db, atQuiet)
	monitor.quietHours = QuietHours{Start: "23:00", End: "08:00"}

	stateAccess := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName:         "HDSKY",
		BanThresholdDays: 30,
		RemindBeforeDays: 10,
		ReminderCron:     "* * * * *",
		LastReminderTier: tierNone,
		LastAccessAt:     &stateAccess,
	}).Error)

	monitor.RunReminderOnce(context.Background())

	var row models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&row).Error)
	assert.Nil(t, row.LastReminderSentAt, "non-imminent tier in quiet hours must NOT fire")
}

func TestReminderSilenceWindowAfterMigration(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	migrationCompletedAt := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.MigrationState{SchemaVersion: 10, CompletedAt: migrationCompletedAt}).Error)

	monitor := newReminderMonitorForTest(db, migrationCompletedAt)
	advanceClock(monitor, 12*time.Hour)

	stateAccess := migrationCompletedAt.Add(-25 * 24 * time.Hour)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName:         "HDSKY",
		BanThresholdDays: 30,
		RemindBeforeDays: 10,
		ReminderCron:     "* * * * *",
		LastReminderTier: tierNone,
		LastAccessAt:     &stateAccess,
	}).Error)

	monitor.RunReminderOnce(context.Background())

	var row models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&row).Error)
	assert.Nil(t, row.LastReminderSentAt, "migration silence window should block non-imminent reminders within 24h")
	assert.Equal(t, tierNone, row.LastReminderTier)
}

func TestReminderResumesAfter24h(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	migrationCompletedAt := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.MigrationState{SchemaVersion: 10, CompletedAt: migrationCompletedAt}).Error)

	monitor := newReminderMonitorForTest(db, migrationCompletedAt)
	advanceClock(monitor, 25*time.Hour)

	stateAccess := migrationCompletedAt.Add(-25 * 24 * time.Hour)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName:         "HDSKY",
		BanThresholdDays: 30,
		RemindBeforeDays: 10,
		ReminderCron:     "* * * * *",
		LastReminderTier: tierNone,
		LastAccessAt:     &stateAccess,
	}).Error)

	monitor.RunReminderOnce(context.Background())

	var row models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&row).Error)
	require.NotNil(t, row.LastReminderSentAt, "reminders should resume after the 24h migration silence window")
	assert.Equal(t, migrationCompletedAt.Add(25*time.Hour), *row.LastReminderSentAt)
	assert.Equal(t, tier3d, row.LastReminderTier)
}

func TestSendTestReminderRoutesWithoutMutatingReminderState(t *testing.T) {
	db := newReminderTestDB(t)
	recorder := &captureRouter{}
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	recent := now.Add(-5 * 24 * time.Hour)
	sentAt := now.Add(-2 * time.Hour)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName:               "HDSKY",
		BanThresholdDays:       30,
		RemindBeforeDays:       10,
		ReminderCron:           "0 10,22 * * *",
		NotificationChannelIDs: "[1]",
		LastReminderTier:       tier7d,
		LastReminderSentAt:     &sentAt,
		LastAccessAt:           &recent,
	}).Error)

	monitor := newReminderMonitorForTest(db, now)
	monitor.router = newCapturingRouter(recorder)

	require.NoError(t, monitor.SendTestReminder(context.Background(), "HDSKY"))
	require.NoError(t, monitor.SendTestReminder(context.Background(), "HDSKY"))

	assert.Equal(t, int32(2), recorder.count.Load(), "test reminders should bypass router dedupe")
	assert.Equal(t, "[pt-tools][测试] 站点 HDSKY 登录提醒", recorder.last.Title)
	assert.Contains(t, recorder.last.Text, "这是一条测试提醒")
	assert.Contains(t, recorder.last.Text, "tier=none")

	var after models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&after).Error)
	assert.Equal(t, tier7d, after.LastReminderTier)
	require.NotNil(t, after.LastReminderSentAt)
	assert.Equal(t, sentAt, *after.LastReminderSentAt)
}

func TestRunProbeOnceUpdatesState(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true, Cookie: "abc=1"}).Error)

	access := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC).Unix()
	resolver := &fakeReminderResolver{
		def:  &v2.SiteDefinition{ID: "hdsky", Schema: v2.SchemaNexusPHP},
		site: &fakeReminderSite{info: v2.UserInfo{LastAccess: access}},
	}

	monitor := newReminderMonitorForTest(db, time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))
	monitor.resolver = resolver

	monitor.RunProbeOnce(context.Background())

	var state models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&state).Error)
	require.NotNil(t, state.LastAccessAt)
	assert.Equal(t, access, state.LastAccessAt.Unix())
	assert.Equal(t, "OK", state.LastProbeStatus)
	assert.Equal(t, 0, state.ConsecutiveProbeFailures)
}

func TestRunProbeOnceCountsFailures(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	resolver := &fakeReminderResolver{
		def:  &v2.SiteDefinition{ID: "hdsky", Schema: v2.SchemaNexusPHP},
		site: &fakeReminderSite{err: fmt.Errorf("boom: %w", v2.ErrSessionExpired)},
	}

	monitor := newReminderMonitorForTest(db, time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))
	monitor.resolver = resolver

	monitor.RunProbeOnce(context.Background())
	monitor.RunProbeOnce(context.Background())
	monitor.RunProbeOnce(context.Background())

	var state models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&state).Error)
	assert.Equal(t, 3, state.ConsecutiveProbeFailures)
	assert.Equal(t, "SESSION_EXPIRED", state.LastProbeStatus)
}

func TestStartStopIdempotent(t *testing.T) {
	db := newReminderTestDB(t)
	monitor := newReminderMonitorForTest(db, time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))
	monitor.probeEvery = 24 * time.Hour
	monitor.reminderTick = 24 * time.Hour

	monitor.Start()
	monitor.Start()
	time.Sleep(20 * time.Millisecond)
	monitor.Stop()
	monitor.Stop()
}

func TestDecodeChannelIDs(t *testing.T) {
	cases := []struct {
		raw  string
		want []uint
	}{
		{"", nil},
		{"[]", []uint{}},
		{"[1,3,5]", []uint{1, 3, 5}},
		{"not json", nil},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			assert.Equal(t, tc.want, decodeChannelIDs(tc.raw))
		})
	}
}

func TestTierIsHigher(t *testing.T) {
	assert.True(t, tierIsHigher(tier3d, tier7d))
	assert.True(t, tierIsHigher(tierImminent, tier3d))
	assert.False(t, tierIsHigher(tier7d, tier3d))
	assert.False(t, tierIsHigher(tierNone, tierNone))
}

func newReminderMonitorForTest(db *gorm.DB, now time.Time) *LoginReminderMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &LoginReminderMonitor{
		ctx:          ctx,
		cancel:       cancel,
		db:           db,
		router:       nil,
		resolver:     &fakeReminderResolver{def: &v2.SiteDefinition{ID: "fake"}, site: &fakeReminderSite{}},
		decryptor:    fakeDecryptor{},
		clock:        sitelogin.NewFakeClock(now),
		logger:       zap.NewNop().Sugar(),
		probeEvery:   6 * time.Hour,
		reminderTick: time.Minute,
		probeLocks:   make(map[string]*probeSlot),
	}
}

func advanceClock(m *LoginReminderMonitor, d time.Duration) {
	if fc, ok := m.clock.(*sitelogin.FakeClock); ok {
		fc.Advance(d)
	}
}

var _ = errors.New

func TestEffectiveLastActiveV2Semantic(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	lastAccessAt := now.Add(-7 * 24 * time.Hour)
	apiLastLoginAt := now.Add(-2 * 24 * time.Hour)
	cookieLastLoginAt := now.Add(-1 * 24 * time.Hour)
	legacyLastLoginAt := now.Add(-3 * 24 * time.Hour)
	lastVisitAt := now.Add(-4 * 24 * time.Hour)
	staleProbeAt := now.Add(-13 * time.Hour)
	freshProbeAt := now.Add(-2 * time.Hour)

	cases := []struct {
		name  string
		state *models.SiteLoginState
		want  time.Time
	}{
		{
			name:  "last_access_only_returns_last_access",
			state: &models.SiteLoginState{LastAccessAt: &lastAccessAt},
			want:  lastAccessAt,
		},
		{
			name:  "last_access_wins_over_api_last_login",
			state: &models.SiteLoginState{LastAccessAt: &lastAccessAt, ApiLastLoginAt: &apiLastLoginAt},
			want:  lastAccessAt,
		},
		{
			name:  "api_last_login_only_returns_api_last_login",
			state: &models.SiteLoginState{ApiLastLoginAt: &apiLastLoginAt},
			want:  apiLastLoginAt,
		},
		{
			name:  "cookie_last_login_only_returns_cookie_last_login",
			state: &models.SiteLoginState{CookieLastLoginAt: &cookieLastLoginAt},
			want:  cookieLastLoginAt,
		},
		{
			name:  "cookie_newer_than_api_returns_cookie",
			state: &models.SiteLoginState{ApiLastLoginAt: &apiLastLoginAt, CookieLastLoginAt: &cookieLastLoginAt},
			want:  cookieLastLoginAt,
		},
		{
			name:  "legacy_last_login_only_returns_legacy_last_login",
			state: &models.SiteLoginState{LastLoginAt: &legacyLastLoginAt},
			want:  legacyLastLoginAt,
		},
		{
			name: "last_visit_returns_only_after_probe_failures_stale",
			state: &models.SiteLoginState{
				LastVisitAt:              &lastVisitAt,
				LastProbeAt:              &staleProbeAt,
				ConsecutiveProbeFailures: 1,
			},
			want: lastVisitAt,
		},
		{
			name:  "last_visit_healthy_probe_returns_zero",
			state: &models.SiteLoginState{LastVisitAt: &lastVisitAt, ConsecutiveProbeFailures: 0},
			want:  time.Time{},
		},
		{
			name:  "all_empty_returns_zero",
			state: &models.SiteLoginState{},
			want:  time.Time{},
		},
		{
			name: "last_access_wins_over_last_visit_when_probe_healthy",
			state: &models.SiteLoginState{
				LastAccessAt:             &lastAccessAt,
				LastVisitAt:              &lastVisitAt,
				LastProbeAt:              &freshProbeAt,
				ConsecutiveProbeFailures: 0,
			},
			want: lastAccessAt,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EffectiveLastActive(tc.state, now)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestReminderMessageContainsLoginSuggestion(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	lastAccessAt := now.Add(-5 * 24 * time.Hour)
	channel := &capturingReminderChannel{}

	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName:         "HDSKY",
		BanThresholdDays: 30,
		RemindBeforeDays: 30,
		ReminderCron:     "* * * * *",
		LastReminderTier: tierNone,
		LastAccessAt:     &lastAccessAt,
	}).Error)

	monitor := newReminderMonitorForTest(db, now)
	monitor.router = newReminderRouterForTest(channel)
	monitor.RunReminderOnce(context.Background())

	notificationText := channel.lastNotificationText(t)
	for _, phrase := range []string{"建议", "访问", "浏览", "页面", "刷新", lastAccessAt.Format(time.RFC3339)} {
		assert.Contains(t, notificationText, phrase)
	}
}

func TestEffectiveLastActiveV1Compat(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		banDays  int
		remind   int
		ageDays  int
		wantTier string
	}{
		{name: "none beyond pre-warn", banDays: 30, remind: 10, ageDays: 5, wantTier: tierNone},
		{name: "7d band", banDays: 30, remind: 10, ageDays: 25, wantTier: tier7d},
		{name: "custom threshold 60 within pre-warn", banDays: 60, remind: 14, ageDays: 50, wantTier: tier14d},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lastAccessAt := now.Add(-time.Duration(tc.ageDays) * 24 * time.Hour)
			state := &models.SiteLoginState{
				BanThresholdDays: tc.banDays,
				RemindBeforeDays: tc.remind,
				LastAccessAt:     &lastAccessAt,
			}

			effective := EffectiveLastActive(state, now)
			gotTier := ComputeTier(state, effective, now)
			assert.Equal(t, tc.wantTier, gotTier)
		})
	}
}

type reminderConfLister struct{}

func (reminderConfLister) ListNotificationConfs(context.Context) ([]models.NotificationConf, error) {
	return []models.NotificationConf{{ID: 1, ChannelType: "capture-reminder", Name: "capture", Enabled: true}}, nil
}

type capturingReminderChannel struct {
	mu           sync.Mutex
	notification notify.Notification
}

func (c *capturingReminderChannel) Type() string { return "capture-reminder" }

func (c *capturingReminderChannel) Init(context.Context, *models.NotificationConf) error { return nil }

func (c *capturingReminderChannel) SupportsInbound() bool { return false }

func (c *capturingReminderChannel) Send(_ context.Context, notification notify.Notification) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.notification = notification
	return nil
}

func (c *capturingReminderChannel) OnInbound(notify.InboundHandler) {}

func (c *capturingReminderChannel) Close(context.Context) error { return nil }

func (c *capturingReminderChannel) Healthy() bool { return true }

func (c *capturingReminderChannel) lastNotificationText(t *testing.T) string {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	require.False(t, strings.TrimSpace(c.notification.Text) == "", "expected reminder notification text")
	return c.notification.Text
}

func newReminderRouterForTest(channel *capturingReminderChannel) *notify.Router {
	registry := notify.NewRegistry()
	registry.Register("capture-reminder", func() notify.Channel { return channel })
	return notify.NewRouter(registry, nil, reminderConfLister{})
}

func TestProbeSourceDispatch_HTTPCookie(t *testing.T) {
	state := &models.SiteLoginState{}
	login := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	access := time.Date(2026, 5, 18, 10, 30, 0, 0, time.UTC)
	result := &sitelogin.ProbeResult{
		Status:       sitelogin.OK,
		Source:       sitelogin.ProbeSourceHTTPCookie,
		LastLoginAt:  &login,
		LastAccessAt: &access,
	}

	dispatchProbeTimestamps(state, result)

	if assert.NotNil(t, state.CookieLastLoginAt) {
		assert.True(t, state.CookieLastLoginAt.Equal(login))
	}
	assert.Nil(t, state.ApiLastLoginAt)
	if assert.NotNil(t, state.LastLoginAt) {
		assert.True(t, state.LastLoginAt.Equal(login))
	}
	if assert.NotNil(t, state.LastAccessAt) {
		assert.True(t, state.LastAccessAt.Equal(access))
	}
}

func TestProbeSourceDispatch_HTTPAPIKey(t *testing.T) {
	state := &models.SiteLoginState{}
	login := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	access := time.Date(2026, 5, 18, 10, 30, 0, 0, time.UTC)
	result := &sitelogin.ProbeResult{
		Status:       sitelogin.OK,
		Source:       sitelogin.ProbeSourceHTTPAPIKey,
		LastLoginAt:  &login,
		LastAccessAt: &access,
	}

	dispatchProbeTimestamps(state, result)

	if assert.NotNil(t, state.ApiLastLoginAt) {
		assert.True(t, state.ApiLastLoginAt.Equal(login))
	}
	assert.Nil(t, state.CookieLastLoginAt)
	if assert.NotNil(t, state.LastLoginAt) {
		assert.True(t, state.LastLoginAt.Equal(login))
	}
	if assert.NotNil(t, state.LastAccessAt) {
		assert.True(t, state.LastAccessAt.Equal(access))
	}
}

func TestProbeSourceDispatch_Cloak(t *testing.T) {
	state := &models.SiteLoginState{}
	login := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	result := &sitelogin.ProbeResult{
		Status:      sitelogin.OK,
		Source:      sitelogin.ProbeSourceCloak,
		LastLoginAt: &login,
	}

	dispatchProbeTimestamps(state, result)

	if assert.NotNil(t, state.CookieLastLoginAt) {
		assert.True(t, state.CookieLastLoginAt.Equal(login))
	}
	assert.Nil(t, state.ApiLastLoginAt)
}

func TestProbeSourceDispatch_AccessOnlyNoLogin(t *testing.T) {
	state := &models.SiteLoginState{}
	access := time.Date(2026, 5, 18, 10, 30, 0, 0, time.UTC)
	result := &sitelogin.ProbeResult{
		Status:       sitelogin.OK,
		Source:       sitelogin.ProbeSourceHTTPCookie,
		LastAccessAt: &access,
	}

	dispatchProbeTimestamps(state, result)

	assert.Nil(t, state.ApiLastLoginAt)
	assert.Nil(t, state.CookieLastLoginAt)
	assert.Nil(t, state.LastLoginAt)
	if assert.NotNil(t, state.LastAccessAt) {
		assert.True(t, state.LastAccessAt.Equal(access))
	}
}

func TestProbeSourceDispatch_NormalizesToUTC(t *testing.T) {
	state := &models.SiteLoginState{}
	loc8 := time.FixedZone("CST", 8*3600)
	login := time.Date(2026, 5, 18, 10, 0, 0, 0, loc8)
	result := &sitelogin.ProbeResult{
		Status:      sitelogin.OK,
		Source:      sitelogin.ProbeSourceHTTPAPIKey,
		LastLoginAt: &login,
	}

	dispatchProbeTimestamps(state, result)

	if assert.NotNil(t, state.ApiLastLoginAt) {
		assert.Equal(t, time.UTC, state.ApiLastLoginAt.Location())
		assert.Equal(t, "2026-05-18T02:00:00Z", state.ApiLastLoginAt.Format(time.RFC3339))
	}
}

func TestProbeSourceDispatch_UnknownSourceFallsBackToCookie(t *testing.T) {
	state := &models.SiteLoginState{}
	login := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	result := &sitelogin.ProbeResult{
		Status:      sitelogin.OK,
		Source:      sitelogin.ProbeSource(""),
		LastLoginAt: &login,
	}

	dispatchProbeTimestamps(state, result)

	if assert.NotNil(t, state.CookieLastLoginAt) {
		assert.True(t, state.CookieLastLoginAt.Equal(login))
	}
	assert.Nil(t, state.ApiLastLoginAt)
}

func TestProbeModeStateMachine_Disabled(t *testing.T) {
	db := newReminderTestDB(t)
	ctx := context.Background()
	baseTime := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	clock := sitelogin.NewFakeClock(baseTime)

	site := &models.SiteSetting{Name: "TestSite", Enabled: true, AuthMethod: "cookie"}
	require.NoError(t, db.Create(site).Error)

	state := models.SiteLoginState{
		SiteName:         "TestSite",
		BanThresholdDays: 30,
		RemindBeforeDays: 10,
		ReminderCron:     "0 10,22 * * *",
		LastReminderTier: tierNone,
		ProbeMode:        ProbeModeDisabled,
		LastProbeAt:      nil,
	}
	require.NoError(t, db.Create(&state).Error)

	resolver := &fakeReminderResolver{
		def: &v2.SiteDefinition{},
		site: &fakeReminderSite{
			info: v2.UserInfo{},
		},
	}

	monitor := NewLoginReminderMonitor(LoginReminderConfig{
		DB:        db,
		Resolver:  resolver,
		Decryptor: fakeDecryptor{},
		Clock:     clock,
		Logger:    zap.NewNop().Sugar(),
	})

	monitor.RunProbeOnce(ctx)

	var updatedState models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "TestSite").First(&updatedState).Error)
	assert.Nil(t, updatedState.LastProbeAt, "LastProbeAt should remain nil when probe skipped due to disabled mode")
}

func TestProbeModeStateMachine_Manual_CronSkip(t *testing.T) {
	db := newReminderTestDB(t)
	ctx := context.Background()
	baseTime := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	clock := sitelogin.NewFakeClock(baseTime)

	site := &models.SiteSetting{Name: "TestSite", Enabled: true, AuthMethod: "cookie"}
	require.NoError(t, db.Create(site).Error)

	state := models.SiteLoginState{
		SiteName:         "TestSite",
		BanThresholdDays: 30,
		RemindBeforeDays: 10,
		ReminderCron:     "0 10,22 * * *",
		LastReminderTier: tierNone,
		ProbeMode:        ProbeModeManual,
		LastProbeAt:      nil,
	}
	require.NoError(t, db.Create(&state).Error)

	resolver := &fakeReminderResolver{
		def: &v2.SiteDefinition{},
		site: &fakeReminderSite{
			info: v2.UserInfo{},
		},
	}

	monitor := NewLoginReminderMonitor(LoginReminderConfig{
		DB:        db,
		Resolver:  resolver,
		Decryptor: fakeDecryptor{},
		Clock:     clock,
		Logger:    zap.NewNop().Sugar(),
	})

	monitor.RunProbeOnce(ctx)

	var updatedState models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "TestSite").First(&updatedState).Error)
	assert.Nil(t, updatedState.LastProbeAt, "cron-driven probe should not execute when mode is manual")
}

func TestProbeModeStateMachine_Auto(t *testing.T) {
	db := newReminderTestDB(t)
	ctx := context.Background()
	baseTime := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	clock := sitelogin.NewFakeClock(baseTime)

	site := &models.SiteSetting{Name: "TestSite", Enabled: true, AuthMethod: "cookie"}
	require.NoError(t, db.Create(site).Error)

	state := models.SiteLoginState{
		SiteName:         "TestSite",
		BanThresholdDays: 30,
		RemindBeforeDays: 10,
		ReminderCron:     "0 10,22 * * *",
		LastReminderTier: tierNone,
		ProbeMode:        ProbeModeAuto,
		LastProbeAt:      nil,
	}
	require.NoError(t, db.Create(&state).Error)

	resolver := &fakeReminderResolver{
		def: &v2.SiteDefinition{},
		site: &fakeReminderSite{
			info: v2.UserInfo{},
		},
	}

	monitor := NewLoginReminderMonitor(LoginReminderConfig{
		DB:        db,
		Resolver:  resolver,
		Decryptor: fakeDecryptor{},
		Clock:     clock,
		Logger:    zap.NewNop().Sugar(),
	})

	monitor.RunProbeOnce(ctx)

	var updatedState models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "TestSite").First(&updatedState).Error)
	assert.NotNil(t, updatedState.LastProbeAt, "auto mode should execute probe and update LastProbeAt")
	assert.True(t, updatedState.LastProbeAt.Equal(baseTime), "LastProbeAt should be set to clock time")
}

func TestProbeModeStateMachine_EmptyDefault(t *testing.T) {
	db := newReminderTestDB(t)
	ctx := context.Background()
	baseTime := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	clock := sitelogin.NewFakeClock(baseTime)

	site := &models.SiteSetting{Name: "TestSite", Enabled: true, AuthMethod: "cookie"}
	require.NoError(t, db.Create(site).Error)

	state := models.SiteLoginState{
		SiteName:         "TestSite",
		BanThresholdDays: 30,
		RemindBeforeDays: 10,
		ReminderCron:     "0 10,22 * * *",
		LastReminderTier: tierNone,
		ProbeMode:        "",
		LastProbeAt:      nil,
	}
	require.NoError(t, db.Create(&state).Error)

	resolver := &fakeReminderResolver{
		def: &v2.SiteDefinition{},
		site: &fakeReminderSite{
			info: v2.UserInfo{},
		},
	}

	monitor := NewLoginReminderMonitor(LoginReminderConfig{
		DB:        db,
		Resolver:  resolver,
		Decryptor: fakeDecryptor{},
		Clock:     clock,
		Logger:    zap.NewNop().Sugar(),
	})

	monitor.RunProbeOnce(ctx)

	var updatedState models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "TestSite").First(&updatedState).Error)
	assert.NotNil(t, updatedState.LastProbeAt, "empty mode should default to auto and execute probe")
}

func TestProbeModeStateMachine_UnknownMode(t *testing.T) {
	db := newReminderTestDB(t)
	ctx := context.Background()
	baseTime := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	clock := sitelogin.NewFakeClock(baseTime)

	site := &models.SiteSetting{Name: "TestSite", Enabled: true, AuthMethod: "cookie"}
	require.NoError(t, db.Create(site).Error)

	state := models.SiteLoginState{
		SiteName:         "TestSite",
		BanThresholdDays: 30,
		RemindBeforeDays: 10,
		ReminderCron:     "0 10,22 * * *",
		LastReminderTier: tierNone,
		ProbeMode:        "invalid_mode",
		LastProbeAt:      nil,
	}
	require.NoError(t, db.Create(&state).Error)

	resolver := &fakeReminderResolver{
		def: &v2.SiteDefinition{},
		site: &fakeReminderSite{
			info: v2.UserInfo{},
		},
	}

	monitor := NewLoginReminderMonitor(LoginReminderConfig{
		DB:        db,
		Resolver:  resolver,
		Decryptor: fakeDecryptor{},
		Clock:     clock,
		Logger:    zap.NewNop().Sugar(),
	})

	monitor.RunProbeOnce(ctx)

	var updatedState models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "TestSite").First(&updatedState).Error)
	assert.Nil(t, updatedState.LastProbeAt, "probe should not execute on unknown mode")
}

func TestProbeModeStateMachine_R18_QueuedSkip(t *testing.T) {
	db := newReminderTestDB(t)
	ctx := context.Background()
	baseTime := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	clock := sitelogin.NewFakeClock(baseTime)

	for _, name := range []string{"SiteA", "SiteB"} {
		site := &models.SiteSetting{Name: name, Enabled: true, AuthMethod: "cookie"}
		require.NoError(t, db.Create(site).Error)

		state := models.SiteLoginState{
			SiteName:         name,
			BanThresholdDays: 30,
			RemindBeforeDays: 10,
			ReminderCron:     "0 10,22 * * *",
			LastReminderTier: tierNone,
			ProbeMode:        ProbeModeAuto,
			LastProbeAt:      nil,
		}
		require.NoError(t, db.Create(&state).Error)
	}

	resolver := &fakeReminderResolver{
		def: &v2.SiteDefinition{},
		site: &fakeReminderSite{
			info: v2.UserInfo{},
		},
	}

	monitor := NewLoginReminderMonitor(LoginReminderConfig{
		DB:        db,
		Resolver:  resolver,
		Decryptor: fakeDecryptor{},
		Clock:     clock,
		Logger:    zap.NewNop().Sugar(),
	})

	sites, err := monitor.listEnabledSites()
	require.NoError(t, err)
	require.Equal(t, 2, len(sites))

	monitor.probeSite(ctx, sites[0])

	require.NoError(t, db.Model(&models.SiteLoginState{}).
		Where("site_name = ?", "SiteB").
		Update("probe_mode", ProbeModeDisabled).Error)

	monitor.probeSite(ctx, sites[1])

	var stateB models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "SiteB").First(&stateB).Error)
	assert.Nil(t, stateB.LastProbeAt, "SiteB probe should be skipped when mode changed to disabled mid-run")
}

func TestProbeModeStateMachine_Manual_ManualTrigger(t *testing.T) {
	db := newReminderTestDB(t)
	ctx := context.Background()
	baseTime := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	clock := sitelogin.NewFakeClock(baseTime)

	site := &models.SiteSetting{Name: "TestSite", Enabled: true, AuthMethod: "cookie"}
	require.NoError(t, db.Create(site).Error)

	state := models.SiteLoginState{
		SiteName:         "TestSite",
		BanThresholdDays: 30,
		RemindBeforeDays: 10,
		ReminderCron:     "0 10,22 * * *",
		LastReminderTier: tierNone,
		ProbeMode:        ProbeModeManual,
		LastProbeAt:      nil,
	}
	require.NoError(t, db.Create(&state).Error)

	resolver := &fakeReminderResolver{
		def: &v2.SiteDefinition{},
		site: &fakeReminderSite{
			info: v2.UserInfo{},
		},
	}

	monitor := NewLoginReminderMonitor(LoginReminderConfig{
		DB:        db,
		Resolver:  resolver,
		Decryptor: fakeDecryptor{},
		Clock:     clock,
		Logger:    zap.NewNop().Sugar(),
	})

	monitor.RunProbeOnceForSite(ctx, "TestSite")

	var updatedState models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "TestSite").First(&updatedState).Error)
	assert.NotNil(t, updatedState.LastProbeAt, "manual endpoint trigger should execute probe regardless of manual mode")
	assert.True(t, updatedState.LastProbeAt.Equal(baseTime))
}
