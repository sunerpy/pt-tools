package reseed

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// 站点开关：从「全部站点」关掉一个时名单换成其余已配置的站点；全部关掉不会被当成「全部」；
// 打开的站点必须已配置；保存设置不带站点名单时不动站点范围。
func TestSiteSelection(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.siteIDs = []string{"hdsky", "ourbits", "mteam"}
	e.enable(nil)

	got, err := e.svc.Settings(ctx)
	require.NoError(t, err)
	assert.Equal(t, models.ReseedSitesAll, got.SiteMode, "默认是全部站点")

	sel, err := e.svc.SetSiteSelected(ctx, "ourbits", false)
	require.NoError(t, err)
	assert.Equal(t, SiteSelection{Mode: models.ReseedSitesSelected, SiteNames: []string{"hdsky", "mteam"}}, sel)

	sel, err = e.svc.SetSiteSelected(ctx, "ourbits", true)
	require.NoError(t, err)
	assert.Equal(t, []string{"hdsky", "mteam", "ourbits"}, sel.SiteNames)
	assert.Equal(t, models.ReseedSitesSelected, sel.Mode, "开回来也还是名单：以后新配置的站点默认不参与")

	_, err = e.svc.SetSiteSelected(ctx, "nowhere", true)
	require.ErrorIs(t, err, ErrInvalid, "没配置的站点不能打开")
	_, err = e.svc.SetSiteSelected(ctx, " ", false)
	require.ErrorIs(t, err, ErrInvalid)

	// 保存设置不带站点名单：站点范围不变
	_, err = e.svc.SaveSettings(ctx, SettingsUpdate{Enabled: true, IntervalHours: 6})
	require.NoError(t, err)
	got, err = e.svc.Settings(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"hdsky", "mteam", "ourbits"}, got.SiteNames)
	assert.Equal(t, models.ReseedSitesSelected, got.SiteMode)

	sel, err = e.svc.SetAllSitesSelected(ctx, false)
	require.NoError(t, err)
	assert.Equal(t, SiteSelection{Mode: models.ReseedSitesSelected, SiteNames: []string{}}, sel)
	_, err = e.svc.Run(ctx, RunOptions{})
	require.ErrorIs(t, err, ErrNoSites, "全部关掉以后没有站点可以辅种")
	items, err := e.svc.SiteMap(ctx)
	require.NoError(t, err)
	assert.False(t, items[0].Selected)

	sel, err = e.svc.SetAllSitesSelected(ctx, true)
	require.NoError(t, err)
	assert.Equal(t, SiteSelection{Mode: models.ReseedSitesAll, SiteNames: []string{}}, sel)
	items, err = e.svc.SiteMap(ctx)
	require.NoError(t, err)
	assert.True(t, items[0].Selected, "全部打开以后 hdsky 又在范围里")
}

// 旧数据没有 site_mode：名单不为空时只算名单，为空时是全部站点。
func TestSiteSelectionLegacyRows(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.enable(nil)
	require.NoError(t, e.db.Model(&models.ReseedSetting{}).Where("id = 1").
		Updates(map[string]any{"site_mode": "", "site_names": `["ourbits"]`}).Error)
	got, err := e.svc.Settings(ctx)
	require.NoError(t, err)
	assert.Equal(t, models.ReseedSitesSelected, got.SiteMode)
	_, err = e.svc.Run(ctx, RunOptions{})
	require.ErrorIs(t, err, ErrNoSites, "旧名单里只有没配置的站点")

	require.NoError(t, e.db.Model(&models.ReseedSetting{}).Where("id = 1").Update("site_names", "").Error)
	got, err = e.svc.Settings(ctx)
	require.NoError(t, err)
	assert.Equal(t, models.ReseedSitesAll, got.SiteMode)

	// 旧接口（设置里带名单）照样能用：空名单是全部，有名单是只算名单
	_, err = e.svc.SaveSettings(ctx, SettingsUpdate{Enabled: true, SiteNames: &[]string{"hdsky"}})
	require.NoError(t, err)
	got, err = e.svc.Settings(ctx)
	require.NoError(t, err)
	assert.Equal(t, models.ReseedSitesSelected, got.SiteMode)
	_, err = e.svc.SaveSettings(ctx, SettingsUpdate{Enabled: true, SiteNames: &[]string{}})
	require.NoError(t, err)
	got, err = e.svc.Settings(ctx)
	require.NoError(t, err)
	assert.Equal(t, models.ReseedSitesAll, got.SiteMode)
}
