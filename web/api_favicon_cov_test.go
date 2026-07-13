package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func setupFaviconServer(t *testing.T) *Server {
	t.Helper()
	server, db := setupTestServer(t)
	// :memory: SQLite 每条连接是独立库；favicon 刷新路径会派生 backgroundRefresh
	// goroutine 并发访问同一 DB，若连接池开出第二条连接会得到未迁移的空库（no such table）。
	// 将连接数限制为 1，保证所有 goroutine 共享同一条已迁移的连接。
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	require.NoError(t, db.AutoMigrate(&models.FaviconCache{}))
	faviconService = &FaviconService{refreshInterval: 12 * time.Hour, stopCh: make(chan struct{})}
	t.Cleanup(func() { faviconService = nil })
	return server
}

// registerOrRefreshFaviconDef registers a favicon-backed site definition, or
// updates its URLs to the current httptest server when re-run under -count>1
// (the previous run's server is already closed, so a stale URL would 500).
func registerOrRefreshFaviconDef(t *testing.T, id, name, baseURL string) {
	t.Helper()
	if def := v2.GetDefinitionRegistry().GetOrDefault(id); def != nil {
		def.URLs = []string{baseURL}
		def.FaviconURL = baseURL + "/favicon.ico"
		return
	}
	v2.RegisterSiteDefinition(&v2.SiteDefinition{
		ID:         id,
		Name:       name,
		URLs:       []string{baseURL},
		FaviconURL: baseURL + "/favicon.ico",
	})
}

func seedFavicon(t *testing.T, siteID, name string) {
	t.Helper()
	require.NoError(t, global.GlobalDB.DB.Create(&models.FaviconCache{
		SiteID:      siteID,
		SiteName:    name,
		FaviconURL:  "https://example.com/favicon.ico",
		Data:        []byte{0x89, 0x50, 0x4e, 0x47},
		ContentType: "image/png",
		ETag:        "abc123",
		LastFetched: time.Now(),
	}).Error)
}

func TestApiFavicon_ServesCachedData(t *testing.T) {
	server := setupFaviconServer(t)
	seedFavicon(t, "hdsky", "HDSky")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicon/hdsky", nil)
	server.apiFavicon(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/png", w.Header().Get("Content-Type"))
	assert.NotEmpty(t, w.Body.Bytes())
	assert.Equal(t, `"abc123"`, w.Header().Get("ETag"))
}

func TestApiFavicon_NoFetchPlaceholder(t *testing.T) {
	server := setupFaviconServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicon/unknownsite?nofetch=1", nil)
	server.apiFavicon(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "1", w.Header().Get("X-Favicon-Placeholder"))
}

func TestServeFaviconData_NotModified(t *testing.T) {
	server := setupFaviconServer(t)
	cache := &models.FaviconCache{ContentType: "image/png", ETag: "tag1", Data: []byte{1, 2, 3}}

	t.Run("returns 304 on matching etag", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/favicon/x", nil)
		req.Header.Set("If-None-Match", `"tag1"`)
		server.serveFaviconData(w, req, cache)
		assert.Equal(t, http.StatusNotModified, w.Code)
	})

	t.Run("returns data on non-matching etag", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/favicon/x", nil)
		req.Header.Set("If-None-Match", `"other"`)
		server.serveFaviconData(w, req, cache)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, []byte{1, 2, 3}, w.Body.Bytes())
	})
}

func TestApiFaviconList_WithData(t *testing.T) {
	server := setupFaviconServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{
		Name: "hdsky", Enabled: true,
	}).Error)
	seedFavicon(t, "hdsky", "HDSky")

	t.Run("GET lists enabled sites", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/favicons", nil)
		server.apiFaviconList(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp []SiteFaviconInfo
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		found := false
		for _, info := range resp {
			if info.SiteID == "hdsky" {
				found = true
				assert.True(t, info.HasCache)
			}
		}
		assert.True(t, found, "hdsky should be listed")
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/favicons", nil)
		server.apiFaviconList(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestLoadEnabledSiteIDsLower(t *testing.T) {
	setupFaviconServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "HDSky", Enabled: true}).Error)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "mteam", Enabled: false}).Error)

	ids := loadEnabledSiteIDsLower()
	assert.True(t, ids["hdsky"])
	assert.False(t, ids["mteam"])
}

func TestServePlaceholderFavicon(t *testing.T) {
	w := httptest.NewRecorder()
	servePlaceholderFavicon(w)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/png", w.Header().Get("Content-Type"))
	assert.Equal(t, "1", w.Header().Get("X-Favicon-Placeholder"))
	assert.NotEmpty(t, w.Body.Bytes())
}

func TestFaviconService_GetFavicon_WithData(t *testing.T) {
	setupFaviconServer(t)
	seedFavicon(t, "hdsky", "HDSky")

	cache, err := faviconService.GetFavicon("HDSKY")
	require.NoError(t, err)
	assert.Equal(t, "hdsky", cache.SiteID)

	_, err = faviconService.GetFavicon("nosuch")
	assert.Error(t, err)
}
