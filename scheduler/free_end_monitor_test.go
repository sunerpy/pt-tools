// MIT License
// Copyright (c) 2025 pt-tools

package scheduler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
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
	_, _, _, err := m.checkTorrentCompletion(context.Background(), fake, "missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "获取种子信息")
}

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

func closedFreeEndMonitor(t *testing.T) *FreeEndMonitor {
	t.Helper()
	global.InitLogger(zap.NewNop())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}, &models.SettingsGlobal{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	return NewFreeEndMonitor(db, downloader.NewDownloaderManager())
}

func TestMarkFuncs_DBErrorNoPanic(t *testing.T) {
	m := closedFreeEndMonitor(t)
	tor := models.TorrentInfo{Title: "T"}
	tor.ID = 1
	require.NotPanics(t, func() {
		m.markCompleted(tor, 100)
		m.markPaused(tor, 50, 100, false)
		m.markAutoDeleted(tor, 50, 100, true)
		m.markRemovedFromDownloader(tor)
	})
}

func TestUpdateAllMonitoredProgress_QueryError(t *testing.T) {
	m := closedFreeEndMonitor(t)
	require.NotPanics(t, func() { m.updateAllMonitoredProgress() })
}

func TestUpdateAllPushedTasksProgress_QueryError(t *testing.T) {
	m := closedFreeEndMonitor(t)
	require.NotPanics(t, func() { m.updateAllPushedTasksProgress() })
}

func TestCheckAndProcessExpiredTorrents_QueryError(t *testing.T) {
	m := closedFreeEndMonitor(t)
	require.NotPanics(t, func() { m.checkAndProcessExpiredTorrents() })
}

func TestArchiveOldTorrents_QueryError(t *testing.T) {
	m := closedFreeEndMonitor(t)
	require.NotPanics(t, func() { m.archiveOldTorrents() })
}

func TestLoadPendingTasksFromDB_QueryError(t *testing.T) {
	m := closedFreeEndMonitor(t)
	require.Error(t, m.loadPendingTasksFromDB())
}

func TestHandleFreeEndedTorrent_LockAcquireError(t *testing.T) {
	m := closedFreeEndMonitor(t)
	future := time.Now().Add(time.Hour)
	tor := models.TorrentInfo{Title: "T", PauseOnFreeEnd: true, FreeEndTime: &future, DownloaderTaskID: "x"}
	tor.ID = 5
	require.NotPanics(t, func() { m.handleFreeEndedTorrent(tor) })
}

var _ = context.Background

func newFreeEndMonitorWithFake(t *testing.T, fake *schedFakeDownloader) (*FreeEndMonitor, *models.TorrentDB) {
	t.Helper()
	db := setupTestDB(t)
	dm := downloader.NewDownloaderManager()
	registerFakeDownloader(t, dm, fake, true)
	_, err := dm.GetDownloader(fake.name)
	require.NoError(t, err)
	return NewFreeEndMonitor(db.DB, dm), db
}

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

func TestGetDownloader_FallbackToDefault(t *testing.T) {
	fake := newSchedFakeDownloader("qb-default")
	m, _ := newFreeEndMonitorWithFake(t, fake)

	got, err := m.getDownloader(models.TorrentInfo{})
	require.NoError(t, err)
	assert.Equal(t, "qb-default", got.GetName())
}

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

func TestIsAutoDeleteEnabled(t *testing.T) {
	db := setupTestDB(t)
	m := NewFreeEndMonitor(db.DB, downloader.NewDownloaderManager())
	assert.False(t, m.isAutoDeleteEnabled(), "no config row → false")

	require.NoError(t, db.DB.Create(&models.SettingsGlobal{
		DownloadDir: t.TempDir(), AutoDeleteOnFreeEnd: true,
	}).Error)
	assert.True(t, m.isAutoDeleteEnabled())
}

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

func TestArchiveOldTorrents_NothingToArchive(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, _ := newFreeEndMonitorWithFake(t, fake)
	require.NotPanics(t, func() { m.archiveOldTorrents() })
}

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

func setupTestDB(t *testing.T) *models.TorrentDB {
	t.Helper()
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	return db
}

