package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==== merged from api_site_categories_cov_test.go ====
func TestApiSiteCategories_All(t *testing.T) {
	s := &Server{}

	t.Run("GET returns map", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v2/sites/categories", nil)
		s.apiSiteCategories(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]SiteCategoriesConfig
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp)
	})

	t.Run("POST not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/sites/categories", nil)
		s.apiSiteCategories(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApiSiteCategoryDetail(t *testing.T) {
	s := &Server{}

	t.Run("known site", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v2/sites/mteam/categories", nil)
		s.apiSiteCategoryDetail(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp SiteCategoriesConfig
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "mteam", resp.SiteID)
	})

	t.Run("unknown site returns empty categories", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v2/sites/nosuchsite/categories", nil)
		s.apiSiteCategoryDetail(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp SiteCategoriesConfig
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "nosuchsite", resp.SiteID)
		assert.Empty(t, resp.Categories)
	})

	t.Run("empty site id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v2/sites//categories", nil)
		s.apiSiteCategoryDetail(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid path", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v2/sites/mteam/levels", nil)
		s.apiSiteCategoryDetail(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/sites/mteam/categories", nil)
		s.apiSiteCategoryDetail(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApiSupportedSites(t *testing.T) {
	s := &Server{}

	t.Run("GET returns supported sites", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v2/sites/supported", nil)
		s.apiSupportedSites(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string][]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp, "supported_sites")
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/v2/sites/supported", nil)
		s.apiSupportedSites(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApiSiteLevelsRouter_CategoryDispatch(t *testing.T) {
	s := &Server{}

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{"all categories", "/api/v2/sites/categories", http.StatusOK},
		{"supported", "/api/v2/sites/supported", http.StatusOK},
		{"site categories detail", "/api/v2/sites/mteam/categories", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			s.apiSiteLevelsRouter(w, req)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestSiteCategoriesHelpers(t *testing.T) {
	t.Run("GetSiteCategories known", func(t *testing.T) {
		assert.NotNil(t, GetSiteCategories("mteam"))
	})
	t.Run("GetSiteCategories unknown", func(t *testing.T) {
		assert.Nil(t, GetSiteCategories("nosuchsite"))
	})
	t.Run("GetAllSiteCategories not empty", func(t *testing.T) {
		assert.NotEmpty(t, GetAllSiteCategories())
	})
	t.Run("ListSupportedSites not empty", func(t *testing.T) {
		assert.NotEmpty(t, ListSupportedSites())
	})
}
