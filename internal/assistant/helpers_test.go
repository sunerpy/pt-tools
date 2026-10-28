package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/mcp"
	"github.com/sunerpy/pt-tools/models"
)

func TestMain(m *testing.M) {
	global.GlobalLogger = zap.NewNop()
	m.Run()
}

// passkey 是测试种子地址里的 passkey：不许出现在落库、事件与交给模型的内容里。
const passkey = "PASSKEY0123456789abcdefSECRET"

const torrentURL = "https://hdsky.me/download.php?id=123&passkey=" + passkey

// plainCipher 是测试用的 Cipher：加个前缀，能看出落库的是不是「密文」。
type plainCipher struct{}

func (plainCipher) Encrypt(plain string) (string, error) { return "enc:" + plain, nil }
func (plainCipher) Decrypt(text string) (string, error) {
	if !strings.HasPrefix(text, "enc:") {
		return "", errors.New("不是密文")
	}
	return strings.TrimPrefix(text, "enc:"), nil
}

func resolveURL(raw string) (string, string, bool) {
	if strings.HasPrefix(raw, "https://hdsky.me/download.php?id=123") {
		return "hdsky", "123", true
	}
	return "", "", false
}

// reply 是假模型服务的一次回答。
type reply struct {
	text      string
	reasoning string
	// reasoningKey 是推理内容放在哪个字段（为空是 reasoning_content；OpenRouter、Groq 是 reasoning）
	reasoningKey string
	calls        []replyCall
	// status 不为 0 时回这个状态码，错误信息是 body，回应头加上 header；raw 不为空时原样回它
	status int
	body   string
	header map[string]string
	raw    string
	// block 不为 nil 时先发一段文字，再等它关上或者请求被取消
	block chan struct{}
	// usage 不为零时报这次的输入、输出 token：流式请求带了 stream_options.include_usage 时最后发一片 usage，非流式写在回应里
	usage [2]int64
}

// replyCall 是一个工具调用；extra 不为空时作为 extra_content 放进这个调用的第一个分片（Gemini 的思考签名就在这里）。
type replyCall struct{ id, name, args, extra string }

// llmRequest 是假模型服务收到的一个请求。
type llmRequest struct {
	Model    string           `json:"model"`
	Messages []map[string]any `json:"messages"`
	Tools    []struct {
		Type     string `json:"type"`
		Function struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			Parameters  map[string]any `json:"parameters"`
		} `json:"function"`
	} `json:"tools"`
	Stream          bool   `json:"stream"`
	ReasoningEffort string `json:"reasoning_effort"`
	StreamOptions   *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options"`
	Auth string `json:"-"`
	Raw  string `json:"-"`
}

func (r llmRequest) toolNames() []string {
	var out []string
	for _, t := range r.Tools {
		out = append(out, t.Function.Name)
	}
	return out
}

// fakeLLM 是可编排的 OpenAI 兼容服务：按顺序回放 replies，收到的请求记下来。
type fakeLLM struct {
	t       *testing.T
	srv     *httptest.Server
	mu      sync.Mutex
	replies []reply
	reqs    []llmRequest
	// started 在每个请求开始回答时收到一个值
	started chan struct{}
	// strictArgs 为真时像部分兼容网关那样校验历史里工具调用的参数：不是 JSON 对象就回 400
	strictArgs bool
	// models 是 GET /models 回的模型编号；modelAuths 是这个接口收到的 Authorization
	models     []string
	modelAuths []string
	// modelStatus 不为 0 时 GET /models 回这个状态码
	modelStatus int
	// modelWindows 是 GET /models 里给哪些模型报 context_length（OpenRouter 的写法）
	modelWindows map[string]int
	// noStreamOptions 为真时像不认 stream_options 的兼容服务：带了就回 400（不消耗 replies）
	noStreamOptions bool
}

func newFakeLLM(t *testing.T) *fakeLLM {
	f := &fakeLLM{t: t, started: make(chan struct{}, 64)}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeLLM) strict() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.strictArgs = true
}

// badArgs 是请求历史里参数不是 JSON 对象的工具调用（没有时为空）。
func badArgs(req llmRequest) string {
	for _, m := range req.Messages {
		calls, _ := m["tool_calls"].([]any)
		for _, c := range calls {
			fn, _ := c.(map[string]any)["function"].(map[string]any)
			args, _ := fn["arguments"].(string)
			var obj map[string]any
			if json.Unmarshal([]byte(args), &obj) != nil || obj == nil {
				return args
			}
		}
	}
	return ""
}

func (f *fakeLLM) push(rs ...reply) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replies = append(f.replies, rs...)
}

