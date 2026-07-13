// MIT License
// Copyright (c) 2025 pt-tools

package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
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
