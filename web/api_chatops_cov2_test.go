package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
)

func TestChatopsHandlers_NotWiredBranches(t *testing.T) {
	h := &chatopsHandlers{deps: &ChatOpsDeps{}}

	t.Run("createToken no admin store", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/chatops/tokens", nil)
		h.createToken(w, req)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})

	t.Run("listTokens no admin store", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/chatops/tokens", nil)
		h.listTokens(w, req)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})

	t.Run("deleteToken no admin store", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/chatops/tokens/1", nil)
		h.deleteToken(w, req)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})
}

func TestChatopsRSSNotify_NoDB(t *testing.T) {
	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })

	h := &chatopsHandlers{deps: &ChatOpsDeps{}}

	t.Run("list no db", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/chatops/rss-notifications", nil)
		h.listRSSNotifications(w, req)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})

	t.Run("retry no db", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/chatops/rss-notifications/1/retry", nil)
		h.retryRSSNotification(w, req)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})

	t.Run("cancel no db", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/chatops/rss-notifications/1/cancel", nil)
		h.cancelRSSNotification(w, req)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})
}

func TestChatopsRSSNotify_InvalidID(t *testing.T) {
	setupChatOpsDB(t)
	h := &chatopsHandlers{deps: &ChatOpsDeps{}}

	t.Run("retry invalid id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/chatops/rss-notifications//retry", nil)
		h.retryRSSNotification(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("cancel invalid id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/chatops/rss-notifications//cancel", nil)
		h.cancelRSSNotification(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestChatopsDeleteToken_InvalidID(t *testing.T) {
	store := newStubBotTokenStore()
	h := &chatopsHandlers{deps: &ChatOpsDeps{TokenAdmin: store}}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/chatops/tokens/abc", nil)
	h.deleteToken(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRegisterChatOpsIfWired(t *testing.T) {
	t.Run("no deps registers nothing", func(t *testing.T) {
		s := &Server{}
		mux := http.NewServeMux()
		s.registerChatOpsIfWired(mux)
	})

	t.Run("with deps registers routes", func(t *testing.T) {
		s := &Server{sessions: map[string]string{}}
		store := newStubBotTokenStore()
		s.SetChatOpsDeps(&ChatOpsDeps{
			NotificationSvc: &stubNotificationSvc{},
			BindingSvc:      &stubBindingSvc{},
			AuditSvc:        &stubAuditSvc{},
			BotTokenStore:   store,
			TokenAdmin:      store,
		})
		mux := http.NewServeMux()
		s.registerChatOpsIfWired(mux)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/chatops/notifications", nil)
		mux.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code)
	})
}
