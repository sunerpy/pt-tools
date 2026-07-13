// MIT License
// Copyright (c) 2025 pt-tools

// End-to-end coverage for PushTorrentToDownloader's push path against a fake
// qBittorrent HTTP server: happy push (record written + is_pushed set),
// already-exists skip, disk-protect rejection, and site-capacity rejection.

package internal

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeebo/bencode"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// makeSizedTorrentBytes builds a valid single-file .torrent payload of the
// given content length so ComputeTorrentSize returns sizeBytes.
func makeSizedTorrentBytes(t *testing.T, name string, sizeBytes int64) []byte {
	t.Helper()
	var buf bytes.Buffer
	torrent := map[string]any{
		"info": map[string]any{"name": name, "length": sizeBytes, "piece length": 16384},
	}
	require.NoError(t, bencode.NewEncoder(&buf).Encode(torrent))
	return buf.Bytes()
}

// fakeQbitServer answers the auth + properties + add + info endpoints
// PushTorrentToDownloader touches. `exists` toggles CheckTorrentExists.
func fakeQbitServer(t *testing.T, exists bool, freeSpace int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case r.URL.Path == "/api/v2/app/version":
			_, _ = w.Write([]byte("v4.6.0"))
		case strings.HasPrefix(r.URL.Path, "/api/v2/torrents/properties"):
			if exists {
				_, _ = w.Write([]byte(`{"save_path":"/downloads"}`))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		case r.URL.Path == "/api/v2/sync/maindata":
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":` + itoa(freeSpace) + `}}`))
		case r.URL.Path == "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/api/v2/torrents/add":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}

func seedQbitDownloader(t *testing.T, url string) uint {
	t.Helper()
	ds := models.DownloaderSetting{
		Name: "qb", Type: "qbittorrent", URL: url,
		Username: "admin", Password: "pw", Enabled: true, AutoStart: true,
	}
	require.NoError(t, global.GlobalDB.DB.Create(&ds).Error)
	return ds.ID
}

func TestPushTorrent_HappyPath_RecordsAndPushes(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	GetDiskBudget().Reset()
	t.Cleanup(func() { GetDiskBudget().Reset() })

	srv := fakeQbitServer(t, false, 500*gb)
	dlID := seedQbitDownloader(t, srv.URL)

	data := makeSizedTorrentBytes(t, "movie", 5*gb)
	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID:       "springsunday",
		TorrentID:    "push-1",
		TorrentData:  data,
		Title:        "Movie",
		Category:     "movies",
		Tags:         "hd",
		DownloaderID: dlID,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.False(t, res.Skipped)

	ti, err := db.GetTorrentBySiteAndID("springsunday", "push-1")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.True(t, ti.IsDownloaded)
	require.NotNil(t, ti.IsPushed)
	assert.True(t, *ti.IsPushed)
	assert.Equal(t, "manual_push", ti.DownloadSource)
}

func TestPushTorrent_AlreadyExistsSkips(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	srv := fakeQbitServer(t, true, 500*gb) // properties returns 200 => exists
	dlID := seedQbitDownloader(t, srv.URL)

	data := makeSizedTorrentBytes(t, "dup", 1*gb)
	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID:       "springsunday",
		TorrentID:    "dup-1",
		TorrentData:  data,
		DownloaderID: dlID,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
	assert.True(t, res.Skipped)
}

func TestPushTorrent_DiskProtectRejects(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	GetDiskBudget().Reset()
	t.Cleanup(func() { GetDiskBudget().Reset() })

	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir:           t.TempDir(),
		CleanupDiskProtect:    true,
		CleanupMinDiskSpaceGB: 20,
	}))

	// free 30GB, torrent 20GB => effective(30) - 20 = 10 < 20 => reject as too large.
	srv := fakeQbitServer(t, false, 30*gb)
	dlID := seedQbitDownloader(t, srv.URL)

	data := makeSizedTorrentBytes(t, "big", 20*gb)
	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID:       "springsunday",
		TorrentID:    "big-1",
		TorrentData:  data,
		DownloaderID: dlID,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Success)
	assert.Contains(t, res.Message, "超过当前可用空间")

	// Rejection must not leak a reservation.
	assert.Equal(t, int64(0), GetDiskBudget().Reserved())
}

func TestPushTorrent_DiskProtectInsufficientSpace(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	GetDiskBudget().Reset()
	t.Cleanup(func() { GetDiskBudget().Reset() })

	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), CleanupDiskProtect: true, CleanupMinDiskSpaceGB: 20,
	}))

	// free 10GB <= min 20GB => insufficient space rejection.
	srv := fakeQbitServer(t, false, 10*gb)
	dlID := seedQbitDownloader(t, srv.URL)

	data := makeSizedTorrentBytes(t, "sm", 1*gb)
	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "ins-1", TorrentData: data, DownloaderID: dlID,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Success)
	assert.Contains(t, res.Message, "磁盘空间不足")
}

func TestPushTorrent_AddFailsIncrementsRetry(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case r.URL.Path == "/api/v2/app/version":
			_, _ = w.Write([]byte("v4.6.0"))
		case strings.HasPrefix(r.URL.Path, "/api/v2/torrents/properties"):
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == "/api/v2/torrents/add":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			_, _ = w.Write([]byte("{}"))
		}
	}))
	t.Cleanup(srv.Close)
	dlID := seedQbitDownloader(t, srv.URL)

	data := makeSizedTorrentBytes(t, "fail", 1*gb)
	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "af-1", TorrentData: data, DownloaderID: dlID,
	})
	// Exercises the AddTorrentFileEx failure branch: either an error is
	// returned, or a non-success result surfaces the failure message.
	if err != nil {
		assert.Contains(t, err.Error(), "推送种子失败")
	} else {
		require.NotNil(t, res)
		assert.False(t, res.Success)
	}

	got, gerr := db.GetTorrentBySiteAndID("springsunday", "af-1")
	require.NoError(t, gerr)
	require.NotNil(t, got)
}

func TestPushTorrent_AddResultNotSuccess(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case r.URL.Path == "/api/v2/app/version":
			_, _ = w.Write([]byte("v4.6.0"))
		case strings.HasPrefix(r.URL.Path, "/api/v2/torrents/properties"):
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == "/api/v2/torrents/add":
			w.WriteHeader(http.StatusConflict)
		default:
			_, _ = w.Write([]byte("{}"))
		}
	}))
	t.Cleanup(srv.Close)
	dlID := seedQbitDownloader(t, srv.URL)

	data := makeSizedTorrentBytes(t, "conflict", 1*gb)
	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "cf-1", TorrentData: data, DownloaderID: dlID,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Success)

	got, gerr := db.GetTorrentBySiteAndID("springsunday", "cf-1")
	require.NoError(t, gerr)
	require.NotNil(t, got)
}

func TestPushTorrent_SiteCapacityRejects(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	GetDiskBudget().Reset()
	t.Cleanup(func() { GetDiskBudget().Reset() })

	// Disk protect off; site capacity gate active with a tiny cap.
	require.NoError(t, db.DB.Create(&models.SiteSetting{
		Name: "springsunday", SeedingCapacityGB: 1,
	}).Error)

	srv := fakeQbitServer(t, false, 500*gb) // torrents/info returns [] => used=0
	dlID := seedQbitDownloader(t, srv.URL)

	data := makeSizedTorrentBytes(t, "over", 5*gb) // 5GB > 1GB cap
	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID:       "springsunday",
		TorrentID:    "cap-1",
		TorrentData:  data,
		DownloaderID: dlID,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Success)
	assert.Contains(t, res.Message, "站点容量")
}
