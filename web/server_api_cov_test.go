package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func TestApiGlobal_GetAndPost(t *testing.T) {
	srv := setupServer(t)

	t.Run("GET returns settings", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/global", nil)
		srv.apiGlobal(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("POST invalid body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/global", bytes.NewBufferString(`{bad`))
		srv.apiGlobal(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("POST empty download dir", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{"default_interval_minutes": 20})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/global", bytes.NewReader(body))
		srv.apiGlobal(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("POST valid settings", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"default_interval_minutes": 20,
			"download_dir":             t.TempDir(),
			"torrent_size_gb":          200,
			"free_end_advance_minutes": 120,
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/global", bytes.NewReader(body))
		srv.apiGlobal(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/global", nil)
		srv.apiGlobal(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApiQbit_GetAndPost(t *testing.T) {
	srv := setupServer(t)

	t.Run("GET", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/qbit", nil)
		srv.apiQbit(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("POST invalid body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/qbit", bytes.NewBufferString(`{bad`))
		srv.apiQbit(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("POST valid", func(t *testing.T) {
		body, _ := json.Marshal(models.QbitSettings{URL: "http://localhost:8080", User: "admin", Password: "x"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/qbit", bytes.NewReader(body))
		srv.apiQbit(w, req)
		assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusBadRequest)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/qbit", nil)
		srv.apiQbit(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestLoginHandler_FailPaths(t *testing.T) {
	srv := setupServer(t)
	require.NoError(t, srv.store.EnsureAdmin("admin", hashPassword("secret")))

	t.Run("empty credentials", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"username": "", "password": ""})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		srv.loginHandler(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("wrong password", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"username": "admin", "password": "nope"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		srv.loginHandler(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("unknown user", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"username": "ghost", "password": "x"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		srv.loginHandler(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}
