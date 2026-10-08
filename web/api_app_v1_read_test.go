package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/web/middleware"
)

// appAs 用一个令牌主体调处理函数（路由与鉴权另有测试）。
func appAs(h http.HandlerFunc, method, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req = req.WithContext(middleware.WithPrincipal(req.Context(), &middleware.Principal{Kind: middleware.KindAPIToken, ID: "1", Scopes: []string{"app:read"}}))
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

// forbiddenKeys 是 App API 的回应里不能出现的字段名（Cookie、API key、passkey、密码、RSS 地址、链接、通知配置）。
var forbiddenKeys = []string{"cookie", "passkey", "apikey", "api_key", "password", "secret", "token", "rss", "url", "link", "credential", "config_json", "authorization"}

// assertNoSecrets 检查一段 JSON：字段名里没有上面那些词，值里没有 passkey=。
func assertNoSecrets(t *testing.T, raw []byte) {
	t.Helper()
	var v any
	require.NoError(t, json.Unmarshal(raw, &v), string(raw))
	var walk func(any, string)
	walk = func(x any, path string) {
		switch val := x.(type) {
		case map[string]any:
			for k, child := range val {
				lk := strings.ToLower(k)
				for _, bad := range forbiddenKeys {
					assert.NotContains(t, lk, bad, "%s.%s", path, k)
				}
				walk(child, path+"."+k)
			}
		case []any:
			for _, child := range val {
				walk(child, path+"[]")
			}
		case string:
			assert.NotContains(t, strings.ToLower(val), "passkey=", path)
		}
	}
	walk(v, "$")
}

func TestAppRedact(t *testing.T) {
	assert.Equal(t, "下载种子失败: Get \"https://pt.example/download.php?…\": EOF",
		appRedact(`下载种子失败: Get "https://pt.example/download.php?id=1&passkey=abc": EOF`))
	assert.Equal(t, "没有地址的错误", appRedact("没有地址的错误"))
}

// 概览：已启用站点的合计与今天的增量
func TestAppOverview(t *testing.T) {
	srv, repo := historyFixture(t)
	saveOn(t, repo, "2026-10-05", "hdsky", 100, 10, 1)
	saveOn(t, repo, "2026-10-06", "hdsky", 300, 30, 5)
	saveOn(t, repo, "2026-10-06", "disabledsite", 999, 999, 999)
	repo.SetClock(func() time.Time { return time.Date(2026, 10, 6, 20, 0, 0, 0, time.UTC) }, time.UTC)
	w := appAs(srv.appOverview, http.MethodGet, "/api/app/v1/overview")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	var ov AppOverview
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ov))
	assert.EqualValues(t, 200, ov.Today.Uploaded, "只算已启用的站点")
	require.Len(t, ov.Today.Sites, 1)
	assert.Equal(t, "hdsky", ov.Today.Sites[0].Site)

	prev := userInfoService
	userInfoService = nil
	t.Cleanup(func() { userInfoService = prev })
	assert.Equal(t, http.StatusServiceUnavailable, appAs(srv.appOverview, http.MethodGet, "/api/app/v1/overview").Code)
}

// 站点：名字、登录状态、签到与用户数据；没有 Cookie、passkey 与地址
func TestAppSites(t *testing.T) {
	srv, repo := historyFixture(t)
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.SiteLoginState{}, &models.SiteAttendanceLog{}))
	saveOn(t, repo, "2026-10-06", "hdsky", 300, 30, 5)
	require.NoError(t, global.GlobalDB.DB.Model(&models.SiteSetting{}).Where("name = ?", "hdsky").Updates(map[string]any{"passkey": "pk-secret", "api_key": "ak-secret"}).Error)
	w := appAs(srv.appSites, http.MethodGet, "/api/app/v1/sites")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	assert.NotContains(t, w.Body.String(), "pk-secret")
	assert.NotContains(t, w.Body.String(), "ak-secret")
	var list AppSiteList
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	assert.Empty(t, list.UserError)
	names := map[string]AppSite{}
	for _, s := range list.Items {
		names[s.Name] = s
	}
	require.Contains(t, names, "hdsky")
	assert.True(t, names["hdsky"].Enabled)
	assert.NotEmpty(t, names["hdsky"].Login.Tier)
}

