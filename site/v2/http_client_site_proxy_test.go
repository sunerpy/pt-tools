package v2

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/utils/httpclient"
)

// recorder 记下 httptest 服务器收到的请求（代理收到的是绝对形式的 URL）。
type recorder struct {
	mu   sync.Mutex
	urls []string
}

func (r *recorder) add(u string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.urls = append(r.urls, u)
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.urls)
}

func (r *recorder) last() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.urls) == 0 {
		return ""
	}
	return r.urls[len(r.urls)-1]
}

// newProxyAndTarget 起一个只记录请求的假代理（回 "via-proxy"）和一个目标（回 "direct"）。
func newProxyAndTarget(t *testing.T) (proxy *httptest.Server, proxied *recorder, target *httptest.Server, direct *recorder) {
	t.Helper()
	proxied, direct = &recorder{}, &recorder{}
	proxy = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied.add(r.URL.String())
		_, _ = io.WriteString(w, "via-proxy")
	}))
	t.Cleanup(proxy.Close)
	target = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		direct.add(r.URL.String())
		_, _ = io.WriteString(w, "direct")
	}))
	t.Cleanup(target.Close)
	return proxy, proxied, target, direct
}

func setSitePolicy(t *testing.T, p *httpclient.SiteProxyPolicy) {
	t.Helper()
	httpclient.SetSiteProxyPolicy(p)
	t.Cleanup(func() { httpclient.SetSiteProxyPolicy(nil) })
}

// 站点代理：选中 → 经设置的代理；没选中 → 直连（即使「环境变量」给了代理）；关闭 → 按环境变量。
// 同一个客户端，运行中换设置，下一个请求就按新设置走。
func TestSiteHTTPClient_SiteProxyPolicy(t *testing.T) {
	proxy, proxied, target, direct := newProxyAndTarget(t)
	t.Cleanup(httpclient.SetEnvProxyForTest(proxy.URL))

	cfg := DefaultSiteHTTPClientConfig()
	cfg.SiteID = "HDSky"
	c := NewSiteHTTPClient(cfg)
	t.Cleanup(func() { _ = c.Close() })
	ctx := context.Background()
	get := func() string {
		t.Helper()
		resp, err := c.Get(ctx, target.URL+"/torrents.php", nil)
		require.NoError(t, err)
		return string(resp.Body)
	}

	setSitePolicy(t, nil)
	assert.Equal(t, "via-proxy", get(), "设置关闭：按环境变量")
	assert.Equal(t, 1, proxied.count())
	assert.Equal(t, target.URL+"/torrents.php", proxied.last(), "代理收到的是去目标的请求")

	setSitePolicy(t, &httpclient.SiteProxyPolicy{Enabled: true, Proxy: proxy.URL, Mode: httpclient.SiteProxyInclude, Sites: map[string]bool{"hdsky": true}})
	assert.Equal(t, "via-proxy", get(), "选中：经设置的代理")
	assert.Equal(t, 2, proxied.count())
	assert.Equal(t, 0, direct.count())

	setSitePolicy(t, &httpclient.SiteProxyPolicy{Enabled: true, Proxy: proxy.URL, Mode: httpclient.SiteProxyInclude, Sites: map[string]bool{"mteam": true}})
	assert.Equal(t, "direct", get(), "没选中：直连，不认环境变量")
	assert.Equal(t, 2, proxied.count())
	assert.Equal(t, 1, direct.count())

	setSitePolicy(t, &httpclient.SiteProxyPolicy{Enabled: true, Proxy: proxy.URL, Mode: httpclient.SiteProxyExclude, Sites: map[string]bool{"hdsky": true}})
	assert.Equal(t, "direct", get(), "被排除：直连")
	assert.Equal(t, 2, direct.count())

	setSitePolicy(t, nil)
	assert.Equal(t, "via-proxy", get(), "关掉以后回到环境变量")
	assert.Equal(t, 3, proxied.count())
}

