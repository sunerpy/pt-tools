package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type echoIn struct {
	Text string `json:"text"`
}

// remoteMCP 起一个要令牌的 MCP 服务（streamable HTTP），带一个 echo 工具与一个总是报错的工具。
func remoteMCP(t *testing.T, token string) *httptest.Server {
	t.Helper()
	srv := sdk.NewServer(&sdk.Implementation{Name: "pt-tools", Version: "v-test"}, &sdk.ServerOptions{Instructions: "说明"})
	sdk.AddTool(srv, &sdk.Tool{Name: "echo", Description: "echo"}, func(_ context.Context, _ *sdk.CallToolRequest, in echoIn) (*sdk.CallToolResult, any, error) {
		return nil, map[string]any{"text": in.Text}, nil
	})
	sdk.AddTool(srv, &sdk.Tool{Name: "fail", Description: "fail"}, func(context.Context, *sdk.CallToolRequest, echoIn) (*sdk.CallToolResult, any, error) {
		return nil, nil, assert.AnError
	})
	h := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return srv }, &sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)
	return ts
}

// 桥：带着令牌连上远端，把远端的工具与说明原样挂到本地；调用（成功的、工具报错的）原样转发；客户端断开以后退出
func TestMCPBridge(t *testing.T) {
	ts := remoteMCP(t, "ptt_1_secret")
	ct, st := sdk.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runMCPBridge(ctx, ts.URL+"/mcp", "ptt_1_secret", st) }()

	cs, err := sdk.NewClient(&sdk.Implementation{Name: "claude", Version: "1"}, nil).Connect(ctx, ct, nil)
	require.NoError(t, err)
	assert.Equal(t, "说明", cs.InitializeResult().Instructions)
	assert.Equal(t, "pt-tools", cs.InitializeResult().ServerInfo.Name)
	tools, err := cs.ListTools(ctx, nil)
	require.NoError(t, err)
	require.Len(t, tools.Tools, 2)

	res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "echo", Arguments: map[string]any{"text": "你好"}})
	require.NoError(t, err)
	assert.False(t, res.IsError)
	assert.Contains(t, res.Content[0].(*sdk.TextContent).Text, "你好")
	res, err = cs.CallTool(ctx, &sdk.CallToolParams{Name: "fail", Arguments: map[string]any{"text": "x"}})
	require.NoError(t, err)
	assert.True(t, res.IsError, "工具报错原样转发")

	require.NoError(t, cs.Close())
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("客户端断开以后桥没有退出")
	}
}

// 令牌不对：连不上，提示检查地址与令牌
func TestMCPBridgeBadToken(t *testing.T) {
	ts := remoteMCP(t, "ptt_1_secret")
	_, st := sdk.NewInMemoryTransports()
	err := runMCPBridge(context.Background(), ts.URL+"/mcp", "ptt_1_wrong", st)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "令牌")
}

// 地址：只给主机与端口时补 /mcp；带路径时原样；不是 http(s) 的拒绝
func TestMCPEndpoint(t *testing.T) {
	for in, want := range map[string]string{
		"http://127.0.0.1:8080":          "http://127.0.0.1:8080/mcp",
		"https://pt.example.com/":        "https://pt.example.com/mcp",
		" https://pt.example.com/x/mcp ": "https://pt.example.com/x/mcp",
	} {
		got, err := mcpEndpoint(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got)
	}
	for _, bad := range []string{"127.0.0.1:8080", "ftp://x", "http://", "::"} {
		_, err := mcpEndpoint(bad)
		assert.Error(t, err, bad)
	}
}

// 命令：没给令牌时报错并写明去哪里建；地址不对时报错
func TestMCPCommandArgs(t *testing.T) {
	prevURL, prevToken, prevDiscard := mcpBridgeURL, mcpBridgeToken, mcpDiscardKey
	t.Cleanup(func() { mcpBridgeURL, mcpBridgeToken, mcpDiscardKey = prevURL, prevToken, prevDiscard })
	discarded := 0
	mcpDiscardKey = func() error { discarded++; return nil }
	t.Setenv("PT_TOOLS_MCP_URL", "")
	t.Setenv("PT_TOOLS_MCP_TOKEN", "")
	mcpBridgeURL, mcpBridgeToken = "", ""
	err := mcpCmd.RunE(mcpCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PT_TOOLS_MCP_TOKEN")
	mcpBridgeURL = "not a url"
	err = mcpCmd.RunE(mcpCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "http://")
	assert.Equal(t, "b", firstNonEmpty(" ", "b", "c"))
	assert.Equal(t, 2, discarded, "桥启动时先删掉本进程生成的密钥文件")
}
