// MIT License
// Copyright (c) 2025 pt-tools

package scheduler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func TestHandleFreeEnded_PauseSuccessMarksPaused(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.torrentByID["task-ps"] = downloader.Torrent{
		ID: "task-ps", Progress: 0.5, State: downloader.TorrentDownloading, TotalSize: 100,
	}
	m, db := newFreeEndMonitorWithFake(t, fake)

	future := time.Now().Add(time.Hour)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "ps1", Title: "PS", PauseOnFreeEnd: true,
		FreeEndTime: &future, DownloaderTaskID: "task-ps", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.handleFreeEndedTorrent(tor)

	var got models.TorrentInfo
	require.NoError(t, db.DB.First(&got, tor.ID).Error)
	assert.True(t, got.IsPausedBySystem)
	assert.Contains(t, fake.pausedIDs, "task-ps")
}

func TestGetDownloader_FallbackToDownloaderIDLookup(t *testing.T) {
	fake := newSchedFakeDownloader("qb-bound")
	m, db := newFreeEndMonitorWithFake(t, fake)

	ds := models.DownloaderSetting{Name: "qb-bound", Type: "qbittorrent", URL: "http://x", Enabled: true}
	require.NoError(t, db.DB.Create(&ds).Error)

	// DownloaderName empty → falls to DownloaderID lookup → resolves qb-bound.
	tor := models.TorrentInfo{SiteName: "s", TorrentID: "gd", DownloaderID: &ds.ID}
	dl, err := m.getDownloader(tor)
	require.NoError(t, err)
	assert.Equal(t, "qb-bound", dl.GetName())
}

func TestGetDownloader_NilManager(t *testing.T) {
	m := &FreeEndMonitor{}
	_, err := m.getDownloader(models.TorrentInfo{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "下载器管理器未初始化")
}

func TestUpdateAllPushedTasksProgress_SkipsMissingDownloaderName(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)
	pushed := true
	ti := models.TorrentInfo{
		SiteName: "s", TorrentID: "nn", Title: "NoName", IsPushed: &pushed,
		DownloaderTaskID: "task-x", DownloaderName: "",
	}
	require.NoError(t, db.DB.Create(&ti).Error)
	require.NotPanics(t, func() { m.updateAllPushedTasksProgress() })
}

func TestCheckTorrentCompletion_Error(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.getErr = errSendBoom
	m, _ := newFreeEndMonitorWithFake(t, fake)
	_, _, _, err := m.checkTorrentCompletion(nil, fake, "missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "获取种子信息")
}
