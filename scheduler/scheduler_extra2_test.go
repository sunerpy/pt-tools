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
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

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

func TestParseCron_FieldOutOfRangeErrors(t *testing.T) {
	cases := []string{
		"0 0 40 * *", // dom > 31
		"0 0 * 13 *", // month > 12
		"0 0 * * 9",  // dow > 6
	}
	for _, spec := range cases {
		_, err := ParseCron(spec)
		require.Error(t, err, "spec %q should error", spec)
	}
}

func TestCronWindowStart_NoMatchReturnsZero(t *testing.T) {
	// Feb 30 never exists → WindowStart scans a week and returns zero.
	c, err := ParseCron("0 0 30 2 *")
	require.NoError(t, err)
	got := c.WindowStart(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	assert.True(t, got.IsZero())
}

func TestPauseTorrentWithRetry_CtxCancelledMidway(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.pauseErr = assertErr
	m, _ := newFreeEndMonitorWithFake(t, fake)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tor := models.TorrentInfo{Title: "T", DownloaderTaskID: "x"}
	err := m.pauseTorrentWithRetry(ctx, fake, tor)
	require.Error(t, err)
}

func TestMarkPaused_AdvancedReason(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)
	tor := models.TorrentInfo{SiteName: "s", TorrentID: "mp-adv", Title: "T"}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.markPaused(tor, 40.0, 100, true)
	var got models.TorrentInfo
	require.NoError(t, db.DB.First(&got, tor.ID).Error)
	assert.Equal(t, "免费期临近结束，已暂停（未完成）", got.PauseReason)
}

func TestMarkRetry_SchedulesWhenUnderMax(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.torrentByID["task-r"] = downloader.Torrent{ID: "task-r", Progress: 0.5, State: downloader.TorrentDownloading}
	m, db := newFreeEndMonitorWithFake(t, fake)

	future := time.Now().Add(time.Hour)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "retry-sched", Title: "T", PauseOnFreeEnd: true,
		FreeEndTime: &future, DownloaderTaskID: "task-r", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.markRetry(tor, "boom")
	var got models.TorrentInfo
	require.NoError(t, db.DB.First(&got, tor.ID).Error)
	assert.Equal(t, 1, got.RetryCount)
	m.mu.Lock()
	_, scheduled := m.pendingTasks[tor.ID]
	m.mu.Unlock()
	assert.True(t, scheduled, "under-max retry should reschedule a task")
	m.CancelTorrent(tor.ID)
}

func TestGetHRInfoMap_FromDefinitionAndDB(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)

	// Explicit HR from DB row.
	h1 := "aaaa1111"
	require.NoError(t, cm.db.Create(&models.TorrentInfo{
		SiteName: "s", TorrentID: "t1", TorrentHash: &h1, HasHR: true, HRSeedTimeH: 96,
	}).Error)
	// A row for an HR-enabled site definition (agsvpt) with HasHR=false → derived.
	h2 := "bbbb2222"
	require.NoError(t, cm.db.Create(&models.TorrentInfo{
		SiteName: "agsvpt", TorrentID: "t2", TorrentHash: &h2, HasHR: false, TorrentSize: 10 << 30,
	}).Error)

	m := cm.getHRInfoMap()
	require.Contains(t, m, "aaaa1111")
	assert.Equal(t, 96, m["aaaa1111"].HRSeedTimeH)
	if _, ok := m["bbbb2222"]; ok {
		assert.True(t, m["bbbb2222"].HasHR)
	}
}

func TestArchiveOldTorrents_MovesCompletedAged(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)

	old := time.Now().AddDate(0, 0, -40)
	require.NoError(t, db.DB.Create(&models.TorrentInfo{
		SiteName: "s", TorrentID: "aged", Title: "Aged", IsCompleted: true, CreatedAt: old,
	}).Error)

	m.archiveOldTorrents()

	var remaining int64
	require.NoError(t, db.DB.Model(&models.TorrentInfo{}).Where("torrent_id = ?", "aged").Count(&remaining).Error)
	assert.EqualValues(t, 0, remaining, "aged completed row should be archived away from active table")
}

func TestCheckAndProcessExpiredTorrents_FiresPast(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.torrentByID["task-exp"] = downloader.Torrent{ID: "task-exp", Progress: 1.0, State: downloader.TorrentSeeding, TotalSize: 10}
	m, db := newFreeEndMonitorWithFake(t, fake)

	past := time.Now().Add(-time.Hour)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "exp1", Title: "Exp", PauseOnFreeEnd: true,
		FreeEndTime: &past, DownloaderTaskID: "task-exp", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.checkAndProcessExpiredTorrents()
	m.wg.Wait()

	var got models.TorrentInfo
	require.NoError(t, db.DB.First(&got, tor.ID).Error)
	assert.True(t, got.IsCompleted)
}

func TestRescheduleMissingFutureTorrents_SchedulesUnseen(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)

	future := time.Now().Add(2 * time.Hour)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "future1", Title: "Fut", PauseOnFreeEnd: true,
		FreeEndTime: &future, DownloaderTaskID: "task-f", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.rescheduleMissingFutureTorrents(time.Now())
	m.mu.Lock()
	_, scheduled := m.pendingTasks[tor.ID]
	m.mu.Unlock()
	assert.True(t, scheduled)
	m.CancelTorrent(tor.ID)
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
