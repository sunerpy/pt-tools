package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/cloakdriver"
	internalcrypto "github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/models"
)

// ==== merged from api_cloak_cov2_test.go ====
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

// ==== merged from api_cloak_cov_test.go ====
func TestApiCloakConfig_MethodDispatch(t *testing.T) {
	srv, _, cleanup := newCloakTestServer(t)
	defer cleanup()

	t.Run("unsupported method", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiCloakConfig(rec, cloakAuthedReq(http.MethodDelete, "/api/cloak/config", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("get with nil store", func(t *testing.T) {
		nilSrv := &Server{}
		rec := httptest.NewRecorder()
		nilSrv.handleCloakConfigGet(rec, cloakAuthedReq(http.MethodGet, "/api/cloak/config", nil))
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})
}

func TestHandleCloakConfigPut_BadInput(t *testing.T) {
	srv, _, cleanup := newCloakTestServer(t)
	defer cleanup()

	t.Run("nil store", func(t *testing.T) {
		nilSrv := &Server{}
		rec := httptest.NewRecorder()
		nilSrv.handleCloakConfigPut(rec, cloakAuthedReq(http.MethodPut, "/api/cloak/config", cloakConfigPutRequest{Endpoint: "x"}))
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})

	t.Run("empty endpoint", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.handleCloakConfigPut(rec, cloakAuthedReq(http.MethodPut, "/api/cloak/config", cloakConfigPutRequest{Endpoint: "  "}))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("valid put", func(t *testing.T) {
		tok := "tok"
		rec := httptest.NewRecorder()
		srv.handleCloakConfigPut(rec, cloakAuthedReq(http.MethodPut, "/api/cloak/config", cloakConfigPutRequest{Endpoint: "http://m:8080", Token: &tok}))
		require.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestApiCloakTest_BadInput(t *testing.T) {
	srv, _, cleanup := newCloakTestServer(t)
	defer cleanup()

	t.Run("method not allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiCloakTest(rec, cloakAuthedReq(http.MethodGet, "/api/cloak/test", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("nil store", func(t *testing.T) {
		nilSrv := &Server{}
		rec := httptest.NewRecorder()
		nilSrv.apiCloakTest(rec, cloakAuthedReq(http.MethodPost, "/api/cloak/test", nil))
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})

	t.Run("no endpoint configured returns unknown category", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiCloakTest(rec, cloakAuthedReq(http.MethodPost, "/api/cloak/test", cloakTestRequest{}))
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), cloakCatUnknown)
	})
}

func TestClassifyCloakTestResult(t *testing.T) {
	tests := []struct {
		name    string
		info    *cloakdriver.ManagerStatusInfo
		err     error
		wantCat string
	}{
		{"success", &cloakdriver.ManagerStatusInfo{Version: "1.2.3"}, nil, cloakCatSuccess},
		{"success nil info", nil, nil, cloakCatSuccess},
		{"auth fail", nil, cloakdriver.ErrManagerAuthFailed, cloakCatAuthFail},
		{"not found", nil, cloakdriver.ErrManagerNotFound, cloakCatNotFound},
		{"server error", nil, cloakdriver.ErrManagerServerError, cloakCatServerError},
		{"dns fail", nil, cloakdriver.ErrManagerDNSFailed, cloakCatDNSFail},
		{"conn refused", nil, cloakdriver.ErrManagerConnRefused, cloakCatConnRefused},
		{"timeout", nil, cloakdriver.ErrManagerTimeout, cloakCatTimeout},
		{"protocol error", nil, cloakdriver.ErrManagerProtocolError, cloakCatProtocolError},
		{"unknown", nil, errors.New("boom"), cloakCatUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := classifyCloakTestResult(tt.info, tt.err)
			assert.Equal(t, tt.wantCat, resp.Category)
		})
	}

	t.Run("success carries version", func(t *testing.T) {
		resp := classifyCloakTestResult(&cloakdriver.ManagerStatusInfo{Version: "9.9"}, nil)
		assert.Equal(t, "9.9", resp.ManagerVersion)
	})
}

// ==== merged from api_cloak_test.go ====
func newCloakTestServer(t *testing.T) (*Server, *core.ConfigStore, func()) {
	t.Helper()

	t.Setenv("PT_TOOLS_SECRET_KEY", "")
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	keyDir := filepath.Join(homeDir, ".pt-tools")
	require.NoError(t, os.MkdirAll(keyDir, 0o700))
	keyFile := filepath.Join(keyDir, "secret.key")
	require.NoError(t, os.WriteFile(keyFile, []byte(strings.Repeat("a", 64)), 0o600))
	internalcrypto.ResetForTest()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.CloakSettings{}))

	tdb := &models.TorrentDB{DB: db}
	prevDB := global.GlobalDB
	global.GlobalDB = tdb

	store := core.NewConfigStore(tdb)
	srv := &Server{
		store:    store,
		sessions: map[string]string{"sess-test": "admin"},
	}
	cleanup := func() { global.GlobalDB = prevDB }
	return srv, store, cleanup
}

