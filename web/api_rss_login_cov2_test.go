package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

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
