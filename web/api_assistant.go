package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
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
	mux.HandleFunc("POST /api/assistant/settings/models", s.auth(s.apiAssistantSettingsModels))
	mux.HandleFunc("POST /api/assistant/settings/voice-test", s.auth(s.apiAssistantVoiceTest))
	mux.HandleFunc("POST /api/assistant/transcribe", s.auth(s.apiAssistantTranscribe))
	mux.HandleFunc("GET /api/assistant/status", s.auth(s.apiAssistantStatus))
	mux.HandleFunc("GET /api/assistant/models", s.auth(s.apiAssistantModels))
	mux.HandleFunc("POST /api/assistant/attachments", s.auth(s.apiAssistantUpload))
	mux.HandleFunc("GET /api/assistant/attachments/{id}", s.auth(s.apiAssistantAttachment))
	mux.HandleFunc("DELETE /api/assistant/attachments/{id}", s.auth(s.apiAssistantAttachmentDelete))
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
	return assistant.Actor{Owner: "web:" + user, Entry: models.AssistantEntryWeb, Name: user, CanWrite: true, CanConfig: true}, true
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

// apiAssistantVoiceTest 按输入（还没保存的也行）读一次 Voltip 的模型列表，看语音输入的地址与令牌能不能用。令牌的规则同 API Key。
func (s *Server) apiAssistantVoiceTest(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.assistantService(w)
	if !ok {
		return
	}
	var in assistant.SettingsInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.TestVoice(r.Context(), in)
	if err != nil {
		writeAIError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiAssistantTranscribe 把一段录音（请求体是 WAV，最多 MaxVoiceBytes）交给 Voltip 识别，回 {"text": "…"}。
// ?language= 不为空时用它。只认网页登录：访问令牌只在后端。
func (s *Server) apiAssistantTranscribe(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.assistantService(w)
	if !ok {
		return
	}
	wav, err := io.ReadAll(http.MaxBytesReader(w, r.Body, assistant.MaxVoiceBytes))
	if err != nil {
		http.Error(w, "录音太长：最多约 10 分钟", http.StatusRequestEntityTooLarge)
		return
	}
	text, err := svc.Transcribe(r.Context(), wav, r.URL.Query().Get("language"))
	switch {
	case errors.Is(err, assistant.ErrVoiceDisabled):
		http.Error(w, err.Error(), http.StatusConflict)
		return
	case errors.Is(err, assistant.ErrInvalid):
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]string{"text": text})
}

// apiAssistantSettingsModels 按输入读服务商的模型列表（GET {接口地址}/models），给设置页的模型下拉框。Key 的规则同测试连接。
func (s *Server) apiAssistantSettingsModels(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.assistantService(w)
	if !ok {
		return
	}
	var in assistant.SettingsInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	got, err := svc.ListModels(r.Context(), in)
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

// apiAssistantModels 是对话页的模型下拉框：设置里的模型在第一个，后面是服务商列出的（读不到时 error 写明原因）。
func (s *Server) apiAssistantModels(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.assistantService(w)
	if !ok {
		return
	}
	if _, ok := s.assistantWebActor(w, r); !ok {
		return
	}
	got, err := svc.ChatModels(r.Context())
	if err != nil {
		writeAIError(w, err)
		return
	}
	writeJSON(w, got)
}

// apiAssistantUpload 收一个附件（multipart 的 file 字段）：图片或文本文件，传了还没发的只有自己看得到。
func (s *Server) apiAssistantUpload(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.assistantService(w)
	if !ok {
		return
	}
	a, ok := s.assistantWebActor(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, assistant.MaxUploadBytes)
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "要用 multipart/form-data 上传（字段名 file）", http.StatusBadRequest)
		return
	}
	for {
		part, err := mr.NextPart()
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				http.Error(w, "文件太大：图片最大 5 MB，文本文件最大 200 KB", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, "没有收到文件（字段名 file）", http.StatusBadRequest)
			return
		}
		if part.FormName() != "file" {
			_ = part.Close()
			continue
		}
		data, err := io.ReadAll(part)
		_ = part.Close()
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				http.Error(w, "文件太大：图片最大 5 MB，文本文件最大 200 KB", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, "读取上传的文件失败", http.StatusBadRequest)
			return
		}
		got, err := svc.Upload(r.Context(), a, part.FileName(), data)
		if err != nil {
			writeAIError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(got)
		return
	}
}

