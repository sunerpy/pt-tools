package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// rssDeleteFixture 建两个站点，各一条 RSS，都关联同一条过滤规则；返回两条 RSS 与规则编号。
func rssDeleteFixture(t *testing.T, srv *Server) (hdsky, ssd models.RSSSubscription, ruleID uint) {
	t.Helper()
	enabled := true
	for _, site := range []string{"hdsky", "springsunday"} {
		require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup(site), models.SiteConfig{
			Enabled: &enabled, AuthMethod: "cookie", Cookie: "c=1",
			RSS: []models.RSSConfig{{Name: site + "-feed", URL: "http://example.invalid/" + site}},
		}))
	}
	db := global.GlobalDB.DB
	rule := models.FilterRule{Name: "收藏", Pattern: "x"}
	require.NoError(t, db.Create(&rule).Error)
	feedOf := func(name string) models.RSSSubscription {
		var site models.SiteSetting
		require.NoError(t, db.Where("name = ?", name).First(&site).Error)
		var feed models.RSSSubscription
		require.NoError(t, db.Where("site_id = ?", site.ID).First(&feed).Error)
		require.NoError(t, models.NewRSSFilterAssociationDB(db).SetFilterRulesForRSS(feed.ID, []uint{rule.ID}))
		return feed
	}
	return feedOf("hdsky"), feedOf("springsunday"), rule.ID
}

func deleteSiteRSS(t *testing.T, srv *Server, site string, id uint) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	srv.apiSiteDetail(w, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/sites/%s?id=%d", site, id), nil))
	return w
}

// 站点详情里删掉一条 RSS 时，它和过滤规则的关联一起删掉；别的 RSS 的关联不动。
func TestApiSiteDetail_DeleteRSSRemovesFilterAssociations(t *testing.T) {
	srv := setupServer(t)
	hdsky, ssd, ruleID := rssDeleteFixture(t, srv)

	w := deleteSiteRSS(t, srv, "hdsky", hdsky.ID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	db := global.GlobalDB.DB
	var left int64
	require.NoError(t, db.Model(&models.RSSSubscription{}).Where("id = ?", hdsky.ID).Count(&left).Error)
	assert.Zero(t, left, "RSS 删掉了")
	assoc := models.NewRSSFilterAssociationDB(db)
	ids, err := assoc.GetFilterRuleIDsForRSS(hdsky.ID)
	require.NoError(t, err)
	assert.Empty(t, ids, "删掉的 RSS 不留关联")
	ids, err = assoc.GetFilterRuleIDsForRSS(ssd.ID)
	require.NoError(t, err)
	assert.Equal(t, []uint{ruleID}, ids, "别的 RSS 的关联不动")
}

// 编号是别的站点的 RSS 时什么都不删，关联也不删（和原来一样回 200）。
func TestApiSiteDetail_DeleteRSSOfOtherSiteKeepsIt(t *testing.T) {
	srv := setupServer(t)
	_, ssd, ruleID := rssDeleteFixture(t, srv)

	w := deleteSiteRSS(t, srv, "hdsky", ssd.ID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	db := global.GlobalDB.DB
	var left int64
	require.NoError(t, db.Model(&models.RSSSubscription{}).Where("id = ?", ssd.ID).Count(&left).Error)
	assert.EqualValues(t, 1, left, "别的站点的 RSS 还在")
	ids, err := models.NewRSSFilterAssociationDB(db).GetFilterRuleIDsForRSS(ssd.ID)
	require.NoError(t, err)
	assert.Equal(t, []uint{ruleID}, ids)
}

// 关联删不掉时整个删除回滚：RSS 还在，回 500。
func TestApiSiteDetail_DeleteRSSRollsBackWhenAssociationsFail(t *testing.T) {
	srv := setupServer(t)
	hdsky, _, _ := rssDeleteFixture(t, srv)
	db := global.GlobalDB.DB
	require.NoError(t, db.Migrator().DropTable(&models.RSSFilterAssociation{}))

	w := deleteSiteRSS(t, srv, "hdsky", hdsky.ID)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	var left int64
	require.NoError(t, db.Model(&models.RSSSubscription{}).Where("id = ?", hdsky.ID).Count(&left).Error)
	assert.EqualValues(t, 1, left, "关联没删掉时 RSS 也不删")
}
