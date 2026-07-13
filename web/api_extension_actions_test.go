package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/extension"
	"github.com/sunerpy/pt-tools/models"
)

// ==== merged from api_extension_actions_cov_test.go ====
func TestApiExtensionActionsRouter_Dispatch(t *testing.T) {
	srv, cleanup := newExtensionActionTestServer(t)
	defer cleanup()

	require.NoError(t, extension.Enqueue(global.GlobalDB.DB, extension.PendingAction{
		Type:      extension.ActionOpenTab,
		TargetURL: "https://hdsky.me/",
		SiteName:  "hdsky",
	}))

	t.Run("empty rest", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiExtensionActionsRouter(rec, authedRequest(http.MethodPost, "/api/extension/actions/", nil))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("non-ack suffix", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiExtensionActionsRouter(rec, authedRequest(http.MethodPost, "/api/extension/actions/1/other", nil))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("invalid id", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiExtensionActionsRouter(rec, authedRequest(http.MethodPost, "/api/extension/actions/abc/ack", nil))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("ack existing via router", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiExtensionActionsRouter(rec, authedRequest(http.MethodPost, "/api/extension/actions/1/ack", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "acked")
	})

	t.Run("ack wrong method", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.handleExtensionActionAck(rec, authedRequest(http.MethodGet, "/api/extension/actions/1/ack", nil), 1)
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})
}

func TestRequireDB_NilDB(t *testing.T) {
	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })

	s := &Server{}
	w := httptest.NewRecorder()
	db, ok := s.requireDB(w)
	assert.Nil(t, db)
	assert.False(t, ok)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestApiExtensionActionsPending_BadSince(t *testing.T) {
	srv, cleanup := newExtensionActionTestServer(t)
	defer cleanup()

	t.Run("method not allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiExtensionActionsPending(rec, authedRequest(http.MethodPost, "/api/extension/actions/pending", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("bad since", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiExtensionActionsPending(rec, authedRequest(http.MethodGet, "/api/extension/actions/pending?since=-1", nil))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// ==== merged from api_extension_actions_test.go ====
func newExtensionActionTestServer(t *testing.T) (*Server, func()) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, extension.AutoMigrate(db))

	prevDB := global.GlobalDB
	global.GlobalDB = &models.TorrentDB{DB: db}

	srv := &Server{sessions: map[string]string{"sess-test": "admin"}}
	cleanup := func() { global.GlobalDB = prevDB }
	return srv, cleanup
}

func TestApiExtensionActionsPendingEmpty(t *testing.T) {
	srv, cleanup := newExtensionActionTestServer(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	srv.apiExtensionActionsPending(rec, authedRequest(http.MethodGet, "/api/extension/actions/pending", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var out []extension.PendingAction
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Empty(t, out)
}

func TestApiExtensionActionsPendingHappyPath(t *testing.T) {
	srv, cleanup := newExtensionActionTestServer(t)
	defer cleanup()

	require.NoError(t, extension.Enqueue(global.GlobalDB.DB, extension.PendingAction{
		Type:      extension.ActionOpenTab,
		TargetURL: "https://hdsky.me/",
		SiteName:  "hdsky",
		Reason:    "login refresh",
	}))

	rec := httptest.NewRecorder()
	srv.apiExtensionActionsPending(rec, authedRequest(http.MethodGet, "/api/extension/actions/pending", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var out []extension.PendingAction
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out, 1)
	assert.Equal(t, "hdsky", out[0].SiteName)
	assert.Equal(t, extension.ActionOpenTab, out[0].Type)
}

func TestApiExtensionActionsPendingSinceFilter(t *testing.T) {
	srv, cleanup := newExtensionActionTestServer(t)
	defer cleanup()

	old := time.Now().UTC().Add(-2 * time.Hour)
	recent := time.Now().UTC().Add(-1 * time.Minute)
	require.NoError(t, extension.Enqueue(global.GlobalDB.DB, extension.PendingAction{
		Type:      extension.ActionOpenTab,
		TargetURL: "https://a.example/",
		SiteName:  "a",
		CreatedAt: old,
		ExpiresAt: old.Add(extension.DefaultTTL),
	}))
	require.NoError(t, extension.Enqueue(global.GlobalDB.DB, extension.PendingAction{
		Type:      extension.ActionOpenTab,
		TargetURL: "https://b.example/",
		SiteName:  "b",
		CreatedAt: recent,
		ExpiresAt: recent.Add(extension.DefaultTTL),
	}))

	cutoff := old.Add(time.Hour).Unix()
	url := "/api/extension/actions/pending?since=" + itoa(cutoff)
	rec := httptest.NewRecorder()
	srv.apiExtensionActionsPending(rec, authedRequest(http.MethodGet, url, nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var out []extension.PendingAction
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out, 1)
	assert.Equal(t, "b", out[0].SiteName)
}

func TestApiExtensionActionsPendingBadSince(t *testing.T) {
	srv, cleanup := newExtensionActionTestServer(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	srv.apiExtensionActionsPending(rec, authedRequest(http.MethodGet, "/api/extension/actions/pending?since=abc", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestApiExtensionActionsAckIdempotent(t *testing.T) {
	srv, cleanup := newExtensionActionTestServer(t)
	defer cleanup()

	require.NoError(t, extension.Enqueue(global.GlobalDB.DB, extension.PendingAction{
		Type:      extension.ActionOpenTab,
		TargetURL: "https://hdsky.me/",
		SiteName:  "hdsky",
	}))
	var row extension.PendingAction
	require.NoError(t, global.GlobalDB.DB.First(&row).Error)

	url := "/api/extension/actions/" + itoa(int64(row.ID)) + "/ack"
	for i, expectStatus := range []string{"acked", "already_acked"} {
		rec := httptest.NewRecorder()
		srv.apiExtensionActionsRouter(rec, authedRequest(http.MethodPost, url, nil))
		require.Equalf(t, http.StatusOK, rec.Code, "call #%d should succeed: body=%s", i, rec.Body.String())

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, true, resp["ok"])
		assert.Equal(t, expectStatus, resp["status"])
	}

	pending, err := extension.ListPending(global.GlobalDB.DB, 0)
	require.NoError(t, err)
	assert.Empty(t, pending)
}

func TestApiExtensionActionsAckUnknownID(t *testing.T) {
	srv, cleanup := newExtensionActionTestServer(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	srv.apiExtensionActionsRouter(rec, authedRequest(http.MethodPost, "/api/extension/actions/9999/ack", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestApiExtensionActionsAckBadID(t *testing.T) {
	srv, cleanup := newExtensionActionTestServer(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	srv.apiExtensionActionsRouter(rec, authedRequest(http.MethodPost, "/api/extension/actions/abc/ack", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	negative := false
	if v < 0 {
		negative = true
		v = -v
	}
	digits := []byte{}
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	if negative {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

// ==== merged from api_extension_site_cov6_test.go ====
func TestRegisterExtensionActionRoutes_Cov(t *testing.T) {
	srv, cleanup := newExtensionActionTestServer(t)
	defer cleanup()

	mux := http.NewServeMux()
	srv.registerExtensionActionRoutes(mux)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/extension/actions/pending", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "sess-test"})
	mux.ServeHTTP(w, req)
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestApiExtensionActionsPending_WithData(t *testing.T) {
	srv, cleanup := newExtensionActionTestServer(t)
	defer cleanup()

	require.NoError(t, extension.Enqueue(global.GlobalDB.DB, extension.PendingAction{
		Type: extension.ActionOpenTab, TargetURL: "https://hdsky.me/", SiteName: "hdsky",
	}))

	t.Run("list all", func(t *testing.T) {
		w := httptest.NewRecorder()
		srv.apiExtensionActionsPending(w, authedRequest(http.MethodGet, "/api/extension/actions/pending", nil))
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "hdsky")
	})

	t.Run("list since", func(t *testing.T) {
		w := httptest.NewRecorder()
		srv.apiExtensionActionsPending(w, authedRequest(http.MethodGet, "/api/extension/actions/pending?since=1", nil))
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestApiExtensionActionsRouter_AckDispatch(t *testing.T) {
	srv, cleanup := newExtensionActionTestServer(t)
	defer cleanup()

	require.NoError(t, extension.Enqueue(global.GlobalDB.DB, extension.PendingAction{
		Type: extension.ActionOpenTab, TargetURL: "https://hdsky.me/", SiteName: "hdsky",
	}))

	w := httptest.NewRecorder()
	srv.apiExtensionActionsRouter(w, authedRequest(http.MethodPost, "/api/extension/actions/1/ack", nil))
	require.Equal(t, http.StatusOK, w.Code)
}

func TestApiSiteDetail_LoginStateProbeAndTestReminder(t *testing.T) {
	srv := newLoginMonitorServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	t.Run("probe via detail", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/hdsky/login-state/probe", nil)
		srv.apiSiteDetail(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("test-reminder via detail", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/hdsky/login-state/test-reminder", nil)
		srv.apiSiteDetail(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
