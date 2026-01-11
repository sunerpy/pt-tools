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
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

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
	_, _ = s.UpsertSite(models.HDSKY, models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
	h, err := NewUnifiedSiteImpl(context.Background(), models.HDSKY)
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
	if h.SiteGroup() != models.HDSKY {
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
	h, err := NewUnifiedSiteImpl(context.Background(), models.HDSKY)
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
	h, err := NewUnifiedSiteImpl(ctx, models.HDSKY)
	require.NoError(t, err)
	if h.Context() == nil {
		t.Fatalf("nil context")
	}
}

func TestUnifiedSiteImpl_HDSKY_SendTorrentToQbit(t *testing.T) {
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
	ti := &models.TorrentInfo{SiteName: string(models.HDSKY), TorrentHash: &h, IsPushed: &pushed}
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
	hds, err := NewUnifiedSiteImpl(context.Background(), models.HDSKY)
	require.NoError(t, err)
	require.NoError(t, hds.SendTorrentToQbit(context.Background(), models.RSSConfig{Tag: tag, Category: "cat"}))
}
