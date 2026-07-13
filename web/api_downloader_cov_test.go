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
	"github.com/sunerpy/pt-tools/scheduler"
)

func TestNormalizeDownloaderURL(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"adds scheme", "localhost:8080", "http://localhost:8080", false},
		{"keeps https", "https://qb.example.com", "https://qb.example.com", false},
		{"trims trailing slash", "http://host:9091/", "http://host:9091", false},
		{"empty", "  ", "", true},
		{"invalid scheme", "ftp://host", "", true},
		{"has credentials", "http://u:p@host", "", true},
		{"has fragment", "http://host#frag", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeDownloaderURL(tt.in)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestApiSiteDownloaderSummary(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}, &models.DownloaderSetting{}))

	dl := models.DownloaderSetting{Name: "qb1", Type: "qbittorrent", Enabled: true}
	require.NoError(t, db.Create(&dl).Error)
	require.NoError(t, db.Create(&models.SiteSetting{
		Name: "hdsky", DisplayName: "HDSky", Enabled: true, DownloaderID: &dl.ID,
	}).Error)

	t.Run("GET returns summary", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/downloader-summary", nil)
		server.apiSiteDownloaderSummary(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp SiteDownloaderSummaryResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Len(t, resp.Sites, 1)
		assert.Equal(t, "hdsky", resp.Sites[0].SiteName)
		require.NotNil(t, resp.Sites[0].DownloaderName)
		assert.Equal(t, "qb1", *resp.Sites[0].DownloaderName)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/downloader-summary", nil)
		server.apiSiteDownloaderSummary(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApplyDownloaderToSites(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}, &models.DownloaderSetting{}, &models.RSSSubscription{}))

	dl := models.DownloaderSetting{Name: "qb1", Type: "qbittorrent", Enabled: true}
	require.NoError(t, db.Create(&dl).Error)
	s1 := models.SiteSetting{Name: "hdsky", DisplayName: "HDSky", Enabled: true}
	require.NoError(t, db.Create(&s1).Error)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/1/apply-to-sites", nil)
		server.applyDownloaderToSites(w, req, "1")
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("invalid downloader id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/abc/apply-to-sites", nil)
		server.applyDownloaderToSites(w, req, "abc")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("downloader not found", func(t *testing.T) {
		body, _ := json.Marshal(ApplyDownloaderToSitesRequest{SiteIDs: []uint{s1.ID}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/999/apply-to-sites", bytes.NewReader(body))
		server.applyDownloaderToSites(w, req, "999")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("invalid body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/apply-to-sites", bytes.NewBufferString(`{bad`))
		server.applyDownloaderToSites(w, req, "1")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("empty site ids", func(t *testing.T) {
		body, _ := json.Marshal(ApplyDownloaderToSitesRequest{SiteIDs: nil})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/apply-to-sites", bytes.NewReader(body))
		server.applyDownloaderToSites(w, req, "1")
		require.Equal(t, http.StatusOK, w.Code)

		var resp ApplyDownloaderToSitesResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 0, resp.UpdatedCount)
	})

	t.Run("applies to sites", func(t *testing.T) {
		body, _ := json.Marshal(ApplyDownloaderToSitesRequest{SiteIDs: []uint{s1.ID}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/apply-to-sites", bytes.NewReader(body))
		server.applyDownloaderToSites(w, req, "1")
		require.Equal(t, http.StatusOK, w.Code)

		var resp ApplyDownloaderToSitesResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp.UpdatedCount)
	})
}

func TestDownloaderHealthCheck(t *testing.T) {
	server, db := setupTestServer(t)
	mgr := scheduler.NewManager()
	t.Cleanup(func() { mgr.StopAll() })
	server.mgr = mgr

	dlDisabled := models.DownloaderSetting{Name: "off", Type: "qbittorrent", Enabled: false}
	require.NoError(t, db.Create(&dlDisabled).Error)
	dlBadType := models.DownloaderSetting{Name: "bad", Type: "unknown-type", Enabled: true}
	require.NoError(t, db.Create(&dlBadType).Error)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/health", nil)
		server.downloaderHealthCheck(w, req, "1")
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("invalid id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/abc/health", nil)
		server.downloaderHealthCheck(w, req, "abc")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/999/health", nil)
		server.downloaderHealthCheck(w, req, "999")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("disabled downloader", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/1/health", nil)
		server.downloaderHealthCheck(w, req, "1")
		require.Equal(t, http.StatusOK, w.Code)

		var resp HealthCheckResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.False(t, resp.IsHealthy)
	})

	t.Run("unsupported type", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/2/health", nil)
		server.downloaderHealthCheck(w, req, "2")
		require.Equal(t, http.StatusOK, w.Code)

		var resp HealthCheckResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.False(t, resp.IsHealthy)
		assert.Contains(t, resp.Message, "不支持")
	})
}

func TestApiDownloaderDetail_Routing(t *testing.T) {
	server, db := setupTestServer(t)
	mgr := scheduler.NewManager()
	t.Cleanup(func() { mgr.StopAll() })
	server.mgr = mgr

	require.NoError(t, db.Create(&models.DownloaderSetting{Name: "qb1", Type: "qbittorrent", Enabled: false}).Error)

	t.Run("get detail", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/1", nil)
		server.apiDownloaderDetail(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("invalid id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/abc", nil)
		server.apiDownloaderDetail(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("health route", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/1/health", nil)
		server.apiDownloaderDetail(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}
