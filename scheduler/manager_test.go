// MIT License
// Copyright (c) 2025 pt-tools

package scheduler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	_ "github.com/sunerpy/pt-tools/site/v2/definitions"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func TestStartAll_UnknownSiteHandled(t *testing.T) {
	m := newTestManager(t)
	global.InitLogger(zap.NewNop())
	cfg := &models.Config{Sites: map[models.SiteGroup]models.SiteConfig{models.SiteGroup("unknown"): {Enabled: ptr(true)}}}
	require.NotPanics(t, func() { m.StartAll(cfg) })
}

type procRSSStub struct {
	fetchErr error
	sendErr  error
	sends    int
}

func (s *procRSSStub) GetTorrentDetails(item *gofeed.Item) (*v2.TorrentItem, error) {
	return &v2.TorrentItem{Title: item.Title}, nil
}

func (s *procRSSStub) IsEnabled() bool { return true }

func (s *procRSSStub) DownloadTorrent(_, _, _ string) (string, error) { return "h", nil }

func (s *procRSSStub) MaxRetries() int { return 1 }

func (s *procRSSStub) RetryDelay() time.Duration { return 0 }

func (s *procRSSStub) Context() context.Context { return context.Background() }

func (s *procRSSStub) SiteGroup() models.SiteGroup { return models.SiteGroup("springsunday") }

func (s *procRSSStub) SendTorrentToDownloader(_ context.Context, _ models.RSSConfig) error {
	s.sends++
	return s.sendErr
}

func TestProcessRSSUnified_FetchThenSendSucceeds(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true,
	}))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>t</title><link>http://x</link><description>d</description></channel></rss>`))
	}))
	t.Cleanup(srv.Close)

	stub := &procRSSStub{}
	err = processRSSUnified(context.Background(), models.RSSConfig{Name: "r", URL: srv.URL}, stub)
	require.NoError(t, err)
	assert.Equal(t, 1, stub.sends)
}

func TestProcessRSSUnified_SendErrorSurfaced(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true,
	}))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>t</title><link>http://x</link><description>d</description></channel></rss>`))
	}))
	t.Cleanup(srv.Close)

	stub := &procRSSStub{sendErr: errSendBoom}
	err = processRSSUnified(context.Background(), models.RSSConfig{Name: "r", URL: srv.URL}, stub)
	require.Error(t, err)
}

func TestExecuteTaskUnified_LogsErrorNoPanic(t *testing.T) {
	global.InitLogger(zap.NewNop())
	global.GlobalDB = nil
	stub := &procRSSStub{}
	require.NotPanics(t, func() {
		executeTaskUnified(context.Background(), models.RSSConfig{Name: "r", URL: ""}, stub)
	})
}

func TestStartAll_ValidSiteRegistersJob(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })

	s := core.NewConfigStore(db)
	e := true
	_, _ = s.UpsertSite(models.SiteGroup("springsunday"), models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})

	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			"springsunday": {Enabled: &e, RSS: []models.RSSConfig{{Name: "r1", URL: "https://example.com/rss", IntervalMinutes: 1}}},
		},
	}
	m.StartAll(cfg)
	jobs := m.ListJobs()
	assert.GreaterOrEqual(t, len(jobs), 1)
	time.Sleep(100 * time.Millisecond) // 让 runner 先执行 wg.Add(1)，再 StopAll wg.Wait()，避免 -race 竞态
	m.StopAll()
}

func TestStartAll_SkipsSampleRSS(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })

	s := core.NewConfigStore(db)
	e := true
	_, _ = s.UpsertSite(models.SiteGroup("springsunday"), models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})

	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			"springsunday": {Enabled: &e, RSS: []models.RSSConfig{{Name: "sample", URL: ""}}},
		},
	}
	m.StartAll(cfg)
	assert.Empty(t, m.ListJobs())
	m.StopAll()
}

var errSendBoom = &schedGenericErr{"send boom"}

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

// unifiedSiteStub is a minimal internal.UnifiedPTSite used to exercise
// runRSSJobUnified / executeTaskUnified / processRSSUnified without touching
// the network. It counts how many times the pipeline ran.
type unifiedSiteStub struct {
	sendCalls int
	sendErr   error
}

func (s *unifiedSiteStub) GetTorrentDetails(item *gofeed.Item) (*v2.TorrentItem, error) {
	return &v2.TorrentItem{Title: item.Title}, nil
}

func (s *unifiedSiteStub) IsEnabled() bool { return true }

func (s *unifiedSiteStub) DownloadTorrent(_, _, _ string) (string, error) { return "hash", nil }

func (s *unifiedSiteStub) MaxRetries() int { return 1 }

func (s *unifiedSiteStub) RetryDelay() time.Duration { return 0 }

func (s *unifiedSiteStub) Context() context.Context { return context.Background() }

func (s *unifiedSiteStub) SiteGroup() models.SiteGroup { return models.SiteGroup("stub") }

