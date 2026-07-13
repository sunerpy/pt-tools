// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

func addResultFailServer(t *testing.T, freeSpace int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case r.URL.Path == "/api/v2/app/version":
			_, _ = w.Write([]byte("v4.6.0"))
		case strings.HasPrefix(r.URL.Path, "/api/v2/torrents/properties"):
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == "/api/v2/sync/maindata":
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":` + itoa(freeSpace) + `}}`))
		case r.URL.Path == "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/api/v2/torrents/add":
			w.WriteHeader(http.StatusConflict)
		default:
			_, _ = w.Write([]byte("{}"))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestPushTorrent_DiskProtectReserveThenAddFails(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	GetDiskBudget().Reset()
	t.Cleanup(func() { GetDiskBudget().Reset() })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), CleanupDiskProtect: true, CleanupMinDiskSpaceGB: 5,
	}))

	srv := addResultFailServer(t, 500*gb)
	dlID := seedQbitDownloader(t, srv.URL)

	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "dpaf", TorrentData: makeSizedTorrentBytes(t, "m", 5*gb), DownloaderID: dlID,
	})
	if err == nil {
		require.NotNil(t, res)
		assert.False(t, res.Success)
	}
	// Reservation released on failure (budget back to 0).
	assert.Equal(t, int64(0), GetDiskBudget().Reserved())
}

func TestProcessTorrentsWithDBUpdate_OrphanDeleted(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	_ = db
	dir := t.TempDir()
	path, _ := makeTorrentFile(t, dir)

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, ProcessTorrentsWithDBUpdate(context.Background(), client, dir, "cat", "tag", models.SiteGroup("springsunday")))
	// Orphan (no DB row) is deleted by processSingleTorrent.
	assert.NoFileExists(t, path)
}