// apiAssistantAttachment 回一个附件的内容：图片按原类型在页面上显示，文本文件按纯文本下载。
// 只有白名单里的类型，不让浏览器猜类型，回应在沙箱里（就算内容是 HTML 也不会当网页跑）。
func (s *Server) apiAssistantAttachment(w http.ResponseWriter, r *http.Request) {
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
	att, err := svc.Attachment(r.Context(), a, id)
	if err != nil {
		writeAIError(w, err)
		return
	}
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'; img-src 'self'")
	h.Set("Cache-Control", "private, max-age=86400")
	disposition := "inline"
	if att.Kind == models.AssistantAttachImage {
		h.Set("Content-Type", att.MIME)
	} else {
		h.Set("Content-Type", "text/plain; charset=utf-8")
		disposition = "attachment"
	}
	if cd := mime.FormatMediaType(disposition, map[string]string{"filename": att.Name}); cd != "" {
		h.Set("Content-Disposition", cd)
	} else {
		h.Set("Content-Disposition", disposition)
	}
	_, _ = w.Write(att.Data)
}

func (s *Server) apiAssistantAttachmentDelete(w http.ResponseWriter, r *http.Request) {
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
	if err := svc.DeleteAttachment(r.Context(), a, id); err != nil {
		writeAIError(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
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

// assistantCreateInput 是建会话的参数（网页与 App API 共用）：Message 不为空时建好以后接着发这一条，Model、Effort 同发消息。
type assistantCreateInput struct {
	Title   string `json:"title"`
	Message string `json:"message"`
	Model   string `json:"model,omitempty"`
	Effort  string `json:"effort,omitempty"`
}

// assistantWebCreateInput 是网页建会话的参数：还可以带附件（先 POST /api/assistant/attachments 传）。
type assistantWebCreateInput struct {
	assistantCreateInput
	AttachmentIDs []uint `json:"attachment_ids,omitempty"`
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
	var in assistantWebCreateInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	res, status, err := createAndSend(r.Context(), svc, a, in.assistantCreateInput, in.AttachmentIDs)
	if err != nil {
		writeAIError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(res)
}

// createAndSend 建会话，带了消息或附件时接着发（回 202）；网页与 App API 共用（App API 不带附件）。
func createAndSend(ctx context.Context, svc *assistant.Service, a assistant.Actor, in assistantCreateInput, attachments []uint) (assistantCreateResult, int, error) {
	title := in.Title
	if title == "" {
		title = in.Message
	}
	conv, err := svc.CreateConversation(ctx, a, title)
	if err != nil {
		return assistantCreateResult{}, 0, err
	}
	res := assistantCreateResult{Conversation: conv}
	if in.Message == "" && len(attachments) == 0 {
		return res, http.StatusCreated, nil
	}
	sent, err := svc.SendWith(ctx, a, conv.ID, assistant.SendOptions{
		Text: in.Message, Model: in.Model, Effort: in.Effort, Attachments: attachments,
	})
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

// assistantSendInput 是发消息的参数（网页与 App API 共用）：Model 为空用设置里的模型，Effort 为空不指定推理等级。
type assistantSendInput struct {
	Text   string `json:"text"`
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
}

func (in assistantSendInput) options(attachments []uint) assistant.SendOptions {
	return assistant.SendOptions{Text: in.Text, Model: in.Model, Effort: in.Effort, Attachments: attachments}
}

// assistantWebSendInput 是网页发消息的参数：还可以带附件。
type assistantWebSendInput struct {
	assistantSendInput
	AttachmentIDs []uint `json:"attachment_ids,omitempty"`
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
	var in assistantWebSendInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	res, err := svc.SendWith(r.Context(), a, id, in.options(in.AttachmentIDs))
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
