package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func TestProcessTorrentPush_DownloadPath(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}, &models.DownloaderSetting{}, &models.SiteSetting{}))
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: []byte("torrentbytes")})

	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb1", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true,
	}).Error)

	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/push", nil)

	t.Run("download error surfaces", func(t *testing.T) {
		withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", err: assertErr("boom")})
		resp := server.processTorrentPush(req, TorrentPushRequest{
			DownloadURL:   "/api/site/hdsky/torrent/1/download",
			DownloaderIDs: []uint{1},
		})
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Message, "Failed to download torrent")
	})

	t.Run("push to unreachable downloader reports failure", func(t *testing.T) {
		withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: []byte("torrentbytes")})
		resp := server.processTorrentPush(req, TorrentPushRequest{
			DownloadURL:   "/api/site/hdsky/torrent/1/download",
			DownloaderIDs: []uint{1},
			TorrentTitle:  "T1",
		})
		// downloader instance creation/connection fails -> not successful,
		// but the result set has one entry for the target downloader.
		require.Len(t, resp.Results, 1)
		assert.Equal(t, uint(1), resp.Results[0].DownloaderID)
		assert.False(t, resp.Success)
	})
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
