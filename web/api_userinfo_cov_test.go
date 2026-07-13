package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

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
