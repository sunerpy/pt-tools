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

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
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

// HDDolby 要 Cookie 和 API Key：扩展只同步来 Cookie、库里还没有 API Key 时，Cookie 照样存下，站点保持停用，
// 回 200 并说明还缺 API Key（以前走完整校验回 500「API Key 不能为空」，扩展每次同步都报失败、Cookie 也存不进去）。
func TestUpdateSiteCredential_IncompleteKeepsDisabled(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)
	require.NoError(t, srv.store.SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, AutoStart: true,
	}))
	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hddolby", DisplayName: "hddolby", AuthMethod: "cookie_and_api_key", IsBuiltin: true}).Error)

	put := func(body credentialUpdateRequest) (*httptest.ResponseRecorder, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		srv.updateSiteCredential(w, httptest.NewRequest(http.MethodPut, "/api/sites/hddolby", bytes.NewReader(raw)), models.SiteGroup("hddolby"))
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w, resp
	}

	w, resp := put(credentialUpdateRequest{Cookie: credStrPtr("c_secure_uid=1; c_secure_pass=2")})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, true, resp["success"])
	assert.Equal(t, false, resp["enabled"])
	assert.Contains(t, resp["warning"], "API Key")
	assert.Contains(t, resp["warning"], "HD Dolby", "用站点定义里的名字")

	sc, err := srv.store.GetSiteConf(models.SiteGroup("hddolby"))
	require.NoError(t, err)
	assert.Equal(t, "c_secure_uid=1; c_secure_pass=2", sc.Cookie, "Cookie 已加密存下")
	require.NotNil(t, sc.Enabled)
	assert.False(t, *sc.Enabled, "缺 API Key，站点不启用")
	var row models.SiteSetting
	require.NoError(t, db.Where("name = ?", "hddolby").First(&row).Error)
	assert.Empty(t, row.Cookie, "不写旧的明文列")
	assert.NotEmpty(t, row.CookieEncrypted)

	// 再补上 API Key：凭据齐了，照原来的路径启用站点
	w, resp = put(credentialUpdateRequest{APIKey: credStrPtr("dolby-key")})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Nil(t, resp["warning"])
	sc, err = srv.store.GetSiteConf(models.SiteGroup("hddolby"))
	require.NoError(t, err)
	require.NotNil(t, sc.Enabled)
	assert.True(t, *sc.Enabled)
	assert.Equal(t, "c_secure_uid=1; c_secure_pass=2", sc.Cookie, "补 API Key 不影响已存的 Cookie")
	settleCredentialReload(t, srv)
}

// 校验失败是请求的问题，回 400，不是 500：启用中的站点把 API Key 清空就会校验失败
func TestUpdateSiteCredential_ValidationIs400(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)
	enabled := true
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("mteam"), models.SiteConfig{
		Enabled: &enabled, AuthMethod: "api_key", APIKey: "k", APIUrl: "https://api.m-team.cc",
	}))
	raw, _ := json.Marshal(credentialUpdateRequest{APIKey: credStrPtr("")})
	w := httptest.NewRecorder()
	srv.updateSiteCredential(w, httptest.NewRequest(http.MethodPut, "/api/sites/mteam", bytes.NewReader(raw)), models.SiteGroup("mteam"))
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "API Key 不能为空")
}

func TestMissingCredential(t *testing.T) {
	assert.Equal(t, "", core.MissingCredential("cookie", "a=1", "", ""))
	assert.Equal(t, "cookie", core.MissingCredential("cookie", " ", "", ""))
	assert.Equal(t, "api_key", core.MissingCredential("api_key", "a=1", "", ""))
	assert.Equal(t, "api_key", core.MissingCredential("cookie_and_api_key", "a=1", "", ""))
	assert.Equal(t, "cookie", core.MissingCredential("cookie_and_api_key", "", "k", ""))
	assert.Equal(t, "", core.MissingCredential("cookie_and_api_key", "a=1", "k", ""))
	assert.Equal(t, "passkey", core.MissingCredential("passkey", "", "", ""))
}

// 扩展的定时同步会推浏览器里登录着、但 pt-tools 里还没启用的站点，带 keep_enabled：停用的站点只存凭据、保持停用，
// 不回传 Cookie；启用中的站点照旧更新。以前「全部站点」只推已启用的站点，而启用又要先有 Cookie —— 新站点两头都等着对方。
func TestUpdateSiteCredential_KeepEnabled(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)
	require.NoError(t, srv.store.SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, AutoStart: true,
	}))
	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", DisplayName: "hdsky", AuthMethod: "cookie", IsBuiltin: true}).Error)
	keep := true

	put := func(site string, body credentialUpdateRequest) (*httptest.ResponseRecorder, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		srv.updateSiteCredential(w, httptest.NewRequest(http.MethodPut, "/api/sites/"+site, bytes.NewReader(raw)), models.SiteGroup(site))
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w, resp
	}

	w, resp := put("hdsky", credentialUpdateRequest{Cookie: credStrPtr("uid=1; pass=2"), KeepEnabled: &keep})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, true, resp["success"])
	assert.Equal(t, false, resp["enabled"])
	assert.Nil(t, resp["warning"], "按请求不启用，不是缺凭据")
	assert.NotContains(t, w.Body.String(), "pass=2", "响应里不回传 Cookie")

	sc, err := srv.store.GetSiteConf(models.SiteGroup("hdsky"))
	require.NoError(t, err)
	assert.Equal(t, "uid=1; pass=2", sc.Cookie, "Cookie 存下了")
	require.NotNil(t, sc.Enabled)
	assert.False(t, *sc.Enabled, "站点保持停用")
	var row models.SiteSetting
	require.NoError(t, db.Where("name = ?", "hdsky").First(&row).Error)
	assert.Empty(t, row.Cookie, "不写旧的明文列")
	assert.NotEmpty(t, row.CookieEncrypted)

	// 启用中的站点带 keep_enabled：照旧更新凭据，站点仍是启用
	enabled := true
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("hdtime"), models.SiteConfig{
		Enabled: &enabled, AuthMethod: "cookie", Cookie: "old=1", APIUrl: "https://hdtime.org",
	}))
	w, resp = put("hdtime", credentialUpdateRequest{Cookie: credStrPtr("new=2"), KeepEnabled: &keep})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, true, resp["success"])
	sc, err = srv.store.GetSiteConf(models.SiteGroup("hdtime"))
	require.NoError(t, err)
	assert.Equal(t, "new=2", sc.Cookie)
	require.NotNil(t, sc.Enabled)
	assert.True(t, *sc.Enabled)
	settleCredentialReload(t, srv)
}
