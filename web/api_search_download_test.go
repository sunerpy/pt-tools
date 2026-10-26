package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 搜索页的「下载种子」「复制下载链接」与推送都用搜索结果里的 /api/site/{站点}/torrent/{种子}/download，
// 后端按地址里的站点 ID 找站点。星陨阁（pt.xingyungept.org）以前拼成 xingyungept，下载回 404
// （2026-10-26 用户：「下载搜索到的种子文件时：下载失败: HTTP 404: Not Found」）；
// MooKo（Gazelle）给的是站点页面上的链接，网页上取不到，推送也不认。

const searchDownloadTorrent = "d8:announce9:http://a/4:infod4:name4:testee"

const xingyungeSearchPage = `<html><body><table class="torrents"><tbody>
<tr>
	<td class="rowfollow"><img alt="Movie" /></td>
	<td class="rowfollow"><table class="torrentname"><tr><td class="embedded">
		<a href="details.php?id=98525&amp;hit=1">Weathering.With.You.2019.2160p.UHD.BluRay</a>
	</td></tr></table></td>
	<td class="rowfollow"></td>
	<td class="rowfollow"><span title="2026-10-20 10:00:00">6天前</span></td>
	<td class="rowfollow">20.0 GB</td>
	<td class="rowfollow">10</td>
	<td class="rowfollow">0</td>
	<td class="rowfollow">5</td>
</tr>
</tbody></table></body></html>`

const xingyungeDetailPage = `<html><body><table>
<tr><td class="rowhead">下载</td><td class="rowfollow"><a href="download.php?id=98525&amp;passkey=fakepass">Weathering.torrent</a></td></tr>
</table></body></html>`

const mookoBrowse = `{"status":"success","response":{"currentPage":1,"pages":1,"results":[
{"groupId":1,"groupName":"Weathering With You","torrents":[{"torrentId":777,"size":1024,"seeders":3,"time":"2026-10-20 10:00:00"}]}]}}`

func TestSearchResultDownloadURLReachesSite(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)
	enabled := true
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("xingyunge"), models.SiteConfig{
		Enabled: &enabled, AuthMethod: "cookie", Cookie: "c_secure_pass=x",
	}))
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("mooko"), models.SiteConfig{
		Enabled: &enabled, AuthMethod: "api_key", APIKey: "k",
	}))

	nexus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/torrents.php":
			_, _ = w.Write([]byte(xingyungeSearchPage))
		case "/details.php":
			_, _ = w.Write([]byte(xingyungeDetailPage))
		case "/download.php":
			_, _ = w.Write([]byte(searchDownloadTorrent))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(nexus.Close)
	gazelle := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "browse":
			_, _ = w.Write([]byte(mookoBrowse))
		case "download":
			if r.URL.Query().Get("id") != "777" {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(searchDownloadTorrent))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(gazelle.Close)

	factory := v2.NewSiteFactory(nil)
	xyg, err := factory.CreateSite(v2.SiteConfig{
		Type: "nexusphp", ID: "xingyunge", Name: "XingYunGe", BaseURL: nexus.URL,
		Options: json.RawMessage(`{"cookie":"c_secure_pass=x"}`),
	})
	require.NoError(t, err)
	mooko, err := factory.CreateSite(v2.SiteConfig{
		Type: "gazelle", ID: "mooko", Name: "MooKo", BaseURL: gazelle.URL,
		Options: json.RawMessage(`{"apiKey":"k"}`),
	})
	require.NoError(t, err)
	withOrchestrator(t, xyg, mooko)

	body, _ := json.Marshal(MultiSiteSearchRequest{Keyword: "天气之子", Sites: []string{"xingyunge", "mooko"}, TimeoutSecs: 5})
	w := httptest.NewRecorder()
	srv.apiMultiSiteSearch(w, httptest.NewRequest(http.MethodPost, "/api/v2/search/multi", bytes.NewReader(body)))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp MultiSiteSearchResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Empty(t, resp.Errors)

	got := map[string]string{}
	for _, it := range resp.Items {
		got[it.SourceSite] = it.DownloadURL
		assert.Equal(t, "/api/site/"+it.SourceSite+"/torrent/"+it.ID+"/download", it.DownloadURL)

		// 推送：按同一个地址解析出站点与种子
		parsed, err := parseDownloadURL(it.DownloadURL)
		require.NoError(t, err, it.SourceSite)
		assert.Equal(t, it.SourceSite, parsed.SiteID)
		assert.Equal(t, it.ID, parsed.TorrentID)

		// 网页下载：和前端一样带上 title
		dl := httptest.NewRecorder()
		srv.apiSiteRouter(dl, httptest.NewRequest(http.MethodGet, it.DownloadURL+"?title="+url.QueryEscape(it.Title), nil))
		require.Equal(t, http.StatusOK, dl.Code, "%s: %s", it.SourceSite, dl.Body.String())
		assert.Equal(t, searchDownloadTorrent, dl.Body.String(), it.SourceSite)
		assert.Equal(t, "application/x-bittorrent", dl.Header().Get("Content-Type"))
	}
	assert.Equal(t, map[string]string{
		"xingyunge": "/api/site/xingyunge/torrent/98525/download",
		"mooko":     "/api/site/mooko/torrent/777/download",
	}, got)
}
