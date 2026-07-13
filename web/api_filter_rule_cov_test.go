package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/filter"
	"github.com/sunerpy/pt-tools/models"
)

func TestApiFilterRuleDetail_Dispatch(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	t.Run("invalid id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/filter-rules/abc", nil)
		server.apiFilterRuleDetail(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/filter-rules/999", nil)
		server.apiFilterRuleDetail(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/api/filter-rules/1", nil)
		server.apiFilterRuleDetail(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("test dispatch bad body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewBufferString(`{bad`))
		server.apiFilterRuleDetail(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestApiFilterRuleDetail_CRUD(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	body, _ := json.Marshal(FilterRuleRequest{
		Name: "Rule1", Pattern: "foo*", PatternType: "wildcard", Enabled: true, Priority: 10,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
	server.apiFilterRules(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var created FilterRuleResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))

	t.Run("get detail", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/filter-rules/1", nil)
		server.apiFilterRuleDetail(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp FilterRuleResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "Rule1", resp.Name)
	})

	t.Run("update -> disabled", func(t *testing.T) {
		ub, _ := json.Marshal(FilterRuleRequest{
			Name: "Rule1", Pattern: "bar*", PatternType: "wildcard", Enabled: false, Priority: 20,
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/filter-rules/1", bytes.NewReader(ub))
		server.apiFilterRuleDetail(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp FilterRuleResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.False(t, resp.Enabled)
		assert.Equal(t, "bar*", resp.Pattern)
	})

	t.Run("delete", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/filter-rules/1", nil)
		server.apiFilterRuleDetail(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("delete not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/filter-rules/1", nil)
		server.apiFilterRuleDetail(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestMatchesField(t *testing.T) {
	m, err := filter.NewMatcher(filter.PatternKeyword, "movie")
	require.NoError(t, err)

	assert.True(t, matchesField(m, models.MatchFieldTitle, "A movie", "tag"))
	assert.False(t, matchesField(m, models.MatchFieldTitle, "A show", "movie"))
	assert.True(t, matchesField(m, models.MatchFieldTag, "A show", "movie"))
	assert.True(t, matchesField(m, models.MatchFieldBoth, "A show", "movie"))
	assert.False(t, matchesField(m, models.MatchFieldBoth, "A show", "tv"))
}

func TestValidatePattern(t *testing.T) {
	assert.NoError(t, validatePattern(models.PatternKeyword, "foo"))
	assert.NoError(t, validatePattern(models.PatternWildcard, "foo*"))
	assert.NoError(t, validatePattern(models.PatternRegex, "^foo.*$"))
	assert.Error(t, validatePattern(models.PatternRegex, "[invalid("))
}

func TestSanitizeRuleSize(t *testing.T) {
	assert.Equal(t, 0, sanitizeRuleSize(-5))
	assert.Equal(t, 0, sanitizeRuleSize(0))
	assert.Equal(t, 7, sanitizeRuleSize(7))
}

func TestDecisionLabel(t *testing.T) {
	assert.Equal(t, "downloaded", decisionLabel(true))
	assert.Equal(t, "skipped", decisionLabel(false))
}

func TestBytesToGB(t *testing.T) {
	assert.Equal(t, 0.0, bytesToGB(0))
	assert.Equal(t, 0.0, bytesToGB(-1))
	assert.InDelta(t, 1.0, bytesToGB(1024*1024*1024), 0.001)
}

func TestEvaluateTestDecision(t *testing.T) {
	rule := &models.FilterRule{Pattern: "x", RequireFree: false, Enabled: true}

	t.Run("global size exceeded", func(t *testing.T) {
		d := evaluateTestDecision(rule, models.FilterModeAutoFree, 10, 20, true)
		assert.False(t, d.ShouldDownload)
		assert.Equal(t, filter.SourceNone, d.Source)
	})

	t.Run("free only + free", func(t *testing.T) {
		d := evaluateTestDecision(rule, models.FilterModeFreeOnly, 0, 5, true)
		assert.True(t, d.ShouldDownload)
		assert.Equal(t, filter.SourceFreeDownload, d.Source)
	})

	t.Run("free only + not free", func(t *testing.T) {
		d := evaluateTestDecision(rule, models.FilterModeFreeOnly, 0, 5, false)
		assert.False(t, d.ShouldDownload)
	})

	t.Run("require free but not free", func(t *testing.T) {
		freeRule := &models.FilterRule{Pattern: "x", RequireFree: true, Enabled: true}
		d := evaluateTestDecision(freeRule, models.FilterModeAutoFree, 0, 5, false)
		assert.False(t, d.ShouldDownload)
		assert.Equal(t, freeRule, d.MatchedRule)
	})

	t.Run("matched and downloadable", func(t *testing.T) {
		d := evaluateTestDecision(rule, models.FilterModeAutoFree, 0, 5, true)
		assert.True(t, d.ShouldDownload)
		assert.Equal(t, filter.SourceFilterRule, d.Source)
	})
}

func TestTestFilterRuleWithRSS_NotFound(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	m, err := filter.NewMatcher(filter.PatternKeyword, "x")
	require.NoError(t, err)

	w := httptest.NewRecorder()
	server.testFilterRuleWithRSS(w, m, models.MatchFieldBoth, false, 999, 20)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
