package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApiDownloaderTorrentActions_BatchFailsPerTargetFallback(t *testing.T) {
	fake := &fakeDownloader{
		torrents:      sampleTorrents(),
		batchPauseErr: assertErr("batchfail"),
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
	assert.Equal(t, 1, resp.SuccessCount)
}

func TestApiDownloaderTorrentActions_PerTargetError(t *testing.T) {
	fake := &fakeDownloader{
		torrents:      sampleTorrents(),
		batchPauseErr: assertErr("batchfail"),
		pauseErr:      assertErr("perfail"),
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

func TestApiDownloaderTorrentActions_RecheckError(t *testing.T) {
	fake := &fakeDownloader{torrents: sampleTorrents()}
	server, _ := setupServerWithFakeDownloader(t, fake)

	body, _ := json.Marshal(BatchTorrentActionRequest{
		Action:  "recheck",
		Targets: []TorrentActionTarget{{DownloaderID: 1, TaskID: "t1"}},
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
	server.apiDownloaderTorrentActions(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp BatchTorrentActionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.SuccessCount)
}
