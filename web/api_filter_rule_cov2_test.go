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

func TestCreateFilterRule_ValidationBranches(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	t.Run("empty name", func(t *testing.T) {
		body, _ := json.Marshal(FilterRuleRequest{Pattern: "x*", PatternType: "wildcard"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
		server.createFilterRule(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("empty pattern", func(t *testing.T) {
		body, _ := json.Marshal(FilterRuleRequest{Name: "n"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
		server.createFilterRule(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("unsupported pattern type", func(t *testing.T) {
		body, _ := json.Marshal(FilterRuleRequest{Name: "n", Pattern: "x", PatternType: "bogus"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
		server.createFilterRule(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid regex pattern", func(t *testing.T) {
		body, _ := json.Marshal(FilterRuleRequest{Name: "n", Pattern: "[bad(", PatternType: "regex"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
		server.createFilterRule(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("bad match field", func(t *testing.T) {
		body, _ := json.Marshal(FilterRuleRequest{Name: "n", Pattern: "x*", PatternType: "wildcard", MatchField: "bogus"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
		server.createFilterRule(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("duplicate name", func(t *testing.T) {
		b1, _ := json.Marshal(FilterRuleRequest{Name: "dup", Pattern: "x*", PatternType: "wildcard", Enabled: true})
		w1 := httptest.NewRecorder()
		server.createFilterRule(w1, httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(b1)))
		require.Equal(t, http.StatusOK, w1.Code)

		b2, _ := json.Marshal(FilterRuleRequest{Name: "dup", Pattern: "y*", PatternType: "wildcard", Enabled: true})
		w2 := httptest.NewRecorder()
		server.createFilterRule(w2, httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(b2)))
		assert.Equal(t, http.StatusBadRequest, w2.Code)
	})

	t.Run("bad body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewBufferString(`{bad`))
		server.createFilterRule(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestTestFilterRule_WithSeededTorrents(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	require.NoError(t, global.GlobalDB.DB.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "1", Title: "Foo Movie 2024", Tag: "hd", IsFree: true, TorrentSize: 5 << 30,
	}).Error)
	require.NoError(t, global.GlobalDB.DB.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "2", Title: "Bar Show", Tag: "sd", IsFree: false, TorrentSize: 1 << 30,
	}).Error)

	t.Run("matches title pattern", func(t *testing.T) {
		body, _ := json.Marshal(FilterRuleTestRequest{
			Pattern: "Foo*", PatternType: "wildcard", MatchField: "title",
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		server.testFilterRule(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp FilterRuleTestResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp.MatchCount)
	})

	t.Run("test with size and free override", func(t *testing.T) {
		free := false
		body, _ := json.Marshal(FilterRuleTestRequest{
			Pattern: "*", PatternType: "wildcard", MatchField: "both",
			TestSizeGB: 3.5, TestIsFree: &free, RequireFree: true,
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		server.testFilterRule(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("empty pattern", func(t *testing.T) {
		body, _ := json.Marshal(FilterRuleTestRequest{Pattern: ""})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		server.testFilterRule(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid pattern", func(t *testing.T) {
		body, _ := json.Marshal(FilterRuleTestRequest{Pattern: "[bad(", PatternType: "regex"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		server.testFilterRule(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/filter-rules/test", nil)
		server.testFilterRule(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}
