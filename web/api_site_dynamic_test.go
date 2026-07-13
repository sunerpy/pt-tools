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
)

func TestApiDynamicSites_List(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}))
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{
		Name: "hdsky", DisplayName: "HDSky", BaseURL: "https://hdsky.me", Enabled: true, AuthMethod: "cookie",
	}).Error)

	t.Run("GET lists sites", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/dynamic", nil)
		server.apiDynamicSites(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp []DynamicSiteResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/sites/dynamic", nil)
		server.apiDynamicSites(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}
