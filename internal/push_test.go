// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	sm "github.com/sunerpy/pt-tools/mocks"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

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

func TestPushTorrent_DiskProtectPendingBytesError(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	GetDiskBudget().Reset()
	t.Cleanup(func() { GetDiskBudget().Reset() })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), CleanupDiskProtect: true, CleanupMinDiskSpaceGB: 5,
	}))

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
		case r.URL.Path == "/api/v2/torrents/add":
			_, _ = w.Write([]byte("Ok."))
		default:
			_, _ = w.Write([]byte("{}"))
		}
	}))
	t.Cleanup(srv.Close)
	dlID := seedQbitDownloader(t, srv.URL)

	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "pbe", TorrentData: makeSizedTorrentBytes(t, "m", 5*gb), DownloaderID: dlID,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success, "pending-bytes read failure is non-fatal (treated as 0)")
}

func TestPushTorrent_DiskProtectReservedMakesNegativeClamped(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	GetDiskBudget().Reset()
	t.Cleanup(func() { GetDiskBudget().Reset() })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), CleanupDiskProtect: true, CleanupMinDiskSpaceGB: 5,
	}))

	GetDiskBudget().Reserve(1000 * gb)
	srv := fakeQbitServer(t, false, 100*gb)
	dlID := seedQbitDownloader(t, srv.URL)

	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "neg", TorrentData: makeSizedTorrentBytes(t, "m", gb), DownloaderID: dlID,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Success)
	assert.Contains(t, res.Message, "磁盘空间不足")
}

func TestRecordPushDiskProtectError_NilDB(t *testing.T) {
	global.GlobalDB = nil
	require.NotPanics(t, func() { recordPushDiskProtectError("s", "id", "msg") })
}

func TestRecordPushDiskProtectError_WritesLastError(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, db.DB.Create(&models.TorrentInfo{SiteName: "springsunday", TorrentID: "rp1"}).Error)
	recordPushDiskProtectError("springsunday", "rp1", "磁盘满")
	got, err := db.GetTorrentBySiteAndID("springsunday", "rp1")
	require.NoError(t, err)
	assert.Equal(t, "磁盘满", got.LastError)
}

func TestPushTorrentToDownloader_NilDB(t *testing.T) {
	global.GlobalDB = nil
	_, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "数据库未初始化")
}

func TestPushTorrentToDownloader_ComputeHashFails(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	srv := fakeQbitServer(t, false, 500*gb)
	dlID := seedQbitDownloader(t, srv.URL)
	_ = db

	_, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "bad", TorrentData: []byte("not-a-torrent"), DownloaderID: dlID,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "计算种子哈希")
}

func TestPushTorrentToDownloader_DisabledDownloader(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	ds := models.DownloaderSetting{Name: "off", Type: "qbittorrent", URL: "http://x", Enabled: false}
	require.NoError(t, db.DB.Create(&ds).Error)
	_, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "d1", TorrentData: makeSizedTorrentBytes(t, "x", gb), DownloaderID: ds.ID,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "未启用")
}

func TestPushTorrentToDownloader_UnknownDownloaderID(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	_, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "d1", TorrentData: []byte("x"), DownloaderID: 99999,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "获取下载器")
}

func TestPushTorrent_DiskProtectOnSuccessReserves(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	GetDiskBudget().Reset()
	t.Cleanup(func() { GetDiskBudget().Reset() })

	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), CleanupDiskProtect: true, CleanupMinDiskSpaceGB: 5,
	}))
	// free 500GB, torrent 5GB, min 5GB → 495 - 5 = 490 >= 5 → reserve + add.
	srv := fakeQbitServer(t, false, 500*gb)
	dlID := seedQbitDownloader(t, srv.URL)

	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "dp-ok", TorrentData: makeSizedTorrentBytes(t, "m", 5*gb), DownloaderID: dlID,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success)
}

func TestPushTorrent_SiteCapacityPassesThrough(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	GetDiskBudget().Reset()
	t.Cleanup(func() { GetDiskBudget().Reset() })

	require.NoError(t, db.DB.Create(&models.SiteSetting{Name: "springsunday", SeedingCapacityGB: 100}).Error)
	srv := fakeQbitServer(t, false, 500*gb) // torrents/info returns [] → used=0
	dlID := seedQbitDownloader(t, srv.URL)

	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "cap-ok", TorrentData: makeSizedTorrentBytes(t, "s", 2*gb), DownloaderID: dlID,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Success, "2GB under 100GB cap should push")
}

