package web

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/notify"
)

// newTemplateAppEnv 是接好消息模板服务的 App API 环境；set 是发送前套用的那份模板。
func newTemplateAppEnv(t *testing.T) (*appEnv, *notify.Templates) {
	t.Helper()
	e := newAppEnv(t)
	set := notify.NewTemplates(nil)
	e.srv.SetChatOpsDeps(&ChatOpsDeps{TemplateSvc: app.NewTemplateService(global.GlobalDB.DB, set)})
	return e, set
}

func findTemplate(t *testing.T, cat AppTemplateCatalog, typ string) AppTemplate {
	t.Helper()
	for _, v := range cat.Events {
		if v.Type == typ {
			return v
		}
	}
	t.Fatalf("目录里没有 %s", typ)
	return AppTemplate{}
}

// 目录：三组事件、公共变量与各事件的变量（示例写成文字，列表没有示例），没有凭证与链接字段
func TestAppTemplatesCatalog(t *testing.T) {
	e, _ := newTemplateAppEnv(t)
	read := e.token(apitoken.ScopeAppRead)
	w := e.do(appReq{method: http.MethodGet, path: "/api/app/v1/templates", bearer: read})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	var cat AppTemplateCatalog
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cat))
	require.Len(t, cat.Groups, 3)
	assert.Equal(t, []string{notify.GroupNotify, notify.GroupReply, notify.GroupAssistant},
		[]string{cat.Groups[0].Key, cat.Groups[1].Key, cat.Groups[2].Key})
	assert.Len(t, cat.Events, len(notify.Events))
	assert.NotEmpty(t, cat.CommonVars)

	matched := findTemplate(t, cat, notify.EventRSSMatched)
	assert.Equal(t, notify.GroupNotify, matched.Group)
	assert.False(t, matched.NoTitle)
	assert.NotEmpty(t, matched.SampleTitle)
	vars := map[string]AppTemplateVar{}
	for _, v := range matched.Vars {
		vars[v.Key] = v
	}
	assert.Equal(t, "16106127360", vars["size_bytes"].Sample, "整数写成十进制")
	assert.Equal(t, "true", vars["free"].Sample)

	status := findTemplate(t, cat, notify.EventReplyStatus)
	assert.Equal(t, notify.GroupReply, status.Group)
	assert.True(t, status.NoTitle)
	assert.Empty(t, status.SampleTitle)
	for _, v := range status.Vars {
		if v.Key == "items" {
			assert.Empty(t, v.Sample, "列表没有示例")
			assert.NotEmpty(t, v.Item)
		}
	}
	assert.False(t, status.Customized)
	assert.Zero(t, status.UpdatedAt)

	report := findTemplate(t, cat, notify.EventReportDaily)
	for _, v := range report.Vars {
		if v.Key == "bonus" {
			assert.Equal(t, "1234.5", v.Sample, "小数按最短的写法")
		}
	}
}

