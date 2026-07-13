package web

import (
	"bytes"
	"context"
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

// minimalTorrentBytes returns a bencoded single-file torrent whose info.length
// equals the requested size. Sufficient for qbit.ComputeTorrentSize.
func minimalTorrentBytes(length int64) []byte {
	var b bytes.Buffer
	b.WriteString("d4:infod6:lengthi")
	b.WriteString(torrentItoa(length))
	b.WriteString("e4:name4:test12:piece lengthi16384e6:pieces0:ee")
	return b.Bytes()
}

func torrentItoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func TestReserveDownloaderAddDiskBudget(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.SettingsGlobal{}))

	fake := &fakeDownloader{freSpace: 100 * 1024 * 1024 * 1024, name: "qb1"}
	ctx := context.Background()

	t.Run("disk protect off returns 0", func(t *testing.T) {
		gs, err := server.store.GetGlobalSettings()
		require.NoError(t, err)
		require.NoError(t, server.store.SaveGlobalSettings(gs))
		require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
			Where("1 = 1").Update("cleanup_disk_protect", false).Error)

		size, err := reserveDownloaderAddDiskBudget(ctx, fake, minimalTorrentBytes(1024), "")
		require.NoError(t, err)
		assert.Equal(t, int64(0), size)
	})

	t.Run("disk protect on rejects magnet without bytes", func(t *testing.T) {
		gs, err := server.store.GetGlobalSettings()
		require.NoError(t, err)
		require.NoError(t, server.store.SaveGlobalSettings(gs))
		require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
			Where("1 = 1").Updates(map[string]any{"cleanup_disk_protect": true, "cleanup_min_disk_space_gb": 10}).Error)

		_, err = reserveDownloaderAddDiskBudget(ctx, fake, nil, "magnet:?xt=urn:btih:x")
		assert.Error(t, err)
	})

	t.Run("disk protect on with sufficient space reserves", func(t *testing.T) {
		gs, err := server.store.GetGlobalSettings()
		require.NoError(t, err)
		require.NoError(t, server.store.SaveGlobalSettings(gs))
		require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
			Where("1 = 1").Updates(map[string]any{"cleanup_disk_protect": true, "cleanup_min_disk_space_gb": 10}).Error)

		size, err := reserveDownloaderAddDiskBudget(ctx, fake, minimalTorrentBytes(1024), "")
		require.NoError(t, err)
		assert.Equal(t, int64(1024), size)
	})
}

func TestApiAddDownloaderTorrent_Base64Path(t *testing.T) {
	fake := &fakeDownloader{
		addResult: downloader.AddTorrentResult{Success: true, ID: "n1", Hash: "h1", Message: "ok"},
		freSpace:  1 << 40,
	}
	server, _ := setupServerWithFakeDownloader(t, fake)
	require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
		Where("1 = 1").Update("cleanup_disk_protect", false).Error)

	torrentB64 := base64.StdEncoding.EncodeToString(minimalTorrentBytes(2048))
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
	assert.Equal(t, 1, resp.SuccessCount)
}
