package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/app"
)

func TestMain(m *testing.M) {
	global.GlobalLogger = zap.NewNop()
	m.Run()
}

// fakeBackend 记下工具发给 App API 的请求，回 reply（没设时回 {"items":[]}）。
type fakeBackend struct {
	mu    sync.Mutex
	calls []Request
	reply func(req Request) Response
}

func (f *fakeBackend) Call(_ context.Context, _ Caller, req Request) (Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, req)
	if f.reply != nil {
		return f.reply(req), nil
	}
	return Response{Status: http.StatusOK, Body: []byte(`{"items":[]}`)}, nil
}

func (f *fakeBackend) got() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

type fakeAudit struct {
	mu      sync.Mutex
	entries []app.AuditEntry
}

func (a *fakeAudit) Record(_ context.Context, e app.AuditEntry) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, e)
	return nil
}

func (a *fakeAudit) all() []app.AuditEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.entries)
}

type env struct {
	t       *testing.T
	backend *fakeBackend
	audit   *fakeAudit
	caller  Caller
	session *sdk.ClientSession
}

// newEnv 在内存传输上连好一个客户端；调用的令牌默认有 mcp:read 与 mcp:write。
func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{t: t, backend: &fakeBackend{}, audit: &fakeAudit{}, caller: Caller{TokenID: 7, Name: "claude", Scopes: []string{apitoken.ScopeMCPRead, apitoken.ScopeMCPWrite}}}
	srv := NewServer(Deps{
		Backend: e.backend, Audit: e.audit, Version: "v1.0.0-test",
		ResolveURL: func(raw string) (string, string, bool) {
			if strings.HasPrefix(raw, "https://hdsky.me/download.php?id=") {
				return "hdsky", strings.TrimPrefix(raw, "https://hdsky.me/download.php?id="), true
			}
			return "", "", false
		},
		Caller: func(*sdk.CallToolRequest) (Caller, bool) { return e.caller, e.caller.TokenID != 0 },
	})
	ct, st := sdk.NewInMemoryTransports()
	ctx := context.Background()
	ss, err := srv.Connect(ctx, st, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	e.session = cs
	return e
}

// call 调一个工具，回结果的文本与是不是错误。
func (e *env) call(name string, args map[string]any) (string, bool, map[string]any) {
	e.t.Helper()
	res, err := e.session.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	require.NoError(e.t, err)
	var text string
	for _, c := range res.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			text += tc.Text
		}
	}
	var structured map[string]any
	if res.StructuredContent != nil {
		b, _ := json.Marshal(res.StructuredContent)
		_ = json.Unmarshal(b, &structured)
	}
	return text, res.IsError, structured
}

// 工具清单：13 个，名字和 ToolNames 一样；只读工具标 readOnly，删除标 destructive；参数的取值范围写进了 schema
func TestToolList(t *testing.T) {
	e := newEnv(t)
	res, err := e.session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	var names []string
	byName := map[string]*sdk.Tool{}
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		byName[tool.Name] = tool
		assert.NotEmpty(t, tool.Description, tool.Name)
	}
	assert.ElementsMatch(t, ToolNames, names)
	for _, name := range []string{"list_tasks", "search_torrents", "get_downloader_stats", "check_updates"} {
		assert.True(t, byName[name].Annotations.ReadOnlyHint, name)
	}
	for _, name := range []string{"pause_torrent", "resume_torrent", "delete_torrent", "push_torrent", "add_subscription"} {
		ann := byName[name].Annotations
		assert.False(t, ann.ReadOnlyHint, name)
		require.NotNil(t, ann.DestructiveHint, name)
		assert.Equal(t, name == "delete_torrent", *ann.DestructiveHint, name)
	}
	schema, _ := json.Marshal(byName["list_downloader_torrents"].InputSchema)
	assert.Contains(t, string(schema), `"seeding"`, "state 的取值范围")
	schema, _ = json.Marshal(byName["pause_torrent"].InputSchema)
	assert.Contains(t, string(schema), `"confirm"`)
}