func (f *fakeLLM) requests() []llmRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]llmRequest(nil), f.reqs...)
}

func chunkJSON(delta map[string]any, finish string) string {
	choice := map[string]any{"index": 0, "delta": delta}
	if finish != "" {
		choice["finish_reason"] = finish
	}
	buf, _ := json.Marshal(map[string]any{
		"id": "chatcmpl-1", "object": "chat.completion.chunk", "created": 1, "model": "fake-model", "choices": []any{choice},
	})
	return string(buf)
}

func (f *fakeLLM) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/models") {
		f.serveModels(w, r)
		return
	}
	if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
		http.NotFound(w, r)
		return
	}
	var req llmRequest
	raw, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(raw, &req)
	req.Auth, req.Raw = r.Header.Get("Authorization"), string(raw)
	f.mu.Lock()
	f.reqs = append(f.reqs, req)
	rejected := f.strictArgs && badArgs(req) != ""
	noOptions := f.noStreamOptions && req.StreamOptions != nil
	var rep reply
	ok := len(f.replies) > 0
	if ok && !rejected && !noOptions {
		rep, f.replies = f.replies[0], f.replies[1:]
	}
	f.mu.Unlock()
	f.started <- struct{}{}
	if rejected {
		http.Error(w, `{"error":{"message":"invalid tool_call arguments JSON"}}`, http.StatusBadRequest)
		return
	}
	if noOptions {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Unrecognized request argument supplied: stream_options","type":"invalid_request_error"}}`))
		return
	}
	if !ok {
		http.Error(w, `{"error":{"message":"no more replies"}}`, http.StatusInternalServerError)
		return
	}
	if rep.status != 0 {
		for k, v := range rep.header {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rep.status)
		if rep.raw != "" {
			_, _ = w.Write([]byte(rep.raw))
			return
		}
		buf, _ := json.Marshal(map[string]any{"error": map[string]any{"message": rep.body, "type": "invalid_request_error"}})
		_, _ = w.Write(buf)
		return
	}
	usage := map[string]any{"prompt_tokens": rep.usage[0], "completion_tokens": rep.usage[1], "total_tokens": rep.usage[0] + rep.usage[1]}
	if !req.Stream {
		w.Header().Set("Content-Type", "application/json")
		out := map[string]any{
			"id": "chatcmpl-1", "object": "chat.completion", "created": 1, "model": "fake-model",
			"choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": rep.text}}},
		}
		if rep.usage != [2]int64{} {
			out["usage"] = usage
		}
		buf, _ := json.Marshal(out)
		_, _ = w.Write(buf)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	fl := w.(http.Flusher)
	send := func(data string) {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		fl.Flush()
	}
	if rep.reasoning != "" {
		key := rep.reasoningKey
		if key == "" {
			key = "reasoning_content"
		}
		// 分两片发，看拼得对不对
		r := []rune(rep.reasoning)
		half := len(r) / 2
		send(chunkJSON(map[string]any{"role": "assistant", key: string(r[:half])}, ""))
		send(chunkJSON(map[string]any{key: string(r[half:]), "content": nil}, ""))
	}
	text := []rune(rep.text)
	for i := 0; i < len(text); i += 5 {
		end := min(i+5, len(text))
		send(chunkJSON(map[string]any{"content": string(text[i:end])}, ""))
		if i == 0 && rep.block != nil {
			select {
			case <-rep.block:
			case <-r.Context().Done():
				return
			}
		}
	}
	for i, c := range rep.calls {
		head := map[string]any{"index": i, "id": c.id, "type": "function", "function": map[string]any{"name": c.name, "arguments": ""}}
		if c.extra != "" {
			head["extra_content"] = json.RawMessage(c.extra)
		}
		send(chunkJSON(map[string]any{"tool_calls": []any{head}}, ""))
		half := len(c.args) / 2
		for _, part := range []string{c.args[:half], c.args[half:]} {
			send(chunkJSON(map[string]any{"tool_calls": []any{map[string]any{"index": i, "function": map[string]any{"arguments": part}}}}, ""))
		}
	}
	finish := "stop"
	if len(rep.calls) > 0 {
		finish = "tool_calls"
	}
	send(chunkJSON(map[string]any{}, finish))
	if rep.usage != [2]int64{} && req.StreamOptions != nil && req.StreamOptions.IncludeUsage {
		buf, _ := json.Marshal(map[string]any{
			"id": "chatcmpl-1", "object": "chat.completion.chunk", "created": 1, "model": "fake-model", "choices": []any{}, "usage": usage,
		})
		send(string(buf))
	}
	send("[DONE]")
}

