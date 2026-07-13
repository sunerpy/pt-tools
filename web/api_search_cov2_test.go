package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestApiMultiSiteSearch_WithOrchestrator(t *testing.T) {
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky"})
	server, _ := setupTestServer(t)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v2/search/multi", nil)
		server.apiMultiSiteSearch(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("bad body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/search/multi", bytes.NewBufferString(`{bad`))
		server.apiMultiSiteSearch(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("empty keyword", func(t *testing.T) {
		body, _ := json.Marshal(MultiSiteSearchRequest{})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/search/multi", bytes.NewReader(body))
		server.apiMultiSiteSearch(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("search executes", func(t *testing.T) {
		body, _ := json.Marshal(MultiSiteSearchRequest{Keyword: "movie", TimeoutSecs: 5})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/search/multi", bytes.NewReader(body))
		server.apiMultiSiteSearch(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestApiSearchSites_WithOrchestrator(t *testing.T) {
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky"})
	server, _ := setupTestServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/search/sites", nil)
	server.apiSearchSites(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestApiSearchCache_WithOrchestrator(t *testing.T) {
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky"})
	server, _ := setupTestServer(t)

	t.Run("clear cache", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/search/cache/clear", nil)
		server.apiSearchCacheClear(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("cache stats", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v2/search/cache/stats", nil)
		server.apiSearchCacheStats(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

var _ = v2.SiteNexusPHP