// 没有站点 ID 的客户端：设置开着也不判定，按环境变量。
func TestSiteHTTPClient_NoSiteIDFallsBackToEnv(t *testing.T) {
	proxy, proxied, target, direct := newProxyAndTarget(t)
	setSitePolicy(t, &httpclient.SiteProxyPolicy{Enabled: true, Proxy: proxy.URL, Mode: httpclient.SiteProxyAll})
	c := NewSiteHTTPClient(DefaultSiteHTTPClientConfig())
	t.Cleanup(func() { _ = c.Close() })

	t.Cleanup(httpclient.SetEnvProxyForTest(""))
	resp, err := c.Get(context.Background(), target.URL+"/", nil)
	require.NoError(t, err)
	assert.Equal(t, "direct", string(resp.Body))
	assert.Equal(t, 0, proxied.count())
	assert.Equal(t, 1, direct.count())
}

// 代理地址带密码、连不上时，错误里没有密码。
func TestSiteHTTPClient_ProxyErrorHasNoPassword(t *testing.T) {
	_, _, target, _ := newProxyAndTarget(t)
	closed := httptest.NewServer(http.NotFoundHandler())
	dead := strings.Replace(closed.URL, "http://", "http://user:topsecret@", 1)
	closed.Close()
	setSitePolicy(t, &httpclient.SiteProxyPolicy{Enabled: true, Proxy: dead, Mode: httpclient.SiteProxyAll})
	cfg := DefaultSiteHTTPClientConfig()
	cfg.SiteID = "hdsky"
	c := NewSiteHTTPClient(cfg)
	t.Cleanup(func() { _ = c.Close() })

	_, err := c.Get(context.Background(), target.URL+"/", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNetworkError)
	assert.NotContains(t, err.Error(), "topsecret")
}

func siteIDOfDriverClient(t *testing.T, site Site) string {
	t.Helper()
	switch s := site.(type) {
	case *BaseSite[NexusPHPRequest, NexusPHPResponse]:
		return s.GetDriver().(*NexusPHPDriver).httpClient.siteID
	case *BaseSite[MTorrentRequest, MTorrentResponse]:
		return s.GetDriver().(*MTorrentDriver).httpClient.siteID
	case *BaseSite[Unit3DRequest, Unit3DResponse]:
		return s.GetDriver().(*Unit3DDriver).httpClient.siteID
	case *BaseSite[HDDolbyRequest, HDDolbyResponse]:
		return s.GetDriver().(*HDDolbyDriver).httpClient.siteID
	case *BaseSite[GazelleRequest, GazelleResponse]:
		return s.GetDriver().(*GazelleDriver).httpClient.siteID
	}
	t.Fatalf("不认识的站点类型 %T", site)
	return ""
}

// 每个驱动工厂都把注册表 ID 交给了站点的 HTTP 客户端（站点代理按它判定）。
func TestDriverFactoriesPassSiteIDToHTTPClient(t *testing.T) {
	opts, err := json.Marshal(map[string]string{"cookie": "c=1", "apiKey": "k", "passkey": "p"})
	require.NoError(t, err)
	for _, tc := range []struct {
		create func(SiteConfig, *zap.Logger) (Site, error)
		id     string
	}{
		{createNexusPHPSite, "proxytest-nexus"},
		{createMTorrentSite, "proxytest-mteam"},
		{createUnit3DSite, "proxytest-unit3d"},
		{createHDDolbySite, "proxytest-hddolby"},
		{createGazelleSite, "proxytest-gazelle"},
	} {
		site, err := tc.create(SiteConfig{ID: tc.id, Name: tc.id, BaseURL: "https://example.test", Options: opts}, zap.NewNop())
		require.NoError(t, err, tc.id)
		assert.Equal(t, tc.id, siteIDOfDriverClient(t, site), tc.id)
	}
}
