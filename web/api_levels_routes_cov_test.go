package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/sunerpy/pt-tools/site/v2/definitions"
)

func TestApiAllSiteLevels(t *testing.T) {
	s := &Server{}

	t.Run("GET returns all levels", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v2/sites/levels", nil)
		s.apiAllSiteLevels(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp AllSiteLevelsResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp.Sites)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/sites/levels", nil)
		s.apiAllSiteLevels(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("router dispatches levels", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v2/sites/levels", nil)
		s.apiSiteLevelsRouter(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestSetChatOpsDeps_And_SessionChecker(t *testing.T) {
	s := &Server{sessions: map[string]string{"valid": "admin"}}

	s.SetChatOpsDeps(&ChatOpsDeps{})
	assert.NotNil(t, s.chatopsDeps)

	t.Run("no cookie -> false", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		assert.False(t, s.sessionChecker(httptest.NewRecorder(), req))
	})

	t.Run("valid session -> true", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: "valid"})
		assert.True(t, s.sessionChecker(httptest.NewRecorder(), req))
	})

	t.Run("unknown session -> false", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: "bogus"})
		assert.False(t, s.sessionChecker(httptest.NewRecorder(), req))
	})
}

func TestRegisterChatOpsIfWired_NoDeps(t *testing.T) {
	s := &Server{sessions: map[string]string{}}
	mux := http.NewServeMux()
	s.registerChatOpsIfWired(mux)
	// With no deps, no routes registered; a chatops path should 404.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/chatops/notifications", nil)
	mux.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestApiSiteFreeTorrents(t *testing.T) {
	s := &Server{}

	t.Run("download placeholder", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/site/hdsky/free-torrents/download", nil)
		s.apiSiteFreeTorrentsDownload(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("download bad archive type", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/site/hdsky/free-torrents/download?type=rar", nil)
		s.apiSiteFreeTorrentsDownload(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("download method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/site/hdsky/free-torrents/download", nil)
		s.apiSiteFreeTorrentsDownload(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("list placeholder", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/site/hdsky/free-torrents", nil)
		s.apiSiteFreeTorrentsList(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("list method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/site/hdsky/free-torrents", nil)
		s.apiSiteFreeTorrentsList(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApiSiteTemplateImport_BadInput(t *testing.T) {
	server, _ := setupTestServer(t)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/templates/import", nil)
		server.apiSiteTemplateImport(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("invalid body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewBufferString(`{bad`))
		server.apiSiteTemplateImport(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid template json", func(t *testing.T) {
		body, _ := json.Marshal(TemplateImportRequest{Template: json.RawMessage(`not-json`)})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader(body))
		server.apiSiteTemplateImport(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing name", func(t *testing.T) {
		body, _ := json.Marshal(TemplateImportRequest{Template: json.RawMessage(`{"auth_method":"cookie"}`)})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader(body))
		server.apiSiteTemplateImport(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("cookie auth missing cookie", func(t *testing.T) {
		body, _ := json.Marshal(TemplateImportRequest{Template: json.RawMessage(`{"name":"x","auth_method":"cookie"}`)})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader(body))
		server.apiSiteTemplateImport(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestApiSiteTemplates_List(t *testing.T) {
	server, db := setupTestServer(t)
	_ = db
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sites/templates", nil)
	server.apiSiteTemplates(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/sites/templates", nil)
	server.apiSiteTemplates(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}
