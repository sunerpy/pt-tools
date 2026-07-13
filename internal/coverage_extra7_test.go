// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestFetchUnified_MinFreeMinutesSkips(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true, MinFreeMinutes: 120,
	}))

	soon := time.Now().Add(30 * time.Minute)
	srv := feedServerUnified(t, rssBody(itemXML("gmf", "SoonEnd", "http://x/s.torrent")))
	site := &unifiedFake{
		enabled: true,
		detail: &v2.TorrentItem{
			ID: "gmf", Title: "SoonEnd", DiscountLevel: v2.DiscountFree,
			SizeBytes: 1024, DiscountEndTime: soon,
		},
	}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "t"}))

	assert.Equal(t, int32(0), site.downloadCalls.Load(), "free ending too soon must skip")
	ti, err := db.GetTorrentBySiteAndID("springsunday", "gmf")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.True(t, ti.IsSkipped)
}

func TestFetchUnified_ExistingRowAlreadyDownloadedSkipsRedownload(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	// Pre-create a row already downloaded with the local file present so
	// shouldSkipSiteDownload short-circuits the re-download.
	future := time.Now().Add(time.Hour)
	pushed := false
	ti := &models.TorrentInfo{
		SiteName: "springsunday", TorrentID: "gex", FreeEndTime: &future, IsPushed: &pushed,
		IsDownloaded: true, RetryCount: 0,
	}
	require.NoError(t, db.UpsertTorrent(ti))

	srv := feedServerUnified(t, rssBody(itemXML("gex", "Existing", "http://x/e.torrent")))
	site := &unifiedFake{
		enabled: true,
		detail:  &v2.TorrentItem{ID: "gex", Title: "Existing", DiscountLevel: v2.DiscountFree, SizeBytes: 1024},
	}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "movie"}))
	// Detail fetched (row not skipped up-front since not pushed/skipped), but
	// re-download skipped by shouldSkipSiteDownload (already downloaded + file).
	assert.Equal(t, int32(1), site.detailCalls.Load())
}

func TestFetchUnified_MaxRetrySkipsRedownload(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true, MaxRetry: 2,
	}))

	future := time.Now().Add(time.Hour)
	pushed := false
	ti := &models.TorrentInfo{
		SiteName: "springsunday", TorrentID: "gmr", FreeEndTime: &future, IsPushed: &pushed, RetryCount: 5,
	}
	require.NoError(t, db.UpsertTorrent(ti))

	srv := feedServerUnified(t, rssBody(itemXML("gmr", "MaxRetry", "http://x/m.torrent")))
	site := &unifiedFake{
		enabled: true,
		detail:  &v2.TorrentItem{ID: "gmr", Title: "MaxRetry", DiscountLevel: v2.DiscountFree, SizeBytes: 1024},
	}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "movie"}))
	assert.Equal(t, int32(0), site.downloadCalls.Load(), "over-max-retry torrent must not re-download")
}

func TestShouldSkipSiteDownload_Branches(t *testing.T) {
	assert.Equal(t, false, mustSkip(nil, "", "", 0))
	pushed := true
	sk, _ := shouldSkipSiteDownload(&models.TorrentInfo{IsPushed: &pushed}, "", "f", 0)
	assert.True(t, sk)
	sk2, _ := shouldSkipSiteDownload(&models.TorrentInfo{RetryCount: 3}, "", "f", 2)
	assert.True(t, sk2)
	sk3, _ := shouldSkipSiteDownload(&models.TorrentInfo{}, "", "f", 0)
	assert.False(t, sk3)
}

func mustSkip(ti *models.TorrentInfo, path, base string, mr int) bool {
	s, _ := shouldSkipSiteDownload(ti, path, base, mr)
	return s
}

func TestShouldSkipExistingTorrent_Branches(t *testing.T) {
	assert.False(t, shouldSkipExistingTorrent(nil))

	// Skipped + free → skip.
	assert.True(t, shouldSkipExistingTorrent(&models.TorrentInfo{IsSkipped: true, IsFree: true}))

	// Skipped + non-free + recent check → skip.
	recent := time.Now()
	assert.True(t, shouldSkipExistingTorrent(&models.TorrentInfo{IsSkipped: true, IsFree: false, LastCheckTime: &recent}))

	// Skipped + non-free + stale check → allow re-check.
	old := time.Now().Add(-(SkipRecheckHours + 1) * time.Hour)
	assert.False(t, shouldSkipExistingTorrent(&models.TorrentInfo{IsSkipped: true, IsFree: false, LastCheckTime: &old}))

	// Pushed → skip.
	pushed := true
	assert.True(t, shouldSkipExistingTorrent(&models.TorrentInfo{IsPushed: &pushed}))

	// Fresh torrent → not skipped.
	assert.False(t, shouldSkipExistingTorrent(&models.TorrentInfo{}))
}