func TestApplySiteSpeedLimits_ReadsRow(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, db.DB.Create(&models.SiteSetting{Name: "springsunday", UploadLimitKBs: 100, DownloadLimitKBs: 200}).Error)

	o := &downloader.AddTorrentOptions{}
	applySiteSpeedLimits(o, "springsunday")
	assert.Equal(t, 100, o.UploadSpeedLimitKBs)
	assert.Equal(t, 200, o.DownloadSpeedLimitKBs)

	o2 := &downloader.AddTorrentOptions{}
	applySiteSpeedLimits(o2, "")
	assert.Equal(t, 0, o2.UploadSpeedLimitKBs)
}

func TestPushTorrent_AddErrorReturnsError(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	_ = db

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case r.URL.Path == "/api/v2/app/version":
			_, _ = w.Write([]byte("v4.6.0"))
		case strings.HasPrefix(r.URL.Path, "/api/v2/torrents/properties"):
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == "/api/v2/torrents/add":
			w.WriteHeader(http.StatusForbidden)
		default:
			_, _ = w.Write([]byte("{}"))
		}
	}))
	t.Cleanup(srv.Close)
	dlID := seedQbitDownloader(t, srv.URL)

	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "adderr", TorrentData: makeSizedTorrentBytes(t, "x", gb), DownloaderID: dlID,
	})
	if err != nil {
		assert.Contains(t, err.Error(), "推送种子失败")
	} else {
		require.NotNil(t, res)
		assert.False(t, res.Success)
	}
}

func TestPushTorrent_DiskProtectFreeSpaceReadFails(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	GetDiskBudget().Reset()
	t.Cleanup(func() { GetDiskBudget().Reset() })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), CleanupDiskProtect: true, CleanupMinDiskSpaceGB: 10,
	}))

	// maindata endpoint 500 → GetClientFreeSpace fails → reject.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case r.URL.Path == "/api/v2/app/version":
			_, _ = w.Write([]byte("v4.6.0"))
		case strings.HasPrefix(r.URL.Path, "/api/v2/torrents/properties"):
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == "/api/v2/sync/maindata":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			_, _ = w.Write([]byte("{}"))
		}
	}))
	t.Cleanup(srv.Close)
	dlID := seedQbitDownloader(t, srv.URL)

	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "fsp", TorrentData: makeSizedTorrentBytes(t, "x", gb), DownloaderID: dlID,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Success)
	assert.Contains(t, res.Message, "无法读取磁盘空间")
}

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

// TestRSSPushPath_AppliesSiteSpeedLimits is the end-to-end regression guard for
// the bug where the RSS/common push path never applied per-site speed limits
// (only the manual push path did). It drives processSingleTorrentWithDownloader
// and captures the AddTorrentOptions handed to the downloader, asserting the
// site's UploadLimitKBs/DownloadLimitKBs actually reach AddTorrentFileEx.
func TestRSSPushPath_AppliesSiteSpeedLimits(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	makeTorrentInfoWithSize(t, global.GlobalDB, hash, 1*gb)

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{
		Name:             "springsunday",
		AuthMethod:       "cookie",
		UploadLimitKBs:   500,
		DownloadLimitKBs: 2000,
	}).Error)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("test-dl").AnyTimes()
	mockDl.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(80*gb, nil)
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil)

	var captured downloader.AddTorrentOptions
	mockDl.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ []byte, opt downloader.AddTorrentOptions) (downloader.AddTorrentResult, error) {
			captured = opt
			return downloader.AddTorrentResult{Success: true, Hash: hash}, nil
		})

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	require.NoError(t, err)

	assert.Equal(t, 500, captured.UploadSpeedLimitKBs,
		"RSS push path must apply site upload limit (regression: previously never called applySiteSpeedLimits)")
	assert.Equal(t, 2000, captured.DownloadSpeedLimitKBs,
		"RSS push path must apply site download limit")
}

