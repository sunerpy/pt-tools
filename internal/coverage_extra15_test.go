// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeebo/bencode"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

func badJSONServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{not-json`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSendTorrentToDownloader_SuccessPath(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	require.NoError(t, core.NewConfigStore(db).SaveGlobal(models.SettingsGlobal{
		DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true,
	}))

	srv := fakeQbitServer(t, false, 500*gb)
	dm := downloader.NewDownloaderManager()
	dm.RegisterFactory(downloader.DownloaderQBittorrent, func(cfg downloader.DownloaderConfig, name string) (downloader.Downloader, error) {
		return qbit.NewQbitClient(qbit.NewQBitConfigWithAutoStart(cfg.GetURL(), cfg.GetUsername(), cfg.GetPassword(), cfg.GetAutoStart()), name)
	})
	SetGlobalDownloaderManager(dm)
	t.Cleanup(func() { SetGlobalDownloaderManager(nil) })
	require.NoError(t, db.DB.Create(&models.DownloaderSetting{
		Name: "qb-def", Type: "qbittorrent", URL: srv.URL, Enabled: true, IsDefault: true, AutoStart: true,
	}).Error)

	tag := "movie"
	sub := filepath.Join(dir, tag)
	require.NoError(t, os.MkdirAll(sub, 0o755))
	var buf bytes.Buffer
	require.NoError(t, bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": "z"}}))
	fp := filepath.Join(sub, "springsunday-ok.torrent")
	require.NoError(t, os.WriteFile(fp, buf.Bytes(), 0o644))
	hash, err := qbit.ComputeTorrentHashWithPath(fp)
	require.NoError(t, err)

	future := time.Now().Add(time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "ok", FreeEndTime: &future, IsPushed: &pushed}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	impl, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("springsunday"))
	require.NoError(t, err)
	require.NoError(t, impl.SendTorrentToDownloader(context.Background(), models.RSSConfig{Tag: tag, Category: "cat"}))

	got, gerr := db.GetTorrentBySiteAndID("springsunday", "ok")
	require.NoError(t, gerr)
	require.NotNil(t, got.IsPushed)
	assert.True(t, *got.IsPushed)
}

func TestGetTorrentDetails_ProviderNotSupported(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("mteam"), models.SiteConfig{
		Enabled: boolPtrIX(true), AuthMethod: "api_key", APIKey: "k", APIUrl: "http://127.0.0.1:0",
	}))
	impl, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("mteam"))
	require.NoError(t, err)
	require.NotPanics(t, func() {
		_, _ = impl.GetTorrentDetails(&gofeed.Item{GUID: "1", Link: "http://x/details.php?id=1", Title: "t"})
	})
}

func TestFetchMTorrentDetail_DecodeError(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	srv := badJSONServer(t)
	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("mteam"), models.SiteConfig{
		Enabled: boolPtrIX(true), AuthMethod: "api_key", APIKey: "k", APIUrl: srv.URL,
	}))
	_, err := fetchMTorrentDetail(context.Background(), models.SiteGroup("mteam"), &gofeed.Item{GUID: "1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "解析 JSON")
}