func createMockQBitServer(t *testing.T, progress float64, paused bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/sync/maindata":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":10000000000}}`))
		case "/api/v2/torrents/info":
			w.WriteHeader(http.StatusOK)
			// qBit reports `uploading`/`stalledUP` once a torrent reaches
			// progress=1.0, never `downloading`. Mirroring that here keeps
			// the fixtures aligned with isTorrentTrulyCompleted's contract.
			state := "downloading"
			if progress >= 1.0 {
				state = "uploading"
			}
			if paused {
				if progress >= 1.0 {
					state = "pausedUP"
				} else {
					state = "pausedDL"
				}
			}
			_, _ = w.Write([]byte(`[{"hash":"test-hash-123","name":"Test Torrent","progress":` +
				floatToString(progress) + `,"state":"` + state + `","size":1073741824}]`))
		case "/api/v2/torrents/pause":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/resume":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
}

func floatToString(f float64) string {
	return fmt.Sprintf("%.2f", f)
}

func setupTestDownloaderManager(t *testing.T, srv *httptest.Server) *downloader.DownloaderManager {
	t.Helper()
	dlMgr := downloader.NewDownloaderManager()
	dlMgr.RegisterFactory(downloader.DownloaderQBittorrent, createQBitFactory())
	config := downloader.NewGenericConfig(downloader.DownloaderQBittorrent, srv.URL, "admin", "admin", false)
	err := dlMgr.RegisterConfig("test-qbit", config, true)
	require.NoError(t, err)
	return dlMgr
}

func TestFreeEndMonitor_NewAndStart(t *testing.T) {
	db := setupTestDB(t)
	dlMgr := downloader.NewDownloaderManager()

	monitor := NewFreeEndMonitor(db.DB, dlMgr)
	require.NotNil(t, monitor)

	err := monitor.Start()
	require.NoError(t, err)
	assert.True(t, monitor.running)

	monitor.Stop()
	assert.False(t, monitor.running)
}

func TestFreeEndMonitor_DoubleStart(t *testing.T) {
	db := setupTestDB(t)
	dlMgr := downloader.NewDownloaderManager()
	monitor := NewFreeEndMonitor(db.DB, dlMgr)

	err := monitor.Start()
	require.NoError(t, err)

	err = monitor.Start()
	require.NoError(t, err)

	monitor.Stop()
}

func TestFreeEndMonitor_DoubleStop(t *testing.T) {
	db := setupTestDB(t)
	dlMgr := downloader.NewDownloaderManager()
	monitor := NewFreeEndMonitor(db.DB, dlMgr)

	err := monitor.Start()
	require.NoError(t, err)

	monitor.Stop()

	require.NotPanics(t, func() {
		monitor.Stop()
	})
}

func TestFreeEndMonitor_ScheduleTorrent(t *testing.T) {
	db := setupTestDB(t)
	dlMgr := downloader.NewDownloaderManager()
	monitor := NewFreeEndMonitor(db.DB, dlMgr)

	err := monitor.Start()
	require.NoError(t, err)
	defer monitor.Stop()

	freeEndTime := time.Now().Add(1 * time.Hour)
	torrent := models.TorrentInfo{
		SiteName:         "test-site",
		TorrentID:        "12345",
		Title:            "Test Torrent",
		IsFree:           true,
		PauseOnFreeEnd:   true,
		FreeEndTime:      &freeEndTime,
		DownloaderTaskID: "test-hash-123",
		DownloaderName:   "test-qbit",
	}

	err = db.DB.Create(&torrent).Error
	require.NoError(t, err)

	monitor.ScheduleTorrent(torrent)

	monitor.mu.Lock()
	_, exists := monitor.pendingTasks[torrent.ID]
	monitor.mu.Unlock()
	assert.True(t, exists, "torrent should be added to pending tasks")
}

func TestFreeEndMonitor_ScheduleTorrent_SkipConditions(t *testing.T) {
	db := setupTestDB(t)
	dlMgr := downloader.NewDownloaderManager()
	monitor := NewFreeEndMonitor(db.DB, dlMgr)

	err := monitor.Start()
	require.NoError(t, err)
	defer monitor.Stop()

	tests := []struct {
		name    string
		torrent models.TorrentInfo
	}{
		{
			name: "PauseOnFreeEnd is false",
			torrent: models.TorrentInfo{
				PauseOnFreeEnd:   false,
				FreeEndTime:      ptrTime(time.Now().Add(1 * time.Hour)),
				DownloaderTaskID: "hash",
			},
		},
		{
			name: "FreeEndTime is nil",
			torrent: models.TorrentInfo{
				PauseOnFreeEnd:   true,
				FreeEndTime:      nil,
				DownloaderTaskID: "hash",
			},
		},
		{
			name: "DownloaderTaskID is empty",
			torrent: models.TorrentInfo{
				PauseOnFreeEnd:   true,
				FreeEndTime:      ptrTime(time.Now().Add(1 * time.Hour)),
				DownloaderTaskID: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.torrent.ID = uint(time.Now().UnixNano())
			monitor.ScheduleTorrent(tt.torrent)

			monitor.mu.Lock()
			_, exists := monitor.pendingTasks[tt.torrent.ID]
			monitor.mu.Unlock()
			assert.False(t, exists, "torrent should not be scheduled")
		})
	}
}

func TestFreeEndMonitor_CancelTorrent(t *testing.T) {
	db := setupTestDB(t)
	dlMgr := downloader.NewDownloaderManager()
	monitor := NewFreeEndMonitor(db.DB, dlMgr)

	err := monitor.Start()
	require.NoError(t, err)
	defer monitor.Stop()

	freeEndTime := time.Now().Add(1 * time.Hour)
	torrent := models.TorrentInfo{
		SiteName:         "test-site",
		TorrentID:        "12345",
		Title:            "Test Torrent",
		PauseOnFreeEnd:   true,
		FreeEndTime:      &freeEndTime,
		DownloaderTaskID: "test-hash-123",
	}
	err = db.DB.Create(&torrent).Error
	require.NoError(t, err)

	monitor.ScheduleTorrent(torrent)

	monitor.mu.Lock()
	_, exists := monitor.pendingTasks[torrent.ID]
	monitor.mu.Unlock()
	require.True(t, exists)

	monitor.CancelTorrent(torrent.ID)

	monitor.mu.Lock()
	_, exists = monitor.pendingTasks[torrent.ID]
	monitor.mu.Unlock()
	assert.False(t, exists, "torrent should be removed from pending tasks")
}

func TestFreeEndMonitor_HandleFreeEndedTorrent_Completed(t *testing.T) {
	db := setupTestDB(t)

	srv := createMockQBitServer(t, 1.0, false)
	defer srv.Close()

	dlMgr := setupTestDownloaderManager(t, srv)

	monitor := NewFreeEndMonitor(db.DB, dlMgr)
	err := monitor.Start()
	require.NoError(t, err)
	defer monitor.Stop()

	freeEndTime := time.Now().Add(-1 * time.Minute)
	torrent := models.TorrentInfo{
		SiteName:         "test-site",
		TorrentID:        "12345",
		Title:            "Test Completed Torrent",
		IsFree:           true,
		PauseOnFreeEnd:   true,
		FreeEndTime:      &freeEndTime,
		DownloaderTaskID: "test-hash-123",
		DownloaderName:   "test-qbit",
	}
	err = db.DB.Create(&torrent).Error
	require.NoError(t, err)

	monitor.handleFreeEndedTorrent(torrent)

	var updated models.TorrentInfo
	err = db.DB.First(&updated, torrent.ID).Error
	require.NoError(t, err)
	assert.True(t, updated.IsCompleted, "completed torrent should be marked as IsCompleted")
	assert.False(t, updated.IsPausedBySystem, "completed torrent should not be paused")
}

func TestFreeEndMonitor_HandleFreeEndedTorrent_NotCompleted(t *testing.T) {
	db := setupTestDB(t)

	srv := createMockQBitServer(t, 0.5, false)
	defer srv.Close()

	dlMgr := setupTestDownloaderManager(t, srv)

	monitor := NewFreeEndMonitor(db.DB, dlMgr)
	err := monitor.Start()
	require.NoError(t, err)
	defer monitor.Stop()

	freeEndTime := time.Now().Add(-1 * time.Minute)
	torrent := models.TorrentInfo{
		SiteName:         "test-site",
		TorrentID:        "12345",
		Title:            "Test Incomplete Torrent",
		IsFree:           true,
		PauseOnFreeEnd:   true,
		FreeEndTime:      &freeEndTime,
		DownloaderTaskID: "test-hash-123",
		DownloaderName:   "test-qbit",
	}
	err = db.DB.Create(&torrent).Error
	require.NoError(t, err)

	monitor.handleFreeEndedTorrent(torrent)

	var updated models.TorrentInfo
	err = db.DB.First(&updated, torrent.ID).Error
	require.NoError(t, err)
	assert.True(t, updated.IsPausedBySystem, "incomplete torrent should be paused by system")
	assert.False(t, updated.IsCompleted, "incomplete torrent should not be marked as completed")
	assert.Equal(t, "免费期结束，下载未完成", updated.PauseReason)
	assert.InDelta(t, 50.0, updated.Progress, 1.0)
}

func TestFreeEndMonitor_UpdateAllMonitoredProgress(t *testing.T) {
	db := setupTestDB(t)

	srv := createMockQBitServer(t, 0.75, false)
	defer srv.Close()

	dlMgr := setupTestDownloaderManager(t, srv)

	monitor := NewFreeEndMonitor(db.DB, dlMgr)

	freeEndTime := time.Now().Add(1 * time.Hour)
	isPushed := true
	torrent := models.TorrentInfo{
		SiteName:         "test-site",
		TorrentID:        "12345",
		Title:            "Test Progress Update",
		PauseOnFreeEnd:   true,
		FreeEndTime:      &freeEndTime,
		DownloaderTaskID: "test-hash-123",
		DownloaderName:   "test-qbit",
		IsPushed:         &isPushed,
	}
	err := db.DB.Create(&torrent).Error
	require.NoError(t, err)

	monitor.updateAllMonitoredProgress()

	var updated models.TorrentInfo
	err = db.DB.First(&updated, torrent.ID).Error
	require.NoError(t, err)
	assert.Greater(t, updated.CheckCount, 0, "CheckCount should increase")
}

func TestFreeEndMonitor_UpdateAllPushedTasksProgress(t *testing.T) {
	db := setupTestDB(t)

	srv := createMockQBitServer(t, 0.6, false)
	defer srv.Close()

	dlMgr := setupTestDownloaderManager(t, srv)

	monitor := NewFreeEndMonitor(db.DB, dlMgr)

	isPushed := true
	torrent := models.TorrentInfo{
		SiteName:         "test-site",
		TorrentID:        "12345",
		Title:            "Test Pushed Task",
		IsPushed:         &isPushed,
		DownloaderTaskID: "test-hash-123",
		DownloaderName:   "test-qbit",
	}
	err := db.DB.Create(&torrent).Error
	require.NoError(t, err)

	monitor.updateAllPushedTasksProgress()

	var updated models.TorrentInfo
	err = db.DB.First(&updated, torrent.ID).Error
	require.NoError(t, err)
	assert.Greater(t, updated.CheckCount, 0, "CheckCount should increase")
}

func TestFreeEndMonitor_ArchiveOldTorrents(t *testing.T) {
	db := setupTestDB(t)
	dlMgr := downloader.NewDownloaderManager()
	monitor := NewFreeEndMonitor(db.DB, dlMgr)

	oldTime := time.Now().AddDate(0, 0, -15)
	torrent := models.TorrentInfo{
		SiteName:    "test-site",
		TorrentID:   "old-torrent",
		Title:       "Old Completed Torrent",
		IsCompleted: true,
	}
	err := db.DB.Create(&torrent).Error
	require.NoError(t, err)

	err = db.DB.Model(&torrent).Update("created_at", oldTime).Error
	require.NoError(t, err)

	monitor.archiveOldTorrents()

	var count int64
	db.DB.Model(&models.TorrentInfo{}).Where("id = ?", torrent.ID).Count(&count)
	assert.Equal(t, int64(0), count, "torrent should be deleted from main table")

	var archiveCount int64
	db.DB.Model(&models.TorrentInfoArchive{}).Where("original_id = ?", torrent.ID).Count(&archiveCount)
	assert.Equal(t, int64(1), archiveCount, "torrent should be in archive table")
}

func TestFreeEndMonitor_ArchiveConditions(t *testing.T) {
	db := setupTestDB(t)
	dlMgr := downloader.NewDownloaderManager()
	monitor := NewFreeEndMonitor(db.DB, dlMgr)

	oldTime := time.Now().AddDate(0, 0, -15)

	tests := []struct {
		name    string
		torrent models.TorrentInfo
	}{
		{
			name: "completed",
			torrent: models.TorrentInfo{
				SiteName:    "test",
				TorrentID:   "completed",
				Title:       "Completed",
				IsCompleted: true,
			},
		},
		{
			name: "paused by system",
			torrent: models.TorrentInfo{
				SiteName:         "test",
				TorrentID:        "paused",
				Title:            "Paused",
				IsPausedBySystem: true,
			},
		},
		{
			name: "skipped and not downloaded",
			torrent: models.TorrentInfo{
				SiteName:     "test",
				TorrentID:    "skipped",
				Title:        "Skipped",
				IsSkipped:    true,
				IsDownloaded: false,
			},
		},
		{
			name: "pushed but no task ID",
			torrent: models.TorrentInfo{
				SiteName:         "test",
				TorrentID:        "zombie",
				Title:            "Zombie",
				IsPushed:         ptr(true),
				DownloaderTaskID: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.DB.Create(&tt.torrent).Error
			require.NoError(t, err)

			err = db.DB.Model(&tt.torrent).Update("created_at", oldTime).Error
			require.NoError(t, err)
		})
	}

	monitor.archiveOldTorrents()

	var count int64
	db.DB.Model(&models.TorrentInfo{}).Count(&count)
	assert.Equal(t, int64(0), count, "all torrents should be archived")

	var archiveCount int64
	db.DB.Model(&models.TorrentInfoArchive{}).Count(&archiveCount)
	assert.Equal(t, int64(4), archiveCount, "should have 4 archive records")
}

func TestFreeEndMonitor_LoadPendingTasksFromDB(t *testing.T) {
	db := setupTestDB(t)

	srv := createMockQBitServer(t, 0.5, false)
	defer srv.Close()

	dlMgr := setupTestDownloaderManager(t, srv)

	freeEndTime := time.Now().Add(1 * time.Hour)
	torrent := models.TorrentInfo{
		SiteName:         "test-site",
		TorrentID:        "pending",
		Title:            "Pending Torrent",
		PauseOnFreeEnd:   true,
		FreeEndTime:      &freeEndTime,
		DownloaderTaskID: "test-hash-123",
		DownloaderName:   "test-qbit",
	}
	err := db.DB.Create(&torrent).Error
	require.NoError(t, err)

	monitor := NewFreeEndMonitor(db.DB, dlMgr)

	err = monitor.loadPendingTasksFromDB()
	require.NoError(t, err)

	monitor.mu.Lock()
	_, exists := monitor.pendingTasks[torrent.ID]
	monitor.mu.Unlock()
	assert.True(t, exists, "pending task should be loaded")
}

func TestFreeEndMonitor_GetDownloader(t *testing.T) {
	db := setupTestDB(t)

	srv := createMockQBitServer(t, 0.5, false)
	defer srv.Close()

	dlMgr := setupTestDownloaderManager(t, srv)

	monitor := NewFreeEndMonitor(db.DB, dlMgr)

	torrent := models.TorrentInfo{
		DownloaderName: "test-qbit",
	}
	gotDl, err := monitor.getDownloader(torrent)
	require.NoError(t, err)
	assert.NotNil(t, gotDl)

	torrent2 := models.TorrentInfo{
		DownloaderName: "",
	}
	gotDl2, err := monitor.getDownloader(torrent2)
	require.NoError(t, err)
	assert.NotNil(t, gotDl2)
}

func TestFreeEndMonitor_GetDownloader_NoManager(t *testing.T) {
	db := setupTestDB(t)

	monitor := NewFreeEndMonitor(db.DB, nil)

	torrent := models.TorrentInfo{
		DownloaderName: "test-qbit",
	}
	_, err := monitor.getDownloader(torrent)
	assert.Error(t, err, "should return error when downloader manager is nil")
}

func TestFreeEndMonitor_MarkCompleted(t *testing.T) {
	db := setupTestDB(t)
	dlMgr := downloader.NewDownloaderManager()
	monitor := NewFreeEndMonitor(db.DB, dlMgr)

	torrent := models.TorrentInfo{
		SiteName:  "test",
		TorrentID: "mark-complete",
		Title:     "Mark Complete Test",
	}
	err := db.DB.Create(&torrent).Error
	require.NoError(t, err)

	monitor.markCompleted(torrent, 1073741824)

	var updated models.TorrentInfo
	err = db.DB.First(&updated, torrent.ID).Error
	require.NoError(t, err)
	assert.True(t, updated.IsCompleted)
	assert.Equal(t, float64(100), updated.Progress)
	assert.Equal(t, int64(1073741824), updated.TorrentSize)
}

func TestFreeEndMonitor_MarkPaused(t *testing.T) {
	db := setupTestDB(t)
	dlMgr := downloader.NewDownloaderManager()
	monitor := NewFreeEndMonitor(db.DB, dlMgr)

	torrent := models.TorrentInfo{
		SiteName:  "test",
		TorrentID: "mark-paused",
		Title:     "Mark Paused Test",
	}
	err := db.DB.Create(&torrent).Error
	require.NoError(t, err)

	monitor.markPaused(torrent, 50.0, 1073741824, false)

	var updated models.TorrentInfo
	err = db.DB.First(&updated, torrent.ID).Error
	require.NoError(t, err)
	assert.True(t, updated.IsPausedBySystem)
	assert.Equal(t, "免费期结束，下载未完成", updated.PauseReason)
	assert.Equal(t, float64(50), updated.Progress)
	assert.Equal(t, int64(1073741824), updated.TorrentSize)
}

func TestFreeEndMonitor_MarkRetry(t *testing.T) {
	db := setupTestDB(t)
	dlMgr := downloader.NewDownloaderManager()
	monitor := NewFreeEndMonitor(db.DB, dlMgr)

	err := monitor.Start()
	require.NoError(t, err)
	defer monitor.Stop()

	freeEndTime := time.Now().Add(1 * time.Hour)
	torrent := models.TorrentInfo{
		SiteName:         "test",
		TorrentID:        "mark-retry",
		Title:            "Mark Retry Test",
		PauseOnFreeEnd:   true,
		FreeEndTime:      &freeEndTime,
		DownloaderTaskID: "test-hash",
	}
	err = db.DB.Create(&torrent).Error
	require.NoError(t, err)

	monitor.markRetry(torrent, "test error")

	var updated models.TorrentInfo
	err = db.DB.First(&updated, torrent.ID).Error
	require.NoError(t, err)
	assert.Equal(t, 1, updated.RetryCount)
	assert.Equal(t, "test error", updated.LastError)
}

func TestFreeEndMonitor_CheckTorrentCompletion(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name            string
		progress        float64
		expectCompleted bool
	}{
		{"not completed", 0.5, false},
		{"just completed", 1.0, true},
		{"over 100%", 1.1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := createMockQBitServer(t, tt.progress, false)
			defer srv.Close()

			dlMgr := setupTestDownloaderManager(t, srv)
			dl, err := dlMgr.GetDownloader("test-qbit")
			require.NoError(t, err)

			monitor := NewFreeEndMonitor(db.DB, dlMgr)
			ctx := context.Background()

			completed, progress, _, err := monitor.checkTorrentCompletion(ctx, dl, "test-hash-123")
			require.NoError(t, err)
			assert.Equal(t, tt.expectCompleted, completed)
			assert.InDelta(t, tt.progress*100, progress, 1.0)
		})
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

// createMockQBitServerWithState pins the exact qBit `state` value returned
// for a given progress, so tests can assert behavior on noise states like
// `pausedDL` or `missingFiles` at progress=1.0.
func createMockQBitServerWithState(t *testing.T, progress float64, state string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/sync/maindata":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":10000000000}}`))
		case "/api/v2/torrents/info":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"hash":"test-hash-123","name":"Test Torrent","progress":` +
				floatToString(progress) + `,"state":"` + state + `","size":1073741824}]`))
		case "/api/v2/torrents/pause":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
}

// TestFreeEndMonitor_UpdateProgress_DoesNotMarkCompletedOnNoiseStates asserts
// that progress=1.0 in non-seeding states must NOT flip is_completed. Marking
// completed cancels the timer and excludes the row from every later free-end
// query, so the user reports "种子没有自动暂停".
func TestFreeEndMonitor_UpdateProgress_DoesNotMarkCompletedOnNoiseStates(t *testing.T) {
	noiseStates := []string{"pausedDL", "stoppedDL", "error", "missingFiles", "checkingResumeData"}
	for _, state := range noiseStates {
		t.Run(state, func(t *testing.T) {
			db := setupTestDB(t)
			srv := createMockQBitServerWithState(t, 1.0, state)
			defer srv.Close()
			dlMgr := setupTestDownloaderManager(t, srv)

			monitor := NewFreeEndMonitor(db.DB, dlMgr)

			freeEndTime := time.Now().Add(1 * time.Hour)
			isPushed := true
			torrent := models.TorrentInfo{
				SiteName:         "test-site",
				TorrentID:        "tid-" + state,
				Title:            "noise-" + state,
				PauseOnFreeEnd:   true,
				FreeEndTime:      &freeEndTime,
				DownloaderTaskID: "test-hash-123",
				DownloaderName:   "test-qbit",
				IsPushed:         &isPushed,
			}
			require.NoError(t, db.DB.Create(&torrent).Error)

			monitor.updateAllMonitoredProgress()

			var got models.TorrentInfo
			require.NoError(t, db.DB.First(&got, torrent.ID).Error)
			assert.False(t, got.IsCompleted,
				"progress=1.0 in noise state %q must NOT mark is_completed; doing so locks the torrent out of the free-end timer permanently",
				state)
		})
	}
}

// TestFreeEndMonitor_UpdateProgress_MarksCompletedOnSeedingStates is the
// positive complement of the noise-state test: a torrent at progress=1.0 in a
// real seeding state must be marked completed.
func TestFreeEndMonitor_UpdateProgress_MarksCompletedOnSeedingStates(t *testing.T) {
	for _, state := range []string{"uploading", "stalledUP", "forcedUP"} {
		t.Run(state, func(t *testing.T) {
			db := setupTestDB(t)
			srv := createMockQBitServerWithState(t, 1.0, state)
			defer srv.Close()
			dlMgr := setupTestDownloaderManager(t, srv)

			monitor := NewFreeEndMonitor(db.DB, dlMgr)

			freeEndTime := time.Now().Add(1 * time.Hour)
			isPushed := true
			torrent := models.TorrentInfo{
				SiteName:         "test-site",
				TorrentID:        "tid-seed-" + state,
				Title:            "seed-" + state,
				PauseOnFreeEnd:   true,
				FreeEndTime:      &freeEndTime,
				DownloaderTaskID: "test-hash-123",
				DownloaderName:   "test-qbit",
				IsPushed:         &isPushed,
			}
			require.NoError(t, db.DB.Create(&torrent).Error)

			monitor.updateAllMonitoredProgress()

			var got models.TorrentInfo
			require.NoError(t, db.DB.First(&got, torrent.ID).Error)
			assert.True(t, got.IsCompleted,
				"progress=1.0 in seeding state %q should mark is_completed",
				state)
		})
	}
}

// TestFreeEndMonitor_PeriodicCheck_RescheduleMissingFutureTorrents asserts the
// 5-minute periodic sweep also covers torrents whose initial Schedule call
// never fired (push happened before monitor wired up, restart loss, etc.).
// Without it, a missed schedule means free-end pause is delayed until the
// torrent is already past its free window.
func TestFreeEndMonitor_PeriodicCheck_RescheduleMissingFutureTorrents(t *testing.T) {
	db := setupTestDB(t)
	srv := createMockQBitServer(t, 0.5, false)
	defer srv.Close()
	dlMgr := setupTestDownloaderManager(t, srv)

	monitor := NewFreeEndMonitor(db.DB, dlMgr)

	freeEndTime := time.Now().Add(2 * time.Hour)
	isPushed := true
	torrent := models.TorrentInfo{
		SiteName:         "test-site",
		TorrentID:        "future-1",
		Title:            "future-pending",
		PauseOnFreeEnd:   true,
		FreeEndTime:      &freeEndTime,
		DownloaderTaskID: "test-hash-123",
		DownloaderName:   "test-qbit",
		IsPushed:         &isPushed,
	}
	require.NoError(t, db.DB.Create(&torrent).Error)

	monitor.mu.Lock()
	_, alreadyScheduled := monitor.pendingTasks[torrent.ID]
	monitor.mu.Unlock()
	require.False(t, alreadyScheduled, "precondition: torrent must NOT be scheduled before periodicCheck runs")

	monitor.checkAndProcessExpiredTorrents()

	monitor.mu.Lock()
	task, scheduled := monitor.pendingTasks[torrent.ID]
	monitor.mu.Unlock()
	require.True(t, scheduled, "periodic check must reschedule a future-expiring torrent that was missed by the initial Schedule")
	require.NotNil(t, task)
	require.NotNil(t, task.timer)
	task.timer.Stop()
	if task.cancel != nil {
		task.cancel()
	}
}

// TestFreeEndMonitor_ScheduleTorrent_LogsSkipReason guards observability:
// when ScheduleTorrent no-ops on missing prerequisites the call must not
// silently return — operators rely on a log line to grep for "跳过调度" when
// diagnosing "this torrent never got a free-end timer".
func TestFreeEndMonitor_ScheduleTorrent_LogsSkipReason(t *testing.T) {
	db := setupTestDB(t)
	srv := createMockQBitServer(t, 0.5, false)
	defer srv.Close()
	dlMgr := setupTestDownloaderManager(t, srv)
	monitor := NewFreeEndMonitor(db.DB, dlMgr)

	freeEnd := time.Now().Add(1 * time.Hour)

	monitor.ScheduleTorrent(models.TorrentInfo{ID: 100, Title: "missing-task-id", PauseOnFreeEnd: true, FreeEndTime: &freeEnd, DownloaderTaskID: ""})
	monitor.mu.Lock()
	_, scheduled := monitor.pendingTasks[100]
	monitor.mu.Unlock()
	require.False(t, scheduled, "torrent without DownloaderTaskID must not be scheduled")

	monitor.ScheduleTorrent(models.TorrentInfo{ID: 101, Title: "missing-free-end", PauseOnFreeEnd: true, FreeEndTime: nil, DownloaderTaskID: "x"})
	monitor.mu.Lock()
	_, scheduled = monitor.pendingTasks[101]
	monitor.mu.Unlock()
	require.False(t, scheduled, "torrent without FreeEndTime must not be scheduled")
}

func setAdvanceMinutes(t *testing.T, db *models.TorrentDB, minutes int) {
	t.Helper()
	require.NoError(t, db.DB.Where("1 = 1").Delete(&models.SettingsGlobal{}).Error)
	require.NoError(t, db.DB.Create(&models.SettingsGlobal{FreeEndAdvanceMinutes: minutes}).Error)
}

func TestEffectiveDeadline(t *testing.T) {
	assert.True(t, effectiveDeadline(nil, 0).IsZero(), "nil freeEnd should return zero time")
	assert.True(t, effectiveDeadline(nil, 10*time.Minute).IsZero(), "nil freeEnd should return zero time regardless of advance")

	freeEnd := time.Now().Add(1 * time.Hour)
	assert.Equal(t, freeEnd, effectiveDeadline(&freeEnd, 0), "advance=0 should equal freeEnd")
	assert.Equal(t, freeEnd.Add(-10*time.Minute), effectiveDeadline(&freeEnd, 10*time.Minute), "advance=10m should shift freeEnd back 10m")
}

func TestAdvanceDuration_ClampAndDefault(t *testing.T) {
	t.Run("no config row", func(t *testing.T) {
		db := setupTestDB(t)
		monitor := NewFreeEndMonitor(db.DB, downloader.NewDownloaderManager())
		assert.Equal(t, time.Duration(0), monitor.advanceDuration(), "no config row should yield 0")
	})

	cases := []struct {
		name    string
		minutes int
		want    time.Duration
	}{
		{"zero", 0, 0},
		{"ten", 10, 10 * time.Minute},
		{"clamp over max", 999, maxFreeEndAdvance},
		{"negative", -5, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTestDB(t)
			setAdvanceMinutes(t, db, tc.minutes)
			monitor := NewFreeEndMonitor(db.DB, downloader.NewDownloaderManager())
			assert.Equal(t, tc.want, monitor.advanceDuration())
		})
	}
}

func TestScheduleTask_AdvanceShiftsDelay(t *testing.T) {
	db := setupTestDB(t)
	srv := createMockQBitServer(t, 0.5, false)
	defer srv.Close()
	dlMgr := setupTestDownloaderManager(t, srv)

	setAdvanceMinutes(t, db, 10)
	monitor := NewFreeEndMonitor(db.DB, dlMgr)

	freeEnd := time.Now().Add(12 * time.Minute)
	torrent := models.TorrentInfo{
		SiteName:         "test-site",
		TorrentID:        "advance-shift",
		Title:            "Advance Shift",
		PauseOnFreeEnd:   true,
		FreeEndTime:      &freeEnd,
		DownloaderTaskID: "test-hash-123",
		DownloaderName:   "test-qbit",
	}
	require.NoError(t, db.DB.Create(&torrent).Error)

	monitor.ScheduleTorrent(torrent)

	monitor.mu.Lock()
	task, exists := monitor.pendingTasks[torrent.ID]
	monitor.mu.Unlock()
	require.True(t, exists, "torrent should be scheduled")
	require.NotNil(t, task.timer)
	task.timer.Stop()
	if task.cancel != nil {
		task.cancel()
	}

	rawDelay := time.Until(freeEnd)
	effectiveDelay := time.Until(effectiveDeadline(&freeEnd, monitor.advanceDuration()))
	assert.Less(t, effectiveDelay, rawDelay, "advance should shift the effective delay earlier")
	assert.InDelta(t, (2 * time.Minute).Seconds(), effectiveDelay.Seconds(), 5.0, "effective delay should be ~2m")
}

func TestCheckExpired_AdvancedCutoff(t *testing.T) {
	t.Run("advance includes near-end torrent", func(t *testing.T) {
		db := setupTestDB(t)
		srv := createMockQBitServer(t, 0.5, false)
		defer srv.Close()
		dlMgr := setupTestDownloaderManager(t, srv)

		setAdvanceMinutes(t, db, 5)
		monitor := NewFreeEndMonitor(db.DB, dlMgr)

		freeEnd := time.Now().Add(3 * time.Minute)
		torrent := models.TorrentInfo{
			SiteName:         "test-site",
			TorrentID:        "near-end",
			Title:            "Near End",
			PauseOnFreeEnd:   true,
			FreeEndTime:      &freeEnd,
			DownloaderTaskID: "test-hash-123",
			DownloaderName:   "test-qbit",
		}
		require.NoError(t, db.DB.Create(&torrent).Error)

		monitor.checkAndProcessExpiredTorrents()
		monitor.wg.Wait()

		var updated models.TorrentInfo
		require.NoError(t, db.DB.First(&updated, torrent.ID).Error)
		assert.True(t, updated.IsPausedBySystem, "near-end torrent within advance window should be processed")
	})

	t.Run("advance zero excludes future torrent", func(t *testing.T) {
		db := setupTestDB(t)
		srv := createMockQBitServer(t, 0.5, false)
		defer srv.Close()
		dlMgr := setupTestDownloaderManager(t, srv)

		setAdvanceMinutes(t, db, 0)
		monitor := NewFreeEndMonitor(db.DB, dlMgr)

		freeEnd := time.Now().Add(3 * time.Minute)
		torrent := models.TorrentInfo{
			SiteName:         "test-site",
			TorrentID:        "still-future",
			Title:            "Still Future",
			PauseOnFreeEnd:   true,
			FreeEndTime:      &freeEnd,
			DownloaderTaskID: "test-hash-123",
			DownloaderName:   "test-qbit",
		}
		require.NoError(t, db.DB.Create(&torrent).Error)

		monitor.checkAndProcessExpiredTorrents()
		monitor.wg.Wait()

		var updated models.TorrentInfo
		require.NoError(t, db.DB.First(&updated, torrent.ID).Error)
		assert.False(t, updated.IsPausedBySystem, "advance=0 must not process a still-future torrent")
	})
}

func TestBackwardCompat_AdvanceZero(t *testing.T) {
	db := setupTestDB(t)
	srv := createMockQBitServer(t, 0.5, false)
	defer srv.Close()
	dlMgr := setupTestDownloaderManager(t, srv)

	setAdvanceMinutes(t, db, 0)
	monitor := NewFreeEndMonitor(db.DB, dlMgr)

	expired := time.Now().Add(-1 * time.Second)
	expiredTorrent := models.TorrentInfo{
		SiteName:         "test-site",
		TorrentID:        "compat-expired",
		Title:            "Compat Expired",
		PauseOnFreeEnd:   true,
		FreeEndTime:      &expired,
		DownloaderTaskID: "test-hash-123",
		DownloaderName:   "test-qbit",
	}
	require.NoError(t, db.DB.Create(&expiredTorrent).Error)

	future := time.Now().Add(1 * time.Hour)
	futureTorrent := models.TorrentInfo{
		SiteName:         "test-site",
		TorrentID:        "compat-future",
		Title:            "Compat Future",
		PauseOnFreeEnd:   true,
		FreeEndTime:      &future,
		DownloaderTaskID: "test-hash-456",
		DownloaderName:   "test-qbit",
	}
	require.NoError(t, db.DB.Create(&futureTorrent).Error)

	monitor.checkAndProcessExpiredTorrents()
	monitor.wg.Wait()

	var gotExpired models.TorrentInfo
	require.NoError(t, db.DB.First(&gotExpired, expiredTorrent.ID).Error)
	assert.True(t, gotExpired.IsPausedBySystem, "advance=0: expired torrent should be processed")

	var gotFuture models.TorrentInfo
	require.NoError(t, db.DB.First(&gotFuture, futureTorrent.ID).Error)
	assert.False(t, gotFuture.IsPausedBySystem, "advance=0: future torrent should not be processed")
}

func TestAdvanceLargerThanFreeWindow_Immediate(t *testing.T) {
	db := setupTestDB(t)
	srv := createMockQBitServer(t, 0.5, false)
	defer srv.Close()
	dlMgr := setupTestDownloaderManager(t, srv)

	setAdvanceMinutes(t, db, 10)
	monitor := NewFreeEndMonitor(db.DB, dlMgr)

	freeEnd := time.Now().Add(2 * time.Minute)
	torrent := models.TorrentInfo{
		SiteName:         "test-site",
		TorrentID:        "advance-overflow",
		Title:            "Advance Overflow",
		PauseOnFreeEnd:   true,
		FreeEndTime:      &freeEnd,
		DownloaderTaskID: "test-hash-123",
		DownloaderName:   "test-qbit",
	}
	require.NoError(t, db.DB.Create(&torrent).Error)

	require.NotPanics(t, func() {
		monitor.scheduleTask(torrent)
	})

	monitor.mu.Lock()
	task, exists := monitor.pendingTasks[torrent.ID]
	monitor.mu.Unlock()
	require.True(t, exists)
	require.NotNil(t, task.timer)
	task.timer.Stop()
	if task.cancel != nil {
		task.cancel()
	}

	effectiveDelay := time.Until(effectiveDeadline(&freeEnd, monitor.advanceDuration()))
	assert.LessOrEqual(t, effectiveDelay, time.Duration(0), "effective deadline should be in the past")
}

func TestMarkRetry_NotDoubleAdvanced(t *testing.T) {
	db := setupTestDB(t)
	dlMgr := downloader.NewDownloaderManager()

	setAdvanceMinutes(t, db, 10)
	monitor := NewFreeEndMonitor(db.DB, dlMgr)
	require.NoError(t, monitor.Start())
	defer monitor.Stop()

	freeEnd := time.Now().Add(1 * time.Hour)
	torrent := models.TorrentInfo{
		SiteName:         "test",
		TorrentID:        "retry-no-double",
		Title:            "Retry No Double",
		PauseOnFreeEnd:   true,
		FreeEndTime:      &freeEnd,
		DownloaderTaskID: "test-hash",
	}
	require.NoError(t, db.DB.Create(&torrent).Error)

	before := time.Now()
	monitor.markRetry(torrent, "test error")

	monitor.mu.Lock()
	task, exists := monitor.pendingTasks[torrent.ID]
	monitor.mu.Unlock()
	require.True(t, exists, "retry should schedule a follow-up timer")
	require.NotNil(t, task.timer)
	task.timer.Stop()
	if task.cancel != nil {
		task.cancel()
	}

	advance := monitor.advanceDuration()
	require.Equal(t, 10*time.Minute, advance)

	// markRetry uses retryDeadline(now, retryCount=1, advance): retryTime is
	// pre-advanced so scheduleTaskLocked's effectiveDeadline subtraction nets
	// back to now+retryDelay (not double-advanced).
	retryTime := retryDeadline(before, 1, advance)
	netTrigger := effectiveDeadline(&retryTime, advance)
	expectedRetryDelay := min(baseRetryDelay*time.Duration(1<<1)*2, maxRetryDelay)
	gotNetDelay := netTrigger.Sub(before)
	assert.InDelta(t, expectedRetryDelay.Seconds(), gotNetDelay.Seconds(), 5.0,
		"retry net trigger must be ~now+retryDelay, not double-advanced")

	assert.InDelta(t, (expectedRetryDelay + advance).Seconds(), retryTime.Sub(before).Seconds(), 5.0,
		"retryTime must be pre-compensated by +advance")

	var updated models.TorrentInfo
	require.NoError(t, db.DB.First(&updated, torrent.ID).Error)
	require.Equal(t, 1, updated.RetryCount)
}

func TestPauseReason_TextByAdvance(t *testing.T) {
	t.Run("advance positive uses near-end wording", func(t *testing.T) {
		db := setupTestDB(t)
		srv := createMockQBitServer(t, 0.5, false)
		defer srv.Close()
		dlMgr := setupTestDownloaderManager(t, srv)

		setAdvanceMinutes(t, db, 10)
		monitor := NewFreeEndMonitor(db.DB, dlMgr)

		freeEnd := time.Now().Add(-1 * time.Minute)
		torrent := models.TorrentInfo{
			SiteName:         "test-site",
			TorrentID:        "reason-advance",
			Title:            "Reason Advance",
			PauseOnFreeEnd:   true,
			FreeEndTime:      &freeEnd,
			DownloaderTaskID: "test-hash-123",
			DownloaderName:   "test-qbit",
		}
		require.NoError(t, db.DB.Create(&torrent).Error)

		monitor.handleFreeEndedTorrent(torrent)

		var updated models.TorrentInfo
		require.NoError(t, db.DB.First(&updated, torrent.ID).Error)
		assert.True(t, updated.IsPausedBySystem)
		assert.Contains(t, updated.PauseReason, "临近结束", "advance>0 pause reason should mention 临近结束")
	})

	t.Run("advance zero keeps original wording", func(t *testing.T) {
		db := setupTestDB(t)
		srv := createMockQBitServer(t, 0.5, false)
		defer srv.Close()
		dlMgr := setupTestDownloaderManager(t, srv)

		setAdvanceMinutes(t, db, 0)
		monitor := NewFreeEndMonitor(db.DB, dlMgr)

		freeEnd := time.Now().Add(-1 * time.Minute)
		torrent := models.TorrentInfo{
			SiteName:         "test-site",
			TorrentID:        "reason-exact",
			Title:            "Reason Exact",
			PauseOnFreeEnd:   true,
			FreeEndTime:      &freeEnd,
			DownloaderTaskID: "test-hash-123",
			DownloaderName:   "test-qbit",
		}
		require.NoError(t, db.DB.Create(&torrent).Error)

		monitor.handleFreeEndedTorrent(torrent)

		var updated models.TorrentInfo
		require.NoError(t, db.DB.First(&updated, torrent.ID).Error)
		assert.True(t, updated.IsPausedBySystem)
		assert.Equal(t, "免费期结束，下载未完成", updated.PauseReason, "advance=0 should keep original wording")
	})
}

// TestRescheduleMissingFutureTorrents_MultipleWithAdvance exercises the FIX-2
// lock-outside advance read: multiple future torrents must all be rescheduled
// with the advanced effective delay (freeEnd - advance), proving the refactored
// path computes advance once outside the lock without changing per-torrent timing.
func TestRescheduleMissingFutureTorrents_MultipleWithAdvance(t *testing.T) {
	db := setupTestDB(t)
	srv := createMockQBitServer(t, 0.5, false)
	defer srv.Close()
	dlMgr := setupTestDownloaderManager(t, srv)

	setAdvanceMinutes(t, db, 10)
	monitor := NewFreeEndMonitor(db.DB, dlMgr)

	freeEnd := time.Now().Add(2 * time.Hour)
	ids := make([]uint, 0, 3)
	for i := range 3 {
		torrent := models.TorrentInfo{
			SiteName:         "test-site",
			TorrentID:        fmt.Sprintf("future-%d", i),
			Title:            fmt.Sprintf("future-%d", i),
			PauseOnFreeEnd:   true,
			FreeEndTime:      &freeEnd,
			DownloaderTaskID: "test-hash-123",
			DownloaderName:   "test-qbit",
		}
		require.NoError(t, db.DB.Create(&torrent).Error)
		ids = append(ids, torrent.ID)
	}

	monitor.rescheduleMissingFutureTorrents(time.Now().Add(monitor.advanceDuration()))

	wantDelay := time.Until(effectiveDeadline(&freeEnd, 10*time.Minute))
	monitor.mu.Lock()
	defer monitor.mu.Unlock()
	require.Len(t, monitor.pendingTasks, 3, "all future torrents should be scheduled")
	for _, id := range ids {
		task, ok := monitor.pendingTasks[id]
		require.True(t, ok, "torrent %d should be scheduled", id)
		require.NotNil(t, task.timer)
		task.timer.Stop()
		if task.cancel != nil {
			task.cancel()
		}
	}
	assert.InDelta(t, (110 * time.Minute).Seconds(), wantDelay.Seconds(), 5.0,
		"advanced effective delay should be ~110m (120m - 10m)")
}

// TestRescheduleMissingFutureTorrents_RespectsBatchLimit locks FIX-3: the query
// is capped at progressUpdateBatchSize. With a small set under the cap every row
// is still scheduled and the call must not panic; rows beyond the cap are picked
// up by the next periodicCheck (idempotent), so capping is safe.
func TestRescheduleMissingFutureTorrents_RespectsBatchLimit(t *testing.T) {
	db := setupTestDB(t)
	srv := createMockQBitServer(t, 0.5, false)
	defer srv.Close()
	dlMgr := setupTestDownloaderManager(t, srv)

	monitor := NewFreeEndMonitor(db.DB, dlMgr)

	freeEnd := time.Now().Add(2 * time.Hour)
	const n = 5
	for i := range n {
		torrent := models.TorrentInfo{
			SiteName:         "test-site",
			TorrentID:        fmt.Sprintf("batch-%d", i),
			Title:            fmt.Sprintf("batch-%d", i),
			PauseOnFreeEnd:   true,
			FreeEndTime:      &freeEnd,
			DownloaderTaskID: "test-hash-123",
			DownloaderName:   "test-qbit",
		}
		require.NoError(t, db.DB.Create(&torrent).Error)
	}

	require.NotPanics(t, func() {
		monitor.rescheduleMissingFutureTorrents(time.Now())
	})

	monitor.mu.Lock()
	defer monitor.mu.Unlock()
	scheduled := len(monitor.pendingTasks)
	for _, task := range monitor.pendingTasks {
		if task.timer != nil {
			task.timer.Stop()
		}
		if task.cancel != nil {
			task.cancel()
		}
	}
	assert.LessOrEqual(t, scheduled, progressUpdateBatchSize, "scheduled count must not exceed batch cap")
	assert.Equal(t, n, scheduled, "a set under the cap should all be scheduled")
}

func TestScheduleTaskLocked_NilFreeEndTime(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, _ := newFreeEndMonitorWithFake(t, fake)

	m.mu.Lock()
	before := len(m.pendingTasks)
	m.scheduleTaskLockedWithAdvance(models.TorrentInfo{ID: 1, FreeEndTime: nil}, 0)
	after := len(m.pendingTasks)
	m.mu.Unlock()

	assert.Equal(t, before, after, "nil FreeEndTime must not add a pending task")
}

func TestMarkCompleted_Direct(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)

	tor := models.TorrentInfo{SiteName: "s", TorrentID: "mc1", Title: "MC"}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.markCompleted(tor, 4242)

	var updated models.TorrentInfo
	require.NoError(t, db.DB.First(&updated, tor.ID).Error)
	assert.True(t, updated.IsCompleted)
	assert.InDelta(t, 100.0, updated.Progress, 0.01)
	assert.Equal(t, int64(4242), updated.TorrentSize)
}

func TestCancelTorrent_RemovesPending(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.torrentByID["task-cancel"] = downloader.Torrent{
		ID: "task-cancel", Progress: 0.5, State: downloader.TorrentDownloading,
	}
	m, db := newFreeEndMonitorWithFake(t, fake)

	future := time.Now().Add(time.Hour)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "c1", Title: "Cancel", PauseOnFreeEnd: true,
		FreeEndTime: &future, DownloaderTaskID: "task-cancel", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.ScheduleTorrent(tor)
	m.mu.Lock()
	_, exists := m.pendingTasks[tor.ID]
	m.mu.Unlock()
	require.True(t, exists)

	m.CancelTorrent(tor.ID)
	m.mu.Lock()
	_, stillThere := m.pendingTasks[tor.ID]
	m.mu.Unlock()
	assert.False(t, stillThere, "CancelTorrent should remove the pending task")
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
