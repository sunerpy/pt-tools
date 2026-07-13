package scheduler

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func newFreeEndMonitorWithFake(t *testing.T, fake *schedFakeDownloader) (*FreeEndMonitor, *models.TorrentDB) {
	t.Helper()
	db := setupTestDB(t)
	dm := downloader.NewDownloaderManager()
	registerFakeDownloader(t, dm, fake, true)
	_, err := dm.GetDownloader(fake.name)
	require.NoError(t, err)
	return NewFreeEndMonitor(db.DB, dm), db
}

// === handleFreeEndedTorrent: auto-delete path ===

func TestHandleFreeEnded_AutoDelete(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)

	// Enable auto-delete.
	require.NoError(t, db.DB.Create(&models.SettingsGlobal{
		DownloadDir: t.TempDir(), AutoDeleteOnFreeEnd: true,
	}).Error)

	// Incomplete torrent in downloader.
	fake.torrentByID["task-del"] = downloader.Torrent{
		ID: "task-del", Progress: 0.4, State: downloader.TorrentDownloading, TotalSize: 500,
	}

	freeEnd := time.Now().Add(-time.Minute)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "ad1", Title: "AutoDel", PauseOnFreeEnd: true,
		FreeEndTime: &freeEnd, DownloaderTaskID: "task-del", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.handleFreeEndedTorrent(tor)

	require.Len(t, fake.removedSingle, 1)
	assert.Equal(t, "task-del", fake.removedSingle[0])

	var updated models.TorrentInfo
	require.NoError(t, db.DB.First(&updated, tor.ID).Error)
	assert.True(t, updated.IsPausedBySystem)
	assert.Equal(t, "免费期结束，自动删除（未完成）", updated.PauseReason)
	assert.Empty(t, updated.DownloaderTaskID, "auto-deleted clears downloader_task_id")
}

// === handleFreeEndedTorrent: auto-delete tolerates ErrTorrentNotFound ===

func TestHandleFreeEnded_AutoDelete_NotFoundTolerated(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)

	require.NoError(t, db.DB.Create(&models.SettingsGlobal{
		DownloadDir: t.TempDir(), AutoDeleteOnFreeEnd: true,
	}).Error)

	fake.torrentByID["task-nf"] = downloader.Torrent{
		ID: "task-nf", Progress: 0.3, State: downloader.TorrentDownloading,
	}
	fake.removeErr = downloader.ErrTorrentNotFound

	freeEnd := time.Now().Add(-time.Minute)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "ad2", Title: "AutoDelNF", PauseOnFreeEnd: true,
		FreeEndTime: &freeEnd, DownloaderTaskID: "task-nf", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.handleFreeEndedTorrent(tor)

	var updated models.TorrentInfo
	require.NoError(t, db.DB.First(&updated, tor.ID).Error)
	assert.True(t, updated.IsPausedBySystem, "ErrTorrentNotFound still marks auto-deleted")
}

// === handleFreeEndedTorrent: auto-delete real error → markRetry ===

func TestHandleFreeEnded_AutoDelete_RealError_Retries(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)

	require.NoError(t, db.DB.Create(&models.SettingsGlobal{
		DownloadDir: t.TempDir(), AutoDeleteOnFreeEnd: true,
	}).Error)

	fake.torrentByID["task-err"] = downloader.Torrent{
		ID: "task-err", Progress: 0.3, State: downloader.TorrentDownloading,
	}
	fake.removeErr = errors.New("delete boom")

	freeEnd := time.Now().Add(-time.Minute)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "ad3", Title: "AutoDelErr", PauseOnFreeEnd: true,
		FreeEndTime: &freeEnd, DownloaderTaskID: "task-err", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.handleFreeEndedTorrent(tor)

	var updated models.TorrentInfo
	require.NoError(t, db.DB.First(&updated, tor.ID).Error)
	assert.False(t, updated.IsPausedBySystem)
	assert.Greater(t, updated.RetryCount, 0, "delete error triggers retry")
}

// === handleFreeEndedTorrent: getDownloader failure → markRetry ===

func TestHandleFreeEnded_GetDownloaderError_Retries(t *testing.T) {
	db := setupTestDB(t)
	// no downloader registered → getDownloader returns error
	m := NewFreeEndMonitor(db.DB, downloader.NewDownloaderManager())

	freeEnd := time.Now().Add(-time.Minute)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "gd1", Title: "NoDL", PauseOnFreeEnd: true,
		FreeEndTime: &freeEnd, DownloaderTaskID: "task-x", DownloaderName: "missing",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.handleFreeEndedTorrent(tor)

	var updated models.TorrentInfo
	require.NoError(t, db.DB.First(&updated, tor.ID).Error)
	assert.Greater(t, updated.RetryCount, 0)
	assert.Contains(t, updated.LastError, "获取下载器失败")
}

