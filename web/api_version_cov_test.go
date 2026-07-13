package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApiPing(t *testing.T) {
	s := &Server{}

	t.Run("GET returns ok", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
		s.apiPing(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "ok", resp["status"])
		assert.Contains(t, resp, "version")
	})

	t.Run("POST not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/ping", nil)
		s.apiPing(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApiVersion(t *testing.T) {
	s := &Server{}

	t.Run("GET returns version info", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
		s.apiVersion(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp, "version")
	})

	t.Run("DELETE not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/version", nil)
		s.apiVersion(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApiVersionRuntime(t *testing.T) {
	s := &Server{}

	t.Run("GET returns runtime info", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/version/runtime", nil)
		s.apiVersionRuntime(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp, "runtime")
		assert.Contains(t, resp, "upgrade_progress")
	})

	t.Run("POST not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/version/runtime", nil)
		s.apiVersionRuntime(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApiVersionUpgrade_MethodDispatch(t *testing.T) {
	s := &Server{}

	t.Run("GET returns progress", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/version/upgrade", nil)
		s.apiVersionUpgrade(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("DELETE cancels", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/version/upgrade", nil)
		s.apiVersionUpgrade(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, true, resp["success"])
	})

	t.Run("PUT not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/version/upgrade", nil)
		s.apiVersionUpgrade(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApiUpgradeStart_BadInput(t *testing.T) {
	s := &Server{}

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"invalid json", `{invalid`, http.StatusBadRequest},
		{"missing version", `{}`, http.StatusBadRequest},
		{"unknown version not cached", `{"version":"v99.99.99"}`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/version/upgrade", bytes.NewBufferString(tt.body))
			s.apiUpgradeStart(w, req)
			// CanUpgrade may reject earlier (also 400); either way must be an error status.
			assert.GreaterOrEqual(t, w.Code, http.StatusBadRequest)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, false, resp["success"])
			assert.Contains(t, resp, "error")
		})
	}
}

func TestApiUpgradeProgress(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/version/upgrade", nil)
	s.apiUpgradeProgress(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Body.Bytes())
}

func TestApiUpgradeCancel(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/version/upgrade", nil)
	s.apiUpgradeCancel(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["success"])
}

func TestWriteJSONError(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSONError(w, "boom", http.StatusTeapot)
	require.Equal(t, http.StatusTeapot, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, false, resp["success"])
	assert.Equal(t, "boom", resp["error"])
}
