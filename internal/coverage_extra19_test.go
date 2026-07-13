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
)

func TestPushTorrent_AlreadyExistsSkipEarly(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	srv := fakeQbitServer(t, true, 500*gb)
	dlID := seedQbitDownloaderNamed(t, "qb-exist-early", srv.URL)

	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "exist-early", TorrentData: makeSizedTorrentBytes(t, "d", gb), DownloaderID: dlID,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Skipped)
}

func seedQbitDownloaderNamed(t *testing.T, name, url string) uint {
	t.Helper()
	ds := models.DownloaderSetting{
		Name: name, Type: "qbittorrent", URL: url,
		Username: "admin", Password: "pw", Enabled: true, AutoStart: true,
	}
	require.NoError(t, global.GlobalDB.DB.Create(&ds).Error)
	return ds.ID
}

func TestPushTorrent_AddResultNotSuccessBranch(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true,
	}))
	require.NoError(t, db.DB.Create(&models.TorrentInfo{SiteName: "springsunday", TorrentID: "cf2"}).Error)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case r.URL.Path == "/api/v2/app/version":
			_, _ = w.Write([]byte("v5.2.0"))
		case strings.HasPrefix(r.URL.Path, "/api/v2/torrents/properties"):
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == "/api/v2/sync/maindata":
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":536870912000}}`))
		case r.URL.Path == "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/api/v2/torrents/add":
			w.WriteHeader(http.StatusConflict)
		default:
			_, _ = w.Write([]byte("{}"))
		}
	}))
	t.Cleanup(srv.Close)
	dlID := seedQbitDownloaderNamed(t, "qb-conflict-branch", srv.URL)

	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "cf2", TorrentData: makeSizedTorrentBytes(t, "x", gb), DownloaderID: dlID,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Success)
}
