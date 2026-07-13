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
	"github.com/sunerpy/pt-tools/scheduler"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func TestLoginHandler_Success(t *testing.T) {
	srv := setupServer(t)
	require.NoError(t, srv.store.EnsureAdmin("admin", hashPassword("secret")))

	t.Run("json login sets session", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"username": "admin", "password": "secret"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		srv.loginHandler(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		assert.NotEmpty(t, w.Result().Cookies())
	})

	t.Run("form login redirects", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString("username=admin&password=secret"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		srv.loginHandler(w, req)
		assert.Equal(t, http.StatusFound, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/login", nil)
		srv.loginHandler(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestLoginHandler_AutoCreatesAdmin(t *testing.T) {
	srv := setupServer(t)

	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "adminadmin"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.loginHandler(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestDownloaderHealthCheck_Success(t *testing.T) {
	server, db := setupTestServer(t)
	mgr := scheduler.NewManager()
	t.Cleanup(func() { mgr.StopAll() })
	dm := mgr.GetDownloaderManager()
	fake := &fakeDownloader{}
	dm.RegisterFactory(downloader.DownloaderQBittorrent, func(_ downloader.DownloaderConfig, name string) (downloader.Downloader, error) {
		fake.name = name
		return fake, nil
	})
	server.mgr = mgr

	dl := models.DownloaderSetting{Name: "healthy-qb", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true}
	require.NoError(t, db.Create(&dl).Error)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/downloaders/1/health", nil)
	server.downloaderHealthCheck(w, req, "1")
	require.Equal(t, http.StatusOK, w.Code)

	var resp HealthCheckResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.IsHealthy)
}

func TestApiSiteDetail_LoginStateDispatch(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	t.Run("login-state get", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/hdsky/login-state", nil)
		srv.apiSiteDetail(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("login-state unknown action", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/hdsky/login-state/bogus", nil)
		srv.apiSiteDetail(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("login-state probe no monitor", func(t *testing.T) {
		srv.mgr = scheduler.NewManager()
		t.Cleanup(func() { srv.mgr.StopAll() })
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/hdsky/login-state/probe", nil)
		srv.apiSiteDetail(w, req)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})
}

func TestApiSiteDetail_GetAndNotFound(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)
	enabled := true
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("hdsky"), models.SiteConfig{
		Enabled: &enabled, AuthMethod: "cookie", Cookie: "a=1; b=2", APIUrl: "https://hdsky.me",
	}))

	t.Run("get existing", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/hdsky", nil)
		srv.apiSiteDetail(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("unknown site 404", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/no-such-site-zzz", nil)
		srv.apiSiteDetail(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/api/sites/hdsky", nil)
		srv.apiSiteDetail(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}
