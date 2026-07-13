package web

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sunerpy/pt-tools/internal/app"
)

func TestChatops_MoreErrorBranches(t *testing.T) {
	srv, deps, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")
	notif := deps.NotificationSvc.(*stubNotificationSvc)
	bind := deps.BindingSvc.(*stubBindingSvc)

	t.Run("create notification service error", func(t *testing.T) {
		notif.createErr = errors.New("boom")
		defer func() { notif.createErr = nil }()
		resp := chatopsReq(t, srv, "POST", "/api/chatops/notifications", tok, map[string]any{
			"channel_type": "telegram", "name": "n",
		})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("update notification invalid id", func(t *testing.T) {
		resp := chatopsReq(t, srv, "PUT", "/api/chatops/notifications/abc", tok, map[string]any{"name": "x"})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("update notification service error", func(t *testing.T) {
		notif.updateErr = errors.New("boom")
		defer func() { notif.updateErr = nil }()
		resp := chatopsReq(t, srv, "PUT", "/api/chatops/notifications/1", tok, map[string]any{"name": "x"})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("delete notification invalid id", func(t *testing.T) {
		resp := chatopsReq(t, srv, "DELETE", "/api/chatops/notifications/abc", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("delete notification service error", func(t *testing.T) {
		notif.deleteErr = errors.New("boom")
		defer func() { notif.deleteErr = nil }()
		resp := chatopsReq(t, srv, "DELETE", "/api/chatops/notifications/1", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("test notification invalid id", func(t *testing.T) {
		resp := chatopsReq(t, srv, "POST", "/api/chatops/notifications/abc/test", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("issue bind code missing conf id", func(t *testing.T) {
		resp := chatopsReq(t, srv, "POST", "/api/chatops/bindings/issue-code", tok, map[string]any{})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("issue bind code service error", func(t *testing.T) {
		bind.issueErr = app.ErrTooManyActiveCodes
		defer func() { bind.issueErr = nil }()
		resp := chatopsReq(t, srv, "POST", "/api/chatops/bindings/issue-code", tok, map[string]any{"conf_id": 1})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("patch binding invalid id", func(t *testing.T) {
		resp := chatopsReq(t, srv, "PATCH", "/api/chatops/bindings/abc", tok, map[string]any{"reply_lang": "zh"})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("patch binding no fields", func(t *testing.T) {
		resp := chatopsReq(t, srv, "PATCH", "/api/chatops/bindings/1", tok, map[string]any{})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("patch binding service error", func(t *testing.T) {
		bind.setLangErr = app.ErrInvalidReplyLang
		defer func() { bind.setLangErr = nil }()
		resp := chatopsReq(t, srv, "PATCH", "/api/chatops/bindings/1", tok, map[string]any{"reply_lang": "xx"})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}
