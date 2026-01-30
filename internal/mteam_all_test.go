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
	m, err := NewUnifiedSiteImpl(context.Background(), models.MTEAM)
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
	_, err = core.NewConfigStore(db).UpsertSite(models.MTEAM, models.SiteConfig{Enabled: &enabled})
	require.NoError(t, err)
	m, err := NewUnifiedSiteImpl(context.Background(), models.MTEAM)
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

	m, err := NewUnifiedSiteImpl(context.Background(), models.MTEAM)
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
	m, err := NewUnifiedSiteImpl(ctx, models.MTEAM)
	require.NoError(t, err)
	require.NotNil(t, m)
	require.Equal(t, models.MTEAM, m.SiteGroup())
}

// TestUnifiedSiteImpl_MTEAM_MaxRetries 测试最大重试次数
func TestUnifiedSiteImpl_MTEAM_MaxRetries(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	m, err := NewUnifiedSiteImpl(context.Background(), models.MTEAM)
	require.NoError(t, err)
	require.Equal(t, maxRetries, m.MaxRetries())
}

// TestUnifiedSiteImpl_MTEAM_RetryDelay 测试重试延迟
func TestUnifiedSiteImpl_MTEAM_RetryDelay(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	m, err := NewUnifiedSiteImpl(context.Background(), models.MTEAM)
	require.NoError(t, err)
	require.Equal(t, retryDelay, m.RetryDelay())
}
