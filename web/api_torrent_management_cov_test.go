package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

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
