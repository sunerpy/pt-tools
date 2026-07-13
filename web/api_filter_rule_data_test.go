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

func TestTestFilterRule_DBTorrents(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	torrents := []models.TorrentInfo{
		{SiteName: "hdsky", TorrentID: "1", Title: "Cool Movie 2024", Tag: "movie", IsFree: true, TorrentSize: 5 * 1024 * 1024 * 1024},
		{SiteName: "hdsky", TorrentID: "2", Title: "Another Show", Tag: "tv", IsFree: false, TorrentSize: 2 * 1024 * 1024 * 1024},
		{SiteName: "hdsky", TorrentID: "3", Title: "Cool Series", Tag: "tv", IsFree: true, TorrentSize: 1 * 1024 * 1024 * 1024},
	}
	for _, tr := range torrents {
		require.NoError(t, global.GlobalDB.DB.Create(&tr).Error)
	}

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/filter-rules/test", nil)
		server.testFilterRule(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("empty pattern", func(t *testing.T) {
		body, _ := json.Marshal(FilterRuleTestRequest{Pattern: ""})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		server.testFilterRule(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid regex pattern", func(t *testing.T) {
		body, _ := json.Marshal(FilterRuleTestRequest{Pattern: "[bad(", PatternType: "regex"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		server.testFilterRule(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("keyword match against DB torrents", func(t *testing.T) {
		body, _ := json.Marshal(FilterRuleTestRequest{
			Pattern:     "Cool",
			PatternType: "keyword",
			MatchField:  "title",
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		server.testFilterRule(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleTestResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 2, resp.MatchCount)
	})

	t.Run("free-only filter with size override", func(t *testing.T) {
		body, _ := json.Marshal(FilterRuleTestRequest{
			Pattern:     "Cool",
			PatternType: "keyword",
			MatchField:  "title",
			RequireFree: true,
			TestIsFree:  boolPtr(true),
			TestSizeGB:  3,
			FilterMode:  "free_only",
			Limit:       5,
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		server.testFilterRule(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleTestResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.GreaterOrEqual(t, resp.MatchCount, 1)
	})
}
