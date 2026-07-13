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
)

func TestApiDownloaderTorrents_ErrorBranches(t *testing.T) {
	t.Run("method not allowed", func(t *testing.T) {
		server, _ := setupTestServer(t)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents", nil)
		server.apiDownloaderTorrents(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("invalid downloader_id", func(t *testing.T) {
		server, _ := setupServerWithFakeDownloader(t, &fakeDownloader{})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents?downloader_id=abc", nil)
		server.apiDownloaderTorrents(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("no manager", func(t *testing.T) {
		server, _ := setupTestServer(t)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents", nil)
		server.apiDownloaderTorrents(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("list error surfaces empty then ok", func(t *testing.T) {
		fake := &fakeDownloader{listErr: assertErr("listfail")}
		server, _ := setupServerWithFakeDownloader(t, fake)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents", nil)
		server.apiDownloaderTorrents(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp DownloaderTorrentsResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 0, resp.Total)
	})

	t.Run("filter by downloader_id with data", func(t *testing.T) {
		fake := &fakeDownloader{torrents: sampleTorrents()}
		server, id := setupServerWithFakeDownloader(t, fake)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents?downloader_id=1&page_size=0", nil)
		require.Equal(t, uint(1), id)
		server.apiDownloaderTorrents(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestApiAddDownloaderTorrent_DiskProtectRejectsMagnet(t *testing.T) {
	fake := &fakeDownloader{freSpace: 1 << 40}
	server, _ := setupServerWithFakeDownloader(t, fake)
	require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
		Where("1 = 1").Updates(map[string]any{
		"cleanup_disk_protect":      true,
		"cleanup_min_disk_space_gb": 10.0,
	}).Error)

	body, _ := json.Marshal(AddDownloaderTorrentRequest{
		DownloaderIDs: []uint{1},
		MagnetLink:    "magnet:?xt=urn:btih:abc",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/add", bytes.NewReader(body))
	server.apiAddDownloaderTorrent(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp AddDownloaderTorrentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.FailedCount)
}

func TestApiAddDownloaderTorrent_UnknownDownloaderInList(t *testing.T) {
	fake := &fakeDownloader{
		freSpace: 1 << 40,
	}
	server, _ := setupServerWithFakeDownloader(t, fake)
	require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
		Where("1 = 1").Update("cleanup_disk_protect", false).Error)

	body, _ := json.Marshal(AddDownloaderTorrentRequest{
		DownloaderIDs: []uint{1, 999},
		MagnetLink:    "magnet:?xt=urn:btih:abc",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/add", bytes.NewReader(body))
	server.apiAddDownloaderTorrent(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}
