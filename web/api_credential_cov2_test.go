package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

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
