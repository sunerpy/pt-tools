// MIT License
// Copyright (c) 2025 pt-tools

// Fills remaining internal gaps: ProcessTorrentsWithDownloaderByRSS success +
// disabled-downloader ("未启用") paths, SendTorrentToDownloader surfacing that
// error, and the unified download-worker's shouldSkipSiteDownload skip branch.

package internal

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeebo/bencode"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

func TestProcessTorrentsWithDownloaderByRSS_DisabledDownloader(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	dlID := uint(1)
	ds := models.DownloaderSetting{Name: "off", Type: "qbittorrent", URL: "http://x", Enabled: false}
	ds.ID = dlID
	require.NoError(t, db.DB.Create(&ds).Error)

	err := ProcessTorrentsWithDownloaderByRSS(context.Background(),
		models.RSSConfig{DownloaderID: &dlID}, t.TempDir(), "cat", "tag", models.SiteGroup("springsunday"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "未启用")
}

func TestProcessTorrentsWithDownloaderByRSS_SuccessAndSweep(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	srv := fakeQbitServer(t, false, 500*gb)
	dm := downloader.NewDownloaderManager()
	dm.RegisterFactory(downloader.DownloaderQBittorrent, func(cfg downloader.DownloaderConfig, name string) (downloader.Downloader, error) {
		return qbit.NewQbitClient(qbit.NewQBitConfigWithAutoStart(cfg.GetURL(), cfg.GetUsername(), cfg.GetPassword(), cfg.GetAutoStart()), name)
	})
	SetGlobalDownloaderManager(dm)
	t.Cleanup(func() { SetGlobalDownloaderManager(nil) })

	ds := models.DownloaderSetting{Name: "qb-def", Type: "qbittorrent", URL: srv.URL, Enabled: true, IsDefault: true, AutoStart: true}
	require.NoError(t, db.DB.Create(&ds).Error)

	dir := t.TempDir()
	var buf bytes.Buffer
	require.NoError(t, bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": "z"}}))
	fp := filepath.Join(dir, "springsunday-tsucc.torrent")
	require.NoError(t, os.WriteFile(fp, buf.Bytes(), 0o644))
	hash, err := qbit.ComputeTorrentHashWithPath(fp)
	require.NoError(t, err)

	future := time.Now().Add(1 * time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "tsucc", FreeEndTime: &future, IsPushed: &pushed}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	require.NoError(t, ProcessTorrentsWithDownloaderByRSS(context.Background(),
		models.RSSConfig{}, dir, "cat", "tag", models.SiteGroup("springsunday")))

	got, gerr := db.GetTorrentBySiteAndID("springsunday", "tsucc")
	require.NoError(t, gerr)
	require.NotNil(t, got.IsPushed)
	assert.True(t, *got.IsPushed)
}

func TestUnifiedSite_SendTorrentToDownloader_DisabledSurfacesError(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	require.NoError(t, core.NewConfigStore(db).SaveGlobal(models.SettingsGlobal{
		DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true,
	}))

	tag := "cmct-disabled"
	sub := filepath.Join(dir, tag)
	require.NoError(t, os.MkdirAll(sub, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "springsunday-x.torrent"), []byte("d4:infod4:name1:aee"), 0o644))

	dlID := uint(9)
	ds := models.DownloaderSetting{Name: "disabled", Type: "qbittorrent", URL: "http://x", Enabled: false}
	ds.ID = dlID
	require.NoError(t, db.DB.Create(&ds).Error)
	require.NoError(t, db.DB.Create(&models.SiteSetting{Name: "springsunday", DownloaderID: &dlID}).Error)

	impl, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("springsunday"))
	require.NoError(t, err)
	err = impl.SendTorrentToDownloader(context.Background(), models.RSSConfig{Tag: tag, Category: "cat"})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "未启用"))
}

func TestFetchUnified_SkipsAlreadyDownloadedFile(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	gl, err := core.NewConfigStore(db).GetGlobalOnly()
	require.NoError(t, err)
	base := gl.DownloadDir

	// Pre-create the target .torrent so shouldSkipSiteDownload returns true
	// (IsDownloaded && local file exists) — the worker must skip re-download.
	sub := filepath.Join(base, "movie")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	fileBase := "springsunday-g900"
	require.NoError(t, os.WriteFile(filepath.Join(sub, fileBase+".torrent"), []byte("d4:infod4:name1:aee"), 0o644))

	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "g900", IsDownloaded: true}
	require.NoError(t, db.UpsertTorrent(ti))

	srv := feedServerUnified(t, rssBody(itemXML("g900", "Dl", "http://x/d.torrent")))
	site := &unifiedFake{
		enabled: true,
		detail:  &v2.TorrentItem{ID: "g900", Title: "Dl", DiscountLevel: v2.DiscountFree, SizeBytes: 1024},
	}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "movie"}))

	assert.Equal(t, int32(0), site.downloadCalls.Load(), "existing downloaded file must skip re-download")
}
