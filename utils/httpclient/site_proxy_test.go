package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setSiteProxy(t *testing.T, p *SiteProxyPolicy) {
	t.Helper()
	prev := siteProxy.Load()
	SetSiteProxyPolicy(p)
	t.Cleanup(func() { SetSiteProxyPolicy(prev) })
}

func TestResolveSiteProxy(t *testing.T) {
	const proxy = "http://u:p@proxy.test:8080"
	sites := map[string]bool{"hdsky": true}
	tests := []struct {
		name        string
		policy      *SiteProxyPolicy
		site        string
		wantProxy   string
		wantDecided bool
	}{
		{name: "没有设置", policy: nil, site: "hdsky"},
		{name: "关闭", policy: &SiteProxyPolicy{Enabled: false, Proxy: proxy, Mode: SiteProxyAll}, site: "hdsky"},
		{name: "开启但没有地址", policy: &SiteProxyPolicy{Enabled: true, Mode: SiteProxyAll}, site: "hdsky"},
		{name: "没有站点 ID", policy: &SiteProxyPolicy{Enabled: true, Proxy: proxy, Mode: SiteProxyAll}, site: "  "},
		{name: "全部站点", policy: &SiteProxyPolicy{Enabled: true, Proxy: proxy, Mode: SiteProxyAll}, site: "mteam", wantProxy: proxy, wantDecided: true},
		{name: "只有选中的：选中", policy: &SiteProxyPolicy{Enabled: true, Proxy: proxy, Mode: SiteProxyInclude, Sites: sites}, site: "hdsky", wantProxy: proxy, wantDecided: true},
		{name: "只有选中的：没选中直连", policy: &SiteProxyPolicy{Enabled: true, Proxy: proxy, Mode: SiteProxyInclude, Sites: sites}, site: "mteam", wantDecided: true},
		{name: "除了选中的：选中直连", policy: &SiteProxyPolicy{Enabled: true, Proxy: proxy, Mode: SiteProxyExclude, Sites: sites}, site: "hdsky", wantDecided: true},
		{name: "除了选中的：没选中", policy: &SiteProxyPolicy{Enabled: true, Proxy: proxy, Mode: SiteProxyExclude, Sites: sites}, site: "mteam", wantProxy: proxy, wantDecided: true},
		{name: "站点 ID 的大小写与空白不影响", policy: &SiteProxyPolicy{Enabled: true, Proxy: proxy, Mode: SiteProxyInclude, Sites: sites}, site: " HDSky ", wantProxy: proxy, wantDecided: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setSiteProxy(t, tt.policy)
			got, decided := ResolveSiteProxy(tt.site)
			assert.Equal(t, tt.wantDecided, decided)
			assert.Equal(t, tt.wantProxy, got)
		})
	}
}

func TestResolveSiteProxyOrEnv(t *testing.T) {
	t.Cleanup(SetEnvProxyForTest("http://env.test:3128"))
	setSiteProxy(t, nil)
	assert.Equal(t, "http://env.test:3128", ResolveSiteProxyOrEnv("hdsky", "https://hdsky.me/"), "设置关闭时按环境变量")

	setSiteProxy(t, &SiteProxyPolicy{Enabled: true, Proxy: "http://site.test:1081", Mode: SiteProxyInclude, Sites: map[string]bool{"hdsky": true}})
	assert.Equal(t, "http://site.test:1081", ResolveSiteProxyOrEnv("hdsky", "https://hdsky.me/"))
	assert.Equal(t, "", ResolveSiteProxyOrEnv("mteam", "https://kp.m-team.cc/"), "开启后没选中的站点直连，不认环境变量")
	assert.Equal(t, "http://env.test:3128", ResolveSiteProxyOrEnv("", "https://example.test/"), "没有站点 ID 时按环境变量")
}

func TestSiteTransportProxy(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://hdsky.me/torrentrss.php", nil)
	t.Cleanup(SetEnvProxyForTest("http://env.test:3128"))

	setSiteProxy(t, nil)
	u, err := SiteTransportProxy("hdsky", req)
	require.NoError(t, err)
	require.NotNil(t, u)
	assert.Equal(t, "env.test:3128", u.Host, "设置关闭时按环境变量")

	setSiteProxy(t, &SiteProxyPolicy{Enabled: true, Proxy: "socks5://site.test:1080", Mode: SiteProxyExclude, Sites: map[string]bool{"mteam": true}})
	u, err = SiteTransportProxy("hdsky", req)
	require.NoError(t, err)
	require.NotNil(t, u)
	assert.Equal(t, "socks5://site.test:1080", u.String())
	u, err = SiteTransportProxy("mteam", req)
	require.NoError(t, err)
	assert.Nil(t, u, "被排除的站点直连")
}

func TestSetEnvProxyForTestRestores(t *testing.T) {
	before := envProxy.Load()
	restore := SetEnvProxyForTest("")
	assert.Equal(t, "", envProxy.Load().forURL("http://example.test/"))
	u, err := envProxy.Load().forRequest(httptest.NewRequest(http.MethodGet, "http://example.test/", nil))
	require.NoError(t, err)
	assert.Nil(t, u)
	restore()
	assert.Same(t, before, envProxy.Load())
}

func TestSiteTransportUsesSiteFromContext(t *testing.T) {
	tr := SiteTransport()
	assert.Same(t, tr, SiteTransport(), "共用同一个 Transport")
	setSiteProxy(t, &SiteProxyPolicy{Enabled: true, Proxy: "http://site.test:1081", Mode: SiteProxyInclude, Sites: map[string]bool{"hdsky": true}})
	t.Cleanup(SetEnvProxyForTest(""))

	req := httptest.NewRequest(http.MethodGet, "https://hdsky.me/", nil).WithContext(WithSiteID(context.Background(), "hdsky"))
	u, err := tr.Proxy(req)
	require.NoError(t, err)
	require.NotNil(t, u)
	assert.Equal(t, "site.test:1081", u.Host)

	other := httptest.NewRequest(http.MethodGet, "https://kp.m-team.cc/", nil).WithContext(WithSiteID(context.Background(), "mteam"))
	u, err = tr.Proxy(other)
	require.NoError(t, err)
	assert.Nil(t, u)
	assert.Equal(t, "", SiteIDFrom(context.Background()))
}

func TestParseProxyURL(t *testing.T) {
	for _, ok := range []string{"http://127.0.0.1:7890", "https://p.test", "socks5://u:p@p.test:1080", "socks5h://p.test:1080", "  http://p.test:1  "} {
		_, err := ParseProxyURL(ok)
		assert.NoError(t, err, ok)
	}
	for _, bad := range []string{"", "p.test:1080", "ftp://p.test", "http://", "://x"} {
		_, err := ParseProxyURL(bad)
		assert.Error(t, err, bad)
	}
}

func TestMaskProxyURL(t *testing.T) {
	assert.Equal(t, "http://user:***@p.test:1081", MaskProxyURL("http://user:secret@p.test:1081"))
	assert.Equal(t, "socks5://user@p.test:1080", MaskProxyURL("socks5://user@p.test:1080"), "只有用户名时不变")
	assert.Equal(t, "http://p.test:1081", MaskProxyURL("http://p.test:1081"))
	assert.Equal(t, "***", MaskProxyURL("http://user:secret@[::1"), "解析不了时不交出原文")
}
