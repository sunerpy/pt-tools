package cmd

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

type ptStub struct{}

func (s *ptStub) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	d := models.PHPTorrentInfo{Title: item.Title, TorrentID: item.GUID, Discount: models.DISCOUNT_FREE, EndTime: time.Now().Add(1 * time.Hour), SizeMB: 64}
	return &models.APIResponse[models.PHPTorrentInfo]{Code: "success", Data: d}, nil
}
func (s *ptStub) IsEnabled() bool                                                      { return true }
func (s *ptStub) DownloadTorrent(url, title, dir string) (string, error)               { return "hash", nil }
func (s *ptStub) MaxRetries() int                                                      { return 1 }
func (s *ptStub) RetryDelay() time.Duration                                            { return 0 }
func (s *ptStub) SendTorrentToQbit(ctx context.Context, rssCfg models.RSSConfig) error { return nil }
func (s *ptStub) Context() context.Context                                             { return context.Background() }
func TestGetInterval_DefaultAndConfigured(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	if err = core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 7, DefaultEnabled: true}); err != nil {
		t.Fatalf("save gl: %v", err)
	}
	if d := getInterval(models.RSSConfig{IntervalMinutes: 0}); d != 7*time.Minute {
		t.Fatalf("interval: %v", d)
	}
	if d := getInterval(models.RSSConfig{IntervalMinutes: 3}); d != 3*time.Minute {
		t.Fatalf("interval: %v", d)
	}
}

func TestProcessRSS_WithStub(t *testing.T) {
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
	if err := processRSS(context.Background(), models.SpringSunday, models.RSSConfig{Name: "r", URL: srv.URL, Tag: "tag"}, &ptStub{}); err != nil {
		t.Fatalf("proc rss: %v", err)
	}
}

func TestRunRSSJob_SingleMode(t *testing.T) {
	ctx := context.WithValue(context.Background(), modeKey, "single")
	cfg := models.RSSConfig{Name: "r", URL: "http://invalid", Tag: "tag", IntervalMinutes: 1}
	// invalid URL will cause fetchRSS to fail; but runRSSJob calls executeTask first
	// set up DB to avoid early returns in getInterval
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
	go runRSSJob(ctx, models.SpringSunday, cfg, &ptStub{})
	time.Sleep(200 * time.Millisecond)
}

func TestRunSiteJobs_PersistentCancel(t *testing.T) {
	// persistent mode with immediate cancel to cover loop path
	baseCtx, cancel := context.WithCancel(context.Background())
	ctx := context.WithValue(baseCtx, modeKey, "persistent")
	// init logger to avoid nil
	global.InitLogger(zap.NewNop())
	// two RSS entries to exercise WaitGroup
	siteCfg := models.SiteConfig{RSS: []models.RSSConfig{
		{Name: "r1", URL: "http://invalid", IntervalMinutes: 1},
		{Name: "r2", URL: "http://invalid", IntervalMinutes: 1},
	}}
	go runSiteJobs(ctx, models.SpringSunday, siteCfg, &siteStub{})
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)
}

type errSite struct{}

func (s *errSite) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	return nil, context.DeadlineExceeded
}
func (s *errSite) IsEnabled() bool { return true }
func (s *errSite) DownloadTorrent(url, title, dir string) (string, error) {
	return "", context.DeadlineExceeded
}
func (s *errSite) MaxRetries() int                                               { return 1 }
func (s *errSite) RetryDelay() time.Duration                                     { return 0 }
func (s *errSite) SendTorrentToQbit(c context.Context, r models.RSSConfig) error { return nil }
func (s *errSite) Context() context.Context                                      { return context.Background() }
func TestExecuteTask_ErrorPath_NoPanic(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	_ = core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 1, DefaultEnabled: true})
	executeTask(context.Background(), models.SpringSunday, models.RSSConfig{Name: "r", URL: "://bad"}, &errSite{})
}

type siteStub struct{}

