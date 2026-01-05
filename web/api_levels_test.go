package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	v2 "github.com/sunerpy/pt-tools/site/v2"
	// Import definitions to register them
	_ "github.com/sunerpy/pt-tools/site/v2/definitions"
)

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
