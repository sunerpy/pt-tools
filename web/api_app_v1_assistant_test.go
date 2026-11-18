package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/assistant"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/web/middleware"
)

// newAppAssistantEnv 是接好 AI 助手（假的大模型服务）的 App API 环境；e.handler 是完整的路由。
func newAppAssistantEnv(t *testing.T) (*appEnv, *fakeChatLLM) {
	t.Helper()
	e := newAppEnv(t)
	llm := &fakeChatLLM{}
	srv := httptest.NewServer(llm)
	t.Cleanup(srv.Close)
	svc := assistant.New(assistant.Config{
		DB: global.GlobalDB.DB, Cipher: apiTestCipher{}, Tools: e.srv.AssistantToolServer, HTTPClient: srv.Client(),
	})
	require.NoError(t, svc.Start(context.Background()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = svc.Close(ctx)
	})
	// 也配上语音输入：App API 的状态里不回它（识别接口只认网页登录），见 TestAppAssistant_ReadOnlyTokenChats
	voiceURL, voiceToken := "http://127.0.0.1:47840/v1", "voltip-app-test-000000"
	_, err := svc.SaveSettings(context.Background(), assistant.SettingsInput{
		Enabled: true, BaseURL: srv.URL + "/v1", Model: "fake", VoiceBaseURL: &voiceURL, VoiceToken: &voiceToken,
	})
	require.NoError(t, err)
	e.srv.SetAssistantService(svc)
	return e, llm
}

// waitPending 长轮询事件直到出现待确认，返回它。
func waitPending(t *testing.T, e *appEnv, bearer string, conv uint) *assistant.ConfirmationView {
	t.Helper()
	var after int64
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		w := e.do(appReq{method: http.MethodGet, path: fmt.Sprintf("/api/app/v1/assistant/conversations/%d/events?after=%d&wait=2", conv, after), bearer: bearer})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var page assistant.EventPage
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
		for _, ev := range page.Events {
			if ev.Type == assistant.EventConfirmation && ev.Confirmation.Status == models.AssistantConfirmPending {
				return ev.Confirmation
			}
		}
		after = page.Last
	}
	t.Fatal("等待确认超时")
	return nil
}

// pollRun 长轮询事件直到这次运行结束，返回读到的事件。
func pollRun(t *testing.T, e *appEnv, bearer string, conv uint, runID string) []assistant.Event {
	t.Helper()
	var all []assistant.Event
	var after int64
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		w := e.do(appReq{method: http.MethodGet, path: fmt.Sprintf("/api/app/v1/assistant/conversations/%d/events?after=%d&wait=2", conv, after), bearer: bearer})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assertNoSecrets(t, w.Body.Bytes())
		var page assistant.EventPage
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
		require.False(t, page.Reset)
		for _, ev := range page.Events {
			all = append(all, ev)
			if ev.Type == assistant.EventRunFinished && ev.RunID == runID {
				return all
			}
		}
		after = page.Last
	}
	t.Fatal("等运行结束超时")
	return nil
}

