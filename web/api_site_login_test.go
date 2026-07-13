package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
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
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// ==== merged from api_login_state_route_test.go ====
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

// ==== merged from api_misc_cov3_test.go ====
func TestApiDownloaderTorrentDetail_FilesTrackersErrFallback(t *testing.T) {
	fake := &fakeDownloader{
		torrents:   sampleTorrents(),
		filesErr:   assertErr("filesfail"),
		trackerErr: assertErr("trackerfail"),
	}
	server, _ := setupServerWithFakeDownloader(t, fake)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/detail?downloader_id=1&task_id=t1", nil)
	server.apiDownloaderTorrentDetail(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp TorrentDetailResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.Files)
	assert.Empty(t, resp.Trackers)
}

func TestApiDownloaderTorrentDetail_DownloaderConnFail(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "unreach", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true,
	}).Error)
	mgr := scheduler.NewManager()
	t.Cleanup(func() { mgr.StopAll() })
	server.mgr = mgr

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/detail?downloader_id=1&task_id=t1", nil)
	server.apiDownloaderTorrentDetail(w, req)
	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestApiSiteLoginStateVisit_NilDB(t *testing.T) {
	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })

	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/visit", nil)
	s.apiSiteLoginStateVisit(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestApiSiteLoginStateVisit_BadTimestamp(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	body := []byte(`{"site_name":"hdsky","last_visit_at":"not-a-time"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/visit", bytes.NewReader(body))
	srv.apiSiteLoginStateVisit(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

var _ = context.Background

// ==== merged from api_probe_singleflight_test.go ====
type slowResolver struct {
	calls atomic.Int32
	delay time.Duration
}

func (s *slowResolver) Resolve(_ models.SiteSetting) (*v2.SiteDefinition, v2.Site, error) {
	s.calls.Add(1)
	time.Sleep(s.delay)
	return nil, nil, errResolveStub
}

var errResolveStub = stubError("resolver disabled in test")

type stubError string

func (e stubError) Error() string { return string(e) }

type stubDecryptor struct{}

func (stubDecryptor) Decrypt(_ models.SiteSetting) (string, error) { return "", nil }

func newProbeSingleFlightServer(t *testing.T, resolveDelay time.Duration) (*Server, *slowResolver, *scheduler.LoginReminderMonitor, func()) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(
		&models.SiteSetting{},
		&models.SiteLoginState{},
		&models.MigrationState{},
	))

	prevDB := global.GlobalDB
	global.GlobalDB = &models.TorrentDB{DB: db}

	resolver := &slowResolver{delay: resolveDelay}
	mon := scheduler.NewLoginReminderMonitor(scheduler.LoginReminderConfig{
		DB:        db,
		Router:    nil,
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
	cleanup := func() {
		mgr.SetLoginReminderMonitor(nil)
		mgr.StopAll()
		global.GlobalDB = prevDB
	}
	return srv, resolver, mon, cleanup
}

func issueProbe(srv *Server, siteName string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/"+siteName+"/probe", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "sess-test"})
	srv.apiSiteLoginStateRouter(rec, req)
	return rec
}

func TestProbeSingleFlight_Concurrent(t *testing.T) {
	srv, resolver, _, cleanup := newProbeSingleFlightServer(t, 200*time.Millisecond)
	defer cleanup()

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{
		Name: "HDSKY", Enabled: true,
	}).Error)

	var wg sync.WaitGroup
	results := make([]int, 2)
	starts := make([]time.Time, 2)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		i := i
		go func() {
			defer wg.Done()
			starts[i] = time.Now()
			rec := issueProbe(srv, "HDSKY")
			results[i] = rec.Code
			if rec.Code == http.StatusConflict {
				var body map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				assert.Equal(t, "probe_in_progress", body["error"])
				assert.Contains(t, body["message"], "探测进行中")
			}
		}()
		time.Sleep(20 * time.Millisecond)
	}
	wg.Wait()

	codes := []int{results[0], results[1]}
	successCount := 0
	conflictCount := 0
	for _, c := range codes {
		switch c {
		case http.StatusOK:
			successCount++
		case http.StatusConflict:
			conflictCount++
		default:
			t.Fatalf("unexpected status code %d", c)
		}
	}
	assert.Equal(t, 1, successCount, "exactly one probe should succeed; got codes=%v", codes)
	assert.Equal(t, 1, conflictCount, "exactly one probe should be rejected with 409; got codes=%v", codes)
	assert.Equal(t, int32(1), resolver.calls.Load(), "resolver should be invoked exactly once (single-flight)")
}

func TestProbeSingleFlight_DifferentSitesIndependent(t *testing.T) {
	srv, resolver, _, cleanup := newProbeSingleFlightServer(t, 150*time.Millisecond)
	defer cleanup()

	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "MTEAM", Enabled: true}).Error)

	var wg sync.WaitGroup
	codes := make([]int, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		codes[0] = issueProbe(srv, "HDSKY").Code
	}()
	go func() {
		defer wg.Done()
		codes[1] = issueProbe(srv, "MTEAM").Code
	}()
	wg.Wait()

	assert.Equal(t, http.StatusOK, codes[0], "HDSKY probe should succeed")
	assert.Equal(t, http.StatusOK, codes[1], "MTEAM probe should succeed concurrently")
	assert.Equal(t, int32(2), resolver.calls.Load(), "both probes should run (per-site mutex, not global)")
}

func TestProbeSingleFlight_SequentialOk(t *testing.T) {
	srv, resolver, _, cleanup := newProbeSingleFlightServer(t, 20*time.Millisecond)
	defer cleanup()

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{
		Name: "HDSKY", Enabled: true,
	}).Error)

	first := issueProbe(srv, "HDSKY")
	require.Equal(t, http.StatusOK, first.Code, "first probe should succeed; body=%s", first.Body.String())

	second := issueProbe(srv, "HDSKY")
	require.Equal(t, http.StatusOK, second.Code, "second sequential probe should succeed after first completes; body=%s", second.Body.String())

	assert.Equal(t, int32(2), resolver.calls.Load(), "both sequential probes should invoke the resolver")
}

func TestProbeSingleFlight_CronAndManualSharedLock(t *testing.T) {
	srv, resolver, mon, cleanup := newProbeSingleFlightServer(t, 200*time.Millisecond)
	defer cleanup()

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{
		Name: "HDSKY", Enabled: true,
	}).Error)

	release, ok := mon.TryAcquireProbeLock("HDSKY")
	require.True(t, ok, "test setup: should acquire lock as cron would")

	rec := issueProbe(srv, "HDSKY")
	assert.Equal(t, http.StatusConflict, rec.Code, "manual REST probe must see cron's lock and return 409")
	assert.Equal(t, int32(0), resolver.calls.Load(), "manual handler must not run probe while cron holds lock")

	release()

	rec2 := issueProbe(srv, "HDSKY")
	assert.Equal(t, http.StatusOK, rec2.Code, "after cron releases, manual probe should succeed")
	assert.Equal(t, int32(1), resolver.calls.Load(), "exactly one probe runs after cron release")
}

var _ = context.Background

// ==== merged from api_site_login_config_test.go ====
func TestHandleLoginStateConfigUpdate_Cov(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.handleLoginStateConfigUpdate(rec, authedRequest(http.MethodGet, "/api/sites/login-state/hdsky/config", nil), "hdsky")
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("invalid cron", func(t *testing.T) {
		cron := "bad cron"
		rec := httptest.NewRecorder()
		srv.handleLoginStateConfigUpdate(rec,
			authedRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", SiteLoginConfigUpdateRequest{ReminderCron: &cron}), "hdsky")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("ban threshold out of range", func(t *testing.T) {
		bad := 9999
		rec := httptest.NewRecorder()
		srv.handleLoginStateConfigUpdate(rec,
			authedRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", SiteLoginConfigUpdateRequest{BanThresholdDays: &bad}), "hdsky")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("remind before out of range", func(t *testing.T) {
		bad := 9999
		rec := httptest.NewRecorder()
		srv.handleLoginStateConfigUpdate(rec,
			authedRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", SiteLoginConfigUpdateRequest{RemindBeforeDays: &bad}), "hdsky")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("invalid probe mode", func(t *testing.T) {
		bad := "bogus"
		rec := httptest.NewRecorder()
		srv.handleLoginStateConfigUpdate(rec,
			authedRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", SiteLoginConfigUpdateRequest{ProbeMode: &bad}), "hdsky")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("no fields provided", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.handleLoginStateConfigUpdate(rec,
			authedRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", SiteLoginConfigUpdateRequest{}), "hdsky")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("valid multi-field update", func(t *testing.T) {
		ban := 45
		remind := 15
		cron := "0 9 * * *"
		mode := "manual"
		chans := []uint{1, 2}
		rec := httptest.NewRecorder()
		srv.handleLoginStateConfigUpdate(rec,
			authedRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", SiteLoginConfigUpdateRequest{
				BanThresholdDays: &ban, RemindBeforeDays: &remind, ReminderCron: &cron,
				ProbeMode: &mode, NotificationChannelIDs: &chans,
			}), "hdsky")
		require.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestLoginHandler_JSONSuccess(t *testing.T) {
	srv := setupServer(t)
	require.NoError(t, srv.store.EnsureAdmin("admin", hashPassword("secret")))

	rec := httptest.NewRecorder()
	req := authedRequest(http.MethodPost, "/login", map[string]string{"username": "admin", "password": "secret"})
	req.Header.Set("Content-Type", "application/json")
	srv.loginHandler(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "success")
}

func TestParseVisitTimestamp_Cov(t *testing.T) {
	_, err := parseVisitTimestamp("")
	assert.Error(t, err)
	_, err = parseVisitTimestamp("garbage")
	assert.Error(t, err)
	ts, err := parseVisitTimestamp("2024-01-02T03:04:05Z")
	require.NoError(t, err)
	assert.Equal(t, 2024, ts.Year())
}

func TestApiSiteLoginStateList_NilDB(t *testing.T) {
	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })

	srv := &Server{sessions: map[string]string{"sess-test": "admin"}}
	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateList(rec, authedRequest(http.MethodGet, "/api/sites/login-state", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// ==== merged from api_site_login_cov2_test.go ====
type loginCovResolver struct{}

func (loginCovResolver) Resolve(models.SiteSetting) (*v2.SiteDefinition, v2.Site, error) {
	return &v2.SiteDefinition{ID: "hdsky"}, &fakeV2Site{id: "hdsky", name: "HDSky"}, nil
}

type loginCovDecryptor struct{}

func (loginCovDecryptor) Decrypt(models.SiteSetting) (string, error) { return "cookie=1", nil }

func newLoginMonitorServer(t *testing.T) *Server {
	t.Helper()
	srv, cleanup := newSiteLoginTestServer(t)
	t.Cleanup(cleanup)
	mgr := scheduler.NewManager()
	t.Cleanup(func() { mgr.StopAll() })
	mon := scheduler.NewLoginReminderMonitor(scheduler.LoginReminderConfig{
		DB:        global.GlobalDB.DB,
		Resolver:  loginCovResolver{},
		Decryptor: loginCovDecryptor{},
	})
	mgr.SetLoginReminderMonitor(mon)
	srv.mgr = mgr
	return srv
}

func TestApiSiteLoginStateList_WithSites(t *testing.T) {
	srv := newLoginMonitorServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", DisplayName: "HDSky", Enabled: true}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state", nil)
		srv.apiSiteLoginStateList(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("list synthesizes defaults", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/login-state", nil)
		srv.apiSiteLoginStateList(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var out []SiteLoginStateResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		require.Len(t, out, 1)
		assert.Equal(t, "hdsky", out[0].SiteName)
	})
}

func TestHandleLoginStateProbe_Success(t *testing.T) {
	srv := newLoginMonitorServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/login-state/hdsky/probe", nil)
		srv.handleLoginStateProbe(w, req, "hdsky")
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("unknown site 404", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/nope/probe", nil)
		srv.handleLoginStateProbe(w, req, "nope")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("probe runs", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/hdsky/probe", nil)
		srv.handleLoginStateProbe(w, req, "hdsky")
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestHandleLoginStateTestReminder_Paths(t *testing.T) {
	srv := newLoginMonitorServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/login-state/hdsky/test-reminder", nil)
		srv.handleLoginStateTestReminder(w, req, "hdsky")
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("unknown site 404", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/nope/test-reminder", nil)
		srv.handleLoginStateTestReminder(w, req, "nope")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("router nil returns 500", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/hdsky/test-reminder", nil)
		srv.handleLoginStateTestReminder(w, req, "hdsky")
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandleLoginStateConfigUpdate_Success(t *testing.T) {
	srv := newLoginMonitorServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	ban := 40
	remind := 12
	cron := "0 8 * * *"
	mode := "manual"
	ids := []uint{1, 2}
	body, _ := json.Marshal(SiteLoginConfigUpdateRequest{
		BanThresholdDays:       &ban,
		RemindBeforeDays:       &remind,
		ReminderCron:           &cron,
		NotificationChannelIDs: &ids,
		ProbeMode:              &mode,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", bytes.NewReader(body))
	srv.handleLoginStateConfigUpdate(w, req, "hdsky")
	require.Equal(t, http.StatusOK, w.Code)
}

func TestHandleLoginStateConfigUpdate_Validation(t *testing.T) {
	srv := newLoginMonitorServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/hdsky/config", nil)
		srv.handleLoginStateConfigUpdate(w, req, "hdsky")
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("bad cron", func(t *testing.T) {
		bad := "not a cron"
		body, _ := json.Marshal(SiteLoginConfigUpdateRequest{ReminderCron: &bad})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", bytes.NewReader(body))
		srv.handleLoginStateConfigUpdate(w, req, "hdsky")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("ban out of range", func(t *testing.T) {
		bad := 999
		body, _ := json.Marshal(SiteLoginConfigUpdateRequest{BanThresholdDays: &bad})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", bytes.NewReader(body))
		srv.handleLoginStateConfigUpdate(w, req, "hdsky")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("bad probe mode", func(t *testing.T) {
		bad := "weird"
		body, _ := json.Marshal(SiteLoginConfigUpdateRequest{ProbeMode: &bad})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", bytes.NewReader(body))
		srv.handleLoginStateConfigUpdate(w, req, "hdsky")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("no fields", func(t *testing.T) {
		body, _ := json.Marshal(SiteLoginConfigUpdateRequest{})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", bytes.NewReader(body))
		srv.handleLoginStateConfigUpdate(w, req, "hdsky")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("bad body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", bytes.NewReader([]byte(`{bad`)))
		srv.handleLoginStateConfigUpdate(w, req, "hdsky")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

var _ = context.Background

// ==== merged from api_site_login_cov3_test.go ====
func TestHandleLoginStateConfigUpdate_ChannelIDs(t *testing.T) {
	srv := newLoginMonitorServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	ids := []uint{3, 7}
	body, _ := json.Marshal(SiteLoginConfigUpdateRequest{NotificationChannelIDs: &ids})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", bytes.NewReader(body))
	srv.handleLoginStateConfigUpdate(w, req, "hdsky")
	require.Equal(t, http.StatusOK, w.Code)

	state, err := loadOrInitLoginState(global.GlobalDB.DB, "hdsky")
	require.NoError(t, err)
	require.NotEmpty(t, state.NotificationChannelIDs)
}

func TestApiSiteDetail_TestReminderRoute(t *testing.T) {
	srv := newLoginMonitorServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/sites/hdsky/login-state/test-reminder", nil)
	srv.apiSiteDetail(w, req)
	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestApiSiteDetail_ConfigRoute(t *testing.T) {
	srv := newLoginMonitorServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	mode := "manual"
	body, _ := json.Marshal(SiteLoginConfigUpdateRequest{ProbeMode: &mode})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/sites/hdsky/login-state/config", bytes.NewReader(body))
	srv.apiSiteDetail(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

// ==== merged from api_site_login_cov_test.go ====
func TestHandleLoginStateGet_Cov(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{
		Name: "hdsky", DisplayName: "HDSky", Enabled: true, BaseURL: "https://hdsky.me",
	}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.handleLoginStateGet(rec, authedRequest(http.MethodPost, "/api/sites/hdsky/login-state", nil), "hdsky")
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("site not found", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.handleLoginStateGet(rec, authedRequest(http.MethodGet, "/api/sites/nosuch/login-state", nil), "nosuch")
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("returns state for existing site", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.handleLoginStateGet(rec, authedRequest(http.MethodGet, "/api/sites/hdsky/login-state", nil), "hdsky")
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "hdsky")
	})
}

func TestHandleLoginStateTestReminder_Cov(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	t.Run("method not allowed", func(t *testing.T) {
		srv.mgr = nil
		rec := httptest.NewRecorder()
		srv.handleLoginStateTestReminder(rec, authedRequest(http.MethodGet, "/api/sites/hdsky/login-state/test-reminder", nil), "hdsky")
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("monitor not initialized", func(t *testing.T) {
		mgr := scheduler.NewManager()
		t.Cleanup(func() { mgr.StopAll() })
		srv.mgr = mgr
		rec := httptest.NewRecorder()
		srv.handleLoginStateTestReminder(rec, authedRequest(http.MethodPost, "/api/sites/hdsky/login-state/test-reminder", nil), "hdsky")
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})
}

func TestBuildLoginStateResponse_Cov(t *testing.T) {
	now := time.Now()
	past := now.Add(-24 * time.Hour)

	site := models.SiteSetting{Name: "hdsky", DisplayName: "HDSky", Enabled: true, BaseURL: "https://hdsky.me"}

	t.Run("with active timestamps computes tier", func(t *testing.T) {
		state := models.SiteLoginState{
			SiteName:         "hdsky",
			LastAccessAt:     &past,
			BanThresholdDays: 30,
			RemindBeforeDays: 10,
		}
		resp := buildLoginStateResponse(site, state, now)
		assert.Equal(t, "hdsky", resp.SiteName)
		require.NotNil(t, resp.EffectiveLastActiveAt)
		assert.NotEqual(t, "unknown", resp.Tier)
	})

	t.Run("no active timestamp yields unknown tier", func(t *testing.T) {
		state := models.SiteLoginState{SiteName: "hdsky", BanThresholdDays: 30, RemindBeforeDays: 10}
		resp := buildLoginStateResponse(site, state, now)
		assert.Equal(t, "unknown", resp.Tier)
		assert.Nil(t, resp.EffectiveLastActiveAt)
	})
}

func TestTimestampPtr(t *testing.T) {
	assert.Nil(t, timestampPtr(nil))
	zero := time.Time{}
	assert.Nil(t, timestampPtr(&zero))
	now := time.Now()
	got := timestampPtr(&now)
	require.NotNil(t, got)
	assert.Equal(t, now.Unix(), *got)
}

// ==== merged from api_site_login_router_cov3_test.go ====
func TestApiSiteLoginStateRouter_DispatchCov3(t *testing.T) {
	srv := newLoginMonitorServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	t.Run("nil db", func(t *testing.T) {
		prev := global.GlobalDB
		global.GlobalDB = nil
		t.Cleanup(func() { global.GlobalDB = prev })
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/login-state/hdsky", nil)
		srv.apiSiteLoginStateRouter(w, req)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})

	t.Run("missing site name", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/login-state/", nil)
		srv.apiSiteLoginStateRouter(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("get action", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/login-state/hdsky", nil)
		srv.apiSiteLoginStateRouter(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("probe action", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/hdsky/probe", nil)
		srv.apiSiteLoginStateRouter(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("config action", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", bytes.NewReader([]byte(`{"probe_mode":"auto"}`)))
		srv.apiSiteLoginStateRouter(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("test-reminder action", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/hdsky/test-reminder", nil)
		srv.apiSiteLoginStateRouter(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("unknown action", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/login-state/hdsky/bogus", nil)
		srv.apiSiteLoginStateRouter(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandleLoginStateGet_NotFound(t *testing.T) {
	srv := newLoginMonitorServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sites/login-state/no-such-site", nil)
	srv.handleLoginStateGet(w, req, "no-such-site")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ==== merged from api_site_login_test.go ====
func newSiteLoginTestServer(t *testing.T) (*Server, func()) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}, &models.SiteLoginState{}))

	prevDB := global.GlobalDB
	global.GlobalDB = &models.TorrentDB{DB: db}

	srv := &Server{
		sessions: map[string]string{"sess-test": "admin"},
	}
	cleanup := func() { global.GlobalDB = prevDB }
	return srv, cleanup
}

func authedRequest(method, path string, body any) *http.Request {
	var reader *bytes.Reader
	if body != nil {
		buf, _ := json.Marshal(body)
		reader = bytes.NewReader(buf)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.AddCookie(&http.Cookie{Name: "session", Value: "sess-test"})
	return req
}

func TestApiSiteLoginStateListEmpty(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateList(rec, authedRequest(http.MethodGet, "/api/sites/login-state", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var out []SiteLoginStateResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Empty(t, out)
}

func TestApiSiteLoginStateListIncludesAllSites(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", DisplayName: "HDSky", BaseURL: "https://hdsky.me/", Enabled: true}).Error)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "MTEAM", DisplayName: "M-Team", BaseURL: "https://kp.m-team.cc/", Enabled: false}).Error)

	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateList(rec, authedRequest(http.MethodGet, "/api/sites/login-state", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var out []SiteLoginStateResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out, 2)

	for _, row := range out {
		assert.NotEmpty(t, row.SiteName)
		assert.NotZero(t, row.BanThresholdDays, "default ban threshold should populate")
		assert.NotZero(t, row.RemindBeforeDays)
		raw, _ := json.Marshal(row)
		body := strings.ToLower(string(raw))
		assert.NotContains(t, body, "\"cookie\":", "response must not leak cookie field")
		assert.NotContains(t, body, "\"cookie_encrypted\":", "response must not leak cookie_encrypted")
	}
}

func TestApiSiteLoginStateConfigUpdateSuccess(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	body := SiteLoginConfigUpdateRequest{
		BanThresholdDays:       intPtr(60),
		RemindBeforeDays:       intPtr(20),
		ReminderCron:           strPtr("0 8,20 * * *"),
		NotificationChannelIDs: uintSlicePtr([]uint{1, 3}),
	}
	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateRouter(rec, authedRequest(http.MethodPut, "/api/sites/login-state/HDSKY/config", body))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var state models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&state).Error)
	assert.Equal(t, 60, state.BanThresholdDays)
	assert.Equal(t, 20, state.RemindBeforeDays)
	assert.Equal(t, "0 8,20 * * *", state.ReminderCron)
	assert.Equal(t, "[1,3]", state.NotificationChannelIDs)
}

func TestApiSiteLoginStateConfigUpdateRejectsInvalidCron(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	body := SiteLoginConfigUpdateRequest{ReminderCron: strPtr("invalid cron")}
	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateRouter(rec, authedRequest(http.MethodPut, "/api/sites/login-state/HDSKY/config", body))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "cron")
}

func TestApiSiteLoginStateConfigUpdateRejectsBadBounds(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	body := SiteLoginConfigUpdateRequest{BanThresholdDays: intPtr(0)}
	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateRouter(rec, authedRequest(http.MethodPut, "/api/sites/login-state/HDSKY/config", body))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestApiSiteLoginStateConfigUpdatePersistsProbeMode(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	body := SiteLoginConfigUpdateRequest{ProbeMode: strPtr("manual")}
	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateRouter(rec, authedRequest(http.MethodPut, "/api/sites/login-state/HDSKY/config", body))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var state models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&state).Error)
	assert.Equal(t, "manual", state.ProbeMode)
}

func TestApiSiteLoginStateConfigUpdateRejectsInvalidProbeMode(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	body := SiteLoginConfigUpdateRequest{ProbeMode: strPtr("bogus")}
	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateRouter(rec, authedRequest(http.MethodPut, "/api/sites/login-state/HDSKY/config", body))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "probe_mode")
}

func TestApiSiteVisitClampsFutureTimestamp(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	future := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	body := SiteVisitReportRequest{SiteName: "HDSKY", LastVisitAt: future}
	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateVisit(rec, authedRequest(http.MethodPost, "/api/sites/visit", body))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var state models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&state).Error)
	require.NotNil(t, state.LastVisitAt)
	assert.True(t, state.LastVisitAt.Before(time.Now().Add(time.Minute)),
		"server must clamp future timestamp to now (got %s)", state.LastVisitAt)
}

func TestApiSiteVisitRejectsBadTimestamp(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	body := SiteVisitReportRequest{SiteName: "HDSKY", LastVisitAt: "not-a-date"}
	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateVisit(rec, authedRequest(http.MethodPost, "/api/sites/visit", body))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestApiSiteVisitRejectsEmptySiteName(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	body := SiteVisitReportRequest{SiteName: "", LastVisitAt: time.Now().UTC().Format(time.RFC3339)}
	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateVisit(rec, authedRequest(http.MethodPost, "/api/sites/visit", body))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestApiSiteLoginStateGetSingle(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", DisplayName: "HDSky", Enabled: true}).Error)

	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateRouter(rec, authedRequest(http.MethodGet, "/api/sites/login-state/HDSKY", nil))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var resp SiteLoginStateResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "HDSKY", resp.SiteName)
	assert.Equal(t, "HDSky", resp.DisplayName)
}

func TestParseChannelIDs(t *testing.T) {
	cases := []struct {
		raw  string
		want []uint
	}{
		{"", []uint{}},
		{"[]", []uint{}},
		{"[1,3,5]", []uint{1, 3, 5}},
		{"not json", []uint{}},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			assert.Equal(t, tc.want, parseChannelIDs(tc.raw))
		})
	}
}

func TestApiSiteLoginStateListProbeModeSynthesizedDefault(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "testsite", Enabled: true}).Error)

	rec := httptest.NewRecorder()
	srv.apiSiteLoginStateList(rec, authedRequest(http.MethodGet, "/api/sites/login-state", nil))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var out []SiteLoginStateResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out, 1)
	require.Equal(t, "testsite", out[0].SiteName)
	assert.Equal(t, "auto", out[0].ProbeMode, "synthesized SiteLoginState must have ProbeMode='auto', not ''")
}

func intPtr(v int) *int             { return &v }
func strPtr(v string) *string       { return &v }
func uintSlicePtr(v []uint) *[]uint { return &v }
