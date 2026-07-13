package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestApiFaviconList_InitsServiceWhenNil(t *testing.T) {
	setupFaviconServer(t)
	prev := faviconService
	faviconService = nil
	t.Cleanup(func() {
		if faviconService != nil {
			close(faviconService.stopCh)
		}
		faviconService = prev
	})

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)
	if v2.GetDefinitionRegistry().GetOrDefault("hdsky") != nil {
		seedFavicon(t, "hdsky", "HDSky")
	}

	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicons", nil)
	s.apiFaviconList(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, faviconService)
}

func TestApiFaviconList_ListsCachedEnabled(t *testing.T) {
	server := setupFaviconServer(t)

	if v2.GetDefinitionRegistry().GetOrDefault("faviconlist1") == nil {
		v2.RegisterSiteDefinition(&v2.SiteDefinition{
			ID: "faviconlist1", Name: "FaviconList1", URLs: []string{"https://f.example.com"},
			FaviconURL: "https://f.example.com/favicon.ico",
		})
	}
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "faviconlist1", Enabled: true}).Error)
	seedFavicon(t, "faviconlist1", "FaviconList1")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicons", nil)
	server.apiFaviconList(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp []SiteFaviconInfo
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	found := false
	for _, info := range resp {
		if info.SiteID == "faviconlist1" {
			found = true
			assert.True(t, info.HasCache)
		}
	}
	assert.True(t, found)
}

func TestHandleCloakConfigGet_Success(t *testing.T) {
	srv, store, cleanup := newCloakTestServer(t)
	defer cleanup()
	require.NoError(t, store.SaveCloakConfig("http://m:8080", "tok", false))

	w := httptest.NewRecorder()
	srv.handleCloakConfigGet(w, cloakAuthedReq(http.MethodGet, "/api/cloak/config", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var out cloakConfigGetResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Equal(t, "http://m:8080", out.Endpoint)
	assert.True(t, out.HasToken)
}
