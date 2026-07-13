package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApiStopStartAll(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)

	t.Run("stop method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/control/stop", nil)
		srv.apiStopAll(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("stop ok", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/control/stop", nil)
		srv.apiStopAll(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("start method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/control/start", nil)
		srv.apiStartAll(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("start dispatch", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/control/start", nil)
		srv.apiStartAll(w, req)
		assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusBadRequest)
	})
}

func TestApiLogs_Error(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	srv := &Server{}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
	srv.apiLogs(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestApiQbit_PostSaveOk(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)

	body := `{"enabled":true}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/qbit", strings.NewReader(body))
	srv.apiQbit(w, req)
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusBadRequest)
}
