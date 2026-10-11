package web

import (
	"context"
	"encoding/json"
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
	_, err := svc.SaveSettings(context.Background(), assistant.SettingsInput{Enabled: true, BaseURL: srv.URL + "/v1", Model: "fake"})
	require.NoError(t, err)
	e.srv.SetAssistantService(svc)
	return e, llm
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
	llm.push(fakeChatReply{tool: "pause_torrent", args: `{"downloader_id":1,"task_id":"abc"}`}, fakeChatReply{text: "处理完了。"})
	w := e.do(appReq{method: http.MethodGet, path: "/api/app/v1/assistant/status", bearer: tok})
	assert.Contains(t, w.Body.String(), `"can_confirm":true`)

	w = e.do(appReq{method: http.MethodPost, path: "/api/app/v1/assistant/conversations", body: `{"message":"暂停 abc"}`, bearer: tok})
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	var created assistantCreateResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	conv := created.Conversation.ID

	var c *assistant.ConfirmationView
	deadline := time.Now().Add(10 * time.Second)
	var after int64
	for c == nil && time.Now().Before(deadline) {
		w = e.do(appReq{method: http.MethodGet, path: fmt.Sprintf("/api/app/v1/assistant/conversations/%d/events?after=%d&wait=2", conv, after), bearer: tok})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var page assistant.EventPage
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
		for _, ev := range page.Events {
			if ev.Type == assistant.EventConfirmation && ev.Confirmation.Status == models.AssistantConfirmPending {
				c = ev.Confirmation
			}
		}
		after = page.Last
	}
	require.NotNil(t, c, "要等到待确认")
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
		case a.ChannelType == "assistant" && a.Command == "pause_torrent":
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