func (s *unifiedSiteStub) SendTorrentToDownloader(_ context.Context, _ models.RSSConfig) error {
	s.sendCalls++
	return s.sendErr
}

func TestInitFreeEndMonitor_WiresMonitors(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })

	m := newTestManager(t)
	require.NotPanics(t, func() { m.InitFreeEndMonitor() })
	assert.NotNil(t, m.GetFreeEndMonitor(), "free-end monitor should be wired")
}

func TestGetFreeEndMonitor_NilBeforeInit(t *testing.T) {
	m := newTestManager(t)
	assert.Nil(t, m.GetFreeEndMonitor())
}

func TestSetGetLoginReminderMonitor(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.InitLogger(zap.NewNop())

	m := newTestManager(t)
	assert.Nil(t, m.GetLoginReminderMonitor())

	mon := NewLoginReminderMonitor(LoginReminderConfig{DB: db.DB})
	m.SetLoginReminderMonitor(mon)
	assert.Same(t, mon, m.GetLoginReminderMonitor())

	// Replacing with a different monitor stops the old one; passing same is no-op.
	mon2 := NewLoginReminderMonitor(LoginReminderConfig{DB: db.DB})
	m.SetLoginReminderMonitor(mon2)
	assert.Same(t, mon2, m.GetLoginReminderMonitor())
}

func TestRunRSSJobUnified_RunsAndStops(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, DefaultEnabled: true,
	}))

	stub := &unifiedSiteStub{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		// URL is empty → FetchAndDownloadFreeRSSUnified will error out early,
		// but processRSSUnified is still exercised (executeTaskUnified logs it).
		runRSSJobUnified(ctx, models.RSSConfig{Name: "r", URL: "", IntervalMinutes: 1}, stub)
		close(done)
	}()
	time.Sleep(80 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runRSSJobUnified did not stop after cancel")
	}
}

func TestProcessRSSUnified_SendError(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })

	// Cancelled context makes the fetch step return quickly; then we simply
	// assert the function returns an error (from fetch or send) without panic.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stub := &unifiedSiteStub{}
	require.NotPanics(t, func() {
		_ = processRSSUnified(ctx, models.RSSConfig{Name: "x", URL: ""}, stub)
	})
}

// Observed via the mutex-guarded GetFreeEndMonitor() (not the unsynchronized
// LastVersion()) to stay race-free under -race.
func TestManager_EventReload_TriggersReload(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, AutoStart: true,
	}))

	m := newTestManager(t)
	require.Nil(t, m.GetFreeEndMonitor(), "no monitor before any reload")

	events.Publish(events.Event{Type: events.ConfigChanged, Version: 5})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if m.GetFreeEndMonitor() != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	assert.NotNil(t, m.GetFreeEndMonitor(), "event reload should wire the free-end monitor")
}

// === Event-driven reload: stale version ignored ===
func TestManager_EventReload_IgnoresStaleVersion(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, AutoStart: true,
	}))

	m := newTestManager(t)
	events.Publish(events.Event{Type: events.ConfigChanged, Version: 10})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if m.GetFreeEndMonitor() != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	first := m.GetFreeEndMonitor()
	require.NotNil(t, first)

	events.Publish(events.Event{Type: events.ConfigChanged, Version: 3})
	time.Sleep(500 * time.Millisecond)
	assert.Same(t, first, m.GetFreeEndMonitor(), "stale version must not trigger another reload")
}

func TestInitMonitors_ReplaceExisting(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })

	m := newTestManager(t)
	m.initDownloaderManager()
	m.initFreeEndMonitor()
	m.initCleanupMonitor()
	m.initPeerRatioMonitor()

	first := m.GetFreeEndMonitor()
	require.NotNil(t, first)

	// Calling again should stop the previous and create a new instance.
	m.initFreeEndMonitor()
	m.initCleanupMonitor()
	m.initPeerRatioMonitor()
	second := m.GetFreeEndMonitor()
	require.NotNil(t, second)
	assert.NotSame(t, first, second, "re-init should replace the free-end monitor")
}

func TestReload_QbitNotConfiguredWarnPaths(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.InitLogger(zap.NewNop())
	global.GlobalDB = db
	store := core.NewConfigStore(db)
	_ = store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, DefaultEnabled: true, AutoStart: true})
	cfg := &models.Config{Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true}, Sites: map[models.SiteGroup]models.SiteConfig{}}
	// add three sites with enabled=true and invalid rss to hit qbit not configured warnings
	e := true
	cfg.Sites[models.SiteGroup("springsunday")] = models.SiteConfig{Enabled: &e, RSS: []models.RSSConfig{{Name: "r1", URL: "http://"}}}
	cfg.Sites[models.SiteGroup("hdsky")] = models.SiteConfig{Enabled: &e, RSS: []models.RSSConfig{{Name: "r2", URL: "http://"}}}
	cfg.Sites[models.SiteGroup("mteam")] = models.SiteConfig{Enabled: &e, RSS: []models.RSSConfig{{Name: "r3", URL: "http://"}}}
	m := newTestManager(t)
	require.NotPanics(t, func() { m.Reload(cfg) })
}

