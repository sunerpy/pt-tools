package scheduler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"go.uber.org/zap"
)

func TestManager_StartStopAll(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := core.NewConfigStore(db)
	m := NewManager()
	cfg, err := s.Load()
	if err == nil && cfg != nil {
		m.StartAll(cfg)
		m.StopAll()
	}
}

func TestManager_Reload_EarlyReturns(t *testing.T) {
	global.InitLogger(zap.NewNop())
	m := &Manager{}
	// empty download dir -> early return
	cfg1 := &models.Config{Global: models.SettingsGlobal{DownloadDir: "", AutoStart: true}}
	m.Reload(cfg1)
	assert.Equal(t, int64(0), m.LastVersion())
	// autostart false -> early return
	cfg2 := &models.Config{Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: false}}
	m.Reload(cfg2)
}

func TestValidRSS(t *testing.T) {
	assert.False(t, validRSS(""))
	assert.False(t, validRSS("ftp://example.com"))
	assert.False(t, validRSS("http:///path"))
	assert.False(t, validRSS("https://rss.m-team.xxx/path"))
	assert.True(t, validRSS("https://example.com/path"))
	assert.True(t, validRSS("http://example.com/path"))
}

func TestManager_StartAll_NoSites(t *testing.T) {
	m := NewManager()
	cfg := &models.Config{Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true}, Sites: map[models.SiteGroup]models.SiteConfig{}}
	m.StartAll(cfg)
}

func TestReload_AutoStartFalseEarlyReturn(t *testing.T) {
	m := NewManager()
	dir := t.TempDir()
	cfg := &models.Config{Global: models.SettingsGlobal{DownloadDir: dir, AutoStart: false}, Sites: map[models.SiteGroup]models.SiteConfig{}}
	global.InitLogger(zap.NewNop())
	m.Reload(cfg)
}

func TestManager_StopAll_WaitsAndResets(t *testing.T) {
	m := NewManager()
	// StopAll on empty jobs should not panic and should reset map
	m.StopAll()
}

func TestReload_AutoStartFalseEarlyReturn_More(t *testing.T) {
	m := &Manager{}
	cfg := &models.Config{Global: models.SettingsGlobal{DownloadDir: "", AutoStart: false}}
	require.NotPanics(t, func() { m.Reload(cfg) })
}

func TestReload_InvalidRSSNotStarted(t *testing.T) {
	m := NewManager()
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: "/tmp", AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			models.CMCT:  {Enabled: ptr(true), RSS: []models.RSSConfig{{Name: "r1", URL: "http://"}}},
			models.HDSKY: {Enabled: ptr(true), RSS: []models.RSSConfig{{Name: "r2", URL: "https://rss.m-team.xxx/path"}}},
			models.MTEAM: {Enabled: ptr(true), RSS: []models.RSSConfig{{Name: "r3", URL: "https://example/rss"}}},
		},
	}
	require.NotPanics(t, func() { m.Reload(cfg) })
}

func TestReload_NoGlobalDBEarlyReturn(t *testing.T) {
	m := NewManager()
	global.GlobalDB = nil
	m.Reload(&models.Config{Global: models.SettingsGlobal{DownloadDir: "/tmp", AutoStart: true}})
}

func TestValidRSS_Cases(t *testing.T) {
	require.False(t, validRSS(""))
	require.False(t, validRSS("ftp://example.com"))
	require.False(t, validRSS("http:///path"))
	require.False(t, validRSS("https://rss.m-team.xxx/feed"))
	require.True(t, validRSS("https://example.com/feed"))
}

func TestGetInterval_DefaultsAndGlobal(t *testing.T) {
	// default when <=0 and no global
	global.GlobalDB = nil
	d := getInterval(models.RSSConfig{IntervalMinutes: 0})
	require.Equal(t, 10*time.Minute, d)
	// read from global when available
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	_ = core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DefaultIntervalMinutes: 3, DownloadDir: t.TempDir()})
	d2 := getInterval(models.RSSConfig{IntervalMinutes: 0})
	require.Equal(t, time.Duration(models.MinIntervalMinutes)*time.Minute, d2)
	// explicit positive
	require.Equal(t, 5*time.Minute, getInterval(models.RSSConfig{IntervalMinutes: 5}))
}

