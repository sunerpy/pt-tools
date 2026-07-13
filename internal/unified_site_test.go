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
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeebo/bencode"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	_ "github.com/sunerpy/pt-tools/site/v2/definitions"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

func TestUnifiedSiteImpl_CMCT_IsEnabledAndFields(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.InitLogger(zap.NewNop())
	global.GlobalDB = db
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	s := core.NewConfigStore(db)
	_ = s.SaveQbitSettings(models.QbitSettings{Enabled: true, URL: ts.URL, User: "u", Password: "p"})
	e := true
	_, _ = s.UpsertSite(models.SiteGroup("springsunday"), models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
	c, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("springsunday"))
	require.NoError(t, err)
	if !c.IsEnabled() {
		t.Fatalf("expected enabled")
	}
	if c.MaxRetries() != maxRetries {
		t.Fatalf("max retries mismatch")
	}
	if c.RetryDelay() != retryDelay {
		t.Fatalf("retry delay mismatch")
	}
	if c.SiteGroup() != models.SiteGroup("springsunday") {
		t.Fatalf("site group mismatch")
	}
}

func TestUnifiedSiteImpl_CMCT_DownloadTorrentAndContext(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.InitLogger(zap.NewNop())
	global.GlobalDB = db
	// serve minimal bencoded torrent
	var buf bytes.Buffer
	_ = bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": "x"}})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()
	dir := t.TempDir()
	c, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("springsunday"))
	require.NoError(t, err)
	if _, err := c.DownloadTorrent(srv.URL, "t", dir); err != nil {
		t.Fatalf("download: %v", err)
	}
	if c.Context() == nil {
		t.Fatalf("nil context")
	}
}

