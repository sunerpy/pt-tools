package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/mcp"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// bearerTransport 给每个请求带上令牌。
type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

// mcpSession 用 go-sdk 的客户端经 streamable HTTP 连上 /mcp。
func mcpSession(t *testing.T, base, token string) (*sdk.ClientSession, error) {
	t.Helper()
	tr := &sdk.StreamableClientTransport{
		Endpoint: base + mcpPath, HTTPClient: &http.Client{Transport: bearerTransport{token}}, MaxRetries: -1, DisableStandaloneSSE: true,
	}
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(context.Background(), tr, nil)
	if err == nil {
		t.Cleanup(func() { _ = cs.Close() })
	}
	return cs, err
}

// initialize 是一次 MCP 的 initialize 请求（看 HTTP 层的状态码）。
func mcpInitialize(t *testing.T, e *appEnv, bearer string, session bool) int {
	t.Helper()
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`
	req := httptest.NewRequest(http.MethodPost, mcpPath, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if session {
		req.AddCookie(&http.Cookie{Name: "session", Value: "sess-app"})
	}
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)
	return w.Code
}

// /mcp 只认 API 令牌：没有令牌、令牌不对、只有网页登录都回 401；没有 mcp 权限的令牌 403；有 mcp:read 或 mcp:write 的能连上
func TestMCPHTTPAuth(t *testing.T) {
	e := newAppEnv(t)
	assert.Equal(t, http.StatusUnauthorized, mcpInitialize(t, e, "", false))
	assert.Equal(t, http.StatusUnauthorized, mcpInitialize(t, e, "ptt_1_wrong", false))
	assert.Equal(t, http.StatusUnauthorized, mcpInitialize(t, e, "", true), "网页登录的 session 不收")
	assert.Equal(t, http.StatusForbidden, mcpInitialize(t, e, e.token(apitoken.ScopeAppRead, apitoken.ScopeAppWrite), false))
	assert.Equal(t, http.StatusOK, mcpInitialize(t, e, e.token(apitoken.ScopeMCPWrite), false))

	ts := httptest.NewServer(e.handler)
	t.Cleanup(ts.Close)
	cs, err := mcpSession(t, ts.URL, e.token(apitoken.ScopeMCPRead))
	require.NoError(t, err)
	res, err := cs.ListTools(context.Background(), nil)
	require.NoError(t, err)
	assert.Len(t, res.Tools, len(mcp.ToolNames))
	assert.Equal(t, "pt-tools", cs.InitializeResult().ServerInfo.Name)
}

// 经真实的 App API 调工具：只读工具回 App API 的回应；写工具记审计（通道 mcp、命令是工具名）；只有 mcp:read 的令牌调写工具被拒
func TestMCPToolsEndToEnd(t *testing.T) {
	e := newAppEnv(t)
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}, &models.DownloaderSetting{}))
	require.NoError(t, db.Create(&models.TorrentInfo{SiteName: "hdsky", TorrentID: "1", Title: "Dune.Part.Two"}).Error)
	ts := httptest.NewServer(e.handler)
	t.Cleanup(ts.Close)
	cs, err := mcpSession(t, ts.URL, e.token(apitoken.ScopeMCPRead, apitoken.ScopeMCPWrite))
	require.NoError(t, err)
	call := func(cs *sdk.ClientSession, name string, args map[string]any) (*sdk.CallToolResult, map[string]any) {
		t.Helper()
		res, cerr := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
		require.NoError(t, cerr)
		out := map[string]any{}
		if res.StructuredContent != nil {
			b, _ := json.Marshal(res.StructuredContent)
			require.NoError(t, json.Unmarshal(b, &out))
		}
		return res, out
	}

	res, out := call(cs, "list_tasks", map[string]any{"site": "hdsky"})
	require.False(t, res.IsError)
	items := out["items"].([]any)
	require.Len(t, items, 1)
	assert.Equal(t, "Dune.Part.Two", items[0].(map[string]any)["title"])

	res, out = call(cs, "check_updates", nil)
	require.False(t, res.IsError)
	assert.Equal(t, "v1.0.0-test", out["current_version"])

	res, out = call(cs, "get_downloader_stats", nil)
	require.False(t, res.IsError)
	assert.Empty(t, out["items"])

	res, _ = call(cs, "pause_torrent", map[string]any{"downloader_id": 99, "task_id": "abc", "confirm": true})
	require.False(t, res.IsError, "App API 回了批量结果")
	a := e.audit.all()
	require.NotEmpty(t, a)
	last := a[len(a)-1]
	assert.Equal(t, mcp.ChannelType, last.ChannelType)
	assert.Equal(t, "pause_torrent", last.Command)
	assert.NotEqual(t, "success", last.Result, "下载器不存在：批量动作全失败")

	ro, err := mcpSession(t, ts.URL, e.token(apitoken.ScopeMCPRead))
	require.NoError(t, err)
	res, _ = call(ro, "pause_torrent", map[string]any{"downloader_id": 99, "task_id": "abc", "confirm": true})
	assert.True(t, res.IsError)
	a = e.audit.all()
	assert.Equal(t, "denied:scope", a[len(a)-1].Result)
}

// 进程内调用：主体的种类是 mcp，mcp:read 当 app:read、mcp:write 当 app:write；没有对应权限的接口回 403；没有的接口 404
func TestMCPBackendScopes(t *testing.T) {
	e := newAppEnv(t)
	b := &mcpBackend{routes: e.srv.mcpRoutes()}
	ctx := context.Background()
	read := mcp.Caller{TokenID: 1, Scopes: []string{apitoken.ScopeMCPRead}}
	resp, err := b.Call(ctx, read, mcp.Request{Method: http.MethodGet, Path: "/meta"})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.Status, string(resp.Body))
	var meta AppMeta
	require.NoError(t, json.Unmarshal(resp.Body, &meta))
	assert.Equal(t, "mcp", meta.Principal.Kind)
	assert.Equal(t, []string{apitoken.ScopeAppRead}, meta.Principal.Scopes)

	resp, err = b.Call(ctx, read, mcp.Request{Method: http.MethodPost, Path: "/torrents/actions", Body: map[string]any{"action": "pause"}})
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, resp.Status, "没有 mcp:write")
	resp, err = b.Call(ctx, read, mcp.Request{Method: http.MethodGet, Path: "/nope"})
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.Status)
	_, err = b.Call(ctx, read, mcp.Request{Method: http.MethodPost, Path: "/push", Body: func() {}})
	require.Error(t, err, "请求体编码不了")
}

// 经 MCP 推送的种子记录来源是 mcp_push（和 App 的 app_push 分开）
func TestMCPPushSource(t *testing.T) {
	srv, _ := setupAppSearch(t)
	dl := &fakeDownloader{addResult: downloader.AddTorrentResult{Success: true, Hash: "abc"}, freSpace: 1 << 50}
	t.Cleanup(internal.SwapPushDownloaderFactory(func(models.DownloaderSetting) (downloader.Downloader, error) { return dl, nil }))
	internal.GetDiskBudget().Reset()
	t.Cleanup(func() { internal.GetDiskBudget().Reset() })
	require.NoError(t, global.GlobalDB.DB.Create(&models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true, IsDefault: true}).Error)

	b := &mcpBackend{routes: srv.mcpRoutes()}
	resp, err := b.Call(context.Background(), mcp.Caller{TokenID: 3, Scopes: []string{apitoken.ScopeMCPWrite}},
		mcp.Request{Method: http.MethodPost, Path: "/push", Body: map[string]any{"site": "hdsky", "torrent_id": "5"}})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.Status, string(resp.Body))
	var info models.TorrentInfo
	require.NoError(t, global.GlobalDB.DB.Where("site_name = ? AND torrent_id = ?", "hdsky", "5").First(&info).Error)
	assert.Equal(t, mcpPushSource, info.DownloadSource)
}
