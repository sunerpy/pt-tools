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

func TestScheduleTorrent_AlreadyPendingIgnored(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)

	future := time.Now().Add(2 * time.Hour)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "sp", Title: "SP", PauseOnFreeEnd: true,
		FreeEndTime: &future, DownloaderTaskID: "task-sp", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.ScheduleTorrent(tor)
	// Second call hits the already-pending early return.
	m.ScheduleTorrent(tor)
	m.mu.Lock()
	_, exists := m.pendingTasks[tor.ID]
	m.mu.Unlock()
	assert.True(t, exists)
	m.CancelTorrent(tor.ID)
}

func TestUpdateAllPushedTasksProgress_DownloaderResolveFails(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)

	pushed := true
	// DownloaderName points at a name the manager cannot resolve; getDownloader
	// falls through to default (fake) but with a mismatched cache key path.
	ti := models.TorrentInfo{
		SiteName: "s", TorrentID: "rf", Title: "RF", IsPushed: &pushed,
		DownloaderTaskID: "missing-task", DownloaderName: "unknown-dl",
	}
	require.NoError(t, db.DB.Create(&ti).Error)
	// fake has no such torrent id → ErrTorrentNotFound → markRemovedFromDownloader.
	require.NotPanics(t, func() { m.updateAllPushedTasksProgress() })

	var got models.TorrentInfo
	require.NoError(t, db.DB.First(&got, ti.ID).Error)
	assert.True(t, got.IsCompleted)
}

func TestUpdateAllPushedTasksProgress_ProgressUpdate(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.torrentByID["task-pp"] = downloader.Torrent{
		ID: "task-pp", Progress: 0.7, State: downloader.TorrentDownloading, TotalSize: 100,
	}
	m, db := newFreeEndMonitorWithFake(t, fake)

	pushed := true
	ti := models.TorrentInfo{
		SiteName: "s", TorrentID: "pp", Title: "PP", IsPushed: &pushed,
		DownloaderTaskID: "task-pp", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&ti).Error)
	m.updateAllPushedTasksProgress()

	var got models.TorrentInfo
	require.NoError(t, db.DB.First(&got, ti.ID).Error)
	assert.InDelta(t, 70.0, got.Progress, 0.5)
}
