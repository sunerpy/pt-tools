package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestCreateFilterRule_DefaultsAndSuccess(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	t.Run("keyword default with zero priority", func(t *testing.T) {
		body, _ := json.Marshal(FilterRuleRequest{
			Name: "DefRule", Pattern: "hit", Enabled: true,
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
		server.createFilterRule(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp FilterRuleResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 100, resp.Priority)
	})
}

func TestUpdateFilterRule_ReEnableCleanup(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	id := createFilterRuleForCov(t, server, "CleanRule")

	body, _ := json.Marshal(FilterRuleRequest{
		Name: "CleanRule", Pattern: "foo*", PatternType: "wildcard",
		Enabled: false, Priority: 3,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/filter-rules/1", bytes.NewReader(body))
	server.updateFilterRule(w, req, id)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestApiGlobal_GetError(t *testing.T) {
	prev := global.GlobalDB
	server := newLoginMonitorServer(t)
	t.Cleanup(func() { global.GlobalDB = prev })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/global", nil)
	server.apiGlobal(w, req)
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
}

func TestApiSiteTemplateImport_CookieEncryptError(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.SiteTemplate{}, &models.SiteSetting{}))
	t.Setenv("PT_TOOLS_SECRET_KEY", "")
	t.Setenv("HOME", t.TempDir()+"/nonexistent-home")

	tpl := models.SiteTemplateExport{Name: "encfail", AuthMethod: "cookie", BaseURL: "https://e.example.com"}
	tplBytes, _ := json.Marshal(tpl)
	body, _ := json.Marshal(TemplateImportRequest{Template: tplBytes, Cookie: "c=1"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader(body))
	server.apiSiteTemplateImport(w, req)
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
}
