// MIT License
// Copyright (c) 2025 pt-tools

// Coverage for UnifiedSiteImpl.GetTorrentDetails (against a fake NexusPHP
// detail page), SendTorrentToDownloader's directory guards + push path,
// getDBInstance, and fetchNexusPHPDetail via GetTorrentDetailForTest.

package internal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	_ "github.com/sunerpy/pt-tools/site/v2/definitions"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func nexusDetailServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><body>
			<input name="torrent_name" value="My.Movie.2024">
			<input name="detail_torrent_id" value="42">
			<h1><font class="free">免费</font><span title="2026-01-20 15:30:00">2天</span></h1>
			<td class="rowhead">基本信息</td><td>大小：16.87 GB</td>
		</body></html>`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func setupUnifiedDB(t *testing.T) *models.TorrentDB {
	t.Helper()
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })
	return db
}

func TestGetDBInstance(t *testing.T) {
	global.GlobalDB = nil
	assert.Nil(t, getDBInstance())

	db := setupUnifiedDB(t)
	assert.NotNil(t, getDBInstance())
	assert.Same(t, db.DB, getDBInstance())
}

func TestUnifiedSiteImpl_GetTorrentDetails_Disabled(t *testing.T) {
	setupUnifiedDB(t)
	impl, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("springsunday"))
	require.NoError(t, err)
	_, err = impl.GetTorrentDetails(&gofeed.Item{GUID: "42"})
	require.Error(t, err)
	assert.Equal(t, enableError, err.Error())
}

func TestUnifiedSiteImpl_GetTorrentDetails_NilItem(t *testing.T) {
	db := setupUnifiedDB(t)
	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{
		Enabled: boolPtrUS(true), AuthMethod: "cookie", Cookie: "c=1",
	}))
	impl, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("springsunday"))
	require.NoError(t, err)
	_, err = impl.GetTorrentDetails(nil)
	require.Error(t, err)
}

func TestUnifiedSiteImpl_GetTorrentDetails_HappyPath(t *testing.T) {
	db := setupUnifiedDB(t)
	srv := nexusDetailServer(t)
	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{
		Enabled: boolPtrUS(true), AuthMethod: "cookie", Cookie: "c=1", APIUrl: srv.URL,
	}))
	impl, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("springsunday"))
	require.NoError(t, err)

	item, err := impl.GetTorrentDetails(&gofeed.Item{GUID: "42", Link: srv.URL + "/details.php?id=42", Title: "t"})
	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Equal(t, "42", item.ID)
	assert.Equal(t, "My.Movie.2024", item.Title)
	assert.True(t, item.IsFree())
}

func TestUnifiedSiteImpl_SendTorrentToDownloader_EmptyDirNoOp(t *testing.T) {
	db := setupUnifiedDB(t)
	dir := t.TempDir()
	require.NoError(t, core.NewConfigStore(db).SaveGlobal(models.SettingsGlobal{
		DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true,
	}))
	impl, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("springsunday"))
	require.NoError(t, err)
	// Empty staging dir -> the push is skipped and no error surfaces.
	require.NoError(t, impl.SendTorrentToDownloader(context.Background(), models.RSSConfig{Tag: "empty-tag"}))
}

func TestUnifiedSiteImpl_SendTorrentToDownloader_PushError(t *testing.T) {
	db := setupUnifiedDB(t)
	dir := t.TempDir()
	require.NoError(t, core.NewConfigStore(db).SaveGlobal(models.SettingsGlobal{
		DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true,
	}))

	tag := "cmct-push-err"
	sub := filepath.Join(dir, tag)
	require.NoError(t, os.MkdirAll(sub, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "a.torrent"), []byte("d4:infod4:name1:aee"), 0o644))

	impl, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("springsunday"))
	require.NoError(t, err)
	// No downloader configured -> GetDownloaderForRSSAndSiteWithInfo fails inside
	// ProcessTorrentsWithDownloaderByRSS, surfacing an error.
	err = impl.SendTorrentToDownloader(context.Background(), models.RSSConfig{Tag: tag, Category: "cat"})
	require.Error(t, err)
}

func boolPtrUS(b bool) *bool { return &b }
