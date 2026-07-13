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

// qbitMockServer returns an httptest server emulating the minimal qBittorrent
// API surface used by the push pipeline: login, add, properties (not-found so
// the torrent is treated as new), maindata (free space), and info.
func qbitMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			_, _ = w.Write([]byte("v4.6.0"))
		case "/api/v2/torrents/properties":
			w.WriteHeader(http.StatusNotFound)
		case "/api/v2/sync/maindata":
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":1099511627776}}`))
		case "/api/v2/torrents/add":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
}

func TestProcessTorrentPush_FullSuccessViaQbitMock(t *testing.T) {
	writeWebTestSecretKey(t)
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}, &models.DownloaderSetting{}, &models.SiteSetting{}, &models.SettingsGlobal{}))
	require.NoError(t, db.Create(&models.SettingsGlobal{DownloadDir: "downloads", CleanupDiskProtect: false}).Error)

	mock := qbitMockServer(t)
	defer mock.Close()

	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qbmock", Type: "qbittorrent", URL: mock.URL, Enabled: true, AutoStart: false,
	}).Error)

	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: minimalTorrentBytes(4096)})

	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/push", nil)
	resp := server.processTorrentPush(req, TorrentPushRequest{
		DownloadURL:   "/api/site/hdsky/torrent/1/download",
		DownloaderIDs: []uint{1},
		TorrentTitle:  "PushOK",
	})
	require.Len(t, resp.Results, 1)
	assert.True(t, resp.Results[0].Success)
	assert.True(t, resp.Success)
	_ = global.GlobalDB
}

func TestProcessBatchTorrentPush_MixedViaQbitMock(t *testing.T) {
	writeWebTestSecretKey(t)
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}, &models.DownloaderSetting{}, &models.SiteSetting{}, &models.SettingsGlobal{}))
	require.NoError(t, db.Create(&models.SettingsGlobal{DownloadDir: "downloads", CleanupDiskProtect: false}).Error)

	mock := qbitMockServer(t)
	defer mock.Close()
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qbmock", Type: "qbittorrent", URL: mock.URL, Enabled: true,
	}).Error)

	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: minimalTorrentBytes(4096)})

	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-push", nil)
	resp := server.processBatchTorrentPush(req, BatchTorrentPushRequest{
		Torrents: []TorrentPushItem{
			{DownloadURL: "/api/site/hdsky/torrent/1/download", TorrentTitle: "t1"},
		},
		DownloaderIDs: []uint{1},
	})
	assert.Equal(t, 1, resp.TotalCount)
	assert.GreaterOrEqual(t, resp.SuccessCount+resp.SkippedCount+resp.FailedCount, 1)
}
