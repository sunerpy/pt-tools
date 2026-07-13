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
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

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

func TestMarkRemovedFromDownloader_ClearsTask(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fm, db := newFreeEndMonitorWithFake(t, fake)
	future := time.Now().Add(time.Hour)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "rm", Title: "T", PauseOnFreeEnd: true,
		FreeEndTime: &future, DownloaderTaskID: "task-rm", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)
	fm.markRemovedFromDownloader(tor)
	var got models.TorrentInfo
	require.NoError(t, db.DB.First(&got, tor.ID).Error)
	assert.True(t, got.IsCompleted)
	assert.Equal(t, "", got.DownloaderTaskID)
}

func TestMarkAutoDeleted_NonAdvancedReason(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fm, db := newFreeEndMonitorWithFake(t, fake)
	tor := models.TorrentInfo{SiteName: "s", TorrentID: "ad", Title: "T"}
	require.NoError(t, db.DB.Create(&tor).Error)
	fm.markAutoDeleted(tor, 20.0, 100, false)
	var got models.TorrentInfo
	require.NoError(t, db.DB.First(&got, tor.ID).Error)
	assert.Equal(t, "免费期结束，自动删除（未完成）", got.PauseReason)
}

func TestIsAutoDeleteEnabled_TrueFalse(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fm, db := newFreeEndMonitorWithFake(t, fake)
	assert.False(t, fm.isAutoDeleteEnabled())
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, AutoDeleteOnFreeEnd: true,
	}))
	assert.True(t, fm.isAutoDeleteEnabled())
}

func TestPeerRatio_RunOnce_ProcessesHealthy(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	pm, db := newPeerRatioMonitorWithFake(t, fake)

	hash := "prro"
	tor := completedManagedTorrent(t, db, "prro1", hash, "qb1")
	fake.torrents = []downloader.Torrent{tor}
	fake.trackers[tor.ID] = []downloader.TorrentTracker{{Status: 2, Seeds: 100, Leeches: 1}}

	pm.runOnce(&models.SettingsGlobal{PeerRatioMaxSL: 5}, 5.0)
	assert.Equal(t, []string{"prro1"}, fake.pausedIDs)
}

func TestPeerRatio_ProcessDownloader_CtxCancelled(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	pm, db := newPeerRatioMonitorWithFake(t, fake)
	tor := completedManagedTorrent(t, db, "prc", "prchash", "qb1")
	fake.torrents = []downloader.Torrent{tor}
	pm.cancel()
	require.NotPanics(t, func() { pm.processDownloader(fake, "qb1", 5.0, false) })
}

var _ = context.Background
