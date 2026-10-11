package web

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/assistant"
	"github.com/sunerpy/pt-tools/internal/mcp"
	"github.com/sunerpy/pt-tools/models"
)

// fakeChatLLM 是假的 OpenAI 兼容服务：按顺序回放回答（文字，或者一个工具调用），收到的请求体记下来。
type fakeChatLLM struct {
	mu      sync.Mutex
	replies []fakeChatReply
	bodies  []string
}

type fakeChatReply struct {
	text       string
	tool, args string
}

func (f *fakeChatLLM) push(r ...fakeChatReply) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replies = append(f.replies, r...)
}

func (f *fakeChatLLM) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.bodies = append(f.bodies, string(body))
	var rep fakeChatReply
	if len(f.replies) > 0 {
		rep, f.replies = f.replies[0], f.replies[1:]
	} else {
		rep.text = "没有更多回答了"
	}
	f.mu.Unlock()
	if !strings.Contains(string(body), `"stream":true`) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"x","object":"chat.completion","created":1,"model":"fake","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":%q}}]}`, rep.text)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	chunk := func(delta string) {
		_, _ = fmt.Fprintf(w, `data: {"id":"x","object":"chat.completion.chunk","created":1,"model":"fake","choices":[{"index":0,"delta":%s}]}`+"\n\n", delta)
		w.(http.Flusher).Flush()
	}
	if rep.tool != "" {
		args, _ := json.Marshal(rep.args)
		chunk(fmt.Sprintf(`{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":%q,"arguments":%s}}]}`, rep.tool, args))
	} else {
		text, _ := json.Marshal(rep.text)
		chunk(fmt.Sprintf(`{"content":%s}`, text))
	}
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
}

type assistantEnv struct {
	t     *testing.T
	srv   *Server
	http  *httptest.Server
	llm   *fakeChatLLM
	audit *fakeAppAudit
}

func newAssistantEnv(t *testing.T) *assistantEnv {
	t.Helper()
	srv := setupServer(t)
	srv.mgr.StopAll()
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.APIToken{}))
	e := &assistantEnv{t: t, srv: srv, llm: &fakeChatLLM{}, audit: &fakeAppAudit{}}
	srv.SetAPITokens(apitoken.New(global.GlobalDB.DB), e.audit)
	llm := httptest.NewServer(e.llm)
	t.Cleanup(llm.Close)
	svc := assistant.New(assistant.Config{
		DB: global.GlobalDB.DB, Cipher: apiTestCipher{}, Tools: srv.AssistantToolServer, HTTPClient: llm.Client(),
	})
	require.NoError(t, svc.Start(context.Background()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = svc.Close(ctx)
	})
	srv.SetAssistantService(svc)
	srv.sessions.put("sess-test", "admin")
	e.http = httptest.NewServer(srv.buildHandler())
	t.Cleanup(e.http.Close)
	w := e.do(http.MethodPut, "/api/assistant/settings", fmt.Sprintf(
		`{"enabled":true,"provider":"custom","base_url":%q,"model":"fake","api_key":"sk-web-test-0000000000"}`, llm.URL+"/v1",
	))
	require.Equal(t, http.StatusOK, w.StatusCode)
	return e
}

func (e *assistantEnv) do(method, path, body string) *http.Response {
	e.t.Helper()
	req, err := http.NewRequest(method, e.http.URL+path, strings.NewReader(body))
	require.NoError(e.t, err)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: "sess-test"})
	res, err := http.DefaultClient.Do(req)
	require.NoError(e.t, err)
	e.t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

func readJSON[T any](t *testing.T, res *http.Response) T {
	t.Helper()
	var v T
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(body, &v), string(body))
	return v
}

// stream 读会话的 SSE，直到 stop 为真，返回读到的事件。
func (e *assistantEnv) stream(conv uint, lastEventID string, stop func(assistant.Event) bool) []assistant.Event {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/assistant/conversations/%d/events", e.http.URL, conv), nil)
	require.NoError(e.t, err)
	req.AddCookie(&http.Cookie{Name: "session", Value: "sess-test"})
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	res, err := http.DefaultClient.Do(req)
	require.NoError(e.t, err)
	defer res.Body.Close()
	require.Equal(e.t, http.StatusOK, res.StatusCode)
	assert.Equal(e.t, "text/event-stream", res.Header.Get("Content-Type"))
	var out []assistant.Event
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	id := ""
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "id: "):
			id = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "data: "):
			var ev assistant.Event
			require.NoError(e.t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev))
			assert.Equal(e.t, strconv.FormatInt(ev.Seq, 10), id, "id 是序号")
			out = append(out, ev)
			if stop(ev) {
				return out
			}
		}
	}
	e.t.Fatalf("事件流提前结束: %v", sc.Err())
	return nil
}