// TestApplySiteSpeedLimits_PopulatesFromSiteRow verifies the core integration
// point for issue #276: push flow reads per-site speed limits from SiteSetting
// and populates AddTorrentOptions, so the downstream downloader.AddTorrentFileEx
// applies them atomically.
func TestApplySiteSpeedLimits_PopulatesFromSiteRow(t *testing.T) {
	db := setupDB(t)
	require.NoError(t, db.DB.Create(&models.SiteSetting{
		Name:             "springsunday",
		AuthMethod:       "cookie",
		UploadLimitKBs:   500,
		DownloadLimitKBs: 2000,
	}).Error)

	opts := downloader.AddTorrentOptions{}
	applySiteSpeedLimits(&opts, "springsunday")

	assert.Equal(t, 500, opts.UploadSpeedLimitKBs)
	assert.Equal(t, 2000, opts.DownloadSpeedLimitKBs)
}

// TestApplySiteSpeedLimits_ZeroLimits verifies that sites with no limits
// configured leave opts at zero (meaning "unlimited" downstream). Regression
// guard: ensure the feature is truly opt-in.
func TestApplySiteSpeedLimits_ZeroLimits(t *testing.T) {
	db := setupDB(t)
	require.NoError(t, db.DB.Create(&models.SiteSetting{
		Name:       "springsunday",
		AuthMethod: "cookie",
	}).Error)

	opts := downloader.AddTorrentOptions{}
	applySiteSpeedLimits(&opts, "springsunday")

	assert.Equal(t, 0, opts.UploadSpeedLimitKBs)
	assert.Equal(t, 0, opts.DownloadSpeedLimitKBs)
}

// TestApplySiteSpeedLimits_UnknownSiteNoOp verifies that an unknown site name
// is silently a no-op (opts unchanged). This is the safety contract that
// allows the push flow to pass any SiteID without risking a panic or error.
func TestApplySiteSpeedLimits_UnknownSiteNoOp(t *testing.T) {
	_ = setupDB(t)

	opts := downloader.AddTorrentOptions{UploadSpeedLimitKBs: 123, DownloadSpeedLimitKBs: 456}
	applySiteSpeedLimits(&opts, "nonexistent-site")

	assert.Equal(t, 123, opts.UploadSpeedLimitKBs, "pre-existing value must not be wiped")
	assert.Equal(t, 456, opts.DownloadSpeedLimitKBs)
}

// TestApplySiteSpeedLimits_EmptySiteNameNoOp verifies no-op on empty siteName.
func TestApplySiteSpeedLimits_EmptySiteNameNoOp(t *testing.T) {
	_ = setupDB(t)
	opts := downloader.AddTorrentOptions{}
	applySiteSpeedLimits(&opts, "")
	assert.Equal(t, 0, opts.UploadSpeedLimitKBs)
	assert.Equal(t, 0, opts.DownloadSpeedLimitKBs)
}

// TestApplySiteSpeedLimits_NilOpts verifies no panic on nil.
func TestApplySiteSpeedLimits_NilOpts(t *testing.T) {
	_ = setupDB(t)
	assert.NotPanics(t, func() {
		applySiteSpeedLimits(nil, "any-site")
	})
}

// TestApplySiteSpeedLimits_NilDB verifies no panic when DB is not initialized.
// Regression guard: push flow should not crash during early-stage testing
// where global.GlobalDB may be unset.
func TestApplySiteSpeedLimits_NilDB(t *testing.T) {
	origDB := global.GlobalDB
	global.GlobalDB = nil
	defer func() { global.GlobalDB = origDB }()

	opts := downloader.AddTorrentOptions{}
	assert.NotPanics(t, func() {
		applySiteSpeedLimits(&opts, "any-site")
	})
	assert.Equal(t, 0, opts.UploadSpeedLimitKBs)
}

// TestApplySiteSpeedLimits_SiteRowUpdates verifies that changes to the site
// row are reflected immediately (no caching). Regression guard: if caching
// is ever introduced, it must correctly invalidate on settings change.
func TestApplySiteSpeedLimits_SiteRowUpdates(t *testing.T) {
	db := setupDB(t)
	site := &models.SiteSetting{
		Name:             "hdsky",
		AuthMethod:       "cookie",
		UploadLimitKBs:   100,
		DownloadLimitKBs: 200,
	}
	require.NoError(t, db.DB.Create(site).Error)

	opts1 := downloader.AddTorrentOptions{}
	applySiteSpeedLimits(&opts1, "hdsky")
	assert.Equal(t, 100, opts1.UploadSpeedLimitKBs)

	site.UploadLimitKBs = 999
	require.NoError(t, db.DB.Save(site).Error)

	opts2 := downloader.AddTorrentOptions{}
	applySiteSpeedLimits(&opts2, "hdsky")
	assert.Equal(t, 999, opts2.UploadSpeedLimitKBs, "updated limit must be reflected on next push")
}

