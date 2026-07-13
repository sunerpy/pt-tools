package web

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// ==== merged from api_downloader_cov2_test.go ====
func newDownloaderCovServer(t *testing.T) *Server {
	t.Helper()
	server, _ := setupTestServer(t)
	mgr := scheduler.NewManager()
	t.Cleanup(func() { mgr.StopAll() })
	server.mgr = mgr
	return server
}

func TestCreateDownloader_Branches(t *testing.T) {
	server := newDownloaderCovServer(t)

	t.Run("bad body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewBufferString(`{bad`))
		server.createDownloader(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("empty name", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{Type: "qbittorrent", URL: "http://x"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
		server.createDownloader(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("empty type", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{Name: "n", URL: "http://x"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
		server.createDownloader(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("bad type", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{Name: "n", Type: "aria2", URL: "http://x"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
		server.createDownloader(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("empty url", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{Name: "n", Type: "qbittorrent"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
		server.createDownloader(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("bad url", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{Name: "n", Type: "qbittorrent", URL: "ftp://x"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
		server.createDownloader(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("first downloader becomes default", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{Name: "first-dl", Type: "qbittorrent", URL: "127.0.0.1:8080", Enabled: true})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
		server.createDownloader(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp DownloaderResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.True(t, resp.IsDefault)
	})
}

func TestCreateDownloader_DuplicateName(t *testing.T) {
	server := newDownloaderCovServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.DownloaderSetting{
		Name: "dup-dl", Type: "qbittorrent", URL: "http://127.0.0.1:7070", Enabled: true,
	}).Error)
	body, _ := json.Marshal(DownloaderRequest{Name: "dup-dl", Type: "qbittorrent", URL: "http://127.0.0.1:9090"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
	server.createDownloader(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateDownloader_Branches(t *testing.T) {
	server := newDownloaderCovServer(t)
	db := global.GlobalDB.DB

	def := models.DownloaderSetting{Name: "def", Type: "qbittorrent", URL: "http://127.0.0.1:1", IsDefault: true, Enabled: true}
	require.NoError(t, db.Create(&def).Error)
	other := models.DownloaderSetting{Name: "other", Type: "qbittorrent", URL: "http://127.0.0.1:2", Enabled: true}
	require.NoError(t, db.Create(&other).Error)

	t.Run("bad body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/1", bytes.NewBufferString(`{bad`))
		server.updateDownloader(w, req, 1)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("not found", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{Name: "x", Type: "qbittorrent", URL: "http://x", Enabled: true})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/999", bytes.NewReader(body))
		server.updateDownloader(w, req, 999)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("cannot unset only default", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{Name: "def", Type: "qbittorrent", URL: "http://127.0.0.1:1", IsDefault: false, Enabled: true})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/1", bytes.NewReader(body))
		server.updateDownloader(w, req, def.ID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("bad url on update", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{Name: "other", Type: "qbittorrent", URL: "ftp://bad", Enabled: true})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/2", bytes.NewReader(body))
		server.updateDownloader(w, req, other.ID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("promote other to default", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{Name: "other", Type: "qbittorrent", URL: "http://127.0.0.1:2", IsDefault: true, Enabled: true})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/2", bytes.NewReader(body))
		server.updateDownloader(w, req, other.ID)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestDeleteDownloader_Branches(t *testing.T) {
	server := newDownloaderCovServer(t)
	db := global.GlobalDB.DB

	def := models.DownloaderSetting{Name: "d1", Type: "qbittorrent", URL: "http://127.0.0.1:1", IsDefault: true, Enabled: true}
	require.NoError(t, db.Create(&def).Error)
	extra := models.DownloaderSetting{Name: "d2", Type: "qbittorrent", URL: "http://127.0.0.1:2", Enabled: true}
	require.NoError(t, db.Create(&extra).Error)

	t.Run("not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/downloaders/999", nil)
		server.deleteDownloader(w, req, 999)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("cannot delete default with others", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/downloaders/1", nil)
		server.deleteDownloader(w, req, def.ID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("delete non-default succeeds", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/downloaders/2", nil)
		server.deleteDownloader(w, req, extra.ID)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestListDownloaders_WithData(t *testing.T) {
	server := newDownloaderCovServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.DownloaderSetting{
		Name: "ld1", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true,
	}).Error)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/downloaders", nil)
	server.listDownloaders(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp []DownloaderResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.GreaterOrEqual(t, len(resp), 1)
}

// ==== merged from api_downloader_cov3_test.go ====
func TestApiDownloaders_Dispatch(t *testing.T) {
	server := newDownloaderCovServer(t)

	t.Run("get list", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders", nil)
		server.apiDownloaders(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/api/downloaders", nil)
		server.apiDownloaders(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApiDownloaderDetail_Dispatch(t *testing.T) {
	server := newDownloaderCovServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.DownloaderSetting{
		Name: "d1", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true, IsDefault: true,
	}).Error)

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

	t.Run("get not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/999", nil)
		server.apiDownloaderDetail(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("health route", func(t *testing.T) {
		mgr := scheduler.NewManager()
		t.Cleanup(func() { mgr.StopAll() })
		server.mgr = mgr
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/1/health", nil)
		server.apiDownloaderDetail(w, req)
		assert.True(t, w.Code == http.StatusOK)
	})

	t.Run("set-default route", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/set-default", nil)
		server.apiDownloaderDetail(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/api/downloaders/1", nil)
		server.apiDownloaderDetail(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApiSiteRouter_Dispatch(t *testing.T) {
	s := &Server{}

	t.Run("unknown endpoint", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/site/hdsky/unknown", nil)
		s.apiSiteRouter(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("free-torrents list route", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/site/hdsky/free-torrents", nil)
		s.apiSiteRouter(w, req)
		assert.NotEqual(t, http.StatusNotFound, w.Code)
	})
}

func TestApiSiteDownloaderSummary_WithData(t *testing.T) {
	server, db := setupTestServer(t)
	dlID := uint(1)
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "sum-dl", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&models.SiteSetting{
		Name: "hdsky", DisplayName: "HDSky", Enabled: true, DownloaderID: &dlID,
	}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/downloader-summary", nil)
		server.apiSiteDownloaderSummary(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("get summary", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/downloader-summary", nil)
		server.apiSiteDownloaderSummary(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp SiteDownloaderSummaryResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.GreaterOrEqual(t, len(resp.Sites), 1)
	})
}

func TestApplyDownloaderToSites_Success(t *testing.T) {
	server := newDownloaderCovServer(t)
	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "apply-dl", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	body, _ := json.Marshal(ApplyDownloaderToSitesRequest{SiteIDs: []uint{1}})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/apply-to-sites", bytes.NewReader(body))
	server.applyDownloaderToSites(w, req, "1")
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)

	t.Run("empty site ids", func(t *testing.T) {
		b, _ := json.Marshal(ApplyDownloaderToSitesRequest{})
		ww := httptest.NewRecorder()
		rr := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/apply-to-sites", bytes.NewReader(b))
		server.applyDownloaderToSites(ww, rr, "1")
		assert.Equal(t, http.StatusOK, ww.Code)
	})

	t.Run("invalid id", func(t *testing.T) {
		ww := httptest.NewRecorder()
		rr := httptest.NewRequest(http.MethodPost, "/api/downloaders/abc/apply-to-sites", nil)
		server.applyDownloaderToSites(ww, rr, "abc")
		assert.Equal(t, http.StatusBadRequest, ww.Code)
	})

	t.Run("downloader not found", func(t *testing.T) {
		b, _ := json.Marshal(ApplyDownloaderToSitesRequest{SiteIDs: []uint{1}})
		ww := httptest.NewRecorder()
		rr := httptest.NewRequest(http.MethodPost, "/api/downloaders/999/apply-to-sites", bytes.NewReader(b))
		server.applyDownloaderToSites(ww, rr, "999")
		assert.Equal(t, http.StatusNotFound, ww.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		ww := httptest.NewRecorder()
		rr := httptest.NewRequest(http.MethodGet, "/api/downloaders/1/apply-to-sites", nil)
		server.applyDownloaderToSites(ww, rr, "1")
		assert.Equal(t, http.StatusMethodNotAllowed, ww.Code)
	})
}

// ==== merged from api_downloader_cov_test.go ====
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

// ==== merged from api_downloader_disk2_test.go ====
func TestReserveDownloaderAddDiskBudget_MoreBranches(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.SettingsGlobal{}))
	gs, err := server.store.GetGlobalSettings()
	require.NoError(t, err)
	require.NoError(t, server.store.SaveGlobalSettings(gs))
	require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
		Where("1 = 1").Updates(map[string]any{"cleanup_disk_protect": true, "cleanup_min_disk_space_gb": 10}).Error)

	ctx := context.Background()

	t.Run("invalid torrent bytes", func(t *testing.T) {
		fake := &fakeDownloader{freSpace: 100 << 30, name: "qb1"}
		_, err := reserveDownloaderAddDiskBudget(ctx, fake, []byte("not-bencode"), "")
		assert.Error(t, err)
	})

	t.Run("insufficient space rejects", func(t *testing.T) {
		fake := &fakeDownloader{freSpace: 1 << 30, name: "qb1"}
		_, err := reserveDownloaderAddDiskBudget(ctx, fake, minimalTorrentBytes(5<<30), "")
		assert.Error(t, err)
	})
}

func TestApiDownloaderTorrentDetail_FilesFallback(t *testing.T) {
	fake := &fakeDownloader{
		torrents: sampleTorrents(),
		getErr:   nil,
	}
	server, _ := setupServerWithFakeDownloader(t, fake)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/detail?downloader_id=1&task_id=t1", nil)
	server.apiDownloaderTorrentDetail(w, req)
	require.Equal(t, 200, w.Code)
}

// ==== merged from api_downloader_disk_cov3_test.go ====
func TestApiAddDownloaderTorrent_DiskProtectSpaceLow(t *testing.T) {
	fake := &fakeDownloader{freSpace: 1024}
	server, _ := setupServerWithFakeDownloader(t, fake)
	gs, err := server.store.GetGlobalSettings()
	require.NoError(t, err)
	require.NoError(t, server.store.SaveGlobalSettings(gs))
	require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
		Where("1 = 1").Updates(map[string]any{
		"cleanup_disk_protect":      true,
		"cleanup_min_disk_space_gb": 100.0,
	}).Error)

	torrentB64 := base64.StdEncoding.EncodeToString(minimalTorrentBytes(50 * 1024 * 1024 * 1024))
	body, _ := json.Marshal(AddDownloaderTorrentRequest{
		DownloaderIDs: []uint{1},
		TorrentBase64: torrentB64,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/add", bytes.NewReader(body))
	server.apiAddDownloaderTorrent(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp AddDownloaderTorrentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.FailedCount)
}

// ==== merged from api_downloader_disk_test.go ====
// minimalTorrentBytes returns a bencoded single-file torrent whose info.length
// equals the requested size. Sufficient for qbit.ComputeTorrentSize.
func minimalTorrentBytes(length int64) []byte {
	var b bytes.Buffer
	b.WriteString("d4:infod6:lengthi")
	b.WriteString(torrentItoa(length))
	b.WriteString("e4:name4:test12:piece lengthi16384e6:pieces0:ee")
	return b.Bytes()
}

func torrentItoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func TestReserveDownloaderAddDiskBudget(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.SettingsGlobal{}))

	fake := &fakeDownloader{freSpace: 100 * 1024 * 1024 * 1024, name: "qb1"}
	ctx := context.Background()

	t.Run("disk protect off returns 0", func(t *testing.T) {
		gs, err := server.store.GetGlobalSettings()
		require.NoError(t, err)
		require.NoError(t, server.store.SaveGlobalSettings(gs))
		require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
			Where("1 = 1").Update("cleanup_disk_protect", false).Error)

		size, err := reserveDownloaderAddDiskBudget(ctx, fake, minimalTorrentBytes(1024), "")
		require.NoError(t, err)
		assert.Equal(t, int64(0), size)
	})

	t.Run("disk protect on rejects magnet without bytes", func(t *testing.T) {
		gs, err := server.store.GetGlobalSettings()
		require.NoError(t, err)
		require.NoError(t, server.store.SaveGlobalSettings(gs))
		require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
			Where("1 = 1").Updates(map[string]any{"cleanup_disk_protect": true, "cleanup_min_disk_space_gb": 10}).Error)

		_, err = reserveDownloaderAddDiskBudget(ctx, fake, nil, "magnet:?xt=urn:btih:x")
		assert.Error(t, err)
	})

	t.Run("disk protect on with sufficient space reserves", func(t *testing.T) {
		gs, err := server.store.GetGlobalSettings()
		require.NoError(t, err)
		require.NoError(t, server.store.SaveGlobalSettings(gs))
		require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
			Where("1 = 1").Updates(map[string]any{"cleanup_disk_protect": true, "cleanup_min_disk_space_gb": 10}).Error)

		size, err := reserveDownloaderAddDiskBudget(ctx, fake, minimalTorrentBytes(1024), "")
		require.NoError(t, err)
		assert.Equal(t, int64(1024), size)
	})
}

func TestApiAddDownloaderTorrent_Base64Path(t *testing.T) {
	fake := &fakeDownloader{
		addResult: downloader.AddTorrentResult{Success: true, ID: "n1", Hash: "h1", Message: "ok"},
		freSpace:  1 << 40,
	}
	server, _ := setupServerWithFakeDownloader(t, fake)
	require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
		Where("1 = 1").Update("cleanup_disk_protect", false).Error)

	torrentB64 := base64.StdEncoding.EncodeToString(minimalTorrentBytes(2048))
	body, _ := json.Marshal(AddDownloaderTorrentRequest{
		DownloaderIDs: []uint{1},
		TorrentBase64: torrentB64,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/add", bytes.NewReader(body))
	server.apiAddDownloaderTorrent(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp AddDownloaderTorrentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.SuccessCount)
}

// ==== merged from api_downloader_more_cov_test.go ====
func TestApiSiteLoginStateRouter_Dispatch(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{
		Name: "hdsky", DisplayName: "HDSky", Enabled: true, BaseURL: "https://hdsky.me",
	}).Error)

	t.Run("empty site name", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiSiteLoginStateRouter(rec, authedRequest(http.MethodGet, "/api/sites/login-state/", nil))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("get login state", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiSiteLoginStateRouter(rec, authedRequest(http.MethodGet, "/api/sites/login-state/hdsky", nil))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("unknown action", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiSiteLoginStateRouter(rec, authedRequest(http.MethodGet, "/api/sites/login-state/hdsky/bogus", nil))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("config action bad body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/sites/login-state/hdsky/config", bytes.NewBufferString(`{bad`))
		req.AddCookie(&http.Cookie{Name: "session", Value: "sess-test"})
		rec := httptest.NewRecorder()
		srv.apiSiteLoginStateRouter(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestApiSiteLoginStateVisit_Cov(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{
		Name: "hdsky", DisplayName: "HDSky", Enabled: true,
	}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiSiteLoginStateVisit(rec, authedRequest(http.MethodGet, "/api/sites/visit", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("invalid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/sites/visit", bytes.NewBufferString(`{bad`))
		req.AddCookie(&http.Cookie{Name: "session", Value: "sess-test"})
		rec := httptest.NewRecorder()
		srv.apiSiteLoginStateVisit(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("empty site name", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiSiteLoginStateVisit(rec, authedRequest(http.MethodPost, "/api/sites/visit", SiteVisitReportRequest{LastVisitAt: "2024-01-01T00:00:00Z"}))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("bad timestamp", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiSiteLoginStateVisit(rec, authedRequest(http.MethodPost, "/api/sites/visit", SiteVisitReportRequest{SiteName: "hdsky", LastVisitAt: "not-a-time"}))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("valid visit", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.apiSiteLoginStateVisit(rec, authedRequest(http.MethodPost, "/api/sites/visit", SiteVisitReportRequest{SiteName: "hdsky", LastVisitAt: "2024-01-01T00:00:00Z"}))
		require.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestGetDownloader_Cov(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb1", Type: "qbittorrent", URL: "http://localhost:8080", Enabled: true,
	}).Error)

	t.Run("found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/1", nil)
		server.getDownloader(w, req, 1)
		require.Equal(t, http.StatusOK, w.Code)

		var resp DownloaderResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "qb1", resp.Name)
	})

	t.Run("not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/999", nil)
		server.getDownloader(w, req, 999)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestUpdateDownloader_Cov(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb1", Type: "qbittorrent", URL: "http://localhost:8080", Enabled: true, IsDefault: true,
	}).Error)
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb2", Type: "qbittorrent", URL: "http://localhost:8081", Enabled: true,
	}).Error)

	t.Run("invalid body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/1", bytes.NewBufferString(`{bad`))
		server.updateDownloader(w, req, 1)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("not found", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{Name: "x", Type: "qbittorrent", URL: "http://h", Enabled: true})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/999", bytes.NewReader(body))
		server.updateDownloader(w, req, 999)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("update qb2 fields", func(t *testing.T) {
		body, _ := json.Marshal(DownloaderRequest{
			Name: "qb2-renamed", Type: "qbittorrent", URL: "http://localhost:8082", Enabled: true, AutoStart: true,
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/2", bytes.NewReader(body))
		server.updateDownloader(w, req, 2)
		require.Equal(t, http.StatusOK, w.Code)

		var resp DownloaderResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "qb2-renamed", resp.Name)
	})
}

func TestDeleteDownloader_Cov(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb1", Type: "qbittorrent", URL: "http://localhost:8080", Enabled: true, IsDefault: true,
	}).Error)
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb2", Type: "qbittorrent", URL: "http://localhost:8081", Enabled: true,
	}).Error)

	t.Run("cannot delete default when others exist", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/downloaders/1", nil)
		server.deleteDownloader(w, req, 1)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("delete non-default", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/downloaders/2", nil)
		server.deleteDownloader(w, req, 2)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("delete non-existent", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/downloaders/999", nil)
		server.deleteDownloader(w, req, 999)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestSetDefaultDownloader_Cov(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb1", Type: "qbittorrent", URL: "http://localhost:8080", Enabled: true, IsDefault: true,
	}).Error)
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb2", Type: "qbittorrent", URL: "http://localhost:8081", Enabled: false,
	}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/2/set-default", nil)
		server.setDefaultDownloader(w, req, "2")
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("invalid id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/abc/set-default", nil)
		server.setDefaultDownloader(w, req, "abc")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/999/set-default", nil)
		server.setDefaultDownloader(w, req, "999")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("set qb2 default", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/2/set-default", nil)
		server.setDefaultDownloader(w, req, "2")
		require.Equal(t, http.StatusOK, w.Code)

		var resp DownloaderResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.True(t, resp.IsDefault)
		assert.True(t, resp.Enabled)
	})
}

func TestListAndCreateDownloader_Cov(t *testing.T) {
	server, _ := setupTestServer(t)

	t.Run("list empty", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders", nil)
		server.listDownloaders(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	tests := []struct {
		name       string
		req        DownloaderRequest
		wantStatus int
	}{
		{"missing name", DownloaderRequest{Type: "qbittorrent", URL: "http://h"}, http.StatusBadRequest},
		{"missing type", DownloaderRequest{Name: "x", URL: "http://h"}, http.StatusBadRequest},
		{"bad type", DownloaderRequest{Name: "x", Type: "bogus", URL: "http://h"}, http.StatusBadRequest},
		{"missing url", DownloaderRequest{Name: "x", Type: "qbittorrent"}, http.StatusBadRequest},
		{"bad url", DownloaderRequest{Name: "x", Type: "qbittorrent", URL: "ftp://h"}, http.StatusBadRequest},
		{"valid", DownloaderRequest{Name: "qbNew", Type: "qbittorrent", URL: "localhost:9000", Enabled: true}, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.req)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
			server.createDownloader(w, req)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

// ==== merged from api_downloader_router_cov3_test.go ====
func TestApiDownloaderRouter_DispatchCov3(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.DownloaderDirectory{}))
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "rdl", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true, IsDefault: true,
	}).Error)
	mgr := scheduler.NewManager()
	t.Cleanup(func() { mgr.StopAll() })
	server.mgr = mgr

	t.Run("directories list route", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/1/directories", nil)
		server.apiDownloaderRouter(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("directory detail route", func(t *testing.T) {
		require.NoError(t, db.Create(&models.DownloaderDirectory{
			DownloaderID: 1, Path: "/d/a", IsDefault: true,
		}).Error)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/1/directories/1", strings.NewReader(`{"path":"/d/b"}`))
		server.apiDownloaderRouter(w, req)
		assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusBadRequest)
	})

	t.Run("apply-to-sites route", func(t *testing.T) {
		require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)
		body, _ := json.Marshal(ApplyDownloaderToSitesRequest{SiteIDs: []uint{1}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/apply-to-sites", bytes.NewReader(body))
		server.apiDownloaderRouter(w, req)
		assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
	})

	t.Run("plain detail route", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/1", nil)
		server.apiDownloaderRouter(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestApiDownloaderDirectoryDetail_Routing(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.DownloaderDirectory{}))
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "rdl", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&models.DownloaderDirectory{
		DownloaderID: 1, Path: "/d/a", IsDefault: true,
	}).Error)

	t.Run("invalid path", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/1/other/1", nil)
		server.apiDownloaderDirectoryDetail(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid downloader id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/abc/directories/1", nil)
		server.apiDownloaderDirectoryDetail(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("set-default route", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/directories/1/set-default", nil)
		server.apiDownloaderDirectoryDetail(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("set-default invalid dir id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/directories/abc/set-default", nil)
		server.apiDownloaderDirectoryDetail(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid dir id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/1/directories/abc", nil)
		server.apiDownloaderDirectoryDetail(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("delete route", func(t *testing.T) {
		require.NoError(t, db.Create(&models.DownloaderDirectory{
			DownloaderID: 1, Path: "/d/b",
		}).Error)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/downloaders/1/directories/2", nil)
		server.apiDownloaderDirectoryDetail(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/api/downloaders/1/directories/1", nil)
		server.apiDownloaderDirectoryDetail(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApiUserInfoAggregated_NoService(t *testing.T) {
	InitUserInfoService(nil)
	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/userinfo/aggregated", nil)
	s.apiUserInfoAggregated(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

var _ = global.GlobalDB

// ==== merged from api_downloader_test.go ====
// setupTestServer 创建测试服务器
func setupTestServer(t *testing.T) (*Server, *gorm.DB) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}

	// 迁移表
	db.AutoMigrate(
		&models.DownloaderSetting{},
		&models.SiteTemplate{},
		&models.AdminUser{},
		&models.SettingsGlobal{},
		&models.QbitSettings{},
		&models.SiteSetting{},
	)

	// 设置全局DB
	global.GlobalDB = &models.TorrentDB{DB: db}

	// 初始化logger（如果未初始化）
	if global.GlobalLogger == nil {
		zapLogger, _ := zap.NewDevelopment()
		global.GlobalLogger = zapLogger
	}

	store := core.NewConfigStore(global.GlobalDB)
	server := NewServer(store, nil)

	return server, db
}

// TestDownloaderCRUD 测试下载器CRUD操作
func TestDownloaderCRUD(t *testing.T) {
	server, db := setupTestServer(t)

	// 测试创建下载器
	t.Run("Create Downloader", func(t *testing.T) {
		reqBody := DownloaderRequest{
			Name:      "test-qbit",
			Type:      "qbittorrent",
			URL:       "http://localhost:8080",
			Username:  "admin",
			Password:  "password",
			IsDefault: true,
			Enabled:   true,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.createDownloader(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp DownloaderResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Name != "test-qbit" {
			t.Errorf("expected name 'test-qbit', got '%s'", resp.Name)
		}
		if resp.Type != "qbittorrent" {
			t.Errorf("expected type 'qbittorrent', got '%s'", resp.Type)
		}
	})

	// 测试列出下载器
	t.Run("List Downloaders", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders", nil)
		w := httptest.NewRecorder()

		server.listDownloaders(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var resp []DownloaderResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if len(resp) != 1 {
			t.Errorf("expected 1 downloader, got %d", len(resp))
		}
	})

	// 测试获取下载器详情
	t.Run("Get Downloader", func(t *testing.T) {
		var dl models.DownloaderSetting
		db.First(&dl)

		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/1", nil)
		w := httptest.NewRecorder()

		server.getDownloader(w, req, dl.ID)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var resp DownloaderResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Name != "test-qbit" {
			t.Errorf("expected name 'test-qbit', got '%s'", resp.Name)
		}
	})

	// 测试更新下载器
	t.Run("Update Downloader", func(t *testing.T) {
		var dl models.DownloaderSetting
		db.First(&dl)

		reqBody := DownloaderRequest{
			Name:      "updated-qbit",
			Type:      "qbittorrent",
			URL:       "http://localhost:9090",
			IsDefault: true, // 保持默认状态，因为是唯一的下载器
			Enabled:   true, // 默认下载器不能禁用
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/1", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.updateDownloader(w, req, dl.ID)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp DownloaderResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Name != "updated-qbit" {
			t.Errorf("expected name 'updated-qbit', got '%s'", resp.Name)
		}
		if resp.URL != "http://localhost:9090" {
			t.Errorf("expected URL 'http://localhost:9090', got '%s'", resp.URL)
		}
	})

	// 测试删除下载器
	t.Run("Delete Downloader", func(t *testing.T) {
		var dl models.DownloaderSetting
		db.First(&dl)

		req := httptest.NewRequest(http.MethodDelete, "/api/downloaders/1", nil)
		w := httptest.NewRecorder()

		server.deleteDownloader(w, req, dl.ID)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		// 验证已删除
		var count int64
		db.Model(&models.DownloaderSetting{}).Count(&count)
		if count != 0 {
			t.Errorf("expected 0 downloaders, got %d", count)
		}
	})
}

// TestDownloaderValidation 测试下载器验证
func TestDownloaderValidation(t *testing.T) {
	server, _ := setupTestServer(t)

	tests := []struct {
		name       string
		request    DownloaderRequest
		expectCode int
	}{
		{
			name:       "Empty Name",
			request:    DownloaderRequest{Type: "qbittorrent", URL: "http://localhost"},
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "Empty Type",
			request:    DownloaderRequest{Name: "test", URL: "http://localhost"},
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "Invalid Type",
			request:    DownloaderRequest{Name: "test", Type: "invalid", URL: "http://localhost"},
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "Empty URL",
			request:    DownloaderRequest{Name: "test", Type: "qbittorrent"},
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "Valid Request",
			request:    DownloaderRequest{Name: "test", Type: "qbittorrent", URL: "http://localhost"},
			expectCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.createDownloader(w, req)

			if w.Code != tt.expectCode {
				t.Errorf("expected status %d, got %d: %s", tt.expectCode, w.Code, w.Body.String())
			}
		})
	}
}

// TestDownloaderDefaultHandling 测试默认下载器处理
func TestDownloaderDefaultHandling(t *testing.T) {
	server, db := setupTestServer(t)

	// 创建第一个默认下载器
	dl1 := DownloaderRequest{
		Name:      "dl-1",
		Type:      "qbittorrent",
		URL:       "http://localhost:8080",
		IsDefault: true,
		Enabled:   true,
	}
	body1, _ := json.Marshal(dl1)
	req1 := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	server.createDownloader(w1, req1)

	// 创建第二个默认下载器
	dl2 := DownloaderRequest{
		Name:      "dl-2",
		Type:      "transmission",
		URL:       "http://localhost:9091",
		IsDefault: true,
		Enabled:   true,
	}
	body2, _ := json.Marshal(dl2)
	req2 := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	server.createDownloader(w2, req2)

	// 验证只有一个默认下载器
	var defaultCount int64
	db.Model(&models.DownloaderSetting{}).Where("is_default = ?", true).Count(&defaultCount)
	if defaultCount != 1 {
		t.Errorf("expected 1 default downloader, got %d", defaultCount)
	}

	// 验证第二个是默认的
	var dl models.DownloaderSetting
	db.Where("is_default = ?", true).First(&dl)
	if dl.Name != "dl-2" {
		t.Errorf("expected dl-2 to be default, got %s", dl.Name)
	}
}

// TestSetDefaultDownloader 测试设置默认下载器
func TestSetDefaultDownloader(t *testing.T) {
	server, db := setupTestServer(t)

	// 创建两个下载器
	dl1 := DownloaderRequest{
		Name:      "dl-1",
		Type:      "qbittorrent",
		URL:       "http://localhost:8080",
		IsDefault: true,
		Enabled:   true,
	}
	body1, _ := json.Marshal(dl1)
	req1 := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	server.createDownloader(w1, req1)

	dl2 := DownloaderRequest{
		Name:      "dl-2",
		Type:      "transmission",
		URL:       "http://localhost:9091",
		IsDefault: false,
		Enabled:   true,
	}
	body2, _ := json.Marshal(dl2)
	req2 := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	server.createDownloader(w2, req2)

	// 获取第二个下载器的ID
	var dlRecord models.DownloaderSetting
	db.Where("name = ?", "dl-2").First(&dlRecord)

	// 测试设置第二个为默认
	t.Run("Set Default", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/2/set-default", nil)
		w := httptest.NewRecorder()

		server.setDefaultDownloader(w, req, "2")

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		// 验证只有一个默认下载器
		var defaultCount int64
		db.Model(&models.DownloaderSetting{}).Where("is_default = ?", true).Count(&defaultCount)
		if defaultCount != 1 {
			t.Errorf("expected 1 default downloader, got %d", defaultCount)
		}

		// 验证dl-2是默认的
		var dl models.DownloaderSetting
		db.Where("is_default = ?", true).First(&dl)
		if dl.Name != "dl-2" {
			t.Errorf("expected dl-2 to be default, got %s", dl.Name)
		}
	})
}

// TestCannotRemoveOnlyDefault 测试不能移除唯一默认下载器的默认状态
func TestCannotRemoveOnlyDefault(t *testing.T) {
	server, db := setupTestServer(t)

	// 创建一个默认下载器
	dl := DownloaderRequest{
		Name:      "only-dl",
		Type:      "qbittorrent",
		URL:       "http://localhost:8080",
		IsDefault: true,
		Enabled:   true,
	}
	body, _ := json.Marshal(dl)
	req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.createDownloader(w, req)

	// 获取下载器ID
	var dlRecord models.DownloaderSetting
	db.First(&dlRecord)

	// 尝试取消默认状态
	updateReq := DownloaderRequest{
		Name:      "only-dl",
		Type:      "qbittorrent",
		URL:       "http://localhost:8080",
		IsDefault: false, // 尝试取消默认
		Enabled:   true,
	}
	updateBody, _ := json.Marshal(updateReq)
	req2 := httptest.NewRequest(http.MethodPut, "/api/downloaders/1", bytes.NewReader(updateBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()

	server.updateDownloader(w2, req2, dlRecord.ID)

	// 应该返回错误
	if w2.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w2.Code)
	}
}

// TestCannotDeleteDefaultWithOthers 测试有其他下载器时不能删除默认下载器
func TestCannotDeleteDefaultWithOthers(t *testing.T) {
	server, db := setupTestServer(t)

	// 创建两个下载器
	dl1 := DownloaderRequest{
		Name:      "default-dl",
		Type:      "qbittorrent",
		URL:       "http://localhost:8080",
		IsDefault: true,
		Enabled:   true,
	}
	body1, _ := json.Marshal(dl1)
	req1 := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	server.createDownloader(w1, req1)

	dl2 := DownloaderRequest{
		Name:      "other-dl",
		Type:      "transmission",
		URL:       "http://localhost:9091",
		IsDefault: false,
		Enabled:   true,
	}
	body2, _ := json.Marshal(dl2)
	req2 := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	server.createDownloader(w2, req2)

	// 获取默认下载器ID
	var defaultDl models.DownloaderSetting
	db.Where("is_default = ?", true).First(&defaultDl)

	// 尝试删除默认下载器
	req3 := httptest.NewRequest(http.MethodDelete, "/api/downloaders/1", nil)
	w3 := httptest.NewRecorder()

	server.deleteDownloader(w3, req3, defaultDl.ID)

	// 应该返回错误
	if w3.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", w3.Code, w3.Body.String())
	}
}

// TestDownloaderAutoStart 测试下载器 auto_start 字段
func TestDownloaderAutoStart(t *testing.T) {
	server, db := setupTestServer(t)

	// 测试创建带 auto_start=true 的下载器
	t.Run("Create with AutoStart true", func(t *testing.T) {
		reqBody := DownloaderRequest{
			Name:      "auto-start-dl",
			Type:      "qbittorrent",
			URL:       "http://localhost:8080",
			Username:  "admin",
			Password:  "password",
			IsDefault: true,
			Enabled:   true,
			AutoStart: true,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.createDownloader(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp DownloaderResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if !resp.AutoStart {
			t.Error("expected auto_start to be true")
		}

		// 验证数据库中的值
		var dl models.DownloaderSetting
		db.First(&dl)
		if !dl.AutoStart {
			t.Error("expected auto_start in DB to be true")
		}
	})

	// 测试创建带 auto_start=false 的下载器
	t.Run("Create with AutoStart false", func(t *testing.T) {
		reqBody := DownloaderRequest{
			Name:      "no-auto-start-dl",
			Type:      "transmission",
			URL:       "http://localhost:9091",
			IsDefault: false,
			Enabled:   true,
			AutoStart: false,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.createDownloader(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp DownloaderResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.AutoStart {
			t.Error("expected auto_start to be false")
		}
	})

	// 测试更新 auto_start 字段
	t.Run("Update AutoStart", func(t *testing.T) {
		var dl models.DownloaderSetting
		db.Where("name = ?", "auto-start-dl").First(&dl)

		reqBody := DownloaderRequest{
			Name:      "auto-start-dl",
			Type:      "qbittorrent",
			URL:       "http://localhost:8080",
			IsDefault: true,
			Enabled:   true,
			AutoStart: false, // 从 true 改为 false
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/1", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.updateDownloader(w, req, dl.ID)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp DownloaderResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.AutoStart {
			t.Error("expected auto_start to be false after update")
		}

		// 验证数据库中的值
		db.First(&dl, dl.ID)
		if dl.AutoStart {
			t.Error("expected auto_start in DB to be false after update")
		}
	})

	// 测试列表返回 auto_start 字段
	t.Run("List includes AutoStart", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders", nil)
		w := httptest.NewRecorder()

		server.listDownloaders(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var resp []DownloaderResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		// 找到 auto-start-dl 并验证 auto_start 字段
		found := false
		for _, dl := range resp {
			if dl.Name == "auto-start-dl" {
				found = true
				// 之前更新为 false
				if dl.AutoStart {
					t.Error("expected auto_start to be false in list")
				}
			}
		}
		if !found {
			t.Error("auto-start-dl not found in list")
		}
	})

	// 测试获取详情返回 auto_start 字段
	t.Run("Get includes AutoStart", func(t *testing.T) {
		var dl models.DownloaderSetting
		db.Where("name = ?", "auto-start-dl").First(&dl)

		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/1", nil)
		w := httptest.NewRecorder()

		server.getDownloader(w, req, dl.ID)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var resp DownloaderResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		// 之前更新为 false
		if resp.AutoStart {
			t.Error("expected auto_start to be false in get response")
		}
	})
}

func TestSetDefaultDownloaderAutoEnable(t *testing.T) {
	server, db := setupTestServer(t)

	db.Create(&models.DownloaderSetting{
		Name:      "disabled-dl",
		Type:      "qbittorrent",
		URL:       "http://localhost:8080",
		IsDefault: false,
		Enabled:   false,
	})

	var dl models.DownloaderSetting
	db.First(&dl)

	req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/set-default", nil)
	w := httptest.NewRecorder()

	server.setDefaultDownloader(w, req, "1")

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	db.First(&dl, dl.ID)
	if !dl.IsDefault {
		t.Error("expected is_default to be true")
	}
	if !dl.Enabled {
		t.Error("expected enabled to be true after setting as default")
	}

	var resp DownloaderResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.Enabled {
		t.Error("expected enabled in response to be true")
	}
}

// TestDownloaderAutoStartDefault 测试 auto_start 默认值
func TestDownloaderAutoStartDefault(t *testing.T) {
	server, db := setupTestServer(t)

	// 创建不指定 auto_start 的下载器（应该默认为 false）
	reqBody := DownloaderRequest{
		Name:      "default-auto-start-dl",
		Type:      "qbittorrent",
		URL:       "http://localhost:8080",
		IsDefault: true,
		Enabled:   true,
		// 不指定 AutoStart
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.createDownloader(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp DownloaderResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.AutoStart {
		t.Error("expected auto_start to default to false")
	}

	// 验证数据库中的值
	var dl models.DownloaderSetting
	db.First(&dl)
	if dl.AutoStart {
		t.Error("expected auto_start in DB to default to false")
	}
}