// 只读工具：参数换成 App API 的请求（查询串、请求体），回应原样交给客户端
func TestReadTools(t *testing.T) {
	e := newEnv(t)
	cases := []struct {
		tool   string
		args   map[string]any
		method string
		path   string
	}{
		{"list_tasks", map[string]any{"site": "hdsky", "keyword": "Dune", "pushed_only": true, "page": 2, "page_size": 50}, http.MethodGet, "/tasks?page=2&page_size=50&pushed=1&q=Dune&site=hdsky"},
		{"list_tasks", nil, http.MethodGet, "/tasks"},
		{"list_downloader_torrents", map[string]any{"downloader_id": 3, "state": "seeding", "sort": "ratio", "order": "desc"}, http.MethodGet, "/torrents?downloader_id=3&order=desc&sort=ratio&state=seeding"},
		{"get_downloader_stats", nil, http.MethodGet, "/downloaders"},
		{"get_site_userinfo", nil, http.MethodGet, "/sites"},
		{"check_updates", map[string]any{"include_prerelease": true}, http.MethodGet, "/updates?include_prerelease=1"},
		{"explore_media", map[string]any{"kind": "movie", "query": "沙丘"}, http.MethodGet, "/explore?kind=movie&list=search&q=%E6%B2%99%E4%B8%98"},
		{"explore_media", map[string]any{"kind": "tv", "list": "popular", "page": 2}, http.MethodGet, "/explore?kind=tv&list=popular&page=2"},
		{"list_subscriptions", map[string]any{"status": "active"}, http.MethodGet, "/subscriptions?status=active"},
	}
	for i, c := range cases {
		text, isErr, _ := e.call(c.tool, c.args)
		require.False(t, isErr, "%s: %s", c.tool, text)
		got := e.backend.got()
		require.Len(t, got, i+1)
		assert.Equal(t, c.method, got[i].Method, c.tool)
		assert.Equal(t, c.path, got[i].Path, c.tool)
		assert.False(t, got[i].Write, c.tool)
	}
	assert.Empty(t, e.audit.all(), "只读工具不记审计")
}

// search_torrents：发 POST /search；结果按 limit 截断（默认 20），没给关键字时不调
func TestSearchTool(t *testing.T) {
	e := newEnv(t)
	items := make([]any, 30)
	for i := range items {
		items[i] = map[string]any{"site": "hdsky", "torrent_id": i}
	}
	e.backend.reply = func(Request) Response {
		b, _ := json.Marshal(map[string]any{"items": items, "total": 30})
		return Response{Status: http.StatusOK, Body: b}
	}
	_, isErr, out := e.call("search_torrents", map[string]any{"keyword": "Dune", "sites": []string{"hdsky"}, "free_only": true, "limit": 5})
	require.False(t, isErr)
	assert.Len(t, out["items"], 5)
	assert.Equal(t, true, out["truncated"])
	req := e.backend.got()[0]
	assert.Equal(t, "/search", req.Path)
	body, _ := json.Marshal(req.Body)
	assert.JSONEq(t, `{"keyword":"Dune","sites":["hdsky"],"category":"","free_only":true,"min_seeders":0}`, string(body))

	_, isErr, out = e.call("search_torrents", map[string]any{"keyword": "Dune"})
	require.False(t, isErr)
	assert.Len(t, out["items"], 20, "默认 20 条")

	text, isErr, _ := e.call("search_torrents", map[string]any{"keyword": "  "})
	assert.True(t, isErr)
	assert.Contains(t, text, "keyword")
	assert.Len(t, e.backend.got(), 2)
}

// get_downloader_stats 与 get_site_userinfo 按参数只留那一项
func TestFilteredTools(t *testing.T) {
	e := newEnv(t)
	e.backend.reply = func(req Request) Response {
		if req.Path == "/downloaders" {
			return Response{Status: http.StatusOK, Body: []byte(`{"items":[{"id":1,"name":"qb"},{"id":2,"name":"tr"}]}`)}
		}
		return Response{Status: http.StatusOK, Body: []byte(`{"items":[{"name":"hdsky"},{"name":"mteam"}]}`)}
	}
	_, _, out := e.call("get_downloader_stats", map[string]any{"downloader_id": 2})
	assert.Equal(t, []any{map[string]any{"id": float64(2), "name": "tr"}}, out["items"])
	_, _, out = e.call("get_site_userinfo", map[string]any{"site": "HDSky"})
	assert.Equal(t, []any{map[string]any{"name": "hdsky"}}, out["items"])
}