func TestAssistantAPI_RequiresSessionAndService(t *testing.T) {
	srv := setupServer(t)
	srv.mgr.StopAll()
	mux := http.NewServeMux()
	srv.registerAssistantRoutes(mux)
	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/assistant/settings"},
		{http.MethodPut, "/api/assistant/settings"},
		{http.MethodPost, "/api/assistant/settings/test"},
		{http.MethodGet, "/api/assistant/status"},
		{http.MethodGet, "/api/assistant/conversations"},
		{http.MethodPost, "/api/assistant/conversations"},
		{http.MethodGet, "/api/assistant/conversations/1"},
		{http.MethodDelete, "/api/assistant/conversations/1"},
		{http.MethodPost, "/api/assistant/conversations/1/messages"},
		{http.MethodPost, "/api/assistant/conversations/1/cancel"},
		{http.MethodGet, "/api/assistant/conversations/1/events"},
		{http.MethodPost, "/api/assistant/confirmations/1"},
	}
	for _, rt := range routes {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(rt.method, rt.path, nil))
		assert.Equal(t, http.StatusUnauthorized, w.Code, rt.path)
	}
	srv.sessions.put("sess-test", "admin")
	for _, rt := range routes {
		assert.Equal(t, http.StatusServiceUnavailable, serveAuthed(mux, rt.method, rt.path, "{}").Code, rt.path)
	}
}

func TestAssistantAPI_SettingsKeyIsWriteOnly(t *testing.T) {
	e := newAssistantEnv(t)
	res := e.do(http.MethodGet, "/api/assistant/settings", "")
	require.Equal(t, http.StatusOK, res.StatusCode)
	got := readJSON[map[string]any](t, res)
	assert.Equal(t, true, got["has_api_key"])
	assert.NotContains(t, got, "api_key")
	body, _ := json.Marshal(got)
	assert.NotContains(t, string(body), "sk-web-test")

	res = e.do(http.MethodPut, "/api/assistant/settings", `{"enabled":true,"base_url":"ftp://x","model":"m"}`)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	res = e.do(http.MethodPut, "/api/assistant/settings", `{"unknown":1}`)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "不收多余的字段")

	e.llm.push(fakeChatReply{text: "OK"})
	res = e.do(http.MethodPost, "/api/assistant/settings/test", fmt.Sprintf(`{"base_url":%q,"model":"fake"}`, got["base_url"]))
	require.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "OK", readJSON[assistant.TestResult](t, res).Reply)

	res = e.do(http.MethodGet, "/api/assistant/status", "")
	require.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, assistant.Status{Enabled: true, Model: "fake", CanConfirm: true}, readJSON[assistant.Status](t, res))
}

