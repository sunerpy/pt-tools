// MIT License
// Copyright (c) 2025 pt-tools

package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	_ "github.com/sunerpy/pt-tools/site/v2/definitions"
)

func TestInitDownloaderManager_RegistersHealthyAndMapsSite(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })

	srv := createMockQBitServer(t, 0.5, false)
	defer srv.Close()

	ds := models.DownloaderSetting{
		Name: "qb-live", Type: "qbittorrent", URL: srv.URL, Username: "admin", Password: "admin",
		Enabled: true, IsDefault: true,
	}
	require.NoError(t, db.DB.Create(&ds).Error)
	require.NoError(t, db.DB.Create(&models.SiteSetting{Name: "mteam", DownloaderID: &ds.ID}).Error)

	m := newTestManager(t)
	m.initDownloaderManager()
	// Health check is async; give it a moment.
	time.Sleep(100 * time.Millisecond)
	dl, gerr := m.downloaderManager.GetDownloader("qb-live")
	require.NoError(t, gerr)
	assert.Equal(t, "qb-live", dl.GetName())
}

func TestReload_FullStartupWaitsForExistingJobs(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })

	srv := createMockQBitServer(t, 0.5, false)
	defer srv.Close()
	require.NoError(t, db.DB.Create(&models.DownloaderSetting{
		Name: "qb-live", Type: "qbittorrent", URL: srv.URL, Username: "admin", Password: "admin",
		Enabled: true, IsDefault: true,
	}).Error)

	s := core.NewConfigStore(db)
	e := true
	_, _ = s.UpsertSite(models.SiteGroup("springsunday"), models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})

	m := newTestManager(t)
	// Seed a running job so Reload's cancel + wait path executes.
	m.Start(models.SiteGroup("springsunday"), models.RSSConfig{Name: "pre"}, func(ctx context.Context) { <-ctx.Done() })

	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			"springsunday": {Enabled: &e, RSS: []models.RSSConfig{{Name: "r1", URL: "https://example.com/rss", IntervalMinutes: 1}}},
		},
	}
	require.NotPanics(t, func() { m.Reload(cfg) })
	assert.NotNil(t, m.GetFreeEndMonitor())
	time.Sleep(100 * time.Millisecond) // 等 StartAll 派生的 runner 执行 wg.Add(1) 后再 StopAll，避免 -race 竞态
	m.StopAll()
}

func TestRunRSSJobLegacy_RunsAndStops(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, DefaultEnabled: true,
	}))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runRSSJob(ctx, models.SiteGroup("springsunday"), models.RSSConfig{Name: "r", URL: "", IntervalMinutes: 1}, &rssSiteStub{})
		close(done)
	}()
	time.Sleep(60 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runRSSJob did not stop")
	}
}
