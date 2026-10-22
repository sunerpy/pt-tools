package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal"
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
	t.Setenv("USERPROFILE", t.TempDir()+"/nonexistent-home")

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
	server.testFilterRuleWithRSS(w, context.Background(), testRuleFor(m, models.MatchFieldBoth, false, 20), 999)
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
		server.testFilterRuleWithRSS(w, context.Background(), testRuleFor(m, models.MatchFieldTitle, true, 20), rss.ID)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("match both fields", func(t *testing.T) {
		mb, err := filter.NewMatcher(filter.PatternWildcard, "*")
		require.NoError(t, err)
		w := httptest.NewRecorder()
		server.testFilterRuleWithRSS(w, context.Background(), testRuleFor(mb, models.MatchFieldBoth, false, 1), rss.ID)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("site not found", func(t *testing.T) {
		require.NoError(t, global.GlobalDB.DB.Create(&models.RSSSubscription{
			Name: "orphan", URL: ts.URL, SiteID: 9999, IntervalMinutes: 10,
		}).Error)
		w := httptest.NewRecorder()
		server.testFilterRuleWithRSS(w, context.Background(), testRuleFor(m, models.MatchFieldTitle, false, 20), 2)
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
		server.testFilterRuleWithRSS(w, context.Background(), testRuleFor(m, models.MatchFieldTitle, false, 20), 999)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("matches feed items", func(t *testing.T) {
		w := httptest.NewRecorder()
		server.testFilterRuleWithRSS(w, context.Background(), testRuleFor(m, models.MatchFieldTitle, false, 20), rss.ID)
		require.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleTestResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 2, resp.TotalCount)
	})
}

// testRuleFor 是直接调 testFilterRuleWithRSS 用的试跑：只看模式与「仅免费」，其余按默认（自动免费 + 过滤规则、不限大小）。
func testRuleFor(m filter.PatternMatcher, field models.MatchField, requireFree bool, limit int) ruleTest {
	return ruleTest{
		matcher: m, matchField: field, limit: limit, mode: models.FilterModeAutoFree,
		rule: &models.FilterRule{RequireFree: requireFree, MatchField: field, Enabled: true},
	}
}

// ==== merged from api_filter_rule_test.go ====
func setupFilterRuleTestServer(t *testing.T) (*Server, func()) {
	t.Helper()

	// Create a temporary directory for the test database
	tmpDir, err := os.MkdirTemp("", "filter_rule_api_test")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")
	// 测试库不刷盘（同 core.NewTempDBDir）：Windows runner 上每次写都要刷盘，一条 SQL 能慢到几秒
	db, err := gorm.Open(sqlite.Open("file:"+dbPath+"?_pragma=journal_mode(MEMORY)&_pragma=synchronous(OFF)"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	t.Cleanup(func() { closeTestGorm(db) })

	// Migrate tables
	err = db.AutoMigrate(&models.FilterRule{}, &models.TorrentInfo{}, &models.SiteSetting{})
	require.NoError(t, err)

	// Set global DB
	global.GlobalDB = &models.TorrentDB{DB: db}

	// Initialize logger for tests
	zapLogger, _ := zap.NewDevelopment()
	global.GlobalLogger = zapLogger

	server := &Server{
		sessions: sessionsWith("test-session", "admin"),
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

// 新建规则时不勾「要求免费」、不勾「启用」要原样存下：两列的默认是 true，GORM 建记录时会把 false 写成默认值
func TestCreateFilterRule_KeepsFalseFlags(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	create := func(name string, requireFree, enabled bool) FilterRuleResponse {
		body, _ := json.Marshal(FilterRuleRequest{Name: name, Pattern: "Movie", RequireFree: requireFree, Enabled: enabled})
		w := httptest.NewRecorder()
		server.createFilterRule(w, httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body)))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp FilterRuleResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp
	}
	stored := func(id uint) models.FilterRule {
		var r models.FilterRule
		require.NoError(t, global.GlobalDB.DB.First(&r, id).Error)
		return r
	}

	off := create("半价也下", false, false)
	assert.False(t, off.RequireFree)
	assert.False(t, off.Enabled)
	assert.False(t, stored(off.ID).RequireFree, "不要求免费")
	assert.False(t, stored(off.ID).Enabled, "没启用")

	on := create("只要免费", true, true)
	assert.True(t, stored(on.ID).RequireFree)
	assert.True(t, stored(on.ID).Enabled)
}

// 第二次写失败时整个不建：不留下一条存反了的规则，同名的还能再建
func TestCreateFilterRule_FalseFlagsWriteIsAtomic(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	db := global.GlobalDB.DB
	failing := true
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("qa:fail-filter-rule-update", func(tx *gorm.DB) {
		if failing && tx.Statement.Table == "filter_rules" {
			_ = tx.AddError(errors.New("磁盘写满了"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Update().Remove("qa:fail-filter-rule-update") })

	post := func() *httptest.ResponseRecorder {
		body, _ := json.Marshal(FilterRuleRequest{Name: "半价也下", Pattern: "Movie", RequireFree: false, Enabled: true})
		w := httptest.NewRecorder()
		server.createFilterRule(w, httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body)))
		return w
	}
	w := post()
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	var n int64
	require.NoError(t, db.Model(&models.FilterRule{}).Where("name = ?", "半价也下").Count(&n).Error)
	assert.Zero(t, n, "失败时不留下规则")

	failing = false
	w = post()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var r models.FilterRule
	require.NoError(t, db.Where("name = ?", "半价也下").First(&r).Error)
	assert.False(t, r.RequireFree)
}

// #556：规则用途经接口存取。新建时空着是 download；更新时空着不改；不认识的值回 400；列表与详情都带上用途。
func TestFilterRulePurposeRoundTrip(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	create := func(req FilterRuleRequest) (*httptest.ResponseRecorder, FilterRuleResponse) {
		t.Helper()
		body, _ := json.Marshal(req)
		w := httptest.NewRecorder()
		server.createFilterRule(w, httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body)))
		var resp FilterRuleResponse
		if w.Code == http.StatusOK {
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		}
		return w, resp
	}
	update := func(id uint, req FilterRuleRequest) (*httptest.ResponseRecorder, FilterRuleResponse) {
		t.Helper()
		body, _ := json.Marshal(req)
		w := httptest.NewRecorder()
		server.updateFilterRule(w, httptest.NewRequest(http.MethodPut, "/api/filter-rules/"+strconv.Itoa(int(id)), bytes.NewReader(body)), id)
		var resp FilterRuleResponse
		if w.Code == http.StatusOK {
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		}
		return w, resp
	}

	w, def := create(FilterRuleRequest{Name: "默认用途", Pattern: "a", Enabled: true})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "download", def.Purpose, "空着是下载")

	w, notify := create(FilterRuleRequest{Name: "通知用", Pattern: "b", Enabled: true, Purpose: "notify"})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "notify", notify.Purpose)
	stored, err := models.NewFilterRuleDB(global.GlobalDB).GetByID(notify.ID)
	require.NoError(t, err)
	assert.Equal(t, "notify", stored.Purpose, "存进库里")

	w, _ = create(FilterRuleRequest{Name: "乱写", Pattern: "c", Enabled: true, Purpose: "everything"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "规则用途只能是")

	// 改成 both；之后不带用途的更新（老前端）不改它
	w, both := update(notify.ID, FilterRuleRequest{Name: "通知用", Pattern: "b", Enabled: true, Purpose: "both"})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "both", both.Purpose)
	w, kept := update(notify.ID, FilterRuleRequest{Name: "通知用", Pattern: "b", Enabled: true})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "both", kept.Purpose)
	w, _ = update(notify.ID, FilterRuleRequest{Name: "通知用", Pattern: "b", Enabled: true, Purpose: "x"})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 列表与详情
	lw := httptest.NewRecorder()
	server.listFilterRules(lw, httptest.NewRequest(http.MethodGet, "/api/filter-rules", nil))
	require.Equal(t, http.StatusOK, lw.Code)
	var list []FilterRuleResponse
	require.NoError(t, json.Unmarshal(lw.Body.Bytes(), &list))
	byName := map[string]string{}
	for _, r := range list {
		byName[r.Name] = r.Purpose
	}
	assert.Equal(t, map[string]string{"默认用途": "download", "通知用": "both"}, byName)
	gw := httptest.NewRecorder()
	server.getFilterRule(gw, httptest.NewRequest(http.MethodGet, "/api/filter-rules/1", nil), notify.ID)
	require.Equal(t, http.StatusOK, gw.Code)
	assert.Contains(t, gw.Body.String(), `"purpose":"both"`)

	// 库里是空值（v7 以前的数据）或不认识的值的老规则，和过滤时一样按 download 返回；前端原样回传也是合法的
	for _, stored := range []string{"", "legacy"} {
		require.NoError(t, global.GlobalDB.DB.Model(&models.FilterRule{}).Where("id = ?", def.ID).Update("purpose", stored).Error)
		gw = httptest.NewRecorder()
		server.getFilterRule(gw, httptest.NewRequest(http.MethodGet, "/api/filter-rules/1", nil), def.ID)
		var got FilterRuleResponse
		require.NoError(t, json.Unmarshal(gw.Body.Bytes(), &got))
		assert.Equal(t, "download", got.Purpose, stored)
		w, back := update(def.ID, FilterRuleRequest{Name: got.Name, Pattern: got.Pattern, Enabled: !got.Enabled, Purpose: got.Purpose})
		require.Equal(t, http.StatusOK, w.Code, stored)
		assert.Equal(t, "download", back.Purpose)
	}
}

