package web

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func TestApiDownloaderTorrentActions_MoreBranches(t *testing.T) {
	fake := &fakeDownloader{torrents: sampleTorrents()}
	server, _ := setupServerWithFakeDownloader(t, fake)

	t.Run("bad body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewBufferString(`{bad`))
		server.apiDownloaderTorrentActions(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("empty targets", func(t *testing.T) {
		body, _ := json.Marshal(BatchTorrentActionRequest{Action: "pause"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
		server.apiDownloaderTorrentActions(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("empty action", func(t *testing.T) {
		body, _ := json.Marshal(BatchTorrentActionRequest{Targets: []TorrentActionTarget{{DownloaderID: 1, TaskID: "t1"}}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
		server.apiDownloaderTorrentActions(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("unsupported action", func(t *testing.T) {
		body, _ := json.Marshal(BatchTorrentActionRequest{
			Action: "bogus", Targets: []TorrentActionTarget{{DownloaderID: 1, TaskID: "t1"}},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
		server.apiDownloaderTorrentActions(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("set_location without path fails per target", func(t *testing.T) {
		body, _ := json.Marshal(BatchTorrentActionRequest{
			Action: "set_location", Targets: []TorrentActionTarget{{DownloaderID: 1, TaskID: "t1"}},
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
			Action: "set_location", SavePath: "/downloads",
			Targets: []TorrentActionTarget{{DownloaderID: 1, TaskID: "t1"}},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
		server.apiDownloaderTorrentActions(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp BatchTorrentActionResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp.SuccessCount)
	})
}

func TestApiDownloaderTorrentActions_BatchError(t *testing.T) {
	fake := &fakeDownloader{torrents: sampleTorrents(), pauseErr: assertErr("pausefail")}
	server, _ := setupServerWithFakeDownloader(t, fake)

	body, _ := json.Marshal(BatchTorrentActionRequest{
		Action: "pause", Targets: []TorrentActionTarget{{DownloaderID: 1, TaskID: "t1"}},
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
	server.apiDownloaderTorrentActions(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp BatchTorrentActionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.FailedCount)
}

func TestApiDownloaderTorrentDetail_ErrorBranches(t *testing.T) {
	server, _ := setupServerWithFakeDownloader(t, &fakeDownloader{getErr: assertErr("nope")})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/detail", nil)
		server.apiDownloaderTorrentDetail(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("missing params", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/detail", nil)
		server.apiDownloaderTorrentDetail(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid downloader id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/detail?downloader_id=abc&task_id=t1", nil)
		server.apiDownloaderTorrentDetail(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("downloader not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/detail?downloader_id=999&task_id=t1", nil)
		server.apiDownloaderTorrentDetail(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("get torrent error", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/detail?downloader_id=1&task_id=t1", nil)
		server.apiDownloaderTorrentDetail(w, req)
		assert.Equal(t, http.StatusBadGateway, w.Code)
	})
}

func TestApiAddDownloaderTorrent_TorrentFileSuccess(t *testing.T) {
	fake := &fakeDownloader{
		addResult: downloader.AddTorrentResult{Success: true, ID: "new1", Hash: "h1", Message: "ok"},
		freSpace:  1 << 40,
	}
	server, _ := setupServerWithFakeDownloader(t, fake)
	require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
		Where("1 = 1").Update("cleanup_disk_protect", false).Error)

	req := AddDownloaderTorrentRequest{
		DownloaderIDs: []uint{1},
		TorrentBase64: base64.StdEncoding.EncodeToString(minimalTorrentBytes(1024)),
		AddPaused:     true,
		SavePath:      "/dl",
		Category:      "movie",
		Tags:          "hd",
	}
	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/add", bytes.NewReader(body))
	server.apiAddDownloaderTorrent(w, r)
	require.Equal(t, http.StatusOK, w.Code)
	var resp AddDownloaderTorrentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.SuccessCount)
}

func TestApiAddDownloaderTorrent_AddErrorReleasesBudget(t *testing.T) {
	fake := &fakeDownloader{addErr: assertErr("addfail"), freSpace: 1 << 40}
	server, _ := setupServerWithFakeDownloader(t, fake)
	require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
		Where("1 = 1").Update("cleanup_disk_protect", false).Error)

	body, _ := json.Marshal(AddDownloaderTorrentRequest{
		DownloaderIDs: []uint{1},
		MagnetLink:    "magnet:?xt=urn:btih:abc",
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/add", bytes.NewReader(body))
	server.apiAddDownloaderTorrent(w, r)
	require.Equal(t, http.StatusOK, w.Code)
	var resp AddDownloaderTorrentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.FailedCount)
}
