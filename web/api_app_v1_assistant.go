package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/assistant"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/web/middleware"
)

// App API 的 AI 助手（路线图 M19）：状态、会话、发消息、停止、长轮询事件与确认。设置只在网页上改，这里没有。
// 属主按主体定：网页登录 web:<用户名>，API 令牌 token:<编号>，远程设备 device:<编号>，互相看不到；
// 主体有 app:write 才能确认改动类操作，否则模型拿不到这些工具。

// appAssistantWait 是长轮询事件最多等几秒。
const appAssistantWait = 25

// appAssistantActor 把 App API 的主体换成助手的属主；别的主体（MCP、助手自己）不能用。
func appAssistantActor(p *middleware.Principal) (assistant.Actor, bool) {
	if p == nil {
		return assistant.Actor{}, false
	}
	a := assistant.Actor{Entry: models.AssistantEntryApp, Name: p.Name, CanWrite: p.Has(apitoken.ScopeAppWrite)}
	switch p.Kind {
	case middleware.KindSession:
		a.Owner, a.Name = "web:"+p.ID, p.ID
	case middleware.KindAPIToken:
		a.Owner = "token:" + p.ID
	case middleware.KindRemoteDevice:
		a.Owner = "device:" + p.ID
	default:
		return assistant.Actor{}, false
	}
	return a, true
}

// appAssistant 取服务与属主；不行时已经写了错误。
func (s *Server) appAssistant(w http.ResponseWriter, r *http.Request) (*assistant.Service, assistant.Actor, bool) {
	if s.assistant == nil {
		appError(w, http.StatusServiceUnavailable, "unavailable", "AI 助手服务没有启动")
		return nil, assistant.Actor{}, false
	}
	a, ok := appAssistantActor(middleware.PrincipalFrom(r.Context()))
	if !ok {
		appError(w, http.StatusForbidden, "forbidden", "这个主体不能用 AI 助手")
		return nil, assistant.Actor{}, false
	}
	return s.assistant, a, true
}

// appAssistantError 写助手的错误：错误码与网页接口的状态码一致。
func appAssistantError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, assistant.ErrInvalid):
		appError(w, http.StatusBadRequest, "invalid_argument", err.Error())
	case errors.Is(err, assistant.ErrForbidden):
		appError(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, assistant.ErrNotFound):
		appError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, assistant.ErrDisabled):
		appError(w, http.StatusConflict, "assistant_disabled", err.Error())
	case errors.Is(err, assistant.ErrBusy):
		appError(w, http.StatusConflict, "busy", err.Error())
	case errors.Is(err, assistant.ErrDecided):
		appError(w, http.StatusConflict, "decided", err.Error())
	case errors.Is(err, assistant.ErrTooMany):
		appError(w, http.StatusTooManyRequests, "too_many", err.Error())
	case errors.Is(err, assistant.ErrClosed):
		appError(w, http.StatusServiceUnavailable, "unavailable", err.Error())
	default:
		appError(w, http.StatusInternalServerError, "internal", appRedact(err.Error()))
	}
}

// appJSONBody 写已经设好状态码的 JSON 回应体（201、202 这类）。
func appJSONBody(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) }

// appPathID 读路径里的 {id}（正整数）。
func appPathID(w http.ResponseWriter, r *http.Request) (uint, bool) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		appError(w, http.StatusBadRequest, "invalid_argument", "编号要是正整数")
		return 0, false
	}
	return uint(id), true
}

func (s *Server) appAssistantStatus(w http.ResponseWriter, r *http.Request) {
	svc, a, ok := s.appAssistant(w, r)
	if !ok {
		return
	}
	got, err := svc.Status(r.Context(), a)
	if err != nil {
		appAssistantError(w, err)
		return
	}
	appJSON(w, got)
}

func (s *Server) appAssistantConversations(w http.ResponseWriter, r *http.Request) {
	svc, a, ok := s.appAssistant(w, r)
	if !ok {
		return
	}
	list, err := svc.Conversations(r.Context(), a)
	if err != nil {
		appAssistantError(w, err)
		return
	}
	appJSON(w, list)
}

