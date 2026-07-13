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
)

func setupDirServer(t *testing.T) (*Server, uint) {
	t.Helper()
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.DownloaderDirectory{}))
	dl := models.DownloaderSetting{Name: "dirdl", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true}
	require.NoError(t, db.Create(&dl).Error)
	return server, dl.ID
}

func TestDownloaderDirectory_CRUD(t *testing.T) {
	server, _ := setupDirServer(t)

	t.Run("list empty", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/1/directories", nil)
		server.apiDownloaderDirectories(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("create first becomes default", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderDirectoryRequest{Path: "/data/a", Alias: "A"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/directories", bytes.NewReader(body))
		server.apiDownloaderDirectories(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp DownloaderDirectoryResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.True(t, resp.IsDefault)
	})

	t.Run("create empty path rejected", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderDirectoryRequest{})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/directories", bytes.NewReader(body))
		server.apiDownloaderDirectories(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("create duplicate path rejected", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderDirectoryRequest{Path: "/data/a"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/directories", bytes.NewReader(body))
		server.apiDownloaderDirectories(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("create second", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderDirectoryRequest{Path: "/data/b", Alias: "B"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/directories", bytes.NewReader(body))
		server.apiDownloaderDirectories(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("update dir", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderDirectoryRequest{Path: "/data/b2", Alias: "B2", IsDefault: true})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/1/directories/2", bytes.NewReader(body))
		server.updateDownloaderDirectory(w, req, 1, 2)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("set default", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/directories/1/set-default", nil)
		server.setDefaultDirectory(w, req, 1, 1)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("delete non-default", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/downloaders/1/directories/2", nil)
		server.deleteDownloaderDirectory(w, req, 1, 2)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestDownloaderDirectory_ErrorPaths(t *testing.T) {
	server, _ := setupDirServer(t)

	t.Run("list nonexistent downloader 404", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/999/directories", nil)
		server.apiDownloaderDirectories(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("update nonexistent dir 404", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderDirectoryRequest{Path: "/x"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/1/directories/999", bytes.NewReader(body))
		server.updateDownloaderDirectory(w, req, 1, 999)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("delete nonexistent dir 404", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/downloaders/1/directories/999", nil)
		server.deleteDownloaderDirectory(w, req, 1, 999)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("set default nonexistent 404", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/directories/999/set-default", nil)
		server.setDefaultDirectory(w, req, 1, 999)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestApiAllDownloaderDirectories_Cov(t *testing.T) {
	server, id := setupDirServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.DownloaderDirectory{
		DownloaderID: id, Path: "/data/x", IsDefault: true,
	}).Error)

	t.Run("get all", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/all-directories", nil)
		server.apiAllDownloaderDirectories(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/all-directories", nil)
		server.apiAllDownloaderDirectories(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}
