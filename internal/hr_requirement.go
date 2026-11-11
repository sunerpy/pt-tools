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

// SiteHRUploadRatio 读站点设置里用户填的「H&R 上传量达标」（#542）：种子下完以后上传量达到体积的这么多倍也算 H&R 达标。
// 没填、查不到、库不可用、值不在 0.1 到 10 倍之间时为 0（不按上传量判断）。
func SiteHRUploadRatio(siteName string) float64 {
	if global.GlobalDB == nil || siteName == "" {
		return 0
	}
	var site models.SiteSetting
	if err := global.GlobalDB.DB.Select("hr_upload_ratio").Where("name = ?", siteName).Take(&site).Error; err != nil {
		return 0
	}
	return UsableHRUploadRatio(site.HRUploadRatio)
}

// UsableHRUploadRatio 是能用的「H&R 上传量达标」：在 0.1 到 10 倍之间时原样返回，别的值（0、越界、NaN）都是 0，不按上传量判断。
func UsableHRUploadRatio(v float64) float64 {
	if v >= 0.1 && v <= 10 {
		return v
	}
	return 0
}
