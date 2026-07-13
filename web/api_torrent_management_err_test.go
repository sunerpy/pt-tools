package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestApiDeletePausedTorrents_RemoveError(t *testing.T) {
	fake := &fakeDownloader{removeErr: errors.New("cannot remove")}
	server, _ := setupServerWithFakeDownloader(t, fake)
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.TorrentInfo{}))

	now := time.Now()
	require.NoError(t, global.GlobalDB.DB.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "1", Title: "P1", IsPausedBySystem: true,
		PausedAt: &now, DownloaderName: "qb1", DownloaderTaskID: "task-1",
	}).Error)

	body, _ := json.Marshal(DeletePausedRequest{RemoveData: true})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/torrents/delete-paused", bytes.NewReader(body))
	server.apiDeletePausedTorrents(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp DeletePausedResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.Failed)
}

func TestApiDeleteTasks_SkipsPushed(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}))

	pushed := true
	require.NoError(t, db.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "1", Title: "Unpushed", IsPushed: &pushed,
	}).Error)
	unpushed := false
	require.NoError(t, db.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "2", Title: "Deletable", IsPushed: &unpushed,
	}).Error)

	body, _ := json.Marshal(DeleteTasksRequest{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tasks/batch-delete", bytes.NewReader(body))
	server.apiDeleteTasks(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp DeleteTasksResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.Success)
}
