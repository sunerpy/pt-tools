package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/web/middleware"
)

// ==== merged from api_chatops_branches_test.go ====
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

// ==== merged from api_chatops_cov2_test.go ====
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

// ==== merged from api_chatops_cov3_test.go ====
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

// ==== merged from api_chatops_cov_test.go ====
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

// ==== merged from api_chatops_err_cov2_test.go ====
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

// ==== merged from api_chatops_err_test.go ====
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

// ==== merged from api_chatops_rss_cov3_test.go ====
func TestListRSSNotifications_AllFilters(t *testing.T) {
	srv, _, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	setupChatOpsDB(t)
	now := time.Now()
	require.NoError(t, global.GlobalDB.DB.Create(&models.RSSNotificationLog{
		RSSID: 1, SiteName: "hdsky", TorrentID: "t1", NotifyKind: "new",
		NotificationConfID: 5, Result: "pending", NextRetryAt: &now,
	}).Error)

	resp := chatopsReq(t, srv, http.MethodGet,
		"/api/chatops/rss-notifications?rss_id=1&kind=new&result=pending&conf_id=5&page=1&page_size=10",
		tok, nil)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Total int64 `json:"total"`
	}
	decodeBody(t, resp, &body)
	assert.Equal(t, int64(1), body.Total)
}

// ==== merged from api_chatops_rssnotify_cov_test.go ====
func setupChatOpsDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	prev := global.GlobalDB
	global.GlobalDB = &models.TorrentDB{DB: db}
	t.Cleanup(func() { global.GlobalDB = prev })
	require.NoError(t, db.AutoMigrate(&models.RSSNotificationLog{}))
}

func TestListRSSNotifications(t *testing.T) {
	srv, _, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	setupChatOpsDB(t)
	now := time.Now()
	require.NoError(t, global.GlobalDB.DB.Create(&models.RSSNotificationLog{
		RSSID: 1, SiteName: "hdsky", TorrentID: "t1", NotifyKind: "new",
		NotificationConfID: 1, Result: "pending", NextRetryAt: &now,
	}).Error)
	require.NoError(t, global.GlobalDB.DB.Create(&models.RSSNotificationLog{
		RSSID: 2, SiteName: "mteam", TorrentID: "t2", NotifyKind: "new",
		NotificationConfID: 1, Result: "failed",
	}).Error)

	t.Run("list all", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodGet, "/api/chatops/rss-notifications", tok, nil)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var body struct {
			Items []models.RSSNotificationLog `json:"items"`
			Total int64                       `json:"total"`
		}
		decodeBody(t, resp, &body)
		assert.Equal(t, int64(2), body.Total)
	})

	t.Run("filter by result and rss_id", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodGet, "/api/chatops/rss-notifications?result=pending&rss_id=1&page=1&page_size=10", tok, nil)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var body struct {
			Total int64 `json:"total"`
		}
		decodeBody(t, resp, &body)
		assert.Equal(t, int64(1), body.Total)
	})
}

