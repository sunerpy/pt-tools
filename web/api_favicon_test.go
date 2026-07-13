package web

import (
	"bytes"
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
	_ "github.com/sunerpy/pt-tools/site/v2/definitions"
)

// ==== merged from api_favicon_bg_test.go ====
func TestInitFaviconService(t *testing.T) {
	setupFaviconServer(t)
	initFaviconService()
	require.NotNil(t, faviconService)
	close(faviconService.stopCh)
}

func TestRefreshExpiredFavicons(t *testing.T) {
	setupFaviconServer(t)

	t.Run("no enabled sites returns early", func(t *testing.T) {
		faviconService.refreshExpiredFavicons()
	})

	t.Run("with enabled site and expired cache", func(t *testing.T) {
		require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)
		old := time.Now().Add(-48 * time.Hour)
		require.NoError(t, global.GlobalDB.DB.Create(&models.FaviconCache{
			SiteID: "hdsky", SiteName: "HDSky", FaviconURL: "http://127.0.0.1:1/favicon.ico",
			Data: []byte{1}, LastFetched: old,
		}).Error)
		faviconService.refreshExpiredFavicons()
	})
}

func TestRefreshExpiredFavicons_NilDB(t *testing.T) {
	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })
	fs := &FaviconService{refreshInterval: time.Hour}
	fs.refreshExpiredFavicons()
}

func TestLoadEnabledSiteIDsLower_NilDB(t *testing.T) {
	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })
	assert.Nil(t, loadEnabledSiteIDsLower())
}

func TestApiFavicon_NilDBList(t *testing.T) {
	setupFaviconServer(t)
	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })

	server := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicons", nil)
	server.apiFaviconList(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ==== merged from api_favicon_cloak_cov6_test.go ====
func TestApiFaviconList_InitsServiceWhenNil(t *testing.T) {
	setupFaviconServer(t)
	prev := faviconService
	faviconService = nil
	t.Cleanup(func() {
		if faviconService != nil {
			close(faviconService.stopCh)
		}
		faviconService = prev
	})

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)
	if v2.GetDefinitionRegistry().GetOrDefault("hdsky") != nil {
		seedFavicon(t, "hdsky", "HDSky")
	}

	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicons", nil)
	s.apiFaviconList(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, faviconService)
}

func TestApiFaviconList_ListsCachedEnabled(t *testing.T) {
	server := setupFaviconServer(t)

	if v2.GetDefinitionRegistry().GetOrDefault("faviconlist1") == nil {
		v2.RegisterSiteDefinition(&v2.SiteDefinition{
			ID: "faviconlist1", Name: "FaviconList1", URLs: []string{"https://f.example.com"},
			FaviconURL: "https://f.example.com/favicon.ico",
		})
	}
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "faviconlist1", Enabled: true}).Error)
	seedFavicon(t, "faviconlist1", "FaviconList1")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicons", nil)
	server.apiFaviconList(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp []SiteFaviconInfo
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	found := false
	for _, info := range resp {
		if info.SiteID == "faviconlist1" {
			found = true
			assert.True(t, info.HasCache)
		}
	}
	assert.True(t, found)
}

func TestHandleCloakConfigGet_Success(t *testing.T) {
	srv, store, cleanup := newCloakTestServer(t)
	defer cleanup()
	require.NoError(t, store.SaveCloakConfig("http://m:8080", "tok", false))

	w := httptest.NewRecorder()
	srv.handleCloakConfigGet(w, cloakAuthedReq(http.MethodGet, "/api/cloak/config", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var out cloakConfigGetResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Equal(t, "http://m:8080", out.Endpoint)
	assert.True(t, out.HasToken)
}

// ==== merged from api_favicon_cov2_test.go ====
func TestApiFavicon_FetchThroughDefinitionOnMiss(t *testing.T) {
	server := setupFaviconServer(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47, 5, 6})
	}))
	defer ts.Close()

	registerOrRefreshFaviconDef(t, "covfetchsite", "CovFetchSite", ts.URL)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicon/covfetchsite", nil)
	server.apiFavicon(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Body.Bytes())
}

