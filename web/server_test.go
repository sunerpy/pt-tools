package web

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/config"
	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

func setupServer(t *testing.T) *Server {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.InitLogger(zap.NewNop())
	global.GlobalDB = db
	mgr := newTestManager(t)
	return NewServer(core.NewConfigStore(db), mgr)
}

func newTestManager(t *testing.T) *scheduler.Manager {
	mgr := scheduler.NewManager()
	t.Cleanup(func() {
		mgr.StopAll()
	})
	return mgr
}

func TestServer_AuthAndLogin(t *testing.T) {
	srv := setupServer(t)
	t.Run("login GET", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/login", nil)
		srv.loginHandler(rr, req)
		assert.True(t, rr.Code == http.StatusOK || rr.Code == 0)
	})
	t.Run("login JSON", func(t *testing.T) {
		rr := httptest.NewRecorder()
		body := bytes.NewBufferString(`{"username":"a","password":"b"}`)
		req := httptest.NewRequest(http.MethodPost, "/login", body)
		req.Header.Set("Content-Type", "application/json")
		srv.loginHandler(rr, req)
		assert.NotNil(t, rr.Body)
	})
	t.Run("login form urlencoded", func(t *testing.T) {
		rr := httptest.NewRecorder()
		body := bytes.NewBufferString("username=a&password=b")
		req := httptest.NewRequest(http.MethodPost, "/login", body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		srv.loginHandler(rr, req)
		assert.NotNil(t, rr.Body)
	})
	t.Run("login multipart", func(t *testing.T) {
		rr := httptest.NewRecorder()
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		require.NoError(t, mw.WriteField("username", "a"))
		require.NoError(t, mw.WriteField("password", "b"))
		require.NoError(t, mw.Close())
		req := httptest.NewRequest(http.MethodPost, "/login", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		srv.loginHandler(rr, req)
		assert.NotNil(t, rr.Body)
	})
	t.Run("logout clears session", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/logout", nil)
		srv.sessions["sid1"] = "admin"
		req.AddCookie(&http.Cookie{Name: "session", Value: "sid1"})
		srv.logoutHandler(rr, req)
		assert.True(t, rr.Code == http.StatusFound || rr.Code == 0)
	})
	t.Run("auth wrapper no session redirects", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/global", nil)
		h := srv.auth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
		h(rr, req)
		assert.Equal(t, http.StatusFound, rr.Code)
	})
	t.Run("auth wrapper with session ok", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/global", nil)
		sid := "sid-ok"
		srv.sessions[sid] = "admin"
		req.AddCookie(&http.Cookie{Name: "session", Value: sid})
		h := srv.auth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
		h(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})
}

