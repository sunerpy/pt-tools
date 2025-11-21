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
	"github.com/stretchr/testify/require"
	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/site"
	"github.com/sunerpy/pt-tools/thirdpart/qbit"
	"github.com/zeebo/bencode"
	"go.uber.org/zap"
)

func TestCmctImpl_IsEnabledAndFields(t *testing.T) {
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
	_, _ = s.UpsertSite(models.CMCT, models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
	c := NewCmctImpl(context.Background())
	if !c.IsEnabled() {
		t.Fatalf("expected enabled")
	}
	if c.MaxRetries() != c.maxRetries {
		t.Fatalf("max retries mismatch")
	}
	if c.RetryDelay() != c.retryDelay {
		t.Fatalf("retry delay mismatch")
	}
}

func TestCmctImpl_DownloadTorrentAndContext(t *testing.T) {
	// serve minimal bencoded torrent
	var buf bytes.Buffer
	_ = bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": "x"}})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()
	dir := t.TempDir()
	c := &CmctImpl{ctx: context.Background(), maxRetries: 1, retryDelay: 0}
	if _, err := c.DownloadTorrent(srv.URL, "t", dir); err != nil {
		t.Fatalf("download: %v", err)
	}
	if c.Context() == nil {
		t.Fatalf("nil context")
	}
}

func TestCmctImpl_GetTorrentDetails(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.InitLogger(zap.NewNop())
	global.GlobalDB = db
	s := core.NewConfigStore(db)
	e := true
	_, _ = s.UpsertSite(models.CMCT, models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><body>
            <input name='torrent_name' value='T1'>
            <input name='detail_torrent_id' value='ID1'>
            <h1><font class='free'></font><span title='2025-01-01 00:00:00'></span></h1>
        </body></html>`))
	}))
	defer srv.Close()
	c := &CmctImpl{Collector: site.NewCollectorWithTransport()}
	item := &gofeed.Item{Link: srv.URL}
	out, err := c.GetTorrentDetails(item)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, "success", out.Code)
}

func TestCmct_SendTorrentToQbit(t *testing.T) {
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
	_ = bencode.NewEncoder(&buf).Encode(map[string]interface{}{"info": map[string]interface{}{"name": "x"}})
	data := buf.Bytes()
	p := filepath.Join(sub, "a.torrent")
	require.NoError(t, os.WriteFile(p, data, 0o644))
	h, err := qbit.ComputeTorrentHashWithPath(p)
	require.NoError(t, err)
	pushed := false
	ti := &models.TorrentInfo{SiteName: string(models.CMCT), TorrentHash: &h, IsPushed: &pushed}
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
	client, err := qbit.NewQbitClient(srv.URL, "u", "p", time.Millisecond*10)
	require.NoError(t, err)
	c := &CmctImpl{qbitClient: client}
	require.NoError(t, c.SendTorrentToQbit(context.Background(), models.RSSConfig{Tag: tag, Category: "cat"}))
	if _, err := os.Stat(p); err == nil {
		t.Fatalf("expected file removed")
	}
}

func TestCmct_CanbeFinished_Branches(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	_ = core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DownloadLimitEnabled: false})
	c := &CmctImpl{}
	d := models.PHPTorrentInfo{EndTime: time.Now().Add(1 * time.Hour), SizeMB: 1024}
	if !c.CanbeFinished(d) {
		t.Fatalf("should finish when disabled")
	}
	_ = core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DownloadLimitEnabled: true, DownloadSpeedLimit: 0})
	if c.CanbeFinished(models.PHPTorrentInfo{EndTime: time.Now().Add(1 * time.Hour), SizeMB: 1024}) {
		t.Fatalf("expected false")
	}
}
