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

func TestUpdateAllPushedTasksProgress_MarksCompletedFull(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.torrentByID["task-done"] = downloader.Torrent{
		ID: "task-done", Progress: 1.0, State: downloader.TorrentSeeding, TotalSize: 100,
	}
	m, db := newFreeEndMonitorWithFake(t, fake)

	pushed := true
	ti := models.TorrentInfo{
		SiteName: "s", TorrentID: "d1", Title: "Done", IsPushed: &pushed,
		DownloaderTaskID: "task-done", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&ti).Error)

	m.updateAllPushedTasksProgress()

	var got models.TorrentInfo
	require.NoError(t, db.DB.First(&got, ti.ID).Error)
	assert.True(t, got.IsCompleted)
	assert.InDelta(t, 100.0, got.Progress, 0.5)
}

func TestUpdateAllPushedTasksProgress_ProgressOnlyUpdate(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.torrentByID["task-part"] = downloader.Torrent{
		ID: "task-part", Progress: 0.4, State: downloader.TorrentDownloading, TotalSize: 100,
	}
	m, db := newFreeEndMonitorWithFake(t, fake)

	pushed := true
	ti := models.TorrentInfo{
		SiteName: "s", TorrentID: "d2", Title: "Part", IsPushed: &pushed,
		DownloaderTaskID: "task-part", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&ti).Error)

	m.updateAllPushedTasksProgress()

	var got models.TorrentInfo
	require.NoError(t, db.DB.First(&got, ti.ID).Error)
	assert.False(t, got.IsCompleted)
	assert.InDelta(t, 40.0, got.Progress, 0.5)
	assert.Equal(t, 1, got.CheckCount)
}

func TestArchiveOldTorrents_ArchivesPausedAged(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)

	old := time.Now().AddDate(0, 0, -40)
	pushed := true
	ti := models.TorrentInfo{
		SiteName: "s", TorrentID: "old1", Title: "OldPaused",
		IsPausedBySystem: true, IsPushed: &pushed, CreatedAt: old,
	}
	require.NoError(t, db.DB.Create(&ti).Error)

	m.archiveOldTorrents()

	var remaining int64
	require.NoError(t, db.DB.Model(&models.TorrentInfo{}).Where("torrent_id = ?", "old1").Count(&remaining).Error)
	assert.EqualValues(t, 0, remaining)

	var archived int64
	require.NoError(t, db.DB.Model(&models.TorrentInfoArchive{}).Where("torrent_id = ?", "old1").Count(&archived).Error)
	assert.EqualValues(t, 1, archived)
}
