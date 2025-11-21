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
	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/site"
	"github.com/sunerpy/pt-tools/thirdpart/qbit"
	"github.com/zeebo/bencode"
	"go.uber.org/zap"
)

func TestHdskyImpl_IsEnabledAndFields(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	if err != nil {
		t.Fatalf("db: %v", err)
	}
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
	h := NewHdskyImpl(context.Background())
	// basic getters
	if !h.IsEnabled() {
		t.Fatalf("expected enabled")
	}
	if h.MaxRetries() != h.maxRetries {
		t.Fatalf("max retries mismatch")
	}
	if h.RetryDelay() != h.retryDelay {
		t.Fatalf("retry delay mismatch")
	}
}

func TestHdskyImpl_DownloadTorrent(t *testing.T) {
	// serve minimal bencoded torrent
	var buf bytes.Buffer
	_ = bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": "x"}})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()
	dir := t.TempDir()
	h := &HdskyImpl{maxRetries: 1, retryDelay: 0}
	if _, err := h.DownloadTorrent(srv.URL, "t", dir); err != nil {
		t.Fatalf("download: %v", err)
	}
}

func TestHdskyImpl_ContextGetter(t *testing.T) {
	ctx := context.Background()
	h := &HdskyImpl{ctx: ctx}
	if h.Context() != ctx {
		t.Fatalf("context mismatch")
	}
}

func TestHdsky_SendTorrentToQbit(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	if err != nil {
		t.Fatalf("db: %v", err)
	}
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
	client, _ := qbit.NewQbitClient(srv.URL, "u", "p", time.Millisecond*10)
	hds := &HdskyImpl{qbitClient: client}
	if err := hds.SendTorrentToQbit(context.Background(), models.RSSConfig{Tag: tag, Category: "cat"}); err != nil {
		t.Fatalf("send: %v", err)
	}
}

func TestHdskyImpl_GetTorrentDetails_HTTPError(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	global.InitLogger(zap.NewNop())
	global.GlobalDB = db
	e := true
	_, _ = core.NewConfigStore(db).UpsertSite(models.HDSKY, models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
	// server returns 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer srv.Close()
	h := &HdskyImpl{Collector: site.NewCollectorWithTransport()}
	item := &gofeed.Item{Link: srv.URL}
	out, err := h.GetTorrentDetails(item)
	if err == nil || out != nil {
		t.Fatalf("expected error on http 500")
	}
}
