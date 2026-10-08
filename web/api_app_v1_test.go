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
