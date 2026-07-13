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

func TestApiAddDownloaderTorrent_ListEnabledAndGetDownloaderError(t *testing.T) {
	fake := &fakeDownloader{freSpace: 1 << 40}
	server, _ := setupServerWithFakeDownloader(t, fake)
	gs, err := server.store.GetGlobalSettings()
	require.NoError(t, err)
	require.NoError(t, server.store.SaveGlobalSettings(gs))
	require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
		Where("1 = 1").Update("cleanup_disk_protect", false).Error)

	// A second enabled downloader present in DB but NOT registered in the manager,
	// exercising the GetDownloader error branch.
	require.NoError(t, global.GlobalDB.DB.Create(&models.DownloaderSetting{
		Name: "phantom", Type: "qbittorrent", URL: "http://127.0.0.1:2", Enabled: true,
	}).Error)

	body, _ := json.Marshal(AddDownloaderTorrentRequest{
		MagnetLink: "magnet:?xt=urn:btih:abc",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/add", bytes.NewReader(body))
	server.apiAddDownloaderTorrent(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp AddDownloaderTorrentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.GreaterOrEqual(t, resp.SuccessCount+resp.FailedCount, 1)
}

func TestApiCloakTest_TokenFromStoreOnly(t *testing.T) {
	srv, store, cleanup := newCloakTestServer(t)
	defer cleanup()

	mock := newCloakManagerMock(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","version":"2.0.0"}`))
	})
	defer mock.Close()

	require.NoError(t, store.SaveCloakConfig(mock.URL, "stored-tok", false))

	// Provide endpoint in request but NO token -> token loaded from store.
	body := map[string]any{"endpoint": mock.URL}
	w := httptest.NewRecorder()
	srv.apiCloakTest(w, cloakAuthedReq(http.MethodPost, "/api/cloak/test", body))
	require.Equal(t, http.StatusOK, w.Code)
	var out map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Equal(t, "success", out["category"])
}
