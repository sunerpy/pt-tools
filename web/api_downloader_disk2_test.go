package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestReserveDownloaderAddDiskBudget_MoreBranches(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.SettingsGlobal{}))
	gs, err := server.store.GetGlobalSettings()
	require.NoError(t, err)
	require.NoError(t, server.store.SaveGlobalSettings(gs))
	require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
		Where("1 = 1").Updates(map[string]any{"cleanup_disk_protect": true, "cleanup_min_disk_space_gb": 10}).Error)

	ctx := context.Background()

	t.Run("invalid torrent bytes", func(t *testing.T) {
		fake := &fakeDownloader{freSpace: 100 << 30, name: "qb1"}
		_, err := reserveDownloaderAddDiskBudget(ctx, fake, []byte("not-bencode"), "")
		assert.Error(t, err)
	})

	t.Run("insufficient space rejects", func(t *testing.T) {
		fake := &fakeDownloader{freSpace: 1 << 30, name: "qb1"}
		_, err := reserveDownloaderAddDiskBudget(ctx, fake, minimalTorrentBytes(5<<30), "")
		assert.Error(t, err)
	})
}

func TestApiDownloaderTorrentDetail_FilesFallback(t *testing.T) {
	fake := &fakeDownloader{
		torrents: sampleTorrents(),
		getErr:   nil,
	}
	server, _ := setupServerWithFakeDownloader(t, fake)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/detail?downloader_id=1&task_id=t1", nil)
	server.apiDownloaderTorrentDetail(w, req)
	require.Equal(t, 200, w.Code)
}