// === handleFreeEndedTorrent: checkCompletion error → markRetry ===

func TestHandleFreeEnded_CheckCompletionError_Retries(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.getErr = errors.New("get torrent boom")
	m, db := newFreeEndMonitorWithFake(t, fake)

	freeEnd := time.Now().Add(-time.Minute)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "cc1", Title: "CheckErr", PauseOnFreeEnd: true,
		FreeEndTime: &freeEnd, DownloaderTaskID: "task-cc", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.handleFreeEndedTorrent(tor)

	var updated models.TorrentInfo
	require.NoError(t, db.DB.First(&updated, tor.ID).Error)
	assert.Greater(t, updated.RetryCount, 0)
	assert.Contains(t, updated.LastError, "检查完成状态失败")
}

// === handleFreeEndedTorrent: pause error → markRetry ===

func TestHandleFreeEnded_PauseError_Retries(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.pauseErr = errors.New("pause boom")
	m, db := newFreeEndMonitorWithFake(t, fake)

	fake.torrentByID["task-pe"] = downloader.Torrent{
		ID: "task-pe", Progress: 0.5, State: downloader.TorrentDownloading,
	}

	freeEnd := time.Now().Add(-time.Minute)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "pe1", Title: "PauseErr", PauseOnFreeEnd: true,
		FreeEndTime: &freeEnd, DownloaderTaskID: "task-pe", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.handleFreeEndedTorrent(tor)

	var updated models.TorrentInfo
	require.NoError(t, db.DB.First(&updated, tor.ID).Error)
	assert.False(t, updated.IsPausedBySystem)
	assert.Greater(t, updated.RetryCount, 0)
}

// === handleFreeEndedTorrent: already processed (lock lost) → skip ===

func TestHandleFreeEnded_AlreadyProcessed_Skips(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)

	freeEnd := time.Now().Add(-time.Minute)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "ap1", Title: "Already", PauseOnFreeEnd: true,
		FreeEndTime: &freeEnd, DownloaderTaskID: "task-ap", DownloaderName: "qb1",
		IsPausedBySystem: true, // already paused → processing lock RowsAffected==0
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.handleFreeEndedTorrent(tor)
	// Nothing to assert beyond no panic and no downloader interaction.
	assert.Empty(t, fake.pausedIDs)
	assert.Empty(t, fake.removedSingle)
}

// === TestHandleFreeEndedTorrent exported wrapper ===

func TestExportedTestHandleFreeEndedTorrent(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)

	fake.torrentByID["task-w"] = downloader.Torrent{
		ID: "task-w", Progress: 1.0, State: downloader.TorrentSeeding, TotalSize: 100,
	}
	freeEnd := time.Now().Add(-time.Minute)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "w1", Title: "Wrapper", PauseOnFreeEnd: true,
		FreeEndTime: &freeEnd, DownloaderTaskID: "task-w", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	require.NotPanics(t, func() { m.TestHandleFreeEndedTorrent(tor) })

	var updated models.TorrentInfo
	require.NoError(t, db.DB.First(&updated, tor.ID).Error)
	assert.True(t, updated.IsCompleted)
}

// === markRemovedFromDownloader via updateAllPushedTasksProgress ErrTorrentNotFound ===

func TestUpdateAllPushed_MarksRemovedWhenNotFound(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.getErr = downloader.ErrTorrentNotFound
	m, db := newFreeEndMonitorWithFake(t, fake)

	pushed := true
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "rm1", Title: "Removed", IsPushed: &pushed,
		DownloaderTaskID: "task-gone", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.updateAllPushedTasksProgress()

	var updated models.TorrentInfo
	require.NoError(t, db.DB.First(&updated, tor.ID).Error)
	assert.True(t, updated.IsCompleted, "removed-from-downloader marks completed")
	assert.Empty(t, updated.DownloaderTaskID)
	assert.Equal(t, "种子已从下载器中删除", updated.LastError)
}

// === updateAllPushedTasksProgress: skips rows missing downloader name ===

