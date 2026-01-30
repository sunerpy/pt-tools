package internal

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zeebo/bencode"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
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
	_, _ = s.UpsertSite(models.SpringSunday, models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
	c, err := NewUnifiedSiteImpl(context.Background(), models.SpringSunday)
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
	if c.SiteGroup() != models.SpringSunday {
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
	c, err := NewUnifiedSiteImpl(context.Background(), models.SpringSunday)
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
	ti := &models.TorrentInfo{SiteName: string(models.SpringSunday), TorrentHash: &h, IsPushed: &pushed}
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

	c, err := NewUnifiedSiteImpl(context.Background(), models.SpringSunday)
	require.NoError(t, err)
	require.NoError(t, c.SendTorrentToDownloader(context.Background(), models.RSSConfig{Tag: tag, Category: "cat"}))
	if _, err := os.Stat(p); err == nil {
		t.Fatalf("expected file removed")
	}
}