func TestAssistantAPI_ChatWithToolsAndConfirmation(t *testing.T) {
	e := newAssistantEnv(t)
	e.llm.push(fakeChatReply{tool: "list_tasks", args: `{}`}, fakeChatReply{text: "现在没有抓到的种子。"})
	res := e.do(http.MethodPost, "/api/assistant/conversations", `{"message":"最近抓了什么种子"}`)
	require.Equal(t, http.StatusAccepted, res.StatusCode)
	created := readJSON[assistantCreateResult](t, res)
	require.NotNil(t, created.Run)
	conv := created.Conversation.ID
	assert.Equal(t, "最近抓了什么种子", created.Conversation.Title)

	events := e.stream(conv, "", func(ev assistant.Event) bool { return ev.Type == assistant.EventRunFinished })
	var types []string
	for _, ev := range events {
		types = append(types, ev.Type)
	}
	assert.Contains(t, types, assistant.EventToolStarted)
	assert.Equal(t, assistant.RunDone, events[len(events)-1].Status)

	res = e.do(http.MethodGet, fmt.Sprintf("/api/assistant/conversations/%d", conv), "")
	require.Equal(t, http.StatusOK, res.StatusCode)
	d := readJSON[assistant.ConversationDetail](t, res)
	var roles []string
	for _, m := range d.Messages {
		roles = append(roles, m.Role)
	}
	assert.Equal(t, []string{"user", "assistant", "tool", "assistant"}, roles)
	assert.Contains(t, d.Messages[2].Content, `"items"`, "工具经进程内的 App API 执行")
	assert.False(t, d.Messages[2].IsError, d.Messages[2].Content)

	// 写操作：等确认，确认一次算数，第二次回 409
	e.llm.push(fakeChatReply{tool: "pause_torrent", args: `{"downloader_id":1,"task_id":"abc"}`}, fakeChatReply{text: "处理完了。"})
	res = e.do(http.MethodPost, fmt.Sprintf("/api/assistant/conversations/%d/messages", conv), `{"text":"暂停 abc"}`)
	require.Equal(t, http.StatusAccepted, res.StatusCode)
	sent := readJSON[assistant.SendResult](t, res)
	res = e.do(http.MethodPost, fmt.Sprintf("/api/assistant/conversations/%d/messages", conv), `{"text":"再来一个"}`)
	assert.Equal(t, http.StatusConflict, res.StatusCode, "同一个会话同时只有一次运行")

	last := strconv.FormatInt(events[len(events)-1].Seq, 10)
	pending := e.stream(conv, last, func(ev assistant.Event) bool {
		return ev.Type == assistant.EventConfirmation && ev.Confirmation.Status == models.AssistantConfirmPending
	})
	for _, ev := range pending {
		assert.Greater(t, ev.Seq, events[len(events)-1].Seq, "按 Last-Event-ID 续读")
	}
	c := pending[len(pending)-1].Confirmation
	res = e.do(http.MethodPost, fmt.Sprintf("/api/assistant/confirmations/%d", c.ID), `{}`)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "要给 approve")
	res = e.do(http.MethodPost, fmt.Sprintf("/api/assistant/confirmations/%d", c.ID), `{"approve":true}`)
	require.Equal(t, http.StatusOK, res.StatusCode)
	res = e.do(http.MethodPost, fmt.Sprintf("/api/assistant/confirmations/%d", c.ID), `{"approve":false}`)
	require.Equal(t, http.StatusConflict, res.StatusCode)
	assert.Equal(t, models.AssistantConfirmApproved, readJSON[assistant.ConfirmationView](t, res).Status)

	e.stream(conv, last, func(ev assistant.Event) bool {
		return ev.Type == assistant.EventRunFinished && ev.RunID == sent.RunID
	})
	var found bool
	for _, a := range e.audit.all() {
		if a.Command == "pause_torrent" {
			found = true
			assert.Equal(t, mcp.KindAssistant, a.ChannelType)
			assert.Equal(t, "web:admin", a.ChannelUserID)
		}
	}
	assert.True(t, found, "写工具记进操作审计，通道 assistant，属主是网页用户")

	res = e.do(http.MethodDelete, fmt.Sprintf("/api/assistant/conversations/%d", conv), "")
	require.Equal(t, http.StatusOK, res.StatusCode)
	res = e.do(http.MethodGet, fmt.Sprintf("/api/assistant/conversations/%d", conv), "")
	assert.Equal(t, http.StatusNotFound, res.StatusCode)
	res = e.do(http.MethodGet, "/api/assistant/conversations", "")
	assert.Empty(t, readJSON[[]assistant.ConversationView](t, res))
}

func TestAssistantAPI_Disabled(t *testing.T) {
	e := newAssistantEnv(t)
	res := e.do(http.MethodPut, "/api/assistant/settings", `{"enabled":false,"base_url":"https://x","model":"m"}`)
	require.Equal(t, http.StatusOK, res.StatusCode)
	res = e.do(http.MethodPost, "/api/assistant/conversations", `{"message":"你好"}`)
	assert.Equal(t, http.StatusConflict, res.StatusCode)
	body, _ := io.ReadAll(res.Body)
	assert.Contains(t, string(body), "没有启用")
	res = e.do(http.MethodGet, "/api/assistant/conversations", "")
	assert.Empty(t, readJSON[[]assistant.ConversationView](t, res), "发不出去的消息不留空会话")
}
