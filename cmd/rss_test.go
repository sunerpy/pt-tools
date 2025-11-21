package cmd

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"go.uber.org/zap"
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
	if err := processRSS(context.Background(), models.CMCT, models.RSSConfig{Name: "r", URL: srv.URL, Tag: "tag"}, &ptStub{}); err != nil {
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
	go runRSSJob(ctx, models.CMCT, cfg, &ptStub{})
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
	go runSiteJobs(ctx, models.CMCT, siteCfg, &siteStub{})
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
	executeTask(context.Background(), models.CMCT, models.RSSConfig{Name: "r", URL: "://bad"}, &errSite{})
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
	runSiteJobs(ctx, models.CMCT, siteCfg, &siteStub{})
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