// 写工具：暂停、继续、删除换成 /torrents/actions；要 confirm=true；每次调用记审计（通道 mcp，命令是工具名，用户是令牌编号）
func TestWriteTools(t *testing.T) {
	e := newEnv(t)
	e.backend.reply = func(Request) Response {
		return Response{Status: http.StatusOK, Body: []byte(`{"succeeded":1,"failed":0,"results":[]}`)}
	}
	target := map[string]any{"downloader_id": 1, "task_id": "abc", "confirm": true}
	for tool, action := range map[string]string{"pause_torrent": "pause", "resume_torrent": "resume"} {
		_, isErr, _ := e.call(tool, target)
		require.False(t, isErr, tool)
		req := e.backend.got()[len(e.backend.got())-1]
		assert.Equal(t, "/torrents/actions", req.Path)
		assert.True(t, req.Write)
		body, _ := json.Marshal(req.Body)
		assert.JSONEq(t, `{"action":"`+action+`","targets":[{"downloader_id":1,"task_id":"abc"}]}`, string(body))
	}
	_, isErr, _ := e.call("delete_torrent", map[string]any{"downloader_id": 1, "task_id": "abc", "remove_data": true, "confirm": true})
	require.False(t, isErr)
	body, _ := json.Marshal(e.backend.got()[2].Body)
	assert.Contains(t, string(body), `"delete_with_files"`)

	a := e.audit.all()
	require.Len(t, a, 3)
	assert.Equal(t, ChannelType, a[0].ChannelType)
	assert.Equal(t, "7", a[0].ChannelUserID)
	assert.Equal(t, "success", a[0].Result)
	assert.Equal(t, "claude", a[0].Args["name"])
	assert.Equal(t, "delete_torrent", a[2].Command)
	assert.Equal(t, "delete_with_files", a[2].Args["action"])

	// 没有 confirm：不调，审计记 denied:confirm
	text, isErr, _ := e.call("pause_torrent", map[string]any{"downloader_id": 1, "task_id": "abc", "confirm": false})
	assert.True(t, isErr)
	assert.Contains(t, text, "confirm=true")
	assert.Len(t, e.backend.got(), 3)
	assert.Equal(t, "denied:confirm", e.audit.all()[3].Result)
	// 没给种子：不调，审计记 error:invalid_argument
	_, isErr, _ = e.call("resume_torrent", map[string]any{"downloader_id": 0, "task_id": "", "confirm": true})
	assert.True(t, isErr)
	assert.Equal(t, "error:invalid_argument", e.audit.all()[4].Result)
}

// 权限范围：只有 mcp:read 的令牌调写工具被拒（记 denied:scope），只有 mcp:write 的调只读工具被拒；没有令牌时一律拒绝
func TestToolScopes(t *testing.T) {
	e := newEnv(t)
	e.caller.Scopes = []string{apitoken.ScopeMCPRead}
	text, isErr, _ := e.call("pause_torrent", map[string]any{"downloader_id": 1, "task_id": "abc", "confirm": true})
	assert.True(t, isErr)
	assert.Contains(t, text, "mcp:write")
	assert.Equal(t, "denied:scope", e.audit.all()[0].Result)
	_, isErr, _ = e.call("list_tasks", nil)
	assert.False(t, isErr)

	e.caller.Scopes = []string{apitoken.ScopeMCPWrite}
	text, isErr, _ = e.call("list_tasks", nil)
	assert.True(t, isErr)
	assert.Contains(t, text, "mcp:read")

	e.caller = Caller{}
	text, isErr, _ = e.call("list_tasks", nil)
	assert.True(t, isErr)
	assert.Contains(t, text, "令牌")
	assert.Len(t, e.backend.got(), 1, "被拒的都没有调 App API")
	assert.Len(t, e.audit.all(), 1, "只读工具被拒不记审计")
}

