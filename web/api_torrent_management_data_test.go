package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestApiDeletePausedTorrents_WithDownloader(t *testing.T) {
	fake := &fakeDownloader{}
	server, _ := setupServerWithFakeDownloader(t, fake)
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.TorrentInfo{}))

	now := time.Now()
	require.NoError(t, global.GlobalDB.DB.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "1", Title: "Paused A", IsPausedBySystem: true,
		PausedAt: &now, DownloaderName: "qb1", DownloaderTaskID: "task-1",
	}).Error)
	require.NoError(t, global.GlobalDB.DB.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "2", Title: "Paused B", IsPausedBySystem: true,
		PausedAt: &now,
	}).Error)

	body, _ := json.Marshal(DeletePausedRequest{RemoveData: false})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/torrents/delete-paused", bytes.NewReader(body))
	server.apiDeletePausedTorrents(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp DeletePausedResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.Success)
}

func TestApiResumeTorrent_WithDownloader(t *testing.T) {
	fake := &fakeDownloader{}
	server, _ := setupServerWithFakeDownloader(t, fake)
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.TorrentInfo{}))

	now := time.Now()
	require.NoError(t, global.GlobalDB.DB.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "1", Title: "Paused A", IsPausedBySystem: true,
		PausedAt: &now, DownloaderName: "qb1", DownloaderTaskID: "task-1",
	}).Error)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/torrents/1/resume", nil)
	server.apiResumeTorrent(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp ResumeTorrentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)

	var row models.TorrentInfo
	require.NoError(t, global.GlobalDB.DB.First(&row, 1).Error)
	assert.False(t, row.IsPausedBySystem)
}
