package internal

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeebo/bencode"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
	"github.com/sunerpy/pt-tools/utils/httpclient"
)

// 站点代理在各接入点的三种断言：选中 → 经设置的代理；没选中 → 直连（即使「环境变量」给了代理）；
// 设置关闭或者没有站点 ID → 按环境变量。假代理与目标回的内容不同，看回来的是谁就知道走了哪条路。

type hitServer struct {
	*httptest.Server
	hits atomic.Int32
}

func newHitServer(t *testing.T, body []byte, contentType string) *hitServer {
	t.Helper()
	h := &hitServer{}
	h.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		h.hits.Add(1)
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(h.Close)
	return h
}

func useSiteProxy(t *testing.T, p *httpclient.SiteProxyPolicy) {
	t.Helper()
	httpclient.SetSiteProxyPolicy(p)
	t.Cleanup(func() { httpclient.SetSiteProxyPolicy(nil) })
}

func includeSites(proxy string, sites ...string) *httpclient.SiteProxyPolicy {
	set := map[string]bool{}
	for _, s := range sites {
		set[s] = true
	}
	return &httpclient.SiteProxyPolicy{Enabled: true, Proxy: proxy, Mode: httpclient.SiteProxyInclude, Sites: set}
}

// deadProxy 是带密码、指向已关闭端口的代理地址。
func deadProxy(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	u := strings.Replace(srv.URL, "http://", "http://user:topsecret@", 1)
	srv.Close()
	return u
}

