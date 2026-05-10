package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBasicAuthMiddleware_Success(t *testing.T) {
	h := BasicAuthMiddleware("user", "pass")(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("user", "pass")
	rr := httptest.NewRecorder()
	h(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
}

func TestBasicAuthMiddleware_Failure(t *testing.T) {
	h := BasicAuthMiddleware("user", "pass")(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rr := httptest.NewRecorder()
	h(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusUnauthorized, rr.Code)
	require.Equal(t, `Basic realm="scraper"`, rr.Header().Get("WWW-Authenticate"))
	assertJSONError(t, rr, "Unauthorized")
}

func TestAPIKeyMiddleware_HeaderSuccess(t *testing.T) {
	h := APIKeyMiddleware("secret")(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "secret")
	rr := httptest.NewRecorder()
	h(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
}

func TestAPIKeyMiddleware_QuerySuccess(t *testing.T) {
	h := APIKeyMiddleware("secret")(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	req := httptest.NewRequest(http.MethodGet, "/?api_key=secret", nil)
	rr := httptest.NewRecorder()
	h(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
}

func TestAPIKeyMiddleware_Failure(t *testing.T) {
	h := APIKeyMiddleware("secret")(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rr := httptest.NewRecorder()
	h(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusUnauthorized, rr.Code)
	assertJSONError(t, rr, "Unauthorized")
}

func TestSessionAuthMiddleware_Success(t *testing.T) {
	h := SessionAuthMiddleware(func(*http.Request) bool { return true })(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	rr := httptest.NewRecorder()
	h(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, rr.Code)
}

func TestSessionAuthMiddleware_Failure(t *testing.T) {
	h := SessionAuthMiddleware(func(*http.Request) bool { return false })(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rr := httptest.NewRecorder()
	h(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusUnauthorized, rr.Code)
	assertJSONError(t, rr, "Unauthorized")
}

func TestNoAuthMiddleware_Passthrough(t *testing.T) {
	h := NoAuthMiddleware()(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	rr := httptest.NewRecorder()
	h(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, rr.Code)
}

func TestGenerateAPIKey(t *testing.T) {
	key1, err := GenerateAPIKey()
	require.NoError(t, err)
	key2, err := GenerateAPIKey()
	require.NoError(t, err)
	require.Len(t, key1, 64)
	require.Len(t, key2, 64)
	require.NotEqual(t, key1, key2)
}

func assertJSONError(t *testing.T, rr *httptest.ResponseRecorder, expected string) {
	t.Helper()
	var payload map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &payload))
	require.Equal(t, expected, payload["error"])
}
