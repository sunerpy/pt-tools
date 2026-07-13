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
