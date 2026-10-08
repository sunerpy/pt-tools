package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func appTorrentBytes(name string) []byte {
	info := fmt.Sprintf("d6:lengthi%de4:name%d:%s12:piece lengthi16384e6:pieces20:%se", int64(1)<<30, len(name), name, strings.Repeat("x", 20))
	return []byte(fmt.Sprintf("d8:announce23:http://tracker/announce4:info%se", info))
}

// pushSite 是能搜、能下载种子文件的假站点；hashes 记下按 downhash 下载时用的值。
type pushSite struct {
	mockSearchSiteForAPI
	hashes []string
}

func (p *pushSite) Download(_ context.Context, id string) ([]byte, error) {
	return appTorrentBytes(p.id + "-" + id), nil
}

func (p *pushSite) DownloadWithHash(_ context.Context, id, hash string) ([]byte, error) {
	p.hashes = append(p.hashes, hash)
	return appTorrentBytes(p.id + "-" + id), nil
}

func setupAppSearch(t *testing.T) (*Server, *pushSite) {
	t.Helper()
	srv, _ := historyFixture(t)
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.TorrentInfo{}, &models.DownloaderSetting{}))
	site := &pushSite{mockSearchSiteForAPI: mockSearchSiteForAPI{id: "hdsky", name: "HDSky", kind: v2.SiteNexusPHP, items: []v2.TorrentItem{
		{
			ID: "1", Title: "Dune.Part.Two.2024.1080p", SizeBytes: 8 << 30, Seeders: 30, SourceSite: "hdsky", DiscountLevel: v2.DiscountFree,
			URL: "https://hdsky.me/details.php?id=1", DownloadURL: "/api/site/hdsky/torrent/1/download?downhash=dh-secret",
			Magnet: "magnet:?xt=urn:btih:abc&tr=https://hdsky.me/announce?passkey=pk", DiscountEndTime: time.Now().Add(time.Hour),
		},
	}}}
	off := &pushSite{mockSearchSiteForAPI: mockSearchSiteForAPI{id: "disabledsite", name: "Off", kind: v2.SiteNexusPHP, items: []v2.TorrentItem{{ID: "9", Title: "Off", SourceSite: "disabledsite"}}}}
	orch := v2.NewSearchOrchestrator(v2.SearchOrchestratorConfig{})
	orch.RegisterSite(site)
	orch.RegisterSite(off)
	prev := searchOrchestrator
	InitSearchOrchestrator(v2.NewCachedSearchOrchestrator(orch, v2.SearchCacheConfig{TTL: time.Minute}))
	t.Cleanup(func() { InitSearchOrchestrator(prev) })
	return srv, site
}

// 搜索：只回站点与种子编号，没有详情、下载、磁力链接；只搜启用的站点；指定的站点都没启用时不搜
func TestAppSearch(t *testing.T) {
	srv, _ := setupAppSearch(t)
	w := appAsWith(srv.appSearch, http.MethodPost, "/api/app/v1/search", `{"keyword":"Dune"}`, "app:read")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	assert.NotContains(t, w.Body.String(), "dh-secret")
	assert.NotContains(t, w.Body.String(), "magnet")
	var res AppSearchResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	require.Len(t, res.Items, 1, "没启用的站点不搜")
	it := res.Items[0]
	assert.Equal(t, "hdsky", it.Site)
	assert.Equal(t, "1", it.TorrentID)
	assert.True(t, it.Free)
	assert.NotZero(t, it.DiscountEndAt)
	assert.Equal(t, "dh-secret", lookupDownhash("hdsky", "1"), "下载要的 downhash 留在服务端")

	w = appAsWith(srv.appSearch, http.MethodPost, "/api/app/v1/search", `{"keyword":"Off","sites":["disabledsite"]}`, "app:read")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Empty(t, res.Items, "指定的站点都没启用：不搜，也不退回到搜所有站点")

	for _, bad := range []string{`{"keyword":""}`, `{"keyword":"` + strings.Repeat("长", 101) + `"}`, `{"keyword":"x","min_seeders":-1}`, `{"keyword":"x","url":"y"}`} {
		assert.Equal(t, http.StatusBadRequest, appAsWith(srv.appSearch, http.MethodPost, "/api/app/v1/search", bad, "app:read").Code, bad)
	}
}

