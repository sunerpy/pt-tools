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
)

func TestApiAddDownloaderTorrent_DiskProtectSpaceLow(t *testing.T) {
	fake := &fakeDownloader{freSpace: 1024}
	server, _ := setupServerWithFakeDownloader(t, fake)
	gs, err := server.store.GetGlobalSettings()
	require.NoError(t, err)
	require.NoError(t, server.store.SaveGlobalSettings(gs))
	require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
		Where("1 = 1").Updates(map[string]any{
		"cleanup_disk_protect":      true,
		"cleanup_min_disk_space_gb": 100.0,
	}).Error)

	torrentB64 := base64.StdEncoding.EncodeToString(minimalTorrentBytes(50 * 1024 * 1024 * 1024))
	body, _ := json.Marshal(AddDownloaderTorrentRequest{
		DownloaderIDs: []uint{1},
		TorrentBase64: torrentB64,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/add", bytes.NewReader(body))
	server.apiAddDownloaderTorrent(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp AddDownloaderTorrentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.FailedCount)
}