// RSS 推送记录：分页、按站点与关键字筛；错误信息去掉了地址里的查询串
func TestAppTasks(t *testing.T) {
	srv, _ := historyFixture(t)
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.TorrentInfo{}))
	pushed := true
	now := time.Now()
	for i, title := range []string{"Dune.Part.Two.2024.1080p", "The.Last.of.Us.S02E01", "Dune.Part.One.2021.2160p"} {
		row := models.TorrentInfo{SiteName: "hdsky", TorrentID: string(rune('1' + i)), Title: title, IsPushed: &pushed, PushTime: &now, TorrentSize: 1 << 30}
		if i == 0 {
			row.LastError = `推送失败: Get "https://hdsky.me/download.php?id=1&passkey=pk-secret": EOF`
		}
		require.NoError(t, global.GlobalDB.DB.Create(&row).Error)
	}
	w := appAs(srv.appTasks, http.MethodGet, "/api/app/v1/tasks?site=hdsky&q=Dune&page_size=1")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	assert.NotContains(t, w.Body.String(), "pk-secret")
	var page AppPage[AppTask]
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	assert.Equal(t, 2, page.Total)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "Dune.Part.One.2021.2160p", page.Items[0].Title, "新的在前")
	assert.True(t, page.Items[0].Pushed)

	w = appAs(srv.appTasks, http.MethodGet, "/api/app/v1/tasks?q=Part.Two")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "download.php?…")
	for _, q := range []string{"?page=0", "?page_size=101", "?page=x", "?q=" + strings.Repeat("x", 201)} {
		assert.Equal(t, http.StatusBadRequest, appAs(srv.appTasks, http.MethodGet, "/api/app/v1/tasks"+q).Code, q)
	}
}

// 站点图标：站点名不对 400；没有缓存、不去取时回占位图
func TestAppFavicon(t *testing.T) {
	srv, _ := historyFixture(t)
	req := httptest.NewRequest(http.MethodGet, "/api/app/v1/favicon/x?nofetch=1", nil)
	req.SetPathValue("site", "nosuchsite")
	w := httptest.NewRecorder()
	srv.appFavicon(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Body.Bytes())

	req = httptest.NewRequest(http.MethodGet, "/api/app/v1/favicon/x", nil)
	req.SetPathValue("site", "a\\b")
	w = httptest.NewRecorder()
	srv.appFavicon(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// appRedact：错误信息里的凭证都要遮住（上游的回应体、tracker 地址、裸的 key=value、地址里的用户信息与路径里的长串），普通的说明不动
func TestAppRedactCredentials(t *testing.T) {
	for _, c := range []struct{ in, gone string }{
		{`Get "https://tracker.example/announce.php?passkey=QAPASS123": EOF`, "QAPASS123"},
		{`HTTPS://Tracker.Example/a?authkey=K9`, "K9"},
		{`下载器回应 403: passkey=abcdef123 已失效`, "abcdef123"},
		{`token=tok_live_x; sign=s1g`, "tok_live_x"},
		{`token=tok_live_x; sign=s1g`, "s1g"},
		{`udp://tracker.example:6969/announce/0123456789abcdef0123`, "0123456789abcdef0123"},
		{`https://tracker.example/0123456789abcdefABCD/announce`, "0123456789abcdefABCD"},
		{`连不上 http://admin:hunter2@127.0.0.1:8080/api/v2/auth/login`, "hunter2"},
		{`回应体：{"rsskey":"R5Sk3y"}`, "R5Sk3y"},
		{`apikey 是 a1b2c3d4e5f6a7b8c9d0e1f2a3b4`, "a1b2c3d4e5f6a7b8c9d0e1f2a3b4"},
	} {
		got := appRedact(c.in)
		assert.NotContains(t, got, c.gone, c.in)
	}
	assert.Equal(t, "下载器拒绝：磁盘空间不足（还剩 12 GB）", appRedact("下载器拒绝：磁盘空间不足（还剩 12 GB）"))
	assert.Equal(t, "站点 hdsky 没有启用", appRedact("站点 hdsky 没有启用"))
}

// 读站点用户数据失败时照样列出站点，但写明用户数据没读到，不能让 App 当成从没同步过
func TestAppSitesUserInfoFailure(t *testing.T) {
	srv, _ := historyFixture(t)
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.SiteLoginState{}, &models.SiteAttendanceLog{}))
	require.NoError(t, global.GlobalDB.DB.Migrator().DropTable("user_info"))
	w := appAs(srv.appSites, http.MethodGet, "/api/app/v1/sites")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	var list AppSiteList
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	assert.NotEmpty(t, list.Items)
	assert.NotEmpty(t, list.UserError)
}

// 今天的增量算不出来时写明原因，不能让 App 当成今天没有流量
func TestAppOverviewDeltaFailure(t *testing.T) {
	srv, repo := historyFixture(t)
	saveOn(t, repo, "2026-10-06", "hdsky", 300, 30, 5)
	require.NoError(t, global.GlobalDB.DB.Migrator().DropTable("user_info_daily_snapshot"))
	w := appAs(srv.appOverview, http.MethodGet, "/api/app/v1/overview")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var ov AppOverview
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ov))
	assert.NotEmpty(t, ov.Today.Error)
	assert.EqualValues(t, 300, ov.Totals.Uploaded, "合计照常")
}
