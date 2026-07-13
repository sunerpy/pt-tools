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
