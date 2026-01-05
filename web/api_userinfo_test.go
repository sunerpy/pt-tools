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