// push_torrent：给站点与编号，或者已知站点的下载地址（解析成站点与编号，不请求它）；审计里不记地址（可能带 passkey）
func TestPushTool(t *testing.T) {
	e := newEnv(t)
	e.backend.reply = func(Request) Response {
		return Response{Status: http.StatusOK, Body: []byte(`{"success":true,"skipped":false}`)}
	}
	_, isErr, _ := e.call("push_torrent", map[string]any{"site": "mteam", "torrent_id": "123", "downloader_id": 2, "category": "movies", "confirm": true})
	require.False(t, isErr)
	body, _ := json.Marshal(e.backend.got()[0].Body)
	assert.JSONEq(t, `{"site":"mteam","torrent_id":"123","downloader_id":2,"category":"movies","tags":"","save_path":""}`, string(body))

	_, isErr, _ = e.call("push_torrent", map[string]any{"torrent_url": "https://hdsky.me/download.php?id=99", "confirm": true})
	require.False(t, isErr)
	body, _ = json.Marshal(e.backend.got()[1].Body)
	assert.JSONEq(t, `{"site":"hdsky","torrent_id":"99","downloader_id":0,"category":"","tags":"","save_path":""}`, string(body))
	a := e.audit.all()
	assert.Equal(t, "hdsky", a[1].Args["site"])
	assert.NotContains(t, a[1].Args, "torrent_url")

	for _, args := range []map[string]any{
		{"torrent_url": "magnet:?xt=urn:btih:abc", "confirm": true},
		{"torrent_url": "https://hdsky.me/download.php?id=1", "site": "hdsky", "confirm": true},
		{"site": "hdsky", "confirm": true},
	} {
		_, isErr, _ = e.call("push_torrent", args)
		assert.True(t, isErr, args)
	}
	assert.Len(t, e.backend.got(), 2, "参数不对的都没有调")
}

// App API 回错误：工具报错并带上原因；写工具的审计记 error:http_<状态码>；回 200 但没成（推送被拦下）时记 App API 给的结果
func TestToolErrors(t *testing.T) {
	e := newEnv(t)
	e.backend.reply = func(req Request) Response {
		if req.Path == "/push" {
			return Response{Status: http.StatusOK, Body: []byte(`{"success":false,"message":"磁盘空间不足"}`), Outcome: "error:push_failed"}
		}
		return Response{Status: http.StatusNotFound, Body: []byte(`{"error":"not_found","message":"没有这个订阅"}`)}
	}
	text, isErr, _ := e.call("add_subscription", map[string]any{"media_type": "movie", "tmdb_id": 438631, "confirm": true})
	assert.True(t, isErr)
	assert.Contains(t, text, "没有这个订阅")
	assert.Equal(t, "error:http_404", e.audit.all()[0].Result)
	assert.EqualValues(t, 438631, e.audit.all()[0].Args["tmdb_id"])

	_, isErr, out := e.call("push_torrent", map[string]any{"site": "hdsky", "torrent_id": "1", "confirm": true})
	assert.False(t, isErr, "业务上的失败照样把回应交给客户端")
	assert.Equal(t, "磁盘空间不足", out["message"])
	assert.Equal(t, "error:push_failed", e.audit.all()[1].Result)

	text, isErr, _ = e.call("add_subscription", map[string]any{"media_type": "movie", "tmdb_id": 0, "confirm": true})
	assert.True(t, isErr)
	assert.Contains(t, text, "tmdb_id")

	e.backend.reply = func(Request) Response { return Response{Status: http.StatusOK, Body: []byte(`not json`)} }
	text, isErr, _ = e.call("list_tasks", nil)
	assert.True(t, isErr)
	assert.Contains(t, text, "JSON")
	e.backend.reply = func(Request) Response { return Response{Status: http.StatusOK, Body: []byte(`"just a string"`)} }
	_, isErr, _ = e.call("list_tasks", nil)
	assert.True(t, isErr, "不是对象也不是数组")
}

