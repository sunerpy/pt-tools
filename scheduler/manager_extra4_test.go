// MIT License
// Copyright (c) 2025 pt-tools

package scheduler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/models"
)

func TestInitFreeEndMonitor_RegisteredSchedulerCallback(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() {
		global.GlobalDB = nil
		internal.RegisterTorrentScheduler(nil)
	})

	m := newTestManager(t)
	m.initFreeEndMonitor()
	require.NotNil(t, m.GetFreeEndMonitor())

	// Exercise the registered callback path (manager.go ScheduleTorrent wiring).
	future := time.Now().Add(time.Hour)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "cb", Title: "CB", PauseOnFreeEnd: true,
		FreeEndTime: &future, DownloaderTaskID: "task-cb", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)
	require.NotPanics(t, func() { internal.ScheduleTorrentForMonitoring(tor) })
	m.StopAll()
}

func TestReload_DefaultDownloaderPingFails(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })

	// Default downloader points at an unreachable URL → Ping fails → Reload
	// returns after the health-check branch (no jobs started).
	require.NoError(t, db.DB.Create(&models.DownloaderSetting{
		Name: "qb-dead", Type: "qbittorrent", URL: "http://127.0.0.1:1", Username: "a", Password: "b",
		Enabled: true, IsDefault: true,
	}).Error)

	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true},
		Sites:  map[models.SiteGroup]models.SiteConfig{},
	}
	require.NotPanics(t, func() { m.Reload(cfg) })
	assert.Empty(t, m.ListJobs())
	m.StopAll()
}
