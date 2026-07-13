package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/extension"
	"github.com/sunerpy/pt-tools/models"
)

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