// App API 回的是数组时（订阅列表）放进 items 交给客户端（structuredContent 要是对象）
func TestArrayResponse(t *testing.T) {
	e := newEnv(t)
	e.backend.reply = func(Request) Response {
		return Response{Status: http.StatusOK, Body: []byte(`[{"id":1,"title":"沙丘2"}]`)}
	}
	_, isErr, out := e.call("list_subscriptions", nil)
	require.False(t, isErr)
	assert.Equal(t, []any{map[string]any{"id": float64(1), "title": "沙丘2"}}, out["items"])
}

// TokenInfo 与 CallerFromRequest 互转：令牌编号、名字、权限范围；没有 TokenInfo 时取不到
func TestCallerRoundTrip(t *testing.T) {
	c := Caller{TokenID: 9, Name: "cursor", Scopes: []string{apitoken.ScopeMCPRead}}
	ti := TokenInfo(c, time.Time{})
	assert.Equal(t, "token:9", ti.UserID)
	got, ok := CallerFromRequest(&sdk.CallToolRequest{Extra: &sdk.RequestExtra{TokenInfo: ti}})
	require.True(t, ok)
	assert.Equal(t, c, got)
	assert.True(t, got.Has(apitoken.ScopeMCPRead))
	assert.False(t, got.Has(apitoken.ScopeMCPWrite))
	_, ok = CallerFromRequest(&sdk.CallToolRequest{})
	assert.False(t, ok)
	_, ok = CallerFromRequest(nil)
	assert.False(t, ok)
}

// 参数的校验在工具里做：写工具没带 confirm、带了多余的字段、取值不在范围里都不调用 App API，并且记审计；
// 只读工具的参数不对也不调用（不记审计）；会访问站点、GitHub、TMDB 的工具标 openWorld
func TestArgumentValidation(t *testing.T) {
	e := newEnv(t)
	cases := []struct {
		tool   string
		args   map[string]any
		result string
	}{
		{"pause_torrent", map[string]any{"downloader_id": 1, "task_id": "abc"}, "denied:confirm"},
		{"pause_torrent", map[string]any{"downloader_id": 1, "task_id": "abc", "confirm": true, "extra": 1}, "error:invalid_argument"},
		{"delete_torrent", map[string]any{"downloader_id": 0, "task_id": "abc", "confirm": true}, "error:invalid_argument"},
		{"add_subscription", map[string]any{"media_type": "book", "tmdb_id": 1, "confirm": true}, "error:invalid_argument"},
		{"add_subscription", map[string]any{"media_type": "movie", "tmdb_id": -3, "confirm": true}, "error:invalid_argument"},
	}
	for i, c := range cases {
		_, isErr, _ := e.call(c.tool, c.args)
		assert.True(t, isErr, c.args)
		a := e.audit.all()
		require.Len(t, a, i+1, c.args)
		assert.Equal(t, c.result, a[i].Result, c.args)
		assert.Equal(t, c.tool, a[i].Command)
	}
	for _, args := range []map[string]any{
		{"page_size": 500},
		{"state": "flying"},
		{"page": 0},
	} {
		_, isErr, _ := e.call("list_downloader_torrents", args)
		assert.True(t, isErr, args)
	}
	_, isErr, _ := e.call("search_torrents", map[string]any{"keyword": "x", "limit": 1000})
	assert.True(t, isErr)
	assert.Empty(t, e.backend.got(), "参数不对的都没有调用 App API")
	assert.Len(t, e.audit.all(), len(cases), "只读工具参数不对不记审计")

	res, err := e.session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	open := map[string]bool{}
	for _, tool := range res.Tools {
		open[tool.Name] = tool.Annotations.OpenWorldHint != nil && *tool.Annotations.OpenWorldHint
	}
	for _, name := range []string{"search_torrents", "push_torrent", "check_updates", "explore_media", "add_subscription"} {
		assert.True(t, open[name], name)
	}
	for _, name := range []string{"list_tasks", "list_downloader_torrents", "pause_torrent", "delete_torrent", "get_site_userinfo"} {
		assert.False(t, open[name], name)
	}
	schema, _ := json.Marshal(func() any {
		for _, tool := range res.Tools {
			if tool.Name == "list_downloader_torrents" {
				return tool.InputSchema
			}
		}
		return nil
	}())
	assert.Contains(t, string(schema), `"maximum":200`)
}
