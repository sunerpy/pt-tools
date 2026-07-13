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
)

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
