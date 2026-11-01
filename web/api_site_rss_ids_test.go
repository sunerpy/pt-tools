package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// hdskyRSSIDs 是 seedHDSkyRSS 建的两条 RSS 与过滤规则的编号。
type hdskyRSSIDs struct {
	movie, example, rule uint
}

// seedHDSkyRSS 建 HDSky 和两条 RSS：「电影」关联一条过滤规则，「剧集」是示例配置（只由迁移标记，保存站点不写这一列）。
func seedHDSkyRSS(t *testing.T, srv *Server) hdskyRSSIDs {
	t.Helper()
	db := global.GlobalDB.DB
	rule := models.FilterRule{Name: "r", Pattern: "*", Enabled: true}
	require.NoError(t, db.Create(&rule).Error)
	enabled := true
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("hdsky"), models.SiteConfig{
		Enabled: &enabled, AuthMethod: "cookie", Cookie: "uid=1; pass=1", APIUrl: "https://hdsky.me",
		RSS: []models.RSSConfig{
			{Name: "电影", URL: "https://hdsky.me/torrentrss.php?movie", Tag: "movie", IntervalMinutes: 10, FilterRuleIDs: []uint{rule.ID}},
			{Name: "剧集", URL: "https://hdsky.me/torrentrss.php?tv", Tag: "movie", IntervalMinutes: 10},
		},
	}))
	ids := hdskyRSSIDs{rule: rule.ID}
	ids.movie, ids.example = hdskyRSSRow(t, "电影").ID, hdskyRSSRow(t, "剧集").ID
	require.NoError(t, db.Model(&models.RSSSubscription{}).Where("id = ?", ids.example).Update("is_example", true).Error)
	return ids
}

func hdskyRSSRow(t *testing.T, name string) models.RSSSubscription {
	t.Helper()
	var row models.RSSSubscription
	require.NoError(t, global.GlobalDB.DB.Where("name = ?", name).First(&row).Error)
	return row
}

// assertHDSkyRSSKept：两条 RSS 的编号都没变，示例标记与「电影」的过滤规则关联都在。
func assertHDSkyRSSKept(t *testing.T, ids hdskyRSSIDs) {
	t.Helper()
	var n int64
	require.NoError(t, global.GlobalDB.DB.Model(&models.RSSSubscription{}).Count(&n).Error)
	assert.Equal(t, int64(2), n, "不新增、不重建")
	assert.Equal(t, ids.movie, hdskyRSSRow(t, "电影").ID, "编号不变")
	example := hdskyRSSRow(t, "剧集")
	assert.Equal(t, ids.example, example.ID, "编号不变")
	assert.True(t, example.IsExample, "示例标记不丢")
	rules, err := models.NewRSSFilterAssociationDB(global.GlobalDB.DB).GetFilterRuleIDsForRSS(ids.movie)
	require.NoError(t, err)
	assert.Equal(t, []uint{ids.rule}, rules, "过滤规则关联不丢")
}

// 网页编辑 RSS：站点详情页读出整站配置、改一条 RSS 再整站保存（编辑框不带示例标记）。保存以后各条 RSS 的编号不变，
// 改动存进同一行，示例标记与过滤规则关联都不丢。以前保存时 RSS 删掉重建，编号全变。
func TestApiSiteDetail_SaveKeepsRSSIDs(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)
	ids := seedHDSkyRSS(t, srv)

	get := httptest.NewRecorder()
	srv.apiSiteDetail(get, httptest.NewRequest(http.MethodGet, "/api/sites/hdsky", nil))
	require.Equal(t, http.StatusOK, get.Code, get.Body.String())
	var form models.SiteConfig
	require.NoError(t, json.Unmarshal(get.Body.Bytes(), &form))
	require.Len(t, form.RSS, 2)
	for i := range form.RSS {
		if form.RSS[i].ID == ids.example {
			form.RSS[i].IntervalMinutes = 20
			form.RSS[i].IsExample = false
		}
	}
	body, err := json.Marshal(form)
	require.NoError(t, err)

	post := httptest.NewRecorder()
	srv.apiSiteDetail(post, httptest.NewRequest(http.MethodPost, "/api/sites/hdsky", bytes.NewReader(body)))
	require.Equal(t, http.StatusOK, post.Code, post.Body.String())

	assertHDSkyRSSKept(t, ids)
	assert.Equal(t, int32(20), hdskyRSSRow(t, "剧集").IntervalMinutes, "改动存进原来那一行")
}

// 浏览器扩展同步 Cookie（PUT /api/sites/{站点}）：服务端读出整站配置、换上 Cookie 再整站保存。
// 同步以后各条 RSS 的编号不变，示例标记与过滤规则关联都不丢。
func TestUpdateSiteCredential_KeepsRSSIDs(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)
	require.NoError(t, srv.store.SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, AutoStart: true,
	}))
	ids := seedHDSkyRSS(t, srv)

	body, err := json.Marshal(credentialUpdateRequest{Cookie: credStrPtr("uid=2; pass=2")})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	srv.updateSiteCredential(w, httptest.NewRequest(http.MethodPut, "/api/sites/hdsky", bytes.NewReader(body)), models.SiteGroup("hdsky"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	settleCredentialReload(t, srv)

	assertHDSkyRSSKept(t, ids)
	sc, err := srv.store.GetSiteConf(models.SiteGroup("hdsky"))
	require.NoError(t, err)
	assert.Equal(t, "uid=2; pass=2", sc.Cookie, "Cookie 换成了这次同步的")
}
