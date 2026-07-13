package scheduler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// === loadPendingTasksFromDB: past-due torrent fires handleFreeEndedTorrent ===

func TestLoadPendingTasksFromDB_PastDueFires(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	// Torrent seeding at 100% → handleFreeEndedTorrent marks completed.
	fake.torrentByID["task-past"] = downloader.Torrent{
		ID: "task-past", Progress: 1.0, State: downloader.TorrentSeeding, TotalSize: 10,
	}
	m, db := newFreeEndMonitorWithFake(t, fake)

	past := time.Now().Add(-time.Hour)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "past", Title: "Past", PauseOnFreeEnd: true,
		FreeEndTime: &past, DownloaderTaskID: "task-past", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	require.NoError(t, m.loadPendingTasksFromDB())

	// The past-due branch spawns a goroutine calling handleFreeEndedTorrent;
	// wait for the DB to reflect completion.
	deadline := time.Now().Add(2 * time.Second)
	var updated models.TorrentInfo
	for time.Now().Before(deadline) {
		require.NoError(t, db.DB.First(&updated, tor.ID).Error)
		if updated.IsCompleted {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	assert.True(t, updated.IsCompleted, "past-due torrent should be processed to completion")
}

// === archiveOldTorrents: moves aged completed rows into archive table ===

func TestArchiveOldTorrents_MovesAged(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)

	old := time.Now().AddDate(0, 0, -archiveRetentionDays-1)
	pushed := true
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "arch1", Title: "Aged", IsCompleted: true,
		IsPushed: &pushed, DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)
	// Force created_at into the past (GORM sets it to now on create).
	require.NoError(t, db.DB.Model(&models.TorrentInfo{}).Where("id = ?", tor.ID).
		Update("created_at", old).Error)

	m.archiveOldTorrents()

	var liveCount int64
	db.DB.Model(&models.TorrentInfo{}).Where("torrent_id = ?", "arch1").Count(&liveCount)
	assert.Equal(t, int64(0), liveCount, "aged row removed from live table")

	var archCount int64
	db.DB.Model(&models.TorrentInfoArchive{}).Where("torrent_id = ?", "arch1").Count(&archCount)
	assert.Equal(t, int64(1), archCount, "aged row moved to archive")
}

// === archiveOldTorrents: nothing to archive → no-op ===

func TestArchiveOldTorrents_NothingToArchive(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, _ := newFreeEndMonitorWithFake(t, fake)
	require.NotPanics(t, func() { m.archiveOldTorrents() })
}

// === updateAllPushedTasksProgress: marks completed when downloader reports done ===

func TestUpdateAllPushed_MarksCompleted(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.torrentByID["task-done"] = downloader.Torrent{
		ID: "task-done", Progress: 1.0, State: downloader.TorrentSeeding, TotalSize: 555,
	}
	m, db := newFreeEndMonitorWithFake(t, fake)

	pushed := true
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "up1", Title: "Done", IsPushed: &pushed,
		DownloaderTaskID: "task-done", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.updateAllPushedTasksProgress()

	var updated models.TorrentInfo
	require.NoError(t, db.DB.First(&updated, tor.ID).Error)
	assert.True(t, updated.IsCompleted)
	assert.Greater(t, updated.CheckCount, 0)
}

// === updateAllMonitoredProgress: marks completed on seeding 100% ===

func TestUpdateAllMonitored_MarksCompleted(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.torrentByID["task-mon"] = downloader.Torrent{
		ID: "task-mon", Progress: 1.0, State: downloader.TorrentSeeding, TotalSize: 100,
	}
	m, db := newFreeEndMonitorWithFake(t, fake)

	future := time.Now().Add(time.Hour)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "mon1", Title: "MonDone", PauseOnFreeEnd: true,
		FreeEndTime: &future, DownloaderTaskID: "task-mon", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.updateAllMonitoredProgress()

	var updated models.TorrentInfo
	require.NoError(t, db.DB.First(&updated, tor.ID).Error)
	assert.True(t, updated.IsCompleted)
}

// === scheduleTaskLockedWithAdvance: past deadline → delay clamped to 0, fires ===

func TestScheduleTaskLocked_PastDeadlineFires(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.torrentByID["task-imm"] = downloader.Torrent{
		ID: "task-imm", Progress: 1.0, State: downloader.TorrentSeeding, TotalSize: 10,
	}
	m, db := newFreeEndMonitorWithFake(t, fake)

	past := time.Now().Add(-time.Hour)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "imm", Title: "Imm", PauseOnFreeEnd: true,
		FreeEndTime: &past, DownloaderTaskID: "task-imm", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.mu.Lock()
	m.scheduleTaskLockedWithAdvance(tor, 0)
	m.mu.Unlock()

	deadline := time.Now().Add(2 * time.Second)
	var updated models.TorrentInfo
	for time.Now().Before(deadline) {
		require.NoError(t, db.DB.First(&updated, tor.ID).Error)
		if updated.IsCompleted {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	assert.True(t, updated.IsCompleted, "past-deadline timer should fire ~immediately")
}
