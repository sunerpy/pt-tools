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
)

func TestProcessTorrentPush_ValidTorrentData(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}, &models.DownloaderSetting{}, &models.SiteSetting{}, &models.SettingsGlobal{}))
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: minimalTorrentBytes(1024)})

	require.NoError(t, db.Create(&models.DownloaderSetting{Name: "qb1", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true, AutoStart: true}).Error)
	require.NoError(t, db.Model(&models.SettingsGlobal{}).Where("1=1").Update("cleanup_disk_protect", false).Error)

	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/push", nil)
	resp := server.processTorrentPush(req, TorrentPushRequest{
		DownloadURL:   "/api/site/hdsky/torrent/1/download",
		DownloaderIDs: []uint{1},
		TorrentTitle:  "T1",
	})
	require.Len(t, resp.Results, 1)
	assert.Equal(t, uint(1), resp.Results[0].DownloaderID)
}

func TestApiFavicon_FetchThroughDefinition(t *testing.T) {
	server := setupFaviconServer(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47, 1, 2, 3})
	}))
	defer ts.Close()

	require.NoError(t, faviconService.fetchAndSave("hdsky", "HDSky", ts.URL))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicon/hdsky", nil)
	server.apiFavicon(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Body.Bytes())
}

func TestApiDeleteTasks_Errors(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}))

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/tasks/batch-delete", nil)
		server.apiDeleteTasks(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("invalid body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/tasks/batch-delete", bytes.NewBufferString(`{bad`))
		server.apiDeleteTasks(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("no rows returns zero", func(t *testing.T) {
		body, _ := json.Marshal(DeleteTasksRequest{IDs: []uint{999}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/tasks/batch-delete", bytes.NewReader(body))
		server.apiDeleteTasks(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp DeleteTasksResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 0, resp.Success)
	})
}

func TestApiDownloaderRouter_ApplyToSites(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}, &models.DownloaderSetting{}, &models.RSSSubscription{}))
	require.NoError(t, db.Create(&models.DownloaderSetting{Name: "qb1", Type: "qbittorrent", Enabled: true}).Error)

	body, _ := json.Marshal(ApplyDownloaderToSitesRequest{SiteIDs: nil})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/apply-to-sites", bytes.NewReader(body))
	server.apiDownloaderRouter(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestApiDownloaderTorrentActions_SetLocation(t *testing.T) {
	fake := &fakeDownloader{torrents: sampleTorrents()}
	server, _ := setupServerWithFakeDownloader(t, fake)

	t.Run("set_location missing path fails", func(t *testing.T) {
		body, _ := json.Marshal(BatchTorrentActionRequest{
			Action:  "set_location",
			Targets: []TorrentActionTarget{{DownloaderID: 1, TaskID: "t1"}},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
		server.apiDownloaderTorrentActions(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp BatchTorrentActionResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp.FailedCount)
	})

	t.Run("set_location with path succeeds", func(t *testing.T) {
		body, _ := json.Marshal(BatchTorrentActionRequest{
			Action:   "set_location",
			SavePath: "/downloads",
			Targets:  []TorrentActionTarget{{DownloaderID: 1, TaskID: "t1"}},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
		server.apiDownloaderTorrentActions(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp BatchTorrentActionResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp.SuccessCount)
	})

	t.Run("unknown action", func(t *testing.T) {
		body, _ := json.Marshal(BatchTorrentActionRequest{
			Action:  "explode",
			Targets: []TorrentActionTarget{{DownloaderID: 1, TaskID: "t1"}},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
		server.apiDownloaderTorrentActions(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}