func TestFetchRSSFeed_SiteProxy(t *testing.T) {
	rss := func(title string) []byte {
		return []byte(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>` + title + `</title></channel></rss>`)
	}
	proxy := newHitServer(t, rss("via-proxy"), "application/rss+xml")
	target := newHitServer(t, rss("direct"), "application/rss+xml")
	t.Cleanup(httpclient.SetEnvProxyForTest(proxy.URL))
	title := func(siteID string) string {
		t.Helper()
		feed, err := FetchRSSFeed(context.Background(), siteID, target.URL+"/torrentrss.php")
		require.NoError(t, err)
		return feed.Title
	}

	useSiteProxy(t, nil)
	assert.Equal(t, "via-proxy", title("hdsky"), "设置关闭：按环境变量")

	useSiteProxy(t, includeSites(proxy.URL, "hdsky"))
	assert.Equal(t, "via-proxy", title("HDSky"), "选中：经设置的代理")
	assert.Equal(t, int32(0), target.hits.Load())

	useSiteProxy(t, includeSites(proxy.URL, "mteam"))
	assert.Equal(t, "direct", title("hdsky"), "没选中：直连，不认环境变量")
	assert.Equal(t, int32(1), target.hits.Load())

	t.Cleanup(httpclient.SetEnvProxyForTest(""))
	useSiteProxy(t, &httpclient.SiteProxyPolicy{Enabled: true, Proxy: proxy.URL, Mode: httpclient.SiteProxyAll})
	before := proxy.hits.Load()
	assert.Equal(t, "direct", title(""), "没有站点 ID：按环境变量（这里是直连）")
	assert.Equal(t, before, proxy.hits.Load())
}

func TestFetchRSSFeed_ProxyErrorHasNoPassword(t *testing.T) {
	target := newHitServer(t, []byte("<rss/>"), "application/rss+xml")
	useSiteProxy(t, &httpclient.SiteProxyPolicy{Enabled: true, Proxy: deadProxy(t), Mode: httpclient.SiteProxyAll})
	_, err := FetchRSSFeed(context.Background(), "hdsky", target.URL+"/torrentrss.php?passkey=PASSKEY")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "topsecret")
	assert.NotContains(t, err.Error(), "PASSKEY")
}

// RSS 任务按 RSS 绑定的站点判定（这里的假站点是 springsunday）。
func TestFetchAndDownloadFreeRSSUnified_UsesSiteProxy(t *testing.T) {
	setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	feed := []byte(rssBody(""))
	proxy := newHitServer(t, feed, "application/rss+xml")
	target := newHitServer(t, feed, "application/rss+xml")
	t.Cleanup(httpclient.SetEnvProxyForTest(""))
	useSiteProxy(t, includeSites(proxy.URL, "springsunday"))

	site := &unifiedFake{enabled: true}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site, models.RSSConfig{Name: "r", URL: target.URL + "/torrentrss.php", Tag: "movie"}))
	assert.Equal(t, int32(1), proxy.hits.Load(), "春天选中了：取 RSS 经代理")
	assert.Equal(t, int32(0), target.hits.Load())
}

func torrentBytes(t *testing.T, name string) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": name}}))
	return buf.Bytes()
}

func TestAttemptDownloadWithContext_SiteProxy(t *testing.T) {
	viaProxy, direct := torrentBytes(t, "via-proxy"), torrentBytes(t, "direct")
	proxy := newHitServer(t, viaProxy, "application/x-bittorrent")
	target := newHitServer(t, direct, "application/x-bittorrent")
	hashOf := func(b []byte) string {
		h, err := qbit.ComputeTorrentHash(b)
		require.NoError(t, err)
		return h
	}
	t.Cleanup(httpclient.SetEnvProxyForTest(proxy.URL))
	ctx := context.Background()

	useSiteProxy(t, nil)
	got, err := attemptDownloadWithContext(ctx, "byrpt", target.URL+"/download.php?id=1", "a", t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, hashOf(viaProxy), got, "设置关闭：按环境变量（以前这里一直直连）")

	useSiteProxy(t, includeSites(proxy.URL, "byrpt"))
	got, err = attemptDownloadWithContext(ctx, "BYRPT", target.URL+"/download.php?id=1", "b", t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, hashOf(viaProxy), got, "选中：经设置的代理")
	assert.Equal(t, int32(0), target.hits.Load())

	useSiteProxy(t, includeSites(proxy.URL, "hdsky"))
	got, err = attemptDownloadWithContext(ctx, "byrpt", target.URL+"/download.php?id=1", "c", t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, hashOf(direct), got, "没选中：直连，不认环境变量")
	assert.Equal(t, int32(1), target.hits.Load())
}

func TestAttemptDownloadWithContext_CanceledContext(t *testing.T) {
	target := newHitServer(t, torrentBytes(t, "x"), "application/x-bittorrent")
	t.Cleanup(httpclient.SetEnvProxyForTest(""))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := attemptDownloadWithContext(ctx, "hdsky", target.URL+"/download.php?id=1", "x", t.TempDir())
	require.Error(t, err)
	assert.Equal(t, int32(0), target.hits.Load(), "ctx 已取消时不发请求")
}

func TestDownloadTorrent_ProxyErrorHasNoPasswordInErrorOrLog(t *testing.T) {
	target := newHitServer(t, torrentBytes(t, "x"), "application/x-bittorrent")
	useSiteProxy(t, &httpclient.SiteProxyPolicy{Enabled: true, Proxy: deadProxy(t), Mode: httpclient.SiteProxyAll})
	core0, logs := observer.New(zapcore.DebugLevel)
	saved := global.GlobalLogger
	global.GlobalLogger = zap.New(core0)
	t.Cleanup(func() { global.GlobalLogger = saved })

	_, err := downloadTorrent("hdsky", target.URL+"/download.php?id=1&passkey=PASSKEY", "x", t.TempDir(), 2, 0)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "topsecret")
	require.NotEmpty(t, logs.All(), "重试时记了日志")
	for _, e := range logs.All() {
		assert.NotContains(t, e.Message, "topsecret")
		assert.NotContains(t, e.Message, "PASSKEY")
	}
}

func TestFetchMTorrentDetail_SiteProxy(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	detail := func(name string) []byte {
		return []byte(`{"code":"0","data":{"id":"1","name":"` + name + `","smallDescr":"S","size":"1024","status":{"discount":"FREE"}}}`)
	}
	proxy := newHitServer(t, detail("via-proxy"), "application/json")
	target := newHitServer(t, detail("direct"), "application/json")
	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("mteam"), models.SiteConfig{
		Enabled: boolPtrIX(true), AuthMethod: "api_key", APIKey: "k", APIUrl: target.URL,
	}))
	t.Cleanup(httpclient.SetEnvProxyForTest(proxy.URL))
	fetch := func() string {
		t.Helper()
		d, err := fetchMTorrentDetail(context.Background(), models.SiteGroup("mteam"), &gofeed.Item{GUID: "1"})
		require.NoError(t, err)
		return d.GetName()
	}

	useSiteProxy(t, nil)
	assert.Equal(t, "via-proxy", fetch(), "设置关闭：按环境变量")
	useSiteProxy(t, includeSites(proxy.URL, "mteam"))
	assert.Equal(t, "via-proxy", fetch(), "选中：经设置的代理")
	assert.Equal(t, int32(0), target.hits.Load())
	useSiteProxy(t, includeSites(proxy.URL, "hdsky"))
	assert.Equal(t, "direct", fetch(), "没选中：直连，不认环境变量")
	assert.Equal(t, int32(1), target.hits.Load())

	useSiteProxy(t, &httpclient.SiteProxyPolicy{Enabled: true, Proxy: deadProxy(t), Mode: httpclient.SiteProxyAll})
	_, err := fetchMTorrentDetail(context.Background(), models.SiteGroup("mteam"), &gofeed.Item{GUID: "1"})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "topsecret", "代理连不上时错误里没有密码")
}