func (s *Server) appAssistantConversationCreate(w http.ResponseWriter, r *http.Request) {
	svc, a, ok := s.appAssistant(w, r)
	if !ok {
		return
	}
	var in assistantCreateInput
	if !appDecode(w, r, &in) {
		return
	}
	res, status, err := createAndSend(r.Context(), svc, a, in)
	if err != nil {
		appAssistantError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	appJSONBody(w, res)
}

func (s *Server) appAssistantConversation(w http.ResponseWriter, r *http.Request) {
	svc, a, ok := s.appAssistant(w, r)
	if !ok {
		return
	}
	id, ok := appPathID(w, r)
	if !ok {
		return
	}
	got, err := svc.Conversation(r.Context(), a, id)
	if err != nil {
		appAssistantError(w, err)
		return
	}
	appJSON(w, got)
}

func (s *Server) appAssistantConversationDelete(w http.ResponseWriter, r *http.Request) {
	svc, a, ok := s.appAssistant(w, r)
	if !ok {
		return
	}
	id, ok := appPathID(w, r)
	if !ok {
		return
	}
	if err := svc.DeleteConversation(r.Context(), a, id); err != nil {
		appAssistantError(w, err)
		return
	}
	appJSON(w, map[string]bool{"ok": true})
}

func (s *Server) appAssistantSend(w http.ResponseWriter, r *http.Request) {
	svc, a, ok := s.appAssistant(w, r)
	if !ok {
		return
	}
	id, ok := appPathID(w, r)
	if !ok {
		return
	}
	var in assistantSendInput
	if !appDecode(w, r, &in) {
		return
	}
	res, err := svc.Send(r.Context(), a, id, in.Text)
	if err != nil {
		appAssistantError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	appJSONBody(w, res)
}

func (s *Server) appAssistantCancel(w http.ResponseWriter, r *http.Request) {
	svc, a, ok := s.appAssistant(w, r)
	if !ok {
		return
	}
	id, ok := appPathID(w, r)
	if !ok {
		return
	}
	if !appNoBody(w, r) {
		return
	}
	if err := svc.Cancel(r.Context(), a, id); err != nil {
		appAssistantError(w, err)
		return
	}
	appJSON(w, map[string]bool{"ok": true})
}

// appAssistantEvents 是长轮询：after 以后有事件马上回，没有时最多等 wait 秒（默认、上限 25）。
// reset 为真时中间的事件丢了（或者 pt-tools 重启过），重新读会话，从它的 last_seq 接着读。
func (s *Server) appAssistantEvents(w http.ResponseWriter, r *http.Request) {
	svc, a, ok := s.appAssistant(w, r)
	if !ok {
		return
	}
	id, ok := appPathID(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	var after int64
	if v := q.Get("after"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			appError(w, http.StatusBadRequest, "invalid_argument", "after 要是非负整数")
			return
		}
		after = n
	}
	wait := appAssistantWait
	if v := q.Get("wait"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > appAssistantWait {
			appError(w, http.StatusBadRequest, "invalid_argument", "wait 在 0 到 25 秒之间")
			return
		}
		wait = n
	}
	page, err := svc.Events(r.Context(), a, id, after, time.Duration(wait)*time.Second)
	if err != nil {
		appAssistantError(w, err)
		return
	}
	appJSON(w, page)
}

func (s *Server) appAssistantDecide(w http.ResponseWriter, r *http.Request) {
	svc, a, ok := s.appAssistant(w, r)
	if !ok {
		return
	}
	id, ok := appPathID(w, r)
	if !ok {
		return
	}
	var in assistantDecideInput
	if !appDecode(w, r, &in) {
		return
	}
	if in.Approve == nil {
		appError(w, http.StatusBadRequest, "invalid_argument", "要给 approve（true 确认、false 拒绝）")
		return
	}
	got, err := svc.Decide(r.Context(), a, id, *in.Approve)
	if err != nil {
		appAssistantError(w, err)
		return
	}
	if got.Status == models.AssistantConfirmDenied {
		appSetOutcome(r, "denied:user")
	}
	appJSON(w, got)
}
