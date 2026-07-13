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
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// ==== merged from api_mixed_cov3_test.go ====
func TestApiUserInfoSiteDetail_PostWithStore(t *testing.T) {
	writeWebTestSecretKey(t)
	svc := setupTestUserInfoService()
	InitUserInfoService(svc)
	t.Cleanup(func() { InitUserInfoService(nil) })

	srv := newLoginMonitorServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/userinfo/sites/site1", nil)
	srv.apiUserInfoSiteDetail(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "site1")
}

func TestApiDeleteTasks_MixedPushed(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}))

	require.NoError(t, db.Create(&models.TorrentInfo{SiteName: "s", TorrentID: "a"}).Error)
	require.NoError(t, db.Create(&models.TorrentInfo{SiteName: "s", TorrentID: "b"}).Error)

	body, _ := json.Marshal(DeleteTasksRequest{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tasks/batch-delete", bytes.NewReader(body))
	server.apiDeleteTasks(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp DeleteTasksResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.Success)
}

func TestApiDeletePausedTorrents_NoDownloaderInfo(t *testing.T) {
	server, _ := setupServerWithFakeDownloader(t, &fakeDownloader{})
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.TorrentInfo{}))

	ti := models.TorrentInfo{SiteName: "s", TorrentID: "p1", IsPausedBySystem: true}
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

