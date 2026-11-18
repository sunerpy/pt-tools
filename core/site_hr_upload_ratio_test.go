package core

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// #542：站点设置「H&R 上传量达标」能存能读；部分更新、整站保存（网页编辑、站点列表上的开关）、浏览器扩展与 CookieCloud
// 同步凭据都不会把它清成 0；读站点配置的几处都带上它。
func TestSiteHRUploadRatio_SavedAndKept(t *testing.T) {
	f := newRSSKeepFixture(t)
	sg := models.SiteGroup("springsunday")
	ratio := 0.9
	require.NoError(t, f.store.UpdateSiteOptions(sg, SiteOptions{HRUploadRatio: &ratio}))
	check := func(t *testing.T, how string) {
		t.Helper()
		var row models.SiteSetting
		require.NoError(t, f.db.DB.Where("name = ?", string(sg)).First(&row).Error)
		assert.InDelta(t, 0.9, row.HRUploadRatio, 1e-9, how)
	}
	check(t, "部分更新")

	sc, err := f.store.GetSiteConf(sg)
	require.NoError(t, err)
	assert.InDelta(t, 0.9, sc.HRUploadRatio, 1e-9, "单个站点的配置带上它")
	require.NoError(t, f.store.UpsertSiteWithRSS(sg, sc))
	check(t, "单个站点的配置存回去（网页编辑、扩展同步凭据）")

	sites, err := f.store.ListSites()
	require.NoError(t, err)
	listed := sites[sg]
	assert.InDelta(t, 0.9, listed.HRUploadRatio, 1e-9, "站点列表带上它")
	disabled := false
	listed.Enabled = &disabled
	require.NoError(t, f.store.UpsertSiteWithRSS(sg, listed))
	check(t, "站点列表上停用站点")

	require.NoError(t, f.store.SetSiteEnabled(sg, true))
	check(t, "启用开关")
	failed, err := f.store.SetSiteCookies(context.Background(), map[models.SiteGroup]string{sg: "c=3"})
	require.NoError(t, err)
	require.Empty(t, failed)
	check(t, "CookieCloud 同步 Cookie")
	cookie := "c=4"
	require.NoError(t, f.store.SaveSiteCredentials(sg, &cookie, nil, nil))
	check(t, "只存凭据")

	cfg, err := f.store.Load()
	require.NoError(t, err)
	assert.InDelta(t, 0.9, cfg.Sites[sg].HRUploadRatio, 1e-9, "加载的配置带上它")

	zero := 0.0
	require.NoError(t, f.store.UpdateSiteOptions(sg, SiteOptions{HRUploadRatio: &zero}), "0 是不用")
	var row models.SiteSetting
	require.NoError(t, f.db.DB.Where("name = ?", string(sg)).First(&row).Error)
	assert.Zero(t, row.HRUploadRatio)
}

// 「H&R 上传量达标」只能是 0（不用）或 0.1 到 10 倍；越界的部分更新和整站保存都不收，库里的值不变。
func TestSiteHRUploadRatio_RejectsOutOfRange(t *testing.T) {
	f := newRSSKeepFixture(t)
	sg := models.SiteGroup("springsunday")
	ratio := 1.5
	require.NoError(t, f.store.UpdateSiteOptions(sg, SiteOptions{HRUploadRatio: &ratio}))
	for _, v := range []float64{-1, 0.05, 10.5, 11, math.NaN(), math.Inf(1)} {
		bad := v
		require.Error(t, f.store.UpdateSiteOptions(sg, SiteOptions{HRUploadRatio: &bad}), "部分更新：%v", v)
		sc, err := f.store.GetSiteConf(sg)
		require.NoError(t, err)
		sc.HRUploadRatio = v
		require.Error(t, f.store.UpsertSiteWithRSS(sg, sc), "整站保存：%v", v)
	}
	var row models.SiteSetting
	require.NoError(t, f.db.DB.Where("name = ?", string(sg)).First(&row).Error)
	assert.InDelta(t, 1.5, row.HRUploadRatio, 1e-9)
	for _, v := range []float64{0.1, 10} {
		ok := v
		require.NoError(t, f.store.UpdateSiteOptions(sg, SiteOptions{HRUploadRatio: &ok}), "边界值 %v", v)
	}
}
