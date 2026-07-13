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

func TestApiSiteLoginStateRouter_Dispatch(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{
		Name: "hdsky", DisplayName: "HDSky", Enabled: true, BaseURL: "https://hdsky.me",
	}).Error)

	t.Run("empty site name", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiSiteLoginStateRouter(rec, authedRequest(http.MethodGet, "/api/sites/login-state/", nil))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("get login state", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiSiteLoginStateRouter(rec, authedRequest(http.MethodGet, "/api/sites/login-state/hdsky", nil))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("unknown action", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiSiteLoginStateRouter(rec, authedRequest(http.MethodGet, "/api/sites/login-state/hdsky/bogus", nil))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("config action bad body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", bytes.NewBufferString(`{bad`))
		req.AddCookie(&http.Cookie{Name: "session", Value: "sess-test"})
		rec := httptest.NewRecorder()
		srv.apiSiteLoginStateRouter(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestApiSiteLoginStateVisit_Cov(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{
		Name: "hdsky", DisplayName: "HDSky", Enabled: true,
	}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiSiteLoginStateVisit(rec, authedRequest(http.MethodGet, "/api/sites/visit", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("invalid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/sites/visit", bytes.NewBufferString(`{bad`))
		req.AddCookie(&http.Cookie{Name: "session", Value: "sess-test"})
		rec := httptest.NewRecorder()
		srv.apiSiteLoginStateVisit(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("empty site name", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiSiteLoginStateVisit(rec, authedRequest(http.MethodPost, "/api/sites/visit", SiteVisitReportRequest{LastVisitAt: "2024-01-01T00:00:00Z"}))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("bad timestamp", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiSiteLoginStateVisit(rec, authedRequest(http.MethodPost, "/api/sites/visit", SiteVisitReportRequest{SiteName: "hdsky", LastVisitAt: "not-a-time"}))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("valid visit", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiSiteLoginStateVisit(rec, authedRequest(http.MethodPost, "/api/sites/visit", SiteVisitReportRequest{SiteName: "hdsky", LastVisitAt: "2024-01-01T00:00:00Z"}))
		require.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestGetDownloader_Cov(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb1", Type: "qbittorrent", URL: "http://localhost:8080", Enabled: true,
	}).Error)

	t.Run("found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/1", nil)
		server.getDownloader(w, req, 1)
		require.Equal(t, http.StatusOK, w.Code)

		var resp DownloaderResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "qb1", resp.Name)
	})

	t.Run("not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/999", nil)
		server.getDownloader(w, req, 999)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestUpdateDownloader_Cov(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb1", Type: "qbittorrent", URL: "http://localhost:8080", Enabled: true, IsDefault: true,
	}).Error)
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb2", Type: "qbittorrent", URL: "http://localhost:8081", Enabled: true,
	}).Error)

	t.Run("invalid body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/1", bytes.NewBufferString(`{bad`))
		server.updateDownloader(w, req, 1)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("not found", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{Name: "x", Type: "qbittorrent", URL: "http://h", Enabled: true})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/999", bytes.NewReader(body))
		server.updateDownloader(w, req, 999)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("update qb2 fields", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{
			Name: "qb2-renamed", Type: "qbittorrent", URL: "http://localhost:8082", Enabled: true, AutoStart: true,
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/2", bytes.NewReader(body))
		server.updateDownloader(w, req, 2)
		require.Equal(t, http.StatusOK, w.Code)

		var resp DownloaderResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "qb2-renamed", resp.Name)
	})
}

func TestDeleteDownloader_Cov(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb1", Type: "qbittorrent", URL: "http://localhost:8080", Enabled: true, IsDefault: true,
	}).Error)
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb2", Type: "qbittorrent", URL: "http://localhost:8081", Enabled: true,
	}).Error)

	t.Run("cannot delete default when others exist", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/downloaders/1", nil)
		server.deleteDownloader(w, req, 1)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("delete non-default", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/downloaders/2", nil)
		server.deleteDownloader(w, req, 2)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("delete non-existent", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/downloaders/999", nil)
		server.deleteDownloader(w, req, 999)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestSetDefaultDownloader_Cov(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb1", Type: "qbittorrent", URL: "http://localhost:8080", Enabled: true, IsDefault: true,
	}).Error)
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb2", Type: "qbittorrent", URL: "http://localhost:8081", Enabled: false,
	}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/2/set-default", nil)
		server.setDefaultDownloader(w, req, "2")
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("invalid id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/abc/set-default", nil)
		server.setDefaultDownloader(w, req, "abc")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/999/set-default", nil)
		server.setDefaultDownloader(w, req, "999")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("set qb2 default", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/2/set-default", nil)
		server.setDefaultDownloader(w, req, "2")
		require.Equal(t, http.StatusOK, w.Code)

		var resp DownloaderResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.True(t, resp.IsDefault)
		assert.True(t, resp.Enabled)
	})
}

func TestListAndCreateDownloader_Cov(t *testing.T) {
	server, _ := setupTestServer(t)

	t.Run("list empty", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders", nil)
		server.listDownloaders(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	tests := []struct {
		name       string
		req        DownloaderRequest
		wantStatus int
	}{
		{"missing name", DownloaderRequest{Type: "qbittorrent", URL: "http://h"}, http.StatusBadRequest},
		{"missing type", DownloaderRequest{Name: "x", URL: "http://h"}, http.StatusBadRequest},
		{"bad type", DownloaderRequest{Name: "x", Type: "bogus", URL: "http://h"}, http.StatusBadRequest},
		{"missing url", DownloaderRequest{Name: "x", Type: "qbittorrent"}, http.StatusBadRequest},
		{"bad url", DownloaderRequest{Name: "x", Type: "qbittorrent", URL: "ftp://h"}, http.StatusBadRequest},
		{"valid", DownloaderRequest{Name: "qbNew", Type: "qbittorrent", URL: "localhost:9000", Enabled: true}, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.req)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
			server.createDownloader(w, req)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}
