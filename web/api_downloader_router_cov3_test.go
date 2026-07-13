package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

func TestApiDownloaderRouter_DispatchCov3(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.DownloaderDirectory{}))
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "rdl", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true, IsDefault: true,
	}).Error)
	mgr := scheduler.NewManager()
	t.Cleanup(func() { mgr.StopAll() })
	server.mgr = mgr

	t.Run("directories list route", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/1/directories", nil)
		server.apiDownloaderRouter(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("directory detail route", func(t *testing.T) {
		require.NoError(t, db.Create(&models.DownloaderDirectory{
			DownloaderID: 1, Path: "/d/a", IsDefault: true,
		}).Error)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/1/directories/1", strings.NewReader(`{"path":"/d/b"}`))
		server.apiDownloaderRouter(w, req)
		assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusBadRequest)
	})

	t.Run("apply-to-sites route", func(t *testing.T) {
		require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)
		body, _ := json.Marshal(ApplyDownloaderToSitesRequest{SiteIDs: []uint{1}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/apply-to-sites", bytes.NewReader(body))
		server.apiDownloaderRouter(w, req)
		assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
	})

	t.Run("plain detail route", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/1", nil)
		server.apiDownloaderRouter(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestApiDownloaderDirectoryDetail_Routing(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.DownloaderDirectory{}))
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "rdl", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&models.DownloaderDirectory{
		DownloaderID: 1, Path: "/d/a", IsDefault: true,
	}).Error)

	t.Run("invalid path", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/1/other/1", nil)
		server.apiDownloaderDirectoryDetail(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid downloader id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/abc/directories/1", nil)
		server.apiDownloaderDirectoryDetail(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("set-default route", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/directories/1/set-default", nil)
		server.apiDownloaderDirectoryDetail(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("set-default invalid dir id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/directories/abc/set-default", nil)
		server.apiDownloaderDirectoryDetail(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid dir id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/1/directories/abc", nil)
		server.apiDownloaderDirectoryDetail(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("delete route", func(t *testing.T) {
		require.NoError(t, db.Create(&models.DownloaderDirectory{
			DownloaderID: 1, Path: "/d/b",
		}).Error)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/downloaders/1/directories/2", nil)
		server.apiDownloaderDirectoryDetail(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/api/downloaders/1/directories/1", nil)
		server.apiDownloaderDirectoryDetail(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApiUserInfoAggregated_NoService(t *testing.T) {
	InitUserInfoService(nil)
	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/aggregated", nil)
	s.apiUserInfoAggregated(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

var _ = global.GlobalDB
