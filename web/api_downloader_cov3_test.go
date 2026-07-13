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

func TestApiDownloaders_Dispatch(t *testing.T) {
	server := newDownloaderCovServer(t)

	t.Run("get list", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders", nil)
		server.apiDownloaders(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/api/downloaders", nil)
		server.apiDownloaders(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApiDownloaderDetail_Dispatch(t *testing.T) {
	server := newDownloaderCovServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.DownloaderSetting{
		Name: "d1", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true, IsDefault: true,
	}).Error)

	t.Run("get detail", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/1", nil)
		server.apiDownloaderDetail(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("invalid id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/abc", nil)
		server.apiDownloaderDetail(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("get not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/999", nil)
		server.apiDownloaderDetail(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("health route", func(t *testing.T) {
		mgr := scheduler.NewManager()
		t.Cleanup(func() { mgr.StopAll() })
		server.mgr = mgr
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/1/health", nil)
		server.apiDownloaderDetail(w, req)
		assert.True(t, w.Code == http.StatusOK)
	})

	t.Run("set-default route", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/set-default", nil)
		server.apiDownloaderDetail(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/api/downloaders/1", nil)
		server.apiDownloaderDetail(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApiSiteRouter_Dispatch(t *testing.T) {
	s := &Server{}

	t.Run("unknown endpoint", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/site/hdsky/unknown", nil)
		s.apiSiteRouter(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("free-torrents list route", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/site/hdsky/free-torrents", nil)
		s.apiSiteRouter(w, req)
		assert.NotEqual(t, http.StatusNotFound, w.Code)
	})
}

func TestApiSiteDownloaderSummary_WithData(t *testing.T) {
	server, db := setupTestServer(t)
	dlID := uint(1)
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "sum-dl", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&models.SiteSetting{
		Name: "hdsky", DisplayName: "HDSky", Enabled: true, DownloaderID: &dlID,
	}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/downloader-summary", nil)
		server.apiSiteDownloaderSummary(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("get summary", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/downloader-summary", nil)
		server.apiSiteDownloaderSummary(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp SiteDownloaderSummaryResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.GreaterOrEqual(t, len(resp.Sites), 1)
	})
}

func TestApplyDownloaderToSites_Success(t *testing.T) {
	server := newDownloaderCovServer(t)
	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "apply-dl", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	body, _ := json.Marshal(ApplyDownloaderToSitesRequest{SiteIDs: []uint{1}})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/apply-to-sites", bytes.NewReader(body))
	server.applyDownloaderToSites(w, req, "1")
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)

	t.Run("empty site ids", func(t *testing.T) {
		b, _ := json.Marshal(ApplyDownloaderToSitesRequest{})
		ww := httptest.NewRecorder()
		rr := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/apply-to-sites", bytes.NewReader(b))
		server.applyDownloaderToSites(ww, rr, "1")
		assert.Equal(t, http.StatusOK, ww.Code)
	})

	t.Run("invalid id", func(t *testing.T) {
		ww := httptest.NewRecorder()
		rr := httptest.NewRequest(http.MethodPost, "/api/downloaders/abc/apply-to-sites", nil)
		server.applyDownloaderToSites(ww, rr, "abc")
		assert.Equal(t, http.StatusBadRequest, ww.Code)
	})

	t.Run("downloader not found", func(t *testing.T) {
		b, _ := json.Marshal(ApplyDownloaderToSitesRequest{SiteIDs: []uint{1}})
		ww := httptest.NewRecorder()
		rr := httptest.NewRequest(http.MethodPost, "/api/downloaders/999/apply-to-sites", bytes.NewReader(b))
		server.applyDownloaderToSites(ww, rr, "999")
		assert.Equal(t, http.StatusNotFound, ww.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		ww := httptest.NewRecorder()
		rr := httptest.NewRequest(http.MethodGet, "/api/downloaders/1/apply-to-sites", nil)
		server.applyDownloaderToSites(ww, rr, "1")
		assert.Equal(t, http.StatusMethodNotAllowed, ww.Code)
	})
}
