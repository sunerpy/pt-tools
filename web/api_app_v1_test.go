package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/models"
)

type fakeAppAudit struct {
	mu      sync.Mutex
	entries []app.AuditEntry
}

func (f *fakeAppAudit) Record(_ context.Context, e app.AuditEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, e)
	return nil
}

func (f *fakeAppAudit) all() []app.AuditEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]app.AuditEntry(nil), f.entries...)
}

type appEnv struct {
	t       *testing.T
	srv     *Server
	handler http.Handler
	tokens  *apitoken.Store
	audit   *fakeAppAudit
}

// newAppEnv 起一个接好令牌库的服务，handler 是 Serve 用的那一套完整路由（真实的 mux）。
func newAppEnv(t *testing.T) *appEnv {
	t.Helper()
	srv := setupServer(t)
	srv.mgr.StopAll()
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.APIToken{}))
	e := &appEnv{t: t, srv: srv, tokens: apitoken.New(global.GlobalDB.DB), audit: &fakeAppAudit{}}
	srv.SetAPITokens(e.tokens, e.audit)
	srv.sessions.put("sess-app", "admin")
	e.handler = srv.buildHandler()
	return e
}

func (e *appEnv) token(scopes ...string) string {
	e.t.Helper()
	_, plain, err := e.tokens.Create(context.Background(), apitoken.CreateInput{Name: "测试 " + strings.Join(scopes, ","), Scopes: scopes}, "admin")
	require.NoError(e.t, err)
	return plain
}

type appReq struct {
	method, path, body string
	bearer             string
	session            bool
}

func (e *appEnv) do(q appReq) *httptest.ResponseRecorder {
	e.t.Helper()
	req := httptest.NewRequest(q.method, q.path, bytes.NewBufferString(q.body))
	if q.body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if q.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+q.bearer)
	}
	if q.session {
		req.AddCookie(&http.Cookie{Name: "session", Value: "sess-app"})
	}
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)
	return w
}

func errorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	return body.Error
}

// 主体：没有 401；令牌不对 401；权限范围不够 403；session 拥有全部权限范围；令牌只认请求头
func TestAppAPI_PrincipalAndScope(t *testing.T) {
	e := newAppEnv(t)
	w := e.do(appReq{method: http.MethodGet, path: "/api/app/v1/meta"})
	require.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "unauthorized", errorCode(t, w))
	assert.Equal(t, http.StatusUnauthorized, e.do(appReq{method: http.MethodGet, path: "/api/app/v1/meta", bearer: "ptt_1_nope"}).Code)

	read := e.token(apitoken.ScopeAppRead)
	assert.Equal(t, http.StatusUnauthorized, e.do(appReq{method: http.MethodGet, path: "/api/app/v1/meta?access_token=" + read}).Code, "查询串里的令牌不认")
	mcp := e.token(apitoken.ScopeMCPRead, apitoken.ScopeMCPWrite, apitoken.ScopeQbitCompat)
	w = e.do(appReq{method: http.MethodGet, path: "/api/app/v1/meta", bearer: mcp})
	require.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "forbidden", errorCode(t, w))

	w = e.do(appReq{method: http.MethodGet, path: "/api/app/v1/meta", bearer: read})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var meta AppMeta
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &meta))
	assert.Equal(t, AppRemoteAPILevel, meta.RemoteAPILevel)
	assert.Equal(t, "api_token", meta.Principal.Kind)
	assert.Equal(t, []string{apitoken.ScopeAppRead}, meta.Principal.Scopes)
	assert.Contains(t, meta.Features, "subscriptions")

	w = e.do(appReq{method: http.MethodGet, path: "/api/app/v1/meta", session: true})
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &meta))
	assert.Equal(t, "session", meta.Principal.Kind)
	assert.Equal(t, apitoken.Scopes, meta.Principal.Scopes)

	w = e.do(appReq{method: http.MethodGet, path: "/api/app/v1/nothing", bearer: read})
	require.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "not_found", errorCode(t, w), "没有的接口回 JSON 的 404，不落到 SPA")
}