func TestUnifiedSiteImpl_CMCT_SendTorrentToDownloader(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	abs := t.TempDir()
	require.NoError(t, core.NewConfigStore(db).SaveGlobal(models.SettingsGlobal{DownloadDir: abs, DefaultIntervalMinutes: 10, DefaultEnabled: true}))
	tag := "CMCT"
	sub := filepath.Join(abs, tag)
	require.NoError(t, os.MkdirAll(sub, 0o755))
	var buf bytes.Buffer
	_ = bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": "x"}})
	data := buf.Bytes()
	p := filepath.Join(sub, "a.torrent")
	require.NoError(t, os.WriteFile(p, data, 0o644))
	h, err := qbit.ComputeTorrentHashWithPath(p)
	require.NoError(t, err)
	pushed := false
	ti := &models.TorrentInfo{SiteName: string(models.SiteGroup("springsunday")), TorrentHash: &h, IsPushed: &pushed}
	require.NoError(t, db.UpsertTorrent(ti))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/sync/maindata":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":9999999999},"torrents":{}}`))
		case "/api/v2/torrents/properties":
			w.WriteHeader(http.StatusNotFound)
		case "/api/v2/torrents/add":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()
	// Create a default downloader setting in the database
	dlSetting := models.DownloaderSetting{
		Name:      "test-qbit",
		Type:      "qbittorrent",
		URL:       srv.URL,
		Username:  "u",
		Password:  "p",
		Enabled:   true,
		IsDefault: true,
	}
	require.NoError(t, db.DB.Create(&dlSetting).Error)

	dlMgr := downloader.NewDownloaderManager()
	dlMgr.RegisterFactory(downloader.DownloaderQBittorrent, func(config downloader.DownloaderConfig, name string) (downloader.Downloader, error) {
		qbitConfig := qbit.NewQBitConfigWithAutoStart(config.GetURL(), config.GetUsername(), config.GetPassword(), config.GetAutoStart())
		return qbit.NewQbitClient(qbitConfig, name)
	})
	SetGlobalDownloaderManager(dlMgr)
	defer SetGlobalDownloaderManager(nil)

	c, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("springsunday"))
	require.NoError(t, err)
	require.NoError(t, c.SendTorrentToDownloader(context.Background(), models.RSSConfig{Tag: tag, Category: "cat"}))
	if _, err := os.Stat(p); err == nil {
		t.Fatalf("expected file removed")
	}
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

func TestIsEnabled_Branches(t *testing.T) {
	global.GlobalDB = nil
	impl := &UnifiedSiteImpl{siteGroup: models.SiteGroup("springsunday")}
	assert.False(t, impl.IsEnabled())

	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	// Site not present in config → false.
	impl2, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("springsunday"))
	require.NoError(t, err)
	assert.False(t, impl2.IsEnabled())

	// Enabled=true → true.
	e := true
	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{
		Enabled: &e, AuthMethod: "cookie", Cookie: "c=1",
	}))
	impl3, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("springsunday"))
	require.NoError(t, err)
	assert.True(t, impl3.IsEnabled())
}

func TestSendTorrentToDownloader_MissingDirNoOp(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	require.NoError(t, core.NewConfigStore(db).SaveGlobal(models.SettingsGlobal{
		DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true,
	}))
	impl, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("springsunday"))
	require.NoError(t, err)
	// Tag path that does not exist yet gets MkdirAll'd then reported empty → no-op.
	require.NoError(t, impl.SendTorrentToDownloader(context.Background(), models.RSSConfig{Tag: "brand-new-tag"}))
}

func TestGetTorrentDetails_Disabled(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	_ = db
	impl, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("springsunday"))
	require.NoError(t, err)
	_, err = impl.GetTorrentDetails(&gofeed.Item{GUID: "1"})
	require.Error(t, err)
}

func TestUnifiedSiteImpl_HDSKY_IsEnabledAndFields(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.InitLogger(zap.NewNop())
	global.GlobalDB = db
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	s := core.NewConfigStore(db)
	_ = s.SaveQbitSettings(models.QbitSettings{Enabled: true, URL: ts.URL, User: "u", Password: "p"})
	e := true
	_, _ = s.UpsertSite(models.SiteGroup("hdsky"), models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
	h, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("hdsky"))
	require.NoError(t, err)
	// basic getters
	if !h.IsEnabled() {
		t.Fatalf("expected enabled")
	}
	if h.MaxRetries() != maxRetries {
		t.Fatalf("max retries mismatch")
	}
	if h.RetryDelay() != retryDelay {
		t.Fatalf("retry delay mismatch")
	}
	if h.SiteGroup() != models.SiteGroup("hdsky") {
		t.Fatalf("site group mismatch")
	}
}

func TestUnifiedSiteImpl_HDSKY_DownloadTorrent(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.InitLogger(zap.NewNop())
	global.GlobalDB = db
	// serve minimal bencoded torrent
	var buf bytes.Buffer
	_ = bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": "x"}})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()
	dir := t.TempDir()
	h, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("hdsky"))
	require.NoError(t, err)
	if _, err := h.DownloadTorrent(srv.URL, "t", dir); err != nil {
		t.Fatalf("download: %v", err)
	}
}

func TestUnifiedSiteImpl_HDSKY_ContextGetter(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.InitLogger(zap.NewNop())
	global.GlobalDB = db
	ctx := context.Background()
	h, err := NewUnifiedSiteImpl(ctx, models.SiteGroup("hdsky"))
	require.NoError(t, err)
	if h.Context() == nil {
		t.Fatalf("nil context")
	}
}

func TestUnifiedSiteImpl_HDSKY_SendTorrentToDownloader(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.InitLogger(zap.NewNop())
	global.GlobalDB = db
	abs := t.TempDir()
	_ = core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: abs, DefaultIntervalMinutes: 10, DefaultEnabled: true})
	tag := "HDSKY"
	sub := filepath.Join(abs, tag)
	_ = os.MkdirAll(sub, 0o755)
	var buf bytes.Buffer
	_ = bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": "x"}})
	data := buf.Bytes()
	p := filepath.Join(sub, "a.torrent")
	_ = os.WriteFile(p, data, 0o644)
	h, _ := qbit.ComputeTorrentHashWithPath(p)
	pushed := false
	ti := &models.TorrentInfo{SiteName: string(models.SiteGroup("hdsky")), TorrentHash: &h, IsPushed: &pushed}
	_ = db.UpsertTorrent(ti)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/sync/maindata":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":9999999999},"torrents":{}}`))
		case "/api/v2/torrents/properties":
			w.WriteHeader(http.StatusNotFound)
		case "/api/v2/torrents/add":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()
	// Create a default downloader setting in the database
	dlSetting := models.DownloaderSetting{
		Name:      "test-qbit",
		Type:      "qbittorrent",
		URL:       srv.URL,
		Username:  "u",
		Password:  "p",
		Enabled:   true,
		IsDefault: true,
	}
	require.NoError(t, db.DB.Create(&dlSetting).Error)

	dlMgr := downloader.NewDownloaderManager()
	dlMgr.RegisterFactory(downloader.DownloaderQBittorrent, func(config downloader.DownloaderConfig, name string) (downloader.Downloader, error) {
		qbitConfig := qbit.NewQBitConfigWithAutoStart(config.GetURL(), config.GetUsername(), config.GetPassword(), config.GetAutoStart())
		return qbit.NewQbitClient(qbitConfig, name)
	})
	SetGlobalDownloaderManager(dlMgr)
	defer SetGlobalDownloaderManager(nil)

	hds, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("hdsky"))
	require.NoError(t, err)
	require.NoError(t, hds.SendTorrentToDownloader(context.Background(), models.RSSConfig{Tag: tag, Category: "cat"}))
}

