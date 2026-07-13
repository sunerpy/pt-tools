package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

func newDownloaderCovServer(t *testing.T) *Server {
	t.Helper()
	server, _ := setupTestServer(t)
	mgr := scheduler.NewManager()
	t.Cleanup(func() { mgr.StopAll() })
	server.mgr = mgr
	return server
}

func TestCreateDownloader_Branches(t *testing.T) {
	server := newDownloaderCovServer(t)

	t.Run("bad body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewBufferString(`{bad`))
		server.createDownloader(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("empty name", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{Type: "qbittorrent", URL: "http://x"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
		server.createDownloader(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("empty type", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{Name: "n", URL: "http://x"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
		server.createDownloader(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("bad type", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{Name: "n", Type: "aria2", URL: "http://x"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
		server.createDownloader(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("empty url", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{Name: "n", Type: "qbittorrent"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
		server.createDownloader(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("bad url", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{Name: "n", Type: "qbittorrent", URL: "ftp://x"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
		server.createDownloader(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("first downloader becomes default", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{Name: "first-dl", Type: "qbittorrent", URL: "127.0.0.1:8080", Enabled: true})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
		server.createDownloader(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp DownloaderResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.True(t, resp.IsDefault)
	})
}

func TestCreateDownloader_DuplicateName(t *testing.T) {
	server := newDownloaderCovServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.DownloaderSetting{
		Name: "dup-dl", Type: "qbittorrent", URL: "http://127.0.0.1:7070", Enabled: true,
	}).Error)
	body, _ := json.Marshal(DownloaderRequest{Name: "dup-dl", Type: "qbittorrent", URL: "http://127.0.0.1:9090"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
	server.createDownloader(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateDownloader_Branches(t *testing.T) {
	server := newDownloaderCovServer(t)
	db := global.GlobalDB.DB

	def := models.DownloaderSetting{Name: "def", Type: "qbittorrent", URL: "http://127.0.0.1:1", IsDefault: true, Enabled: true}
	require.NoError(t, db.Create(&def).Error)
	other := models.DownloaderSetting{Name: "other", Type: "qbittorrent", URL: "http://127.0.0.1:2", Enabled: true}
	require.NoError(t, db.Create(&other).Error)

	t.Run("bad body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/1", bytes.NewBufferString(`{bad`))
		server.updateDownloader(w, req, 1)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("not found", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{Name: "x", Type: "qbittorrent", URL: "http://x", Enabled: true})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/999", bytes.NewReader(body))
		server.updateDownloader(w, req, 999)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("cannot unset only default", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{Name: "def", Type: "qbittorrent", URL: "http://127.0.0.1:1", IsDefault: false, Enabled: true})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/1", bytes.NewReader(body))
		server.updateDownloader(w, req, def.ID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("bad url on update", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{Name: "other", Type: "qbittorrent", URL: "ftp://bad", Enabled: true})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/2", bytes.NewReader(body))
		server.updateDownloader(w, req, other.ID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("promote other to default", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{Name: "other", Type: "qbittorrent", URL: "http://127.0.0.1:2", IsDefault: true, Enabled: true})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/2", bytes.NewReader(body))
		server.updateDownloader(w, req, other.ID)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestDeleteDownloader_Branches(t *testing.T) {
	server := newDownloaderCovServer(t)
	db := global.GlobalDB.DB

	def := models.DownloaderSetting{Name: "d1", Type: "qbittorrent", URL: "http://127.0.0.1:1", IsDefault: true, Enabled: true}
	require.NoError(t, db.Create(&def).Error)
	extra := models.DownloaderSetting{Name: "d2", Type: "qbittorrent", URL: "http://127.0.0.1:2", Enabled: true}
	require.NoError(t, db.Create(&extra).Error)

	t.Run("not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/downloaders/999", nil)
		server.deleteDownloader(w, req, 999)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("cannot delete default with others", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/downloaders/1", nil)
		server.deleteDownloader(w, req, def.ID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("delete non-default succeeds", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/downloaders/2", nil)
		server.deleteDownloader(w, req, extra.ID)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestListDownloaders_WithData(t *testing.T) {
	server := newDownloaderCovServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.DownloaderSetting{
		Name: "ld1", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true,
	}).Error)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/downloaders", nil)
	server.listDownloaders(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp []DownloaderResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.GreaterOrEqual(t, len(resp), 1)
}
