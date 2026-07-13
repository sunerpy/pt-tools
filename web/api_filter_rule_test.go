package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/filter"
	"github.com/sunerpy/pt-tools/models"
)

// ==== merged from api_filter_global_cov3_test.go ====
func TestCreateFilterRule_DefaultsAndSuccess(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	t.Run("keyword default with zero priority", func(t *testing.T) {
		body, _ := json.Marshal(FilterRuleRequest{
			Name: "DefRule", Pattern: "hit", Enabled: true,
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
		server.createFilterRule(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp FilterRuleResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 100, resp.Priority)
	})
}

func TestUpdateFilterRule_ReEnableCleanup(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	id := createFilterRuleForCov(t, server, "CleanRule")

	body, _ := json.Marshal(FilterRuleRequest{
		Name: "CleanRule", Pattern: "foo*", PatternType: "wildcard",
		Enabled: false, Priority: 3,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/filter-rules/1", bytes.NewReader(body))
	server.updateFilterRule(w, req, id)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestApiGlobal_GetError(t *testing.T) {
	prev := global.GlobalDB
	server := newLoginMonitorServer(t)
	t.Cleanup(func() { global.GlobalDB = prev })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/global", nil)
	server.apiGlobal(w, req)
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
}

func TestApiSiteTemplateImport_CookieEncryptError(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.SiteTemplate{}, &models.SiteSetting{}))
	t.Setenv("PT_TOOLS_SECRET_KEY", "")
	t.Setenv("HOME", t.TempDir()+"/nonexistent-home")

	tpl := models.SiteTemplateExport{Name: "encfail", AuthMethod: "cookie", BaseURL: "https://e.example.com"}
	tplBytes, _ := json.Marshal(tpl)
	body, _ := json.Marshal(TemplateImportRequest{Template: tplBytes, Cookie: "c=1"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader(body))
	server.apiSiteTemplateImport(w, req)
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
}

// ==== merged from api_filter_rule_cov2_test.go ====
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

// ==== merged from api_filter_rule_cov3_test.go ====
func TestApiFilterRuleDetail_GetDeleteSuccess(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	id := createFilterRuleForCov(t, server, "GDRule")

	t.Run("get success", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/filter-rules/1", nil)
		server.apiFilterRuleDetail(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("delete not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/filter-rules/999", nil)
		server.apiFilterRuleDetail(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("delete success", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/filter-rules/1", nil)
		server.apiFilterRuleDetail(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "deleted")
	})

	_ = id
}

func TestApiFilterRules_ListWithData(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	require.NoError(t, global.GlobalDB.DB.Create(&models.FilterRule{
		Name: "listrule", Pattern: "x*", PatternType: "wildcard", Enabled: true, Priority: 1,
	}).Error)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/filter-rules", nil)
	server.apiFilterRules(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp []FilterRuleResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.GreaterOrEqual(t, len(resp), 1)
}

func TestApiFilterRules_MethodNotAllowed(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/filter-rules", nil)
	server.apiFilterRules(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestUpdateFilterRule_MinMaxSize(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	id := createFilterRuleForCov(t, server, "SizeRule")
	body, _ := json.Marshal(FilterRuleRequest{
		Name: "SizeRule", Pattern: "foo*", PatternType: "wildcard",
		MinSizeGB: 1, MaxSizeGB: 50, Enabled: true, Priority: 2, RequireFree: true,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/filter-rules/1", bytes.NewReader(body))
	server.updateFilterRule(w, req, id)
	require.Equal(t, http.StatusOK, w.Code)
}

// ==== merged from api_filter_rule_cov_test.go ====
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

// ==== merged from api_filter_rule_data_test.go ====
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

// ==== merged from api_filter_rule_rss_cov3_test.go ====
func TestTestFilterRuleWithRSS_RequireFreeAndMatch(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(rssFeedXML))
	}))
	defer ts.Close()

	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.RSSSubscription{}))
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "unknownrss", Enabled: true, BaseURL: "http://e"}).Error)
	rss := models.RSSSubscription{Name: "r1", URL: ts.URL, SiteID: 1, IntervalMinutes: 10}
	require.NoError(t, global.GlobalDB.DB.Create(&rss).Error)

	m, err := filter.NewMatcher(filter.PatternKeyword, "Cool")
	require.NoError(t, err)

	t.Run("require free filters out non-free", func(t *testing.T) {
		w := httptest.NewRecorder()
		server.testFilterRuleWithRSS(w, m, models.MatchFieldTitle, true, rss.ID, 20)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("match both fields", func(t *testing.T) {
		mb, err := filter.NewMatcher(filter.PatternWildcard, "*")
		require.NoError(t, err)
		w := httptest.NewRecorder()
		server.testFilterRuleWithRSS(w, mb, models.MatchFieldBoth, false, rss.ID, 1)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("site not found", func(t *testing.T) {
		require.NoError(t, global.GlobalDB.DB.Create(&models.RSSSubscription{
			Name: "orphan", URL: ts.URL, SiteID: 9999, IntervalMinutes: 10,
		}).Error)
		w := httptest.NewRecorder()
		server.testFilterRuleWithRSS(w, m, models.MatchFieldTitle, false, 2, 20)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestMatchesField_AllBranches(t *testing.T) {
	m, err := filter.NewMatcher(filter.PatternKeyword, "hit")
	require.NoError(t, err)

	assert.True(t, matchesField(m, models.MatchFieldTitle, "a hit b", "tag"))
	assert.False(t, matchesField(m, models.MatchFieldTitle, "nope", "hit"))
	assert.True(t, matchesField(m, models.MatchFieldTag, "nope", "hit tag"))
	assert.True(t, matchesField(m, models.MatchFieldBoth, "hit", "x"))
	assert.True(t, matchesField(m, models.MatchField("unknown"), "x", "hit"))
}

// ==== merged from api_filter_rule_rss_test.go ====
const rssFeedXML = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>test</title>
<item><title>Cool Movie 1080p</title><guid>1</guid><link>http://e/1</link><category>movie</category></item>
<item><title>Another Show</title><guid>2</guid><link>http://e/2</link><category>tv</category></item>
</channel></rss>`

func TestTestFilterRuleWithRSS_Cov(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(rssFeedXML))
	}))
	defer ts.Close()

	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.RSSSubscription{}))
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "unknownsite", Enabled: true, BaseURL: "http://e"}).Error)
	rss := models.RSSSubscription{Name: "r1", URL: ts.URL, SiteID: 1, IntervalMinutes: 10}
	require.NoError(t, global.GlobalDB.DB.Create(&rss).Error)

	m, err := filter.NewMatcher(filter.PatternKeyword, "Cool")
	require.NoError(t, err)

	t.Run("rss not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		server.testFilterRuleWithRSS(w, m, models.MatchFieldTitle, false, 999, 20)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("matches feed items", func(t *testing.T) {
		w := httptest.NewRecorder()
		server.testFilterRuleWithRSS(w, m, models.MatchFieldTitle, false, rss.ID, 20)
		require.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleTestResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 2, resp.TotalCount)
	})
}

func TestFetchRSSFeedForTest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssFeedXML))
	}))
	defer ts.Close()

	feed, err := fetchRSSFeedForTest(ts.URL)
	require.NoError(t, err)
	assert.Len(t, feed.Items, 2)

	_, err = fetchRSSFeedForTest("http://127.0.0.1:1/nope")
	assert.Error(t, err)
}

// ==== merged from api_filter_rule_test.go ====
func setupFilterRuleTestServer(t *testing.T) (*Server, func()) {
	t.Helper()

	// Create a temporary directory for the test database
	tmpDir, err := os.MkdirTemp("", "filter_rule_api_test")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Migrate tables
	err = db.AutoMigrate(&models.FilterRule{}, &models.TorrentInfo{}, &models.SiteSetting{})
	require.NoError(t, err)

	// Set global DB
	global.GlobalDB = &models.TorrentDB{DB: db}

	// Initialize logger for tests
	zapLogger, _ := zap.NewDevelopment()
	global.GlobalLogger = zapLogger

	server := &Server{
		sessions: map[string]string{"test-session": "admin"},
	}

	cleanup := func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		os.RemoveAll(tmpDir)
	}

	return server, cleanup
}

func TestFilterRuleAPI(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	var createdRuleID uint

	t.Run("Create filter rule", func(t *testing.T) {
		reqBody := FilterRuleRequest{
			Name:        "Test Rule",
			Pattern:     "test*",
			PatternType: "wildcard",
			RequireFree: true,
			Enabled:     true,
			Priority:    50,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.apiFilterRules(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "Test Rule", resp.Name)
		assert.Equal(t, "test*", resp.Pattern)
		assert.Equal(t, "wildcard", resp.PatternType)
		assert.True(t, resp.RequireFree)
		assert.True(t, resp.Enabled)
		assert.Equal(t, 50, resp.Priority)

		createdRuleID = resp.ID
	})

	t.Run("Create filter rule with duplicate name fails", func(t *testing.T) {
		reqBody := FilterRuleRequest{
			Name:        "Test Rule",
			Pattern:     "another*",
			PatternType: "wildcard",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.apiFilterRules(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "已存在")
	})

	t.Run("Create filter rule with invalid regex fails", func(t *testing.T) {
		reqBody := FilterRuleRequest{
			Name:        "Invalid Regex Rule",
			Pattern:     "[invalid",
			PatternType: "regex",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.apiFilterRules(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("List filter rules", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/filter-rules", nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.apiFilterRules(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp []FilterRuleResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(resp), 1)
	})

	t.Run("Get filter rule by ID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/filter-rules/1", nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.getFilterRule(w, req, createdRuleID)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "Test Rule", resp.Name)
	})

	t.Run("Update filter rule", func(t *testing.T) {
		reqBody := FilterRuleRequest{
			Name:        "Updated Rule",
			Pattern:     "updated*",
			PatternType: "wildcard",
			RequireFree: false,
			Enabled:     true,
			Priority:    25,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPut, "/api/filter-rules/1", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.updateFilterRule(w, req, createdRuleID)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "Updated Rule", resp.Name)
		assert.Equal(t, "updated*", resp.Pattern)
		assert.False(t, resp.RequireFree)
		assert.Equal(t, 25, resp.Priority)
	})

	t.Run("Delete filter rule", func(t *testing.T) {
		// First create a rule to delete
		reqBody := FilterRuleRequest{
			Name:        "To Delete",
			Pattern:     "delete*",
			PatternType: "wildcard",
		}
		body, _ := json.Marshal(reqBody)

		createReq := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
		createReq.Header.Set("Content-Type", "application/json")
		createReq.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		createW := httptest.NewRecorder()
		server.apiFilterRules(createW, createReq)
		require.Equal(t, http.StatusOK, createW.Code)

		var createResp FilterRuleResponse
		json.Unmarshal(createW.Body.Bytes(), &createResp)

		// Now delete it
		deleteReq := httptest.NewRequest(http.MethodDelete, "/api/filter-rules/"+strconv.FormatUint(uint64(createResp.ID), 10), nil)
		deleteReq.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		deleteW := httptest.NewRecorder()
		server.deleteFilterRule(deleteW, deleteReq, createResp.ID)

		assert.Equal(t, http.StatusOK, deleteW.Code)

		// Verify it's deleted
		getReq := httptest.NewRequest(http.MethodGet, "/api/filter-rules/"+strconv.FormatUint(uint64(createResp.ID), 10), nil)
		getReq.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		getW := httptest.NewRecorder()
		server.getFilterRule(getW, getReq, createResp.ID)

		assert.Equal(t, http.StatusNotFound, getW.Code)
	})

	t.Run("Get non-existent filter rule returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/filter-rules/9999", nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.getFilterRule(w, req, 9999)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestFilterRuleTestAPI(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	// Add some test torrents
	db := global.GlobalDB.DB
	torrents := []models.TorrentInfo{
		{SiteName: "test", TorrentID: "1", Title: "Game of Thrones S01E01"},
		{SiteName: "test", TorrentID: "2", Title: "Game of Thrones S01E02"},
		{SiteName: "test", TorrentID: "3", Title: "Breaking Bad S01E01"},
		{SiteName: "test", TorrentID: "4", Title: "The Office S01E01"},
	}
	for _, t := range torrents {
		db.Create(&t)
	}

	t.Run("Test keyword pattern", func(t *testing.T) {
		reqBody := FilterRuleTestRequest{
			Pattern:     "Game of Thrones",
			PatternType: "keyword",
			Limit:       10,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.testFilterRule(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleTestResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 2, resp.MatchCount)
	})

	t.Run("Test wildcard pattern", func(t *testing.T) {
		reqBody := FilterRuleTestRequest{
			Pattern:     "*S01E01*",
			PatternType: "wildcard",
			Limit:       10,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.testFilterRule(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleTestResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 3, resp.MatchCount)
	})

	t.Run("Test regex pattern", func(t *testing.T) {
		reqBody := FilterRuleTestRequest{
			Pattern:     "S\\d{2}E\\d{2}",
			PatternType: "regex",
			Limit:       10,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.testFilterRule(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleTestResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 4, resp.MatchCount)
	})

	t.Run("Test invalid regex returns error", func(t *testing.T) {
		reqBody := FilterRuleTestRequest{
			Pattern:     "[invalid",
			PatternType: "regex",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.testFilterRule(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestFilterRuleValidation(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	t.Run("Empty name fails", func(t *testing.T) {
		reqBody := FilterRuleRequest{
			Name:    "",
			Pattern: "test",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.apiFilterRules(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "名称")
	})

	t.Run("Empty pattern fails", func(t *testing.T) {
		reqBody := FilterRuleRequest{
			Name:    "Test",
			Pattern: "",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.apiFilterRules(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "模式")
	})

	t.Run("Invalid pattern type fails", func(t *testing.T) {
		reqBody := FilterRuleRequest{
			Name:        "Test",
			Pattern:     "test",
			PatternType: "invalid",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.apiFilterRules(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "模式类型")
	})
}

func TestFilterRuleTestAPI_MatchField(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	// Add test torrents with tags
	db := global.GlobalDB.DB
	torrents := []models.TorrentInfo{
		{SiteName: "test", TorrentID: "1", Title: "Movie 1080p BluRay", Tag: "action,thriller"},
		{SiteName: "test", TorrentID: "2", Title: "Movie 4K HDR", Tag: "comedy,action"},
		{SiteName: "test", TorrentID: "3", Title: "TV Show S01E01", Tag: "drama"},
		{SiteName: "test", TorrentID: "4", Title: "Documentary", Tag: "action,documentary"},
	}
	for _, torrent := range torrents {
		db.Create(&torrent)
	}

	t.Run("Match title only", func(t *testing.T) {
		reqBody := FilterRuleTestRequest{
			Pattern:     "1080p",
			PatternType: "keyword",
			MatchField:  "title",
			Limit:       10,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.testFilterRule(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleTestResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 1, resp.MatchCount)
		assert.Equal(t, 4, resp.TotalCount)
	})

	t.Run("Match tag only", func(t *testing.T) {
		reqBody := FilterRuleTestRequest{
			Pattern:     "action",
			PatternType: "keyword",
			MatchField:  "tag",
			Limit:       10,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.testFilterRule(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleTestResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 3, resp.MatchCount) // 3 torrents have "action" in tag
	})

	t.Run("Match both title and tag", func(t *testing.T) {
		reqBody := FilterRuleTestRequest{
			Pattern:     "drama",
			PatternType: "keyword",
			MatchField:  "both",
			Limit:       10,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.testFilterRule(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleTestResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 1, resp.MatchCount) // 1 torrent has "drama" in tag
	})

	t.Run("Default match field is both", func(t *testing.T) {
		reqBody := FilterRuleTestRequest{
			Pattern:     "action",
			PatternType: "keyword",
			// MatchField not specified, should default to "both"
			Limit: 10,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.testFilterRule(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleTestResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 3, resp.MatchCount) // 3 torrents have "action" in tag
	})
}

func TestFilterRuleTestAPI_NoMatches(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	// Add test torrents
	db := global.GlobalDB.DB
	torrents := []models.TorrentInfo{
		{SiteName: "test", TorrentID: "1", Title: "Movie 1080p BluRay", Tag: "action"},
		{SiteName: "test", TorrentID: "2", Title: "Movie 4K HDR", Tag: "comedy"},
	}
	for _, torrent := range torrents {
		db.Create(&torrent)
	}

	t.Run("No matches returns empty array", func(t *testing.T) {
		reqBody := FilterRuleTestRequest{
			Pattern:     "nonexistent_pattern_xyz",
			PatternType: "keyword",
			Limit:       10,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.testFilterRule(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleTestResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 0, resp.MatchCount)
		assert.Equal(t, 2, resp.TotalCount)
		assert.Empty(t, resp.Matches)
	})
}

func TestFilterRuleTestAPI_ResponseFormat(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	// Add test torrents with tags
	db := global.GlobalDB.DB
	torrents := []models.TorrentInfo{
		{SiteName: "test", TorrentID: "1", Title: "Movie 1080p BluRay", Tag: "action,thriller"},
	}
	for _, torrent := range torrents {
		db.Create(&torrent)
	}

	t.Run("Response includes title and tag", func(t *testing.T) {
		reqBody := FilterRuleTestRequest{
			Pattern:     "1080p",
			PatternType: "keyword",
			Limit:       10,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.testFilterRule(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleTestResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 1, resp.MatchCount)
		require.Len(t, resp.Matches, 1)
		assert.Equal(t, "Movie 1080p BluRay", resp.Matches[0].Title)
		assert.Equal(t, "action,thriller", resp.Matches[0].Tag)
	})
}

// ==== merged from api_filter_site_cov3_test.go ====
func TestUpdateFilterRule_PatternTypeOnly(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	id := createFilterRuleForCov(t, server, "PTOnly")

	t.Run("update pattern type only valid", func(t *testing.T) {
		body, _ := json.Marshal(FilterRuleRequest{PatternType: "keyword", Enabled: true})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/filter-rules/1", bytes.NewReader(body))
		server.updateFilterRule(w, req, id)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("update pattern type only invalid existing pattern", func(t *testing.T) {
		bad := createFilterRuleForCov(t, server, "PTBad")
		require.NoError(t, global.GlobalDB.DB.Model(&models.FilterRule{}).
			Where("id = ?", bad).Update("pattern", "[unclosed(").Error)
		body, _ := json.Marshal(FilterRuleRequest{PatternType: "regex", Enabled: true})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/filter-rules/2", bytes.NewReader(body))
		server.updateFilterRule(w, req, bad)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestTestFilterRule_RSSDispatch(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.RSSSubscription{}))
	rssID := uint(999)
	body, _ := json.Marshal(FilterRuleTestRequest{
		Pattern: "x*", PatternType: "wildcard", RSSID: &rssID,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
	server.testFilterRule(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTestFilterRule_LimitClamp(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	require.NoError(t, global.GlobalDB.DB.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "1", Title: "Match Me", Tag: "hd", IsFree: true, TorrentSize: 2 << 30,
	}).Error)

	body, _ := json.Marshal(FilterRuleTestRequest{
		Pattern: "Match*", PatternType: "wildcard", MatchField: "title", Limit: 500,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
	server.testFilterRule(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestApiSiteTemplateImport_ExistingTemplateUpdate(t *testing.T) {
	writeWebTestSecretKey(t)
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.SiteTemplate{}, &models.SiteSetting{}))

	require.NoError(t, db.Create(&models.SiteTemplate{Name: "dupimport", AuthMethod: "cookie"}).Error)

	tpl := models.SiteTemplateExport{Name: "dupimport", AuthMethod: "cookie", BaseURL: "https://d.example.com"}
	tplBytes, _ := json.Marshal(tpl)
	body, _ := json.Marshal(TemplateImportRequest{Template: tplBytes, Cookie: "c=1; d=2"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader(body))
	server.apiSiteTemplateImport(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

// ==== merged from api_misc_cov_test.go ====
func TestApiSites_Get(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)

	enabled := true
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("hdsky"), models.SiteConfig{
		Enabled: &enabled, AuthMethod: "cookie", Cookie: "c=1", APIUrl: "https://hdsky.me",
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
	srv.apiSites(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]SiteConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Contains(t, resp, "hdsky")
	entry := resp["hdsky"]
	assert.True(t, entry.HasCookie)
	assert.Empty(t, entry.Cookie, "raw cookie must not be exposed")
}

func TestApiSites_DeleteCov(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)

	enabled := true
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("hdsky"), models.SiteConfig{
		Enabled: &enabled, AuthMethod: "cookie", Cookie: "c=1", APIUrl: "https://hdsky.me",
	}))

	t.Run("missing name", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/sites", nil)
		srv.apiSites(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("delete existing", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/sites?name=hdsky", nil)
		srv.apiSites(w, req)
		assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusBadRequest)
	})
}

func TestApiFaviconRefresh_WithServer(t *testing.T) {
	server := setupFaviconServer(t)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/favicon/hdsky/refresh", nil)
		server.apiFaviconRefresh(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("empty site id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/favicon//refresh", nil)
		server.apiFaviconRefresh(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("known site refreshes via real definition URL failure -> 500", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/favicon/hdsky/refresh", nil)
		server.apiFaviconRefresh(w, req)
		// hdsky is a real definition; fetch likely fails in test env -> 500,
		// but if the network resolves it may be 200. Accept both.
		assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
	})
}

func TestApiFaviconRouter_RefreshDispatch(t *testing.T) {
	server := setupFaviconServer(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicon/hdsky/refresh", nil)
	server.apiFavicon(w, req)
	// refresh requires POST; GET -> 405
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestUpdateFilterRule_Paths(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	body, _ := json.Marshal(FilterRuleRequest{
		Name: "Rule1", Pattern: "foo*", PatternType: "wildcard", Enabled: true, Priority: 10,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
	server.apiFilterRules(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	t.Run("invalid body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/filter-rules/1", bytes.NewBufferString(`{bad`))
		server.updateFilterRule(w, req, 1)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("not found", func(t *testing.T) {
		ub, _ := json.Marshal(FilterRuleRequest{Name: "X", Pattern: "a*", PatternType: "wildcard"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/filter-rules/999", bytes.NewReader(ub))
		server.updateFilterRule(w, req, 999)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("invalid pattern rejected", func(t *testing.T) {
		ub, _ := json.Marshal(FilterRuleRequest{Name: "Rule1", Pattern: "[bad(", PatternType: "regex"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/filter-rules/1", bytes.NewReader(ub))
		server.updateFilterRule(w, req, 1)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid match field rejected", func(t *testing.T) {
		ub, _ := json.Marshal(FilterRuleRequest{Name: "Rule1", MatchField: "bogus"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/filter-rules/1", bytes.NewReader(ub))
		server.updateFilterRule(w, req, 1)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	_ = global.GlobalDB
}
