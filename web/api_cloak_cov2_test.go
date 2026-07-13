package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApiCloakTest_LoadsFromStore(t *testing.T) {
	srv, store, cleanup := newCloakTestServer(t)
	defer cleanup()

	mock := newCloakManagerMock(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","version":"1.2.3"}`))
	})
	defer mock.Close()

	require.NoError(t, store.SaveCloakConfig(mock.URL, "stored-token", false))

	rec := httptest.NewRecorder()
	srv.apiCloakTest(rec, cloakAuthedReq(http.MethodPost, "/api/cloak/test", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "success", out["category"])
}

func TestApiCloakTest_NoEndpointConfigured(t *testing.T) {
	srv, _, cleanup := newCloakTestServer(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	srv.apiCloakTest(rec, cloakAuthedReq(http.MethodPost, "/api/cloak/test", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "unknown", out["category"])
}
