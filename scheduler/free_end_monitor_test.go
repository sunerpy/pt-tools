package scheduler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
			state := "downloading"
			if paused {
				state = "pausedDL"
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
	config := &qbitDownloaderConfig{
		url:      srv.URL,
		username: "admin",
		password: "admin",
	}
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

	monitor.markPaused(torrent, 50.0, 1073741824)

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
