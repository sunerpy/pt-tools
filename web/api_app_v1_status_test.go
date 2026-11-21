package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/remote"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/version"
	"github.com/sunerpy/pt-tools/web/middleware"
)

// GET /downloaders：启用的下载器与传输状态（没有地址、账号与密码）；连不上的写明原因
func TestAppDownloaders(t *testing.T) {
	fake := &fakeDownloader{freSpace: 5 << 30}
	srv, id := setupServerWithFakeDownloader(t, fake)
	require.NoError(t, global.GlobalDB.DB.Create(&models.DownloaderSetting{Name: "gone", Type: "qbittorrent", URL: "http://admin:secret@127.0.0.1:9", Enabled: true}).Error)
	w := appAs(t, srv.appDownloaders, http.MethodGet, "/api/app/v1/downloaders")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "secret")
	var out AppDownloaderList
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Len(t, out.Items, 2)
	assert.Equal(t, id, out.Items[0].ID)
	assert.True(t, out.Items[0].Reachable)
	assert.EqualValues(t, 5<<30, out.Items[0].FreeSpace)
	assert.Equal(t, "gone", out.Items[1].Name)
	assert.False(t, out.Items[1].Reachable)
	assert.NotEmpty(t, out.Items[1].Error)

	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })
	assert.Equal(t, http.StatusServiceUnavailable, appAs(t, srv.appDownloaders, http.MethodGet, "/api/app/v1/downloaders").Code)
}

// GET /updates：include_prerelease=1 时也报预览版；查不了（又没有上次的结果）时回 502
func TestAppUpdates(t *testing.T) {
	srv := setupServer(t)
	var got version.CheckOptions
	srv.checkUpdates = func(_ context.Context, opts version.CheckOptions) (*version.VersionCheckResult, error) {
		got = opts
		return &version.VersionCheckResult{CurrentVersion: "v1", HasUpdate: true}, nil
	}
	w := appAs(t, srv.appUpdates, http.MethodGet, "/api/app/v1/updates?include_prerelease=1")
	require.Equal(t, http.StatusOK, w.Code)
	assert.True(t, got.IncludePrerelease)
	assert.Contains(t, w.Body.String(), `"has_update":true`)

	// 和真实的 Checker 一样：查不了时也给一个带 error 字段的结果，那不是检查的结论
	srv.checkUpdates = func(context.Context, version.CheckOptions) (*version.VersionCheckResult, error) {
		return &version.VersionCheckResult{CurrentVersion: "v1", Error: "连不上 GitHub"}, errors.New("连不上 GitHub")
	}
	assert.Equal(t, http.StatusBadGateway, appAs(t, srv.appUpdates, http.MethodGet, "/api/app/v1/updates").Code)
}

// 下载器的错误常带着它的内网地址：App 与 MCP 拿到的错误里去掉地址、端口与主机名，只留是什么错
func TestAppRedactAddr(t *testing.T) {
	cases := [][2]string{
		{
			`Get "http://admin:secret@10.0.0.5:8080/api/v2/app/version": dial tcp 10.0.0.5:8080: connect: connection refused`,
			`Get "<地址>": dial tcp <地址>: connect: connection refused`,
		},
		{`dial tcp: lookup qb.lan on 127.0.0.53:53: no such host`, `dial tcp: lookup <地址>: no such host`},
		{`lookup nas.local: no such host`, `lookup <地址>: no such host`},
		{`Post "https://[fd00::5]:9091/transmission/rpc": EOF`, `Post "<地址>": EOF`},
		{`推送种子失败: 连接 192.168.1.20:8080 超时`, `推送种子失败: 连接 <地址> 超时`},
		{`下载器拒绝`, `下载器拒绝`},
	}
	for _, c := range cases {
		assert.Equal(t, c[1], appRedactAddr(c[0]), c[0])
	}
}

// 下载器一直不回：请求结束时不再等，回已经知道的（这一台记成没有读到），不挂住
func TestAppDownloadersStopsWhenRequestEnds(t *testing.T) {
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	srv, _ := setupServerWithFakeDownloader(t, &fakeDownloader{statusBlock: block})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/app/v1/downloaders", nil).WithContext(ctx)
	req = req.WithContext(middleware.WithPrincipal(req.Context(), &middleware.Principal{Kind: middleware.KindAPIToken, Scopes: []string{"app:read"}}))
	w := httptest.NewRecorder()
	start := time.Now()
	srv.appDownloaders(w, req)
	assert.Less(t, time.Since(start), 3*time.Second)
	require.Equal(t, http.StatusOK, w.Code)
	validateAppResponse(t, req, w)
	var out AppDownloaderList
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Len(t, out.Items, 1)
	assert.False(t, out.Items[0].Reachable)
	assert.Contains(t, out.Items[0].Error, "没有读到")
}

