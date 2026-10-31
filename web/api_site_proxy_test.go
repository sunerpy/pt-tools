package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/utils/httpclient"
)

func setupSiteProxyServer(t *testing.T) *Server {
	t.Helper()
	t.Setenv("PT_TOOLS_SECRET_KEY", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")
	srv, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.SiteProxySetting{}, &models.FaviconCache{}))
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", AuthMethod: "cookie", Enabled: true}).Error)
	httpclient.SetSiteProxyPolicy(nil)
	t.Cleanup(func() { httpclient.SetSiteProxyPolicy(nil) })
	return srv
}

func siteProxyCall(t *testing.T, srv *Server, method, body string) (*httptest.ResponseRecorder, SiteProxyResponse) {
	t.Helper()
	w := httptest.NewRecorder()
	srv.apiSiteProxy(w, httptest.NewRequest(method, "/api/site-proxy", bytes.NewBufferString(body)))
	var out SiteProxyResponse
	if w.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out), w.Body.String())
	}
	return w, out
}

func TestSiteProxyAPI_DefaultsAndOptions(t *testing.T) {
	srv := setupSiteProxyServer(t)
	w, got := siteProxyCall(t, srv, http.MethodGet, "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.False(t, got.Enabled)
	assert.Equal(t, "", got.ProxyURL)
	assert.Equal(t, httpclient.SiteProxyAll, got.Mode, "没有设置时默认「全部站点」")
	assert.Equal(t, []string{}, got.Sites)
	assert.Nil(t, got.UpdatedAt)
	assert.Contains(t, w.Body.String(), `"updated_at":null`)

	require.NotEmpty(t, got.SiteOptions)
	assert.Equal(t, "hdsky", got.SiteOptions[0].ID, "启用的站点排在前面")
	assert.True(t, got.SiteOptions[0].Enabled)
	assert.Equal(t, "HDSky", got.SiteOptions[0].Name)
	seen := map[string]bool{}
	for i, o := range got.SiteOptions {
		assert.Equal(t, strings.ToLower(o.ID), o.ID, "ID 是小写")
		assert.False(t, seen[o.ID], "ID 不重复")
		seen[o.ID] = true
		if i > 0 && !o.Enabled && !got.SiteOptions[i-1].Enabled {
			assert.LessOrEqual(t, strings.ToLower(got.SiteOptions[i-1].Name), strings.ToLower(o.Name), "同组按名字排")
		}
	}
	assert.True(t, seen["byrpt"])
}

func TestSiteProxyAPI_SaveMaskKeepClear(t *testing.T) {
	srv := setupSiteProxyServer(t)
	w, got := siteProxyCall(t, srv, http.MethodPut, `{"enabled":true,"proxy_url":"http://user:topsecret@192.168.3.150:1081","mode":"include","sites":[" HDSky ","byrpt"]}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "http://user:***@192.168.3.150:1081", got.ProxyURL, "读出时密码打码")
	assert.NotContains(t, w.Body.String(), "topsecret")
	assert.Equal(t, []string{"byrpt", "hdsky"}, got.Sites)
	require.NotNil(t, got.UpdatedAt)

	// 保存以后立即生效
	proxy, decided := httpclient.ResolveSiteProxy("byrpt")
	assert.True(t, decided)
	assert.Equal(t, "http://user:topsecret@192.168.3.150:1081", proxy)

	// 交回打码地址：不改
	w, got = siteProxyCall(t, srv, http.MethodPut, `{"enabled":true,"proxy_url":"http://user:***@192.168.3.150:1081","mode":"exclude","sites":["hdsky"]}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, httpclient.SiteProxyExclude, got.Mode)
	proxy, _ = httpclient.ResolveSiteProxy("byrpt")
	assert.Equal(t, "http://user:topsecret@192.168.3.150:1081", proxy)
	proxy, decided = httpclient.ResolveSiteProxy("hdsky")
	assert.True(t, decided)
	assert.Equal(t, "", proxy, "被排除的站点直连")

	// 清除
	w, got = siteProxyCall(t, srv, http.MethodPut, `{"enabled":false,"proxy_url":"","mode":"all","sites":[]}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "", got.ProxyURL)
	_, decided = httpclient.ResolveSiteProxy("byrpt")
	assert.False(t, decided)
}

func TestSiteProxyAPI_Validation(t *testing.T) {
	srv := setupSiteProxyServer(t)
	for _, tc := range []struct{ name, body, want string }{
		{"坏地址", `{"enabled":true,"proxy_url":"ftp://user:topsecret@p.test","mode":"all"}`, "代理地址要以"},
		{"坏模式", `{"enabled":true,"proxy_url":"http://p.test:1","mode":"x"}`, "不认识的模式"},
		{"开启却没地址", `{"enabled":true,"proxy_url":"","mode":"all"}`, "开启时要填代理地址"},
		{"不认识的站点", `{"enabled":true,"proxy_url":"http://p.test:1","mode":"include","sites":["nosuchsite"]}`, "不认识的站点：nosuchsite"},
		{"只有选中的却没选", `{"enabled":true,"proxy_url":"http://p.test:1","mode":"include","sites":[]}`, "没有选站点"},
		{"不是 JSON", `{`, "请求格式错误"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, _ := siteProxyCall(t, srv, http.MethodPut, tc.body)
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), tc.want)
			assert.NotContains(t, w.Body.String(), "topsecret")
			assert.NotContains(t, w.Body.String(), "站点代理设置有误", "只回原因，界面自己会写是站点代理")
		})
	}
	var n int64
	require.NoError(t, global.GlobalDB.DB.Model(&models.SiteProxySetting{}).Count(&n).Error)
	assert.Zero(t, n)

	w, _ := siteProxyCall(t, srv, http.MethodDelete, "")
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestSiteProxyAPI_DropsSitesNoLongerOffered(t *testing.T) {
	srv := setupSiteProxyServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteProxySetting{ID: 1, Mode: "exclude", SitesJSON: `["hdsky","gone-site"]`}).Error)
	w, got := siteProxyCall(t, srv, http.MethodGet, "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{"hdsky"}, got.Sites, "注册表里没有的站点不回给界面")
}

func TestSiteProxyAPI_ExcludedFromConfigGateway(t *testing.T) {
	found := false
	for _, s := range configExcluded {
		if strings.Contains(s, "站点代理") {
			found = true
		}
	}
	assert.True(t, found, "App API 与 MCP 的配置网关读改不到站点代理")
}

// 站点图标按站点代理判定：选中 → 代理；没选中 → 直连（即使「环境变量」给了代理）；关闭 → 按环境变量。
func TestFetchFavicon_SiteProxy(t *testing.T) {
	setupSiteProxyServer(t)
	if sqlDB, err := global.GlobalDB.DB.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	icon := func(tag string) []byte { return append([]byte("\x00\x00\x01\x00"), tag...) }
	var proxied, direct atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxied.Add(1)
		_, _ = w.Write(icon("via-proxy"))
	}))
	t.Cleanup(proxy.Close)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		direct.Add(1)
		_, _ = w.Write(icon("direct"))
	}))
	t.Cleanup(target.Close)
	t.Cleanup(httpclient.SetEnvProxyForTest(proxy.URL))
	fs := &FaviconService{}
	stored := func() string {
		t.Helper()
		var c models.FaviconCache
		require.NoError(t, global.GlobalDB.DB.Where("site_id = ?", "hdsky").First(&c).Error)
		return string(c.Data[4:])
	}

	httpclient.SetSiteProxyPolicy(nil)
	require.NoError(t, fs.fetchAndSave("hdsky", "HDSky", target.URL+"/favicon.ico"))
	assert.Equal(t, "via-proxy", stored(), "设置关闭：按环境变量")

	httpclient.SetSiteProxyPolicy(&httpclient.SiteProxyPolicy{Enabled: true, Proxy: proxy.URL, Mode: httpclient.SiteProxyInclude, Sites: map[string]bool{"hdsky": true}})
	require.NoError(t, fs.fetchAndSave("hdsky", "HDSky", target.URL+"/favicon.ico"))
	assert.Equal(t, "via-proxy", stored(), "选中：经设置的代理")
	assert.Equal(t, int32(0), direct.Load())

	httpclient.SetSiteProxyPolicy(&httpclient.SiteProxyPolicy{Enabled: true, Proxy: proxy.URL, Mode: httpclient.SiteProxyInclude, Sites: map[string]bool{"byrpt": true}})
	require.NoError(t, fs.fetchAndSave("hdsky", "HDSky", target.URL+"/favicon.ico"))
	assert.Equal(t, "direct", stored(), "没选中：直连，不认环境变量")
	assert.Equal(t, int32(1), direct.Load())

	closed := httptest.NewServer(http.NotFoundHandler())
	dead := strings.Replace(closed.URL, "http://", "http://user:topsecret@", 1)
	closed.Close()
	httpclient.SetSiteProxyPolicy(&httpclient.SiteProxyPolicy{Enabled: true, Proxy: dead, Mode: httpclient.SiteProxyAll})
	err := fs.fetchAndSave("hdsky", "HDSky", target.URL+"/favicon.ico")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "topsecret", "代理连不上时错误里没有密码")
}

// 站点页「试一下链接」带上站点时按站点判定，不带时按环境变量。
func TestRSSLinkTest_UsesSite(t *testing.T) {
	srv := setupSiteProxyServer(t)
	rss := func(title string) string {
		return `<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>` + title + `</title></channel></rss>`
	}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = io.WriteString(w, rss("via-proxy"))
	}))
	t.Cleanup(proxy.Close)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = io.WriteString(w, rss("direct"))
	}))
	t.Cleanup(target.Close)
	t.Cleanup(httpclient.SetEnvProxyForTest(""))
	httpclient.SetSiteProxyPolicy(&httpclient.SiteProxyPolicy{Enabled: true, Proxy: proxy.URL, Mode: httpclient.SiteProxyInclude, Sites: map[string]bool{"hdsky": true}})

	test := func(body string) string {
		t.Helper()
		w := httptest.NewRecorder()
		srv.apiRSSLinkTest(w, httptest.NewRequest(http.MethodPost, "/api/rss/test", bytes.NewBufferString(body)))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var out RSSLinkTestResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		return out.Title
	}
	assert.Equal(t, "via-proxy", test(`{"url":"`+target.URL+`/torrentrss.php","site":"hdsky"}`))
	assert.Equal(t, "direct", test(`{"url":"`+target.URL+`/torrentrss.php"}`), "不带站点：按环境变量（这里是直连）")
}
