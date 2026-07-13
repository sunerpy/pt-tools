package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func TestApiDownloaderTorrentDetail_GetError(t *testing.T) {
	fake := &fakeDownloader{getErr: errors.New("boom")}
	server, _ := setupServerWithFakeDownloader(t, fake)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/detail?downloader_id=1&task_id=t1", nil)
	server.apiDownloaderTorrentDetail(w, req)
	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestApiAddDownloaderTorrent_AddFailure(t *testing.T) {
	fake := &fakeDownloader{
		addResult: downloader.AddTorrentResult{Success: false, Message: "rejected"},
		addErr:    errors.New("add failed"),
		freSpace:  1 << 40,
	}
	server, _ := setupServerWithFakeDownloader(t, fake)
	require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
		Where("1 = 1").Update("cleanup_disk_protect", false).Error)

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

func TestApiAddDownloaderTorrent_ResultNotSuccess(t *testing.T) {
	fake := &fakeDownloader{
		addResult: downloader.AddTorrentResult{Success: false, Message: "dup"},
		freSpace:  1 << 40,
	}
	server, _ := setupServerWithFakeDownloader(t, fake)
	require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
		Where("1 = 1").Update("cleanup_disk_protect", false).Error)

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

func TestApiDownloaderTorrentActions_ListFallbackSingle(t *testing.T) {
	fake := &fakeDownloader{
		torrents:  sampleTorrents(),
		pauseErr:  errors.New("batch pause failed"),
		resumeErr: nil,
	}
	server, _ := setupServerWithFakeDownloader(t, fake)

	body, _ := json.Marshal(BatchTorrentActionRequest{
		Action:  "pause",
		Targets: []TorrentActionTarget{{DownloaderID: 1, TaskID: "t1"}},
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
	server.apiDownloaderTorrentActions(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp BatchTorrentActionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.FailedCount)
}