func TestReload_StartAndStopAll_WithValidConfig(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	store := core.NewConfigStore(db)
	_ = store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, DefaultEnabled: true, AutoStart: true})
	// qbit server that returns login OK and maindata
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/sync/maindata":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":10000000}}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()
	_ = store.SaveQbitSettings(models.QbitSettings{Enabled: true, URL: srv.URL, User: "u", Password: "p"})
	cfg := &models.Config{Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true}, Sites: map[models.SiteGroup]models.SiteConfig{models.MTEAM: {Enabled: ptr(true), RSS: []models.RSSConfig{{Name: "r1", URL: "https://example.com/rss", IntervalMinutes: 1}}}}}
	m := NewManager()
	m.Reload(cfg)
	time.Sleep(100 * time.Millisecond)
	m.StopAll()
}

func TestManager_Reload_StartAllBranches(t *testing.T) {
	db, _ := core.NewTempDBDir(t.TempDir())
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	store := core.NewConfigStore(db)
	_ = store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, DefaultEnabled: true, AutoStart: true})
	// set qbit
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/sync/maindata":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":10000000}}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()
	_ = store.SaveQbitSettings(models.QbitSettings{Enabled: true, URL: srv.URL, User: "u", Password: "p"})
	cfg := &models.Config{Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true}, Sites: map[models.SiteGroup]models.SiteConfig{
		models.CMCT:  {Enabled: ptr(true), RSS: []models.RSSConfig{{Name: "r1", URL: "https://example.com/rss", IntervalMinutes: 1}}},
		models.HDSKY: {Enabled: ptr(true), RSS: []models.RSSConfig{{Name: "r2", URL: "https://example.com/rss", IntervalMinutes: 1}}},
		models.MTEAM: {Enabled: ptr(true), RSS: []models.RSSConfig{{Name: "r3", URL: "https://example.com/rss", IntervalMinutes: 1}}},
	}}
	m := NewManager()
	m.Reload(cfg)
	time.Sleep(100 * time.Millisecond)
	m.StopAll()
}
func ptr(b bool) *bool { return &b }

type rssSiteStub struct{}

func (s *rssSiteStub) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	d := models.PHPTorrentInfo{Title: item.Title, TorrentID: item.GUID, Discount: models.DISCOUNT_FREE, EndTime: time.Now().Add(1 * time.Hour), SizeMB: 64}
	return &models.APIResponse[models.PHPTorrentInfo]{Code: "success", Data: d}, nil
}
func (s *rssSiteStub) IsEnabled() bool                                        { return true }
func (s *rssSiteStub) DownloadTorrent(url, title, dir string) (string, error) { return "hash-rss", nil }
func (s *rssSiteStub) MaxRetries() int                                        { return 1 }
func (s *rssSiteStub) RetryDelay() time.Duration                              { return 0 }
func (s *rssSiteStub) SendTorrentToQbit(ctx context.Context, rssCfg models.RSSConfig) error {
	return nil
}
func (s *rssSiteStub) Context() context.Context { return context.Background() }
func TestRunRSSJob_WithStub(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	if err = core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 1, DefaultEnabled: true}); err != nil {
		t.Fatalf("save gl: %v", err)
	}
	feed := bytes.NewBufferString(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Test</title>
<item><title>ItemRSS</title><guid>guid-1</guid><enclosure url="http://localhost/file.torrent" type="application/x-bittorrent"/></item>
</channel></rss>`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); _, _ = w.Write(feed.Bytes()) }))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := models.RSSConfig{Name: "r", URL: srv.URL, Tag: "tag", IntervalMinutes: 1}
	go runRSSJob(ctx, models.CMCT, cfg, &rssSiteStub{})
	time.Sleep(200 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)
}

func TestManager_StartStop(t *testing.T) {
	m := NewManager()
	r := models.RSSConfig{Name: "r1", URL: "http://example", IntervalMinutes: 1}
	ran := make(chan struct{}, 1)
	m.Start(models.CMCT, r, func(ctx context.Context) { ran <- struct{}{} })
	m.Start(models.CMCT, r, func(ctx context.Context) { ran <- struct{}{} })
	if _, ok := <-ran; !ok {
		t.Fatalf("runner not invoked")
	}
	m.Stop(models.CMCT, r.Name)
}

func TestKeyFormat(t *testing.T) {
	assert.Equal(t, "cmct|rss", key(models.SiteGroup("cmct"), "rss"))
}

func TestValidRSS_MoreBranches(t *testing.T) {
	assert.False(t, validRSS("ftp://example.com"))
	assert.False(t, validRSS("http:///path"))
	assert.False(t, validRSS("https://"))
	assert.True(t, validRSS("http://host/path"))
	assert.True(t, validRSS("https://host/path"))
}