func cloakAuthedReq(method, path string, body any) *http.Request {
	var reader *bytes.Reader
	if body != nil {
		buf, _ := json.Marshal(body)
		reader = bytes.NewReader(buf)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.AddCookie(&http.Cookie{Name: "session", Value: "sess-test"})
	return req
}

func TestApiCloakConfigGet_NoTokenInResponse(t *testing.T) {
	srv, store, cleanup := newCloakTestServer(t)
	defer cleanup()

	require.NoError(t, store.SaveCloakConfig("http://manager.local:8080", "secret-token-XYZ", false))

	rec := httptest.NewRecorder()
	srv.apiCloakConfig(rec, cloakAuthedReq(http.MethodGet, "/api/cloak/config", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	if strings.Contains(body, "secret-token-XYZ") {
		t.Fatalf("response leaked plaintext token: %s", body)
	}

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "http://manager.local:8080", out["endpoint"])
	assert.Equal(t, true, out["has_token"])
	if v, ok := out["token"]; ok {
		assert.NotEqual(t, "secret-token-XYZ", v)
	}
}

func TestApiCloakConfigPut_TokenEncrypted(t *testing.T) {
	srv, _, cleanup := newCloakTestServer(t)
	defer cleanup()

	payload := map[string]any{
		"endpoint": "http://manager.local:9000",
		"token":    "abc123",
	}
	rec := httptest.NewRecorder()
	srv.apiCloakConfig(rec, cloakAuthedReq(http.MethodPut, "/api/cloak/config", payload))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var row models.CloakSettings
	require.NoError(t, global.GlobalDB.DB.First(&row).Error)
	assert.Equal(t, "http://manager.local:9000", row.Endpoint)
	assert.NotEmpty(t, row.TokenEncrypted)
	assert.NotEqual(t, "abc123", row.TokenEncrypted, "token must not be stored as plaintext")
	assert.NotContains(t, row.TokenEncrypted, "abc123")
}

func newCloakManagerMock(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(h)
}

func TestApiCloakTest_Success(t *testing.T) {
	srv, _, cleanup := newCloakTestServer(t)
	defer cleanup()

	mock := newCloakManagerMock(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/status", r.URL.Path)
		assert.Equal(t, "Bearer good-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","version":"0.0.4"}`))
	})
	defer mock.Close()

	body := map[string]any{"endpoint": mock.URL, "token": "good-token"}
	rec := httptest.NewRecorder()
	srv.apiCloakTest(rec, cloakAuthedReq(http.MethodPost, "/api/cloak/test", body))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "success", out["category"])
	assert.Equal(t, "0.0.4", out["manager_version"])
}

func TestApiCloakTest_AuthFail(t *testing.T) {
	srv, _, cleanup := newCloakTestServer(t)
	defer cleanup()

	mock := newCloakManagerMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	defer mock.Close()

	body := map[string]any{"endpoint": mock.URL, "token": "wrong"}
	rec := httptest.NewRecorder()
	srv.apiCloakTest(rec, cloakAuthedReq(http.MethodPost, "/api/cloak/test", body))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "auth_fail", out["category"])
	assert.NotContains(t, rec.Body.String(), "wrong")
}

func TestApiCloakTest_NotFound(t *testing.T) {
	srv, _, cleanup := newCloakTestServer(t)
	defer cleanup()

	mock := newCloakManagerMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer mock.Close()

	body := map[string]any{"endpoint": mock.URL, "token": "tok"}
	rec := httptest.NewRecorder()
	srv.apiCloakTest(rec, cloakAuthedReq(http.MethodPost, "/api/cloak/test", body))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "not_found", out["category"])
}

func TestApiCloakTest_ServerError(t *testing.T) {
	srv, _, cleanup := newCloakTestServer(t)
	defer cleanup()

	mock := newCloakManagerMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer mock.Close()

	body := map[string]any{"endpoint": mock.URL, "token": "tok"}
	rec := httptest.NewRecorder()
	srv.apiCloakTest(rec, cloakAuthedReq(http.MethodPost, "/api/cloak/test", body))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "server_error", out["category"])
}

func TestApiCloakTest_DnsFail(t *testing.T) {
	srv, _, cleanup := newCloakTestServer(t)
	defer cleanup()

	body := map[string]any{
		"endpoint": "http://bogus-no-such-host-pt-tools-cloak.invalid:8080",
		"token":    "tok",
	}
	rec := httptest.NewRecorder()
	srv.apiCloakTest(rec, cloakAuthedReq(http.MethodPost, "/api/cloak/test", body))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "dns_fail", out["category"])
}

func TestApiCloakTest_Timeout(t *testing.T) {
	srv, _, cleanup := newCloakTestServer(t)
	defer cleanup()

	mock := newCloakManagerMock(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	})
	defer mock.Close()

	body := map[string]any{
		"endpoint":   mock.URL,
		"token":      "tok",
		"timeout_ms": 200,
	}
	rec := httptest.NewRecorder()
	srv.apiCloakTest(rec, cloakAuthedReq(http.MethodPost, "/api/cloak/test", body))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "timeout", out["category"])
}

func TestApiCloakTest_ConnRefused(t *testing.T) {
	srv, _, cleanup := newCloakTestServer(t)
	defer cleanup()

	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedURL := closed.URL
	closed.Close()

	body := map[string]any{"endpoint": closedURL, "token": "tok"}
	rec := httptest.NewRecorder()
	srv.apiCloakTest(rec, cloakAuthedReq(http.MethodPost, "/api/cloak/test", body))
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "conn_refused", out["category"])
}