// GET /downloaders/seeding：每台启用的下载器正在做种的个数与体积；连不上的照样列出，原因里没有地址、账号与密码，不计入合计
func TestAppDownloaderSeeding(t *testing.T) {
	fake := &fakeDownloader{torrents: []downloader.Torrent{
		{ID: "s1", State: downloader.TorrentSeeding, Progress: 1, IsCompleted: true, TotalSize: 300},
		{ID: "p1", State: downloader.TorrentPaused, Progress: 1, IsCompleted: true, TotalSize: 40},
		{ID: "d1", State: downloader.TorrentDownloading, Progress: 0.5, TotalSize: 1000},
	}}
	srv, id := setupServerWithFakeDownloader(t, fake)
	// 没注册进下载器管理器的那台取不到实例
	require.NoError(t, global.GlobalDB.DB.Create(&models.DownloaderSetting{Name: "gone", Type: "qbittorrent", URL: "http://admin:secret@127.0.0.1:9", Enabled: true}).Error)
	require.NoError(t, global.GlobalDB.DB.Create(&models.DownloaderSetting{Name: "off", Type: "qbittorrent", Enabled: false}).Error)

	w := appAs(t, srv.appDownloaderSeeding, http.MethodGet, "/api/app/v1/downloaders/seeding")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "secret")
	assert.NotContains(t, w.Body.String(), "127.0.0.1")
	assertNoSecrets(t, w.Body.Bytes())
	var out AppDownloaderSeedingList
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Len(t, out.Items, 2, "停用的不列")
	assert.Equal(t, AppDownloaderSeeding{
		ID: id, Name: "qb1", Type: "qbittorrent", Reachable: true,
		SeedingCount: 1, SeedingSize: 300, IdleCount: 1, IdleSize: 40,
	}, out.Items[0])
	assert.Equal(t, "gone", out.Items[1].Name)
	assert.False(t, out.Items[1].Reachable)
	assert.NotEmpty(t, out.Items[1].Error)
	assert.Equal(t, 1, out.TotalSeedingCount)
	assert.EqualValues(t, 300, out.TotalSeedingSize)

	// 下载器管理器没有初始化：每台都写明这一次没有读到
	srv.mgr = nil
	w = appAs(t, srv.appDownloaderSeeding, http.MethodGet, "/api/app/v1/downloaders/seeding")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Len(t, out.Items, 2)
	assert.False(t, out.Items[0].Reachable)
	assert.Contains(t, out.Items[0].Error, "没有读到")
	assert.Zero(t, out.TotalSeedingCount)

	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })
	assert.Equal(t, http.StatusServiceUnavailable, appAs(t, srv.appDownloaderSeeding, http.MethodGet, "/api/app/v1/downloaders/seeding").Code)
}

// 种子列表迟迟不回时不一直等：请求结束（或到了时限）就回，这一台写明没有在时限内读到
func TestAppDownloaderSeedingStopsWhenRequestEnds(t *testing.T) {
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	srv, _ := setupServerWithFakeDownloader(t, &fakeDownloader{listBlock: block})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/app/v1/downloaders/seeding", nil).WithContext(ctx)
	req = req.WithContext(middleware.WithPrincipal(req.Context(), &middleware.Principal{Kind: middleware.KindAPIToken, Scopes: []string{"app:read"}}))
	w := httptest.NewRecorder()
	start := time.Now()
	srv.appDownloaderSeeding(w, req)
	assert.Less(t, time.Since(start), 3*time.Second)
	require.Equal(t, http.StatusOK, w.Code)
	validateAppResponse(t, req, w)
	var out AppDownloaderSeedingList
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Len(t, out.Items, 1)
	assert.False(t, out.Items[0].Reachable)
	assert.Contains(t, out.Items[0].Error, "没有在时限内读到种子列表")
}

// 走真实路由：只要 app:read；/meta 的 features 里有 downloader_seeding
func TestAppDownloaderSeedingRoute(t *testing.T) {
	e := newAppEnv(t)
	w := e.do(appReq{method: http.MethodGet, path: appPrefix + "/downloaders/seeding", bearer: e.token(apitoken.ScopeAppRead)})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = e.do(appReq{method: http.MethodGet, path: appPrefix + "/downloaders/seeding"})
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	w = e.do(appReq{method: http.MethodGet, path: appPrefix + "/meta", bearer: e.token(apitoken.ScopeAppRead)})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var meta AppMeta
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &meta))
	assert.Contains(t, meta.Features, "downloader_seeding")
}