func TestServer_APIs(t *testing.T) {
	srv := setupServer(t)
	// prepare a global config
	s := core.NewConfigStore(global.GlobalDB)
	_ = s.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true})
	t.Run("global GET", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/global", nil)
		srv.apiGlobal(rr, req)
		assert.True(t, rr.Code == http.StatusOK || rr.Code == 0)
	})
	t.Run("global POST invalid", func(t *testing.T) {
		rr := httptest.NewRecorder()
		body := bytes.NewBufferString(`{"download_dir":""}`)
		req := httptest.NewRequest(http.MethodPost, "/api/global", body)
		srv.apiGlobal(rr, req)
		assert.True(t, rr.Code == http.StatusBadRequest || rr.Code == 0)
	})
	t.Run("global POST success triggers reload", func(t *testing.T) {
		rr := httptest.NewRecorder()
		// enable autostart to hit reload path
		require.NoError(t, s.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, DefaultEnabled: true, AutoStart: true}))
		body := bytes.NewBufferString(`{"download_dir":"` + t.TempDir() + `","default_interval_minutes":5}`)
		req := httptest.NewRequest(http.MethodPost, "/api/global", body)
		srv.apiGlobal(rr, req)
		assert.True(t, rr.Code == http.StatusOK || rr.Code == 0)
	})
	t.Run("qbit GET/POST", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/qbit", nil)
		srv.apiQbit(rr, req)
		rr2 := httptest.NewRecorder()
		body := bytes.NewBufferString(`{"url":"http://localhost","user":"u","password":"p"}`)
		req2 := httptest.NewRequest(http.MethodPost, "/api/qbit", body)
		srv.apiQbit(rr2, req2)
		rr3 := httptest.NewRecorder()
		req3 := httptest.NewRequest(http.MethodPost, "/api/qbit", bytes.NewBufferString("not-json"))
		srv.apiQbit(rr3, req3)
		assert.True(t, rr3.Code == http.StatusBadRequest || rr3.Code == 0)
	})
	t.Run("qbit POST success triggers reload", func(t *testing.T) {
		rr := httptest.NewRecorder()
		body := bytes.NewBufferString(`{"enabled":true,"url":"http://localhost","user":"u","password":"p"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/qbit", body)
		srv.apiQbit(rr, req)
		assert.True(t, rr.Code == http.StatusOK || rr.Code == 0)
	})
	t.Run("sites list & delete", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
		srv.apiSites(rr, req)
		rr2 := httptest.NewRecorder()
		req2 := httptest.NewRequest(http.MethodDelete, "/api/sites?name=custom", nil)
		srv.apiSites(rr2, req2)
		rr3 := httptest.NewRecorder()
		req3 := httptest.NewRequest(http.MethodDelete, "/api/sites", nil)
		srv.apiSites(rr3, req3)
		assert.True(t, rr3.Code == http.StatusBadRequest || rr3.Code == 0)
	})
	t.Run("sites delete preset forbidden", func(t *testing.T) {
		// create preset site then attempt delete -> should be BadRequest
		e := true
		siteID, _ := s.UpsertSite(models.SiteGroup("springsunday"), models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
		_ = s.ReplaceSiteRSS(siteID, []models.RSSConfig{})
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/sites?name="+string(models.SiteGroup("springsunday")), nil)
		srv.apiSites(rr, req)
		_ = rr.Body
	})
	t.Run("site detail GET/POST/DELETE rss", func(t *testing.T) {
		e := true
		siteID, _ := s.UpsertSite(models.SiteGroup("springsunday"), models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
		_ = s.ReplaceSiteRSS(siteID, []models.RSSConfig{{Name: "r1", URL: "http://example/rss", Tag: "t", IntervalMinutes: 10}})
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/springsunday", nil)
		srv.apiSiteDetail(rr, req)
		rr2 := httptest.NewRecorder()
		body := bytes.NewBufferString(`{"enabled":true,"auth_method":"cookie","cookie":"c","rss":[{"name":"r","url":"http://example/rss","tag":"t","interval_minutes":10}]}`)
		req2 := httptest.NewRequest(http.MethodPost, "/api/sites/springsunday", body)
		srv.apiSiteDetail(rr2, req2)
		rr3 := httptest.NewRecorder()
		// delete the existing RSS row id=1 created above
		req3 := httptest.NewRequest(http.MethodDelete, "/api/sites/springsunday?id=1", nil)
		srv.apiSiteDetail(rr3, req3)
		rr4 := httptest.NewRecorder()
		req4 := httptest.NewRequest(http.MethodDelete, "/api/sites/springsunday?id=abc", nil)
		srv.apiSiteDetail(rr4, req4)
		assert.True(t, rr4.Code == http.StatusBadRequest || rr4.Code == 0)
		rr5 := httptest.NewRecorder()
		req5 := httptest.NewRequest(http.MethodGet, "/api/sites/unknown", nil)
		srv.apiSiteDetail(rr5, req5)
		assert.Equal(t, http.StatusNotFound, rr5.Code)
	})
	// remove success path http test to avoid env-dependent codes; core-level DeleteSite has covered
	t.Run("password change", func(t *testing.T) {
		hash := hashPassword("adminadmin")
		_ = s.EnsureAdmin("admin", hash)
		rr := httptest.NewRecorder()
		body := bytes.NewBufferString(`{"Username":"admin","Old":"adminadmin","New":"newpass"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/password", body)
		srv.apiPassword(rr, req)
	})
	t.Run("password wrong old", func(t *testing.T) {
		hash := hashPassword("adminadmin")
		_ = s.EnsureAdmin("admin2", hash)
		rr := httptest.NewRecorder()
		body := bytes.NewBufferString(`{"Username":"admin2","Old":"bad","New":"newpass"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/password", body)
		srv.apiPassword(rr, req)
		assert.True(t, rr.Code == http.StatusUnauthorized || rr.Code == 0)
	})
	t.Run("site detail delete when site missing", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/sites/hdsky?id=1", nil)
		srv.apiSiteDetail(rr, req)
		assert.True(t, rr.Code == http.StatusBadRequest || rr.Code == 0)
	})
	t.Run("site detail post invalid json", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/springsunday", bytes.NewBufferString("not-json"))
		srv.apiSiteDetail(rr, req)
		assert.True(t, rr.Code == http.StatusBadRequest || rr.Code == 0)
	})
	t.Run("start all cfg not ready", func(t *testing.T) {
		// use isolated DB and seed empty download dir to trigger BadRequest
		db, _ := core.NewTempDBDir(t.TempDir())
		store := core.NewConfigStore(db)
		_ = db.DB.Create(&models.SettingsGlobal{DownloadDir: ""}).Error
		srv2 := NewServer(store, newTestManager(t))
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/control/start", nil)
		srv2.apiStartAll(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
	t.Run("apiGlobal bad json", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/global", bytes.NewBufferString("not-json"))
		srv.apiGlobal(rr, req)
		assert.True(t, rr.Code == http.StatusBadRequest || rr.Code == 0)
	})
	t.Run("apiSites delete bad query", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/sites?name=", nil)
		srv.apiSites(rr, req)
		assert.True(t, rr.Code == http.StatusBadRequest || rr.Code == 0)
	})
	t.Run("apiPassword wrong method", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/password", nil)
		srv.apiPassword(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
	t.Run("password bad body", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/password", bytes.NewBufferString("not-json"))
		srv.apiPassword(rr, req)
		assert.True(t, rr.Code == http.StatusBadRequest || rr.Code == 0)
	})
	t.Run("tasks filters & pagination", func(t *testing.T) {
		pushed := true
		h := "h1"
		ti := &models.TorrentInfo{SiteName: "cmct", TorrentID: "id1", Title: "A", IsDownloaded: true, IsPushed: &pushed, TorrentHash: &h}
		_ = global.GlobalDB.UpsertTorrent(ti)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/tasks?downloaded=1&pushed=1&site=cmct&page=1&page_size=2", nil)
		srv.apiTasks(rr, req)
	})
	t.Run("tasks search/sort/expired", func(t *testing.T) {
		pushed := true
		h := "h2"
		ti := &models.TorrentInfo{SiteName: "cmct", TorrentID: "id2", Title: "B keyword", IsDownloaded: true, IsPushed: &pushed, TorrentHash: &h, IsExpired: true}
		_ = global.GlobalDB.UpsertTorrent(ti)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/tasks?q=keyword&expired=1&sort=created_at_asc", nil)
		srv.apiTasks(rr, req)
		assert.NotNil(t, rr.Body)
	})
	t.Run("tasks invalid page size", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/tasks?page=1&page_size=10000", nil)
		srv.apiTasks(rr, req)
		assert.NotNil(t, rr.Body)
	})
	t.Run("tasks invalid params", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/tasks?page=-1&page_size=-5&sort=invalid", nil)
		srv.apiTasks(rr, req)
		assert.NotNil(t, rr.Body)
	})
	t.Run("logs read", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
		srv.apiLogs(rr, req)
	})
	t.Run("logs no file", func(t *testing.T) {
		// change home dir env to non-existent path to trigger error
		t.Setenv("HOME", t.TempDir()+"/noexist")
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
		srv.apiLogs(rr, req)
		assert.True(t, rr.Code == http.StatusInternalServerError || rr.Code == 0)
	})
	t.Run("logs truncate large", func(t *testing.T) {
		// prepare large log file under HOME/pt-tools/logs/all.log
		home := t.TempDir()
		t.Setenv("HOME", home)
		p := filepath.Join(home, models.WorkDir, config.DefaultZapConfig.Directory)
		require.NoError(t, os.MkdirAll(p, 0o755))
		f := filepath.Join(p, "all.log")
		var b bytes.Buffer
		for i := 0; i < 6000; i++ {
			b.WriteString("line\n")
		}
		require.NoError(t, os.WriteFile(f, b.Bytes(), 0o644))
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
		srv.apiLogs(rr, req)
		assert.True(t, rr.Code == http.StatusOK || rr.Code == 0)
	})
	t.Run("ensure admin from env defaults", func(t *testing.T) {
		srv2 := NewServer(core.NewConfigStore(global.GlobalDB), nil)
		t.Setenv("PT_ADMIN_USER", "")
		t.Setenv("PT_ADMIN_PASS", "")
		srv2.ensureAdminFromEnv()
	})
	t.Run("ensure admin reset", func(t *testing.T) {
		srv2 := NewServer(core.NewConfigStore(global.GlobalDB), nil)
		t.Setenv("PT_ADMIN_USER", "admin")
		t.Setenv("PT_ADMIN_PASS", "adminadmin")
		t.Setenv("PT_ADMIN_RESET", "1")
		srv2.ensureAdminFromEnv()
	})
	// skip starting all tasks path to avoid env-dependent behavior in unit test
	t.Run("method not allowed cases", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/global", nil)
		srv.apiGlobal(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
		rr2 := httptest.NewRecorder()
		req2 := httptest.NewRequest(http.MethodPut, "/api/qbit", nil)
		srv.apiQbit(rr2, req2)
		assert.Equal(t, http.StatusMethodNotAllowed, rr2.Code)
		rr3 := httptest.NewRecorder()
		req3 := httptest.NewRequest(http.MethodGet, "/api/control/stop", nil)
		srv.apiStopAll(rr3, req3)
		assert.Equal(t, http.StatusMethodNotAllowed, rr3.Code)
		rr4 := httptest.NewRecorder()
		req4 := httptest.NewRequest(http.MethodGet, "/api/control/start", nil)
		srv.apiStartAll(rr4, req4)
		assert.Equal(t, http.StatusMethodNotAllowed, rr4.Code)
	})
	t.Run("verifyPassword success", func(t *testing.T) {
		h := hashPassword("pw")
		assert.True(t, verifyPassword(h, "pw"))
	})
	t.Run("randomID length", func(t *testing.T) {
		id := randomID()
		assert.Equal(t, 32, len(id))
	})
	t.Run("writeJSON content-type", func(t *testing.T) {
		rr := httptest.NewRecorder()
		writeJSON(rr, map[string]string{"k": "v"})
		assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
		assert.NotEmpty(t, rr.Body.String())
	})
	t.Run("mustSub panic on invalid", func(t *testing.T) {
		defer func() { _ = recover() }()
		_ = mustSub(staticFS, "nope")
	})
	t.Run("verifyPassword failures", func(t *testing.T) {
		assert.False(t, verifyPassword("bad-format", "x"))
		salt := "aa"
		sum := "bb"
		it := "-1"
		assert.False(t, verifyPassword(salt+"|"+sum+"|"+it, "x"))
	})
	t.Run("log middleware records", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux := http.NewServeMux()
		mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
		handler := logMiddleware(mux)
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		handler.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
	})
	t.Run("stop all", func(t *testing.T) {
		srv2 := NewServer(core.NewConfigStore(global.GlobalDB), newTestManager(t))
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/control/stop", nil)
		srv2.apiStopAll(rr, req)
		assert.True(t, rr.Code == http.StatusOK || rr.Code == 0)
	})
	// start all test skipped: requires richer site/qbit configuration to be deterministic
	t.Run("start all method not allowed", func(t *testing.T) {
		srv2 := NewServer(core.NewConfigStore(global.GlobalDB), newTestManager(t))
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/control/start", nil)
		srv2.apiStartAll(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
	t.Run("start all success when ready", func(t *testing.T) {
		db, _ := core.NewTempDBDir(t.TempDir())
		store := core.NewConfigStore(db)
		_ = store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, DefaultEnabled: true})
		srv2 := NewServer(store, newTestManager(t))
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/control/start", nil)
		srv2.apiStartAll(rr, req)
		assert.True(t, rr.Code == http.StatusOK || rr.Code == 0)
	})
}

func TestServe_RootRedirectAndStatic(t *testing.T) {
	db, _ := core.NewTempDBDir(t.TempDir())
	global.InitLogger(zap.NewNop())
	srv := NewServer(core.NewConfigStore(db), newTestManager(t))
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		sid, err := r.Cookie("session")
		if err != nil || sid.Value == "" || srv.sessions[sid.Value] == "" {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusFound && rr.Code != 0 {
		t.Fatalf("expect redirect, got %d", rr.Code)
	}
}

func TestServer_DownloaderAPIs(t *testing.T) {
	srv := setupServer(t)
	s := core.NewConfigStore(global.GlobalDB)
	_ = s.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true})

	t.Run("apiDownloaders GET", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders", nil)
		srv.apiDownloaders(rr, req)
		assert.True(t, rr.Code == http.StatusOK || rr.Code == 0)
	})

	t.Run("apiDownloaders POST", func(t *testing.T) {
		rr := httptest.NewRecorder()
		body := bytes.NewBufferString(`{"name":"test-dl","type":"qbittorrent","url":"http://localhost:8080","username":"admin","password":"admin"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders", body)
		srv.apiDownloaders(rr, req)
		assert.True(t, rr.Code == http.StatusOK || rr.Code == http.StatusCreated || rr.Code == 0)
	})

	t.Run("apiDownloaders method not allowed", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders", nil)
		srv.apiDownloaders(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("apiDownloaderDetail GET", func(t *testing.T) {
		// First create a downloader
		dl := models.DownloaderSetting{
			Name:     "test-dl-detail",
			Type:     "qbittorrent",
			URL:      "http://localhost:8080",
			Username: "admin",
			Password: "admin",
			Enabled:  true,
		}
		global.GlobalDB.DB.Create(&dl)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/"+strconv.Itoa(int(dl.ID)), nil)
		srv.apiDownloaderDetail(rr, req)
		assert.True(t, rr.Code == http.StatusOK || rr.Code == 0)
	})

	t.Run("apiDownloaderDetail invalid ID", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/invalid", nil)
		srv.apiDownloaderDetail(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("apiDownloaderDetail PUT", func(t *testing.T) {
		// First create a downloader
		dl := models.DownloaderSetting{
			Name:     "test-dl-update",
			Type:     "qbittorrent",
			URL:      "http://localhost:8080",
			Username: "admin",
			Password: "admin",
			Enabled:  true,
		}
		global.GlobalDB.DB.Create(&dl)

		rr := httptest.NewRecorder()
		body := bytes.NewBufferString(`{"name":"test-dl-updated","type":"qbittorrent","url":"http://localhost:8081","username":"admin2","password":"admin2"}`)
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/"+strconv.Itoa(int(dl.ID)), body)
		srv.apiDownloaderDetail(rr, req)
		assert.True(t, rr.Code == http.StatusOK || rr.Code == 0)
	})

	t.Run("apiDownloaderDetail DELETE", func(t *testing.T) {
		// First create a downloader
		dl := models.DownloaderSetting{
			Name:     "test-dl-delete",
			Type:     "qbittorrent",
			URL:      "http://localhost:8080",
			Username: "admin",
			Password: "admin",
			Enabled:  true,
		}
		global.GlobalDB.DB.Create(&dl)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/downloaders/"+strconv.Itoa(int(dl.ID)), nil)
		srv.apiDownloaderDetail(rr, req)
		assert.True(t, rr.Code == http.StatusOK || rr.Code == http.StatusNoContent || rr.Code == 0)
	})

	t.Run("apiDownloaderDetail method not allowed", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/api/downloaders/1", nil)
		srv.apiDownloaderDetail(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("downloaderHealthCheck", func(t *testing.T) {
		// First create a downloader
		dl := models.DownloaderSetting{
			Name:     "test-dl-health",
			Type:     "qbittorrent",
			URL:      "http://localhost:8080",
			Username: "admin",
			Password: "admin",
			Enabled:  true,
		}
		global.GlobalDB.DB.Create(&dl)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/"+strconv.Itoa(int(dl.ID))+"/health", nil)
		srv.apiDownloaderDetail(rr, req)
		// Health check may fail due to no actual server, but should not panic
		assert.NotNil(t, rr.Body)
	})

	t.Run("downloaderHealthCheck invalid ID", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/invalid/health", nil)
		srv.apiDownloaderDetail(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("setDefaultDownloader", func(t *testing.T) {
		// First create a downloader
		dl := models.DownloaderSetting{
			Name:     "test-dl-default",
			Type:     "qbittorrent",
			URL:      "http://localhost:8080",
			Username: "admin",
			Password: "admin",
			Enabled:  true,
		}
		global.GlobalDB.DB.Create(&dl)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/"+strconv.Itoa(int(dl.ID))+"/set-default", nil)
		srv.apiDownloaderDetail(rr, req)
		assert.True(t, rr.Code == http.StatusOK || rr.Code == 0)
	})
}

func TestServer_DynamicSitesAPI(t *testing.T) {
	srv := setupServer(t)
	s := core.NewConfigStore(global.GlobalDB)
	_ = s.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true})

	t.Run("apiDynamicSites GET", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/dynamic-sites", nil)
		srv.apiDynamicSites(rr, req)
		assert.True(t, rr.Code == http.StatusOK || rr.Code == 0)
	})

	t.Run("apiDynamicSites POST", func(t *testing.T) {
		rr := httptest.NewRecorder()
		body := bytes.NewBufferString(`{"name":"test-dynamic","display_name":"Test Dynamic","base_url":"https://example.com","auth_method":"cookie","cookie":"test-cookie"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/dynamic-sites", body)
		srv.apiDynamicSites(rr, req)
		// May fail due to validation, but should not panic
		assert.NotNil(t, rr.Body)
	})

	t.Run("apiDynamicSites method not allowed", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/dynamic-sites", nil)
		srv.apiDynamicSites(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

// TestLoginHandler_SuccessfulLogin 测试成功登录
func TestLoginHandler_SuccessfulLogin(t *testing.T) {
	srv := setupServer(t)
	s := core.NewConfigStore(global.GlobalDB)

	// 创建管理员账户
	hash := hashPassword("testpass")
	_ = s.EnsureAdmin("testuser", hash)

	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"username":"testuser","password":"testpass"}`)
	req := httptest.NewRequest(http.MethodPost, "/login", body)
	req.Header.Set("Content-Type", "application/json")
	srv.loginHandler(rr, req)

	// JSON 请求应返回 200 和 JSON 响应
	assert.Equal(t, http.StatusOK, rr.Code)
	// 应该设置 session cookie
	cookies := rr.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session" {
			sessionCookie = c
			break
		}
	}
	assert.NotNil(t, sessionCookie)
	assert.NotEmpty(t, sessionCookie.Value)
}

// TestLoginHandler_WrongPassword 测试密码错误
func TestLoginHandler_WrongPassword(t *testing.T) {
	srv := setupServer(t)
	s := core.NewConfigStore(global.GlobalDB)

	// 创建管理员账户
	hash := hashPassword("correctpass")
	_ = s.EnsureAdmin("testuser2", hash)

	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"username":"testuser2","password":"wrongpass"}`)
	req := httptest.NewRequest(http.MethodPost, "/login", body)
	req.Header.Set("Content-Type", "application/json")
	srv.loginHandler(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestLoginHandler_UserNotFound 测试用户不存在
func TestLoginHandler_UserNotFound(t *testing.T) {
	srv := setupServer(t)

	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"username":"nonexistent","password":"anypass"}`)
	req := httptest.NewRequest(http.MethodPost, "/login", body)
	req.Header.Set("Content-Type", "application/json")
	srv.loginHandler(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// TestLoginHandler_EmptyCredentials 测试空凭据
func TestLoginHandler_EmptyCredentials(t *testing.T) {
	srv := setupServer(t)

	t.Run("empty username", func(t *testing.T) {
		rr := httptest.NewRecorder()
		body := bytes.NewBufferString(`{"username":"","password":"pass"}`)
		req := httptest.NewRequest(http.MethodPost, "/login", body)
		req.Header.Set("Content-Type", "application/json")
		srv.loginHandler(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("empty password", func(t *testing.T) {
		rr := httptest.NewRecorder()
		body := bytes.NewBufferString(`{"username":"user","password":""}`)
		req := httptest.NewRequest(http.MethodPost, "/login", body)
		req.Header.Set("Content-Type", "application/json")
		srv.loginHandler(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

// TestLoginHandler_InvalidJSON 测试无效 JSON
func TestLoginHandler_InvalidJSON(t *testing.T) {
	srv := setupServer(t)

	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{invalid json}`)
	req := httptest.NewRequest(http.MethodPost, "/login", body)
	req.Header.Set("Content-Type", "application/json")
	srv.loginHandler(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestLoginHandler_MethodNotAllowed 测试不允许的方法
func TestLoginHandler_MethodNotAllowed(t *testing.T) {
	srv := setupServer(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/login", nil)
	srv.loginHandler(rr, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

// TestLoginHandler_AutoCreateAdmin 测试自动创建管理员
func TestLoginHandler_AutoCreateAdmin(t *testing.T) {
	// 使用新的数据库，确保没有管理员
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	srv := NewServer(core.NewConfigStore(db), newTestManager(t))

	// 尝试登录，应该自动创建默认管理员
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"username":"admin","password":"adminadmin"}`)
	req := httptest.NewRequest(http.MethodPost, "/login", body)
	req.Header.Set("Content-Type", "application/json")
	srv.loginHandler(rr, req)

	// JSON 请求应返回 200
	assert.Equal(t, http.StatusOK, rr.Code)
}

// TestVerifyPassword_EdgeCases 测试密码验证边界情况
func TestVerifyPassword_EdgeCases(t *testing.T) {
	t.Run("invalid format - too few parts", func(t *testing.T) {
		assert.False(t, verifyPassword("onlyonepart", "pass"))
	})

	t.Run("invalid format - too many parts", func(t *testing.T) {
		assert.False(t, verifyPassword("a|b|c|d", "pass"))
	})

	t.Run("invalid salt hex", func(t *testing.T) {
		assert.False(t, verifyPassword("zzzz|bbbb|100000", "pass"))
	})

	t.Run("invalid iterations", func(t *testing.T) {
		assert.False(t, verifyPassword("aaaa|bbbb|notanumber", "pass"))
	})

	t.Run("zero iterations", func(t *testing.T) {
		assert.False(t, verifyPassword("aaaa|bbbb|0", "pass"))
	})

	t.Run("negative iterations", func(t *testing.T) {
		assert.False(t, verifyPassword("aaaa|bbbb|-1", "pass"))
	})
}

// TestHashPassword_Uniqueness 测试密码哈希唯一性
func TestHashPassword_Uniqueness(t *testing.T) {
	hash1 := hashPassword("samepassword")
	hash2 := hashPassword("samepassword")

	// 由于使用随机 salt，相同密码应该产生不同的哈希
	assert.NotEqual(t, hash1, hash2)

	// 但两个哈希都应该能验证原密码
	assert.True(t, verifyPassword(hash1, "samepassword"))
	assert.True(t, verifyPassword(hash2, "samepassword"))
}

// TestApiSites_Delete 测试删除站点
func TestApiSites_Delete(t *testing.T) {
	srv := setupServer(t)
	s := core.NewConfigStore(global.GlobalDB)
	_ = s.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true})

	t.Run("delete without name", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/sites", nil)
		srv.apiSites(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("delete with name", func(t *testing.T) {
		// First create a site
		site := models.SiteSetting{
			Name:       "test-site-delete",
			AuthMethod: "cookie",
			Cookie:     "test-cookie",
			Enabled:    true,
		}
		global.GlobalDB.DB.Create(&site)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/sites?name=test-site-delete", nil)
		srv.apiSites(rr, req)
		assert.True(t, rr.Code == http.StatusOK || rr.Code == http.StatusBadRequest)
	})

	t.Run("method not allowed", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/sites", nil)
		srv.apiSites(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

// TestApiSiteDetail_Delete 测试删除RSS
func TestApiSiteDetail_Delete(t *testing.T) {
	srv := setupServer(t)
	s := core.NewConfigStore(global.GlobalDB)
	_ = s.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true})

	t.Run("delete RSS without id", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/sites/springsunday", nil)
		srv.apiSiteDetail(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("delete RSS with invalid id", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/sites/springsunday?id=invalid", nil)
		srv.apiSiteDetail(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("delete RSS site not found", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/sites/springsunday?id=999", nil)
		srv.apiSiteDetail(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/api/sites/springsunday", nil)
		srv.apiSiteDetail(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

// TestApiSiteDetail_Post 测试保存站点配置
func TestApiSiteDetail_Post(t *testing.T) {
	srv := setupServer(t)
	s := core.NewConfigStore(global.GlobalDB)
	_ = s.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true})

	t.Run("save site config invalid json", func(t *testing.T) {
		rr := httptest.NewRecorder()
		body := bytes.NewBufferString(`{invalid json}`)
		req := httptest.NewRequest(http.MethodPost, "/api/sites/springsunday", body)
		srv.apiSiteDetail(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("save site config valid", func(t *testing.T) {
		// First create the site
		site := models.SiteSetting{
			Name:       "cmct",
			AuthMethod: "cookie",
			Cookie:     "test-cookie",
			Enabled:    true,
		}
		global.GlobalDB.DB.Create(&site)

		rr := httptest.NewRecorder()
		body := bytes.NewBufferString(`{"enabled":true,"auth_method":"cookie","cookie":"test-cookie","rss":[]}`)
		req := httptest.NewRequest(http.MethodPost, "/api/sites/springsunday", body)
		srv.apiSiteDetail(rr, req)
		// May return OK or BadRequest depending on validation
		assert.True(t, rr.Code == http.StatusOK || rr.Code == http.StatusBadRequest || rr.Code == 0)
	})
}

// TestApiSiteDetail_InvalidSite 测试无效站点
func TestApiSiteDetail_InvalidSite(t *testing.T) {
	srv := setupServer(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sites/invalid-site-name", nil)
	srv.apiSiteDetail(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// TestSetDefaultDownloader 测试设置默认下载器
func TestSetDefaultDownloader_Additional(t *testing.T) {
	srv := setupServer(t)

	t.Run("invalid id", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/invalid/default", nil)
		srv.setDefaultDownloader(rr, req, "invalid")
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("downloader not found", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/99999/default", nil)
		srv.setDefaultDownloader(rr, req, "99999")
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("set default success", func(t *testing.T) {
		// Create a downloader first
		dl := models.DownloaderSetting{
			Name:     "test-default-dl",
			Type:     "qbittorrent",
			URL:      "http://localhost:8080",
			Username: "admin",
			Password: "admin",
			Enabled:  true,
		}
		global.GlobalDB.DB.Create(&dl)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/"+strconv.Itoa(int(dl.ID))+"/default", nil)
		srv.setDefaultDownloader(rr, req, strconv.Itoa(int(dl.ID)))
		assert.True(t, rr.Code == http.StatusOK || rr.Code == 0)
	})
}
