package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func TestProcessTorrentPush_SiteNotFound(t *testing.T) {
	server, _ := setupTestServer(t)
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: []byte("x")})

	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/push", nil)
	resp := server.processTorrentPush(req, TorrentPushRequest{
		DownloadURL:   "/api/site/othersite/torrent/1/download",
		DownloaderIDs: []uint{1},
	})
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Message, "Site not found")
}

func TestProcessTorrentPush_DownloadWithHash(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}, &models.DownloaderSetting{}, &models.SiteSetting{}))
	withOrchestrator(t, &fakeV2Site{id: "hddolby", name: "HDDolby", hashData: minimalTorrentBytes(1024)})
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb1", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true,
	}).Error)

	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/push", nil)
	resp := server.processTorrentPush(req, TorrentPushRequest{
		DownloadURL:   "/api/site/hddolby/torrent/42/download?downhash=abcdef",
		DownloaderIDs: []uint{1},
		TorrentTitle:  "HashT",
	})
	// hash download succeeded; downloader connection fails -> result recorded, not successful
	require.Len(t, resp.Results, 1)
	assert.Equal(t, uint(1), resp.Results[0].DownloaderID)
}

func TestProcessTorrentPush_HashDownloadError(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}, &models.DownloaderSetting{}))
	withOrchestrator(t, &fakeV2Site{id: "hddolby", name: "HDDolby", hashErr: assertErr("hashboom")})

	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/push", nil)
	resp := server.processTorrentPush(req, TorrentPushRequest{
		DownloadURL:   "/api/site/hddolby/torrent/42/download?downhash=abc",
		DownloaderIDs: []uint{1},
	})
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Message, "Failed to download torrent")
}

func TestProcessTorrentPush_OverrideSiteAndTorrentID(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}, &models.DownloaderSetting{}))
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: minimalTorrentBytes(512)})
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb1", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true,
	}).Error)

	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/push", nil)
	resp := server.processTorrentPush(req, TorrentPushRequest{
		DownloadURL:   "/api/site/ignored/torrent/0/download",
		TorrentID:     "77",
		SourceSite:    "hdsky",
		DownloaderIDs: []uint{1},
		TorrentTitle:  "OverrideT",
	})
	require.Len(t, resp.Results, 1)
}

func TestApiTorrentPush_FullDispatch(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}, &models.DownloaderSetting{}))
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: minimalTorrentBytes(256)})
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb1", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true,
	}).Error)

	body := `{"downloadUrl":"/api/site/hdsky/torrent/1/download","downloaderIds":[1],"torrentTitle":"T"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/push", strings.NewReader(body))
	server.apiTorrentPush(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestApiTorrentBatchPush_FullDispatch(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}, &models.DownloaderSetting{}))
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: minimalTorrentBytes(256)})
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb1", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true,
	}).Error)

	body := `{"torrents":[{"downloadUrl":"/api/site/hdsky/torrent/1/download","torrentTitle":"t1"}],"downloaderIds":[1]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-push", strings.NewReader(body))
	server.apiTorrentBatchPush(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
