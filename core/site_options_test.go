package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
)

// 原地改一条 RSS：编号不变，字段与过滤规则关联换成新的；地址可以和自己原来的一样，不能和同站点的别的订阅重复；
// 下载器、过滤规则要存在；别的站点的订阅改不了。
func TestUpdateRSSInSite(t *testing.T) {
	db, err := OpenTestDB(t)
	require.NoError(t, err)
	s := NewConfigStore(db)
	site := models.SiteSetting{Name: "mteam", Enabled: true, AuthMethod: "api_key", APIUrl: "https://api.m-team.cc", APIKey: "k"}
	other := models.SiteSetting{Name: "hdsky", Enabled: true, AuthMethod: "cookie", CookieEncrypted: "x"}
	require.NoError(t, db.DB.Create(&site).Error)
	require.NoError(t, db.DB.Create(&other).Error)
	dl := models.DownloaderSetting{Name: "qbit", Type: "qbittorrent", URL: "http://127.0.0.1:8080", IsDefault: true, Enabled: true}
	require.NoError(t, db.DB.Create(&dl).Error)
	r1, r2 := models.FilterRule{Name: "a", Pattern: "*", Enabled: true}, models.FilterRule{Name: "b", Pattern: "x", Enabled: true}
	require.NoError(t, db.DB.Create(&r1).Error)
	require.NoError(t, db.DB.Create(&r2).Error)
	rows := []models.RSSSubscription{
		{SiteID: site.ID, Name: "a", URL: "https://example.com/a", IntervalMinutes: 10, IsExample: true},
		{SiteID: site.ID, Name: "b", URL: "https://example.com/b", IntervalMinutes: 10},
		{SiteID: other.ID, Name: "c", URL: "https://example.com/c", IntervalMinutes: 10},
	}
	require.NoError(t, db.DB.Create(&rows).Error)
	require.NoError(t, models.NewRSSFilterAssociationDB(db.DB).SetFilterRulesForRSS(rows[0].ID, []uint{r1.ID}))

	_, ch, cancel := events.Subscribe(4)
	defer cancel()
	got, err := s.UpdateRSSInSite("mteam", models.RSSConfig{
		ID: rows[0].ID, Name: "改名", URL: "https://example.com/a", Category: "movie", Tag: "t", IntervalMinutes: 1,
		DownloaderID: &dl.ID, DownloadPath: "/dl", FilterRuleIDs: []uint{r2.ID, r2.ID}, NotifyMode: "all", NotifyConfIDs: "[3]",
	})
	require.NoError(t, err)
	assert.Equal(t, rows[0].ID, got.ID)
	assert.True(t, got.IsExample, "示例标记不变")
	assert.Equal(t, models.MinIntervalMinutes, got.IntervalMinutes)
	select {
	case ev := <-ch:
		assert.Equal(t, events.ConfigChanged, ev.Type)
	case <-time.After(time.Second):
		t.Fatal("没有发 ConfigChanged")
	}
	var row models.RSSSubscription
	require.NoError(t, db.DB.First(&row, rows[0].ID).Error)
	assert.Equal(t, "改名", row.Name)
	assert.Equal(t, "movie", row.Category)
	assert.Equal(t, "/dl", row.DownloadPath)
	assert.Equal(t, "[3]", row.NotifyConfIDs)
	require.NotNil(t, row.DownloaderID)
	ids, err := models.NewRSSFilterAssociationDB(db.DB).GetFilterRuleIDsForRSS(row.ID)
	require.NoError(t, err)
	assert.Equal(t, []uint{r2.ID}, ids, "关联换成新的，重复的去掉")
	var count int64
	require.NoError(t, db.DB.Model(&models.RSSSubscription{}).Count(&count).Error)
	assert.Equal(t, int64(3), count, "不新增、不重建")

	_, err = s.UpdateRSSInSite("mteam", models.RSSConfig{ID: rows[0].ID, Name: "a", URL: " HTTPS://EXAMPLE.COM/B "})
	require.ErrorContains(t, err, "重复")
	missing := uint(999)
	_, err = s.UpdateRSSInSite("mteam", models.RSSConfig{ID: rows[0].ID, Name: "a", URL: "https://example.com/a", DownloaderID: &missing})
	require.ErrorContains(t, err, "下载器不存在")
	_, err = s.UpdateRSSInSite("mteam", models.RSSConfig{ID: rows[0].ID, Name: "a", URL: "https://example.com/a", FilterRuleIDs: []uint{999}})
	require.ErrorContains(t, err, "过滤规则不存在")
	_, err = s.UpdateRSSInSite("mteam", models.RSSConfig{ID: rows[2].ID, Name: "c", URL: "https://example.com/c"})
	require.ErrorContains(t, err, "RSS 订阅不存在", "别的站点的订阅改不了")
	_, err = s.UpdateRSSInSite("nope", models.RSSConfig{ID: rows[0].ID, Name: "a", URL: "https://example.com/a"})
	require.ErrorContains(t, err, "站点不存在")
	_, err = s.UpdateRSSInSite("mteam", models.RSSConfig{ID: rows[0].ID, Name: " ", URL: "https://example.com/a"})
	require.Error(t, err)
}