func TestUnifiedSiteImpl_MTEAM_DownloadTorrentAndContext(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	// server returns minimal bencoded torrent
	var buf bytes.Buffer
	_ = bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": "x"}})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()
	dir := t.TempDir()
	m, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("mteam"))
	require.NoError(t, err)
	if _, err := m.DownloadTorrent(srv.URL, "t", dir); err != nil {
		t.Fatalf("download: %v", err)
	}
	if m.Context() == nil {
		t.Fatalf("nil context")
	}
}

func TestUnifiedSiteImpl_MTEAM_IsEnabled(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	enabled := true
	_, err = core.NewConfigStore(db).UpsertSite(models.SiteGroup("mteam"), models.SiteConfig{Enabled: &enabled})
	require.NoError(t, err)
	m, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("mteam"))
	require.NoError(t, err)
	require.True(t, m.IsEnabled())
}

func TestUnifiedSiteImpl_MTEAM_SendTorrentToDownloader_WithStub(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	abs := t.TempDir()
	require.NoError(t, core.NewConfigStore(db).SaveGlobal(models.SettingsGlobal{DownloadDir: abs, DefaultIntervalMinutes: 10, DefaultEnabled: true}))
	tag := "MT"
	sub := filepath.Join(abs, tag)
	require.NoError(t, os.MkdirAll(sub, 0o755))
	var buf bytes.Buffer
	_ = bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": "x"}})
	data := buf.Bytes()
	p := filepath.Join(sub, "a.torrent")
	require.NoError(t, os.WriteFile(p, data, 0o644))
	h, err := qbit.ComputeTorrentHashWithPath(p)
	require.NoError(t, err)
	pushed := false
	ti := &models.TorrentInfo{SiteName: string(models.SiteGroup("mteam")), TorrentHash: &h, IsPushed: &pushed}
	require.NoError(t, db.UpsertTorrent(ti))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/sync/maindata":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":9999999999},"torrents":{}}`))
		case "/api/v2/torrents/properties":
			w.WriteHeader(http.StatusNotFound)
		case "/api/v2/torrents/add":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()
	// Create a default downloader setting in the database
	dlSetting := models.DownloaderSetting{
		Name:      "test-qbit",
		Type:      "qbittorrent",
		URL:       srv.URL,
		Username:  "u",
		Password:  "p",
		Enabled:   true,
		IsDefault: true,
	}
	require.NoError(t, db.DB.Create(&dlSetting).Error)

	dlMgr := downloader.NewDownloaderManager()
	dlMgr.RegisterFactory(downloader.DownloaderQBittorrent, func(config downloader.DownloaderConfig, name string) (downloader.Downloader, error) {
		qbitConfig := qbit.NewQBitConfigWithAutoStart(config.GetURL(), config.GetUsername(), config.GetPassword(), config.GetAutoStart())
		return qbit.NewQbitClient(qbitConfig, name)
	})
	SetGlobalDownloaderManager(dlMgr)
	defer SetGlobalDownloaderManager(nil)

	m, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("mteam"))
	require.NoError(t, err)
	require.NoError(t, m.SendTorrentToDownloader(context.Background(), models.RSSConfig{Tag: tag, Category: "cat"}))
}

// TestNewUnifiedSiteImpl_MTEAM 测试创建 UnifiedSiteImpl for MTEAM
func TestNewUnifiedSiteImpl_MTEAM(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	ctx := context.Background()
	m, err := NewUnifiedSiteImpl(ctx, models.SiteGroup("mteam"))
	require.NoError(t, err)
	require.NotNil(t, m)
	require.Equal(t, models.SiteGroup("mteam"), m.SiteGroup())
}

// TestUnifiedSiteImpl_MTEAM_MaxRetries 测试最大重试次数
func TestUnifiedSiteImpl_MTEAM_MaxRetries(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	m, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("mteam"))
	require.NoError(t, err)
	require.Equal(t, maxRetries, m.MaxRetries())
}

// TestUnifiedSiteImpl_MTEAM_RetryDelay 测试重试延迟
func TestUnifiedSiteImpl_MTEAM_RetryDelay(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	m, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("mteam"))
	require.NoError(t, err)
	require.Equal(t, retryDelay, m.RetryDelay())
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

func TestNewUnifiedSiteImpl(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	tests := []struct {
		name      string
		siteGroup models.SiteGroup
		wantErr   bool
	}{
		{
			name:      "MTEAM site",
			siteGroup: models.SiteGroup("mteam"),
			wantErr:   false,
		},
		{
			name:      "HDSKY site",
			siteGroup: models.SiteGroup("hdsky"),
			wantErr:   false,
		},
		{
			name:      "CMCT site",
			siteGroup: models.SiteGroup("springsunday"),
			wantErr:   false,
		},
		{
			name:      "Unknown site",
			siteGroup: models.SiteGroup("unknown"),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			impl, err := NewUnifiedSiteImpl(context.Background(), tt.siteGroup)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewUnifiedSiteImpl() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && impl == nil {
				t.Error("NewUnifiedSiteImpl() returned nil without error")
			}
			if !tt.wantErr {
				if impl.SiteGroup() != tt.siteGroup {
					t.Errorf("SiteGroup() = %v, want %v", impl.SiteGroup(), tt.siteGroup)
				}
				if impl.Context() == nil {
					t.Error("Context() returned nil")
				}
				if impl.MaxRetries() != maxRetries {
					t.Errorf("MaxRetries() = %v, want %v", impl.MaxRetries(), maxRetries)
				}
				if impl.RetryDelay() != retryDelay {
					t.Errorf("RetryDelay() = %v, want %v", impl.RetryDelay(), retryDelay)
				}
			}
		})
	}
}

func TestUnifiedSiteImpl_IsEnabled(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// 创建站点配置
	enabled := true
	disabled := false

	// 插入启用的站点
	global.GlobalDB.DB.Create(&models.SiteSetting{
		Name:       string(models.SiteGroup("mteam")),
		Enabled:    true,
		AuthMethod: "api_key",
		APIKey:     "test-key",
		APIUrl:     "https://api.m-team.cc",
	})

	// 插入禁用的站点
	global.GlobalDB.DB.Create(&models.SiteSetting{
		Name:       string(models.SiteGroup("hdsky")),
		Enabled:    false,
		AuthMethod: "cookie",
		Cookie:     "test-cookie",
	})

	tests := []struct {
		name      string
		siteGroup models.SiteGroup
		want      bool
	}{
		{
			name:      "Enabled site",
			siteGroup: models.SiteGroup("mteam"),
			want:      true,
		},
		{
			name:      "Disabled site",
			siteGroup: models.SiteGroup("hdsky"),
			want:      false,
		},
		{
			name:      "Non-existent site",
			siteGroup: models.SiteGroup("springsunday"),
			want:      false,
		},
	}

	_ = enabled
	_ = disabled

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			impl, err := NewUnifiedSiteImpl(context.Background(), tt.siteGroup)
			if err != nil {
				t.Fatalf("NewUnifiedSiteImpl() error = %v", err)
			}
			if got := impl.IsEnabled(); got != tt.want {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnifiedSiteImpl_RateLimiter(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	impl, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("hdsky"))
	require.NoError(t, err)
	require.NotNil(t, impl)
	require.NotNil(t, impl.limiter, "limiter should be initialized")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	for i := 0; i < 3; i++ {
		err := impl.waitForRateLimit(ctx)
		require.NoError(t, err)
	}
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Logf("Rate limit applied, elapsed: %v (burst allowed initial requests)", elapsed)
	}
}

func TestUnifiedSiteImpl_RateLimiter_ContextCanceled(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	impl, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("hdsky"))
	require.NoError(t, err)

	for i := 0; i < 200; i++ {
		_ = impl.limiter.Allow()
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = impl.waitForRateLimit(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}