// addFakeDownloader 在 srv 的下载器管理器里注册一台假下载器（类型 typ），并在库里记一条启用的设置，返回它的编号。
func addFakeDownloader(t *testing.T, srv *Server, name string, typ downloader.DownloaderType, fake *fakeDownloader) uint {
	t.Helper()
	dm := srv.mgr.GetDownloaderManager()
	dm.RegisterFactory(typ, func(_ downloader.DownloaderConfig, n string) (downloader.Downloader, error) {
		fake.name, fake.dlType = n, typ
		return fake, nil
	})
	require.NoError(t, dm.RegisterConfig(name, downloader.NewGenericConfig(typ, "http://localhost:8080", "u", "p", true), false))
	ds := models.DownloaderSetting{Name: name, Type: string(typ), Enabled: true}
	require.NoError(t, global.GlobalDB.DB.Create(&ds).Error)
	return ds.ID
}

func labelsPath(id uint) string {
	return appPrefix + "/downloaders/" + strconv.FormatUint(uint64(id), 10) + "/labels"
}

// GET /downloaders/{id}/labels：qBittorrent 建好的分类加上种子上用到的分类，种子上的标签（qB 用「, 」隔开）；
// 去掉空的与重复的，排好序。读不到建好的分类时只列种子上用到的，不算出错
func TestAppDownloaderLabels(t *testing.T) {
	fake := &fakeDownloader{
		labels: []string{"movie", "anime", " "},
		torrents: []downloader.Torrent{
			{ID: "t1", Category: "movie", Tags: "hd, foreign"},
			{ID: "t2", Category: "tv", Tags: "hd,,sd"},
			{ID: "t3"},
		},
	}
	srv, id := setupServerWithFakeDownloader(t, fake)
	idText := strconv.FormatUint(uint64(id), 10)
	w := appWithID(t, srv.appDownloaderLabels, http.MethodGet, labelsPath(id), idText, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	var out AppDownloaderLabels
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Equal(t, AppDownloaderLabels{
		DownloaderID: id, Downloader: "qb1", Type: "qbittorrent", SupportsCategory: true,
		Categories: []string{"anime", "movie", "tv"}, Tags: []string{"foreign", "hd", "sd"},
	}, out)
	assert.Equal(t, 1, fake.labelsCalls)

	// 读不到建好的分类：照样回种子上用到的，原因记进日志（AGENTS.md：错误要返回或记日志）
	core, logs := observer.New(zapcore.WarnLevel)
	saved := global.GlobalLogger
	global.GlobalLogger = zap.New(core)
	t.Cleanup(func() { global.GlobalLogger = saved })
	fake.labelsErr = errors.New("categories request failed")
	w = appWithID(t, srv.appDownloaderLabels, http.MethodGet, labelsPath(id), idText, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Equal(t, []string{"movie", "tv"}, out.Categories)
	warned := logs.FilterMessageSnippet("categories request failed").All()
	require.Len(t, warned, 1)
	assert.Contains(t, warned[0].Message, "qb1")

	// 下载器里一个种子也没有：两个列表是空数组，不是 null
	fake.torrents, fake.labelsErr, fake.labels = nil, nil, nil
	w = appWithID(t, srv.appDownloaderLabels, http.MethodGet, labelsPath(id), idText, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"categories":[]`)
	assert.Contains(t, w.Body.String(), `"tags":[]`)
}

// Transmission 没有分类（推送时给的分类会写成第一个 label）：supports_category 为假、categories 是空的，
// 也不去读建好的分类；种子上的 labels 都是标签
func TestAppDownloaderLabelsTransmission(t *testing.T) {
	srv, _ := setupTestServer(t)
	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)
	srv.mgr = mgr
	fake := &fakeDownloader{labels: []string{"not-a-category"}, torrents: []downloader.Torrent{
		{ID: "1", Category: "movie", Tags: "movie,hd"},
		{ID: "2", Category: "pt-tools-keep", Tags: "pt-tools-keep"},
	}}
	id := addFakeDownloader(t, srv, "tr1", downloader.DownloaderTransmission, fake)
	w := appWithID(t, srv.appDownloaderLabels, http.MethodGet, labelsPath(id), strconv.FormatUint(uint64(id), 10), "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	var out AppDownloaderLabels
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Equal(t, AppDownloaderLabels{
		DownloaderID: id, Downloader: "tr1", Type: "transmission", SupportsCategory: false,
		Categories: []string{}, Tags: []string{"hd", "movie", "pt-tools-keep"},
	}, out)
	assert.Zero(t, fake.labelsCalls)
}

// 编号不对 400；没有这台或停用了 404；取不到实例、读不到种子列表 502（原因去掉地址）；下载器管理器或数据库没有初始化 503
func TestAppDownloaderLabelsErrors(t *testing.T) {
	fake := &fakeDownloader{}
	srv, id := setupServerWithFakeDownloader(t, fake)
	call := func(idText string) *httptest.ResponseRecorder {
		t.Helper()
		return appWithID(t, srv.appDownloaderLabels, http.MethodGet, appPrefix+"/downloaders/"+idText+"/labels", idText, "")
	}
	for _, bad := range []string{"x", "0", "-1"} {
		w := call(bad)
		assert.Equal(t, http.StatusBadRequest, w.Code, bad)
		assert.Equal(t, "invalid_argument", errorCode(t, w), bad)
	}
	off := models.DownloaderSetting{Name: "off", Type: "qbittorrent", Enabled: false}
	require.NoError(t, global.GlobalDB.DB.Create(&off).Error)
	// 没注册进下载器管理器的那台取不到实例
	gone := models.DownloaderSetting{Name: "gone", Type: "qbittorrent", URL: "http://admin:secret@127.0.0.1:9", Enabled: true}
	require.NoError(t, global.GlobalDB.DB.Create(&gone).Error)
	for _, missing := range []uint{999, off.ID} {
		w := call(strconv.FormatUint(uint64(missing), 10))
		assert.Equal(t, http.StatusNotFound, w.Code, missing)
		assert.Equal(t, "not_found", errorCode(t, w))
	}
	w := call(strconv.FormatUint(uint64(gone.ID), 10))
	assert.Equal(t, http.StatusBadGateway, w.Code, w.Body.String())
	assert.Equal(t, "upstream", errorCode(t, w))
	assert.NotContains(t, w.Body.String(), "secret")

	fake.listErr = errors.New(`获取失败: Get "http://qb.lan:8080/api/v2/torrents/info?token=abc": EOF`)
	w = call(strconv.FormatUint(uint64(id), 10))
	assert.Equal(t, http.StatusBadGateway, w.Code, w.Body.String())
	assert.Equal(t, "upstream", errorCode(t, w))
	assert.Contains(t, w.Body.String(), "qb1")
	assert.NotContains(t, w.Body.String(), "token=abc")
	assert.NotContains(t, w.Body.String(), "qb.lan")

	srv.mgr = nil
	assert.Equal(t, http.StatusServiceUnavailable, call(strconv.FormatUint(uint64(id), 10)).Code)

	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })
	assert.Equal(t, http.StatusServiceUnavailable, call(strconv.FormatUint(uint64(id), 10)).Code)
}

// 种子列表迟迟不回：请求结束（或到了时限）就回 504，不挂住
func TestAppDownloaderLabelsStopsWhenRequestEnds(t *testing.T) {
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	srv, id := setupServerWithFakeDownloader(t, &fakeDownloader{listBlock: block})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, labelsPath(id), nil).WithContext(ctx)
	req.SetPathValue("id", strconv.FormatUint(uint64(id), 10))
	req = req.WithContext(middleware.WithPrincipal(req.Context(), &middleware.Principal{Kind: middleware.KindAPIToken, Scopes: []string{"app:read"}}))
	w := httptest.NewRecorder()
	start := time.Now()
	srv.appDownloaderLabels(w, req)
	assert.Less(t, time.Since(start), 3*time.Second)
	require.Equal(t, http.StatusGatewayTimeout, w.Code, w.Body.String())
	assert.Equal(t, "timeout", errorCode(t, w))
	assert.Contains(t, w.Body.String(), "qb1")
	validateAppResponse(t, req, w)
}

// 走真实路由与远程访问的隧道：只要 app:read，只读的设备也能读；/meta 的 features 里有 downloader_labels
func TestAppDownloaderLabelsRouteAndTunnel(t *testing.T) {
	e := newRemoteEnv(t)
	id := addFakeDownloader(t, e.srv, "qb-tunnel", downloader.DownloaderQBittorrent, &fakeDownloader{torrents: sampleTorrents()})

	w := e.do(appReq{method: http.MethodGet, path: labelsPath(id), bearer: e.token(apitoken.ScopeAppRead)})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, http.StatusUnauthorized, e.do(appReq{method: http.MethodGet, path: labelsPath(id)}).Code)
	assert.Equal(t, http.StatusMethodNotAllowed, e.do(appReq{method: http.MethodPost, path: labelsPath(id), bearer: e.token(apitoken.ScopeAppRead)}).Code)

	_, c := e.device(remote.ScopesRead)
	status, body := tunnel(t, c, http.MethodGet, labelsPath(id), "", nil)
	require.Equal(t, http.StatusOK, status, body)
	var out AppDownloaderLabels
	require.NoError(t, json.Unmarshal([]byte(body), &out))
	assert.Equal(t, "qb-tunnel", out.Downloader)
	assert.Equal(t, []string{"movie", "tv"}, out.Categories)
	assert.Equal(t, []string{"foreign", "hd", "sd"}, out.Tags)

	w = e.do(appReq{method: http.MethodGet, path: appPrefix + "/meta", bearer: e.token(apitoken.ScopeAppRead)})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var meta AppMeta
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &meta))
	assert.Contains(t, meta.Features, "downloader_labels")
}
