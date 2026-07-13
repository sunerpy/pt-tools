package web

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

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
