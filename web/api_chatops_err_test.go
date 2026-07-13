package web

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sunerpy/pt-tools/internal/app"
)

func TestChatOpsErrorBranches(t *testing.T) {
	srv, deps, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	t.Run("list bindings error", func(t *testing.T) {
		deps.BindingSvc.(*stubBindingSvc).listErr = app.ErrConfNotFound
		resp := chatopsReq(t, srv, http.MethodGet, "/api/chatops/bindings", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		deps.BindingSvc.(*stubBindingSvc).listErr = nil
	})

	t.Run("audit stats error", func(t *testing.T) {
		deps.AuditSvc.(*stubAuditSvc).statsErr = app.ErrConfNotFound
		resp := chatopsReq(t, srv, http.MethodGet, "/api/chatops/audit/stats", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		deps.AuditSvc.(*stubAuditSvc).statsErr = nil
	})

	t.Run("list tokens error", func(t *testing.T) {
		store.listErr = app.ErrConfNotFound
		resp := chatopsReq(t, srv, http.MethodGet, "/api/chatops/tokens", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		store.listErr = nil
	})

	t.Run("update notification error", func(t *testing.T) {
		deps.NotificationSvc.(*stubNotificationSvc).updateErr = app.ErrConfNotFound
		resp := chatopsReq(t, srv, http.MethodPut, "/api/chatops/notifications/1", tok,
			map[string]any{"name": "x"})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		deps.NotificationSvc.(*stubNotificationSvc).updateErr = nil
	})
}
