package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestApiSiteTemplateExport_Success(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.SiteTemplate{}))
	require.NoError(t, db.Create(&models.SiteTemplate{
		Name: "exptpl", DisplayName: "ExpTpl", BaseURL: "https://e.example.com", AuthMethod: "cookie",
	}).Error)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sites/templates/1/export", nil)
	server.apiSiteTemplateExport(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "exptpl")
}

func TestApiArchiveTorrents_Paging(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfoArchive{}))
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&models.TorrentInfoArchive{
			SiteName: "hdsky", Title: "A", IsCompleted: true,
		}).Error)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/torrents/archive?page=1&page_size=2", nil)
	server.apiArchiveTorrents(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestApiPausedTorrents_Paging(t *testing.T) {
	server, _ := setupServerWithFakeDownloader(t, &fakeDownloader{})
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.TorrentInfo{}))
	require.NoError(t, global.GlobalDB.DB.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "1", Title: "P", IsPausedBySystem: true,
	}).Error)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/torrents/paused?page=1&page_size=1&site=hdsky", nil)
	server.apiPausedTorrents(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}
