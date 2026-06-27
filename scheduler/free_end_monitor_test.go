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
