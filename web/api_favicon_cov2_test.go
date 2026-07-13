package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestApiFavicon_FetchThroughDefinitionOnMiss(t *testing.T) {
	server := setupFaviconServer(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47, 5, 6})
	}))
	defer ts.Close()

	registerOrRefreshFaviconDef(t, "covfetchsite", "CovFetchSite", ts.URL)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicon/covfetchsite", nil)
	server.apiFavicon(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Body.Bytes())
}

func TestApiFavicon_DefinitionFetchFailsPlaceholder(t *testing.T) {
	server := setupFaviconServer(t)

	if v2.GetDefinitionRegistry().GetOrDefault("covdeadsite") == nil {
		v2.RegisterSiteDefinition(&v2.SiteDefinition{
			ID:         "covdeadsite",
			Name:       "CovDeadSite",
			URLs:       []string{"http://127.0.0.1:1"},
			FaviconURL: "http://127.0.0.1:1/favicon.ico",
		})
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicon/covdeadsite", nil)
	server.apiFavicon(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "1", w.Header().Get("X-Favicon-Placeholder"))
}

func TestApiFavicon_EmptySiteAndMethod(t *testing.T) {
	server := setupFaviconServer(t)

	t.Run("empty site id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/favicon/", nil)
		server.apiFavicon(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/favicon/hdsky", nil)
		server.apiFavicon(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("unknown site no definition placeholder", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/favicon/definitely-no-such-site-xyz", nil)
		server.apiFavicon(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "1", w.Header().Get("X-Favicon-Placeholder"))
	})
}

func TestApiFaviconRefresh_Success(t *testing.T) {
	server := setupFaviconServer(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47, 7})
	}))
	defer ts.Close()

	registerOrRefreshFaviconDef(t, "covrefreshsite", "CovRefreshSite", ts.URL)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/favicon/covrefreshsite/refresh", nil)
	server.apiFaviconRefresh(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "ok")
}

func TestApiFaviconRefresh_Errors(t *testing.T) {
	server := setupFaviconServer(t)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/favicon/hdsky/refresh", nil)
		server.apiFaviconRefresh(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("empty site id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/favicon//refresh", nil)
		server.apiFaviconRefresh(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("unknown site returns 404", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/favicon/no-such-site-refresh-xyz/refresh", nil)
		server.apiFaviconRefresh(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("fetch failure returns 500", func(t *testing.T) {
		if v2.GetDefinitionRegistry().GetOrDefault("covdeadrefresh") == nil {
			v2.RegisterSiteDefinition(&v2.SiteDefinition{
				ID:         "covdeadrefresh",
				Name:       "CovDeadRefresh",
				URLs:       []string{"http://127.0.0.1:1"},
				FaviconURL: "http://127.0.0.1:1/favicon.ico",
			})
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/favicon/covdeadrefresh/refresh", nil)
		server.apiFaviconRefresh(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestRefreshExpiredFavicons_NewSiteAndExpired(t *testing.T) {
	setupFaviconServer(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47, 8})
	}))
	defer ts.Close()

	registerOrRefreshFaviconDef(t, "covexpiredsite", "CovExpiredSite", ts.URL)

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "covexpiredsite", Enabled: true}).Error)

	fs := &FaviconService{refreshInterval: time.Nanosecond}
	fs.refreshExpiredFavicons()

	var count int64
	global.GlobalDB.DB.Model(&models.FaviconCache{}).Where("site_id = ?", "covexpiredsite").Count(&count)
	assert.Equal(t, int64(1), count)
}
