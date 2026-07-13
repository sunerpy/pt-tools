// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestFetchAndDownloadFreeRSS_BlankDownloadDir(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, db.DB.Exec("DELETE FROM settings_globals").Error)
	require.NoError(t, db.DB.Create(&models.SettingsGlobal{DownloadDir: "", DefaultIntervalMinutes: 10}).Error)

	err := FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"),
		&legacyPTStub{enabled: true}, models.RSSConfig{URL: "http://x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "下载目录为空")
}

func TestFetchAndDownloadFreeRSS_FeedFetchError(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	err := FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"),
		&legacyPTStub{enabled: true}, models.RSSConfig{URL: "http://127.0.0.1:0/rss"})
	require.Error(t, err)
}

func TestFetchAndDownloadFreeRSS_AlreadySkippedRecheckSkips(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	recent := time.Now()
	ti := &models.TorrentInfo{
		SiteName: "springsunday", TorrentID: "lgsk", IsSkipped: true, IsFree: false, LastCheckTime: &recent,
	}
	require.NoError(t, db.UpsertTorrent(ti))

	srv := feedServerUnified(t, rssBody(itemXML("lgsk", "Skipped", "http://x/s.torrent")))
	site := &legacyPTStub{enabled: true, discount: models.DISCOUNT_FREE, sizeMB: 1}
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL}))
	assert.Equal(t, 0, site.dlCalls, "recently-skipped non-free torrent must be skipped before detail")
}

func TestExtractTorrentRef_HostFromLink(t *testing.T) {
	host, ref := extractTorrentRef(&gofeed.Item{Link: "https://tracker.example.com/details.php?id=99"})
	assert.Equal(t, "tracker.example.com", host)
	_ = ref

	// Invalid URL → empty.
	h2, _ := extractTorrentRef(&gofeed.Item{Link: "://bad"})
	assert.Equal(t, "", h2)
}

func TestFetchAndDownloadFreeRSS_ContextCanceled(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	srv := feedServerUnified(t, rssBody(itemXML("lgc", "Cancel", "http://x/c.torrent")))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	site := &legacyPTStub{enabled: true, discount: models.DISCOUNT_FREE, sizeMB: 1}
	_ = FetchAndDownloadFreeRSS(ctx, models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL})
}

func TestFetchAndDownloadFreeRSS_ConfigStoreZeroValueDefault(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true, DefaultConcurrency: 2,
	}))
	srv := feedServerUnified(t, rssBody(
		itemXML("lgm1", "A", "http://x/a.torrent")+itemXML("lgm2", "B", "http://x/b.torrent"),
	))
	site := &legacyPTStub{enabled: true, discount: models.DISCOUNT_FREE, sizeMB: 1, writeFile: true}
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "t", Concurrency: 2}))
	assert.GreaterOrEqual(t, site.dlCalls, 1)
}
