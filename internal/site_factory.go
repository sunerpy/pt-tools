package internal

import (
	"context"
	"fmt"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func NewUnifiedSiteImpl(ctx context.Context, siteGroup models.SiteGroup) (*UnifiedSiteImpl, error) {
	siteID := string(siteGroup)

	registry := v2.GetGlobalSiteRegistry()
	meta, ok := registry.Get(siteID)
	if !ok {
		return nil, fmt.Errorf("unsupported site: %s", siteID)
	}

	return newUnifiedSiteImplWithID(ctx, siteGroup, siteID, meta.Kind)
}

func GetAllSupportedSiteGroups() []models.SiteGroup {
	registry := v2.GetGlobalSiteRegistry()
	ids := registry.List()

	groups := make([]models.SiteGroup, 0, len(ids))
	for _, id := range ids {
		if _, ok := models.AllowedSiteGroups[models.SiteGroup(id)]; ok {
			groups = append(groups, models.SiteGroup(id))
		}
	}
	return groups
}

func IsSiteGroupSupported(siteGroup models.SiteGroup) bool {
	if _, ok := models.AllowedSiteGroups[siteGroup]; !ok {
		return false
	}

	registry := v2.GetGlobalSiteRegistry()
	_, ok := registry.Get(string(siteGroup))
	return ok
}
