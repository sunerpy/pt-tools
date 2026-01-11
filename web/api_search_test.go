package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/global"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func init() {
	// Initialize global logger for tests
	if global.GlobalLogger == nil {
		zapLogger, _ := zap.NewDevelopment()
		global.GlobalLogger = zapLogger
	}
}

// mockSearchSiteForAPI implements v2.Site for testing
type mockSearchSiteForAPI struct {
	id       string
	name     string
	kind     v2.SiteKind
	items    []v2.TorrentItem
	err      error
	userInfo v2.UserInfo
}

func (m *mockSearchSiteForAPI) ID() string                                            { return m.id }
func (m *mockSearchSiteForAPI) Name() string                                          { return m.name }
func (m *mockSearchSiteForAPI) Kind() v2.SiteKind                                     { return m.kind }
func (m *mockSearchSiteForAPI) Login(ctx context.Context, creds v2.Credentials) error { return nil }
func (m *mockSearchSiteForAPI) GetUserInfo(ctx context.Context) (v2.UserInfo, error) {
	return m.userInfo, nil
}

func (m *mockSearchSiteForAPI) Download(ctx context.Context, torrentID string) ([]byte, error) {
	return nil, nil
}
func (m *mockSearchSiteForAPI) Close() error { return nil }

func (m *mockSearchSiteForAPI) Search(ctx context.Context, query v2.SearchQuery) ([]v2.TorrentItem, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.items, nil
}

func setupTestSearchOrchestrator() *v2.CachedSearchOrchestrator {
	orchestrator := v2.NewSearchOrchestrator(v2.SearchOrchestratorConfig{})

	site1 := &mockSearchSiteForAPI{
		id:   "site1",
		name: "Site 1",
		kind: v2.SiteNexusPHP,
		items: []v2.TorrentItem{
			{
				ID:            "1",
				Title:         "Test Torrent 1",
				SizeBytes:     1024 * 1024 * 1024,
				Seeders:       10,
				Leechers:      5,
				SourceSite:    "site1",
				DiscountLevel: v2.DiscountFree,
			},
		},
	}
	site2 := &mockSearchSiteForAPI{
		id:   "site2",
		name: "Site 2",
		kind: v2.SiteMTorrent,
		items: []v2.TorrentItem{
			{
				ID:            "2",
				Title:         "Test Torrent 2",
				SizeBytes:     2 * 1024 * 1024 * 1024,
				Seeders:       20,
				Leechers:      10,
				SourceSite:    "site2",
				DiscountLevel: v2.DiscountNone,
			},
		},
	}

	orchestrator.RegisterSite(site1)
	orchestrator.RegisterSite(site2)

	return v2.NewCachedSearchOrchestrator(orchestrator, v2.SearchCacheConfig{
		TTL: 5 * time.Minute,
	})
}

