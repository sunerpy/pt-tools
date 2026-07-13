// MIT License
// Copyright (c) 2025 pt-tools

package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

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
