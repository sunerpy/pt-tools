package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
	_ "github.com/sunerpy/pt-tools/site/v2/definitions"
)

// ==== merged from api_levels_routes_cov_test.go ====
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

// ==== merged from api_levels_test.go ====
func TestApiSiteLevels_HDSky(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/sites/hdsky/levels", nil)
	w := httptest.NewRecorder()

	s.apiSiteLevels(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var resp SiteLevelsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.SiteID != "hdsky" {
		t.Errorf("Expected siteId 'hdsky', got '%s'", resp.SiteID)
	}

	if resp.SiteName != "HDSky" {
		t.Errorf("Expected siteName 'HDSky', got '%s'", resp.SiteName)
	}

	if len(resp.Levels) == 0 {
		t.Error("Expected levels to be non-empty")
	}

	// Verify first level is User
	if len(resp.Levels) > 0 && resp.Levels[0].Name != "User" {
		t.Errorf("Expected first level to be 'User', got '%s'", resp.Levels[0].Name)
	}
}

func TestApiSiteLevels_MTeam(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/sites/mteam/levels", nil)
	w := httptest.NewRecorder()

	s.apiSiteLevels(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var resp SiteLevelsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.SiteID != "mteam" {
		t.Errorf("Expected siteId 'mteam', got '%s'", resp.SiteID)
	}

	if len(resp.Levels) == 0 {
		t.Error("Expected levels to be non-empty for mteam")
	}
}

func TestApiSiteLevels_UnknownSite(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/sites/unknownsite/levels", nil)
	w := httptest.NewRecorder()

	s.apiSiteLevels(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var resp SiteLevelsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.SiteID != "unknownsite" {
		t.Errorf("Expected siteId 'unknownsite', got '%s'", resp.SiteID)
	}

	// Unknown site should return empty levels array
	if len(resp.Levels) != 0 {
		t.Errorf("Expected empty levels for unknown site, got %d", len(resp.Levels))
	}
}

func TestApiSiteLevels_EmptySiteID(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/sites//levels", nil)
	w := httptest.NewRecorder()

	s.apiSiteLevels(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for empty site ID, got %d", w.Code)
	}
}

func TestApiSiteLevels_MethodNotAllowed(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodPost, "/api/v2/sites/hdsky/levels", nil)
	w := httptest.NewRecorder()

	s.apiSiteLevels(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 for POST, got %d", w.Code)
	}
}

func TestApiSiteLevels_ResponseStructure(t *testing.T) {
	s := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/sites/hdsky/levels", nil)
	w := httptest.NewRecorder()

	s.apiSiteLevels(w, req)

	var resp SiteLevelsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify each level has required fields
	for i, level := range resp.Levels {
		if level.ID == 0 && i > 0 {
			// ID 0 is only valid for first level (User)
			t.Errorf("Level %d has invalid ID: %d", i, level.ID)
		}
		if level.Name == "" {
			t.Errorf("Level %d has empty name", i)
		}
	}
}

func TestApiSiteLevelsRouter(t *testing.T) {
	s := &Server{}

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{"levels endpoint", "/api/v2/sites/hdsky/levels", http.StatusOK},
		{"levels with trailing slash", "/api/v2/sites/hdsky/levels/", http.StatusOK},
		{"unknown path", "/api/v2/sites/hdsky/unknown", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			s.apiSiteLevelsRouter(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Expected status %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}

func TestSiteLevelsResponse_AllSites(t *testing.T) {
	registry := v2.GetDefinitionRegistry()
	sites := registry.List()

	if len(sites) == 0 {
		t.Skip("No sites registered")
	}

	s := &Server{}

	for _, siteID := range sites {
		t.Run(siteID, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v2/sites/"+siteID+"/levels", nil)
			w := httptest.NewRecorder()

			s.apiSiteLevels(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200 for site %s, got %d", siteID, w.Code)
			}

			var resp SiteLevelsResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("Failed to decode response for site %s: %v", siteID, err)
			}

			if resp.SiteID != siteID {
				t.Errorf("Expected siteId '%s', got '%s'", siteID, resp.SiteID)
			}

			if resp.SiteName == "" {
				t.Errorf("Expected non-empty siteName for site %s", siteID)
			}
		})
	}
}
