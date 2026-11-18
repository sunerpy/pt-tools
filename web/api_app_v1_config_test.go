package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/mcp"
	"github.com/sunerpy/pt-tools/models"
)

// App API 的配置接口：要 app:config，读与预览另要 app:read，保存另要 app:write；保存记审计（资源、操作、改了哪些字段，不记值）；
// 回应合乎契约，密钥的原文不出现。
func TestAppConfigScopesAndAudit(t *testing.T) {
	e := newConfigEnv(t)
	const pw = "dl-password-0123"
	dl := models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:1", Password: pw, Enabled: true, IsDefault: true}
	require.NoError(t, global.GlobalDB.DB.Create(&dl).Error)
	id := jsonUint(dl.ID)

	read := e.token(apitoken.ScopeAppRead)
	write := e.token(apitoken.ScopeAppWrite)
	readCfg := e.token(apitoken.ScopeAppRead, apitoken.ScopeAppConfig)
	writeCfg := e.token(apitoken.ScopeAppWrite, apitoken.ScopeAppConfig)

	w := e.do(appReq{method: http.MethodGet, path: "/api/app/v1/config", bearer: read})
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "app:config")
	w = e.do(appReq{method: http.MethodGet, path: "/api/app/v1/config", bearer: readCfg})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var cat ConfigCatalog
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cat))
	assert.NotEmpty(t, cat.Resources)

	w = e.do(appReq{method: http.MethodGet, path: "/api/app/v1/config/downloaders", bearer: readCfg})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), pw)
	w = e.do(appReq{method: http.MethodGet, path: "/api/app/v1/config/downloaders?id=" + id, bearer: readCfg})
	require.Equal(t, http.StatusOK, w.Code)
	w = e.do(appReq{method: http.MethodGet, path: "/api/app/v1/config/nope", bearer: readCfg})
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "not_found", errorCode(t, w))

	change := `{"op":"update","id":"` + id + `","values":{"auto_start":true,"password":"new-pass-0123456"}}`
	w = e.do(appReq{method: http.MethodPost, path: "/api/app/v1/config/downloaders/preview", bearer: readCfg, body: change})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var pv ConfigChangeResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &pv))
	assert.False(t, pv.Applied)
	assert.Len(t, pv.Changes, 2)
	assert.NotContains(t, w.Body.String(), "new-pass")

	w = e.do(appReq{method: http.MethodPost, path: "/api/app/v1/config/downloaders/changes", bearer: readCfg, body: change})
	assert.Equal(t, http.StatusForbidden, w.Code, "保存要 app:write")
	before := len(e.audit.all())
	w = e.do(appReq{method: http.MethodPost, path: "/api/app/v1/config/downloaders/changes", bearer: write, body: change})
	assert.Equal(t, http.StatusForbidden, w.Code, "保存也要 app:config")
	a := e.audit.all()
	require.Len(t, a, before+1)
	assert.Equal(t, "denied:scope", a[len(a)-1].Result)

	w = e.do(appReq{method: http.MethodPost, path: "/api/app/v1/config/downloaders/changes", bearer: writeCfg, body: change})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var res ConfigChangeResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.True(t, res.Applied)
	assert.NotContains(t, w.Body.String(), "new-pass")
	require.NoError(t, global.GlobalDB.DB.First(&dl, dl.ID).Error)
	assert.True(t, dl.AutoStart)
	assert.Equal(t, "new-pass-0123456", dl.Password)
	a = e.audit.all()
	last := a[len(a)-1]
	assert.Equal(t, "success", last.Result)
	assert.Equal(t, "downloaders", last.Args["resource"])
	assert.Equal(t, "update", last.Args["op"])
	assert.Equal(t, []string{"auto_start", "password"}, last.Args["fields"])
	assert.NotContains(t, configJSON(t, last.Args), "new-pass")

	w = e.do(appReq{method: http.MethodPost, path: "/api/app/v1/config/downloaders/changes", bearer: writeCfg, body: `{"op":"update","id":"` + id + `","values":{"nope":1}}`})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "invalid_argument", errorCode(t, w))
	w = e.do(appReq{method: http.MethodPost, path: "/api/app/v1/config/downloaders/changes", bearer: writeCfg, body: `{"op":"update","extra":1}`})
	assert.Equal(t, http.StatusBadRequest, w.Code, "不收多余的字段")

	w = e.do(appReq{method: http.MethodGet, path: "/api/app/v1/config", session: true})
	assert.Equal(t, http.StatusOK, w.Code, "网页登录有全部权限")
}