func TestApiMultiSiteSearch(t *testing.T) {
	orchestrator := setupTestSearchOrchestrator()
	InitSearchOrchestrator(orchestrator)
	defer InitSearchOrchestrator(nil)

	server := &Server{}

	req := MultiSiteSearchRequest{
		Keyword: "test",
	}
	body, _ := json.Marshal(req)

	httpReq := httptest.NewRequest(http.MethodPost, "/api/v2/search/multi", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.apiMultiSiteSearch(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var response MultiSiteSearchResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Len(t, response.Items, 2)
	assert.Equal(t, 2, response.TotalResults)
}

func TestApiMultiSiteSearch_MethodNotAllowed(t *testing.T) {
	orchestrator := setupTestSearchOrchestrator()
	InitSearchOrchestrator(orchestrator)
	defer InitSearchOrchestrator(nil)

	server := &Server{}
	httpReq := httptest.NewRequest(http.MethodGet, "/api/v2/search/multi", nil)
	w := httptest.NewRecorder()

	server.apiMultiSiteSearch(w, httpReq)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestApiMultiSiteSearch_ServiceNotInitialized(t *testing.T) {
	InitSearchOrchestrator(nil)

	server := &Server{}
	req := MultiSiteSearchRequest{Keyword: "test"}
	body, _ := json.Marshal(req)

	httpReq := httptest.NewRequest(http.MethodPost, "/api/v2/search/multi", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.apiMultiSiteSearch(w, httpReq)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestApiMultiSiteSearch_EmptyKeyword(t *testing.T) {
	orchestrator := setupTestSearchOrchestrator()
	InitSearchOrchestrator(orchestrator)
	defer InitSearchOrchestrator(nil)

	server := &Server{}
	req := MultiSiteSearchRequest{Keyword: ""}
	body, _ := json.Marshal(req)

	httpReq := httptest.NewRequest(http.MethodPost, "/api/v2/search/multi", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.apiMultiSiteSearch(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestApiMultiSiteSearch_InvalidJSON(t *testing.T) {
	orchestrator := setupTestSearchOrchestrator()
	InitSearchOrchestrator(orchestrator)
	defer InitSearchOrchestrator(nil)

	server := &Server{}
	httpReq := httptest.NewRequest(http.MethodPost, "/api/v2/search/multi", bytes.NewReader([]byte("invalid")))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.apiMultiSiteSearch(w, httpReq)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestApiMultiSiteSearch_WithFilters(t *testing.T) {
	orchestrator := setupTestSearchOrchestrator()
	InitSearchOrchestrator(orchestrator)
	defer InitSearchOrchestrator(nil)

	server := &Server{}

	req := MultiSiteSearchRequest{
		Keyword:    "test",
		FreeOnly:   true,
		MinSeeders: 5,
	}
	body, _ := json.Marshal(req)

	httpReq := httptest.NewRequest(http.MethodPost, "/api/v2/search/multi", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.apiMultiSiteSearch(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var response MultiSiteSearchResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Only free torrents with >= 5 seeders
	for _, item := range response.Items {
		assert.True(t, item.IsFree)
		assert.GreaterOrEqual(t, item.Seeders, 5)
	}
}

func TestApiMultiSiteSearch_SpecificSites(t *testing.T) {
	orchestrator := setupTestSearchOrchestrator()
	InitSearchOrchestrator(orchestrator)
	defer InitSearchOrchestrator(nil)

	server := &Server{}

	req := MultiSiteSearchRequest{
		Keyword: "test",
		Sites:   []string{"site1"},
	}
	body, _ := json.Marshal(req)

	httpReq := httptest.NewRequest(http.MethodPost, "/api/v2/search/multi", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.apiMultiSiteSearch(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var response MultiSiteSearchResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Only results from site1
	for _, item := range response.Items {
		assert.Equal(t, "site1", item.SourceSite)
	}
}

func TestApiSearchSites(t *testing.T) {
	orchestrator := setupTestSearchOrchestrator()
	InitSearchOrchestrator(orchestrator)
	defer InitSearchOrchestrator(nil)

	server := &Server{}
	httpReq := httptest.NewRequest(http.MethodGet, "/api/v2/search/sites", nil)
	w := httptest.NewRecorder()

	server.apiSearchSites(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string][]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	sites := response["sites"]
	assert.Len(t, sites, 2)
	assert.Contains(t, sites, "site1")
	assert.Contains(t, sites, "site2")
}

func TestApiSearchSites_MethodNotAllowed(t *testing.T) {
	orchestrator := setupTestSearchOrchestrator()
	InitSearchOrchestrator(orchestrator)
	defer InitSearchOrchestrator(nil)

	server := &Server{}
	httpReq := httptest.NewRequest(http.MethodPost, "/api/v2/search/sites", nil)
	w := httptest.NewRecorder()

	server.apiSearchSites(w, httpReq)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestApiSearchCacheClear(t *testing.T) {
	orchestrator := setupTestSearchOrchestrator()
	InitSearchOrchestrator(orchestrator)
	defer InitSearchOrchestrator(nil)

	server := &Server{}
	httpReq := httptest.NewRequest(http.MethodPost, "/api/v2/search/cache/clear", nil)
	w := httptest.NewRecorder()

	server.apiSearchCacheClear(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "ok", response["status"])
}

func TestApiSearchCacheClear_MethodNotAllowed(t *testing.T) {
	orchestrator := setupTestSearchOrchestrator()
	InitSearchOrchestrator(orchestrator)
	defer InitSearchOrchestrator(nil)

	server := &Server{}
	httpReq := httptest.NewRequest(http.MethodGet, "/api/v2/search/cache/clear", nil)
	w := httptest.NewRecorder()

	server.apiSearchCacheClear(w, httpReq)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestApiSearchCacheStats(t *testing.T) {
	orchestrator := setupTestSearchOrchestrator()
	InitSearchOrchestrator(orchestrator)
	defer InitSearchOrchestrator(nil)

	server := &Server{}
	httpReq := httptest.NewRequest(http.MethodGet, "/api/v2/search/cache/stats", nil)
	w := httptest.NewRecorder()

	server.apiSearchCacheStats(w, httpReq)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]int
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, response["size"], 0)
}

func TestApiSearchCacheStats_MethodNotAllowed(t *testing.T) {
	orchestrator := setupTestSearchOrchestrator()
	InitSearchOrchestrator(orchestrator)
	defer InitSearchOrchestrator(nil)

	server := &Server{}
	httpReq := httptest.NewRequest(http.MethodPost, "/api/v2/search/cache/stats", nil)
	w := httptest.NewRecorder()

	server.apiSearchCacheStats(w, httpReq)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestToTorrentItemResponse(t *testing.T) {
	now := time.Now()
	item := v2.TorrentItem{
		ID:              "123",
		URL:             "http://example.com/torrent/123",
		Title:           "Test Torrent",
		InfoHash:        "abc123",
		Magnet:          "magnet:?xt=urn:btih:abc123",
		SizeBytes:       1024 * 1024 * 1024,
		Seeders:         10,
		Leechers:        5,
		Snatched:        100,
		UploadedAt:      now.Unix(),
		Tags:            []string{"action", "movie"},
		SourceSite:      "site1",
		DiscountLevel:   v2.DiscountFree,
		DiscountEndTime: now.Add(24 * time.Hour),
		HasHR:           true,
		DownloadURL:     "http://example.com/download/123",
		Category:        "movies",
	}

	response := toTorrentItemResponse(item)

	assert.Equal(t, item.ID, response.ID)
	assert.Equal(t, item.URL, response.URL)
	assert.Equal(t, item.Title, response.Title)
	assert.Equal(t, item.InfoHash, response.InfoHash)
	assert.Equal(t, item.Magnet, response.Magnet)
	assert.Equal(t, item.SizeBytes, response.SizeBytes)
	assert.Equal(t, item.Seeders, response.Seeders)
	assert.Equal(t, item.Leechers, response.Leechers)
	assert.Equal(t, item.Snatched, response.Snatched)
	assert.Equal(t, item.UploadedAt, response.UploadedAt)
	assert.Equal(t, item.Tags, response.Tags)
	assert.Equal(t, item.SourceSite, response.SourceSite)
	assert.Equal(t, string(item.DiscountLevel), response.DiscountLevel)
	assert.Equal(t, item.DiscountEndTime.Unix(), response.DiscountEndTime)
	assert.Equal(t, item.HasHR, response.HasHR)
	assert.Equal(t, item.DownloadURL, response.DownloadURL)
	assert.Equal(t, item.Category, response.Category)
	assert.True(t, response.IsFree)
}

func TestToTorrentItemResponse_ZeroDiscountEndTime(t *testing.T) {
	item := v2.TorrentItem{
		ID:            "123",
		DiscountLevel: v2.DiscountFree,
		// DiscountEndTime is zero
	}

	response := toTorrentItemResponse(item)

	assert.Equal(t, int64(0), response.DiscountEndTime)
}