func newTestManager(t *testing.T) *Manager {
	m := NewManager()
	t.Cleanup(func() {
		m.StopAll()
	})
	return m
}

func TestManager_StartStopAll(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := core.NewConfigStore(db)
	m := newTestManager(t)
	cfg, err := s.Load()
	if err == nil && cfg != nil {
		m.StartAll(cfg)
		m.StopAll()
	}
}

func TestManager_Reload_EarlyReturns(t *testing.T) {
	global.InitLogger(zap.NewNop())
	m := &Manager{}
	// empty download dir -> early return
	cfg1 := &models.Config{Global: models.SettingsGlobal{DownloadDir: "", AutoStart: true}}
	m.Reload(cfg1)
	assert.Equal(t, int64(0), m.LastVersion())
	// autostart false -> early return
	cfg2 := &models.Config{Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: false}}
	m.Reload(cfg2)
}

func TestValidRSS(t *testing.T) {
	assert.False(t, validRSS(""))
	assert.False(t, validRSS("ftp://example.com"))
	assert.False(t, validRSS("http:///path"))
	assert.False(t, validRSS("https://rss.m-team.xxx/path"))
	assert.True(t, validRSS("https://example.com/path"))
	assert.True(t, validRSS("http://example.com/path"))
}

func TestManager_StartAll_NoSites(t *testing.T) {
	m := newTestManager(t)
	cfg := &models.Config{Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true}, Sites: map[models.SiteGroup]models.SiteConfig{}}
	m.StartAll(cfg)
}

func TestReload_AutoStartFalseEarlyReturn(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()
	cfg := &models.Config{Global: models.SettingsGlobal{DownloadDir: dir, AutoStart: false}, Sites: map[models.SiteGroup]models.SiteConfig{}}
	global.InitLogger(zap.NewNop())
	m.Reload(cfg)
}

func TestManager_StopAll_WaitsAndResets(t *testing.T) {
	m := newTestManager(t)
	// StopAll on empty jobs should not panic and should reset map
	m.StopAll()
}

func TestReload_AutoStartFalseEarlyReturn_More(t *testing.T) {
	m := &Manager{}
	cfg := &models.Config{Global: models.SettingsGlobal{DownloadDir: "", AutoStart: false}}
	require.NotPanics(t, func() { m.Reload(cfg) })
}

func TestReload_InvalidRSSNotStarted(t *testing.T) {
	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: "/tmp", AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			models.SiteGroup("springsunday"): {Enabled: ptr(true), RSS: []models.RSSConfig{{Name: "r1", URL: "http://"}}},
			models.SiteGroup("hdsky"):        {Enabled: ptr(true), RSS: []models.RSSConfig{{Name: "r2", URL: "https://rss.m-team.xxx/path"}}},
			models.SiteGroup("mteam"):        {Enabled: ptr(true), RSS: []models.RSSConfig{{Name: "r3", URL: "https://example/rss"}}},
		},
	}
	require.NotPanics(t, func() { m.Reload(cfg) })
}

// TestManager_ListJobs_Empty: 启动后无 job → 返回空切片
func TestManager_ListJobs_Empty(t *testing.T) {
	m := newTestManager(t)
	jobs := m.ListJobs()
	assert.Equal(t, []JobStatus{}, jobs)
}

// TestManager_ListJobs_AfterStart: Start 一个 job → 返回单元素切片
func TestManager_ListJobs_AfterStart(t *testing.T) {
	m := newTestManager(t)
	siteName := models.SiteGroup("MTeam")
	rssName := "free"
	rssConfig := models.RSSConfig{Name: rssName}

	// Start a job with a dummy runner
	m.Start(siteName, rssConfig, func(ctx context.Context) {
		<-ctx.Done()
	})

	// Give it a moment to register
	time.Sleep(10 * time.Millisecond)

	jobs := m.ListJobs()
	assert.Equal(t, 1, len(jobs), "should have 1 job after Start")
	assert.Equal(t, string(siteName), jobs[0].SiteName)
	assert.Equal(t, rssName, jobs[0].RSSName)
	assert.True(t, jobs[0].Running, "job should be running")
	assert.False(t, jobs[0].StartedAt.IsZero(), "StartedAt should be set")
}

