package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/sunerpy/pt-tools/internal/assistant"
	"github.com/sunerpy/pt-tools/models"
)

// AI 助手（路线图 M19）的网页接口：只认网页登录（s.auth）。设置只在这里改；对话的进度经 SSE 推给页面。
// App 经 App API v1 的 /assistant/*（api_app_v1_assistant.go）用同一个服务，ChatOps 在 cmd 里接。

// SetAssistantService 接上 AI 助手服务（为空时接口回 503）。
func (s *Server) SetAssistantService(svc *assistant.Service) { s.assistant = svc }

func (s *Server) registerAssistantRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/assistant/settings", s.auth(s.apiAssistantSettingsGet))
	mux.HandleFunc("PUT /api/assistant/settings", s.auth(s.apiAssistantSettingsPut))
	mux.HandleFunc("POST /api/assistant/settings/test", s.auth(s.apiAssistantSettingsTest))
	mux.HandleFunc("GET /api/assistant/status", s.auth(s.apiAssistantStatus))
	mux.HandleFunc("GET /api/assistant/conversations", s.auth(s.apiAssistantConversations))
	mux.HandleFunc("POST /api/assistant/conversations", s.auth(s.apiAssistantConversationCreate))
	mux.HandleFunc("GET /api/assistant/conversations/{id}", s.auth(s.apiAssistantConversation))
	mux.HandleFunc("DELETE /api/assistant/conversations/{id}", s.auth(s.apiAssistantConversationDelete))
	mux.HandleFunc("POST /api/assistant/conversations/{id}/messages", s.auth(s.apiAssistantSend))
	mux.HandleFunc("POST /api/assistant/conversations/{id}/cancel", s.auth(s.apiAssistantCancel))
	mux.HandleFunc("GET /api/assistant/conversations/{id}/events", s.auth(s.apiAssistantEvents))
	mux.HandleFunc("POST /api/assistant/confirmations/{id}", s.auth(s.apiAssistantDecide))
}

func (s *Server) assistantService(w http.ResponseWriter) (*assistant.Service, bool) {
	if s != nil && s.assistant != nil {
		return s.assistant, true
	}
	http.Error(w, "AI 助手服务没有启动", http.StatusServiceUnavailable)
	return nil, false
}

// assistantWebActor 是网页登录的用户：属主 web:<用户名>，能确认改动类操作。
func (s *Server) assistantWebActor(w http.ResponseWriter, r *http.Request) (assistant.Actor, bool) {
	user := ""
	if c, err := r.Cookie("session"); err == nil {
		user, _ = s.sessions.lookup(c.Value)
	}
	if user == "" {
		http.Error(w, "session 无效或已过期，请重新登录", http.StatusUnauthorized)
		return assistant.Actor{}, false
	}
	return assistant.Actor{Owner: "web:" + user, Entry: models.AssistantEntryWeb, Name: user, CanWrite: true}, true
}

// aiErrorStatus 是 AI 助手的错误对应的状态码。
func aiErrorStatus(err error) int {
	var te *assistant.TestError
	switch {
	case errors.Is(err, assistant.ErrInvalid):
		return http.StatusBadRequest
	case errors.Is(err, assistant.ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, assistant.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, assistant.ErrDisabled), errors.Is(err, assistant.ErrBusy), errors.Is(err, assistant.ErrDecided):
		return http.StatusConflict
	case errors.Is(err, assistant.ErrTooMany):
		return http.StatusTooManyRequests
	case errors.Is(err, assistant.ErrClosed):
		return http.StatusServiceUnavailable
	case errors.As(err, &te):
		return http.StatusBadGateway
	}
	return http.StatusInternalServerError
}

func writeAIError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), aiErrorStatus(err))
}

func (s *Server) apiAssistantSettingsGet(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.assistantService(w)
	if !ok {
		return
	}
	got, err := svc.Settings(r.Context())
	if err != nil {
		writeAIError(w, err)
		return
	}
	writeJSON(w, got)
}

func (s *Server) apiAssistantSettingsPut(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.assistantService(w)
	if !ok {
		return
	}
	var in assistant.SettingsInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.SaveSettings(r.Context(), in)
	if err != nil {
		writeAIError(w, err)
		return
	}
	writeJSON(w, got)
}

func (s *Server) apiAssistantSettingsTest(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.assistantService(w)
	if !ok {
		return
	}
	var in assistant.SettingsInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.TestSettings(r.Context(), in)
	if err != nil {
		writeAIError(w, err)
		return
	}
	writeJSON(w, got)
}

func (s *Server) apiAssistantStatus(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.assistantService(w)
	if !ok {
		return
	}
	a, ok := s.assistantWebActor(w, r)
	if !ok {
		return
	}
	got, err := svc.Status(r.Context(), a)
	if err != nil {
		writeAIError(w, err)
		return
	}
	writeJSON(w, got)
}

func (s *Server) apiAssistantConversations(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.assistantService(w)
	if !ok {
		return
	}
	a, ok := s.assistantWebActor(w, r)
	if !ok {
		return
	}
	list, err := svc.Conversations(r.Context(), a)
	if err != nil {
		writeAIError(w, err)
		return
	}
	writeJSON(w, list)
}

// assistantCreateInput 是建会话的参数：Message 不为空时建好以后接着发这一条。
type assistantCreateInput struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

