package web

import (
	"context"
	"net/http"
	"time"

	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/web/middleware"
)

type TokenDTO struct {
	ID        uint       `json:"id"`
	Kind      string     `json:"kind"`
	Scope     string     `json:"scope"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type TokenAdminStore interface {
	ListTokens(ctx context.Context) ([]TokenDTO, error)
	CreateToken(ctx context.Context, kind, scope string, ttl time.Duration) (TokenDTO, string, error)
	DeleteToken(ctx context.Context, id uint) error
}

type ChatOpsDeps struct {
	NotificationSvc app.NotificationService
	BindingSvc      app.BindingService
	AuditSvc        app.AuditService
	BotTokenStore   middleware.BotTokenStore
	TokenAdmin      TokenAdminStore
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

	mux.Handle("POST /api/chatops/tokens", wrap(h.createToken))
	mux.Handle("GET /api/chatops/tokens", wrap(h.listTokens))
	mux.Handle("DELETE /api/chatops/tokens/{id}", wrap(h.deleteToken))
}

func (s *Server) SetChatOpsDeps(deps *ChatOpsDeps) {
	s.chatopsDeps = deps
}

func (s *Server) registerChatOpsIfWired(mux *http.ServeMux) {
	if s.chatopsDeps == nil {
		return
	}
	requireAuth := middleware.RequireAuth(s.chatopsDeps.BotTokenStore, s.sessionChecker)
	RegisterChatOpsRoutes(mux, s.chatopsDeps, requireAuth)
}

func (s *Server) sessionChecker(_ http.ResponseWriter, r *http.Request) bool {
	sid, err := r.Cookie("session")
	if err != nil || sid.Value == "" {
		return false
	}
	return s.sessions[sid.Value] != ""
}