// 令牌（哪怕权限范围全有）访问现有的 /api/* 与令牌管理接口一律被拒：那些只认 session
func TestAppAPI_TokensCannotReachSessionAPIs(t *testing.T) {
	e := newAppEnv(t)
	all := e.token(apitoken.Scopes...)
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/qbit"},
		{http.MethodGet, "/api/sites"},
		{http.MethodGet, "/api/global"},
		{http.MethodGet, "/api/tokens"},
		{http.MethodPost, "/api/tokens"},
		{http.MethodDelete, "/api/tokens/1"},
		{http.MethodGet, "/api/chatops/tokens"},
		{http.MethodGet, "/api/media/subscriptions"},
		{http.MethodGet, "/api/downloader-torrents"},
		{http.MethodPost, "/api/v2/torrents/push"},
	} {
		w := e.do(appReq{method: c.method, path: c.path, bearer: all, body: "{}"})
		assert.Contains(t, []int{http.StatusUnauthorized, http.StatusFound}, w.Code, "%s %s 回了 %d", c.method, c.path, w.Code)
	}
	w := e.do(appReq{method: http.MethodGet, path: "/", bearer: all})
	assert.Equal(t, http.StatusFound, w.Code, "SPA 也只认 session")
}

// 只读但用 POST 传条件的路由
var appReadPosts = map[string]bool{"POST /search": true}

// 路由表：每条都声明了权限范围；读是 GET（或者上面列出的只读 POST）+ app:read，写是别的方法 + app:write；没有重复
func TestAppAPI_RouteTable(t *testing.T) {
	srv := setupServer(t)
	srv.mgr.StopAll()
	seen := map[string]bool{}
	for _, rt := range srv.appRoutes() {
		key := rt.Method + " " + rt.Path
		require.False(t, seen[key], "重复的路由 %s", key)
		seen[key] = true
		require.NotNil(t, rt.Handler, key)
		if rt.Method == http.MethodGet || appReadPosts[key] {
			assert.Equal(t, apitoken.ScopeAppRead, rt.Scope, key)
		} else {
			assert.Equal(t, apitoken.ScopeAppWrite, rt.Scope, key)
		}
	}
}

// 权限范围矩阵：只有 app:read 的令牌调不了写接口，只有 app:write 的调不了读接口
func TestAppAPI_ScopeMatrix(t *testing.T) {
	e := newAppEnv(t)
	read := e.token(apitoken.ScopeAppRead)
	write := e.token(apitoken.ScopeAppWrite)
	for _, rt := range e.srv.appRoutes() {
		path := appPrefix + concretePath(rt.Path)
		gotRead := e.do(appReq{method: rt.Method, path: path, bearer: read, body: "{}"}).Code
		gotWrite := e.do(appReq{method: rt.Method, path: path, bearer: write, body: "{}"}).Code
		if rt.Scope == apitoken.ScopeAppRead {
			assert.NotContains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, gotRead, "%s %s 读令牌", rt.Method, rt.Path)
			assert.Equal(t, http.StatusForbidden, gotWrite, "%s %s 写令牌", rt.Method, rt.Path)
		} else {
			assert.Equal(t, http.StatusForbidden, gotRead, "%s %s 读令牌", rt.Method, rt.Path)
			assert.NotContains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, gotWrite, "%s %s 写令牌", rt.Method, rt.Path)
		}
	}
}

// concretePath 把路由里的 {参数} 换成一个具体的值。
func concretePath(p string) string {
	for {
		i := strings.Index(p, "{")
		if i < 0 {
			return p
		}
		j := strings.Index(p[i:], "}")
		p = p[:i] + "1" + p[i+j+1:]
	}
}

