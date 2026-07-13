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

func TestApiDynamicSites_Dispatch(t *testing.T) {
	writeWebTestSecretKey(t)
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}))

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/sites/dynamic", nil)
		server.apiDynamicSites(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("get list", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/dynamic", nil)
		server.apiDynamicSites(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("create bad body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/dynamic", bytes.NewBufferString(`{bad`))
		server.apiDynamicSites(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("create missing name", func(t *testing.T) {
		body, _ := json.Marshal(DynamicSiteRequest{AuthMethod: "cookie"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/dynamic", bytes.NewReader(body))
		server.apiDynamicSites(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("create missing auth", func(t *testing.T) {
		body, _ := json.Marshal(DynamicSiteRequest{Name: "x"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/dynamic", bytes.NewReader(body))
		server.apiDynamicSites(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("create success with cookie", func(t *testing.T) {
		body, _ := json.Marshal(DynamicSiteRequest{
			Name: "dynsite", DisplayName: "Dyn", BaseURL: "https://dyn.example.com",
			AuthMethod: "cookie", Cookie: "c=1; d=2",
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/dynamic", bytes.NewReader(body))
		server.apiDynamicSites(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp DynamicSiteResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "dynsite", resp.Name)
	})
}

func TestApiSiteDefinitions_Cov(t *testing.T) {
	server, _ := setupTestServer(t)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/definitions", nil)
		server.apiSiteDefinitions(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("get", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/definitions", nil)
		server.apiSiteDefinitions(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})
}