func TestAppAssistant_ReadOnlyTokenChats(t *testing.T) {
	e, llm := newAppAssistantEnv(t)
	tok := e.token("app:read")
	llm.push(fakeChatReply{tool: "list_tasks", args: `{}`}, fakeChatReply{text: "没有抓到种子。"})

	w := e.do(appReq{method: http.MethodGet, path: "/api/app/v1/assistant/status", bearer: tok})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"enabled":true,"model":"fake","can_confirm":false,"chatops":false}`, w.Body.String())

	w = e.do(appReq{method: http.MethodPost, path: "/api/app/v1/assistant/conversations", body: `{"message":"最近抓了什么"}`, bearer: tok})
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	var created assistantCreateResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.NotNil(t, created.Run)
	assert.Equal(t, models.AssistantEntryApp, created.Conversation.Entry)
	events := pollRun(t, e, tok, created.Conversation.ID, created.Run.RunID)
	assert.Equal(t, assistant.RunDone, events[len(events)-1].Status)

	w = e.do(appReq{method: http.MethodGet, path: fmt.Sprintf("/api/app/v1/assistant/conversations/%d", created.Conversation.ID), bearer: tok})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	var d assistant.ConversationDetail
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &d))
	require.Len(t, d.Messages, 4)
	assert.Equal(t, "没有抓到种子。", d.Messages[3].Content)

	// 只有 app:read：模型拿不到改动类工具
	llm.mu.Lock()
	first := llm.bodies[0]
	llm.mu.Unlock()
	assert.Contains(t, first, `"list_tasks"`)
	assert.NotContains(t, first, `"pause_torrent"`)
	assert.NotContains(t, first, `"push_torrent"`)

	// 再发一条、停止、列表
	llm.push(fakeChatReply{text: "好的"})
	w = e.do(appReq{method: http.MethodPost, path: fmt.Sprintf("/api/app/v1/assistant/conversations/%d/messages", created.Conversation.ID), body: `{"text":"谢谢"}`, bearer: tok})
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	var sent assistant.SendResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sent))
	pollRun(t, e, tok, created.Conversation.ID, sent.RunID)
	w = e.do(appReq{method: http.MethodPost, path: fmt.Sprintf("/api/app/v1/assistant/conversations/%d/cancel", created.Conversation.ID), bearer: tok})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = e.do(appReq{method: http.MethodGet, path: "/api/app/v1/assistant/conversations", bearer: tok})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var list []assistant.ConversationView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list, 1)

	// 只读令牌不能确认（路由要 app:write）
	w = e.do(appReq{method: http.MethodPost, path: "/api/app/v1/assistant/confirmations/1", body: `{"approve":true}`, bearer: tok})
	assert.Equal(t, http.StatusForbidden, w.Code)

	w = e.do(appReq{method: http.MethodDelete, path: fmt.Sprintf("/api/app/v1/assistant/conversations/%d", created.Conversation.ID), bearer: tok})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestAppAssistant_WriteTokenConfirms(t *testing.T) {
	e, llm := newAppAssistantEnv(t)
	tok := e.token("app:read", "app:write")
	// 写工具建确认前先取预览（读现状）；添加订阅的预览只看参数，这里不用准备下载器
	llm.push(fakeChatReply{tool: "add_subscription", args: `{"media_type":"movie","tmdb_id":438631}`}, fakeChatReply{text: "处理完了。"})
	w := e.do(appReq{method: http.MethodGet, path: "/api/app/v1/assistant/status", bearer: tok})
	assert.Contains(t, w.Body.String(), `"can_confirm":true`)

	w = e.do(appReq{method: http.MethodPost, path: "/api/app/v1/assistant/conversations", body: `{"message":"暂停 abc"}`, bearer: tok})
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	var created assistantCreateResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	conv := created.Conversation.ID

	c := waitPending(t, e, tok, conv)
	path := "/api/app/v1/assistant/confirmations/" + strconv.Itoa(int(c.ID))
	w = e.do(appReq{method: http.MethodPost, path: path, body: `{}`, bearer: tok})
	assert.Equal(t, http.StatusBadRequest, w.Code, "要给 approve")
	w = e.do(appReq{method: http.MethodPost, path: path, body: `{"approve":true}`, bearer: tok})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"status":"approved"`)
	w = e.do(appReq{method: http.MethodPost, path: path, body: `{"approve":false}`, bearer: tok})
	require.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, "decided", errorCode(t, w))
	pollRun(t, e, tok, conv, created.Run.RunID)

	var toolAudit, routeAudit bool
	for _, a := range e.audit.all() {
		switch {
		case a.ChannelType == "assistant" && a.Command == "add_subscription":
			toolAudit = true
			assert.True(t, strings.HasPrefix(a.ChannelUserID, "token:"), a.ChannelUserID)
		case a.ChannelType == middleware.KindAPIToken && strings.Contains(a.Command, "/assistant/confirmations/"):
			routeAudit = true
		}
	}
	assert.True(t, toolAudit, "工具调用记在 assistant 通道，属主是令牌")
	assert.True(t, routeAudit, "确认是写请求，按 App API 的规则记审计")
}

