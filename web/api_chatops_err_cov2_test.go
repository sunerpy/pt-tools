package web

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/sunerpy/pt-tools/internal/app"
)

func TestChatops_ServiceErrorBranches(t *testing.T) {
	srv, deps, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	notif := deps.NotificationSvc.(*stubNotificationSvc)
	bind := deps.BindingSvc.(*stubBindingSvc)
	audit := deps.AuditSvc.(*stubAuditSvc)

	t.Run("list notifications error", func(t *testing.T) {
		notif.listErr = errors.New("boom")
		defer func() { notif.listErr = nil }()
		resp := chatopsReq(t, srv, "GET", "/api/chatops/notifications", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("get notification not found maps 404", func(t *testing.T) {
		notif.getErr = app.ErrConfNotFound
		defer func() { notif.getErr = nil }()
		resp := chatopsReq(t, srv, "GET", "/api/chatops/notifications/5", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("test notification send failed maps 502", func(t *testing.T) {
		notif.testErr = errors.New("channel down")
		defer func() { notif.testErr = nil }()
		resp := chatopsReq(t, srv, "POST", "/api/chatops/notifications/1/test", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
	})

	t.Run("test notification not found", func(t *testing.T) {
		notif.testErr = app.ErrConfNotFound
		defer func() { notif.testErr = nil }()
		resp := chatopsReq(t, srv, "POST", "/api/chatops/notifications/1/test", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("list bindings error", func(t *testing.T) {
		bind.listErr = errors.New("boom")
		defer func() { bind.listErr = nil }()
		resp := chatopsReq(t, srv, "GET", "/api/chatops/bindings", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("revoke binding error", func(t *testing.T) {
		bind.revokeErr = errors.New("boom")
		defer func() { bind.revokeErr = nil }()
		resp := chatopsReq(t, srv, "DELETE", "/api/chatops/bindings/3", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("revoke binding invalid id", func(t *testing.T) {
		resp := chatopsReq(t, srv, "DELETE", "/api/chatops/bindings/abc", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("query audit error", func(t *testing.T) {
		audit.queryErr = errors.New("boom")
		defer func() { audit.queryErr = nil }()
		resp := chatopsReq(t, srv, "GET", "/api/chatops/audit", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("query audit bad page", func(t *testing.T) {
		resp := chatopsReq(t, srv, "GET", "/api/chatops/audit?page=abc", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("query audit bad until", func(t *testing.T) {
		resp := chatopsReq(t, srv, "GET", "/api/chatops/audit?until=nope", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("audit stats error", func(t *testing.T) {
		audit.statsErr = errors.New("boom")
		defer func() { audit.statsErr = nil }()
		resp := chatopsReq(t, srv, "GET", "/api/chatops/audit/stats", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("create token service error", func(t *testing.T) {
		store.createFn = func(_, _ string, _ time.Duration) (TokenDTO, string, error) {
			return TokenDTO{}, "", errors.New("boom")
		}
		defer func() { store.createFn = nil }()
		resp := chatopsReq(t, srv, "POST", "/api/chatops/tokens", tok, map[string]any{"kind": "bearer"})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("list tokens error", func(t *testing.T) {
		store.listErr = errors.New("boom")
		defer func() { store.listErr = nil }()
		resp := chatopsReq(t, srv, "GET", "/api/chatops/tokens", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("delete token error", func(t *testing.T) {
		store.deleteFn = func(uint) error { return errors.New("boom") }
		defer func() { store.deleteFn = nil }()
		resp := chatopsReq(t, srv, "DELETE", "/api/chatops/tokens/7", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}

var _ = context.Background
