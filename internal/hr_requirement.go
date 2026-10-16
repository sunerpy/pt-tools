package internal

import (
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// HRSeedTimeH 是一个种子的 H&R 做种要求（小时）：按站点定义算，定义里没有时用站点设置里填的「H&R 做种要求」，
// 都没有时为 0（要求未知），见 v2.ResolveHRSeedTimeH。def 可以为 nil。
func HRSeedTimeH(siteName string, def *v2.SiteDefinition, sizeBytes int64) int {
	return v2.ResolveHRSeedTimeH(def, sizeBytes, SiteHRSeedTimeH(siteName))
}

// SiteHRSeedTimeH 读站点设置里用户填的 H&R 做种要求（小时）；没填、查不到、库不可用时为 0。
func SiteHRSeedTimeH(siteName string) int {
	if global.GlobalDB == nil || siteName == "" {
		return 0
	}
	var site models.SiteSetting
	if err := global.GlobalDB.DB.Select("hr_seed_time_h").Where("name = ?", siteName).Take(&site).Error; err != nil {
		return 0
	}
	return max(site.HRSeedTimeH, 0)
}