func TestAppAssistant_OwnersAreIsolated(t *testing.T) {
	e, llm := newAppAssistantEnv(t)
	a, b := e.token("app:read"), e.token("app:read")
	llm.push(fakeChatReply{text: "a 的回答"})
	w := e.do(appReq{method: http.MethodPost, path: "/api/app/v1/assistant/conversations", body: `{"title":"a 的会话"}`, bearer: a})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created assistantCreateResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	path := fmt.Sprintf("/api/app/v1/assistant/conversations/%d", created.Conversation.ID)
	for _, q := range []appReq{
		{method: http.MethodGet, path: path, bearer: b},
		{method: http.MethodDelete, path: path, bearer: b},
		{method: http.MethodPost, path: path + "/messages", body: `{"text":"偷看"}`, bearer: b},
		{method: http.MethodGet, path: path + "/events?wait=0", bearer: b},
		{method: http.MethodGet, path: path, session: true},
	} {
		w = e.do(q)
		assert.Equal(t, http.StatusNotFound, w.Code, "%s %s", q.method, q.path)
	}
	w = e.do(appReq{method: http.MethodGet, path: "/api/app/v1/assistant/conversations", bearer: b})
	assert.JSONEq(t, `[]`, w.Body.String())
}

func TestAppAssistant_ArgumentsAndAvailability(t *testing.T) {
	e, _ := newAppAssistantEnv(t)
	tok := e.token("app:read")
	for _, q := range []struct{ path, want string }{
		{"/api/app/v1/assistant/conversations/0", "invalid_argument"},
		{"/api/app/v1/assistant/conversations/1/events?after=-1", "invalid_argument"},
		{"/api/app/v1/assistant/conversations/1/events?wait=26", "invalid_argument"},
	} {
		w := e.do(appReq{method: http.MethodGet, path: q.path, bearer: tok})
		assert.Equal(t, http.StatusBadRequest, w.Code, q.path)
		assert.Equal(t, q.want, errorCode(t, w))
	}
	w := e.do(appReq{method: http.MethodPost, path: "/api/app/v1/assistant/conversations", body: `{"unknown":1}`, bearer: tok})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	e.srv.SetAssistantService(nil)
	w = e.do(appReq{method: http.MethodGet, path: "/api/app/v1/assistant/status", bearer: tok})
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestAppAssistant_BusyAndDenyAreReported(t *testing.T) {
	e, llm := newAppAssistantEnv(t)
	tok := e.token("app:read", "app:write")
	llm.push(fakeChatReply{tool: "add_subscription", args: `{"media_type":"movie","tmdb_id":438631}`}, fakeChatReply{text: "好的，不订阅。"})
	w := e.do(appReq{method: http.MethodPost, path: "/api/app/v1/assistant/conversations", body: `{"message":"暂停 abc"}`, bearer: tok})
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	var created assistantCreateResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	conv := created.Conversation.ID
	c := waitPending(t, e, tok, conv)

	// 运行在等确认：同一个会话再发一条回 409 busy
	w = e.do(appReq{method: http.MethodPost, path: fmt.Sprintf("/api/app/v1/assistant/conversations/%d/messages", conv), body: `{"text":"再问一句"}`, bearer: tok})
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Equal(t, "busy", errorCode(t, w))

	w = e.do(appReq{method: http.MethodPost, path: "/api/app/v1/assistant/confirmations/" + strconv.Itoa(int(c.ID)), body: `{"approve":false}`, bearer: tok})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"status":"denied"`)
	events := pollRun(t, e, tok, conv, created.Run.RunID)
	assert.Equal(t, assistant.RunDone, events[len(events)-1].Status)

	var denied bool
	for _, a := range e.audit.all() {
		if a.ChannelType == middleware.KindAPIToken && strings.Contains(a.Command, "/assistant/confirmations/") {
			denied = true
			assert.Equal(t, "denied:user", a.Result, "拒绝记成 denied:user，不是 success")
		}
		assert.False(t, a.ChannelType == "assistant" && a.Command == "add_subscription", "拒绝以后不执行")
	}
	assert.True(t, denied)
}

// 每个助手接口都先认服务与主体：MCP 主体回 403、服务没启动回 503；编号、请求体不对回 400；会话或确认不存在回 404。
func TestAppAssistant_EveryRouteGuards(t *testing.T) {
	e, _ := newAppAssistantEnv(t)
	tok := e.token("app:read", "app:write")
	var routes []appRoute
	for _, rt := range e.srv.appRoutes() {
		if strings.HasPrefix(rt.Path, "/assistant/") {
			routes = append(routes, rt)
		}
	}
	require.Len(t, routes, 9)
	for _, rt := range routes {
		r := httptest.NewRequest(rt.Method, appPrefix+rt.Path, strings.NewReader(`{}`))
		r.SetPathValue("id", "1")
		r = r.WithContext(middleware.WithPrincipal(r.Context(), &middleware.Principal{Kind: middleware.KindMCP, ID: "3", Scopes: []string{"app:read", "app:write"}}))
		w := httptest.NewRecorder()
		rt.Handler(w, r)
		assert.Equal(t, http.StatusForbidden, w.Code, "%s %s", rt.Method, rt.Path)
	}

	for _, q := range []struct {
		method, path, body string
		status             int
		code               string
	}{
		{http.MethodGet, "/assistant/conversations/x", "", http.StatusBadRequest, "invalid_argument"},
		{http.MethodDelete, "/assistant/conversations/x", "", http.StatusBadRequest, "invalid_argument"},
		{http.MethodPost, "/assistant/conversations/x/messages", `{"text":"hi"}`, http.StatusBadRequest, "invalid_argument"},
		{http.MethodPost, "/assistant/conversations/x/cancel", "", http.StatusBadRequest, "invalid_argument"},
		{http.MethodGet, "/assistant/conversations/x/events", "", http.StatusBadRequest, "invalid_argument"},
		{http.MethodPost, "/assistant/confirmations/x", `{"approve":true}`, http.StatusBadRequest, "invalid_argument"},
		{http.MethodPost, "/assistant/conversations/1/messages", `{"txt":"hi"}`, http.StatusBadRequest, "invalid_body"},
		{http.MethodPost, "/assistant/conversations/1/cancel", `{"x":1}`, http.StatusBadRequest, "invalid_body"},
		{http.MethodPost, "/assistant/confirmations/1", `[]`, http.StatusBadRequest, "invalid_body"},
		{http.MethodDelete, "/assistant/conversations/999", "", http.StatusNotFound, "not_found"},
		{http.MethodPost, "/assistant/conversations/999/messages", `{"text":"hi"}`, http.StatusNotFound, "not_found"},
		{http.MethodPost, "/assistant/conversations/999/cancel", "", http.StatusNotFound, "not_found"},
		{http.MethodGet, "/assistant/conversations/999/events?wait=0", "", http.StatusNotFound, "not_found"},
		{http.MethodPost, "/assistant/confirmations/999", `{"approve":true}`, http.StatusNotFound, "not_found"},
	} {
		w := e.do(appReq{method: q.method, path: appPrefix + q.path, body: q.body, bearer: tok})
		assert.Equal(t, q.status, w.Code, "%s %s: %s", q.method, q.path, w.Body.String())
		assert.Equal(t, q.code, errorCode(t, w), "%s %s", q.method, q.path)
	}

	// 关掉以后：状态说没启用；带消息建会话回 409，不留下空会话
	_, err := e.srv.assistant.SaveSettings(context.Background(), assistant.SettingsInput{})
	require.NoError(t, err)
	w := e.do(appReq{method: http.MethodGet, path: "/api/app/v1/assistant/status", bearer: tok})
	assert.JSONEq(t, `{"enabled":false,"model":"","can_confirm":false,"chatops":false}`, w.Body.String())
	w = e.do(appReq{method: http.MethodPost, path: "/api/app/v1/assistant/conversations", body: `{"message":"在吗"}`, bearer: tok})
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Equal(t, "assistant_disabled", errorCode(t, w))
	w = e.do(appReq{method: http.MethodGet, path: "/api/app/v1/assistant/conversations", bearer: tok})
	assert.JSONEq(t, `[]`, w.Body.String())

	e.srv.SetAssistantService(nil)
	for _, rt := range routes {
		w := e.do(appReq{method: rt.Method, path: appPrefix + strings.ReplaceAll(rt.Path, "{id}", "1"), bearer: tok})
		assert.Equal(t, http.StatusServiceUnavailable, w.Code, "%s %s", rt.Method, rt.Path)
		assert.Equal(t, "unavailable", errorCode(t, w))
	}
}

func TestAppAssistantError(t *testing.T) {
	for _, c := range []struct {
		err    error
		status int
		code   string
	}{
		{fmt.Errorf("%w: 太长", assistant.ErrInvalid), http.StatusBadRequest, "invalid_argument"},
		{assistant.ErrForbidden, http.StatusForbidden, "forbidden"},
		{assistant.ErrNotFound, http.StatusNotFound, "not_found"},
		{assistant.ErrDisabled, http.StatusConflict, "assistant_disabled"},
		{assistant.ErrBusy, http.StatusConflict, "busy"},
		{assistant.ErrDecided, http.StatusConflict, "decided"},
		{assistant.ErrTooMany, http.StatusTooManyRequests, "too_many"},
		{assistant.ErrClosed, http.StatusServiceUnavailable, "unavailable"},
		{errors.New("读 https://tracker.example.com/announce.php?passkey=0123456789abcdef0123456789abcdef 失败"), http.StatusInternalServerError, "internal"},
	} {
		w := httptest.NewRecorder()
		appAssistantError(w, c.err)
		assert.Equal(t, c.status, w.Code, c.err.Error())
		assert.Equal(t, c.code, errorCode(t, w))
		assertNoSecrets(t, w.Body.Bytes())
		assert.NotContains(t, w.Body.String(), "0123456789abcdef0123456789abcdef")
	}
}

func TestAppAssistantActor(t *testing.T) {
	cases := []struct {
		p        *middleware.Principal
		owner    string
		canWrite bool
		ok       bool
	}{
		{&middleware.Principal{Kind: middleware.KindSession, ID: "admin"}, "web:admin", true, true},
		{&middleware.Principal{Kind: middleware.KindAPIToken, ID: "7", Scopes: []string{"app:read"}}, "token:7", false, true},
		{&middleware.Principal{Kind: middleware.KindRemoteDevice, ID: "2", Scopes: []string{"app:read", "app:write"}}, "device:2", true, true},
		{&middleware.Principal{Kind: middleware.KindMCP, ID: "3", Scopes: []string{"app:read"}}, "", false, false},
		{&middleware.Principal{Kind: middleware.KindAssistant, ID: "web:admin", Scopes: []string{"app:read"}}, "", false, false},
		{nil, "", false, false},
	}
	for _, c := range cases {
		a, ok := appAssistantActor(c.p)
		assert.Equal(t, c.ok, ok)
		assert.Equal(t, c.owner, a.Owner)
		assert.Equal(t, c.canWrite, a.CanWrite)
		if ok {
			assert.Equal(t, models.AssistantEntryApp, a.Entry)
		}
	}
}

// App API 不给推理内容的增量（装着旧版本的 App 认不得新的事件种类），推理内容在消息里；模型与推理等级照收
func TestAppAssistant_ReasoningOnlyInMessages(t *testing.T) {
	e, llm := newAppAssistantEnv(t)
	tok := e.token("app:read")
	llm.push(fakeChatReply{text: "你好。", reasoning: "用户在打招呼"})
	w := e.do(appReq{method: http.MethodPost, path: "/api/app/v1/assistant/conversations", body: `{"message":"你好","model":"fake-2","effort":"medium"}`, bearer: tok})
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	var created assistantCreateResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.NotNil(t, created.Run)
	assert.Equal(t, "fake-2", created.Run.Message.Model)
	events := pollRun(t, e, tok, created.Conversation.ID, created.Run.RunID)
	for _, ev := range events {
		assert.NotEqual(t, assistant.EventReasoning, ev.Type, "App API 不给 reasoning 事件")
	}
	llm.mu.Lock()
	body := llm.bodies[len(llm.bodies)-1]
	llm.mu.Unlock()
	assert.Contains(t, body, `"model":"fake-2"`)
	assert.Contains(t, body, `"reasoning_effort":"medium"`)
	w = e.do(appReq{method: http.MethodGet, path: fmt.Sprintf("/api/app/v1/assistant/conversations/%d", created.Conversation.ID), bearer: tok})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var d assistant.ConversationDetail
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &d))
	require.Len(t, d.Messages, 2)
	assert.Equal(t, "用户在打招呼", d.Messages[1].Reasoning)
	assert.Equal(t, "medium", d.Messages[1].Effort)

	w = e.do(appReq{method: http.MethodPost, path: fmt.Sprintf("/api/app/v1/assistant/conversations/%d/messages", created.Conversation.ID), body: `{"text":"x","attachment_ids":[1]}`, bearer: tok})
	assert.Equal(t, http.StatusBadRequest, w.Code, "App API 不收附件")
	w = e.do(appReq{method: http.MethodPost, path: fmt.Sprintf("/api/app/v1/assistant/conversations/%d/messages", created.Conversation.ID), body: `{"text":"x","effort":"max"}`, bearer: tok})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
