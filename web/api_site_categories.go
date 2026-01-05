// MIT License
// Copyright (c) 2025 pt-tools

package web

import (
	"net/http"
	"strings"
)

// apiSiteCategories handles GET /api/v2/sites/categories
// Returns all supported sites and their category configurations
func (s *Server) apiSiteCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Get all site categories - already a map[string]SiteCategoriesConfig
	allCategories := GetAllSiteCategories()

	// Return as a map directly (key is site_id)
	writeJSON(w, allCategories)
}

// apiSiteCategoryDetail handles GET /api/v2/sites/:siteId/categories
// Returns category configuration for a specific site
func (s *Server) apiSiteCategoryDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Parse site ID from path: /api/v2/sites/:siteId/categories
	path := strings.TrimPrefix(r.URL.Path, "/api/v2/sites/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "categories" {
		http.Error(w, "无效的路径", http.StatusBadRequest)
		return
	}

	siteID := parts[0]
	if siteID == "" {
		http.Error(w, "站点ID不能为空", http.StatusBadRequest)
		return
	}

	// Get site categories
	config := GetSiteCategories(siteID)
	if config == nil {
		// Return empty categories for unsupported sites
		writeJSON(w, SiteCategoriesConfig{
			SiteID:     siteID,
			SiteName:   siteID,
			Categories: []SiteCategory{},
		})
		return
	}

	writeJSON(w, config)
}

// apiSupportedSites handles GET /api/v2/sites/supported
// Returns list of sites that support category filtering
func (s *Server) apiSupportedSites(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	sites := ListSupportedSites()
	writeJSON(w, map[string][]string{
		"supported_sites": sites,
	})
}