func TestRetryRSSNotification(t *testing.T) {
	srv, _, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	setupChatOpsDB(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.RSSNotificationLog{
		RSSID: 1, SiteName: "hdsky", TorrentID: "t1", NotifyKind: "new",
		NotificationConfID: 1, Result: "failed",
	}).Error)

	t.Run("retry existing row", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodPost, "/api/chatops/rss-notifications/1/retry", tok, nil)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var row models.RSSNotificationLog
		require.NoError(t, global.GlobalDB.DB.First(&row, 1).Error)
		assert.Equal(t, "pending", row.Result)
	})

	t.Run("retry missing row returns 404", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodPost, "/api/chatops/rss-notifications/999/retry", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

func TestCancelRSSNotification(t *testing.T) {
	srv, _, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	setupChatOpsDB(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.RSSNotificationLog{
		RSSID: 1, SiteName: "hdsky", TorrentID: "t1", NotifyKind: "new",
		NotificationConfID: 1, Result: "pending",
	}).Error)
	require.NoError(t, global.GlobalDB.DB.Create(&models.RSSNotificationLog{
		RSSID: 2, SiteName: "mteam", TorrentID: "t2", NotifyKind: "new",
		NotificationConfID: 1, Result: "sent",
	}).Error)

	t.Run("cancel pending row", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodPost, "/api/chatops/rss-notifications/1/cancel", tok, nil)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var row models.RSSNotificationLog
		require.NoError(t, global.GlobalDB.DB.First(&row, 1).Error)
		assert.Equal(t, "suppressed", row.Result)
	})

	t.Run("cancel non-retryable row returns 400", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodPost, "/api/chatops/rss-notifications/2/cancel", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// ==== merged from api_chatops_test.go ====
// ----- stub services -----

type stubNotificationSvc struct {
	listResp     []app.NotificationConfDTO
	listErr      error
	getResp      app.NotificationConfDTO
	getErr       error
	createResp   app.NotificationConfDTO
	createErr    error
	updateErr    error
	deleteErr    error
	testErr      error
	createCalls  int32
	updateCalls  int32
	deleteCalls  int32
	testCalls    int32
	getCalls     int32
	lastCreate   app.CreateConfReq
	lastUpdateID uint
	lastUpdate   app.UpdateConfReq
	lastDeleteID uint
	lastTestID   uint
	lastGetID    uint
}

func (s *stubNotificationSvc) ListConfs(ctx context.Context) ([]app.NotificationConfDTO, error) {
	return s.listResp, s.listErr
}

func (s *stubNotificationSvc) GetConf(ctx context.Context, id uint) (app.NotificationConfDTO, error) {
	atomic.AddInt32(&s.getCalls, 1)
	s.lastGetID = id
	return s.getResp, s.getErr
}

func (s *stubNotificationSvc) CreateConf(ctx context.Context, req app.CreateConfReq) (app.NotificationConfDTO, error) {
	atomic.AddInt32(&s.createCalls, 1)
	s.lastCreate = req
	return s.createResp, s.createErr
}

func (s *stubNotificationSvc) UpdateConf(ctx context.Context, id uint, req app.UpdateConfReq) error {
	atomic.AddInt32(&s.updateCalls, 1)
	s.lastUpdateID = id
	s.lastUpdate = req
	return s.updateErr
}

func (s *stubNotificationSvc) DeleteConf(ctx context.Context, id uint) error {
	atomic.AddInt32(&s.deleteCalls, 1)
	s.lastDeleteID = id
	return s.deleteErr
}

func (s *stubNotificationSvc) TestConf(ctx context.Context, id uint) error {
	atomic.AddInt32(&s.testCalls, 1)
	s.lastTestID = id
	return s.testErr
}

func (s *stubNotificationSvc) Push(ctx context.Context, n app.Notification) error { return nil }
func (s *stubNotificationSvc) PushSync(ctx context.Context, n app.Notification) error {
	return nil
}

func (s *stubNotificationSvc) Enqueue(ctx context.Context, n app.Notification, confID uint) error {
	return nil
}

type stubBindingSvc struct {
	issueResp   app.BindCodeDTO
	issueErr    error
	listResp    []app.BindingDTO
	listErr     error
	revokeErr   error
	setLangErr  error
	issueCalls  int32
	revokeCalls int32
	langCalls   int32
	lastIssue   struct {
		ConfID uint
		Label  string
		TTL    time.Duration
	}
	lastRevokeID uint
	lastLangID   uint
	lastLang     string
}

func (s *stubBindingSvc) IssueCode(ctx context.Context, confID uint, label string, ttl time.Duration) (app.BindCodeDTO, error) {
	atomic.AddInt32(&s.issueCalls, 1)
	s.lastIssue.ConfID = confID
	s.lastIssue.Label = label
	s.lastIssue.TTL = ttl
	return s.issueResp, s.issueErr
}

func (s *stubBindingSvc) ListPendingCodes(ctx context.Context) ([]app.BindCodeDTO, error) {
	return nil, nil
}

func (s *stubBindingSvc) ConsumeCode(ctx context.Context, code, channelType, channelUserID string) (app.BindingDTO, error) {
	return app.BindingDTO{}, nil
}

func (s *stubBindingSvc) ListBindings(ctx context.Context) ([]app.BindingDTO, error) {
	return s.listResp, s.listErr
}

func (s *stubBindingSvc) Revoke(ctx context.Context, id uint) error {
	atomic.AddInt32(&s.revokeCalls, 1)
	s.lastRevokeID = id
	return s.revokeErr
}

func (s *stubBindingSvc) SetReplyLang(ctx context.Context, id uint, lang string) error {
	atomic.AddInt32(&s.langCalls, 1)
	s.lastLangID = id
	s.lastLang = lang
	return s.setLangErr
}

type stubAuditSvc struct {
	queryItems []app.AuditDTO
	queryTotal int
	queryErr   error
	lastQuery  app.AuditQuery
	statsResp  app.AuditStatsDTO
	statsErr   error
	statsCalls int32
}

func (s *stubAuditSvc) Record(ctx context.Context, e app.AuditEntry) error { return nil }

func (s *stubAuditSvc) Query(ctx context.Context, q app.AuditQuery) ([]app.AuditDTO, int, error) {
	s.lastQuery = q
	return s.queryItems, s.queryTotal, s.queryErr
}

func (s *stubAuditSvc) Stats(ctx context.Context) (app.AuditStatsDTO, error) {
	atomic.AddInt32(&s.statsCalls, 1)
	return s.statsResp, s.statsErr
}

func (s *stubAuditSvc) Prune(ctx context.Context) (int64, error) { return 0, nil }

// stubBotTokenStore implements middleware.BotTokenStore + supports our own CRUD methods
// for /api/chatops/tokens endpoints. We use a simple in-memory map keyed by plaintext
// token (test only). Production wiring will use a real store; here we test only the route layer.
type stubBotTokenStore struct {
	tokens   map[uint]*models.BotToken
	plainIdx map[string]*models.BotToken
	nextID   uint
	listResp []TokenDTO
	listErr  error
	createFn func(kind, scope string, ttl time.Duration) (TokenDTO, string, error)
	deleteFn func(id uint) error
}

func newStubBotTokenStore() *stubBotTokenStore {
	return &stubBotTokenStore{
		tokens:   make(map[uint]*models.BotToken),
		plainIdx: make(map[string]*models.BotToken),
	}
}

func (s *stubBotTokenStore) Lookup(ctx context.Context, plain string) (*models.BotToken, error) {
	if t, ok := s.plainIdx[plain]; ok {
		c := *t
		return &c, nil
	}
	return nil, nil
}

func (s *stubBotTokenStore) ListTokens(ctx context.Context) ([]TokenDTO, error) {
	return s.listResp, s.listErr
}

func (s *stubBotTokenStore) CreateToken(ctx context.Context, kind, scope string, ttl time.Duration) (TokenDTO, string, error) {
	if s.createFn != nil {
		return s.createFn(kind, scope, ttl)
	}
	s.nextID++
	id := s.nextID
	plain := fmt.Sprintf("plain-%d", id)
	hash, _ := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.MinCost)
	tk := &models.BotToken{
		ID:              id,
		Kind:            kind,
		CodeOrTokenHash: string(hash),
		Scope:           scope,
		CreatedAt:       time.Now(),
	}
	if ttl > 0 {
		exp := time.Now().Add(ttl)
		tk.ExpiresAt = &exp
	}
	s.tokens[id] = tk
	s.plainIdx[plain] = tk
	return TokenDTO{ID: id, Kind: kind, Scope: scope, CreatedAt: tk.CreatedAt, ExpiresAt: tk.ExpiresAt}, plain, nil
}