// 推送：服务端下载种子文件再经推送闸门；用默认下载器；按 downhash 下载；站点没启用 404、下载器不对 400
func TestAppPush(t *testing.T) {
	srv, site := setupAppSearch(t)
	dl := &fakeDownloader{addResult: downloader.AddTorrentResult{Success: true, Hash: "abc"}, freSpace: 1 << 50}
	t.Cleanup(internal.SwapPushDownloaderFactory(func(models.DownloaderSetting) (downloader.Downloader, error) { return dl, nil }))
	internal.GetDiskBudget().Reset()
	t.Cleanup(func() { internal.GetDiskBudget().Reset() })
	off := models.DownloaderSetting{Name: "off", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: false}
	require.NoError(t, global.GlobalDB.DB.Create(&off).Error)
	ds := models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true, IsDefault: true}
	require.NoError(t, global.GlobalDB.DB.Create(&ds).Error)

	w := appAsWith(srv.appPush, http.MethodPost, "/api/app/v1/push", `{"site":"hdsky","torrent_id":"2","title":"Dune 2","category":"movies"}`, "app:write")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var res AppPushResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.True(t, res.Success, res.Message)
	assert.Equal(t, ds.ID, res.DownloaderID, "没指定时用默认下载器")
	var info models.TorrentInfo
	require.NoError(t, global.GlobalDB.DB.Where("site_name = ? AND torrent_id = ?", "hdsky", "2").First(&info).Error)
	assert.Equal(t, appPushSource, info.DownloadSource)
	assert.Equal(t, "movies", info.Category)

	// 搜索时记下了 downhash：推送按它下载
	appAsWith(srv.appSearch, http.MethodPost, "/api/app/v1/search", `{"keyword":"Dune"}`, "app:read")
	w = appAsWith(srv.appPush, http.MethodPost, "/api/app/v1/push", `{"site":"hdsky","torrent_id":"1"}`, "app:write")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, []string{"dh-secret"}, site.hashes)
	// 没带 title：记录里的标题取种子文件里的名字，任务列表不会出现没有标题的行
	var untitled models.TorrentInfo
	require.NoError(t, global.GlobalDB.DB.Where("site_name = ? AND torrent_id = ?", "hdsky", "1").First(&untitled).Error)
	assert.Equal(t, "hdsky-1", untitled.Title)

	for _, c := range []struct {
		body string
		want int
	}{
		{`{"site":"disabledsite","torrent_id":"9"}`, http.StatusNotFound},
		{`{"site":"hdsky","torrent_id":"1","downloader_id":999}`, http.StatusBadRequest},
		{`{"site":"hdsky","torrent_id":"1","downloader_id":` + fmt.Sprint(off.ID) + `}`, http.StatusBadRequest},
		{`{"site":"","torrent_id":"1"}`, http.StatusBadRequest},
		{`{"site":"hdsky","torrent_id":"1","download_url":"http://evil/x.torrent"}`, http.StatusBadRequest},
	} {
		assert.Equal(t, c.want, appAsWith(srv.appPush, http.MethodPost, "/api/app/v1/push", c.body, "app:write").Code, c.body)
	}

	dl.addResult = downloader.AddTorrentResult{Success: false, Message: "下载器拒绝"}
	w = appAsWith(srv.appPush, http.MethodPost, "/api/app/v1/push", `{"site":"hdsky","torrent_id":"3"}`, "app:write")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.False(t, res.Success)
	assert.Equal(t, "下载器拒绝", res.Message)
}

// 签到：站点不存在 404；签到服务没启动 503
func TestAppAttend(t *testing.T) {
	srv, _ := setupAppSearch(t)
	call := func(site string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/app/v1/sites/"+site+"/attend", nil)
		req.SetPathValue("site", site)
		w := httptest.NewRecorder()
		srv.appAttend(w, req)
		return w.Code
	}
	assert.Equal(t, http.StatusNotFound, call("nosuchsite"))
	assert.Equal(t, http.StatusServiceUnavailable, call("hdsky"), "测试里没有签到服务")
}
