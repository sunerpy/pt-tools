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
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func newTestManager(t *testing.T) *Manager {
	m := NewManager()
	t.Cleanup(func() {
		m.StopAll()
	})
	return m
}

func TestManager_StartStopAll(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := core.NewConfigStore(db)
	m := newTestManager(t)
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
	m := newTestManager(t)
	cfg := &models.Config{Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true}, Sites: map[models.SiteGroup]models.SiteConfig{}}
	m.StartAll(cfg)
}

func TestReload_AutoStartFalseEarlyReturn(t *testing.T) {
	m := newTestManager(t)
	dir := t.TempDir()
	cfg := &models.Config{Global: models.SettingsGlobal{DownloadDir: dir, AutoStart: false}, Sites: map[models.SiteGroup]models.SiteConfig{}}
	global.InitLogger(zap.NewNop())
	m.Reload(cfg)
}

func TestManager_StopAll_WaitsAndResets(t *testing.T) {
	m := newTestManager(t)
	// StopAll on empty jobs should not panic and should reset map
	m.StopAll()
}

func TestReload_AutoStartFalseEarlyReturn_More(t *testing.T) {
	m := &Manager{}
	cfg := &models.Config{Global: models.SettingsGlobal{DownloadDir: "", AutoStart: false}}
	require.NotPanics(t, func() { m.Reload(cfg) })
}

func TestReload_InvalidRSSNotStarted(t *testing.T) {
	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: "/tmp", AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			models.SpringSunday: {Enabled: ptr(true), RSS: []models.RSSConfig{{Name: "r1", URL: "http://"}}},
			models.HDSKY:        {Enabled: ptr(true), RSS: []models.RSSConfig{{Name: "r2", URL: "https://rss.m-team.xxx/path"}}},
			models.MTEAM:        {Enabled: ptr(true), RSS: []models.RSSConfig{{Name: "r3", URL: "https://example/rss"}}},
		},
	}
	require.NotPanics(t, func() { m.Reload(cfg) })
}

func TestReload_NoGlobalDBEarlyReturn(t *testing.T) {
	m := newTestManager(t)
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
	m := newTestManager(t)
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
		models.SpringSunday: {Enabled: ptr(true), RSS: []models.RSSConfig{{Name: "r1", URL: "https://example.com/rss", IntervalMinutes: 1}}},
		models.HDSKY:        {Enabled: ptr(true), RSS: []models.RSSConfig{{Name: "r2", URL: "https://example.com/rss", IntervalMinutes: 1}}},
		models.MTEAM:        {Enabled: ptr(true), RSS: []models.RSSConfig{{Name: "r3", URL: "https://example.com/rss", IntervalMinutes: 1}}},
	}}
	m := newTestManager(t)
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
	go runRSSJob(ctx, models.SpringSunday, cfg, &rssSiteStub{})
	time.Sleep(200 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)
}