func (s *stubBotTokenStore) DeleteToken(ctx context.Context, id uint) error {
	if s.deleteFn != nil {
		return s.deleteFn(id)
	}
	tk, ok := s.tokens[id]
	if !ok {
		return errors.New("not found")
	}
	delete(s.tokens, id)
	for k, v := range s.plainIdx {
		if v == tk {
			delete(s.plainIdx, k)
			break
		}
	}
	return nil
}

// helper: register a valid bearer token returning the plaintext.
func (s *stubBotTokenStore) registerValidToken(scope string) string {
	dto, plain, _ := s.CreateToken(context.Background(), "bearer", scope, time.Hour)
	_ = dto
	return plain
}

// ----- helpers -----

func newTestChatOpsServer(t *testing.T) (*httptest.Server, *ChatOpsDeps, *stubBotTokenStore, func()) {
	t.Helper()
	notif := &stubNotificationSvc{}
	bind := &stubBindingSvc{}
	audit := &stubAuditSvc{}
	store := newStubBotTokenStore()

	deps := &ChatOpsDeps{
		NotificationSvc: notif,
		BindingSvc:      bind,
		AuditSvc:        audit,
		BotTokenStore:   store,
		TokenAdmin:      store,
	}

	mux := http.NewServeMux()
	// Test wrapper: use plain RequireBearer (no session fallback) for clearer 401 semantics.
	requireAuth := middleware.RequireBearer(store)
	RegisterChatOpsRoutes(mux, deps, requireAuth)

	srv := httptest.NewServer(mux)
	return srv, deps, store, srv.Close
}

