package web

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/cookiecloud"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

// encryptFixed 按 CookieCloud 0.3.0 起的 aes-128-cbc-fixed 方式加密（key = md5(uuid-password) 的前 16 个字符，iv 全 0）。
func encryptFixed(t *testing.T, uuid, password string, plain []byte) string {
	t.Helper()
	sum := md5.Sum([]byte(uuid + "-" + password))
	block, err := aes.NewCipher([]byte(hex.EncodeToString(sum[:])[:16]))
	require.NoError(t, err)
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	data := append(append([]byte{}, plain...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	out := make([]byte, len(data))
	cipher.NewCBCEncrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(out, data)
	return base64.StdEncoding.EncodeToString(out)
}

type cookieCloudAPIEnv struct {
	mux     *http.ServeMux
	server  *httptest.Server
	mu      sync.Mutex
	applied []map[string]string
}

func newCookieCloudServer(t *testing.T) *cookieCloudAPIEnv {
	t.Helper()
	srv := setupServer(t)
	srv.mgr.StopAll()
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(&models.CookieCloudSetting{}))
	plain, err := json.Marshal(map[string]any{"cookie_data": map[string]any{
		"hdsky.me": []map[string]any{{"name": "c_secure_uid", "value": "42", "domain": ".hdsky.me", "path": "/"}, {"name": "c_secure_pass", "value": "s3cret", "domain": ".hdsky.me", "path": "/"}},
	}})
	require.NoError(t, err)
	enc := encryptFixed(t, "u1", "pw", plain)
	e := &cookieCloudAPIEnv{}
	e.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/get/u1" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"encrypted": enc, "crypto_type": "aes-128-cbc-fixed"})
	}))
	t.Cleanup(e.server.Close)
	svc := cookiecloud.New(cookiecloud.Config{
		DB: db, Cipher: apiTestCipher{}, HTTP: e.server.Client(),
		Sites: func(context.Context) ([]cookiecloud.SiteState, error) {
			return []cookiecloud.SiteState{{Name: "hdsky", DisplayName: "HDSky", BaseURL: "https://hdsky.me/"}}, nil
		},
		Apply: func(_ context.Context, cookies map[string]string) map[string]error {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.applied = append(e.applied, cookies)
			return nil
		},
	})
	srv.mgr.SetCookieCloudWorker(scheduler.NewCookieCloudWorker(scheduler.CookieCloudWorkerConfig{Service: svc}))
	e.mux = http.NewServeMux()
	srv.registerCookieCloudRoutes(e.mux)
	srv.sessions.put("sess-test", "admin")
	return e
}

func TestCookieCloudAPI_RequiresSession(t *testing.T) {
	e := newCookieCloudServer(t)
	for _, path := range []string{"/api/cookiecloud/settings", "/api/cookiecloud/preview", "/api/cookiecloud/import"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		e.mux.ServeHTTP(w, req)
		assert.Contains(t, []int{http.StatusUnauthorized, http.StatusFound}, w.Code, path)
	}
}

func TestCookieCloudAPI_NotStarted(t *testing.T) {
	srv := setupServer(t)
	srv.mgr.StopAll()
	mux := http.NewServeMux()
	srv.registerCookieCloudRoutes(mux)
	srv.sessions.put("sess-test", "admin")
	assert.Equal(t, http.StatusServiceUnavailable, serveAuthed(mux, http.MethodGet, "/api/cookiecloud/settings", "").Code)
	assert.Equal(t, http.StatusServiceUnavailable, serveAuthed(mux, http.MethodPost, "/api/cookiecloud/preview", "").Code)
	assert.Equal(t, http.StatusServiceUnavailable, serveAuthed(mux, http.MethodPost, "/api/cookiecloud/import", `{"sites":["hdsky"]}`).Code)
}

func TestCookieCloudAPI_SettingsPreviewImport(t *testing.T) {
	e := newCookieCloudServer(t)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, "/api/cookiecloud/preview", "").Code, "还没设置")
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPut, "/api/cookiecloud/settings", `{"server":"x"}`).Code, "拼错的字段")
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPut, "/api/cookiecloud/settings", `{"server_url":"ftp://x"}`).Code)
	assert.Equal(t, http.StatusMethodNotAllowed, serveAuthed(e.mux, http.MethodDelete, "/api/cookiecloud/settings", "").Code)
	assert.Equal(t, http.StatusMethodNotAllowed, serveAuthed(e.mux, http.MethodGet, "/api/cookiecloud/preview", "").Code)
	assert.Equal(t, http.StatusMethodNotAllowed, serveAuthed(e.mux, http.MethodGet, "/api/cookiecloud/import", "").Code)

	w := serveAuthed(e.mux, http.MethodPut, "/api/cookiecloud/settings", `{"server_url":"`+e.server.URL+`","uuid":"u1","password":"pw"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got cookiecloud.Settings
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.True(t, got.HasPassword)
	assert.NotContains(t, w.Body.String(), `"pw"`, "密码只写不读")

	w = serveAuthed(e.mux, http.MethodPost, "/api/cookiecloud/preview", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "s3cret", "预览不带 Cookie 的值")
	var p cookiecloud.Preview
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &p))
	require.Len(t, p.Items, 1)
	assert.Equal(t, "hdsky", p.Items[0].Site)
	assert.Equal(t, []string{"c_secure_pass", "c_secure_uid"}, p.Items[0].CookieNames)

	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, "/api/cookiecloud/import", `{"sites":[]}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, "/api/cookiecloud/import", `{"site":["hdsky"]}`).Code)
	w = serveAuthed(e.mux, http.MethodPost, "/api/cookiecloud/import", `{"sites":["hdsky","nope"]}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var res cookiecloud.ImportResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, []string{"hdsky"}, res.Imported)
	assert.Equal(t, []string{"nope"}, res.Missing)
	assert.NotContains(t, w.Body.String(), "s3cret")
	require.Len(t, e.applied, 1)
	assert.Equal(t, "c_secure_pass=s3cret; c_secure_uid=42", e.applied[0]["hdsky"])

	// 服务端没有这个 UUID 的数据：404；密码不对：400
	require.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodPut, "/api/cookiecloud/settings", `{"server_url":"`+e.server.URL+`","uuid":"u2"}`).Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(e.mux, http.MethodPost, "/api/cookiecloud/preview", "").Code)
	require.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodPut, "/api/cookiecloud/settings", `{"server_url":"`+e.server.URL+`","uuid":"u1","password":"wrong"}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, "/api/cookiecloud/preview", "").Code)
}
