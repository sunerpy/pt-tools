// MIT License
// Copyright (c) 2025 pt-tools

package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

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
