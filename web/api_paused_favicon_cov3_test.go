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

func TestApiDeletePausedTorrents_DownloaderNotFound(t *testing.T) {
	server, _ := setupServerWithFakeDownloader(t, &fakeDownloader{})
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.TorrentInfo{}))

	ti := models.TorrentInfo{
		SiteName: "s", TorrentID: "p2", IsPausedBySystem: true,
		DownloaderTaskID: "task-p2", DownloaderName: "no-such-dl",
	}
	require.NoError(t, global.GlobalDB.DB.Create(&ti).Error)

	body, _ := json.Marshal(DeletePausedRequest{IDs: []uint{ti.ID}})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/torrents/delete-paused", bytes.NewReader(body))
	server.apiDeletePausedTorrents(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp DeletePausedResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.Success)
}

func TestServeFaviconData_HeadersAndData(t *testing.T) {
	server := setupFaviconServer(t)
	cache := &models.FaviconCache{ContentType: "image/png", ETag: "e9", Data: []byte{9, 8, 7}}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicon/x", nil)
	server.serveFaviconData(w, req, cache)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/png", w.Header().Get("Content-Type"))
	assert.Equal(t, `"e9"`, w.Header().Get("ETag"))
	assert.Equal(t, []byte{9, 8, 7}, w.Body.Bytes())
}

func TestApiFaviconList_EmptyEnabled(t *testing.T) {
	server := setupFaviconServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicons", nil)
	server.apiFaviconList(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp []SiteFaviconInfo
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp)
}
