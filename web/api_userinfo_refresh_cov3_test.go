package web

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

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