func chatopsReq(t *testing.T, srv *httptest.Server, method, path, token string, body any) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, srv.URL+path, rdr)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	return resp
}

func decodeBody(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	require.NoError(t, json.NewDecoder(resp.Body).Decode(v))
}

// ----- tests -----

func TestRegisterChatOpsRoutes_AllEndpoints(t *testing.T) {
	srv, _, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	cases := []struct {
		method, path string
		// expected NOT to be 404 (route exists). We don't assert success since stubs may
		// reject input; we just want to confirm the route is registered.
		notStatus int
	}{
		{"GET", "/api/chatops/notifications", http.StatusNotFound},
		{"GET", "/api/chatops/notifications/1", http.StatusNotFound},
		{"POST", "/api/chatops/notifications", http.StatusNotFound},
		{"PUT", "/api/chatops/notifications/1", http.StatusNotFound},
		{"DELETE", "/api/chatops/notifications/1", http.StatusNotFound},
		{"POST", "/api/chatops/notifications/1/test", http.StatusNotFound},
		{"GET", "/api/chatops/bindings", http.StatusNotFound},
		{"POST", "/api/chatops/bindings/issue-code", http.StatusNotFound},
		{"DELETE", "/api/chatops/bindings/1", http.StatusNotFound},
		{"PATCH", "/api/chatops/bindings/1", http.StatusNotFound},
		{"GET", "/api/chatops/audit", http.StatusNotFound},
		{"POST", "/api/chatops/tokens", http.StatusNotFound},
		{"GET", "/api/chatops/tokens", http.StatusNotFound},
		{"DELETE", "/api/chatops/tokens/1", http.StatusNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			resp := chatopsReq(t, srv, tc.method, tc.path, tok, nil)
			defer resp.Body.Close()
			require.NotEqual(t, tc.notStatus, resp.StatusCode,
				"endpoint should be registered: %s %s", tc.method, tc.path)
		})
	}
}

