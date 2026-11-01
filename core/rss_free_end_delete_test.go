package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// 免费结束时删除还是暂停每条 RSS 自己选（free_end_delete）：只在开着免费结束处理（pause_on_free_end）时记成删除，
// 三处保存（改一条、新增一条、整站保存）都按这个规范化。
func TestRSSFreeEndDelete_SaveNormalizes(t *testing.T) {
	f := newRSSKeepFixture(t)
	sg := models.SiteGroup("springsunday")
	a := models.RSSConfig{ID: f.a, Name: "A", URL: "https://springsunday.net/rss/a", Tag: "movie", IntervalMinutes: 10}

	a.PauseOnFreeEnd, a.FreeEndDelete = true, true
	got, err := f.store.UpdateRSSInSite(string(sg), a)
	require.NoError(t, err)
	assert.True(t, got.FreeEndDelete)
	assert.True(t, rssByName(t, f.db, "springsunday")["A"].FreeEndDelete, "改一条：开着免费结束处理时记成删除")

	a.PauseOnFreeEnd = false
	got, err = f.store.UpdateRSSInSite(string(sg), a)
	require.NoError(t, err)
	assert.False(t, got.FreeEndDelete)
	assert.False(t, rssByName(t, f.db, "springsunday")["A"].FreeEndDelete, "改一条：关了免费结束处理就不删")

	_, err = f.store.AppendRSSToSite(string(sg), models.RSSConfig{
		Name: "B", URL: "https://springsunday.net/rss/b", IntervalMinutes: 10, PauseOnFreeEnd: false, FreeEndDelete: true,
	})
	require.NoError(t, err)
	_, err = f.store.AppendRSSToSite(string(sg), models.RSSConfig{
		Name: "D", URL: "https://springsunday.net/rss/d", IntervalMinutes: 10, PauseOnFreeEnd: true, FreeEndDelete: true,
	})
	require.NoError(t, err)
	rows := rssByName(t, f.db, "springsunday")
	assert.False(t, rows["B"].FreeEndDelete, "新增：没开免费结束处理时不记删除")
	assert.True(t, rows["D"].FreeEndDelete, "新增：开着时记成删除")

	sc, err := f.store.GetSiteConf(sg)
	require.NoError(t, err)
	for i := range sc.RSS {
		switch sc.RSS[i].Name {
		case "A":
			sc.RSS[i].PauseOnFreeEnd, sc.RSS[i].FreeEndDelete = true, true
		case "D":
			sc.RSS[i].PauseOnFreeEnd, sc.RSS[i].FreeEndDelete = false, true
		}
	}
	sc.RSS = append(sc.RSS, models.RSSConfig{
		Name: "E", URL: "https://springsunday.net/rss/e", IntervalMinutes: 10, PauseOnFreeEnd: true, FreeEndDelete: true,
	})
	require.NoError(t, f.store.UpsertSiteWithRSS(sg, sc))
	rows = rssByName(t, f.db, "springsunday")
	assert.True(t, rows["A"].FreeEndDelete, "整站保存：原地更新带上删除")
	assert.False(t, rows["D"].FreeEndDelete, "整站保存：关了免费结束处理就不删")
	assert.True(t, rows["E"].FreeEndDelete, "整站保存：新建的也带上")
	assert.Equal(t, f.a, rows["A"].ID, "RSS 编号不变")
}

// 读站点配置的几处（站点列表、单个站点、RSS 任务的加载）都带上免费结束时删除的选择：整站保存（网页编辑、站点列表上的启用开关、
// 浏览器扩展同步 Cookie）把读出来的配置原样存回去，选择不丢、RSS 编号不变。
func TestRSSFreeEndDelete_SurvivesWholeSiteSave(t *testing.T) {
	f := newRSSKeepFixture(t)
	sg := models.SiteGroup("springsunday")
	_, err := f.store.UpdateRSSInSite(string(sg), models.RSSConfig{
		ID: f.a, Name: "A", URL: "https://springsunday.net/rss/a", Tag: "movie", IntervalMinutes: 10,
		FilterRuleIDs: []uint{f.ruleA}, PauseOnFreeEnd: true, FreeEndDelete: true,
	})
	require.NoError(t, err)
	check := func(t *testing.T, how string) {
		t.Helper()
		row := rssByName(t, f.db, "springsunday")["A"]
		assert.Equal(t, f.a, row.ID, how+"：RSS 编号不变")
		assert.True(t, row.PauseOnFreeEnd, how)
		assert.True(t, row.FreeEndDelete, how+"：免费结束时删除的选择不丢")
	}

	sc, err := f.store.GetSiteConf(sg)
	require.NoError(t, err)
	require.NoError(t, f.store.UpsertSiteWithRSS(sg, sc))
	check(t, "单个站点的配置存回去（网页编辑、扩展同步 Cookie）")

	sites, err := f.store.ListSites()
	require.NoError(t, err)
	listed := sites[sg]
	disabled := false
	listed.Enabled = &disabled
	require.NoError(t, f.store.UpsertSiteWithRSS(sg, listed))
	check(t, "站点列表上停用站点")

	cfg, err := f.store.Load()
	require.NoError(t, err)
	var loaded *models.RSSConfig
	for i, r := range cfg.Sites[sg].RSS {
		if r.ID == f.a {
			loaded = &cfg.Sites[sg].RSS[i]
		}
	}
	require.NotNil(t, loaded)
	assert.True(t, loaded.FreeEndDelete, "RSS 任务加载的配置带上删除的选择，推送时照它写进种子记录")
}