func TestPushTorrent_DBNil(t *testing.T) {
	global.GlobalDB = nil
	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "数据库未初始化")
}

func TestPushTorrent_DownloaderNotFound(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		DownloaderID: 999, // nonexistent
	})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "获取下载器失败")
}

func TestPushTorrent_DownloaderDisabled(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	ds := models.DownloaderSetting{Name: "off", Type: "qbittorrent", URL: "http://x", Enabled: false}
	require.NoError(t, db.DB.Create(&ds).Error)

	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{DownloaderID: ds.ID})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "未启用")
}

func TestPushTorrent_UnsupportedType(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	ds := models.DownloaderSetting{Name: "weird", Type: "aria2", URL: "http://x", Enabled: true}
	require.NoError(t, db.DB.Create(&ds).Error)

	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{DownloaderID: ds.ID})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "创建下载器实例失败")
}

func TestPushTorrent_InstanceCreationFailsUnreachable(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	// qbittorrent constructor eagerly authenticates; an unreachable URL makes
	// createDownloaderInstanceForPush fail before hashing.
	ds := models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:0", Enabled: true}
	require.NoError(t, db.DB.Create(&ds).Error)

	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID:       "springsunday",
		TorrentID:    "t1",
		TorrentData:  []byte("not a torrent"),
		DownloaderID: ds.ID,
	})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "创建下载器实例失败")
}

func TestCreateDownloaderInstanceForPush(t *testing.T) {
	// qbittorrent branch executes (connection may fail; we only cover the branch).
	_, _ = createDownloaderInstanceForPush(models.DownloaderSetting{
		Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:0",
	})

	// transmission branch executes.
	_, _ = createDownloaderInstanceForPush(models.DownloaderSetting{
		Name: "tr", Type: "Transmission", URL: "http://127.0.0.1:0", AutoStart: true,
	})

	// unsupported -> error.
	_, err := createDownloaderInstanceForPush(models.DownloaderSetting{Name: "x", Type: "deluge"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不支持的下载器类型")
}

func TestRecordPushDiskProtectError(t *testing.T) {
	// DB nil -> no-op, no panic.
	global.GlobalDB = nil
	recordPushDiskProtectError("s", "t", "msg")

	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "t9", IsPushed: &pushed}
	require.NoError(t, db.UpsertTorrent(ti))

	recordPushDiskProtectError("springsunday", "t9", "磁盘空间不足")

	got, err := db.GetTorrentBySiteAndID("springsunday", "t9")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "磁盘空间不足", got.LastError)
}

func TestApplySiteSpeedLimits(t *testing.T) {
	// nil opts -> no-op, no panic.
	applySiteSpeedLimits(nil, "s")

	// empty site name -> no-op leaves opts zeroed.
	opts := &downloader.AddTorrentOptions{}
	applySiteSpeedLimits(opts, "")
	assert.Zero(t, opts.UploadSpeedLimitKBs)

	// DB nil -> no-op.
	global.GlobalDB = nil
	applySiteSpeedLimits(opts, "mteam")
	assert.Zero(t, opts.UploadSpeedLimitKBs)

	// Unknown site (lookup fails) -> no-op leaves zeroed.
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	_ = db
	applySiteSpeedLimits(opts, "nosuchsite")
	assert.Zero(t, opts.UploadSpeedLimitKBs)

	// Known site -> limits applied.
	require.NoError(t, db.DB.Create(&models.SiteSetting{
		Name: "mteam", UploadLimitKBs: 100, DownloadLimitKBs: 200,
	}).Error)
	applySiteSpeedLimits(opts, "mteam")
	assert.Equal(t, 100, opts.UploadSpeedLimitKBs)
	assert.Equal(t, 200, opts.DownloadSpeedLimitKBs)
}