func (f *fakeLLM) serveModels(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.modelAuths = append(f.modelAuths, r.Header.Get("Authorization"))
	ids, status := append([]string(nil), f.models...), f.modelStatus
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if status != 0 {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":{"message":"Incorrect API key provided: sk-test-key-0123456789abcdef","type":"invalid_request_error"}}`))
		return
	}
	data := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		item := map[string]any{"id": id, "object": "model", "created": 1, "owned_by": "test"}
		f.mu.Lock()
		if w := f.modelWindows[id]; w > 0 {
			item["context_length"] = w
		}
		f.mu.Unlock()
		data = append(data, item)
	}
	buf, _ := json.Marshal(map[string]any{"object": "list", "data": data})
	_, _ = w.Write(buf)
}

// fakeBackend 是假的 App API：记下工具发来的请求，按路径回罐头 JSON。
type fakeBackend struct {
	mu      sync.Mutex
	reqs    []mcp.Request
	callers []mcp.Caller
	// big 为真时 /tasks 回一个很大的结果
	big bool
}

func (b *fakeBackend) Call(_ context.Context, c mcp.Caller, req mcp.Request) (mcp.Response, error) {
	b.mu.Lock()
	b.reqs = append(b.reqs, req)
	b.callers = append(b.callers, c)
	big := b.big
	b.mu.Unlock()
	body := `{"ok":true}`
	switch {
	case strings.HasPrefix(req.Path, "/tasks"):
		body = `{"items":[{"title":"Fake.Movie.2026","site":"hdsky","torrent_id":"123"}],"total":1}`
		if big {
			body = `{"items":[{"title":"` + strings.Repeat("长", 20000) + `"}]}`
		}
	case req.Path == "/sites":
		body = `[{"name":"hdsky","ratio":2.5,"uploaded":"1 TB"}]`
	case req.Path == "/downloaders":
		body = `{"items":[{"id":1,"name":"qb","default":true,"reachable":true,"free_space":107374182400}]}`
	case strings.HasPrefix(req.Path, "/torrents?"):
		// 写工具的预览按任务编号找种子：q 是任务编号时回这一个
		body = `{"items":[]}`
		if u, err := url.Parse(req.Path); err == nil {
			if q := u.Query().Get("q"); q != "" {
				body = fmt.Sprintf(`{"items":[{"downloader_id":1,"downloader":"qb","task_id":%q,"title":"Fake.Movie.2026","progress":1,"size":1073741824,"state":"seeding","ratio":1.2}]}`, q)
			}
		}
	case req.Path == "/torrents/actions":
		body = `{"results":[{"ok":true}]}`
	case req.Path == "/push":
		body = `{"ok":true,"downloader":"qb"}`
	case req.Path == "/brush/tasks/preview":
		body = `{"summary":"暂停刷流任务 9","changes":[{"label":"启用","before":"是","after":"否"}],"impact":["停止自动挑种"],"applied":false}`
	case req.Path == "/config":
		body = `{"resources":[{"name":"downloaders","title":"下载器","description":"下载器","kind":"list","id_field":"id","ops":["add","update","delete"],"fields":["enabled","name","password"],"secrets":["password"]}],"excluded":["登录密码"],"secret_placeholder":"<已设置>"}`
	case strings.HasPrefix(req.Path, "/config/") && strings.HasSuffix(req.Path, "/preview"):
		body = `{"resource":"downloaders","op":"update","id":"1","summary":"改下载器「qb」的 2 项","changes":[{"label":"启用（enabled）","field":"enabled","before":"是","after":"否"},{"label":"密码（password）","field":"password","before":"（不显示）","after":"已更新（不显示）","secret":true}],"impact":["改了地址或账号以后 pt-tools 重新连接这个下载器"],"applied":false}`
	case strings.HasPrefix(req.Path, "/config/") && strings.HasSuffix(req.Path, "/changes"):
		body = `{"resource":"downloaders","op":"update","id":"1","summary":"改下载器「qb」的 2 项","changes":[],"impact":[],"applied":true,"item":{"id":1,"name":"qb","enabled":false}}`
	}
	return mcp.Response{Status: http.StatusOK, Body: []byte(body)}, nil
}

func (b *fakeBackend) requests() []mcp.Request {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]mcp.Request(nil), b.reqs...)
}

// writes 是写接口收到的请求（暂停、继续、删除、推送、订阅）。
func (b *fakeBackend) writes() []mcp.Request {
	var out []mcp.Request
	for _, r := range b.requests() {
		if r.Write {
			out = append(out, r)
		}
	}
	return out
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
	return append([]app.AuditEntry(nil), a.entries...)
}

