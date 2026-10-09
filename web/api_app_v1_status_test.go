package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
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
	var out AppDownloaderList
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Len(t, out.Items, 1)
	assert.False(t, out.Items[0].Reachable)
	assert.Contains(t, out.Items[0].Error, "没有读到")
}