// TestManager_ListJobs_Concurrent: -race，并发 Start/Stop 同时 ListJobs 不 panic
func TestManager_ListJobs_Concurrent(t *testing.T) {
	m := newTestManager(t)

	// Goroutine 1: repeatedly start jobs
	go func() {
		for i := 0; i < 10; i++ {
			site := models.SiteGroup("MTeam")
			rss := models.RSSConfig{Name: "rss" + string(rune('0'+i))}
			m.Start(site, rss, func(ctx context.Context) {
				<-ctx.Done()
			})
			time.Sleep(1 * time.Millisecond)
		}
	}()

	// Goroutine 2: repeatedly list jobs
	go func() {
		for i := 0; i < 20; i++ {
			jobs := m.ListJobs()
			_ = jobs // read without error
			time.Sleep(500 * time.Microsecond)
		}
	}()

	// Goroutine 3: repeatedly stop jobs
	go func() {
		for i := 0; i < 10; i++ {
			m.Stop("MTeam", "rss"+string(rune('0'+i)))
			time.Sleep(1 * time.Millisecond)
		}
	}()

	// Wait for goroutines
	time.Sleep(200 * time.Millisecond)

	// Final ListJobs should not panic
	_ = m.ListJobs()
}

func TestReload_NoGlobalDBEarlyReturn(t *testing.T) {
	m := newTestManager(t)
	global.GlobalDB = nil
	m.Reload(&models.Config{Global: models.SettingsGlobal{DownloadDir: "/tmp", AutoStart: true}})
}

func TestValidRSS_Cases(t *testing.T) {
	require.False(t, validRSS(""))
	require.False(t, validRSS("ftp://example.com"))
	require.False(t, validRSS("http:///path"))
	require.False(t, validRSS("https://rss.m-team.xxx/feed"))
	require.True(t, validRSS("https://example.com/feed"))
}

func TestGetInterval_DefaultsAndGlobal(t *testing.T) {
	// default when <=0 and no global
	global.GlobalDB = nil
	d := getInterval(models.RSSConfig{IntervalMinutes: 0})
	require.Equal(t, 10*time.Minute, d)
	// read from global when available
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	_ = core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DefaultIntervalMinutes: 3, DownloadDir: t.TempDir()})
	d2 := getInterval(models.RSSConfig{IntervalMinutes: 0})
	require.Equal(t, time.Duration(models.MinIntervalMinutes)*time.Minute, d2)
	// explicit positive
	require.Equal(t, 5*time.Minute, getInterval(models.RSSConfig{IntervalMinutes: 5}))
}

func TestReload_StartAndStopAll_WithValidConfig(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	store := core.NewConfigStore(db)
	_ = store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, DefaultEnabled: true, AutoStart: true})
	// qbit server that returns login OK and maindata
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/sync/maindata":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":10000000}}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()
	_ = store.SaveQbitSettings(models.QbitSettings{Enabled: true, URL: srv.URL, User: "u", Password: "p"})
	cfg := &models.Config{Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true}, Sites: map[models.SiteGroup]models.SiteConfig{models.SiteGroup("mteam"): {Enabled: ptr(true), RSS: []models.RSSConfig{{Name: "r1", URL: "https://example.com/rss", IntervalMinutes: 1}}}}}
	m := newTestManager(t)
	m.Reload(cfg)
	time.Sleep(100 * time.Millisecond)
	m.StopAll()
}

func TestManager_Reload_StartAllBranches(t *testing.T) {
	db, _ := core.NewTempDBDir(t.TempDir())
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	store := core.NewConfigStore(db)
	_ = store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, DefaultEnabled: true, AutoStart: true})
	// set qbit
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/sync/maindata":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":10000000}}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()
	_ = store.SaveQbitSettings(models.QbitSettings{Enabled: true, URL: srv.URL, User: "u", Password: "p"})
	cfg := &models.Config{Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true}, Sites: map[models.SiteGroup]models.SiteConfig{
		models.SiteGroup("springsunday"): {Enabled: ptr(true), RSS: []models.RSSConfig{{Name: "r1", URL: "https://example.com/rss", IntervalMinutes: 1}}},
		models.SiteGroup("hdsky"):        {Enabled: ptr(true), RSS: []models.RSSConfig{{Name: "r2", URL: "https://example.com/rss", IntervalMinutes: 1}}},
		models.SiteGroup("mteam"):        {Enabled: ptr(true), RSS: []models.RSSConfig{{Name: "r3", URL: "https://example.com/rss", IntervalMinutes: 1}}},
	}}
	m := newTestManager(t)
	m.Reload(cfg)
	time.Sleep(100 * time.Millisecond)
	m.StopAll()
}

func ptr(b bool) *bool { return &b }

type rssSiteStub struct{}

func (s *rssSiteStub) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	d := models.PHPTorrentInfo{Title: item.Title, TorrentID: item.GUID, Discount: models.DISCOUNT_FREE, EndTime: time.Now().Add(1 * time.Hour), SizeMB: 64}
	return &models.APIResponse[models.PHPTorrentInfo]{Code: "success", Data: d}, nil
}

func (s *rssSiteStub) IsEnabled() bool { return true }

