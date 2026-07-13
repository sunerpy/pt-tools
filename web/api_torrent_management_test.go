package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/extension"
	"github.com/sunerpy/pt-tools/models"
)

// ==== merged from api_mixed_cov4_test.go ====
func TestHandleExtensionActionAck_AlreadyAcked(t *testing.T) {
	srv, cleanup := newExtensionActionTestServer(t)
	defer cleanup()

	require.NoError(t, extension.Enqueue(global.GlobalDB.DB, extension.PendingAction{
		Type: extension.ActionOpenTab, TargetURL: "https://hdsky.me/", SiteName: "hdsky",
	}))

	w1 := httptest.NewRecorder()
	srv.handleExtensionActionAck(w1, authedRequest(http.MethodPost, "/api/extension/actions/1/ack", nil), 1)
	require.Equal(t, http.StatusOK, w1.Code)
	assert.Contains(t, w1.Body.String(), "acked")

	w2 := httptest.NewRecorder()
	srv.handleExtensionActionAck(w2, authedRequest(http.MethodPost, "/api/extension/actions/1/ack", nil), 1)
	require.Equal(t, http.StatusOK, w2.Code)
	assert.Contains(t, w2.Body.String(), "already_acked")
}

func TestHandleExtensionActionAck_NotFound(t *testing.T) {
	srv, cleanup := newExtensionActionTestServer(t)
	defer cleanup()

	w := httptest.NewRecorder()
	srv.handleExtensionActionAck(w, authedRequest(http.MethodPost, "/api/extension/actions/999/ack", nil), 999)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestApiArchiveTorrents_Paths(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfoArchive{}))
	require.NoError(t, db.Create(&models.TorrentInfoArchive{
		SiteName: "hdsky", Title: "Archived One", IsCompleted: true,
	}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/archive", nil)
		server.apiArchiveTorrents(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("list with site filter and paging", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/torrents/archive?site=hdsky&page=1&page_size=10", nil)
		server.apiArchiveTorrents(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestApiTorrentManagementRouter_Dispatch(t *testing.T) {
	server, _ := setupServerWithFakeDownloader(t, &fakeDownloader{})
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.TorrentInfo{}, &models.TorrentInfoArchive{}))

	t.Run("paused route", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/torrents/paused", nil)
		server.apiTorrentManagementRouter(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("archive route", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/torrents/archive", nil)
		server.apiTorrentManagementRouter(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("resume route", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/5/resume", nil)
		server.apiTorrentManagementRouter(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("unknown route", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/torrents/bogus", nil)
		server.apiTorrentManagementRouter(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// ==== merged from api_rss_login_cov2_test.go ====
func TestUpdateRSSFilterAssociation_ErrorBranches(t *testing.T) {
	db := setupTestDB(t)
	server := NewServer(core.NewConfigStore(db), nil)

	rss := models.RSSSubscription{Name: "r1", URL: "http://e/rss", IntervalMinutes: 10}
	require.NoError(t, db.DB.Create(&rss).Error)
	disabled := models.FilterRule{Name: "disabled-rule", Pattern: ".*", PatternType: "regex", Enabled: true, Priority: 1}
	require.NoError(t, db.DB.Create(&disabled).Error)
	require.NoError(t, db.DB.Model(&disabled).Update("enabled", false).Error)

	t.Run("rss not found", func(t *testing.T) {
		body, _ := json.Marshal(RSSFilterAssociationRequest{FilterRuleIDs: []uint{}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/rss/999/filter-rules", bytes.NewReader(body))
		server.updateRSSFilterAssociation(w, req, 999)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("bad body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/rss/1/filter-rules", bytes.NewBufferString(`{bad`))
		server.updateRSSFilterAssociation(w, req, rss.ID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("nonexistent rule id", func(t *testing.T) {
		body, _ := json.Marshal(RSSFilterAssociationRequest{FilterRuleIDs: []uint{9999}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/rss/1/filter-rules", bytes.NewReader(body))
		server.updateRSSFilterAssociation(w, req, rss.ID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("disabled rule rejected", func(t *testing.T) {
		body, _ := json.Marshal(RSSFilterAssociationRequest{FilterRuleIDs: []uint{disabled.ID}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/rss/1/filter-rules", bytes.NewReader(body))
		server.updateRSSFilterAssociation(w, req, rss.ID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestApiDeletePausedTorrents_Paths(t *testing.T) {
	server, _ := setupServerWithFakeDownloader(t, &fakeDownloader{})
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.TorrentInfo{}))

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/torrents/delete-paused", nil)
		server.apiDeletePausedTorrents(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("bad body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/delete-paused", bytes.NewBufferString(`{bad`))
		server.apiDeletePausedTorrents(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("no paused torrents returns zero", func(t *testing.T) {
		body, _ := json.Marshal(DeletePausedRequest{})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/delete-paused", bytes.NewReader(body))
		server.apiDeletePausedTorrents(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("delete paused torrent via fake downloader", func(t *testing.T) {
		ti := models.TorrentInfo{
			SiteName: "s", TorrentID: "p1", IsPausedBySystem: true,
			DownloaderTaskID: "task-p1", DownloaderName: "qb1",
		}
		require.NoError(t, global.GlobalDB.DB.Create(&ti).Error)
		body, _ := json.Marshal(DeletePausedRequest{IDs: []uint{ti.ID}, RemoveData: true})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/delete-paused", bytes.NewReader(body))
		server.apiDeletePausedTorrents(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp DeletePausedResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp.Success)
	})
}

func TestApiSiteLoginStateVisit_Paths(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/login-state/visit", nil)
		srv.apiSiteLoginStateVisit(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("bad body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/visit", bytes.NewBufferString(`{bad`))
		srv.apiSiteLoginStateVisit(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("empty site name", func(t *testing.T) {
		body, _ := json.Marshal(SiteVisitReportRequest{SiteName: ""})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/visit", bytes.NewReader(body))
		srv.apiSiteLoginStateVisit(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("success clamps visit", func(t *testing.T) {
		body, _ := json.Marshal(SiteVisitReportRequest{SiteName: "hdsky", LastVisitAt: "2026-01-01T00:00:00Z"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/visit", bytes.NewReader(body))
		srv.apiSiteLoginStateVisit(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

// ==== merged from api_torrent_management_cov_test.go ====
func TestApiPausedTorrents(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}))

	now := time.Now()
	require.NoError(t, db.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "1", Title: "Paused A",
		IsPausedBySystem: true, PausedAt: &now, PauseReason: "free_end",
	}).Error)
	require.NoError(t, db.Create(&models.TorrentInfo{
		SiteName: "mteam", TorrentID: "2", Title: "Not Paused",
		IsPausedBySystem: false,
	}).Error)

	t.Run("list paused torrents", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/torrents/paused", nil)
		server.apiPausedTorrents(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp struct {
			Items    []PausedTorrentResponse `json:"items"`
			Total    int64                   `json:"total"`
			Page     int                     `json:"page"`
			PageSize int                     `json:"page_size"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, int64(1), resp.Total)
		assert.Len(t, resp.Items, 1)
		assert.Equal(t, "Paused A", resp.Items[0].Title)
	})

	t.Run("site filter with paging params", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/torrents/paused?site=hdsky&page=1&page_size=10", nil)
		server.apiPausedTorrents(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp struct {
			Total int64 `json:"total"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, int64(1), resp.Total)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/paused", nil)
		server.apiPausedTorrents(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApiDeletePausedTorrents_NoDownloaderManager(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}))

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/torrents/delete-paused", nil)
		server.apiDeletePausedTorrents(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("invalid body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/delete-paused", bytes.NewBufferString(`{bad`))
		server.apiDeletePausedTorrents(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("no paused torrents returns zero", func(t *testing.T) {
		body, _ := json.Marshal(DeletePausedRequest{IDs: []uint{999}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/delete-paused", bytes.NewReader(body))
		server.apiDeletePausedTorrents(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp DeletePausedResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 0, resp.Success)
		assert.Equal(t, 0, resp.Failed)
	})

	t.Run("paused torrent without downloader task deletes directly", func(t *testing.T) {
		require.NoError(t, db.Create(&models.TorrentInfo{
			SiteName: "hdsky", TorrentID: "10", Title: "P10", IsPausedBySystem: true,
		}).Error)

		// mgr is nil so getDownloaderManager returns nil -> 500
		body, _ := json.Marshal(DeletePausedRequest{})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/delete-paused", bytes.NewReader(body))
		server.apiDeletePausedTorrents(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestApiArchiveTorrents(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfoArchive{}))

	now := time.Now()
	require.NoError(t, db.Create(&models.TorrentInfoArchive{
		OriginalID: 1, SiteName: "hdsky", Title: "Archived A", ArchivedAt: now, OriginalCreatedAt: now,
	}).Error)

	t.Run("list archives", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/torrents/archive", nil)
		server.apiArchiveTorrents(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp struct {
			Items []ArchiveTorrentResponse `json:"items"`
			Total int64                    `json:"total"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, int64(1), resp.Total)
	})

	t.Run("site filter", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/torrents/archive?site=none&page=2&page_size=5", nil)
		server.apiArchiveTorrents(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/archive", nil)
		server.apiArchiveTorrents(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApiResumeTorrent(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}))

	require.NoError(t, db.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "1", Title: "NotPaused", IsPausedBySystem: false,
	}).Error)
	require.NoError(t, db.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "2", Title: "PausedNoTask", IsPausedBySystem: true,
	}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/torrents/1/resume", nil)
		server.apiResumeTorrent(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("invalid id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/abc/resume", nil)
		server.apiResumeTorrent(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/999/resume", nil)
		server.apiResumeTorrent(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("not paused by system", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/1/resume", nil)
		server.apiResumeTorrent(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp ResumeTorrentResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.False(t, resp.Success)
	})

	t.Run("paused without downloader info", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/2/resume", nil)
		server.apiResumeTorrent(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestApiTorrentManagementRouter(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}, &models.TorrentInfoArchive{}))

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{"paused", http.MethodGet, "/api/torrents/paused", http.StatusOK},
		{"archive", http.MethodGet, "/api/torrents/archive", http.StatusOK},
		{"resume invalid id", http.MethodPost, "/api/torrents/abc/resume", http.StatusBadRequest},
		{"unknown", http.MethodGet, "/api/torrents/unknown", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			server.apiTorrentManagementRouter(w, req)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestGetDownloaderManager_NilMgr(t *testing.T) {
	s := &Server{}
	assert.Nil(t, s.getDownloaderManager())
}

// ==== merged from api_torrent_management_data_test.go ====
func TestApiDeletePausedTorrents_WithDownloader(t *testing.T) {
	fake := &fakeDownloader{}
	server, _ := setupServerWithFakeDownloader(t, fake)
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.TorrentInfo{}))

	now := time.Now()
	require.NoError(t, global.GlobalDB.DB.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "1", Title: "Paused A", IsPausedBySystem: true,
		PausedAt: &now, DownloaderName: "qb1", DownloaderTaskID: "task-1",
	}).Error)
	require.NoError(t, global.GlobalDB.DB.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "2", Title: "Paused B", IsPausedBySystem: true,
		PausedAt: &now,
	}).Error)

	body, _ := json.Marshal(DeletePausedRequest{RemoveData: false})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/torrents/delete-paused", bytes.NewReader(body))
	server.apiDeletePausedTorrents(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp DeletePausedResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.Success)
}

func TestApiResumeTorrent_WithDownloader(t *testing.T) {
	fake := &fakeDownloader{}
	server, _ := setupServerWithFakeDownloader(t, fake)
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.TorrentInfo{}))

	now := time.Now()
	require.NoError(t, global.GlobalDB.DB.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "1", Title: "Paused A", IsPausedBySystem: true,
		PausedAt: &now, DownloaderName: "qb1", DownloaderTaskID: "task-1",
	}).Error)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/torrents/1/resume", nil)
	server.apiResumeTorrent(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp ResumeTorrentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)

	var row models.TorrentInfo
	require.NoError(t, global.GlobalDB.DB.First(&row, 1).Error)
	assert.False(t, row.IsPausedBySystem)
}

// ==== merged from api_torrent_management_err_test.go ====
func TestApiDeletePausedTorrents_RemoveError(t *testing.T) {
	fake := &fakeDownloader{removeErr: errors.New("cannot remove")}
	server, _ := setupServerWithFakeDownloader(t, fake)
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.TorrentInfo{}))

	now := time.Now()
	require.NoError(t, global.GlobalDB.DB.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "1", Title: "P1", IsPausedBySystem: true,
		PausedAt: &now, DownloaderName: "qb1", DownloaderTaskID: "task-1",
	}).Error)

	body, _ := json.Marshal(DeletePausedRequest{RemoveData: true})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/torrents/delete-paused", bytes.NewReader(body))
	server.apiDeletePausedTorrents(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp DeletePausedResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.Failed)
}

func TestApiDeleteTasks_SkipsPushed(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}))

	pushed := true
	require.NoError(t, db.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "1", Title: "Unpushed", IsPushed: &pushed,
	}).Error)
	unpushed := false
	require.NoError(t, db.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "2", Title: "Deletable", IsPushed: &unpushed,
	}).Error)

	body, _ := json.Marshal(DeleteTasksRequest{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tasks/batch-delete", bytes.NewReader(body))
	server.apiDeleteTasks(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp DeleteTasksResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.Success)
}

// ==== merged from api_torrent_management_test.go ====
// TestApiDeleteTasks 测试批量删除任务API
func TestApiDeleteTasks(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*testing.T, *gorm.DB)
		request   *DeleteTasksRequest
		checkResp func(*testing.T, *http.Response, *DeleteTasksResponse)
	}{
		{
			name: "Delete single unpushed record",
			setup: func(t *testing.T, db *gorm.DB) {
				// 创建一条未推送的记录
				torrent := models.TorrentInfo{
					SiteName:     "test-site",
					TorrentID:    "123",
					Title:        "Test Torrent 1",
					IsPushed:     boolPtr(false),
					IsFree:       true,
					IsDownloaded: false,
				}
				if err := db.Create(&torrent).Error; err != nil {
					t.Fatalf("failed to create test torrent: %v", err)
				}
			},
			request: &DeleteTasksRequest{IDs: []uint{1}},
			checkResp: func(t *testing.T, resp *http.Response, body *DeleteTasksResponse) {
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				assert.Equal(t, 1, body.Success)
				assert.Equal(t, 0, body.Failed)
				assert.Empty(t, body.FailedIDs)
				assert.Empty(t, body.FailedErrors)

				// 验证数据库中记录已删除
				var count int64
				global.GlobalDB.DB.Model(&models.TorrentInfo{}).Where("id = ?", 1).Count(&count)
				assert.Equal(t, int64(0), count)
			},
		},
		{
			name: "Delete multiple unpushed records",
			setup: func(t *testing.T, db *gorm.DB) {
				// 创建3条未推送的记录
				torrents := []models.TorrentInfo{
					{
						SiteName:     "test-site",
						TorrentID:    "123",
						Title:        "Test Torrent 1",
						IsPushed:     boolPtr(false),
						IsFree:       true,
						IsDownloaded: false,
					},
					{
						SiteName:     "test-site",
						TorrentID:    "124",
						Title:        "Test Torrent 2",
						IsPushed:     boolPtr(false),
						IsFree:       false,
						IsDownloaded: false,
					},
					{
						SiteName:     "test-site",
						TorrentID:    "125",
						Title:        "Test Torrent 3",
						IsPushed:     boolPtr(false),
						IsFree:       true,
						IsDownloaded: true,
					},
				}
				for _, torr := range torrents {
					if err := db.Create(&torr).Error; err != nil {
						panic("failed to create test torrents")
					}
				}
			},
			request: &DeleteTasksRequest{IDs: []uint{1, 2, 3}},
			checkResp: func(t *testing.T, resp *http.Response, body *DeleteTasksResponse) {
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				assert.Equal(t, 3, body.Success)
				assert.Equal(t, 0, body.Failed)
				assert.Empty(t, body.FailedIDs)
				assert.Empty(t, body.FailedErrors)

				// 验证所有记录已删除
				var count int64
				global.GlobalDB.DB.Model(&models.TorrentInfo{}).Count(&count)
				assert.Equal(t, int64(0), count)
			},
		},
		{
			name: "Attempt to delete pushed record",
			setup: func(t *testing.T, db *gorm.DB) {
				// 创建2条记录：一条已推送，一条未推送
				unpushed := models.TorrentInfo{
					SiteName:     "test-site",
					TorrentID:    "126",
					Title:        "Unpushed Torrent",
					IsPushed:     boolPtr(false),
					IsFree:       true,
					IsDownloaded: false,
				}
				pushed := models.TorrentInfo{
					SiteName:     "test-site",
					TorrentID:    "127",
					Title:        "Pushed Torrent",
					IsPushed:     boolPtr(true),
					IsFree:       true,
					IsDownloaded: false,
				}
				if err := db.Create(&unpushed).Error; err != nil {
					t.Fatalf("failed to create unpushed torrent: %v", err)
				}
				if err := db.Create(&pushed).Error; err != nil {
					t.Fatalf("failed to create pushed torrent: %v", err)
				}
			},
			request: &DeleteTasksRequest{IDs: []uint{1, 2}},
			checkResp: func(t *testing.T, resp *http.Response, body *DeleteTasksResponse) {
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				// Query filters is_pushed != true, so pushed record (ID:2) is not returned
				// Only unpushed record (ID:1) is deleted
				assert.Equal(t, 1, body.Success)
				assert.Equal(t, 0, body.Failed)
				assert.Empty(t, body.FailedIDs)
				assert.Empty(t, body.FailedErrors)

				// Verify pushed record still exists
				var count int64
				global.GlobalDB.DB.Model(&models.TorrentInfo{}).Count(&count)
				assert.Equal(t, int64(1), count) // Only pushed record remains
			},
		},
		{
			name: "Delete with empty IDs array",
			setup: func(t *testing.T, db *gorm.DB) {
				// Create 2 unpushed records
				torrents := []models.TorrentInfo{
					{
						SiteName:     "test-site",
						TorrentID:    "128",
						Title:        "Test Torrent 1",
						IsPushed:     boolPtr(false),
						IsFree:       true,
						IsDownloaded: false,
					},
					{
						SiteName:     "test-site",
						TorrentID:    "129",
						Title:        "Test Torrent 2",
						IsPushed:     boolPtr(false),
						IsFree:       false,
						IsDownloaded: false,
					},
				}
				for _, torr := range torrents {
					if err := db.Create(&torr).Error; err != nil {
						t.Fatalf("failed to create test torrent: %v", err)
					}
				}
			},
			request: &DeleteTasksRequest{IDs: []uint{}},
			checkResp: func(t *testing.T, resp *http.Response, body *DeleteTasksResponse) {
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				// Empty IDs means delete ALL unpushed records (no ID filter)
				assert.Equal(t, 2, body.Success)
				assert.Equal(t, 0, body.Failed)
				assert.Empty(t, body.FailedIDs)
				assert.Empty(t, body.FailedErrors)

				// Verify all records deleted
				var count int64
				global.GlobalDB.DB.Model(&models.TorrentInfo{}).Count(&count)
				assert.Equal(t, int64(0), count)
			},
		},
		{
			name: "Delete record with nil IsPushed (default value)",
			setup: func(t *testing.T, db *gorm.DB) {
				// 创建 IsPushed 为 nil 的记录（真实场景默认值）
				torrent := models.TorrentInfo{
					SiteName:     "test-site",
					TorrentID:    "130",
					Title:        "Nil IsPushed Torrent",
					IsPushed:     nil, // 未设置，数据库中为 NULL
					IsFree:       true,
					IsDownloaded: false,
				}
				if err := db.Create(&torrent).Error; err != nil {
					t.Fatalf("failed to create test torrent: %v", err)
				}
			},
			request: &DeleteTasksRequest{IDs: []uint{1}},
			checkResp: func(t *testing.T, resp *http.Response, body *DeleteTasksResponse) {
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				assert.Equal(t, 1, body.Success)
				assert.Equal(t, 0, body.Failed)
				assert.Empty(t, body.FailedIDs)
				assert.Empty(t, body.FailedErrors)

				// 验证数据库中记录已删除
				var count int64
				global.GlobalDB.DB.Model(&models.TorrentInfo{}).Where("id = ?", 1).Count(&count)
				assert.Equal(t, int64(0), count)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 建立测试服务器和数据库
			server, db := setupTestServer(t)

			// 迁移TorrentInfo表
			if err := db.AutoMigrate(&models.TorrentInfo{}); err != nil {
				t.Fatalf("failed to migrate table: %v", err)
			}

			// 设置测试数据
			tt.setup(t, db)

			// 准备请求
			reqBody, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPost, "/api/tasks/batch-delete", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// 执行API调用
			server.apiDeleteTasks(w, req)

			// 解析响应
			var respBody DeleteTasksResponse
			err := json.NewDecoder(w.Body).Decode(&respBody)
			if err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			// 验证响应
			tt.checkResp(t, w.Result(), &respBody)
		})
	}
}

// boolPtr 辅助函数，将bool转换为*bool
func boolPtr(b bool) *bool {
	return &b
}

// ==== merged from api_torrent_mgmt_cov2_test.go ====
func createFilterRuleForCov(t *testing.T, server *Server, name string) uint {
	t.Helper()
	body, _ := json.Marshal(FilterRuleRequest{
		Name: name, Pattern: "foo*", PatternType: "wildcard", Enabled: true, Priority: 5,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
	server.apiFilterRules(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp FilterRuleResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp.ID
}

func TestUpdateFilterRule_FullPaths(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	id := createFilterRuleForCov(t, server, "RuleUpd")

	t.Run("update rename pattern matchfield and disable", func(t *testing.T) {
		body, _ := json.Marshal(FilterRuleRequest{
			Name: "RuleUpd2", Pattern: "bar*", PatternType: "wildcard",
			MatchField: "title", Enabled: false, Priority: 9,
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/filter-rules/1", bytes.NewReader(body))
		server.updateFilterRule(w, req, id)
		require.Equal(t, http.StatusOK, w.Code)
		var resp FilterRuleResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "RuleUpd2", resp.Name)
		assert.False(t, resp.Enabled)
	})

	t.Run("update not found", func(t *testing.T) {
		body, _ := json.Marshal(FilterRuleRequest{Name: "X", Pattern: "y*", PatternType: "wildcard", Enabled: true})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/filter-rules/999", bytes.NewReader(body))
		server.updateFilterRule(w, req, 999)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("update invalid body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/filter-rules/1", bytes.NewBufferString(`{bad`))
		server.updateFilterRule(w, req, id)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("update invalid pattern", func(t *testing.T) {
		body, _ := json.Marshal(FilterRuleRequest{
			Name: "RuleUpd2", Pattern: "[invalid(regex", PatternType: "regex", Enabled: true,
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/filter-rules/1", bytes.NewReader(body))
		server.updateFilterRule(w, req, id)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("update duplicate name conflict", func(t *testing.T) {
		otherID := createFilterRuleForCov(t, server, "OtherRule")
		body, _ := json.Marshal(FilterRuleRequest{
			Name: "RuleUpd2", Pattern: "z*", PatternType: "wildcard", Enabled: true,
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/filter-rules/1", bytes.NewReader(body))
		server.updateFilterRule(w, req, otherID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("update invalid match field", func(t *testing.T) {
		body, _ := json.Marshal(FilterRuleRequest{
			Name: "RuleUpd2", Pattern: "bar*", PatternType: "wildcard",
			MatchField: "bogus", Enabled: true,
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/filter-rules/1", bytes.NewReader(body))
		server.updateFilterRule(w, req, id)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestApiResumeTorrent_Paths(t *testing.T) {
	server, _ := setupServerWithFakeDownloader(t, &fakeDownloader{})
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.TorrentInfo{}))

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/torrents/1/resume", nil)
		server.apiResumeTorrent(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("invalid id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/abc/resume", nil)
		server.apiResumeTorrent(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/999/resume", nil)
		server.apiResumeTorrent(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("not paused by system", func(t *testing.T) {
		ti := models.TorrentInfo{SiteName: "s", TorrentID: "t1", IsPausedBySystem: false}
		require.NoError(t, global.GlobalDB.DB.Create(&ti).Error)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/1/resume", nil)
		server.apiResumeTorrent(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp ResumeTorrentResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.False(t, resp.Success)
	})

	t.Run("missing downloader info", func(t *testing.T) {
		ti := models.TorrentInfo{SiteName: "s", TorrentID: "t2", IsPausedBySystem: true}
		require.NoError(t, global.GlobalDB.DB.Create(&ti).Error)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/2/resume", nil)
		server.apiResumeTorrent(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("resume success via fake downloader", func(t *testing.T) {
		ti := models.TorrentInfo{
			SiteName: "s", TorrentID: "t3", IsPausedBySystem: true,
			DownloaderTaskID: "task3", DownloaderName: "qb1",
		}
		require.NoError(t, global.GlobalDB.DB.Create(&ti).Error)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/3/resume", nil)
		server.apiResumeTorrent(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp ResumeTorrentResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.True(t, resp.Success)
	})
}

func TestApiDeleteTasks_DeletesUnpushed(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}))

	pushed := true
	require.NoError(t, db.Create(&models.TorrentInfo{SiteName: "s", TorrentID: "a", IsPushed: &pushed}).Error)
	require.NoError(t, db.Create(&models.TorrentInfo{SiteName: "s", TorrentID: "b"}).Error)

	body, _ := json.Marshal(DeleteTasksRequest{IDs: []uint{2}})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tasks/batch-delete", bytes.NewReader(body))
	server.apiDeleteTasks(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp DeleteTasksResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.Success)
}

// ==== merged from api_torrent_mgmt_cov6_test.go ====
func closedTorrentServer(t *testing.T) *Server {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}, &models.TorrentInfoArchive{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	prev := global.GlobalDB
	global.GlobalDB = &models.TorrentDB{DB: db}
	t.Cleanup(func() { global.GlobalDB = prev })
	return &Server{sessions: map[string]string{"sess-test": "admin"}}
}

func TestApiPausedTorrents_DBError(t *testing.T) {
	srv := closedTorrentServer(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/torrents/paused", nil)
	srv.apiPausedTorrents(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestApiArchiveTorrents_DBError(t *testing.T) {
	srv := closedTorrentServer(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/torrents/archive", nil)
	srv.apiArchiveTorrents(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestApiDeleteTasks_DBError(t *testing.T) {
	srv := closedTorrentServer(t)
	body, _ := json.Marshal(DeleteTasksRequest{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tasks/batch-delete", bytes.NewReader(body))
	srv.apiDeleteTasks(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestApiDeletePausedTorrents_DBError(t *testing.T) {
	srv := closedTorrentServer(t)
	body, _ := json.Marshal(DeletePausedRequest{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/torrents/delete-paused", bytes.NewReader(body))
	srv.apiDeletePausedTorrents(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
