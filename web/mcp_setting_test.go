package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/models"
)

// 第一次读设置时建一行：已经有带 MCP 权限的令牌（升级上来）就开着，没有（新装）就关着；之后照存的
func TestLoadMCPSettingDefaults(t *testing.T) {
	setupServer(t).mgr.StopAll()
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(&models.MCPSetting{}, &models.APIToken{}))
	ctx := context.Background()

	row, err := loadMCPSetting(ctx, db)
	require.NoError(t, err)
	assert.False(t, row.Enabled, "新装：关着")
	assert.Equal(t, "/mcp", row.Path)

	// 存过以后照存的（有了令牌也不自己打开）
	require.NoError(t, db.Create(&models.APIToken{Name: "agent", Scopes: "mcp:read mcp:write"}).Error)
	row, err = loadMCPSetting(ctx, db)
	require.NoError(t, err)
	assert.False(t, row.Enabled)

	// 升级上来（还没有这一行）而且有带 MCP 权限的令牌：开着
	require.NoError(t, db.Where("1 = 1").Delete(&models.MCPSetting{}).Error)
	row, err = loadMCPSetting(ctx, db)
	require.NoError(t, err)
	assert.True(t, row.Enabled)
	assert.Equal(t, "/mcp", row.Path)
}

// 开关与路径：关着时 /mcp 回 404 写明原因（不是网页的登录跳转）；打开、换路径马上生效，换了以后旧的 /mcp 回 404 不说新地址；
// 设置接口只认网页登录，路径不合规矩、撞上已有的地址回 400
func TestMCPSettingsSwitchAndPath(t *testing.T) {
	e := newAppEnv(t)
	token := e.token(apitoken.ScopeMCPRead)
	initialize := func(path string) *httptest.ResponseRecorder {
		body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		e.handler.ServeHTTP(w, req)
		return w
	}
	put := func(body string) *httptest.ResponseRecorder {
		return e.do(appReq{method: http.MethodPut, path: "/api/mcp/settings", body: body, session: true})
	}
	require.Equal(t, http.StatusOK, initialize("/mcp").Code, "测试环境里开着")

	// 设置要网页登录：令牌不行
	assert.NotEqual(t, http.StatusOK, e.do(appReq{method: http.MethodGet, path: "/api/mcp/settings", bearer: token}).Code)
	w := e.do(appReq{method: http.MethodGet, path: "/api/mcp/settings", session: true})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var v MCPSettingsView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &v))
	assert.Equal(t, MCPSettingsView{Enabled: true, Path: "/mcp", DefaultPath: "/mcp", Tokens: 1, UpdatedAt: v.UpdatedAt}, v)

	// 关掉：/mcp 回 404 写明原因
	w = put(`{"enabled":false,"path":"/mcp"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = initialize("/mcp")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "MCP 接入没有打开")

	// 打开并换路径（末尾的 / 去掉）：新地址能用，旧的 /mcp 回 404、不说新地址
	w = put(`{"enabled":true,"path":"agent/mcp/"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &v))
	assert.Equal(t, "/agent/mcp", v.Path)
	assert.Equal(t, http.StatusOK, initialize("/agent/mcp").Code)
	w = initialize("/mcp")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "不在这个地址")
	assert.NotContains(t, w.Body.String(), "/agent/mcp")
	var row models.MCPSetting
	require.NoError(t, global.GlobalDB.DB.First(&row, 1).Error)
	assert.True(t, row.Enabled)
	assert.Equal(t, "/agent/mcp", row.Path)

	// 换了路径以后关掉：新路径与 /mcp 都回 404 写明原因；网页别的地址不受影响
	require.Equal(t, http.StatusOK, put(`{"enabled":false,"path":"/agent/mcp"}`).Code)
	assert.Contains(t, initialize("/agent/mcp").Body.String(), "MCP 接入没有打开")
	assert.Contains(t, initialize("/mcp").Body.String(), "MCP 接入没有打开")
	assert.Equal(t, http.StatusOK, e.do(appReq{method: http.MethodGet, path: "/api/mcp/settings", session: true}).Code)

	// 空路径是默认的 /mcp
	w = put(`{"enabled":true,"path":""}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &v))
	assert.Equal(t, "/mcp", v.Path)
	assert.Equal(t, http.StatusOK, initialize("/mcp").Code)

	for _, c := range []struct{ path, want string }{
		{"/", "不能是 /"},
		{"/api/mcp", "/api 下面"},
		{"/API", "/api 下面"},
		{"/assets/mcp", "/assets 下面"},
		{"/login", "/login 下面"},
		{"/a b", "只能有"},
		{"/中文", "只能有"},
		{"/x/../mcp", "不能有 . 或 .."},
		{"/logo.svg", "冲突"},
		{"/" + strings.Repeat("a", 128), "最长"},
	} {
		w = put(`{"enabled":true,"path":"` + c.path + `"}`)
		assert.Equal(t, http.StatusBadRequest, w.Code, c.path)
		assert.Contains(t, w.Body.String(), c.want, c.path)
	}
	assert.Equal(t, http.StatusBadRequest, put(`{"enabled":true,"x":1}`).Code, "不认识的字段")
	assert.Equal(t, http.StatusOK, initialize("/mcp").Code, "改失败的不生效")
}