func TestApiFavicon_DefinitionFetchFailsPlaceholder(t *testing.T) {
	server := setupFaviconServer(t)

	if v2.GetDefinitionRegistry().GetOrDefault("covdeadsite") == nil {
		v2.RegisterSiteDefinition(&v2.SiteDefinition{
			ID:         "covdeadsite",
			Name:       "CovDeadSite",
			URLs:       []string{"http://127.0.0.1:1"},
			FaviconURL: "http://127.0.0.1:1/favicon.ico",
		})
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicon/covdeadsite", nil)
	server.apiFavicon(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "1", w.Header().Get("X-Favicon-Placeholder"))
}

func TestApiFavicon_EmptySiteAndMethod(t *testing.T) {
	server := setupFaviconServer(t)

	t.Run("empty site id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/favicon/", nil)
		server.apiFavicon(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/favicon/hdsky", nil)
		server.apiFavicon(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("unknown site no definition placeholder", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/favicon/definitely-no-such-site-xyz", nil)
		server.apiFavicon(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "1", w.Header().Get("X-Favicon-Placeholder"))
	})
}

func TestApiFaviconRefresh_Success(t *testing.T) {
	server := setupFaviconServer(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47, 7})
	}))
	defer ts.Close()

	registerOrRefreshFaviconDef(t, "covrefreshsite", "CovRefreshSite", ts.URL)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/favicon/covrefreshsite/refresh", nil)
	server.apiFaviconRefresh(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "ok")
}

func TestApiFaviconRefresh_Errors(t *testing.T) {
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

	t.Run("unknown site returns 404", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/favicon/no-such-site-refresh-xyz/refresh", nil)
		server.apiFaviconRefresh(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("fetch failure returns 500", func(t *testing.T) {
		if v2.GetDefinitionRegistry().GetOrDefault("covdeadrefresh") == nil {
			v2.RegisterSiteDefinition(&v2.SiteDefinition{
				ID:         "covdeadrefresh",
				Name:       "CovDeadRefresh",
				URLs:       []string{"http://127.0.0.1:1"},
				FaviconURL: "http://127.0.0.1:1/favicon.ico",
			})
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/favicon/covdeadrefresh/refresh", nil)
		server.apiFaviconRefresh(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestRefreshExpiredFavicons_NewSiteAndExpired(t *testing.T) {
	setupFaviconServer(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47, 8})
	}))
	defer ts.Close()

	registerOrRefreshFaviconDef(t, "covexpiredsite", "CovExpiredSite", ts.URL)

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "covexpiredsite", Enabled: true}).Error)

	fs := &FaviconService{refreshInterval: time.Nanosecond}
	fs.refreshExpiredFavicons()

	var count int64
	global.GlobalDB.DB.Model(&models.FaviconCache{}).Where("site_id = ?", "covexpiredsite").Count(&count)
	assert.Equal(t, int64(1), count)
}

// ==== merged from api_favicon_cov3_test.go ====
func TestRefreshExpiredFavicons_ExpiredCacheLoop(t *testing.T) {
	setupFaviconServer(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47, 42})
	}))
	defer ts.Close()

	registerOrRefreshFaviconDef(t, "covexpiredloop", "CovExpiredLoop", ts.URL)

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "covexpiredloop", Enabled: true}).Error)
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, global.GlobalDB.DB.Create(&models.FaviconCache{
		SiteID: "covexpiredloop", SiteName: "CovExpiredLoop", FaviconURL: ts.URL + "/favicon.ico",
		Data: []byte{1}, LastFetched: old,
	}).Error)

	fs := &FaviconService{refreshInterval: time.Nanosecond}
	fs.refreshExpiredFavicons()

	cache, err := fs.GetFavicon("covexpiredloop")
	require.NoError(t, err)
	assert.NotEmpty(t, cache.Data)
}

func TestRefreshExpiredFavicons_EnabledButNoDef(t *testing.T) {
	setupFaviconServer(t)

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "no-def-site-xyz", Enabled: true}).Error)
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, global.GlobalDB.DB.Create(&models.FaviconCache{
		SiteID: "no-def-site-xyz", SiteName: "X", FaviconURL: "http://127.0.0.1:1/f.ico",
		Data: []byte{1}, LastFetched: old,
	}).Error)

	fs := &FaviconService{refreshInterval: time.Nanosecond}
	fs.refreshExpiredFavicons()
}

