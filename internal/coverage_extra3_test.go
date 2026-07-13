// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func TestFetchUnified_FiresFilteredNotification(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{
		Enabled: boolPtrIX(true), AuthMethod: "cookie", Cookie: "c=1",
		RSS: []models.RSSConfig{{Name: "sub", URL: "http://placeholder", IntervalMinutes: 1, Tag: "movie"}},
	}))
	var sub models.RSSSubscription
	require.NoError(t, db.DB.First(&sub).Error)

	rule := models.FilterRule{
		Name: "notify", Pattern: ".*", PatternType: "regex", MatchField: "both",
		RequireFree: false, Enabled: true, Priority: 100, Purpose: "notify",
	}
	require.NoError(t, db.DB.Create(&rule).Error)
	assoc := models.NewRSSFilterAssociationDB(db.DB)
	require.NoError(t, assoc.SetFilterRulesForRSS(sub.ID, []uint{rule.ID}))

	n := &recordingNotifier{}
	SetRSSNotifier(n)
	t.Cleanup(func() { SetRSSNotifier(nil) })

	srv := feedServerUnified(t, rssBody(itemXML("gf1", "FilterNotify", "http://x/f.torrent")))
	site := &unifiedFake{
		enabled: true,
		detail:  &v2.TorrentItem{ID: "gf1", Title: "FilterNotify", DiscountLevel: v2.DiscountFree, SizeBytes: 4096},
	}
	cfg := models.RSSConfig{ID: sub.ID, Name: sub.Name, URL: srv.URL, NotifyMode: "filtered", Tag: "movie"}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site, cfg))

	assert.GreaterOrEqual(t, int(n.filteredItems.Load()), 1)
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