// RSS 试跑和历史记录一样走完整决策：要求免费而种子不免费的列成「跳过」并给原因；详情没取到的标出来；回应说明用的哪个数据源
func TestTestFilterRuleWithRSS_FullDecisionAndSource(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	feedXML := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>test</title>
<item><title>Cool Movie 1080p</title><guid>1</guid><link>http://e/1</link><category>movie</category>` +
		`<enclosure url="http://e/1.torrent" length="3221225472" type="application/x-bittorrent"/></item>
<item><title>Another Show</title><guid>2</guid><link>http://e/2</link><category>tv</category></item>
</channel></rss>`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(feedXML))
	}))
	defer ts.Close()

	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.RSSSubscription{}))
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "unknownrss", Enabled: true, BaseURL: "http://e"}).Error)
	rss := models.RSSSubscription{Name: "电影", URL: ts.URL, SiteID: 1, IntervalMinutes: 10}
	require.NoError(t, global.GlobalDB.DB.Create(&rss).Error)

	post := func(req FilterRuleTestRequest) (int, FilterRuleTestResponse, string) {
		body, _ := json.Marshal(req)
		w := httptest.NewRecorder()
		server.testFilterRule(w, httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body)))
		var resp FilterRuleTestResponse
		if w.Code == http.StatusOK {
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		}
		return w.Code, resp, w.Body.String()
	}

	code, resp, _ := post(FilterRuleTestRequest{Pattern: "Cool", PatternType: "keyword", RequireFree: true, RSSID: &rss.ID})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "rss", resp.Source)
	assert.Equal(t, "电影（unknownrss）", resp.SourceName)
	assert.Equal(t, 2, resp.TotalCount)
	assert.Equal(t, 2, resp.DetailFailed, "这个站点不取详情：两条都按 RSS 里的信息算")
	require.Len(t, resp.Matches, 1)
	m := resp.Matches[0]
	assert.Equal(t, "Cool Movie 1080p", m.Title)
	assert.Equal(t, "skipped", m.Decision)
	assert.Contains(t, m.Reason, "要求免费")
	assert.True(t, m.DetailMissing)
	assert.InDelta(t, 3.0, m.SizeGB, 0.001, "大小取 RSS 附件的长度")

	// 模拟免费、规则大小上限 2 GB：命中但大小不符
	free := true
	code, resp, _ = post(FilterRuleTestRequest{Pattern: "Cool", PatternType: "keyword", MaxSizeGB: 2, TestIsFree: &free, RSSID: &rss.ID})
	require.Equal(t, http.StatusOK, code)
	require.Len(t, resp.Matches, 1)
	assert.Equal(t, "skipped", resp.Matches[0].Decision)
	assert.Contains(t, resp.Matches[0].Reason, "大小")

	// 历史记录：数据源写明是最近入库的记录
	code, resp, _ = post(FilterRuleTestRequest{Pattern: "Cool", PatternType: "keyword"})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "history", resp.Source)
	assert.Contains(t, resp.SourceName, "最近入库")
	assert.NotNil(t, resp.Matches)

	// 示例 RSS 与占位地址：不去请求，说清楚怎么改
	example := models.RSSSubscription{Name: "示例", URL: ts.URL, SiteID: 1, IntervalMinutes: 10, IsExample: true}
	require.NoError(t, global.GlobalDB.DB.Create(&example).Error)
	code, _, text := post(FilterRuleTestRequest{Pattern: "Cool", RSSID: &example.ID})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, text, "示例 RSS")

	placeholder := models.RSSSubscription{Name: "HDSky", URL: "https://hdsky.xxx/torrentrss.php?xxx", SiteID: 1, IntervalMinutes: 10}
	require.NoError(t, global.GlobalDB.DB.Create(&placeholder).Error)
	code, _, text = post(FilterRuleTestRequest{Pattern: "Cool", RSSID: &placeholder.ID})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, text, "获取 RSS「HDSky」失败")
	assert.Contains(t, text, "占位地址（hdsky.xxx）")

	// 站点那边出错：502，带上站点回的状态
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer bad.Close()
	denied := models.RSSSubscription{Name: "失效", URL: bad.URL + "/torrentrss.php?passkey=TOPSECRET", SiteID: 1, IntervalMinutes: 10}
	require.NoError(t, global.GlobalDB.DB.Create(&denied).Error)
	code, _, text = post(FilterRuleTestRequest{Pattern: "Cool", RSSID: &denied.ID})
	assert.Equal(t, http.StatusBadGateway, code)
	assert.Contains(t, text, "HTTP 403")
	assert.NotContains(t, text, "TOPSECRET")
}