// ==== merged from api_favicon_cov4_test.go ====
func TestApiFavicon_InitsServiceWhenNil(t *testing.T) {
	setupFaviconServer(t)
	prev := faviconService
	faviconService = nil
	t.Cleanup(func() {
		if faviconService != nil {
			close(faviconService.stopCh)
		}
		faviconService = prev
	})

	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicon/unknownxyz?nofetch=1", nil)
	s.apiFavicon(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, faviconService)
}

func TestApiFavicon_DefinitionURLsFallback(t *testing.T) {
	server := setupFaviconServer(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47, 11})
	}))
	defer ts.Close()

	if def := v2.GetDefinitionRegistry().GetOrDefault("covurlsonly"); def != nil {
		def.URLs = []string{ts.URL}
	} else {
		v2.RegisterSiteDefinition(&v2.SiteDefinition{
			ID:   "covurlsonly",
			Name: "CovURLsOnly",
			URLs: []string{ts.URL},
		})
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicon/covurlsonly", nil)
	server.apiFavicon(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Body.Bytes())
}

// ==== merged from api_favicon_cov_test.go ====
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

// ==== merged from api_favicon_dl_cov5_test.go ====
func TestApiFaviconRefresh_InitsServiceAndURLFallback(t *testing.T) {
	setupFaviconServer(t)
	prev := faviconService
	faviconService = nil
	t.Cleanup(func() {
		if faviconService != nil {
			close(faviconService.stopCh)
		}
		faviconService = prev
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47, 12})
	}))
	defer ts.Close()

	if def := v2.GetDefinitionRegistry().GetOrDefault("covrefreshurls"); def != nil {
		def.URLs = []string{ts.URL}
	} else {
		v2.RegisterSiteDefinition(&v2.SiteDefinition{
			ID: "covrefreshurls", Name: "CovRefreshURLs", URLs: []string{ts.URL},
		})
	}

	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/favicon/covrefreshurls/refresh", nil)
	s.apiFaviconRefresh(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, faviconService)
}

