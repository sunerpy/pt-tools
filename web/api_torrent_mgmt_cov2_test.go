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