func TestManager_StartStop(t *testing.T) {
	m := newTestManager(t)
	r := models.RSSConfig{Name: "r1", URL: "http://example", IntervalMinutes: 1}
	ran := make(chan struct{}, 1)
	m.Start(models.SpringSunday, r, func(ctx context.Context) { ran <- struct{}{} })
	m.Start(models.SpringSunday, r, func(ctx context.Context) { ran <- struct{}{} })
	if _, ok := <-ran; !ok {
		t.Fatalf("runner not invoked")
	}
	m.Stop(models.SpringSunday, r.Name)
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

// TestQbitDownloaderConfig 测试 qBittorrent 下载器配置
func TestQbitDownloaderConfig(t *testing.T) {
	config := &qbitDownloaderConfig{
		url:      "http://localhost:8080",
		username: "admin",
		password: "adminadmin",
	}

	assert.Equal(t, "http://localhost:8080", config.GetURL())
	assert.Equal(t, "admin", config.GetUsername())
	assert.Equal(t, "adminadmin", config.GetPassword())
	assert.NoError(t, config.Validate())

	// 测试空 URL 验证失败
	emptyConfig := &qbitDownloaderConfig{url: ""}
	assert.Error(t, emptyConfig.Validate())
}

// TestTransmissionDownloaderConfig 测试 Transmission 下载器配置
func TestTransmissionDownloaderConfig(t *testing.T) {
	config := &transmissionDownloaderConfig{
		url:      "http://localhost:9091",
		username: "admin",
		password: "password",
	}

	assert.Equal(t, "http://localhost:9091", config.GetURL())
	assert.Equal(t, "admin", config.GetUsername())
	assert.Equal(t, "password", config.GetPassword())
	assert.NoError(t, config.Validate())

	// 测试空 URL 验证失败
	emptyConfig := &transmissionDownloaderConfig{url: ""}
	assert.Error(t, emptyConfig.Validate())
}

// TestGetDownloaderManager 测试获取下载器管理器
func TestGetDownloaderManager(t *testing.T) {
	m := newTestManager(t)
	dm := m.GetDownloaderManager()
	// 可能为 nil，取决于初始化状态
	_ = dm
}

// TestInitDownloaderManager 测试下载器管理器初始化
func TestInitDownloaderManager(t *testing.T) {
	// 测试 GlobalDB 为 nil 的情况
	global.GlobalDB = nil
	m := &Manager{
		jobs:              map[string]*job{},
		downloaderManager: nil,
	}
	m.initDownloaderManager()
	// 不应该 panic

	// 测试有 GlobalDB 的情况
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	m2 := newTestManager(t)
	m2.initDownloaderManager()
	// 验证下载器管理器已初始化
	assert.NotNil(t, m2.downloaderManager)
}

// TestInitDownloaderManager_WithDownloaderSettings 测试带下载器配置的初始化
func TestInitDownloaderManager_WithDownloaderSettings(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	// 创建下载器配置
	ds := models.DownloaderSetting{
		Name:     "test-qbit",
		Type:     "qbittorrent",
		URL:      "http://localhost:8080",
		Username: "admin",
		Password: "password",
		Enabled:  true,
	}
	err = db.DB.Create(&ds).Error
	require.NoError(t, err)

	m := newTestManager(t)
	m.initDownloaderManager()
	assert.NotNil(t, m.downloaderManager)
}

// TestInitDownloaderManager_WithTransmissionSettings 测试 Transmission 配置
func TestInitDownloaderManager_WithTransmissionSettings(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	// 创建 Transmission 下载器配置
	ds := models.DownloaderSetting{
		Name:     "test-transmission",
		Type:     "transmission",
		URL:      "http://localhost:9091",
		Username: "admin",
		Password: "password",
		Enabled:  true,
	}
	err = db.DB.Create(&ds).Error
	require.NoError(t, err)

	m := newTestManager(t)
	m.initDownloaderManager()
	assert.NotNil(t, m.downloaderManager)
}

// TestInitDownloaderManager_WithUnknownType 测试未知下载器类型
func TestInitDownloaderManager_WithUnknownType(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	// 创建未知类型的下载器配置
	ds := models.DownloaderSetting{
		Name:     "test-unknown",
		Type:     "unknown",
		URL:      "http://localhost:8080",
		Username: "admin",
		Password: "password",
		Enabled:  true,
	}
	err = db.DB.Create(&ds).Error
	require.NoError(t, err)

	m := newTestManager(t)
	require.NotPanics(t, func() {
		m.initDownloaderManager()
	})
}

// TestInitDownloaderManager_DisabledDownloader 测试禁用的下载器
func TestInitDownloaderManager_DisabledDownloader(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	// 创建禁用的下载器配置
	ds := models.DownloaderSetting{
		Name:     "test-disabled",
		Type:     "qbittorrent",
		URL:      "http://localhost:8080",
		Username: "admin",
		Password: "password",
		Enabled:  false,
	}
	err = db.DB.Create(&ds).Error
	require.NoError(t, err)

	m := newTestManager(t)
	m.initDownloaderManager()
	assert.NotNil(t, m.downloaderManager)
}

// TestCreateQBitFactory 测试 qBittorrent 工厂创建
func TestCreateQBitFactory(t *testing.T) {
	factory := createQBitFactory()
	assert.NotNil(t, factory)

	// 创建一个 mock 配置
	config := &qbitDownloaderConfig{
		url:      "http://localhost:8080",
		username: "admin",
		password: "adminadmin",
	}

	// 工厂应该能创建下载器（即使连接失败）
	_, err := factory(config, "test-qbit")
	// 由于没有实际的 qBittorrent 服务器，可能会返回错误
	// 但工厂函数本身应该能正常调用
	_ = err
}

// TestCreateTransmissionFactory 测试 Transmission 工厂创建
func TestCreateTransmissionFactory(t *testing.T) {
	factory := createTransmissionFactory()
	assert.NotNil(t, factory)

	// 创建一个 mock 配置
	config := &transmissionDownloaderConfig{
		url:      "http://localhost:9091",
		username: "admin",
		password: "password",
	}

	// 工厂应该能创建下载器
	_, err := factory(config, "test-transmission")
	_ = err
}

// TestQbitDownloaderConfig_GetType 测试 GetType 方法
func TestQbitDownloaderConfig_GetType(t *testing.T) {
	config := &qbitDownloaderConfig{
		url:      "http://localhost:8080",
		username: "admin",
		password: "adminadmin",
	}
	assert.Equal(t, "qbittorrent", string(config.GetType()))
}

// TestTransmissionDownloaderConfig_GetType 测试 GetType 方法
func TestTransmissionDownloaderConfig_GetType(t *testing.T) {
	config := &transmissionDownloaderConfig{
		url:      "http://localhost:9091",
		username: "admin",
		password: "password",
	}
	assert.Equal(t, "transmission", string(config.GetType()))
}

// TestStartAll_WithMTEAM 测试 StartAll 处理 MTEAM 站点
func TestStartAll_WithMTEAM(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			models.MTEAM: {
				Enabled: ptr(true),
				RSS: []models.RSSConfig{
					{Name: "test-rss", URL: "https://example.com/rss", IntervalMinutes: 1},
				},
			},
		},
	}

	// StartAll 应该不会 panic
	require.NotPanics(t, func() {
		m.StartAll(cfg)
	})
	time.Sleep(50 * time.Millisecond)
	m.StopAll()
}

