package web

import (
	"net/http"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// SiteLevelsResponse represents the response for site level requirements API
type SiteLevelsResponse struct {
	SiteID   string                    `json:"siteId"`
	SiteName string                    `json:"siteName"`
	Levels   []v2.SiteLevelRequirement `json:"levels"`
}

// AllSiteLevelsResponse represents the response for all sites' level requirements
type AllSiteLevelsResponse struct {
	Sites map[string]SiteLevelsResponse `json:"sites"`
}

// apiSiteLevels handles GET /api/v2/sites/:siteId/levels
func (s *Server) apiSiteLevels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract site ID from path: /api/v2/sites/{siteId}/levels
	path := strings.TrimPrefix(r.URL.Path, "/api/v2/sites/")
	path = strings.TrimSuffix(path, "/levels")
	siteID := strings.TrimSuffix(path, "/")

	if siteID == "" {
		http.Error(w, "Site ID required", http.StatusBadRequest)
		return
	}

	// Get site definition from registry
	registry := v2.GetDefinitionRegistry()
	def, found := registry.Get(siteID)

	response := SiteLevelsResponse{
		SiteID: siteID,
		Levels: []v2.SiteLevelRequirement{},
	}

	if found && def != nil {
		response.SiteName = def.Name
		response.Levels = def.LevelRequirements
	} else {
		// Try to get site name by title casing
		response.SiteName = cases.Title(language.English).String(siteID)
	}

	writeJSON(w, response)
}

// apiAllSiteLevels handles GET /api/v2/sites/levels
// Returns level requirements for all registered sites
func (s *Server) apiAllSiteLevels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	registry := v2.GetDefinitionRegistry()
	allDefs := registry.GetAll()

	response := AllSiteLevelsResponse{
		Sites: make(map[string]SiteLevelsResponse),
	}

	for _, def := range allDefs {
		if def != nil && len(def.LevelRequirements) > 0 {
			response.Sites[def.ID] = SiteLevelsResponse{
				SiteID:   def.ID,
				SiteName: def.Name,
				Levels:   def.LevelRequirements,
			}
		}
	}

	writeJSON(w, response)
}

// apiSiteLevelsRouter routes /api/v2/sites/* requests
func (s *Server) apiSiteLevelsRouter(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Handle /api/v2/sites/levels - get all sites' levels
	if path == "/api/v2/sites/levels" || path == "/api/v2/sites/levels/" {
		s.apiAllSiteLevels(w, r)
		return
	}

	// Handle /api/v2/sites/categories - get all sites' categories
	if path == "/api/v2/sites/categories" || path == "/api/v2/sites/categories/" {
		s.apiSiteCategories(w, r)
		return
	}

	// Handle /api/v2/sites/supported - get supported sites for category filtering
	if path == "/api/v2/sites/supported" || path == "/api/v2/sites/supported/" {
		s.apiSupportedSites(w, r)
		return
	}

	// Handle /api/v2/sites/{siteId}/levels - get specific site's levels
	if strings.HasSuffix(path, "/levels") || strings.HasSuffix(path, "/levels/") {
		s.apiSiteLevels(w, r)
		return
	}

	// Handle /api/v2/sites/{siteId}/categories - get specific site's categories
	if strings.HasSuffix(path, "/categories") || strings.HasSuffix(path, "/categories/") {
		s.apiSiteCategoryDetail(w, r)
		return
	}

	// Return 404 for other paths under /api/v2/sites/
	http.NotFound(w, r)
}