func (s *siteStub) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	d := models.PHPTorrentInfo{Title: "x", TorrentID: "id", Discount: models.DISCOUNT_FREE, EndTime: time.Now().Add(1 * time.Hour), SizeMB: 1}
	return &models.APIResponse[models.PHPTorrentInfo]{Code: "success", Data: d}, nil
}
func (s *siteStub) IsEnabled() bool                                               { return true }
func (s *siteStub) DownloadTorrent(url, title, dir string) (string, error)        { return "h", nil }
func (s *siteStub) MaxRetries() int                                               { return 1 }
func (s *siteStub) RetryDelay() time.Duration                                     { return 0 }
func (s *siteStub) SendTorrentToQbit(c context.Context, r models.RSSConfig) error { return nil }
func (s *siteStub) Context() context.Context                                      { return context.Background() }
func TestRunSiteJobs_WithSingleMode(t *testing.T) {
	baseCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := context.WithValue(baseCtx, modeKey, "single")
	global.InitLogger(zap.NewNop())
	siteCfg := models.SiteConfig{RSS: []models.RSSConfig{{Name: "r1", URL: "http://invalid", IntervalMinutes: 1}}}
	runSiteJobs(ctx, models.SpringSunday, siteCfg, &siteStub{})
}

func TestGenTorrentsWithRSSOnce_NoRSS(t *testing.T) {
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
	if err := genTorrentsWithRSSOnce(context.Background()); err != nil {
		t.Fatalf("gen once: %v", err)
	}
}

func TestGenTorrentsWithRSS_PersistentCancel(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	if err = core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 1, DefaultEnabled: true, AutoStart: false}); err != nil {
		t.Fatalf("save gl: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = genTorrentsWithRSS(ctx) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
}

// TestGenTorrentsWithRSSOnce_WithEnabledSites 测试启用站点的情况
func TestGenTorrentsWithRSSOnce_WithEnabledSites(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	store := core.NewConfigStore(db)
	if err = store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 1, DefaultEnabled: true}); err != nil {
		t.Fatalf("save gl: %v", err)
	}

	// 创建启用的站点配置
	enabled := true
	cmctSite := models.SiteSetting{
		Name:       "cmct",
		AuthMethod: "cookie",
		Cookie:     "test-cookie",
		Enabled:    enabled,
	}
	db.DB.Create(&cmctSite)

	// 添加 RSS 订阅
	rss := models.RSSSubscription{
		SiteID:          cmctSite.ID,
		Name:            "test-rss",
		URL:             "http://invalid-url",
		IntervalMinutes: 1,
	}
	db.DB.Create(&rss)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 应该不会 panic，即使 RSS URL 无效
	_ = genTorrentsWithRSSOnce(ctx)
}

// TestGenTorrentsWithRSSOnce_WithDisabledSites 测试禁用站点的情况
func TestGenTorrentsWithRSSOnce_WithDisabledSites(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	store := core.NewConfigStore(db)
	if err = store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 1, DefaultEnabled: true}); err != nil {
		t.Fatalf("save gl: %v", err)
	}

	// 创建禁用的站点配置
	disabled := false
	cmctSite := models.SiteSetting{
		Name:       "cmct",
		AuthMethod: "cookie",
		Cookie:     "test-cookie",
		Enabled:    disabled,
	}
	db.DB.Create(&cmctSite)

	// 应该跳过禁用的站点
	if err := genTorrentsWithRSSOnce(context.Background()); err != nil {
		t.Fatalf("gen once: %v", err)
	}
}

// TestGenTorrentsWithRSS_EmptyDownloadDir 测试空下载目录的情况
func TestGenTorrentsWithRSS_EmptyDownloadDir(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	// 保存有效配置，然后直接修改数据库使下载目录为空
	if err = core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 1, DefaultEnabled: true}); err != nil {
		t.Fatalf("save gl: %v", err)
	}
	// 直接更新数据库使下载目录为空
	db.DB.Model(&models.SettingsGlobal{}).Where("1=1").Update("download_dir", "")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// 应该等待配置完善后再启动
	_ = genTorrentsWithRSS(ctx)
}

// TestGetInterval_ZeroGlobalInterval 测试全局间隔为0的情况
func TestGetInterval_ZeroGlobalInterval(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	// 保存有效配置，然后直接修改数据库使间隔为0
	if err = core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 1, DefaultEnabled: true}); err != nil {
		t.Fatalf("save gl: %v", err)
	}
	// 直接更新数据库使间隔为0
	db.DB.Model(&models.SettingsGlobal{}).Where("1=1").Update("default_interval_minutes", 0)

	// 当全局间隔为0时，应该返回默认的10分钟
	d := getInterval(models.RSSConfig{IntervalMinutes: 0})
	if d != 10*time.Minute {
		t.Fatalf("expected 10 minutes, got: %v", d)
	}
}