// 保存、预览、恢复默认：读要 app:read、写要 app:write；模板不对 400 并写明原因，没有这种事件 404；
// 保存以后发送时就按它套用；令牌的写操作记审计
func TestAppTemplatesSaveResetPreview(t *testing.T) {
	e, set := newTemplateAppEnv(t)
	read := e.token(apitoken.ScopeAppRead)
	write := e.token(apitoken.ScopeAppRead, apitoken.ScopeAppWrite)

	w := e.do(appReq{
		method: http.MethodPost, path: "/api/app/v1/templates/preview", bearer: read,
		body: `{"type":"reply.status","text":"{{.running}}/{{.tasks}}"}`,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var p AppTemplatePreview
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &p))
	assert.Equal(t, AppTemplatePreview{Title: "", Text: "2/3"}, p)

	w = e.do(appReq{method: http.MethodPost, path: "/api/app/v1/templates/preview", bearer: read, body: `{"type":"rss.matched","title":"{{.sitee}}"}`})
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "invalid_argument", errorCode(t, w))
	assert.Contains(t, w.Body.String(), "sitee")

	w = e.do(appReq{method: http.MethodPut, path: "/api/app/v1/templates/reply.status", bearer: read, body: `{"text":"x"}`})
	require.Equal(t, http.StatusForbidden, w.Code, "只读令牌不能改")

	w = e.do(appReq{method: http.MethodPut, path: "/api/app/v1/templates/reply.status", bearer: write, body: `{"text":"{{.running}} 个任务在跑"}`})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	var v AppTemplate
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &v))
	assert.True(t, v.Customized)
	assert.Equal(t, "{{.running}} 个任务在跑", v.Text)
	assert.Positive(t, v.UpdatedAt)
	ev, _, text := notify.SampleEvent(notify.EventReplyStatus)
	assert.Equal(t, "2 个任务在跑", set.ApplyText(ev, text), "保存以后发送时就按它套用")

	for _, c := range []struct {
		path, body, code string
		status           int
	}{
		{"/api/app/v1/templates/reply.status", `{"title":"x","text":"y"}`, "invalid_argument", http.StatusBadRequest},
		{"/api/app/v1/templates/rss.matched", `{"text":"{{range .timestamp}}{{end}}"}`, "invalid_argument", http.StatusBadRequest},
		{"/api/app/v1/templates/nope", `{"text":"x"}`, "not_found", http.StatusNotFound},
		{"/api/app/v1/templates/rss.matched", `{"text":"x","extra":1}`, "invalid_body", http.StatusBadRequest},
	} {
		w = e.do(appReq{method: http.MethodPut, path: c.path, bearer: write, body: c.body})
		require.Equal(t, c.status, w.Code, c.body)
		assert.Equal(t, c.code, errorCode(t, w), c.body)
	}

	w = e.do(appReq{method: http.MethodDelete, path: "/api/app/v1/templates/reply.status", bearer: write})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"ok":true}`, w.Body.String())
	assert.Equal(t, text, set.ApplyText(ev, text), "恢复默认以后发内置文字")
	assert.Equal(t, http.StatusNotFound, e.do(appReq{method: http.MethodDelete, path: "/api/app/v1/templates/nope", bearer: write}).Code)
	assert.Equal(t, http.StatusBadRequest, e.do(appReq{method: http.MethodDelete, path: "/api/app/v1/templates/reply.status", bearer: write, body: `{"x":1}`}).Code)

	w = e.do(appReq{method: http.MethodGet, path: "/api/app/v1/templates", session: true})
	require.Equal(t, http.StatusOK, w.Code)
	var cat AppTemplateCatalog
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cat))
	assert.False(t, findTemplate(t, cat, notify.EventReplyStatus).Customized)

	commands := map[string]int{}
	for _, a := range e.audit.all() {
		commands[a.Command+" "+a.Result]++
	}
	assert.Equal(t, 1, commands["PUT /api/app/v1/templates/{type} denied:scope"])
	assert.Equal(t, 1, commands["PUT /api/app/v1/templates/{type} success"])
	assert.Equal(t, 1, commands["DELETE /api/app/v1/templates/{type} success"])
	assert.Equal(t, 3, commands["PUT /api/app/v1/templates/{type} error:http_400"], "写坏的模板也记")
	assert.Equal(t, 1, commands["PUT /api/app/v1/templates/{type} error:http_404"])
	assert.Equal(t, 1, commands["DELETE /api/app/v1/templates/{type} error:http_404"])
	assert.Equal(t, 1, commands["DELETE /api/app/v1/templates/{type} error:http_400"])
}

// 模板服务没有启动：503
func TestAppTemplatesUnavailable(t *testing.T) {
	e := newAppEnv(t)
	read := e.token(apitoken.ScopeAppRead)
	w := e.do(appReq{method: http.MethodGet, path: "/api/app/v1/templates", bearer: read})
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, "unavailable", errorCode(t, w))
}

// 模板原文只给有 app:write 的调用者：只读的令牌看得到有自定义，拿不到原文；预览的结果遮住像凭证的内容
func TestAppTemplatesSourceOnlyForWriters(t *testing.T) {
	e, _ := newTemplateAppEnv(t)
	read := e.token(apitoken.ScopeAppRead)
	write := e.token(apitoken.ScopeAppRead, apitoken.ScopeAppWrite)
	secret := "https://pt.example/download.php?id=1&passkey=0123456789abcdef0123456789abcdef"
	w := e.do(appReq{method: http.MethodPut, path: "/api/app/v1/templates/rss.new", bearer: write, body: `{"text":"{{.text}} ` + secret + `"}`})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w = e.do(appReq{method: http.MethodGet, path: "/api/app/v1/templates", bearer: read})
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "0123456789abcdef0123456789abcdef")
	var cat AppTemplateCatalog
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cat))
	v := findTemplate(t, cat, notify.EventRSSNew)
	assert.True(t, v.Customized)
	assert.Empty(t, v.Text, "只读的调用者拿不到原文")

	w = e.do(appReq{method: http.MethodGet, path: "/api/app/v1/templates", bearer: write})
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cat))
	assert.Equal(t, "{{.text}} "+secret, findTemplate(t, cat, notify.EventRSSNew).Text, "能改的调用者按写的样子拿到")

	w = e.do(appReq{method: http.MethodPost, path: "/api/app/v1/templates/preview", bearer: read, body: `{"type":"rss.new","text":"{{.text}} ` + secret + `"}`})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "0123456789abcdef0123456789abcdef", "预览的结果遮住像凭证的内容")
}