// 令牌管理：只认 session；新建只回一次明文，列表里没有明文与散列；撤销以后令牌立即失效
func TestTokenAdminAPI(t *testing.T) {
	e := newAppEnv(t)
	w := e.do(appReq{method: http.MethodPost, path: "/api/tokens", session: true, body: `{"name":"手机","scopes":["app:read","app:write"],"expires_in_days":90}`})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var created TokenCreated
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.True(t, strings.HasPrefix(created.Plaintext, "ptt_"))
	assert.Equal(t, []string{"app:read", "app:write"}, created.Token.Scopes)
	assert.Equal(t, "admin", created.Token.CreatedBy)
	assert.Equal(t, http.StatusOK, e.do(appReq{method: http.MethodGet, path: "/api/app/v1/meta", bearer: created.Plaintext}).Code)

	w = e.do(appReq{method: http.MethodGet, path: "/api/tokens", session: true})
	require.Equal(t, http.StatusOK, w.Code)
	_, secret, _ := strings.Cut(strings.TrimPrefix(created.Plaintext, "ptt_"), "_")
	assert.NotContains(t, w.Body.String(), secret)
	assert.NotContains(t, w.Body.String(), "secret")
	assert.NotContains(t, w.Body.String(), "hash")

	for _, body := range []string{`{"name":"","scopes":["app:read"]}`, `{"name":"x","scopes":["app:*"]}`, `{"name":"x","scopes":["app:read"],"admin":true}`} {
		assert.Equal(t, http.StatusBadRequest, e.do(appReq{method: http.MethodPost, path: "/api/tokens", session: true, body: body}).Code, body)
	}
	assert.Equal(t, http.StatusBadRequest, e.do(appReq{method: http.MethodDelete, path: "/api/tokens/x", session: true}).Code)
	assert.Equal(t, http.StatusNotFound, e.do(appReq{method: http.MethodDelete, path: "/api/tokens/999", session: true}).Code)

	id := strconv.FormatUint(uint64(created.Token.ID), 10)
	require.Equal(t, http.StatusOK, e.do(appReq{method: http.MethodDelete, path: "/api/tokens/" + id, session: true}).Code)
	assert.Equal(t, http.StatusUnauthorized, e.do(appReq{method: http.MethodGet, path: "/api/app/v1/meta", bearer: created.Plaintext}).Code, "撤销立即生效")

	bare := setupServer(t)
	bare.mgr.StopAll()
	bare.sessions.put("sess-app", "admin")
	h := bare.buildHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/tokens", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "sess-app"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code, "没有接上令牌库")
}

// 令牌的写请求记审计，结果用审计页认得的写法：处理成功 success，失败 error:http_<状态码>，权限范围不够 denied:scope。
// 读请求、session 的写请求与没有主体的请求不记。
func TestAppAPI_AuditsTokenWrites(t *testing.T) {
	e := newAppEnv(t)
	write := e.token(apitoken.ScopeAppRead, apitoken.ScopeAppWrite)
	w := e.do(appReq{method: http.MethodPost, path: "/api/app/v1/torrents/actions", bearer: write, body: `{"action":"pause","targets":[]}`})
	require.Equal(t, http.StatusBadRequest, w.Code)
	e.do(appReq{method: http.MethodGet, path: "/api/app/v1/meta", bearer: write})
	e.do(appReq{method: http.MethodPost, path: "/api/app/v1/torrents/actions", session: true, body: `{"action":"pause","targets":[]}`})
	e.do(appReq{method: http.MethodPost, path: "/api/app/v1/push", body: `{}`})
	read := e.token(apitoken.ScopeAppRead)
	require.Equal(t, http.StatusForbidden, e.do(appReq{method: http.MethodPost, path: "/api/app/v1/push", bearer: read, body: `{}`}).Code)
	writeOnly := e.token(apitoken.ScopeAppWrite)
	require.Equal(t, http.StatusForbidden, e.do(appReq{method: http.MethodGet, path: "/api/app/v1/meta", bearer: writeOnly}).Code)

	entries := e.audit.all()
	require.Len(t, entries, 2, "只记令牌的写请求")
	got := entries[0]
	assert.Equal(t, "api_token", got.ChannelType)
	assert.NotEmpty(t, got.ChannelUserID)
	assert.Equal(t, "POST /api/app/v1/torrents/actions", got.Command)
	assert.Equal(t, "error:http_400", got.Result)
	assert.Zero(t, got.NotificationConfID)
	assert.Equal(t, "POST /api/app/v1/push", entries[1].Command)
	assert.Equal(t, "denied:scope", entries[1].Result)
	assert.NotEqual(t, got.ChannelUserID, entries[1].ChannelUserID, "记的是各自的令牌编号")

	h := e.srv.appHandler(appRoute{http.MethodPost, "/probe", apitoken.ScopeAppWrite, func(w http.ResponseWriter, _ *http.Request) {
		appJSON(w, map[string]bool{"ok": true})
	}})
	req := httptest.NewRequest(http.MethodPost, appPrefix+"/probe", nil)
	req.Header.Set("Authorization", "Bearer "+write)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	entries = e.audit.all()
	require.Len(t, entries, 3)
	assert.Equal(t, "success", entries[2].Result)
	assert.Equal(t, "POST /api/app/v1/probe", entries[2].Command)
}