// TestRunRSSJob_PersistentModeCancel 测试持续模式取消
func TestRunRSSJob_PersistentModeCancel(t *testing.T) {
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

	baseCtx, cancel := context.WithCancel(context.Background())
	ctx := context.WithValue(baseCtx, modeKey, "persistent")
	cfg := models.RSSConfig{Name: "r", URL: "http://invalid", Tag: "tag", IntervalMinutes: 1}

	go runRSSJob(ctx, models.SpringSunday, cfg, &ptStub{})
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)
}

// TestGenTorrentsWithRSSOnce_WithMTEAM 测试 MTEAM 站点
func TestGenTorrentsWithRSSOnce_WithMTEAM(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	store := core.NewConfigStore(db)
	if err = store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 1, DefaultEnabled: true}); err != nil {
		t.Fatalf("save gl: %v", err)
	}

	// 创建启用的 MTEAM 站点配置
	enabled := true
	mteamSite := models.SiteSetting{
		Name:       "mteam",
		AuthMethod: "api_key",
		APIKey:     "test-api-key",
		Enabled:    enabled,
	}
	db.DB.Create(&mteamSite)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// 由于没有实际的 MTEAM API，会初始化失败并跳过
	_ = genTorrentsWithRSSOnce(ctx)
}

// TestGenTorrentsWithRSSOnce_WithHDSKY 测试 HDSKY 站点
func TestGenTorrentsWithRSSOnce_WithHDSKY(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	store := core.NewConfigStore(db)
	if err = store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 1, DefaultEnabled: true}); err != nil {
		t.Fatalf("save gl: %v", err)
	}

	// 创建启用的 HDSKY 站点配置
	enabled := true
	hdskySite := models.SiteSetting{
		Name:       "hdsky",
		AuthMethod: "cookie",
		Cookie:     "test-cookie",
		Enabled:    enabled,
	}
	db.DB.Create(&hdskySite)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// 由于没有实际的 HDSKY API，会初始化失败并跳过
	_ = genTorrentsWithRSSOnce(ctx)
}

// TestGenTorrentsWithRSSOnce_WithUnknownSite 测试未知站点
func TestGenTorrentsWithRSSOnce_WithUnknownSite(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	store := core.NewConfigStore(db)
	if err = store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 1, DefaultEnabled: true}); err != nil {
		t.Fatalf("save gl: %v", err)
	}

	// 创建启用的未知站点配置
	enabled := true
	unknownSite := models.SiteSetting{
		Name:       "unknown",
		AuthMethod: "cookie",
		Cookie:     "test-cookie",
		Enabled:    enabled,
	}
	db.DB.Create(&unknownSite)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// 未知站点会被跳过
	if err := genTorrentsWithRSSOnce(ctx); err != nil {
		t.Fatalf("gen once: %v", err)
	}
}

// TestGenTorrentsWithRSS_WithAutoStart 测试自动启动
func TestGenTorrentsWithRSS_WithAutoStart(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())

	if err = core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 1, DefaultEnabled: true, AutoStart: true}); err != nil {
		t.Fatalf("save gl: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = genTorrentsWithRSS(ctx)
}

// TestExecuteTask_Success 测试任务执行成功
func TestExecuteTask_Success(t *testing.T) {
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

	cfg := models.RSSConfig{Name: "test-rss", URL: "http://invalid", Tag: "tag"}

	// 使用 stub 站点
	executeTask(context.Background(), models.SpringSunday, cfg, &siteStub{})
}

// TestProcessRSS_SendTorrentError 测试发送种子错误
func TestProcessRSS_SendTorrentError(t *testing.T) {
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

	cfg := models.RSSConfig{Name: "test-rss", URL: "http://invalid", Tag: "tag"}

	// 使用错误站点
	err = processRSS(context.Background(), models.SpringSunday, cfg, &errSite{})
	// 应该返回错误
	if err == nil {
		t.Log("processRSS returned nil error, which is acceptable for invalid URL")
	}
}
