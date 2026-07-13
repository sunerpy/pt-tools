package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestApiSiteLoginStateRouter_DispatchCov3(t *testing.T) {
	srv := newLoginMonitorServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	t.Run("nil db", func(t *testing.T) {
		prev := global.GlobalDB
		global.GlobalDB = nil
		t.Cleanup(func() { global.GlobalDB = prev })
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/login-state/hdsky", nil)
		srv.apiSiteLoginStateRouter(w, req)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})

	t.Run("missing site name", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/login-state/", nil)
		srv.apiSiteLoginStateRouter(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("get action", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/login-state/hdsky", nil)
		srv.apiSiteLoginStateRouter(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("probe action", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/hdsky/probe", nil)
		srv.apiSiteLoginStateRouter(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("config action", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", bytes.NewReader([]byte(`{"probe_mode":"auto"}`)))
		srv.apiSiteLoginStateRouter(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("test-reminder action", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/hdsky/test-reminder", nil)
		srv.apiSiteLoginStateRouter(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("unknown action", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/login-state/hdsky/bogus", nil)
		srv.apiSiteLoginStateRouter(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandleLoginStateGet_NotFound(t *testing.T) {
	srv := newLoginMonitorServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sites/login-state/no-such-site", nil)
	srv.handleLoginStateGet(w, req, "no-such-site")
	assert.Equal(t, http.StatusNotFound, w.Code)
}
