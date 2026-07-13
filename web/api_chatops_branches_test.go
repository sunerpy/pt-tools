package web

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sunerpy/pt-tools/internal/app"
)

func TestChatOpsValidationBranches(t *testing.T) {
	srv, deps, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	t.Run("create notification missing channel_type", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodPost, "/api/chatops/notifications", tok,
			map[string]any{"name": "x"})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("create notification missing name", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodPost, "/api/chatops/notifications", tok,
			map[string]any{"channel_type": "telegram"})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("issue code missing conf_id", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodPost, "/api/chatops/bindings/issue-code", tok,
			map[string]any{"label": "x"})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("patch binding no fields", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodPatch, "/api/chatops/bindings/1", tok, map[string]any{})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("create token missing kind", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodPost, "/api/chatops/tokens", tok,
			map[string]any{"scope": "x"})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("query audit bad since", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodGet, "/api/chatops/audit?since=not-a-time", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("query audit bad page", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodGet, "/api/chatops/audit?page=abc", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("list notifications error maps", func(t *testing.T) {
		deps.NotificationSvc.(*stubNotificationSvc).listErr = app.ErrConfNotFound
		resp := chatopsReq(t, srv, http.MethodGet, "/api/chatops/notifications", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		deps.NotificationSvc.(*stubNotificationSvc).listErr = nil
	})

	t.Run("get notification error maps", func(t *testing.T) {
		deps.NotificationSvc.(*stubNotificationSvc).getErr = app.ErrConfNotFound
		resp := chatopsReq(t, srv, http.MethodGet, "/api/chatops/notifications/1", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		deps.NotificationSvc.(*stubNotificationSvc).getErr = nil
	})

	t.Run("query audit ok", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodGet, "/api/chatops/audit?page=1&page_size=10", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("list bindings ok", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodGet, "/api/chatops/bindings", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("issue code ok", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodPost, "/api/chatops/bindings/issue-code", tok,
			map[string]any{"conf_id": 1, "label": "l"})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("create token ok", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodPost, "/api/chatops/tokens", tok,
			map[string]any{"kind": "bearer", "scope": "chatops:*", "ttl_s": 3600})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}
