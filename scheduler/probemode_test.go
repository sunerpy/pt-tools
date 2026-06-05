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
