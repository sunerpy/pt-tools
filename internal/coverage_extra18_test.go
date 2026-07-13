// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"testing"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestSendTorrentToDownloader_BlankDownloadDirError(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, global.GlobalDB.DB.Exec("DELETE FROM settings_globals").Error)
	require.NoError(t, db.DB.Create(&models.SettingsGlobal{DownloadDir: "", DefaultIntervalMinutes: 10}).Error)

	impl, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("springsunday"))
	require.NoError(t, err)
	err = impl.SendTorrentToDownloader(context.Background(), models.RSSConfig{Tag: "t"})
	require.Error(t, err)
}

func TestGetTorrentDetails_RateLimitCanceledCtx(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{
		Enabled: boolPtrIX(true), AuthMethod: "cookie", Cookie: "c=1",
	}))

	ctx, cancel := context.WithCancel(context.Background())
	impl, err := NewUnifiedSiteImpl(ctx, models.SiteGroup("springsunday"))
	require.NoError(t, err)
	cancel()
	_, derr := impl.GetTorrentDetails(&gofeed.Item{GUID: "1", Link: "http://x/details.php?id=1", Title: "t"})
	require.Error(t, derr)
}

func TestGetTorrentDetails_ConfigLoadErrorNilDB(t *testing.T) {
	db := setupDB(t)
	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{
		Enabled: boolPtrIX(true), AuthMethod: "cookie", Cookie: "c=1",
	}))
	impl, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("springsunday"))
	require.NoError(t, err)
	// Detach DB after building the impl so IsEnabled + Load happen against nil.
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = nil })
	_, derr := impl.GetTorrentDetails(&gofeed.Item{GUID: "1", Link: "http://x", Title: "t"})
	require.Error(t, derr)
}

var _ = assert.True
