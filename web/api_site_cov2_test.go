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

func TestApiSiteTemplateImport_Success(t *testing.T) {
	writeWebTestSecretKey(t)
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.SiteTemplate{}, &models.SiteSetting{}))

	tpl := models.SiteTemplateExport{
		Name:        "importcov",
		DisplayName: "Import Cov",
		BaseURL:     "https://importcov.example.com",
		AuthMethod:  "cookie",
	}
	tplBytes, _ := json.Marshal(tpl)
	body, _ := json.Marshal(TemplateImportRequest{
		Template: tplBytes,
		Cookie:   "sess=1; token=2",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader(body))
	server.apiSiteTemplateImport(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp DynamicSiteResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "importcov", resp.Name)
}

func TestApiSiteTemplateImport_MissingAuthAndCredentials(t *testing.T) {
	writeWebTestSecretKey(t)
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.SiteTemplate{}, &models.SiteSetting{}))

	t.Run("missing auth method", func(t *testing.T) {
		body, _ := json.Marshal(TemplateImportRequest{Template: json.RawMessage(`{"name":"x"}`)})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader(body))
		server.apiSiteTemplateImport(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("api_key auth missing key", func(t *testing.T) {
		body, _ := json.Marshal(TemplateImportRequest{Template: json.RawMessage(`{"name":"x","auth_method":"api_key"}`)})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader(body))
		server.apiSiteTemplateImport(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("passkey auth missing passkey", func(t *testing.T) {
		body, _ := json.Marshal(TemplateImportRequest{Template: json.RawMessage(`{"name":"x","auth_method":"passkey"}`)})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader(body))
		server.apiSiteTemplateImport(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("cookie_and_api_key missing both", func(t *testing.T) {
		body, _ := json.Marshal(TemplateImportRequest{Template: json.RawMessage(`{"name":"x","auth_method":"cookie_and_api_key"}`)})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader(body))
		server.apiSiteTemplateImport(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestListDynamicSites_WithData(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", DisplayName: "HDSky", Enabled: true}).Error)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "customsite", DisplayName: "Custom", Enabled: true, IsBuiltin: false}).Error)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sites/dynamic", nil)
	server.listDynamicSites(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp []DynamicSiteResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.GreaterOrEqual(t, len(resp), 2)
}