// 业务失败（HTTP 200 但推送被拦下、批量动作全失败）在审计里不能记成 success：处理函数写进去的结果优先
func TestAppAPI_AuditOutcome(t *testing.T) {
	e := newAppEnv(t)
	write := e.token(apitoken.ScopeAppRead, apitoken.ScopeAppWrite)
	run := func(outcome string, status int) app.AuditEntry {
		h := e.srv.appHandler(appRoute{http.MethodPost, "/probe", apitoken.ScopeAppWrite, func(w http.ResponseWriter, r *http.Request) {
			if outcome != "" {
				appSetOutcome(r, outcome)
			}
			if status != http.StatusOK {
				appError(w, status, "x", "x")
				return
			}
			appJSON(w, map[string]bool{"ok": true})
		}})
		req := httptest.NewRequest(http.MethodPost, appPrefix+"/probe", nil)
		req.Header.Set("Authorization", "Bearer "+write)
		h.ServeHTTP(httptest.NewRecorder(), req)
		all := e.audit.all()
		return all[len(all)-1]
	}
	assert.Equal(t, "error:all_failed", run("error:all_failed", http.StatusOK).Result)
	assert.Equal(t, "success", run("", http.StatusOK).Result)
	assert.Equal(t, "error:http_409", run("error:all_failed", http.StatusConflict).Result, "状态码已经是失败时按状态码记")
}

// 路径对、方法不对回 405 并带 Allow；没有的路径才是 404
func TestAppAPI_MethodNotAllowed(t *testing.T) {
	e := newAppEnv(t)
	read := e.token(apitoken.ScopeAppRead)
	for _, c := range []struct {
		method, path, allow string
	}{
		{http.MethodDelete, "/api/app/v1/meta", "GET"},
		{http.MethodGet, "/api/app/v1/push", "POST"},
		{http.MethodGet, "/api/app/v1/subscriptions/1/status", "POST"},
		{http.MethodPut, "/api/app/v1/subscriptions/1", "DELETE, GET"},
		{http.MethodPatch, "/api/app/v1/subscriptions", "GET, POST"},
	} {
		w := e.do(appReq{method: c.method, path: c.path, bearer: read})
		require.Equal(t, http.StatusMethodNotAllowed, w.Code, c.method+" "+c.path)
		assert.Equal(t, "method_not_allowed", errorCode(t, w))
		assert.Equal(t, c.allow, w.Header().Get("Allow"), c.method+" "+c.path)
	}
	w := e.do(appReq{method: http.MethodGet, path: "/api/app/v1/nope", bearer: read})
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, http.StatusOK, e.do(appReq{method: http.MethodGet, path: "/api/app/v1/meta", bearer: read}).Code, "方法对的照常")
}

// 不带请求体的写接口：空的或 {} 可以，别的回 400；所有请求体最多 1 MiB
func TestAppAPI_NoBodyEndpoints(t *testing.T) {
	e := newAppEnv(t)
	write := e.token(apitoken.ScopeAppRead, apitoken.ScopeAppWrite)
	for _, p := range []struct{ method, path string }{
		{http.MethodPost, "/api/app/v1/sites/nosuch/attend"},
		{http.MethodPost, "/api/app/v1/subscriptions/1/search"},
		{http.MethodDelete, "/api/app/v1/subscriptions/1"},
	} {
		w := e.do(appReq{method: p.method, path: p.path, bearer: write, body: `{"x":1}`})
		require.Equal(t, http.StatusBadRequest, w.Code, p.path)
		assert.Equal(t, "invalid_body", errorCode(t, w), p.path)
		for _, ok := range []string{"", "{}", " {} \n"} {
			assert.NotEqual(t, http.StatusBadRequest, e.do(appReq{method: p.method, path: p.path, bearer: write, body: ok}).Code, p.path+" body="+ok)
		}
	}
	big := `{"keyword":"` + strings.Repeat("a", appMaxBody) + `"}`
	w := e.do(appReq{method: http.MethodPost, path: "/api/app/v1/search", bearer: write, body: big})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