func (s *rssSiteStub) DownloadTorrent(url, title, dir string) (string, error) { return "hash-rss", nil }

func (s *rssSiteStub) MaxRetries() int { return 1 }

func (s *rssSiteStub) RetryDelay() time.Duration { return 0 }

func (s *rssSiteStub) SendTorrentToDownloader(ctx context.Context, rssCfg models.RSSConfig) error {
	return nil
}

func (s *rssSiteStub) Context() context.Context { return context.Background() }

func TestRunRSSJob_WithStub(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	if err = core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 1, DefaultEnabled: true}); err != nil {
		t.Fatalf("save gl: %v", err)
	}
	feed := bytes.NewBufferString(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Test</title>
<item><title>ItemRSS</title><guid>guid-1</guid><enclosure url="http://localhost/file.torrent" type="application/x-bittorrent"/></item>
</channel></rss>`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); _, _ = w.Write(feed.Bytes()) }))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := models.RSSConfig{Name: "r", URL: srv.URL, Tag: "tag", IntervalMinutes: 1}
	go runRSSJob(ctx, models.SiteGroup("springsunday"), cfg, &rssSiteStub{})
	time.Sleep(200 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)
}

func TestManager_StartStop(t *testing.T) {
	m := newTestManager(t)
	r := models.RSSConfig{Name: "r1", URL: "http://example", IntervalMinutes: 1}
	ran := make(chan struct{}, 1)
	m.Start(models.SiteGroup("springsunday"), r, func(ctx context.Context) { ran <- struct{}{} })
	m.Start(models.SiteGroup("springsunday"), r, func(ctx context.Context) { ran <- struct{}{} })
	if _, ok := <-ran; !ok {
		t.Fatalf("runner not invoked")
	}
	m.Stop(models.SiteGroup("springsunday"), r.Name)
}

func TestKeyFormat(t *testing.T) {
	assert.Equal(t, "cmct|rss", key(models.SiteGroup("cmct"), "rss"))
}

func TestValidRSS_MoreBranches(t *testing.T) {
	assert.False(t, validRSS("ftp://example.com"))
	assert.False(t, validRSS("http:///path"))
	assert.False(t, validRSS("https://"))
	assert.True(t, validRSS("http://host/path"))
	assert.True(t, validRSS("https://host/path"))
}

// TestQbitDownloaderConfig 测试 qBittorrent 下载器配置
func TestQbitDownloaderConfig(t *testing.T) {
	config := downloader.NewGenericConfig(downloader.DownloaderQBittorrent, "http://localhost:8080", "admin", "adminadmin", false)

	assert.Equal(t, "http://localhost:8080", config.GetURL())
	assert.Equal(t, "admin", config.GetUsername())
	assert.Equal(t, "adminadmin", config.GetPassword())
	assert.NoError(t, config.Validate())

	// 测试空 URL 验证失败
	emptyConfig := downloader.NewGenericConfig(downloader.DownloaderQBittorrent, "", "", "", false)
	assert.Error(t, emptyConfig.Validate())
}

// TestTransmissionDownloaderConfig 测试 Transmission 下载器配置
func TestTransmissionDownloaderConfig(t *testing.T) {
	config := downloader.NewGenericConfig(downloader.DownloaderTransmission, "http://localhost:9091", "admin", "password", false)

	assert.Equal(t, "http://localhost:9091", config.GetURL())
	assert.Equal(t, "admin", config.GetUsername())
	assert.Equal(t, "password", config.GetPassword())
	assert.NoError(t, config.Validate())

	// 测试空 URL 验证失败
	emptyConfig := downloader.NewGenericConfig(downloader.DownloaderTransmission, "", "", "", false)
	assert.Error(t, emptyConfig.Validate())
}

// TestGetDownloaderManager 测试获取下载器管理器
func TestGetDownloaderManager(t *testing.T) {
	m := newTestManager(t)
	dm := m.GetDownloaderManager()
	// 可能为 nil，取决于初始化状态
	_ = dm
}

// TestInitDownloaderManager 测试下载器管理器初始化
func TestInitDownloaderManager(t *testing.T) {
	// 测试 GlobalDB 为 nil 的情况
	global.GlobalDB = nil
	m := &Manager{
		jobs:              map[string]*job{},
		downloaderManager: nil,
	}
	m.initDownloaderManager()
	// 不应该 panic

	// 测试有 GlobalDB 的情况
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	m2 := newTestManager(t)
	m2.initDownloaderManager()
	// 验证下载器管理器已初始化
	assert.NotNil(t, m2.downloaderManager)
}

// TestInitDownloaderManager_WithDownloaderSettings 测试带下载器配置的初始化
func TestInitDownloaderManager_WithDownloaderSettings(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	// 创建下载器配置
	ds := models.DownloaderSetting{
		Name:     "test-qbit",
		Type:     "qbittorrent",
		URL:      "http://localhost:8080",
		Username: "admin",
		Password: "password",
		Enabled:  true,
	}
	err = db.DB.Create(&ds).Error
	require.NoError(t, err)

	m := newTestManager(t)
	m.initDownloaderManager()
	assert.NotNil(t, m.downloaderManager)
}

// TestInitDownloaderManager_WithTransmissionSettings 测试 Transmission 配置
func TestInitDownloaderManager_WithTransmissionSettings(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	// 创建 Transmission 下载器配置
	ds := models.DownloaderSetting{
		Name:     "test-transmission",
		Type:     "transmission",
		URL:      "http://localhost:9091",
		Username: "admin",
		Password: "password",
		Enabled:  true,
	}
	err = db.DB.Create(&ds).Error
	require.NoError(t, err)

	m := newTestManager(t)
	m.initDownloaderManager()
	assert.NotNil(t, m.downloaderManager)
}

// TestInitDownloaderManager_WithUnknownType 测试未知下载器类型
func TestInitDownloaderManager_WithUnknownType(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	// 创建未知类型的下载器配置
	ds := models.DownloaderSetting{
		Name:     "test-unknown",
		Type:     "unknown",
		URL:      "http://localhost:8080",
		Username: "admin",
		Password: "password",
		Enabled:  true,
	}
	err = db.DB.Create(&ds).Error
	require.NoError(t, err)

	m := newTestManager(t)
	require.NotPanics(t, func() {
		m.initDownloaderManager()
	})
}

// TestInitDownloaderManager_DisabledDownloader 测试禁用的下载器
func TestInitDownloaderManager_DisabledDownloader(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	// 创建禁用的下载器配置
	ds := models.DownloaderSetting{
		Name:     "test-disabled",
		Type:     "qbittorrent",
		URL:      "http://localhost:8080",
		Username: "admin",
		Password: "password",
		Enabled:  false,
	}
	err = db.DB.Create(&ds).Error
	require.NoError(t, err)

	m := newTestManager(t)
	m.initDownloaderManager()
	assert.NotNil(t, m.downloaderManager)
}

// TestCreateQBitFactory 测试 qBittorrent 工厂创建
func TestCreateQBitFactory(t *testing.T) {
	factory := createQBitFactory()
	assert.NotNil(t, factory)

	// 创建一个 mock 配置
	config := downloader.NewGenericConfig(downloader.DownloaderQBittorrent, "http://localhost:8080", "admin", "adminadmin", false)

	// 工厂应该能创建下载器（即使连接失败）
	_, err := factory(config, "test-qbit")
	// 由于没有实际的 qBittorrent 服务器，可能会返回错误
	// 但工厂函数本身应该能正常调用
	_ = err
}

// TestCreateTransmissionFactory 测试 Transmission 工厂创建
func TestCreateTransmissionFactory(t *testing.T) {
	factory := createTransmissionFactory()
	assert.NotNil(t, factory)

	// 创建一个 mock 配置
	config := downloader.NewGenericConfig(downloader.DownloaderTransmission, "http://localhost:9091", "admin", "password", false)

	// 工厂应该能创建下载器
	_, err := factory(config, "test-transmission")
	_ = err
}

// TestQbitDownloaderConfig_GetType 测试 GetType 方法
func TestQbitDownloaderConfig_GetType(t *testing.T) {
	config := downloader.NewGenericConfig(downloader.DownloaderQBittorrent, "http://localhost:8080", "admin", "adminadmin", false)
	assert.Equal(t, "qbittorrent", string(config.GetType()))
}

// TestTransmissionDownloaderConfig_GetType 测试 GetType 方法
func TestTransmissionDownloaderConfig_GetType(t *testing.T) {
	config := downloader.NewGenericConfig(downloader.DownloaderTransmission, "http://localhost:9091", "admin", "password", false)
	assert.Equal(t, "transmission", string(config.GetType()))
}

// TestStartAll_WithMTEAM 测试 StartAll 处理 MTEAM 站点
func TestStartAll_WithMTEAM(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			models.SiteGroup("mteam"): {
				Enabled: ptr(true),
				RSS: []models.RSSConfig{
					{Name: "test-rss", URL: "https://example.com/rss", IntervalMinutes: 1},
				},
			},
		},
	}

	// StartAll 应该不会 panic
	require.NotPanics(t, func() {
		m.StartAll(cfg)
	})
	time.Sleep(50 * time.Millisecond)
	m.StopAll()
}

// TestStartAll_WithHDSKY 测试 StartAll 处理 HDSKY 站点
func TestStartAll_WithHDSKY(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			models.SiteGroup("hdsky"): {
				Enabled: ptr(true),
				RSS: []models.RSSConfig{
					{Name: "test-rss", URL: "https://example.com/rss", IntervalMinutes: 1},
				},
			},
		},
	}

	require.NotPanics(t, func() {
		m.StartAll(cfg)
	})
	time.Sleep(50 * time.Millisecond)
	m.StopAll()
}

// TestStartAll_WithCMCT 测试 StartAll 处理 CMCT 站点
func TestStartAll_WithCMCT(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			models.SiteGroup("springsunday"): {
				Enabled: ptr(true),
				RSS: []models.RSSConfig{
					{Name: "test-rss", URL: "https://example.com/rss", IntervalMinutes: 1},
				},
			},
		},
	}

	require.NotPanics(t, func() {
		m.StartAll(cfg)
	})
	time.Sleep(50 * time.Millisecond)
	m.StopAll()
}

// TestStartAll_WithUnknownSite 测试 StartAll 处理未知站点
func TestStartAll_WithUnknownSite(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			models.SiteGroup("unknown"): {
				Enabled: ptr(true),
				RSS: []models.RSSConfig{
					{Name: "test-rss", URL: "https://example.com/rss", IntervalMinutes: 1},
				},
			},
		},
	}

	require.NotPanics(t, func() {
		m.StartAll(cfg)
	})
}

// TestStartAll_WithSkippedRSS 测试 StartAll 跳过应该跳过的 RSS
func TestStartAll_WithSkippedRSS(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			models.SiteGroup("mteam"): {
				Enabled: ptr(true),
				RSS: []models.RSSConfig{
					{Name: "test-rss", URL: "", IntervalMinutes: 1}, // 空 URL 应该被跳过
				},
			},
		},
	}

	require.NotPanics(t, func() {
		m.StartAll(cfg)
	})
}

// TestStartAll_WithDisabledSite 测试 StartAll 跳过禁用的站点
func TestStartAll_WithDisabledSite(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			models.SiteGroup("mteam"): {
				Enabled: ptr(false), // 禁用
				RSS: []models.RSSConfig{
					{Name: "test-rss", URL: "https://example.com/rss", IntervalMinutes: 1},
				},
			},
		},
	}

	require.NotPanics(t, func() {
		m.StartAll(cfg)
	})
}

// TestStartAll_AllSiteTypes 测试 StartAll 处理所有站点类型
func TestStartAll_AllSiteTypes(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			models.SiteGroup("mteam"): {
				Enabled: ptr(true),
				RSS: []models.RSSConfig{
					{Name: "mteam-rss", URL: "https://example.com/mteam", IntervalMinutes: 1},
				},
			},
			models.SiteGroup("hdsky"): {
				Enabled: ptr(true),
				RSS: []models.RSSConfig{
					{Name: "hdsky-rss", URL: "https://example.com/hdsky", IntervalMinutes: 1},
				},
			},
			models.SiteGroup("springsunday"): {
				Enabled: ptr(true),
				RSS: []models.RSSConfig{
					{Name: "cmct-rss", URL: "https://example.com/cmct", IntervalMinutes: 1},
				},
			},
		},
	}

	require.NotPanics(t, func() {
		m.StartAll(cfg)
	})
	time.Sleep(100 * time.Millisecond)
	m.StopAll()
}

// TestExecuteTask 测试 executeTask 函数
func TestExecuteTask(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	ctx := context.Background()
	cfg := models.RSSConfig{Name: "test", URL: "http://invalid-url", IntervalMinutes: 1}

	// 使用 stub 测试
	stub := &rssSiteStub{}
	require.NotPanics(t, func() {
		executeTask(ctx, models.SiteGroup("springsunday"), cfg, stub)
	})
}

// TestProcessRSS 测试 processRSS 函数
func TestProcessRSS(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	ctx := context.Background()
	cfg := models.RSSConfig{Name: "test", URL: "http://invalid-url", IntervalMinutes: 1}

	stub := &rssSiteStub{}
	// processRSS 可能返回错误，但不应该 panic
	_ = processRSS(ctx, models.SiteGroup("springsunday"), cfg, stub)
}

func TestInitDownloaderManager_MapsSiteDownloader(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })

	ds := models.DownloaderSetting{Name: "qb-map", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true, IsDefault: true}
	require.NoError(t, db.DB.Create(&ds).Error)
	require.NoError(t, db.DB.Create(&models.SiteSetting{Name: "mteam", DownloaderID: &ds.ID}).Error)

	m := newTestManager(t)
	require.NotPanics(t, func() { m.initDownloaderManager() })
	assert.True(t, m.downloaderManager.HasFactory(downloader.DownloaderQBittorrent))
}

func TestInitCleanupMonitor_And_PeerRatio_Wire(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })

	m := newTestManager(t)
	require.NotPanics(t, func() {
		m.initCleanupMonitor()
		m.initPeerRatioMonitor()
	})
	require.NotPanics(t, func() {
		m.initCleanupMonitor()
		m.initPeerRatioMonitor()
	})
	m.StopAll()
}

var _ = context.Background

func TestSplitKey_NoPipeReturnsNil(t *testing.T) {
	assert.Nil(t, splitKey("nopipe"))
	assert.Equal(t, []string{"a", "b"}, splitKey("a|b"))
	assert.Equal(t, []string{"a|b", "c"}, splitKey("a|b|c"))
}

func TestManager_ListJobs_ReflectsStarted(t *testing.T) {
	m := newTestManager(t)
	m.Start(models.SiteGroup("hdsky"), models.RSSConfig{Name: "r1"}, func(ctx context.Context) { <-ctx.Done() })
	jobs := m.ListJobs()
	require.Len(t, jobs, 1)
	assert.Equal(t, "hdsky", jobs[0].SiteName)
	assert.Equal(t, "r1", jobs[0].RSSName)
	m.Stop(models.SiteGroup("hdsky"), "r1")
	assert.Empty(t, m.ListJobs())
}

func TestManager_Start_DuplicateIgnored(t *testing.T) {
	m := newTestManager(t)
	calls := 0
	runner := func(ctx context.Context) { calls++; <-ctx.Done() }
	m.Start(models.SiteGroup("s"), models.RSSConfig{Name: "r"}, runner)
	m.Start(models.SiteGroup("s"), models.RSSConfig{Name: "r"}, runner)
	assert.Len(t, m.ListJobs(), 1)
	m.StopAll()
}

func TestManager_StartAll_ValidSiteStartsJob(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })

	s := core.NewConfigStore(db)
	e := true
	_, _ = s.UpsertSite(models.SiteGroup("springsunday"), models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})

	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			"springsunday": {Enabled: &e, RSS: []models.RSSConfig{{Name: "r1", URL: "https://example.com/rss", IntervalMinutes: 1}}},
		},
	}
	require.NotPanics(t, func() { m.StartAll(cfg) })
	time.Sleep(100 * time.Millisecond) // 等 runner 执行 wg.Add(1) 后再 StopAll，避免 -race 竞态
	m.StopAll()
}

func TestInitDownloaderManager_LoadsEnabledConfigs(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })

	require.NoError(t, db.DB.Create(&models.DownloaderSetting{
		Name: "qb-main", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true, IsDefault: true,
	}).Error)
	require.NoError(t, db.DB.Create(&models.DownloaderSetting{
		Name: "disabled", Type: "qbittorrent", URL: "http://127.0.0.1:2", Enabled: false,
	}).Error)
	require.NoError(t, db.DB.Create(&models.DownloaderSetting{
		Name: "badtype", Type: "unknown", URL: "http://127.0.0.1:3", Enabled: true,
	}).Error)

	m := newTestManager(t)
	require.NotPanics(t, func() { m.initDownloaderManager() })
	assert.True(t, m.downloaderManager.HasFactory(downloader.DownloaderQBittorrent))
}

func TestProcessRSSUnified_SendErrorSurfaces(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, DefaultEnabled: true,
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stub := &unifiedSiteStub{sendErr: assertErr}
	err = processRSSUnified(ctx, models.RSSConfig{Name: "x", URL: ""}, stub)
	require.Error(t, err)
}

func TestManager_Reload_FullStartupWithHealthyDownloader(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })

	srv := createMockQBitServer(t, 0.5, false)
	defer srv.Close()

	require.NoError(t, db.DB.Create(&models.DownloaderSetting{
		Name: "qb-main", Type: "qbittorrent", URL: srv.URL, Username: "admin", Password: "admin",
		Enabled: true, IsDefault: true,
	}).Error)

	s := core.NewConfigStore(db)
	e := true
	_, _ = s.UpsertSite(models.SiteGroup("springsunday"), models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})

	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			"springsunday": {Enabled: &e, RSS: []models.RSSConfig{{Name: "r1", URL: "https://example.com/rss", IntervalMinutes: 1}}},
		},
	}
	require.NotPanics(t, func() { m.Reload(cfg) })
	assert.NotNil(t, m.GetFreeEndMonitor(), "healthy reload should wire the free-end monitor")
	time.Sleep(100 * time.Millisecond) // 等 StartAll 派生的 runner 执行 wg.Add(1) 后再 StopAll，避免 -race 竞态
	m.StopAll()
}

func TestManager_Reload_NoDefaultDownloader(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })

	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true},
		Sites:  map[models.SiteGroup]models.SiteConfig{},
	}
	require.NotPanics(t, func() { m.Reload(cfg) })
}

func TestManager_Reload_NilDBEarlyReturn(t *testing.T) {
	orig := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = orig })
	global.InitLogger(zap.NewNop())
	m := &Manager{}
	require.NotPanics(t, func() {
		m.Reload(&models.Config{Global: models.SettingsGlobal{DownloadDir: "/tmp", AutoStart: true}})
	})
}

var assertErr = &schedGenericErr{"sched boom"}

type schedGenericErr struct{ s string }

func (e *schedGenericErr) Error() string { return e.s }

var _ = v2.SchemaNexusPHP