func TestApiFaviconRefresh_NoURLConfigured(t *testing.T) {
	server := setupFaviconServer(t)

	if v2.GetDefinitionRegistry().GetOrDefault("covrefreshnourl") == nil {
		v2.RegisterSiteDefinition(&v2.SiteDefinition{ID: "covrefreshnourl", Name: "CovRefreshNoURL"})
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/favicon/covrefreshnourl/refresh", nil)
	server.apiFaviconRefresh(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestApiDeleteTasks_SkipsPushedInTx(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}))

	require.NoError(t, db.Create(&models.TorrentInfo{SiteName: "s", TorrentID: "u1"}).Error)
	require.NoError(t, db.Create(&models.TorrentInfo{SiteName: "s", TorrentID: "u2"}).Error)

	body, _ := json.Marshal(DeleteTasksRequest{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tasks/batch-delete", bytes.NewReader(body))
	server.apiDeleteTasks(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp DeleteTasksResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.Success)
}

func TestApiDownloaderTorrents_SortAndCategoryTag(t *testing.T) {
	fake := &fakeDownloader{torrents: sampleTorrents()}
	server, _ := setupServerWithFakeDownloader(t, fake)

	cases := []string{
		"/api/downloader-torrents?sort_by=title&sort_order=asc",
		"/api/downloader-torrents?sort_by=size&sort_order=desc",
		"/api/downloader-torrents?sort_by=progress",
		"/api/downloader-torrents?sort_by=ratio",
		"/api/downloader-torrents?category=movie",
		"/api/downloader-torrents?tag=hd",
		"/api/downloader-torrents?state=downloading",
		"/api/downloader-torrents?search=alpha",
		"/api/downloader-torrents?page=2&page_size=1",
	}
	for _, url := range cases {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, url, nil)
		server.apiDownloaderTorrents(w, req)
		require.Equal(t, http.StatusOK, w.Code, url)
	}
}

func TestApiDownloaderCapabilities_Cov(t *testing.T) {
	server, _ := setupServerWithFakeDownloader(t, &fakeDownloader{})
	require.NoError(t, global.GlobalDB.DB.Create(&models.DownloaderSetting{
		Name: "cap-dl", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true,
	}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/capabilities", nil)
		server.apiDownloaderCapabilities(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("get", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/capabilities", nil)
		server.apiDownloaderCapabilities(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

// ==== merged from api_favicon_fetch_test.go ====
func TestFaviconService_FetchAndSave(t *testing.T) {
	setupFaviconServer(t)

	pngBytes := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	}))
	defer ts.Close()

	t.Run("fetches and stores favicon", func(t *testing.T) {
		require.NoError(t, faviconService.fetchAndSave("testsite", "TestSite", ts.URL+"/favicon.png"))

		cache, err := faviconService.GetFavicon("testsite")
		require.NoError(t, err)
		assert.NotEmpty(t, cache.Data)
		assert.NotEmpty(t, cache.ETag)
	})

	t.Run("updates existing record on refetch", func(t *testing.T) {
		require.NoError(t, faviconService.fetchAndSave("testsite", "TestSite", ts.URL+"/favicon.png"))
		var count int64
		global.GlobalDB.DB.Model(&models.FaviconCache{}).Where("site_id = ?", "testsite").Count(&count)
		assert.Equal(t, int64(1), count)
	})
}

func TestFaviconService_FetchAndSave_Errors(t *testing.T) {
	setupFaviconServer(t)

	t.Run("non-200 response", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer ts.Close()
		err := faviconService.fetchAndSave("s", "S", ts.URL)
		assert.Error(t, err)
	})

	t.Run("empty data", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()
		err := faviconService.fetchAndSave("s", "S", ts.URL)
		assert.Error(t, err)
	})
}

// ==== merged from api_favicon_more_test.go ====
func TestApiFavicon_FetchOnMiss(t *testing.T) {
	server := setupFaviconServer(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47, 9})
	}))
	defer ts.Close()

	require.NoError(t, global.GlobalDB.DB.Create(&models.FaviconCache{
		SiteID: "hdsky", SiteName: "HDSky", FaviconURL: ts.URL, Data: []byte{1, 2}, ContentType: "image/png", ETag: "e", LastFetched: time.Now(),
	}).Error)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicon/hdsky", nil)
	server.apiFavicon(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Body.Bytes())
}

func TestApiFavicon_RefreshDispatchViaRouter(t *testing.T) {
	server := setupFaviconServer(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/favicon/hdsky/refresh", nil)
	server.apiFavicon(w, req)
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
}

func TestApiUserInfoSiteDetail_PostSync(t *testing.T) {
	svc := setupTestUserInfoService()
	InitUserInfoService(svc)
	t.Cleanup(func() { InitUserInfoService(nil) })

	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/userinfo/sites/site1", nil)
	s.apiUserInfoSiteDetail(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "site1")
}

// ==== merged from api_favicon_site_cov6_test.go ====
func TestFaviconFetchAndSave_ErrorPaths(t *testing.T) {
	setupFaviconServer(t)

	t.Run("nil db", func(t *testing.T) {
		prev := global.GlobalDB
		global.GlobalDB = nil
		t.Cleanup(func() { global.GlobalDB = prev })
		fs := &FaviconService{}
		err := fs.fetchAndSave("s", "S", "http://127.0.0.1:1/f.ico")
		require.Error(t, err)
	})

	t.Run("build error invalid url", func(t *testing.T) {
		fs := &FaviconService{}
		err := fs.fetchAndSave("s", "S", "://bad-url")
		require.Error(t, err)
	})

	t.Run("connection error", func(t *testing.T) {
		fs := &FaviconService{}
		err := fs.fetchAndSave("s", "S", "http://127.0.0.1:1/f.ico")
		require.Error(t, err)
	})
}

func TestGetFavicon_NilDB(t *testing.T) {
	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })
	fs := &FaviconService{}
	_, err := fs.GetFavicon("x")
	require.Error(t, err)
}

func TestApiSiteDetail_GetFullResponse(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)

	enabled := true
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("hdsky"), models.SiteConfig{
		Enabled:    &enabled,
		AuthMethod: "cookie",
		Cookie:     "abc=1",
		APIUrl:     "https://hdsky.me",
		Passkey:    "pk",
		RSS:        []models.RSSConfig{{Name: "f1", URL: "http://e/rss"}},
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sites/hdsky", nil)
	srv.apiSiteDetail(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp SiteConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.HasCookie)
	assert.Empty(t, resp.Cookie)
	assert.True(t, resp.IsBuiltin)
}