func TestUpdateAllPushed_SkipsMissingDownloaderName(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)

	pushed := true
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "sk1", Title: "NoName", IsPushed: &pushed,
		DownloaderTaskID: "task-x", DownloaderName: "",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	require.NotPanics(t, func() { m.updateAllPushedTasksProgress() })

	var updated models.TorrentInfo
	require.NoError(t, db.DB.First(&updated, tor.ID).Error)
	assert.Equal(t, 0, updated.CheckCount, "row without downloader name is skipped")
}

// === updateAllMonitoredProgress: marks removed when not found ===

func TestUpdateAllMonitored_MarksRemovedWhenNotFound(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.getErr = downloader.ErrTorrentNotFound
	m, db := newFreeEndMonitorWithFake(t, fake)

	freeEnd := time.Now().Add(time.Hour)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "mm1", Title: "MonRemoved", PauseOnFreeEnd: true,
		FreeEndTime: &freeEnd, DownloaderTaskID: "task-gone", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.updateAllMonitoredProgress()

	var updated models.TorrentInfo
	require.NoError(t, db.DB.First(&updated, tor.ID).Error)
	assert.True(t, updated.IsCompleted)
}

// === getDownloader: falls back to DownloaderID lookup ===

func TestGetDownloader_FallbackToDownloaderID(t *testing.T) {
	fake := newSchedFakeDownloader("qb-byid")
	m, db := newFreeEndMonitorWithFake(t, fake)

	dl := models.DownloaderSetting{Name: "qb-byid", Type: "qbittorrent", Enabled: true}
	require.NoError(t, db.DB.Create(&dl).Error)

	// DownloaderName empty → falls through to DownloaderID branch.
	got, err := m.getDownloader(models.TorrentInfo{DownloaderID: &dl.ID})
	require.NoError(t, err)
	assert.Equal(t, "qb-byid", got.GetName())
}

// === getDownloader: falls back to default ===

func TestGetDownloader_FallbackToDefault(t *testing.T) {
	fake := newSchedFakeDownloader("qb-default")
	m, _ := newFreeEndMonitorWithFake(t, fake)

	got, err := m.getDownloader(models.TorrentInfo{})
	require.NoError(t, err)
	assert.Equal(t, "qb-default", got.GetName())
}

// === markRemovedFromDownloader directly ===

func TestMarkRemovedFromDownloader(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)

	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "d1", Title: "Del", DownloaderTaskID: "task",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.markRemovedFromDownloader(tor)

	var updated models.TorrentInfo
	require.NoError(t, db.DB.First(&updated, tor.ID).Error)
	assert.True(t, updated.IsCompleted)
	assert.Empty(t, updated.DownloaderTaskID)
}

// === markAutoDeleted: advanced reason variant ===

func TestMarkAutoDeleted_AdvancedReason(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)

	tor := models.TorrentInfo{SiteName: "s", TorrentID: "adv1", Title: "Adv"}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.markAutoDeleted(tor, 33.3, 999, true)

	var updated models.TorrentInfo
	require.NoError(t, db.DB.First(&updated, tor.ID).Error)
	assert.Equal(t, "免费期临近结束，自动删除（未完成）", updated.PauseReason)
	assert.True(t, updated.IsPausedBySystem)
}

// === isAutoDeleteEnabled ===

func TestIsAutoDeleteEnabled(t *testing.T) {
	db := setupTestDB(t)
	m := NewFreeEndMonitor(db.DB, downloader.NewDownloaderManager())
	assert.False(t, m.isAutoDeleteEnabled(), "no config row → false")

	require.NoError(t, db.DB.Create(&models.SettingsGlobal{
		DownloadDir: t.TempDir(), AutoDeleteOnFreeEnd: true,
	}).Error)
	assert.True(t, m.isAutoDeleteEnabled())
}

// === loadPendingTasksFromDB: schedules future, fires past ===

func TestLoadPendingTasksFromDB_FuturePastSplit(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.torrentByID["task-future"] = downloader.Torrent{
		ID: "task-future", Progress: 0.5, State: downloader.TorrentDownloading,
	}
	m, db := newFreeEndMonitorWithFake(t, fake)

	future := time.Now().Add(time.Hour)
	futureTor := models.TorrentInfo{
		SiteName: "s", TorrentID: "fut", Title: "Future", PauseOnFreeEnd: true,
		FreeEndTime: &future, DownloaderTaskID: "task-future", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&futureTor).Error)

	require.NoError(t, m.loadPendingTasksFromDB())

	m.mu.Lock()
	_, scheduled := m.pendingTasks[futureTor.ID]
	m.mu.Unlock()
	assert.True(t, scheduled, "future torrent should be scheduled in pendingTasks")

	// clean up timers
	m.CancelTorrent(futureTor.ID)
}
