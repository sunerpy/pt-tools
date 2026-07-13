// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestFetchAndDownloadFreeRSS_NonFreeSkipsAndRecords(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	srv := feedServerUnified(t, rssBody(itemXMLWithCategory("lgc1", "Paid", "http://x/p.torrent", "TV")))
	site := &legacyPTStub{enabled: true, discount: models.DISCOUNT_NONE, sizeMB: 1}
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "t"}))

	ti, err := db.GetTorrentBySiteAndID("springsunday", "lgc1")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.True(t, ti.IsSkipped)
	assert.Equal(t, "TV", ti.Category)
}

func TestSendTorrentToDownloader_DisabledDownloaderSurfacesError(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	require.NoError(t, global.GlobalDB.DB.Exec("DELETE FROM settings_globals").Error)
	require.NoError(t, db.DB.Create(&models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 10}).Error)

	dlID := uint(0)
	ds := models.DownloaderSetting{Name: "off", Type: "qbittorrent", URL: "http://x", Enabled: false}
	require.NoError(t, db.DB.Create(&ds).Error)
	dlID = ds.ID
	require.NoError(t, db.DB.Create(&models.SiteSetting{Name: "springsunday", DownloaderID: &dlID}).Error)

	sub := filepath.Join(dir, "dtag")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "springsunday-x.torrent"), []byte("d4:infod4:name1:aee"), 0o644))

	impl, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("springsunday"))
	require.NoError(t, err)
	err = impl.SendTorrentToDownloader(context.Background(), models.RSSConfig{Tag: "dtag", DownloaderID: &dlID})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "未启用")
}