// ==== merged from api_favicon_test.go ====
func TestFaviconService_fetchAndSave_NoDB(t *testing.T) {
	// 保存当前 GlobalDB 并在测试后恢复
	oldDB := global.GlobalDB
	global.GlobalDB = nil
	defer func() { global.GlobalDB = oldDB }()

	// 测试在没有数据库时的行为
	fs := &FaviconService{
		refreshInterval: 12 * time.Hour,
	}

	err := fs.fetchAndSave("test", "Test Site", "https://example.com/favicon.ico")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "数据库未初始化")
}

func TestFaviconService_GetFavicon_NoDB(t *testing.T) {
	// 保存当前 GlobalDB 并在测试后恢复
	oldDB := global.GlobalDB
	global.GlobalDB = nil
	defer func() { global.GlobalDB = oldDB }()

	fs := &FaviconService{
		refreshInterval: 12 * time.Hour,
	}

	cache, err := fs.GetFavicon("test")
	assert.Error(t, err)
	assert.Nil(t, cache)
}

func TestApiFavicon_MethodNotAllowed(t *testing.T) {
	server := &Server{}

	// POST 请求到非 refresh 路径应该返回 405
	req := httptest.NewRequest(http.MethodPost, "/api/favicon/hdsky", nil)
	rec := httptest.NewRecorder()

	// 初始化服务避免 nil panic
	faviconService = &FaviconService{
		refreshInterval: 12 * time.Hour,
	}

	server.apiFavicon(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestApiFavicon_EmptySiteID(t *testing.T) {
	server := &Server{}

	// 初始化服务
	faviconService = &FaviconService{
		refreshInterval: 12 * time.Hour,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/favicon/", nil)
	rec := httptest.NewRecorder()

	server.apiFavicon(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestApiFavicon_NonexistentSite(t *testing.T) {
	server := &Server{}

	// 初始化服务
	faviconService = &FaviconService{
		refreshInterval: 12 * time.Hour,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/favicon/nonexistent_site_xyz", nil)
	rec := httptest.NewRecorder()

	server.apiFavicon(rec, req)

	// 未配置/不存在的站点返回 1x1 透明 PNG 占位图，避免 404 日志噪音
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "image/png", rec.Header().Get("Content-Type"))
	assert.Equal(t, "1", rec.Header().Get("X-Favicon-Placeholder"))
	assert.Greater(t, rec.Body.Len(), 0)
}

func TestApiFavicon_NonexistentSite_NoFetch(t *testing.T) {
	server := &Server{}

	faviconService = &FaviconService{
		refreshInterval: 12 * time.Hour,
	}

	// nofetch=1 对未缓存站点返回 200 透明占位图（可缓存 24h），
	// 避免浏览/添加站点视图刷屏 404 噪音，前端仍可降级为字母头像。
	req := httptest.NewRequest(http.MethodGet, "/api/favicon/nonexistent_site_xyz?nofetch=1", nil)
	rec := httptest.NewRecorder()

	server.apiFavicon(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Greater(t, rec.Body.Len(), 0)
}

func TestApiFaviconRefresh_MethodNotAllowed(t *testing.T) {
	server := &Server{}

	// 初始化服务
	faviconService = &FaviconService{
		refreshInterval: 12 * time.Hour,
	}

	// GET 请求应该返回 405
	req := httptest.NewRequest(http.MethodGet, "/api/favicon/hdsky/refresh", nil)
	rec := httptest.NewRecorder()

	server.apiFaviconRefresh(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestApiFaviconRefresh_EmptySiteID(t *testing.T) {
	server := &Server{}

	// 初始化服务
	faviconService = &FaviconService{
		refreshInterval: 12 * time.Hour,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/favicon//refresh", nil)
	rec := httptest.NewRecorder()

	server.apiFaviconRefresh(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestApiFaviconRefresh_NonexistentSite(t *testing.T) {
	server := &Server{}

	// 初始化服务
	faviconService = &FaviconService{
		refreshInterval: 12 * time.Hour,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/favicon/nonexistent_xyz/refresh", nil)
	rec := httptest.NewRecorder()

	server.apiFaviconRefresh(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestServeFaviconData_Caching(t *testing.T) {
	server := &Server{}

	testData := []byte{0x00, 0x00, 0x01, 0x00} // 简单的 ICO 文件头
	cache := &models.FaviconCache{
		SiteID:      "test",
		SiteName:    "Test Site",
		Data:        testData,
		ContentType: "image/x-icon",
		ETag:        "abc123",
	}

	// 第一次请求
	req1 := httptest.NewRequest(http.MethodGet, "/api/favicon/test", nil)
	rec1 := httptest.NewRecorder()

	server.serveFaviconData(rec1, req1, cache)

	assert.Equal(t, http.StatusOK, rec1.Code)
	assert.Equal(t, `"abc123"`, rec1.Header().Get("ETag"))
	assert.Equal(t, "public, max-age=86400", rec1.Header().Get("Cache-Control"))
	assert.Equal(t, "image/x-icon", rec1.Header().Get("Content-Type"))

	// 带有 If-None-Match 的请求（应该返回 304）
	req2 := httptest.NewRequest(http.MethodGet, "/api/favicon/test", nil)
	req2.Header.Set("If-None-Match", `"abc123"`)
	rec2 := httptest.NewRecorder()

	server.serveFaviconData(rec2, req2, cache)

	assert.Equal(t, http.StatusNotModified, rec2.Code)
}

func TestApiFaviconList_WithRegisteredSites(t *testing.T) {
	// 幂等注册：definition registry 对重复 ID 会 panic（除非是同一指针），
	// 因此在 -count>1 时须先检查再注册，否则第二次迭代会崩溃。
	if v2.GetDefinitionRegistry().GetOrDefault("testsite_favicon") == nil {
		v2.RegisterSiteDefinition(&v2.SiteDefinition{
			ID:         "testsite_favicon",
			Name:       "Test Site for Favicon",
			URLs:       []string{"https://test.example.com/"},
			FaviconURL: "https://test.example.com/favicon.ico",
		})
	}

	// 初始化服务
	faviconService = &FaviconService{
		refreshInterval: 12 * time.Hour,
	}

	server := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/api/favicons", nil)
	rec := httptest.NewRecorder()

	// 注意：这个测试需要 global.GlobalDB，如果没有会返回 500
	// 但至少可以测试路由是否正确
	server.apiFaviconList(rec, req)

	// 由于没有初始化数据库，应该返回 500
	// 但这验证了函数被正确调用
	assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusInternalServerError)
}

// ==== merged from api_paused_favicon_cov3_test.go ====
func TestApiDeletePausedTorrents_DownloaderNotFound(t *testing.T) {
	server, _ := setupServerWithFakeDownloader(t, &fakeDownloader{})
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.TorrentInfo{}))

	ti := models.TorrentInfo{
		SiteName: "s", TorrentID: "p2", IsPausedBySystem: true,
		DownloaderTaskID: "task-p2", DownloaderName: "no-such-dl",
	}
	require.NoError(t, global.GlobalDB.DB.Create(&ti).Error)

	body, _ := json.Marshal(DeletePausedRequest{IDs: []uint{ti.ID}})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/torrents/delete-paused", bytes.NewReader(body))
	server.apiDeletePausedTorrents(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp DeletePausedResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.Success)
}

func TestServeFaviconData_HeadersAndData(t *testing.T) {
	server := setupFaviconServer(t)
	cache := &models.FaviconCache{ContentType: "image/png", ETag: "e9", Data: []byte{9, 8, 7}}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicon/x", nil)
	server.serveFaviconData(w, req, cache)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/png", w.Header().Get("Content-Type"))
	assert.Equal(t, `"e9"`, w.Header().Get("ETag"))
	assert.Equal(t, []byte{9, 8, 7}, w.Body.Bytes())
}

func TestApiFaviconList_EmptyEnabled(t *testing.T) {
	server := setupFaviconServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicons", nil)
	server.apiFaviconList(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp []SiteFaviconInfo
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp)
}
