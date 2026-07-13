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

func TestUpdateFilterRule_PatternTypeOnly(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	id := createFilterRuleForCov(t, server, "PTOnly")

	t.Run("update pattern type only valid", func(t *testing.T) {
		body, _ := json.Marshal(FilterRuleRequest{PatternType: "keyword", Enabled: true})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/filter-rules/1", bytes.NewReader(body))
		server.updateFilterRule(w, req, id)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("update pattern type only invalid existing pattern", func(t *testing.T) {
		bad := createFilterRuleForCov(t, server, "PTBad")
		require.NoError(t, global.GlobalDB.DB.Model(&models.FilterRule{}).
			Where("id = ?", bad).Update("pattern", "[unclosed(").Error)
		body, _ := json.Marshal(FilterRuleRequest{PatternType: "regex", Enabled: true})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/filter-rules/2", bytes.NewReader(body))
		server.updateFilterRule(w, req, bad)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestTestFilterRule_RSSDispatch(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.RSSSubscription{}))
	rssID := uint(999)
	body, _ := json.Marshal(FilterRuleTestRequest{
		Pattern: "x*", PatternType: "wildcard", RSSID: &rssID,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
	server.testFilterRule(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTestFilterRule_LimitClamp(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	require.NoError(t, global.GlobalDB.DB.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "1", Title: "Match Me", Tag: "hd", IsFree: true, TorrentSize: 2 << 30,
	}).Error)

	body, _ := json.Marshal(FilterRuleTestRequest{
		Pattern: "Match*", PatternType: "wildcard", MatchField: "title", Limit: 500,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
	server.testFilterRule(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestApiSiteTemplateImport_ExistingTemplateUpdate(t *testing.T) {
	writeWebTestSecretKey(t)
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.SiteTemplate{}, &models.SiteSetting{}))

	require.NoError(t, db.Create(&models.SiteTemplate{Name: "dupimport", AuthMethod: "cookie"}).Error)

	tpl := models.SiteTemplateExport{Name: "dupimport", AuthMethod: "cookie", BaseURL: "https://d.example.com"}
	tplBytes, _ := json.Marshal(tpl)
	body, _ := json.Marshal(TemplateImportRequest{Template: tplBytes, Cookie: "c=1; d=2"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader(body))
	server.apiSiteTemplateImport(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}
