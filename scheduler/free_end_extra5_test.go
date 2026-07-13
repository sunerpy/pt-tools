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

func TestUpdateAllMonitoredProgress_MarksCompleted(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.torrentByID["task-mc"] = downloader.Torrent{
		ID: "task-mc", Progress: 1.0, State: downloader.TorrentSeeding, TotalSize: 100,
	}
	m, db := newFreeEndMonitorWithFake(t, fake)

	future := time.Now().Add(time.Hour)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "mc", Title: "MC", PauseOnFreeEnd: true,
		FreeEndTime: &future, DownloaderTaskID: "task-mc", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.updateAllMonitoredProgress()

	var got models.TorrentInfo
	require.NoError(t, db.DB.First(&got, tor.ID).Error)
	assert.True(t, got.IsCompleted)
}

func TestUpdateAllMonitoredProgress_GetTorrentGenericError(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.getErr = errSendBoom
	m, db := newFreeEndMonitorWithFake(t, fake)

	future := time.Now().Add(time.Hour)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "ge", Title: "GE", PauseOnFreeEnd: true,
		FreeEndTime: &future, DownloaderTaskID: "task-ge", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	require.NotPanics(t, func() { m.updateAllMonitoredProgress() })
}

func TestUpdateAllPushedTasksProgress_GetTorrentGenericError(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.getErr = errSendBoom
	m, db := newFreeEndMonitorWithFake(t, fake)

	pushed := true
	ti := models.TorrentInfo{
		SiteName: "s", TorrentID: "pge", Title: "PGE", IsPushed: &pushed,
		DownloaderTaskID: "task-pge", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&ti).Error)
	require.NotPanics(t, func() { m.updateAllPushedTasksProgress() })
}

func TestRescheduleMissingFutureTorrents_SkipsAlreadyPending(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)

	future := time.Now().Add(2 * time.Hour)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "rsp", Title: "RSP", PauseOnFreeEnd: true,
		FreeEndTime: &future, DownloaderTaskID: "task-rsp", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	// Schedule once so it's already pending; a second reconcile must skip it.
	m.rescheduleMissingFutureTorrents(time.Now())
	m.rescheduleMissingFutureTorrents(time.Now())
	m.mu.Lock()
	_, exists := m.pendingTasks[tor.ID]
	m.mu.Unlock()
	assert.True(t, exists)
	m.CancelTorrent(tor.ID)
}