// assistantCreateResult 是建会话的结果：带了消息时还有这次运行。
type assistantCreateResult struct {
	Conversation assistant.ConversationView `json:"conversation"`
	Run          *assistant.SendResult      `json:"run,omitempty"`
}

func (s *Server) apiAssistantConversationCreate(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.assistantService(w)
	if !ok {
		return
	}
	a, ok := s.assistantWebActor(w, r)
	if !ok {
		return
	}
	var in assistantCreateInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	res, status, err := createAndSend(r.Context(), svc, a, in)
	if err != nil {
		writeAIError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(res)
}

// createAndSend 建会话，带了消息时接着发（回 202）；网页与 App API 共用。
func createAndSend(ctx context.Context, svc *assistant.Service, a assistant.Actor, in assistantCreateInput) (assistantCreateResult, int, error) {
	title := in.Title
	if title == "" {
		title = in.Message
	}
	conv, err := svc.CreateConversation(ctx, a, title)
	if err != nil {
		return assistantCreateResult{}, 0, err
	}
	res := assistantCreateResult{Conversation: conv}
	if in.Message == "" {
		return res, http.StatusCreated, nil
	}
	sent, err := svc.Send(ctx, a, conv.ID, in.Message)
	if err != nil {
		_ = svc.DeleteConversation(context.WithoutCancel(ctx), a, conv.ID)
		return assistantCreateResult{}, 0, err
	}
	res.Run = &sent
	res.Conversation.Running = true
	return res, http.StatusAccepted, nil
}

func (s *Server) apiAssistantConversation(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.assistantService(w)
	if !ok {
		return
	}
	a, ok := s.assistantWebActor(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	got, err := svc.Conversation(r.Context(), a, id)
	if err != nil {
		writeAIError(w, err)
		return
	}
	writeJSON(w, got)
}

func (s *Server) apiAssistantConversationDelete(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.assistantService(w)
	if !ok {
		return
	}
	a, ok := s.assistantWebActor(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := svc.DeleteConversation(r.Context(), a, id); err != nil {
		writeAIError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// assistantSendInput 是发消息的参数。
type assistantSendInput struct {
	Text string `json:"text"`
}

func (s *Server) apiAssistantSend(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.assistantService(w)
	if !ok {
		return
	}
	a, ok := s.assistantWebActor(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in assistantSendInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	res, err := svc.Send(r.Context(), a, id, in.Text)
	if err != nil {
		writeAIError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) apiAssistantCancel(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.assistantService(w)
	if !ok {
		return
	}
	a, ok := s.assistantWebActor(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := svc.Cancel(r.Context(), a, id); err != nil {
		writeAIError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// assistantDecideInput 是确认或拒绝的参数。
type assistantDecideInput struct {
	Approve *bool `json:"approve"`
}

func (s *Server) apiAssistantDecide(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.assistantService(w)
	if !ok {
		return
	}
	a, ok := s.assistantWebActor(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in assistantDecideInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	if in.Approve == nil {
		http.Error(w, "要给 approve（true 确认、false 拒绝）", http.StatusBadRequest)
		return
	}
	got, err := svc.Decide(r.Context(), a, id, *in.Approve)
	if errors.Is(err, assistant.ErrDecided) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(got)
		return
	}
	if err != nil {
		writeAIError(w, err)
		return
	}
	writeJSON(w, got)
}

// eventsAfter 读续读的位置：查询串 after，或者浏览器重连时带的 Last-Event-ID。
func eventsAfter(r *http.Request) (int64, bool) {
	raw := r.URL.Query().Get("after")
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		raw = v
	}
	if raw == "" {
		return 0, true
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	return n, err == nil && n >= 0
}

// sseHeartbeat 是没有事件时发注释行的间隔（反向代理不会因为闲着断开）。
const sseHeartbeat = 15 * time.Second

// apiAssistantEvents 是会话的事件流（SSE）：每个事件一条 data（JSON），id 是序号；事件丢了时发 type 为 reset 的一条，
// 页面重新读会话再接着读。
func (s *Server) apiAssistantEvents(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.assistantService(w)
	if !ok {
		return
	}
	a, ok := s.assistantWebActor(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	after, ok := eventsAfter(r)
	if !ok {
		http.Error(w, "after 要是非负整数", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	page, err := svc.Events(ctx, a, id, after, 0)
	if err != nil {
		writeAIError(w, err)
		return
	}
	rc := http.NewResponseController(w)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	for {
		if writeSSEPage(w, page) != nil || rc.Flush() != nil {
			return
		}
		after = page.Last
		if page, err = svc.Events(ctx, a, id, after, sseHeartbeat); err != nil || ctx.Err() != nil {
			return
		}
	}
}

// writeSSEPage 写一页事件；没有事件时写一行注释当心跳。
func writeSSEPage(w http.ResponseWriter, page assistant.EventPage) error {
	if page.Reset {
		if _, err := fmt.Fprintf(w, "id: %d\ndata: {\"type\":\"reset\",\"last\":%d}\n\n", page.Last, page.Last); err != nil {
			return err
		}
	}
	for _, ev := range page.Events {
		buf, err := json.Marshal(ev)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "id: %d\ndata: %s\n\n", ev.Seq, buf); err != nil {
			return err
		}
	}
	if len(page.Events) == 0 && !page.Reset {
		_, err := fmt.Fprint(w, ": ping\n\n")
		return err
	}
	return nil
}
