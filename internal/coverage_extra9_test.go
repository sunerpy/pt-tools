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

func TestPushTorrent_SiteCapacityReadFails(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true,
	}))

	require.NoError(t, db.DB.Create(&models.SiteSetting{Name: "springsunday", SeedingCapacityGB: 100}).Error)

	// torrents/info 500 → getSiteSeedingSizeBytes fails → reject.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case r.URL.Path == "/api/v2/app/version":
			_, _ = w.Write([]byte("v4.6.0"))
		case strings.HasPrefix(r.URL.Path, "/api/v2/torrents/properties"):
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == "/api/v2/sync/maindata":
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":536870912000}}`))
		case r.URL.Path == "/api/v2/torrents/info":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			_, _ = w.Write([]byte("{}"))
		}
	}))
	t.Cleanup(srv.Close)
	dlID := seedQbitDownloader(t, srv.URL)

	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "capfail", TorrentData: makeSizedTorrentBytes(t, "x", gb), DownloaderID: dlID,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Success)
	assert.Contains(t, res.Message, "站点做种总量")
}

func TestPushTorrent_SiteCapacityAndDiskProtectCombined(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	GetDiskBudget().Reset()
	t.Cleanup(func() { GetDiskBudget().Reset() })

	require.NoError(t, db.DB.Create(&models.SiteSetting{Name: "springsunday", SeedingCapacityGB: 1}).Error)
	srv := fakeQbitServer(t, false, 500*gb) // torrents/info returns [] → used=0
	dlID := seedQbitDownloader(t, srv.URL)

	// 5GB torrent > 1GB cap → reject on site-capacity gate.
	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "capover", TorrentData: makeSizedTorrentBytes(t, "big", 5*gb), DownloaderID: dlID,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Success)
	assert.Contains(t, res.Message, "站点容量")
}

func TestSiteSeedingCapacityGB_Lookup(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	assert.Equal(t, float64(0), siteSeedingCapacityGB(""))
	assert.Equal(t, float64(0), siteSeedingCapacityGB("unknown-site"))
	require.NoError(t, db.DB.Create(&models.SiteSetting{Name: "springsunday", SeedingCapacityGB: 42}).Error)
	assert.Equal(t, float64(42), siteSeedingCapacityGB("springsunday"))
}

func TestSiteSeedingCapacityGB_NilDB(t *testing.T) {
	global.GlobalDB = nil
	assert.Equal(t, float64(0), siteSeedingCapacityGB("springsunday"))
}