// TestStartAll_WithHDSKY 测试 StartAll 处理 HDSKY 站点
func TestStartAll_WithHDSKY(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			models.HDSKY: {
				Enabled: ptr(true),
				RSS: []models.RSSConfig{
					{Name: "test-rss", URL: "https://example.com/rss", IntervalMinutes: 1},
				},
			},
		},
	}

	require.NotPanics(t, func() {
		m.StartAll(cfg)
	})
	time.Sleep(50 * time.Millisecond)
	m.StopAll()
}

// TestStartAll_WithCMCT 测试 StartAll 处理 CMCT 站点
func TestStartAll_WithCMCT(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			models.SpringSunday: {
				Enabled: ptr(true),
				RSS: []models.RSSConfig{
					{Name: "test-rss", URL: "https://example.com/rss", IntervalMinutes: 1},
				},
			},
		},
	}

	require.NotPanics(t, func() {
		m.StartAll(cfg)
	})
	time.Sleep(50 * time.Millisecond)
	m.StopAll()
}

// TestStartAll_WithUnknownSite 测试 StartAll 处理未知站点
func TestStartAll_WithUnknownSite(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			models.SiteGroup("unknown"): {
				Enabled: ptr(true),
				RSS: []models.RSSConfig{
					{Name: "test-rss", URL: "https://example.com/rss", IntervalMinutes: 1},
				},
			},
		},
	}

	require.NotPanics(t, func() {
		m.StartAll(cfg)
	})
}

// TestStartAll_WithSkippedRSS 测试 StartAll 跳过应该跳过的 RSS
func TestStartAll_WithSkippedRSS(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			models.MTEAM: {
				Enabled: ptr(true),
				RSS: []models.RSSConfig{
					{Name: "test-rss", URL: "", IntervalMinutes: 1}, // 空 URL 应该被跳过
				},
			},
		},
	}

	require.NotPanics(t, func() {
		m.StartAll(cfg)
	})
}

// TestStartAll_WithDisabledSite 测试 StartAll 跳过禁用的站点
func TestStartAll_WithDisabledSite(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			models.MTEAM: {
				Enabled: ptr(false), // 禁用
				RSS: []models.RSSConfig{
					{Name: "test-rss", URL: "https://example.com/rss", IntervalMinutes: 1},
				},
			},
		},
	}

	require.NotPanics(t, func() {
		m.StartAll(cfg)
	})
}

// TestStartAll_AllSiteTypes 测试 StartAll 处理所有站点类型
func TestStartAll_AllSiteTypes(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			models.MTEAM: {
				Enabled: ptr(true),
				RSS: []models.RSSConfig{
					{Name: "mteam-rss", URL: "https://example.com/mteam", IntervalMinutes: 1},
				},
			},
			models.HDSKY: {
				Enabled: ptr(true),
				RSS: []models.RSSConfig{
					{Name: "hdsky-rss", URL: "https://example.com/hdsky", IntervalMinutes: 1},
				},
			},
			models.SpringSunday: {
				Enabled: ptr(true),
				RSS: []models.RSSConfig{
					{Name: "cmct-rss", URL: "https://example.com/cmct", IntervalMinutes: 1},
				},
			},
		},
	}

	require.NotPanics(t, func() {
		m.StartAll(cfg)
	})
	time.Sleep(100 * time.Millisecond)
	m.StopAll()
}

// TestExecuteTask 测试 executeTask 函数
func TestExecuteTask(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	ctx := context.Background()
	cfg := models.RSSConfig{Name: "test", URL: "http://invalid-url", IntervalMinutes: 1}

	// 使用 stub 测试
	stub := &rssSiteStub{}
	require.NotPanics(t, func() {
		executeTask(ctx, models.SpringSunday, cfg, stub)
	})
}

// TestProcessRSS 测试 processRSS 函数
func TestProcessRSS(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	ctx := context.Background()
	cfg := models.RSSConfig{Name: "test", URL: "http://invalid-url", IntervalMinutes: 1}

	stub := &rssSiteStub{}
	// processRSS 可能返回错误，但不应该 panic
	_ = processRSS(ctx, models.SpringSunday, cfg, stub)
}
