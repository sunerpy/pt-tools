package internal

import (
	"context"
	"fmt"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// SiteGroupToID 将 models.SiteGroup 映射到 site/v2 的 site ID
var SiteGroupToID = map[models.SiteGroup]string{
	models.MTEAM:        "mteam",
	models.HDSKY:        "hdsky",
	models.SpringSunday: "springsunday",
	models.HDDOLBY:      "hddolby",
	models.OURBITS:      "ourbits",
}

// IDToSiteGroup 将 site/v2 的 site ID 映射到 models.SiteGroup
var IDToSiteGroup = map[string]models.SiteGroup{
	"mteam":        models.MTEAM,
	"hdsky":        models.HDSKY,
	"springsunday": models.SpringSunday,
	"hddolby":      models.HDDOLBY,
	"ourbits":      models.OURBITS,
}

// SiteGroupToKind 将 models.SiteGroup 映射到 v2.SiteKind
var SiteGroupToKind = map[models.SiteGroup]v2.SiteKind{
	models.MTEAM:        v2.SiteMTorrent,
	models.HDSKY:        v2.SiteNexusPHP,
	models.SpringSunday: v2.SiteNexusPHP,
	models.HDDOLBY:      v2.SiteNexusPHP,
	models.OURBITS:      v2.SiteNexusPHP,
}

// NewUnifiedSiteImpl 创建统一站点实现
// 根据 SiteGroup 自动选择正确的 Driver 和配置
func NewUnifiedSiteImpl(ctx context.Context, siteGroup models.SiteGroup) (*UnifiedSiteImpl, error) {
	siteID, ok := SiteGroupToID[siteGroup]
	if !ok {
		return nil, fmt.Errorf("unsupported site group: %s", siteGroup)
	}

	return newUnifiedSiteImplWithID(ctx, siteGroup, siteID)
}

// GetAllSupportedSiteGroups 返回所有支持的站点分组
func GetAllSupportedSiteGroups() []models.SiteGroup {
	groups := make([]models.SiteGroup, 0, len(SiteGroupToID))
	for group := range SiteGroupToID {
		groups = append(groups, group)
	}
	return groups
}

// IsSiteGroupSupported 检查站点分组是否支持
func IsSiteGroupSupported(siteGroup models.SiteGroup) bool {
	_, ok := SiteGroupToID[siteGroup]
	return ok
}
