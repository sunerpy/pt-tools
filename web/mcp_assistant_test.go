package web

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/mcp"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/web/middleware"
)

// 内置 AI 助手经 MCP 工具调 App API 时，主体是 assistant（ID 是会话属主），权限范围按 mcp:* 换成 app:*
func TestMCPBackendPrincipalForAssistant(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /echo", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(middleware.PrincipalFrom(r.Context()))
	})
	b := &mcpBackend{routes: mux}
	decode := func(c mcp.Caller) middleware.Principal {
		resp, err := b.Call(context.Background(), c, mcp.Request{Method: http.MethodGet, Path: "/echo"})
		require.NoError(t, err)
		var p middleware.Principal
		require.NoError(t, json.Unmarshal(resp.Body, &p))
		return p
	}
	p := decode(mcp.Caller{Kind: mcp.KindAssistant, Subject: "web:admin", Name: "AI 助手", Scopes: []string{apitoken.ScopeMCPRead}})
	assert.Equal(t, middleware.KindAssistant, p.Kind)
	assert.Equal(t, "web:admin", p.ID)
	assert.Equal(t, []string{apitoken.ScopeAppRead}, p.Scopes)

	p = decode(mcp.Caller{TokenID: 7, Name: "claude", Scopes: []string{apitoken.ScopeMCPRead, apitoken.ScopeMCPWrite}})
	assert.Equal(t, middleware.KindMCP, p.Kind)
	assert.Equal(t, "7", p.ID)
	assert.Equal(t, []string{apitoken.ScopeAppRead, apitoken.ScopeAppWrite}, p.Scopes)
}

// 内置 AI 助手推送的种子记录来源是 assistant_push（和 MCP、App 分开）
func TestAssistantPushSource(t *testing.T) {
	srv, _ := setupAppSearch(t)
	dl := &fakeDownloader{addResult: downloader.AddTorrentResult{Success: true, Hash: "abc"}, freSpace: 1 << 50}
	t.Cleanup(internal.SwapPushDownloaderFactory(func(models.DownloaderSetting) (downloader.Downloader, error) { return dl, nil }))
	internal.GetDiskBudget().Reset()
	t.Cleanup(func() { internal.GetDiskBudget().Reset() })
	require.NoError(t, global.GlobalDB.DB.Create(&models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true, IsDefault: true}).Error)

	b := &mcpBackend{routes: srv.mcpRoutes()}
	resp, err := b.Call(context.Background(), mcp.Caller{Kind: mcp.KindAssistant, Subject: "web:admin", Scopes: []string{apitoken.ScopeMCPWrite}},
		mcp.Request{Method: http.MethodPost, Path: "/push", Body: map[string]any{"site": "hdsky", "torrent_id": "6"}})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.Status, string(resp.Body))
	var info models.TorrentInfo
	require.NoError(t, global.GlobalDB.DB.Where("site_name = ? AND torrent_id = ?", "hdsky", "6").First(&info).Error)
	assert.Equal(t, assistantPushSource, info.DownloadSource)
}
