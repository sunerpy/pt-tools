package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
