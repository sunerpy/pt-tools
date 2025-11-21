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
	"github.com/sunerpy/pt-tools/thirdpart/qbit"
	"github.com/zeebo/bencode"
	"go.uber.org/zap"
)

func TestMteam_CanbeFinished_LimitBranches(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	require.NoError(t, core.NewConfigStore(db).SaveGlobal(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true, DownloadLimitEnabled: true, DownloadSpeedLimit: 1, TorrentSizeGB: 200}))
	m := &MteamImpl{}
	end := time.Now().Format("2006-01-02 15:04:05")
	d1 := models.MTTorrentDetail{Status: &models.Status{DiscountEndTime: end}, Size: "1099511627776"}
	require.False(t, m.CanbeFinished(d1))
	end2 := time.Now().Add(2 * time.Hour).Format("2006-01-02 15:04:05")
	d2 := models.MTTorrentDetail{Status: &models.Status{DiscountEndTime: end2}, Size: "104857600"}
	require.True(t, m.CanbeFinished(d2))
}

func TestMteam_CanbeFinished_InvalidInputs(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	require.NoError(t, core.NewConfigStore(db).SaveGlobal(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true, DownloadLimitEnabled: true, DownloadSpeedLimit: 20, TorrentSizeGB: 200, AutoStart: false}))
	m := &MteamImpl{}
	d := models.MTTorrentDetail{Status: &models.Status{DiscountEndTime: "bad-time"}, Size: "1000"}
	require.False(t, m.CanbeFinished(d))
	d2 := models.MTTorrentDetail{Status: &models.Status{DiscountEndTime: "2006-01-02 15:04:05"}, Size: "not-int"}
	require.False(t, m.CanbeFinished(d2))
}

func TestMteam_IsFree_Delegates(t *testing.T) {
	m := &MteamImpl{}
	d := models.MTTorrentDetail{Status: &models.Status{Discount: "free"}}
	require.True(t, m.IsFree(d))
	d2 := models.MTTorrentDetail{Status: &models.Status{Discount: "none"}}
	require.False(t, m.IsFree(d2))
}

func TestMteam_DownloadTorrentAndContext(t *testing.T) {
	// server returns minimal bencoded torrent
	var buf bytes.Buffer
	_ = bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": "x"}})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()
	dir := t.TempDir()
	m := &MteamImpl{ctx: context.Background(), maxRetries: 1, retryDelay: 0}
	if _, err := m.DownloadTorrent(srv.URL, "t", dir); err != nil {
		t.Fatalf("download: %v", err)
	}
	if m.Context() == nil {
		t.Fatalf("nil context")
	}
}

func TestMteam_IsEnabled(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	enabled := true
	_, err = core.NewConfigStore(db).UpsertSite(models.MTEAM, models.SiteConfig{Enabled: &enabled})
	require.NoError(t, err)
	m := &MteamImpl{}
	require.True(t, m.IsEnabled())
}

func TestMteam_GetTorrentDetails_InvalidConf(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	enabled := true
	_, err = core.NewConfigStore(db).UpsertSite(models.MTEAM, models.SiteConfig{Enabled: &enabled, AuthMethod: "api_key", APIUrl: models.DefaultAPIUrlMTeam, APIKey: ""})
	require.NoError(t, err)
	m := &MteamImpl{}
	item := &gofeed.Item{GUID: "gid", Title: "T"}
	_, err = m.GetTorrentDetails(item)
	require.Error(t, err)
}

func TestMteam_GetTorrentDetails_HTTP500(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	enabled := true
	// point APIUrl to server that returns 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer srv.Close()
	_, err = core.NewConfigStore(db).UpsertSite(models.MTEAM, models.SiteConfig{Enabled: &enabled, AuthMethod: "api_key", APIUrl: srv.URL, APIKey: "k"})
	require.NoError(t, err)
	m := &MteamImpl{}
	item := &gofeed.Item{GUID: "gid", Title: "T"}
	out, err := m.GetTorrentDetails(item)
	require.NotNil(t, out)
	require.NoError(t, err)
}

func TestMteam_SendTorrentToQbit_WithStub(t *testing.T) {
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
	_ = bencode.NewEncoder(&buf).Encode(map[string]interface{}{"info": map[string]interface{}{"name": "x"}})
	data := buf.Bytes()
	p := filepath.Join(sub, "a.torrent")
	require.NoError(t, os.WriteFile(p, data, 0o644))
	h, err := qbit.ComputeTorrentHashWithPath(p)
	require.NoError(t, err)
	pushed := false
	ti := &models.TorrentInfo{SiteName: string(models.MTEAM), TorrentHash: &h, IsPushed: &pushed}
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
	m := &MteamImpl{qbitClient: client}
	require.NoError(t, m.SendTorrentToQbit(context.Background(), models.RSSConfig{Tag: tag, Category: "cat"}))
}

func TestMteam_GetTorrentDetails_HTTPError(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	enabled := true
	_, err = core.NewConfigStore(db).UpsertSite(models.MTEAM, models.SiteConfig{Enabled: &enabled, AuthMethod: "api_key", APIUrl: "http://example", APIKey: "k"})
	require.NoError(t, err)
	// fake server returns 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer srv.Close()
	_, _ = core.NewConfigStore(db).UpsertSite(models.MTEAM, models.SiteConfig{Enabled: &enabled, AuthMethod: "api_key", APIUrl: srv.URL, APIKey: "k"})
	m := &MteamImpl{}
	item := &gofeed.Item{GUID: "gid", Title: "T"}
	out, err := m.GetTorrentDetails(item)
	require.NotNil(t, out)
	require.NoError(t, err)
}
