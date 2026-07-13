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
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestLoginHandler_LegacyAndErrors(t *testing.T) {
	srv := setupServer(t)

	t.Run("wrong password legacy fallback fails", func(t *testing.T) {
		require.NoError(t, srv.store.EnsureAdmin("legacyuser", "not-a-valid-hash-format"))
		body, _ := json.Marshal(map[string]string{"username": "legacyuser", "password": "whatever"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		srv.loginHandler(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("bad json body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(`{bad`))
		req.Header.Set("Content-Type", "application/json")
		srv.loginHandler(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestApiSites_UnavailableSiteDisabled(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)

	if v2.GetDefinitionRegistry().GetOrDefault("covunavail") == nil {
		v2.RegisterSiteDefinition(&v2.SiteDefinition{
			ID:                "covunavail",
			Name:              "CovUnavail",
			URLs:              []string{"https://covunavail.example.com"},
			Unavailable:       true,
			UnavailableReason: "test",
		})
	}

	enabled := true
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("covunavail"), models.SiteConfig{
		Enabled: &enabled, AuthMethod: "cookie", Cookie: "c=1", APIUrl: "https://covunavail.example.com",
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
	srv.apiSites(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]SiteConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	entry, ok := resp["covunavail"]
	require.True(t, ok)
	assert.True(t, entry.Unavailable)
	require.NotNil(t, entry.Enabled)
	assert.False(t, *entry.Enabled)
}

func TestListDynamicSites_UnavailableSite(t *testing.T) {
	server, db := setupTestServer(t)

	if v2.GetDefinitionRegistry().GetOrDefault("covunavail2") == nil {
		v2.RegisterSiteDefinition(&v2.SiteDefinition{
			ID:                "covunavail2",
			Name:              "CovUnavail2",
			URLs:              []string{"https://covunavail2.example.com"},
			Unavailable:       true,
			UnavailableReason: "test",
		})
	}
	require.NoError(t, db.Create(&models.SiteSetting{Name: "covunavail2", DisplayName: "CU2", Enabled: true}).Error)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sites/dynamic", nil)
	server.listDynamicSites(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp []DynamicSiteResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	found := false
	for _, r := range resp {
		if r.Name == "covunavail2" {
			found = true
			assert.True(t, r.Unavailable)
			assert.False(t, r.Enabled)
		}
	}
	assert.True(t, found)
}

func TestApiGlobal_PostSaveAndValidation(t *testing.T) {
	writeWebTestSecretKey(t)
	server := setupServer(t)

	t.Run("empty download dir", func(t *testing.T) {
		body := `{"default_interval_minutes":10}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/global", bytes.NewBufferString(body))
		server.apiGlobal(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("bad body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/global", bytes.NewBufferString(`{bad`))
		server.apiGlobal(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("valid save", func(t *testing.T) {
		body := `{"default_interval_minutes":30,"download_dir":"downloads"}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/global", bytes.NewBufferString(body))
		server.apiGlobal(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/global", nil)
		server.apiGlobal(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}
