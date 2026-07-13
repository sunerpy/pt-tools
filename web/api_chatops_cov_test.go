package web

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/app"
)

func TestTestNotification_Cov(t *testing.T) {
	srv, deps, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")
	notif := deps.NotificationSvc.(*stubNotificationSvc)

	t.Run("success", func(t *testing.T) {
		notif.testErr = nil
		resp := chatopsReq(t, srv, http.MethodPost, "/api/chatops/notifications/1/test", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("not found", func(t *testing.T) {
		notif.testErr = app.ErrConfNotFound
		resp := chatopsReq(t, srv, http.MethodPost, "/api/chatops/notifications/1/test", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("send failure", func(t *testing.T) {
		notif.testErr = errors.New("network down")
		resp := chatopsReq(t, srv, http.MethodPost, "/api/chatops/notifications/1/test", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
	})
}

func TestRevokeBinding_Cov(t *testing.T) {
	srv, deps, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")
	bind := deps.BindingSvc.(*stubBindingSvc)

	t.Run("success", func(t *testing.T) {
		bind.revokeErr = nil
		resp := chatopsReq(t, srv, http.MethodDelete, "/api/chatops/bindings/5", tok, nil)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, uint(5), bind.lastRevokeID)
	})

	t.Run("service error maps", func(t *testing.T) {
		bind.revokeErr = app.ErrConfNotFound
		resp := chatopsReq(t, srv, http.MethodDelete, "/api/chatops/bindings/6", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

func TestDeleteToken_Cov(t *testing.T) {
	srv, _, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	created, _, err := store.CreateToken(nil, "bearer", "chatops:*", 0) //nolint:staticcheck
	require.NoError(t, err)

	t.Run("delete existing", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodDelete, "/api/chatops/tokens/"+itoaUint(created.ID), tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("delete missing maps error", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodDelete, "/api/chatops/tokens/9999", tok, nil)
		defer resp.Body.Close()
		assert.NotEqual(t, http.StatusOK, resp.StatusCode)
	})
}

func TestListTokens_Cov(t *testing.T) {
	srv, _, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	resp := chatopsReq(t, srv, http.MethodGet, "/api/chatops/tokens", tok, nil)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestAuditStats_Cov(t *testing.T) {
	srv, _, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	resp := chatopsReq(t, srv, http.MethodGet, "/api/chatops/audit/stats", tok, nil)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func itoaUint(n uint) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