func jsonUint(n uint) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// MCP 的配置工具：get_config 读目录与现状；change_config 不带 confirm 回预览、带了才保存并记审计；
// get_skill 的配置技能列出全部资源与不能改的；令牌没有 app:config 时工具报错，技能写明读不到目录。
func TestMCPConfigTools(t *testing.T) {
	e := newConfigEnv(t)
	dl := models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:1", Password: "pw-secret-1", Enabled: true, IsDefault: true}
	require.NoError(t, global.GlobalDB.DB.Create(&dl).Error)
	ts := httptest.NewServer(e.handler)
	t.Cleanup(ts.Close)
	cs, err := mcpSession(t, ts.URL, e.token(apitoken.ScopeMCPRead, apitoken.ScopeMCPWrite, apitoken.ScopeAppConfig))
	require.NoError(t, err)
	call := func(cs *sdk.ClientSession, name string, args map[string]any) (*sdk.CallToolResult, map[string]any, string) {
		t.Helper()
		res, cerr := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
		require.NoError(t, cerr)
		out := map[string]any{}
		if res.StructuredContent != nil {
			b, _ := json.Marshal(res.StructuredContent)
			require.NoError(t, json.Unmarshal(b, &out))
		}
		text := ""
		for _, c := range res.Content {
			if tc, ok := c.(*sdk.TextContent); ok {
				text += tc.Text
			}
		}
		return res, out, text
	}

	res, out, _ := call(cs, "get_config", nil)
	require.False(t, res.IsError)
	resources, _ := out["resources"].([]any)
	require.NotEmpty(t, resources)
	res, out, text := call(cs, "get_config", map[string]any{"resource": "downloaders"})
	require.False(t, res.IsError, text)
	assert.NotContains(t, text, "pw-secret-1")
	items, _ := out["items"].([]any)
	require.Len(t, items, 1)

	id := jsonUint(dl.ID)
	args := map[string]any{"resource": "downloaders", "op": "update", "id": id, "values": map[string]any{"enabled": false, "is_default": true}}
	res, out, text = call(cs, "change_config", args)
	require.False(t, res.IsError, text)
	assert.Equal(t, true, out["pending"], "不带 confirm：只预览")
	assert.Equal(t, "change_config", out["tool"])
	assert.Contains(t, text, "改下载器「qb」")
	require.NoError(t, global.GlobalDB.DB.First(&dl, dl.ID).Error)
	assert.True(t, dl.Enabled, "预览不改")

	// 默认下载器不能停用：接口的校验经工具原样报出来
	args["confirm"] = true
	res, _, text = call(cs, "change_config", args)
	assert.True(t, res.IsError)
	assert.Contains(t, text, "默认下载器")

	args["values"] = map[string]any{"auto_start": true}
	res, out, text = call(cs, "change_config", args)
	require.False(t, res.IsError, text)
	assert.Equal(t, true, out["applied"])
	a := e.audit.all()
	last := a[len(a)-1]
	assert.Equal(t, mcp.ChannelType, last.ChannelType)
	assert.Equal(t, "change_config", last.Command)
	assert.Equal(t, "success", last.Result)
	assert.Equal(t, []any{"auto_start"}, toAnySlice(last.Args["fields"]))

	res, out, _ = call(cs, mcp.SkillTool, nil)
	require.False(t, res.IsError)
	assert.Contains(t, configJSON(t, out), mcp.SkillConfig)
	res, _, text = call(cs, mcp.SkillTool, map[string]any{"name": mcp.SkillConfig})
	require.False(t, res.IsError)
	for _, r := range e.srv.configResources() {
		assert.Contains(t, text, "### "+r.Name+"（", "技能里有 %s", r.Name)
	}
	for _, x := range configExcluded {
		assert.Contains(t, text, x)
	}
	res, _, text = call(cs, mcp.SkillTool, map[string]any{"name": "nope"})
	assert.True(t, res.IsError, text)

	plain, err := mcpSession(t, ts.URL, e.token(apitoken.ScopeMCPRead, apitoken.ScopeMCPWrite))
	require.NoError(t, err)
	res, _, text = call(plain, "get_config", nil)
	assert.True(t, res.IsError)
	assert.Contains(t, text, "app:config")
	res, _, text = call(plain, mcp.SkillTool, map[string]any{"name": mcp.SkillConfig})
	require.False(t, res.IsError)
	assert.Contains(t, text, "读不到配置目录")
}

func toAnySlice(v any) []any {
	b, _ := json.Marshal(v)
	var out []any
	_ = json.Unmarshal(b, &out)
	return out
}
