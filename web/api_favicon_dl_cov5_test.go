package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestApiFaviconRefresh_InitsServiceAndURLFallback(t *testing.T) {
	setupFaviconServer(t)
	prev := faviconService
	faviconService = nil
	t.Cleanup(func() {
		if faviconService != nil {
			close(faviconService.stopCh)
		}
		faviconService = prev
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47, 12})
	}))
	defer ts.Close()

	if def := v2.GetDefinitionRegistry().GetOrDefault("covrefreshurls"); def != nil {
		def.URLs = []string{ts.URL}
	} else {
		v2.RegisterSiteDefinition(&v2.SiteDefinition{
			ID: "covrefreshurls", Name: "CovRefreshURLs", URLs: []string{ts.URL},
		})
	}

	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/favicon/covrefreshurls/refresh", nil)
	s.apiFaviconRefresh(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, faviconService)
}

func TestApiFaviconRefresh_NoURLConfigured(t *testing.T) {
	server := setupFaviconServer(t)

	if v2.GetDefinitionRegistry().GetOrDefault("covrefreshnourl") == nil {
		v2.RegisterSiteDefinition(&v2.SiteDefinition{ID: "covrefreshnourl", Name: "CovRefreshNoURL"})
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/favicon/covrefreshnourl/refresh", nil)
	server.apiFaviconRefresh(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestApiDeleteTasks_SkipsPushedInTx(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}))

	require.NoError(t, db.Create(&models.TorrentInfo{SiteName: "s", TorrentID: "u1"}).Error)
	require.NoError(t, db.Create(&models.TorrentInfo{SiteName: "s", TorrentID: "u2"}).Error)

	body, _ := json.Marshal(DeleteTasksRequest{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tasks/batch-delete", bytes.NewReader(body))
	server.apiDeleteTasks(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp DeleteTasksResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.Success)
}

func TestApiDownloaderTorrents_SortAndCategoryTag(t *testing.T) {
	fake := &fakeDownloader{torrents: sampleTorrents()}
	server, _ := setupServerWithFakeDownloader(t, fake)

	cases := []string{
		"/api/downloader-torrents?sort_by=title&sort_order=asc",
		"/api/downloader-torrents?sort_by=size&sort_order=desc",
		"/api/downloader-torrents?sort_by=progress",
		"/api/downloader-torrents?sort_by=ratio",
		"/api/downloader-torrents?category=movie",
		"/api/downloader-torrents?tag=hd",
		"/api/downloader-torrents?state=downloading",
		"/api/downloader-torrents?search=alpha",
		"/api/downloader-torrents?page=2&page_size=1",
	}
	for _, url := range cases {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, url, nil)
		server.apiDownloaderTorrents(w, req)
		require.Equal(t, http.StatusOK, w.Code, url)
	}
}

func TestApiDownloaderCapabilities_Cov(t *testing.T) {
	server, _ := setupServerWithFakeDownloader(t, &fakeDownloader{})
	require.NoError(t, global.GlobalDB.DB.Create(&models.DownloaderSetting{
		Name: "cap-dl", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true,
	}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/capabilities", nil)
		server.apiDownloaderCapabilities(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("get", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/capabilities", nil)
		server.apiDownloaderCapabilities(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})
}