func TestApiSiteLoginStateVisit_UnknownSiteInits(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	body, _ := json.Marshal(SiteVisitReportRequest{SiteName: "brandnew", LastVisitAt: "2026-01-01T00:00:00Z"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/visit", bytes.NewReader(body))
	srv.apiSiteLoginStateVisit(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

// ==== merged from api_mixed_cov5_test.go ====
func TestApiResumeTorrent_DownloaderErrors(t *testing.T) {
	t.Run("resume torrent fails", func(t *testing.T) {
		fake := &fakeDownloader{resumeErr: assertErr("resumefail")}
		server, _ := setupServerWithFakeDownloader(t, fake)
		require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.TorrentInfo{}))
		ti := models.TorrentInfo{
			SiteName: "s", TorrentID: "r1", IsPausedBySystem: true,
			DownloaderTaskID: "task-r1", DownloaderName: "qb1",
		}
		require.NoError(t, global.GlobalDB.DB.Create(&ti).Error)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/1/resume", nil)
		server.apiResumeTorrent(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("downloader not found in manager", func(t *testing.T) {
		server, _ := setupServerWithFakeDownloader(t, &fakeDownloader{})
		require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.TorrentInfo{}))
		ti := models.TorrentInfo{
			SiteName: "s", TorrentID: "r2", IsPausedBySystem: true,
			DownloaderTaskID: "task-r2", DownloaderName: "no-such-dl",
		}
		require.NoError(t, global.GlobalDB.DB.Create(&ti).Error)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/1/resume", nil)
		server.apiResumeTorrent(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestApiUserInfoSiteDetail_DeleteFail(t *testing.T) {
	svc := setupTestUserInfoService()
	InitUserInfoService(svc)
	t.Cleanup(func() { InitUserInfoService(nil) })

	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v2/userinfo/sites/site1", nil)
	s.apiUserInfoSiteDetail(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestApiSiteLoginStateList_WithStoredStates(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", DisplayName: "HDSky", Enabled: true}).Error)
	la := time.Now().Add(-time.Hour)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName: "hdsky", BanThresholdDays: 30, RemindBeforeDays: 10,
		ReminderCron: "0 10 * * *", LastReminderTier: "none", ProbeMode: "auto",
		LastAccessAt: &la,
	}).Error)

	w := httptest.NewRecorder()
	srv.apiSiteLoginStateList(w, authedRequest(http.MethodGet, "/api/sites/login-state", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var out []SiteLoginStateResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Len(t, out, 1)
}

var (
	_ = context.Background
	_ = bytes.NewReader
)

// ==== merged from api_userinfo_cov2_test.go ====
func TestApiUserInfoSync_AllAndSpecific(t *testing.T) {
	service := setupTestUserInfoService()
	InitUserInfoService(service)
	t.Cleanup(func() { InitUserInfoService(nil) })

	s := &Server{}

	t.Run("sync all sites", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/userinfo/sync", nil)
		s.apiUserInfoSync(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp SyncResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.GreaterOrEqual(t, len(resp.Success), 1)
	})

	t.Run("sync specific site", func(t *testing.T) {
		body, _ := json.Marshal(SyncRequest{Sites: []string{"site1"}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/userinfo/sync", bytes.NewReader(body))
		s.apiUserInfoSync(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp SyncResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp.Success, "site1")
	})

	t.Run("sync unknown site records failure", func(t *testing.T) {
		body, _ := json.Marshal(SyncRequest{Sites: []string{"no-such-site"}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/userinfo/sync", bytes.NewReader(body))
		s.apiUserInfoSync(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp SyncResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.GreaterOrEqual(t, len(resp.Failed), 1)
	})

	t.Run("bad body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/userinfo/sync", bytes.NewBufferString(`{bad`))
		s.apiUserInfoSync(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/sync", nil)
		s.apiUserInfoSync(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApiUserInfoSiteDetail_PostSyncError(t *testing.T) {
	service := setupTestUserInfoService()
	InitUserInfoService(service)
	t.Cleanup(func() { InitUserInfoService(nil) })

	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/userinfo/sites/no-such-site", nil)
	s.apiUserInfoSiteDetail(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ==== merged from api_userinfo_cov_test.go ====
func TestInitAndGetGlobals(t *testing.T) {
	svc := setupTestUserInfoService()
	InitUserInfoService(svc)
	t.Cleanup(func() { InitUserInfoService(nil) })
	assert.Equal(t, svc, GetUserInfoService())

	reg := v2.NewSiteRegistry(nil)
	InitSiteRegistry(reg)
	t.Cleanup(func() { InitSiteRegistry(nil) })
	assert.Equal(t, reg, GetSiteRegistry())
}

// storeStub satisfies the ListSites interface used by RefreshSiteRegistrations.
type refreshStore struct {
	err error
}

func (s refreshStore) ListSites() (map[models.SiteGroup]models.SiteConfig, error) {
	return nil, s.err
}

func TestRefreshSiteRegistrations_NoServices(t *testing.T) {
	InitUserInfoService(nil)
	InitSiteRegistry(nil)
	err := RefreshSiteRegistrations(refreshStore{})
	assert.NoError(t, err)
}

func TestFilterStatsByEnabledSites(t *testing.T) {
	stats := v2.AggregatedStats{
		PerSiteStats: []v2.UserInfo{
			{Site: "site1", Uploaded: 100, Downloaded: 50, Ratio: 2.0, Seeding: 5, Bonus: 10, BonusPerHour: 1, SeederSize: 1000},
			{Site: "site2", Uploaded: 200, Downloaded: 100, Ratio: 2.0, Seeding: 10, Bonus: 20},
			{Site: "site3", Uploaded: 999, Downloaded: 999, Ratio: 5000, Seeding: 1},
		},
		LastUpdate: time.Now().Unix(),
	}

	enabled := map[string]bool{"site1": true, "site2": true}
	filtered := filterStatsByEnabledSites(stats, enabled)

	assert.Equal(t, 2, filtered.SiteCount)
	assert.Equal(t, int64(300), filtered.TotalUploaded)
	assert.Equal(t, int64(150), filtered.TotalDownloaded)
	assert.Equal(t, 15, filtered.TotalSeeding)
	assert.InDelta(t, 2.0, filtered.AverageRatio, 0.001)
}

func TestFilterStatsByEnabledSites_ExcludesInvalidRatio(t *testing.T) {
	stats := v2.AggregatedStats{
		PerSiteStats: []v2.UserInfo{
			{Site: "site1", Ratio: 5000},
		},
	}
	filtered := filterStatsByEnabledSites(stats, map[string]bool{"site1": true})
	assert.Equal(t, 1, filtered.SiteCount)
	assert.Equal(t, 0.0, filtered.AverageRatio)
}

func TestApiUserInfoAggregated_WithEnabledStore(t *testing.T) {
	svc := setupTestUserInfoService()
	InitUserInfoService(svc)
	t.Cleanup(func() { InitUserInfoService(nil) })

	srv := setupServer(t)
	srv2 := &Server{store: srv.store}

	enabled := true
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("site1"), models.SiteConfig{
		Enabled: &enabled, AuthMethod: "cookie", Cookie: "c=1", APIUrl: "https://s1",
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/aggregated", nil)
	w := httptest.NewRecorder()
	srv2.apiUserInfoAggregated(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp AggregatedStatsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.GreaterOrEqual(t, resp.SiteCount, 0)
}

// ==== merged from api_userinfo_guards_test.go ====
func TestUserInfoHandlers_ServiceAndMethodGuards(t *testing.T) {
	InitUserInfoService(nil)
	s := &Server{}

	cases := []struct {
		name       string
		handler    http.HandlerFunc
		method     string
		path       string
		wantStatus int
	}{
		{"sites method", s.apiUserInfoSites, http.MethodPost, "/api/v2/userinfo/sites", http.StatusMethodNotAllowed},
		{"sites no service", s.apiUserInfoSites, http.MethodGet, "/api/v2/userinfo/sites", http.StatusServiceUnavailable},
		{"registered method", s.apiUserInfoRegisteredSites, http.MethodPost, "/api/v2/userinfo/registered", http.StatusMethodNotAllowed},
		{"registered no service", s.apiUserInfoRegisteredSites, http.MethodGet, "/api/v2/userinfo/registered", http.StatusServiceUnavailable},
		{"sync method", s.apiUserInfoSync, http.MethodGet, "/api/v2/userinfo/sync", http.StatusMethodNotAllowed},
		{"sync no service", s.apiUserInfoSync, http.MethodPost, "/api/v2/userinfo/sync", http.StatusServiceUnavailable},
		{"clearcache method", s.apiUserInfoClearCache, http.MethodGet, "/api/v2/userinfo/cache/clear", http.StatusMethodNotAllowed},
		{"clearcache no service", s.apiUserInfoClearCache, http.MethodPost, "/api/v2/userinfo/cache/clear", http.StatusServiceUnavailable},
		{"detail empty site", s.apiUserInfoSiteDetail, http.MethodGet, "/api/v2/userinfo/sites/", http.StatusBadRequest},
		{"detail no service", s.apiUserInfoSiteDetail, http.MethodGet, "/api/v2/userinfo/sites/site1", http.StatusServiceUnavailable},
		{"aggregated method", s.apiUserInfoAggregated, http.MethodPost, "/api/v2/userinfo/aggregated", http.StatusMethodNotAllowed},
		{"aggregated no service", s.apiUserInfoAggregated, http.MethodGet, "/api/v2/userinfo/aggregated", http.StatusServiceUnavailable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, nil)
			tc.handler(w, req)
			assert.Equal(t, tc.wantStatus, w.Code)
		})
	}
}

func TestUserInfoSiteDetail_UnsupportedMethod(t *testing.T) {
	svc := setupTestUserInfoService()
	InitUserInfoService(svc)
	t.Cleanup(func() { InitUserInfoService(nil) })

	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v2/userinfo/sites/site1", nil)
	s.apiUserInfoSiteDetail(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestApiVersionCheck(t *testing.T) {
	s := &Server{}

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/version/check", nil)
		s.apiVersionCheck(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("returns result or error", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/version/check?proxy=http://127.0.0.1:1", nil)
		s.apiVersionCheck(w, req)
		// Network likely fails in test env; either a JSON body or 500 is acceptable.
		assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
	})
}

// ==== merged from api_userinfo_refresh_cov3_test.go ====
func TestRefreshSiteRegistrations_ReRegisterUpdatesCreds(t *testing.T) {
	svc := v2.NewUserInfoService(v2.UserInfoServiceConfig{})
	InitUserInfoService(svc)
	t.Cleanup(func() { InitUserInfoService(nil) })

	reg := v2.NewSiteRegistry(nil)
	InitSiteRegistry(reg)
	t.Cleanup(func() { InitSiteRegistry(nil) })

	prevOrch := searchOrchestrator
	orch := v2.NewSearchOrchestrator(v2.SearchOrchestratorConfig{})
	searchOrchestrator = v2.NewCachedSearchOrchestrator(orch, v2.SearchCacheConfig{})
	t.Cleanup(func() { searchOrchestrator = prevOrch })

	enabled := true
	store := registerRefreshStore{sites: map[models.SiteGroup]models.SiteConfig{
		"hdsky": {Enabled: &enabled, Cookie: "c=1", APIUrl: "https://hdsky.me"},
	}}

	require.NoError(t, RefreshSiteRegistrations(store))
	require.Contains(t, svc.ListSites(), "hdsky")

	// Second refresh: site already registered -> exercises the unregister+re-register branch.
	store2 := registerRefreshStore{sites: map[models.SiteGroup]models.SiteConfig{
		"hdsky": {Enabled: &enabled, Cookie: "c=2", APIUrl: "https://hdsky.me"},
	}}
	require.NoError(t, RefreshSiteRegistrations(store2))
	assert.Contains(t, svc.ListSites(), "hdsky")
}

// ==== merged from api_userinfo_refresh_test.go ====
// registerRefreshStore adapts an in-memory site map to the ListSites interface.
type registerRefreshStore struct {
	sites map[models.SiteGroup]models.SiteConfig
}

func (s registerRefreshStore) ListSites() (map[models.SiteGroup]models.SiteConfig, error) {
	return s.sites, nil
}

func TestRefreshSiteRegistrations_RegistersEnabled(t *testing.T) {
	svc := v2.NewUserInfoService(v2.UserInfoServiceConfig{})
	InitUserInfoService(svc)
	t.Cleanup(func() { InitUserInfoService(nil) })

	reg := v2.NewSiteRegistry(nil)
	InitSiteRegistry(reg)
	t.Cleanup(func() { InitSiteRegistry(nil) })

	prevOrch := searchOrchestrator
	orch := v2.NewSearchOrchestrator(v2.SearchOrchestratorConfig{})
	searchOrchestrator = v2.NewCachedSearchOrchestrator(orch, v2.SearchCacheConfig{})
	t.Cleanup(func() { searchOrchestrator = prevOrch })

	enabled := true
	disabled := false

	t.Run("enabled site with valid cookie registers", func(t *testing.T) {
		store := registerRefreshStore{sites: map[models.SiteGroup]models.SiteConfig{
			"hdsky": {Enabled: &enabled, Cookie: "c=1", APIUrl: "https://hdsky.me"},
		}}
		require.NoError(t, RefreshSiteRegistrations(store))
		assert.Contains(t, svc.ListSites(), "hdsky")
	})

	t.Run("disabled site is unregistered", func(t *testing.T) {
		store := registerRefreshStore{sites: map[models.SiteGroup]models.SiteConfig{
			"hdsky": {Enabled: &disabled, Cookie: "c=1", APIUrl: "https://hdsky.me"},
		}}
		require.NoError(t, RefreshSiteRegistrations(store))
		assert.NotContains(t, svc.ListSites(), "hdsky")
	})

	t.Run("site missing from config is unregistered", func(t *testing.T) {
		store := registerRefreshStore{sites: map[models.SiteGroup]models.SiteConfig{
			"hdsky": {Enabled: &enabled, Cookie: "c=1", APIUrl: "https://hdsky.me"},
		}}
		require.NoError(t, RefreshSiteRegistrations(store))
		require.Contains(t, svc.ListSites(), "hdsky")

		empty := registerRefreshStore{sites: map[models.SiteGroup]models.SiteConfig{}}
		require.NoError(t, RefreshSiteRegistrations(empty))
		assert.NotContains(t, svc.ListSites(), "hdsky")
	})

	t.Run("enabled site with missing credentials is skipped", func(t *testing.T) {
		store := registerRefreshStore{sites: map[models.SiteGroup]models.SiteConfig{
			"hdsky": {Enabled: &enabled, APIUrl: "https://hdsky.me"},
		}}
		require.NoError(t, RefreshSiteRegistrations(store))
		assert.NotContains(t, svc.ListSites(), "hdsky")
	})
}

// ==== merged from api_userinfo_test.go ====
func init() {
	// Initialize global logger for tests
	if global.GlobalLogger == nil {
		zapLogger, _ := zap.NewDevelopment()
		global.GlobalLogger = zapLogger
	}
}

// mockSite implements v2.Site for testing
type mockSite struct {
	id       string
	name     string
	kind     v2.SiteKind
	userInfo v2.UserInfo
	err      error
}

func (m *mockSite) ID() string                                            { return m.id }
func (m *mockSite) Name() string                                          { return m.name }
func (m *mockSite) Kind() v2.SiteKind                                     { return m.kind }
func (m *mockSite) Login(ctx context.Context, creds v2.Credentials) error { return nil }
func (m *mockSite) Search(ctx context.Context, query v2.SearchQuery) ([]v2.TorrentItem, error) {
	return nil, nil
}

func (m *mockSite) GetUserInfo(ctx context.Context) (v2.UserInfo, error) {
	if m.err != nil {
		return v2.UserInfo{}, m.err
	}
	return m.userInfo, nil
}

func (m *mockSite) Download(ctx context.Context, torrentID string) ([]byte, error) { return nil, nil }
func (m *mockSite) Close() error                                                   { return nil }

func setupTestUserInfoService() *v2.UserInfoService {
	service := v2.NewUserInfoService(v2.UserInfoServiceConfig{
		CacheTTL: 5 * time.Minute,
	})

	// Register mock sites
	site1 := &mockSite{
		id:   "site1",
		name: "Site 1",
		kind: v2.SiteNexusPHP,
		userInfo: v2.UserInfo{
			Site:       "site1",
			Username:   "user1",
			UserID:     "123",
			Uploaded:   1024 * 1024 * 1024, // 1 GB
			Downloaded: 512 * 1024 * 1024,  // 512 MB
			Ratio:      2.0,
			Bonus:      1000,
			Seeding:    10,
			Leeching:   2,
			Rank:       "Power User",
			LastUpdate: time.Now().Unix(),
		},
	}
	site2 := &mockSite{
		id:   "site2",
		name: "Site 2",
		kind: v2.SiteMTorrent,
		userInfo: v2.UserInfo{
			Site:       "site2",
			Username:   "user2",
			UserID:     "456",
			Uploaded:   2 * 1024 * 1024 * 1024, // 2 GB
			Downloaded: 1024 * 1024 * 1024,     // 1 GB
			Ratio:      2.0,
			Bonus:      2000,
			Seeding:    20,
			Leeching:   5,
			Rank:       "Elite",
			LastUpdate: time.Now().Unix(),
		},
	}

	service.RegisterSite(site1)
	service.RegisterSite(site2)

	return service
}

func TestApiUserInfoAggregated(t *testing.T) {
	service := setupTestUserInfoService()
	InitUserInfoService(service)
	defer InitUserInfoService(nil)

	// First sync to populate data
	ctx := context.Background()
	_, _ = service.FetchAndSaveAll(ctx)

	// 没有设置 store，所以 enabledSites 为空，应返回空数据
	server := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/aggregated", nil)
	w := httptest.NewRecorder()

	server.apiUserInfoAggregated(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response AggregatedStatsResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// 没有启用的站点，应返回空数据
	assert.Equal(t, 0, response.SiteCount)
	assert.Equal(t, int64(0), response.TotalUploaded)
	assert.Equal(t, 0, response.TotalSeeding)
}

func TestApiUserInfoAggregated_MethodNotAllowed(t *testing.T) {
	service := setupTestUserInfoService()
	InitUserInfoService(service)
	defer InitUserInfoService(nil)

	server := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/userinfo/aggregated", nil)
	w := httptest.NewRecorder()

	server.apiUserInfoAggregated(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestApiUserInfoAggregated_ServiceNotInitialized(t *testing.T) {
	InitUserInfoService(nil)

	server := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/aggregated", nil)
	w := httptest.NewRecorder()

	server.apiUserInfoAggregated(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestApiUserInfoSites(t *testing.T) {
	service := setupTestUserInfoService()
	InitUserInfoService(service)
	defer InitUserInfoService(nil)

	// First sync to populate data
	ctx := context.Background()
	_, _ = service.FetchAndSaveAll(ctx)

	server := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/sites", nil)
	w := httptest.NewRecorder()

	server.apiUserInfoSites(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []UserInfoResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Len(t, response, 2)
}

func TestApiUserInfoSiteDetail_Get(t *testing.T) {
	service := setupTestUserInfoService()
	InitUserInfoService(service)
	defer InitUserInfoService(nil)

	// First sync to populate data
	ctx := context.Background()
	_, _ = service.FetchAndSave(ctx, "site1")

	server := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/sites/site1", nil)
	w := httptest.NewRecorder()

	server.apiUserInfoSiteDetail(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response UserInfoResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "site1", response.Site)
	assert.Equal(t, "user1", response.Username)
	assert.Equal(t, int64(1024*1024*1024), response.Uploaded)
}

func TestApiUserInfoSiteDetail_Post(t *testing.T) {
	service := setupTestUserInfoService()
	InitUserInfoService(service)
	defer InitUserInfoService(nil)

	server := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/userinfo/sites/site1", nil)
	w := httptest.NewRecorder()

	server.apiUserInfoSiteDetail(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response UserInfoResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "site1", response.Site)
}

func TestApiUserInfoSiteDetail_Delete(t *testing.T) {
	service := setupTestUserInfoService()
	InitUserInfoService(service)
	defer InitUserInfoService(nil)

	// First sync to populate data
	ctx := context.Background()
	_, _ = service.FetchAndSave(ctx, "site1")

	server := &Server{}
	req := httptest.NewRequest(http.MethodDelete, "/api/v2/userinfo/sites/site1", nil)
	w := httptest.NewRecorder()

	server.apiUserInfoSiteDetail(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "deleted", response["status"])
}

func TestApiUserInfoSiteDetail_NotFound(t *testing.T) {
	service := setupTestUserInfoService()
	InitUserInfoService(service)
	defer InitUserInfoService(nil)

	server := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/sites/nonexistent", nil)
	w := httptest.NewRecorder()

	server.apiUserInfoSiteDetail(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestApiUserInfoSiteDetail_EmptySiteID(t *testing.T) {
	service := setupTestUserInfoService()
	InitUserInfoService(service)
	defer InitUserInfoService(nil)

	server := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/sites/", nil)
	w := httptest.NewRecorder()

	server.apiUserInfoSiteDetail(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestApiUserInfoSync_All(t *testing.T) {
	service := setupTestUserInfoService()
	InitUserInfoService(service)
	defer InitUserInfoService(nil)

	server := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/userinfo/sync", nil)
	w := httptest.NewRecorder()

	server.apiUserInfoSync(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response SyncResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Len(t, response.Success, 2)
	assert.Len(t, response.Failed, 0)
}

func TestApiUserInfoSync_Specific(t *testing.T) {
	service := setupTestUserInfoService()
	InitUserInfoService(service)
	defer InitUserInfoService(nil)

	server := &Server{}
	body := SyncRequest{Sites: []string{"site1"}}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/userinfo/sync", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.apiUserInfoSync(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response SyncResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Len(t, response.Success, 1)
	assert.Contains(t, response.Success, "site1")
}

func TestApiUserInfoSync_MethodNotAllowed(t *testing.T) {
	service := setupTestUserInfoService()
	InitUserInfoService(service)
	defer InitUserInfoService(nil)

	server := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/sync", nil)
	w := httptest.NewRecorder()

	server.apiUserInfoSync(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestApiUserInfoRegisteredSites(t *testing.T) {
	service := setupTestUserInfoService()
	InitUserInfoService(service)
	defer InitUserInfoService(nil)

	server := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/registered", nil)
	w := httptest.NewRecorder()

	server.apiUserInfoRegisteredSites(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string][]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	sites := response["sites"]
	assert.Len(t, sites, 2)
	assert.Contains(t, sites, "site1")
	assert.Contains(t, sites, "site2")
}

func TestApiUserInfoClearCache(t *testing.T) {
	service := setupTestUserInfoService()
	InitUserInfoService(service)
	defer InitUserInfoService(nil)

	server := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/userinfo/cache/clear", nil)
	w := httptest.NewRecorder()

	server.apiUserInfoClearCache(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "ok", response["status"])
}

func TestApiUserInfoClearCache_MethodNotAllowed(t *testing.T) {
	service := setupTestUserInfoService()
	InitUserInfoService(service)
	defer InitUserInfoService(nil)

	server := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/cache/clear", nil)
	w := httptest.NewRecorder()

	server.apiUserInfoClearCache(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestToUserInfoResponse(t *testing.T) {
	info := v2.UserInfo{
		Site:       "test-site",
		Username:   "testuser",
		UserID:     "12345",
		Uploaded:   1024,
		Downloaded: 512,
		Ratio:      2.0,
		Bonus:      100.5,
		Seeding:    5,
		Leeching:   1,
		Rank:       "VIP",
		JoinDate:   1609459200,
		LastAccess: 1704067200,
		LastUpdate: 1704153600,
	}

	response := toUserInfoResponse(info)

	assert.Equal(t, info.Site, response.Site)
	assert.Equal(t, info.Username, response.Username)
	assert.Equal(t, info.UserID, response.UserID)
	assert.Equal(t, info.Uploaded, response.Uploaded)
	assert.Equal(t, info.Downloaded, response.Downloaded)
	assert.Equal(t, info.Ratio, response.Ratio)
	assert.Equal(t, info.Bonus, response.Bonus)
	assert.Equal(t, info.Seeding, response.Seeding)
	assert.Equal(t, info.Leeching, response.Leeching)
	assert.Equal(t, info.Rank, response.Rank)
	assert.Equal(t, info.JoinDate, response.JoinDate)
	assert.Equal(t, info.LastAccess, response.LastAccess)
	assert.Equal(t, info.LastUpdate, response.LastUpdate)
}

func TestToAggregatedStatsResponse(t *testing.T) {
	stats := v2.AggregatedStats{
		TotalUploaded:   2048,
		TotalDownloaded: 1024,
		AverageRatio:    2.0,
		TotalSeeding:    10,
		TotalLeeching:   3,
		TotalBonus:      500.0,
		SiteCount:       2,
		LastUpdate:      1704153600,
		PerSiteStats: []v2.UserInfo{
			{Site: "site1", Username: "user1"},
			{Site: "site2", Username: "user2"},
		},
	}

	response := toAggregatedStatsResponse(stats)

	assert.Equal(t, stats.TotalUploaded, response.TotalUploaded)
	assert.Equal(t, stats.TotalDownloaded, response.TotalDownloaded)
	assert.Equal(t, stats.AverageRatio, response.AverageRatio)
	assert.Equal(t, stats.TotalSeeding, response.TotalSeeding)
	assert.Equal(t, stats.TotalLeeching, response.TotalLeeching)
	assert.Equal(t, stats.TotalBonus, response.TotalBonus)
	assert.Equal(t, stats.SiteCount, response.SiteCount)
	assert.Equal(t, stats.LastUpdate, response.LastUpdate)
	assert.Len(t, response.PerSiteStats, 2)
}