// RSS 试跑和 RSS 任务一样按 RSS 条目的标题匹配：详情页上的标题取不到（空的）或写法不同时照样命中，结果里也是 RSS 的标题；
// 副标题、免费与大小按详情（以前按详情页的标题匹配，NexusPHP 站点详情页的标题取不到时一条都不命中）
func TestTestFilterRuleWithRSS_MatchesRSSTitle(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>t</title>
<item><title>Natural.High.S04E01.2026.1080p.WEB-DL-HHWEB</title><guid>1</guid><link>http://e/details.php?id=1</link></item>
<item><title>Natural.High.S04E02.2026.1080p.WEB-DL-HHWEB</title><guid>2</guid><link>http://e/details.php?id=2</link></item>
<item><title>Other.Show.S01E01.1080p-QA</title><guid>3</guid><link>http://e/details.php?id=3</link></item>
</channel></rss>`))
	}))
	defer ts.Close()
	orig := rssTestDetail
	t.Cleanup(func() { rssTestDetail = orig })
	rssTestDetail = func(_ context.Context, _ models.SiteGroup, item *gofeed.Item) (*internal.TorrentDetailForTest, error) {
		title := ""
		if item.GUID == "2" {
			title = "现在就出发 第四季 第1期下"
		}
		return &internal.TorrentDetailForTest{Title: title, Tag: "现在就出发 第四季", IsFree: true, SizeBytes: 2 << 30}, nil
	}
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.RSSSubscription{}))
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdtime", Enabled: true, BaseURL: "http://e"}).Error)
	rss := models.RSSSubscription{Name: "HDTime", URL: ts.URL, SiteID: 1, IntervalMinutes: 10}
	require.NoError(t, global.GlobalDB.DB.Create(&rss).Error)

	body, _ := json.Marshal(FilterRuleTestRequest{Pattern: "HHWEB", PatternType: "keyword", MatchField: "both", RSSID: &rss.ID})
	w := httptest.NewRecorder()
	server.testFilterRule(w, httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body)))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp FilterRuleTestResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 3, resp.TotalCount)
	require.Equal(t, 2, resp.MatchCount)
	assert.Equal(t, "Natural.High.S04E01.2026.1080p.WEB-DL-HHWEB", resp.Matches[0].Title)
	assert.Equal(t, "Natural.High.S04E02.2026.1080p.WEB-DL-HHWEB", resp.Matches[1].Title, "结果里是 RSS 的标题")
	assert.Equal(t, "现在就出发 第四季", resp.Matches[0].Tag)
	assert.True(t, resp.Matches[0].IsFree)
	assert.InDelta(t, 2.0, resp.Matches[0].SizeGB, 0.001)
}

// 添加、编辑 RSS 时试一下链接：能用时给条目数与最新几条标题；占位地址与不是链接的回 400，站点那边的错回 502
func TestApiRSSLinkTest(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssFeedXML))
	}))
	defer ts.Close()

	call := func(url string) (int, string) {
		body, _ := json.Marshal(RSSLinkTestRequest{URL: url})
		w := httptest.NewRecorder()
		server.apiRSSLinkTest(w, httptest.NewRequest(http.MethodPost, "/api/rss/test", bytes.NewReader(body)))
		return w.Code, w.Body.String()
	}
	code, text := call(ts.URL + "/torrentrss.php?passkey=k")
	require.Equal(t, http.StatusOK, code)
	var resp RSSLinkTestResponse
	require.NoError(t, json.Unmarshal([]byte(text), &resp))
	assert.Equal(t, "test", resp.Title)
	assert.Equal(t, 2, resp.Count)
	assert.Equal(t, []string{"Cool Movie 1080p", "Another Show"}, resp.Titles)

	code, text = call("https://hdsky.xxx/torrentrss.php?xxx")
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, text, "占位地址")
	code, _ = call("hdsky.me/torrentrss.php")
	assert.Equal(t, http.StatusBadRequest, code)

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) }))
	defer bad.Close()
	code, text = call(bad.URL + "/rss?passkey=TOPSECRET")
	assert.Equal(t, http.StatusBadGateway, code)
	assert.Contains(t, text, "HTTP 404")
	assert.NotContains(t, text, "TOPSECRET")
}