func newTestDB(t testing.TB) *gorm.DB {
	t.Helper()
	// 测试库不刷盘（同 core.NewTempDBDir）：Windows runner 上每次写都要刷盘，一条 SQL 能慢到几秒
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "assistant.db")+"?_pragma=journal_mode(MEMORY)&_pragma=synchronous(OFF)"), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { closeTestGorm(db) })
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.AssistantSetting{}, &models.AssistantConversation{},
		&models.AssistantMessage{}, &models.AssistantConfirmation{}, &models.AssistantAttachment{}))
	return db
}

type env struct {
	t       *testing.T
	db      *gorm.DB
	svc     *Service
	llm     *fakeLLM
	backend *fakeBackend
	audit   *fakeAudit
	// servers 是建过的 MCP 服务（看会话关没关）
	mu      sync.Mutex
	servers []*sdk.Server
}

const testKey = "sk-test-key-0123456789abcdef"

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{t: t, db: newTestDB(t), llm: newFakeLLM(t), backend: &fakeBackend{}, audit: &fakeAudit{}}
	e.svc = New(Config{
		DB: e.db, Cipher: plainCipher{}, ResolveURL: resolveURL, Version: "test", HTTPClient: e.llm.srv.Client(),
		Tools: func(c mcp.Caller) *sdk.Server {
			srv := mcp.NewServer(mcp.Deps{
				Backend: e.backend, Audit: e.audit, ResolveURL: resolveURL, Channel: mcp.KindAssistant,
				Caller: func(*sdk.CallToolRequest) (mcp.Caller, bool) { return c, true },
			})
			e.mu.Lock()
			e.servers = append(e.servers, srv)
			e.mu.Unlock()
			return srv
		},
	})
	e.svc.confirmUnit = 200 * time.Millisecond
	require.NoError(t, e.svc.Start(context.Background()))
	t.Cleanup(func() {
		// 运行都退出了 Close 就返回；时限要给够：等不到就关库，后台还在写，Windows 上临时目录删不掉
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		_ = e.svc.Close(ctx)
	})
	key := testKey
	_, err := e.svc.SaveSettings(context.Background(), SettingsInput{
		Enabled: true, Provider: "custom", BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model", APIKey: &key,
	})
	require.NoError(t, err)
	return e
}

var admin = Actor{Owner: "web:admin", Entry: models.AssistantEntryWeb, Name: "admin", CanWrite: true, CanConfig: true}

func (e *env) conversation(a Actor) uint {
	e.t.Helper()
	c, err := e.svc.CreateConversation(context.Background(), a, "")
	require.NoError(e.t, err)
	return c.ID
}

func (e *env) send(a Actor, conv uint, text string) SendResult {
	e.t.Helper()
	res, err := e.svc.Send(context.Background(), a, conv, text)
	require.NoError(e.t, err)
	return res
}

// until 读事件直到 match 为真，返回读到的全部事件。
func (e *env) until(a Actor, conv uint, after int64, match func(Event) bool) ([]Event, int64) {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var all []Event
	for time.Now().Before(deadline) {
		page, err := e.svc.Events(context.Background(), a, conv, after, time.Second)
		require.NoError(e.t, err)
		require.False(e.t, page.Reset, "事件不该丢")
		for _, ev := range page.Events {
			all = append(all, ev)
			after = ev.Seq
			if match(ev) {
				return all, after
			}
		}
	}
	e.t.Fatalf("等事件超时，读到 %d 个", len(all))
	return nil, after
}

func pendingConfirmation(ev Event) bool {
	return ev.Type == EventConfirmation && ev.Confirmation.Status == models.AssistantConfirmPending
}

// waitRun 等这次运行结束，返回 run_finished 事件与会话到那时的全部事件。
func (e *env) waitRun(a Actor, conv uint, runID string) (Event, []Event) {
	e.t.Helper()
	all, _ := e.until(a, conv, 0, func(ev Event) bool { return ev.Type == EventRunFinished && ev.RunID == runID })
	return all[len(all)-1], all
}

func (e *env) messages(conv uint) []models.AssistantMessage {
	e.t.Helper()
	var rows []models.AssistantMessage
	require.NoError(e.t, e.db.Where("conversation_id = ?", conv).Order("id").Find(&rows).Error)
	return rows
}

func roles(rows []models.AssistantMessage) []string {
	out := make([]string, 0, len(rows))
	for _, m := range rows {
		out = append(out, m.Role)
	}
	return out
}

func call(id, name, args string) reply {
	return reply{calls: []replyCall{{id: id, name: name, args: args}}}
}
