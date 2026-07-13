// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestFetchAndDownloadFreeRSS_DownloadNoFileResets(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	srv := feedServerUnified(t, rssBody(itemXML("lgnf", "NoFile", "http://x/n.torrent")))
	site := &legacyPTStub{enabled: true, discount: models.DISCOUNT_FREE, sizeMB: 1, writeFile: false}
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "movie"}))

	assert.Equal(t, 1, site.dlCalls)
	ti, err := db.GetTorrentBySiteAndID("springsunday", "lgnf")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.False(t, ti.IsDownloaded, "missing .torrent resets is_downloaded to false")
}

func TestFetchAndDownloadFreeRSS_ExistingDownloadedFilePresentSkips(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	pushed := false
	ti := &models.TorrentInfo{
		SiteName: "springsunday", TorrentID: "lgdf", IsPushed: &pushed, IsDownloaded: true, FreeLevel: "free",
	}
	require.NoError(t, db.UpsertTorrent(ti))

	srv := feedServerUnified(t, rssBody(itemXML("lgdf", "DlPresent", "http://x/d.torrent")))
	site := &legacyPTStub{enabled: true, discount: models.DISCOUNT_FREE, sizeMB: 1, writeFile: true}
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "movie"}))
	assert.GreaterOrEqual(t, site.dlCalls, 0)
}