// 只改给了的几项，RSS 不重建（编号不变）；Cookie 加密存；启用时要凭据齐全；负数不收。
func TestUpdateSiteOptions(t *testing.T) {
	db, err := OpenTestDB(t)
	require.NoError(t, err)
	s := NewConfigStore(db)
	site := models.SiteSetting{Name: "hdsky", Enabled: false, AuthMethod: "cookie", UploadLimitKBs: 5}
	require.NoError(t, db.DB.Create(&site).Error)
	rss := models.RSSSubscription{SiteID: site.ID, Name: "a", URL: "https://example.com/a", IntervalMinutes: 10}
	require.NoError(t, db.DB.Create(&rss).Error)

	on := true
	require.ErrorContains(t, s.UpdateSiteOptions("hdsky", SiteOptions{Enabled: &on}), "Cookie", "没有 Cookie 不能启用")

	cookie, up, hr := "uid=1; pass=2", 100, 72
	require.NoError(t, s.UpdateSiteOptions("hdsky", SiteOptions{Enabled: &on, Cookie: &cookie, UploadLimitKBs: &up, HRSeedTimeH: &hr}))
	var row models.SiteSetting
	require.NoError(t, db.DB.First(&row, site.ID).Error)
	assert.True(t, row.Enabled)
	assert.Equal(t, 100, row.UploadLimitKBs)
	assert.Equal(t, 72, row.HRSeedTimeH)
	assert.Empty(t, row.Cookie, "不存明文")
	assert.NotEmpty(t, row.CookieEncrypted)
	assert.NotContains(t, row.CookieEncrypted, "pass=2")
	plain, err := s.DecryptCookie(row.CookieEncrypted)
	require.NoError(t, err)
	assert.Equal(t, cookie, plain)

	var after models.RSSSubscription
	require.NoError(t, db.DB.First(&after, rss.ID).Error, "RSS 编号不变")

	empty := ""
	require.ErrorContains(t, s.UpdateSiteOptions("hdsky", SiteOptions{Cookie: &empty}), "Cookie", "启用着的站点不能清掉 Cookie")
	neg := -1
	require.ErrorContains(t, s.UpdateSiteOptions("hdsky", SiteOptions{DownloadLimitKBs: &neg}), "负数")
	off := false
	require.NoError(t, s.UpdateSiteOptions("hdsky", SiteOptions{Enabled: &off, Cookie: &empty}))
	require.NoError(t, db.DB.First(&row, site.ID).Error)
	assert.False(t, row.Enabled)
	assert.Empty(t, row.CookieEncrypted)
	require.ErrorContains(t, s.UpdateSiteOptions("nope", SiteOptions{Enabled: &off}), "站点不存在")
	require.NoError(t, s.UpdateSiteOptions("hdsky", SiteOptions{}), "什么都没给：不改")
}
