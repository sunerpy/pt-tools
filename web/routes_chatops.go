package web

import (
	"net/http"

	"github.com/sunerpy/pt-tools/internal/app"
)

type ChatOpsDeps struct {
	NotificationSvc app.NotificationService
	BindingSvc      app.BindingService
	AuditSvc        app.AuditService
}

func RegisterChatOpsRoutes(mux *http.ServeMux, deps *ChatOpsDeps, requireAuth func(http.Handler) http.Handler) {
	if mux == nil || deps == nil || requireAuth == nil {
		panic("RegisterChatOpsRoutes: nil mux, deps, or requireAuth")
	}
	h := &chatopsHandlers{deps: deps}

	wrap := func(f http.HandlerFunc) http.Handler {
		return requireAuth(f)
	}

	mux.Handle("GET /api/chatops/notifications", wrap(h.listNotifications))
	mux.Handle("GET /api/chatops/notifications/{id}", wrap(h.getNotification))
	mux.Handle("POST /api/chatops/notifications", wrap(h.createNotification))
	mux.Handle("PUT /api/chatops/notifications/{id}", wrap(h.updateNotification))
	mux.Handle("DELETE /api/chatops/notifications/{id}", wrap(h.deleteNotification))
	mux.Handle("POST /api/chatops/notifications/{id}/test", wrap(h.testNotification))

	mux.Handle("GET /api/chatops/bindings", wrap(h.listBindings))
	mux.Handle("POST /api/chatops/bindings/issue-code", wrap(h.issueBindCode))
	mux.Handle("DELETE /api/chatops/bindings/{id}", wrap(h.revokeBinding))
	mux.Handle("PATCH /api/chatops/bindings/{id}", wrap(h.patchBinding))

	mux.Handle("GET /api/chatops/audit", wrap(h.queryAudit))
	mux.Handle("GET /api/chatops/audit/stats", wrap(h.auditStats))

	mux.Handle("GET /api/chatops/rss-notifications", wrap(h.listRSSNotifications))
	mux.Handle("POST /api/chatops/rss-notifications/{id}/retry", wrap(h.retryRSSNotification))
	mux.Handle("POST /api/chatops/rss-notifications/{id}/cancel", wrap(h.cancelRSSNotification))
}

func (s *Server) SetChatOpsDeps(deps *ChatOpsDeps) {
	s.chatopsDeps = deps
}

// registerChatOpsIfWired 注册 ChatOps 的管理接口：和其他 /api/* 一样只认 session（通知详情会解密通道凭证，
// API 令牌与远程设备都不能访问，见路线图 M12）。
func (s *Server) registerChatOpsIfWired(mux *http.ServeMux) {
	if s.chatopsDeps == nil {
		return
	}
	RegisterChatOpsRoutes(mux, s.chatopsDeps, func(h http.Handler) http.Handler { return s.auth(h.ServeHTTP) })
}

func (s *Server) sessionChecker(_ http.ResponseWriter, r *http.Request) bool {
	sid, err := r.Cookie("session")
	if err != nil || sid.Value == "" {
		return false
	}
	return s.sessions.valid(sid.Value)
}
