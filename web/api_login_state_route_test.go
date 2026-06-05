package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

func newRouteTestServer(t *testing.T) (*http.ServeMux, func()) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.SiteSetting{},
		&models.SiteLoginState{},
		&models.MigrationState{},
	))

	prevDB := global.GlobalDB
	global.GlobalDB = &models.TorrentDB{DB: db}

	resolver := &slowResolver{delay: 0}
	mon := scheduler.NewLoginReminderMonitor(scheduler.LoginReminderConfig{
		DB:        db,
		Resolver:  resolver,
		Decryptor: stubDecryptor{},
		Clock:     sitelogin.NewFakeClock(time.Now()),
		Logger:    zap.NewNop().Sugar(),
	})
	mgr := scheduler.NewManager()
	mgr.SetLoginReminderMonitor(mon)

	srv := &Server{
		mgr:      mgr,
		sessions: map[string]string{"sess-test": "admin"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/sites/", srv.auth(srv.apiSiteDetail))
	srv.registerLoginStateRoutes(mux)

	cleanup := func() {
		mgr.SetLoginReminderMonitor(nil)
		mgr.StopAll()
		global.GlobalDB = prevDB
	}
	return mux, cleanup
}

func authedReq(method, path string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "sess-test"})
	return req
}

// TestRESTfulProbePathRoutesToLoginStateHandler verifies that the RESTful URL
// shape /api/sites/{name}/login-state/probe is routed (through the full mux)
// to the login-state probe handler and NOT swallowed by the /api/sites/
// catch-all (which would 404). For an unknown site the probe handler returns
// 404 with a JSON body, but for a KNOWN, persisted site it must NOT return the
// bare catch-all 404.
func TestRESTfulProbePathRoutesToLoginStateHandler(t *testing.T) {
	mux, cleanup := newRouteTestServer(t)
	defer cleanup()

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{
		Name: "rousipro", Enabled: true,
	}).Error)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedReq(http.MethodPost, "/api/sites/rousipro/login-state/probe"))

	// The bare catch-all 404 (apiSiteDetail) writes an EMPTY body via
	// w.WriteHeader(http.StatusNotFound). If the request reached the
	// login-state probe handler, it must NOT be that empty-body 404.
	if rec.Code == http.StatusNotFound {
		assert.NotEmpty(t, rec.Body.String(),
			"got bare catch-all 404 with empty body — request was NOT forwarded to login-state probe handler (regression)")
	}
	assert.NotEqual(t, http.StatusMethodNotAllowed, rec.Code,
		"probe handler accepts POST; 405 would mean wrong routing")
}

func TestRESTfulProbeUnknownSiteReturnsJSON404(t *testing.T) {
	mux, cleanup := newRouteTestServer(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedReq(http.MethodPost, "/api/sites/does-not-exist/login-state/probe"))

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "不存在",
		"unknown-site 404 should come from the login-state probe handler (JSON body), not the empty catch-all 404")
}

func TestRESTfulConfigPathRoutesToLoginStateHandler(t *testing.T) {
	mux, cleanup := newRouteTestServer(t)
	defer cleanup()

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{
		Name: "rousipro", Enabled: true,
	}).Error)

	rec := httptest.NewRecorder()
	req := authedReq(http.MethodPut, "/api/sites/rousipro/login-state/config")
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	// A PUT to the catch-all apiSiteDetail would hit its default branch and
	// return 405 (method not allowed). Reaching the config handler must not.
	assert.NotEqual(t, http.StatusMethodNotAllowed, rec.Code,
		"PUT config must be forwarded to handleLoginStateConfigUpdate, not the catch-all default 405")
}

func TestLegacyProbePathStillWorks(t *testing.T) {
	mux, cleanup := newRouteTestServer(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedReq(http.MethodPost, "/api/sites/login-state/does-not-exist/probe"))

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "不存在",
		"legacy path must still reach the login-state probe handler")
}

func TestPlainSiteDetailPathUnaffected(t *testing.T) {
	mux, cleanup := newRouteTestServer(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedReq(http.MethodGet, "/api/sites/totally-unknown-site"))

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