func TestNotifications_HappyPath(t *testing.T) {
	srv, deps, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")
	notif := deps.NotificationSvc.(*stubNotificationSvc)

	notif.listResp = []app.NotificationConfDTO{
		{ID: 1, ChannelType: "telegram", Name: "main"},
	}
	resp := chatopsReq(t, srv, "GET", "/api/chatops/notifications", tok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var listOut []app.NotificationConfDTO
	decodeBody(t, resp, &listOut)
	require.Len(t, listOut, 1)
	require.Equal(t, "telegram", listOut[0].ChannelType)

	notif.createResp = app.NotificationConfDTO{ID: 7, ChannelType: "telegram", Name: "newone", Enabled: true}
	resp = chatopsReq(t, srv, "POST", "/api/chatops/notifications", tok, map[string]any{
		"channel_type": "telegram",
		"name":         "newone",
		"enabled":      true,
		"config_json":  json.RawMessage(`{"bot_token":"abc"}`),
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var createOut app.NotificationConfDTO
	decodeBody(t, resp, &createOut)
	require.Equal(t, uint(7), createOut.ID)
	require.Equal(t, int32(1), atomic.LoadInt32(&notif.createCalls))
	require.Equal(t, "telegram", notif.lastCreate.ChannelType)

	resp = chatopsReq(t, srv, "DELETE", "/api/chatops/notifications/7", tok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, uint(7), notif.lastDeleteID)
}

func TestNotifications_Update_HappyPath(t *testing.T) {
	srv, deps, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")
	notif := deps.NotificationSvc.(*stubNotificationSvc)

	resp := chatopsReq(t, srv, "PUT", "/api/chatops/notifications/3", tok, map[string]any{
		"name":    "renamed",
		"enabled": false,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, uint(3), notif.lastUpdateID)
	require.NotNil(t, notif.lastUpdate.Name)
	require.Equal(t, "renamed", *notif.lastUpdate.Name)
	require.NotNil(t, notif.lastUpdate.Enabled)
	require.False(t, *notif.lastUpdate.Enabled)
}

func TestNotifications_Test_TriggersService(t *testing.T) {
	srv, deps, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")
	notif := deps.NotificationSvc.(*stubNotificationSvc)

	resp := chatopsReq(t, srv, "POST", "/api/chatops/notifications/9/test", tok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, int32(1), atomic.LoadInt32(&notif.testCalls))
	require.Equal(t, uint(9), notif.lastTestID)
}

func TestNotifications_Unauth(t *testing.T) {
	srv, _, _, cleanup := newTestChatOpsServer(t)
	defer cleanup()

	resp := chatopsReq(t, srv, "GET", "/api/chatops/notifications", "", nil)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	resp = chatopsReq(t, srv, "POST", "/api/chatops/notifications", "wrong-token", map[string]any{"channel_type": "x"})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
}

func TestNotifications_InvalidInput(t *testing.T) {
	srv, _, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	// Missing body fields -> 400 (handler validates).
	resp := chatopsReq(t, srv, "POST", "/api/chatops/notifications", tok,
		map[string]any{"channel_type": ""})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()

	// Bad JSON -> 400.
	req, err := http.NewRequest("POST", srv.URL+"/api/chatops/notifications",
		strings.NewReader("{not-json"))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err = srv.Client().Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()

	// Non-numeric id -> 400.
	resp = chatopsReq(t, srv, "DELETE", "/api/chatops/notifications/abc", tok, nil)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}

func TestBindings_IssueCode_HappyPath(t *testing.T) {
	srv, deps, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")
	bind := deps.BindingSvc.(*stubBindingSvc)

	exp := time.Now().Add(5 * time.Minute)
	bind.issueResp = app.BindCodeDTO{
		Code:      "ABCD2345",
		ConfID:    7,
		Label:     "primary",
		ExpiresAt: &exp,
		CreatedAt: time.Now(),
	}
	resp := chatopsReq(t, srv, "POST", "/api/chatops/bindings/issue-code", tok, map[string]any{
		"conf_id": 7,
		"label":   "primary",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out map[string]any
	decodeBody(t, resp, &out)
	require.Equal(t, "ABCD2345", out["code"])
	require.Equal(t, int32(1), atomic.LoadInt32(&bind.issueCalls))
	require.Equal(t, 5*time.Minute, bind.lastIssue.TTL)
}

func TestBindings_IssueCode_DefaultTTL(t *testing.T) {
	srv, deps, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")
	bind := deps.BindingSvc.(*stubBindingSvc)

	exp := time.Now().Add(5 * time.Minute)
	bind.issueResp = app.BindCodeDTO{Code: "X1", ConfID: 1, ExpiresAt: &exp, CreatedAt: time.Now()}

	resp := chatopsReq(t, srv, "POST", "/api/chatops/bindings/issue-code", tok, map[string]any{
		"conf_id": 1,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	require.Equal(t, 5*time.Minute, bind.lastIssue.TTL, "missing ttl_seconds should default to 5 min")
}

func TestBindings_IssueCode_PermanentTTL(t *testing.T) {
	srv, deps, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")
	bind := deps.BindingSvc.(*stubBindingSvc)

	bind.issueResp = app.BindCodeDTO{Code: "PERMA42", ConfID: 1, CreatedAt: time.Now()}

	resp := chatopsReq(t, srv, "POST", "/api/chatops/bindings/issue-code", tok, map[string]any{
		"conf_id":     1,
		"ttl_seconds": 0,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out map[string]any
	decodeBody(t, resp, &out)
	require.Equal(t, "PERMA42", out["code"])
	_, hasExp := out["expires_at"]
	require.False(t, hasExp, "expires_at must be omitted for permanent codes")
	require.Equal(t, time.Duration(0), bind.lastIssue.TTL)
}

func TestBindings_IssueCode_NegativeTTLRejected(t *testing.T) {
	srv, _, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	resp := chatopsReq(t, srv, "POST", "/api/chatops/bindings/issue-code", tok, map[string]any{
		"conf_id":     1,
		"ttl_seconds": -1,
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}

func TestBindings_IssueCode_TTLOverCapRejected(t *testing.T) {
	srv, _, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	resp := chatopsReq(t, srv, "POST", "/api/chatops/bindings/issue-code", tok, map[string]any{
		"conf_id":     1,
		"ttl_seconds": int64(101 * 365 * 24 * 3600),
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}

func TestBindings_List_AndDelete(t *testing.T) {
	srv, deps, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")
	bind := deps.BindingSvc.(*stubBindingSvc)

	bind.listResp = []app.BindingDTO{
		{ID: 1, ChannelType: "telegram", ChannelUserID: "user42"},
	}
	resp := chatopsReq(t, srv, "GET", "/api/chatops/bindings", tok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var listEnvelope struct {
		Bindings []app.BindingDTO  `json:"bindings"`
		Pending  []app.BindCodeDTO `json:"pending"`
	}
	decodeBody(t, resp, &listEnvelope)
	require.Len(t, listEnvelope.Bindings, 1)

	resp = chatopsReq(t, srv, "DELETE", "/api/chatops/bindings/1", tok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	require.Equal(t, uint(1), bind.lastRevokeID)
}

func TestBindings_PatchReplyLang(t *testing.T) {
	srv, deps, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")
	bind := deps.BindingSvc.(*stubBindingSvc)

	resp := chatopsReq(t, srv, "PATCH", "/api/chatops/bindings/4", tok, map[string]any{
		"reply_lang": "en",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, uint(4), bind.lastLangID)
	require.Equal(t, "en", bind.lastLang)
}

func TestBindings_PatchInvalidLang(t *testing.T) {
	srv, deps, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")
	bind := deps.BindingSvc.(*stubBindingSvc)
	bind.setLangErr = app.ErrInvalidReplyLang

	resp := chatopsReq(t, srv, "PATCH", "/api/chatops/bindings/4", tok, map[string]any{
		"reply_lang": "fr",
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}

func TestBindings_Unauth(t *testing.T) {
	srv, _, _, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	resp := chatopsReq(t, srv, "GET", "/api/chatops/bindings", "", nil)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
}

func TestAudit_QueryWithFilter(t *testing.T) {
	srv, deps, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")
	audit := deps.AuditSvc.(*stubAuditSvc)

	// Service is responsible for redacting; the handler must pass through the args_json
	// produced by the service unchanged. We seed a redacted blob and confirm round-trip.
	audit.queryItems = []app.AuditDTO{
		{
			ID:            1,
			Command:       "/login",
			ChannelUserID: "user1",
			ArgsJSON:      `{"token":"[REDACTED]","page":2}`,
			Result:        "ok",
			CreatedAt:     time.Now(),
		},
	}
	audit.queryTotal = 1

	url := "/api/chatops/audit?since=2026-01-01T00:00:00Z&until=2026-12-31T00:00:00Z" +
		"&channel_user_id=user1&command=%2Flogin&page=2&page_size=10"
	resp := chatopsReq(t, srv, "GET", url, tok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var out struct {
		Items []app.AuditDTO `json:"items"`
		Total int            `json:"total"`
	}
	decodeBody(t, resp, &out)
	require.Len(t, out.Items, 1)
	require.Equal(t, 1, out.Total)
	// Payload kept whatever service returned (already redacted).
	require.Contains(t, out.Items[0].ArgsJSON, "[REDACTED]")
	require.NotContains(t, out.Items[0].ArgsJSON, "secret")

	// Filter pushed through to service.
	require.Equal(t, "user1", audit.lastQuery.ChannelUserID)
	require.Equal(t, "/login", audit.lastQuery.Command)
	require.Equal(t, 2, audit.lastQuery.Page)
	require.Equal(t, 10, audit.lastQuery.PageSize)
	require.False(t, audit.lastQuery.Since.IsZero())
	require.False(t, audit.lastQuery.Until.IsZero())
}

func TestAudit_InvalidTime(t *testing.T) {
	srv, _, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	resp := chatopsReq(t, srv, "GET", "/api/chatops/audit?since=not-a-date", tok, nil)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}

func TestAudit_Unauth(t *testing.T) {
	srv, _, _, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	resp := chatopsReq(t, srv, "GET", "/api/chatops/audit", "", nil)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
}

func TestQueryAuditStats_HappyPath(t *testing.T) {
	srv, deps, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")
	audit := deps.AuditSvc.(*stubAuditSvc)

	audit.statsResp = app.AuditStatsDTO{
		TodayCount:   8,
		TotalCount:   17,
		SuccessRate:  70.59,
		MaxLatencyMs: 0,
		AvgLatencyMs: 0,
	}

	resp := chatopsReq(t, srv, "GET", "/api/chatops/audit/stats", tok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var out app.AuditStatsDTO
	decodeBody(t, resp, &out)
	require.EqualValues(t, 8, out.TodayCount)
	require.EqualValues(t, 17, out.TotalCount)
	require.InDelta(t, 70.59, out.SuccessRate, 0.001)
	require.EqualValues(t, 0, out.MaxLatencyMs)
	require.Equal(t, int32(1), atomic.LoadInt32(&audit.statsCalls))
}

func TestQueryAuditStats_Unauth(t *testing.T) {
	srv, _, _, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	resp := chatopsReq(t, srv, "GET", "/api/chatops/audit/stats", "", nil)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
}

func TestTokens_CRUD(t *testing.T) {
	srv, _, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	// Create
	resp := chatopsReq(t, srv, "POST", "/api/chatops/tokens", tok, map[string]any{
		"kind":  "bearer",
		"scope": "chatops:read",
		"ttl_s": 3600,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var created struct {
		Token TokenDTO `json:"token"`
		Plain string   `json:"plaintext"`
	}
	decodeBody(t, resp, &created)
	require.NotEmpty(t, created.Plain)
	require.Equal(t, "bearer", created.Token.Kind)
	require.Equal(t, "chatops:read", created.Token.Scope)

	// Configure list response
	store.listResp = []TokenDTO{created.Token}
	resp = chatopsReq(t, srv, "GET", "/api/chatops/tokens", tok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var listed []TokenDTO
	decodeBody(t, resp, &listed)
	require.GreaterOrEqual(t, len(listed), 1)

	// Delete
	resp = chatopsReq(t, srv, "DELETE", fmt.Sprintf("/api/chatops/tokens/%d", created.Token.ID), tok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

func TestTokens_CreateInvalid(t *testing.T) {
	srv, _, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	resp := chatopsReq(t, srv, "POST", "/api/chatops/tokens", tok, map[string]any{"kind": ""})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}

func TestTokens_Unauth(t *testing.T) {
	srv, _, _, cleanup := newTestChatOpsServer(t)
	defer cleanup()

	resp := chatopsReq(t, srv, "GET", "/api/chatops/tokens", "", nil)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
}

func TestErrorBody_Shape(t *testing.T) {
	// Confirm that 4xx errors return JSON {"error":"...","detail":"..."} pattern.
	srv, _, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	resp := chatopsReq(t, srv, "DELETE", "/api/chatops/notifications/abc", tok, nil)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(body, &m))
	require.NotEmpty(t, m["error"], "error key required in body: %s", string(body))
}

func TestNotifications_NotFound_Maps404(t *testing.T) {
	srv, deps, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")
	notif := deps.NotificationSvc.(*stubNotificationSvc)
	notif.deleteErr = app.ErrConfNotFound

	resp := chatopsReq(t, srv, "DELETE", "/api/chatops/notifications/123", tok, nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

func TestGetNotification_HappyPath(t *testing.T) {
	srv, deps, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")
	notif := deps.NotificationSvc.(*stubNotificationSvc)

	notif.getResp = app.NotificationConfDTO{
		ID:          1,
		ChannelType: "qq_onebot",
		Name:        "t",
		Enabled:     true,
		ConfigJSON:  json.RawMessage(`{"listen_addr":"127.0.0.1:5700","access_token":"abc"}`),
	}

	resp := chatopsReq(t, srv, "GET", "/api/chatops/notifications/1", tok, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var out app.NotificationConfDTO
	decodeBody(t, resp, &out)
	require.Equal(t, uint(1), out.ID)
	require.Equal(t, "qq_onebot", out.ChannelType)
	require.Equal(t, "t", out.Name)
	require.True(t, out.Enabled)
	require.NotEmpty(t, out.ConfigJSON)
	var cfg map[string]any
	require.NoError(t, json.Unmarshal(out.ConfigJSON, &cfg))
	require.Equal(t, "127.0.0.1:5700", cfg["listen_addr"])
	require.Equal(t, "abc", cfg["access_token"])

	require.Equal(t, int32(1), atomic.LoadInt32(&notif.getCalls))
	require.Equal(t, uint(1), notif.lastGetID)
}

func TestGetNotification_NotFound(t *testing.T) {
	srv, deps, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")
	notif := deps.NotificationSvc.(*stubNotificationSvc)
	notif.getErr = app.ErrConfNotFound

	resp := chatopsReq(t, srv, "GET", "/api/chatops/notifications/999", tok, nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

func TestGetNotification_InvalidID(t *testing.T) {
	srv, _, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	resp := chatopsReq(t, srv, "GET", "/api/chatops/notifications/abc", tok, nil)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}

func TestGetNotification_Unauth(t *testing.T) {
	srv, _, _, cleanup := newTestChatOpsServer(t)
	defer cleanup()

	resp := chatopsReq(t, srv, "GET", "/api/chatops/notifications/1", "", nil)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
}

// Sanity check: the expected sentinel (re-export from app pkg) is the same instance.
func TestErrConfNotFoundSentinel(t *testing.T) {
	require.Error(t, app.ErrConfNotFound)
	assert.True(t, errors.Is(app.ErrConfNotFound, app.ErrConfNotFound))
}
