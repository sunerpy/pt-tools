package v2

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// 搜索结果的代理下载地址 /api/site/{id}/torrent/{种子}/download 要用站点注册的 ID（网页下载按它找站点）：
// 以前 NexusPHP 从域名猜（pt.xingyungept.org → xingyungept），星陨阁、OpenCD、U2 等站点下载回 404（2026-10-26）。
// 这里的 ID 没有站点定义，测试服务器的地址也猜不出它。
func TestNexusPHPSearchDownloadURLUsesRegisteredSiteID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><table class="torrents"><tbody>
			<tr><td>Header</td></tr>
			<tr><td><img alt="Movie" /></td><td><a href="details.php?id=12345">Test Movie 2024</a></td>
				<td></td><td><span>2024-01-01</span></td><td>1.5 GB</td><td>100</td><td>10</td><td>500</td></tr>
		</tbody></table></body></html>`))
	}))
	defer server.Close()

	site, err := createNexusPHPSite(SiteConfig{
		ID: "custom-nexus", Name: "Custom", BaseURL: server.URL,
		Options: json.RawMessage(`{"cookie":"c_secure_pass=x"}`),
	}, zap.NewNop())
	require.NoError(t, err)

	items, err := site.Search(context.Background(), SearchQuery{Keyword: "test"})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "custom-nexus", items[0].SourceSite)
	assert.Equal(t, "/api/site/custom-nexus/torrent/12345/download", items[0].DownloadURL)
}

func TestNexusPHPDriverSiteIDPrecedence(t *testing.T) {
	def := &SiteDefinition{ID: "xingyunge"}
	cases := []struct {
		name   string
		driver *NexusPHPDriver
		want   string
	}{
		{"注册的 ID", &NexusPHPDriver{BaseURL: "https://pt.xingyungept.org", siteID: "xingyunge", siteName: "other", siteDefinition: &SiteDefinition{ID: "def"}}, "xingyunge"},
		{"站点定义", &NexusPHPDriver{BaseURL: "https://pt.xingyungept.org", siteName: "other", siteDefinition: def}, "xingyunge"},
		{"多地址的站点名", &NexusPHPDriver{BaseURL: "https://hdsky.me", siteName: SiteNameHDSky}, "hdsky"},
		{"都没有时按域名猜", &NexusPHPDriver{BaseURL: "https://springsunday.net"}, "springsunday"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, c.driver.getSiteID())
		})
	}
}

// Gazelle（MooKo）以前给的是站点页面上的下载链接：网页里跨站取不到，推送也不认（只认代理地址）。
func TestGazelleSearchDownloadURLUsesProxy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","response":{"results":[{"groupId":1,"groupName":"Test",
			"torrents":[{"torrentId":777,"size":1024,"seeders":3}]}]}}`))
	}))
	defer server.Close()

	site, err := createGazelleSite(SiteConfig{
		ID: "mooko", Name: "MooKo", BaseURL: server.URL, Options: json.RawMessage(`{"apiKey":"k"}`),
	}, zap.NewNop())
	require.NoError(t, err)
	items, err := site.Search(context.Background(), SearchQuery{Keyword: "test"})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "/api/site/mooko/torrent/777/download", items[0].DownloadURL)
	assert.NotContains(t, items[0].DownloadURL, server.URL, "不给站点页面上的链接")
}
