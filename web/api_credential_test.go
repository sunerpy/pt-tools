package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// ==== merged from api_credential_cov2_test.go ====
func TestUpdateSiteCredential_Success(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)
	require.NoError(t, srv.store.SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, AutoStart: true,
	}))

	enabled := false
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("hdsky"), models.SiteConfig{
		Enabled: &enabled, AuthMethod: "cookie", Cookie: "seed=1", APIUrl: "https://hdsky.me",
	}))

	t.Run("update cookie and passkey enables site", func(t *testing.T) {
		body, _ := json.Marshal(credentialUpdateRequest{
			Cookie:  credStrPtr("uid=9; pass=9"),
			Passkey: credStrPtr("newpass"),
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/sites/hdsky/credential", bytes.NewReader(body))
		srv.updateSiteCredential(w, req, models.SiteGroup("hdsky"))
		require.Equal(t, http.StatusOK, w.Code)
		settleCredentialReload(t, srv)
	})
}

func TestApiUserInfoSiteDetail_DeleteError(t *testing.T) {
	service := setupTestUserInfoService()
	InitUserInfoService(service)
	t.Cleanup(func() { InitUserInfoService(nil) })

	s := &Server{}

	t.Run("delete existing succeeds", func(t *testing.T) {
		ctx := context.Background()
		_, _ = service.FetchAndSave(ctx, "site1")
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/v2/userinfo/sites/site1", nil)
		s.apiUserInfoSiteDetail(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/api/v2/userinfo/sites/site1", nil)
		s.apiUserInfoSiteDetail(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApiUserInfoSites_List(t *testing.T) {
	service := setupTestUserInfoService()
	InitUserInfoService(service)
	t.Cleanup(func() { InitUserInfoService(nil) })
	_, _ = service.FetchAndSaveAll(context.Background())

	s := &Server{}

	t.Run("list all", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/sites", nil)
		s.apiUserInfoSites(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp []UserInfoResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.GreaterOrEqual(t, len(resp), 1)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/userinfo/sites", nil)
		s.apiUserInfoSites(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

// ==== merged from api_credential_cov_test.go ====
func credStrPtr(s string) *string { return &s }

// settleCredentialReload 与 updateSiteCredential 成功路径派生的异步 goroutine 建立
// happens-before 边。该 goroutine 在 mgr.Reload 中读取包级全局 GlobalDB，随后（在
// AutoStart+DownloadDir 已配置的完整路径下）在 mgr.mu 保护下写入 freeEndMonitor。
// GetFreeEndMonitor 复用同一把 mgr.mu，轮询到非 nil 即建立同步关系，保证 goroutine
// 对全局变量的读取先于本测试返回，从而消除与其他测试改写 GlobalDB 的 -race 竞态
// （纯 time.Sleep 不建立同步关系，无法消除竞态）。调用前须已持久化 AutoStart 全局配置。
func settleCredentialReload(t *testing.T, srv *Server) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if srv.mgr != nil && srv.mgr.GetFreeEndMonitor() != nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("credential 异步 reload 未在超时内完成")
}

func TestCredentialProvided(t *testing.T) {
	tests := []struct {
		name string
		req  credentialUpdateRequest
		want bool
	}{
		{"all nil", credentialUpdateRequest{}, false},
		{"empty cookie", credentialUpdateRequest{Cookie: credStrPtr("   ")}, false},
		{"cookie provided", credentialUpdateRequest{Cookie: credStrPtr("a=1")}, true},
		{"api key provided", credentialUpdateRequest{APIKey: credStrPtr("key")}, true},
		{"passkey provided", credentialUpdateRequest{Passkey: credStrPtr("pk")}, true},
		{"empty api key", credentialUpdateRequest{APIKey: credStrPtr("")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, credentialProvided(tt.req))
		})
	}
}

func TestUpdateSiteCredential(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)
	require.NoError(t, srv.store.SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, AutoStart: true,
	}))

	enabled := false
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("hdsky"), models.SiteConfig{
		Enabled:    &enabled,
		AuthMethod: "cookie",
		Cookie:     "seed=1",
		APIUrl:     "https://hdsky.me",
	}))

	t.Run("invalid json", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/sites/hdsky/credential", bytes.NewBufferString(`{bad`))
		srv.updateSiteCredential(w, req, models.SiteGroup("hdsky"))
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("no fields provided", func(t *testing.T) {
		body, _ := json.Marshal(credentialUpdateRequest{})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/sites/hdsky/credential", bytes.NewReader(body))
		srv.updateSiteCredential(w, req, models.SiteGroup("hdsky"))
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("site not found", func(t *testing.T) {
		body, _ := json.Marshal(credentialUpdateRequest{Cookie: credStrPtr("a=1")})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/sites/nosuch/credential", bytes.NewReader(body))
		srv.updateSiteCredential(w, req, models.SiteGroup("nosuch"))
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("valid cookie update enables site", func(t *testing.T) {
		body, _ := json.Marshal(credentialUpdateRequest{Cookie: credStrPtr("uid=1; pass=2")})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/sites/hdsky/credential", bytes.NewReader(body))
		srv.updateSiteCredential(w, req, models.SiteGroup("hdsky"))
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, true, resp["success"])
		assert.Equal(t, "hdsky", resp["site"])

		sc, err := srv.store.GetSiteConf(models.SiteGroup("hdsky"))
		require.NoError(t, err)
		require.NotNil(t, sc.Enabled)
		assert.True(t, *sc.Enabled)
		settleCredentialReload(t, srv)
	})
}
