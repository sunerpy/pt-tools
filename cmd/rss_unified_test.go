package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// fakeUnifiedSite is an in-memory internal.UnifiedPTSite used to exercise the
// RSS pipeline helpers in rss.go without touching a real tracker.
type fakeUnifiedSite struct {
	enabled     bool
	sendErr     error
	sendCallsN  atomic.Int32
	detailCalls atomic.Int32
}

func (f *fakeUnifiedSite) sendCalls() int { return int(f.sendCallsN.Load()) }

func (f *fakeUnifiedSite) GetTorrentDetails(_ *gofeed.Item) (*v2.TorrentItem, error) {
	f.detailCalls.Add(1)
	return &v2.TorrentItem{ID: "1", Title: "t"}, nil
}
func (f *fakeUnifiedSite) IsEnabled() bool { return f.enabled }
func (f *fakeUnifiedSite) DownloadTorrent(_, _, _ string) (string, error) {
	return "hash", nil
}
func (f *fakeUnifiedSite) MaxRetries() int           { return 1 }
func (f *fakeUnifiedSite) RetryDelay() time.Duration { return time.Millisecond }
func (f *fakeUnifiedSite) SendTorrentToDownloader(_ context.Context, _ models.RSSConfig) error {
	f.sendCallsN.Add(1)
	return f.sendErr
}
func (f *fakeUnifiedSite) Context() context.Context    { return context.Background() }
func (f *fakeUnifiedSite) SiteGroup() models.SiteGroup { return models.SiteGroup("fake") }

var _ internal.UnifiedPTSite = (*fakeUnifiedSite)(nil)

// emptyFeedServer serves a minimal valid RSS document with zero items so
// FetchAndDownloadFreeRSSUnified completes its happy path without downloads.
func emptyFeedServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>t</title><link>http://x</link><description>d</description></channel></rss>`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func seedGlobalWithDownloadDir(t *testing.T) {
	t.Helper()
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, DefaultEnabled: true,
	}))
}

func TestProcessRSSUnified_Success(t *testing.T) {
	seedGlobalWithDownloadDir(t)
	srv := emptyFeedServer(t)
	site := &fakeUnifiedSite{enabled: true}
	cfg := models.RSSConfig{Name: "job", URL: srv.URL}
	require.NoError(t, processRSSUnified(context.Background(), cfg, site))
	assert.Equal(t, 1, site.sendCalls(), "downloader push should run after fetch")
}

func TestProcessRSSUnified_FetchError_DisabledSite(t *testing.T) {
	seedGlobalWithDownloadDir(t)
	site := &fakeUnifiedSite{enabled: false}
	cfg := models.RSSConfig{Name: "job", URL: "http://127.0.0.1:0/none"}
	err := processRSSUnified(context.Background(), cfg, site)
	require.Error(t, err)
	assert.Equal(t, 0, site.sendCalls(), "push must be skipped when fetch fails")
}

func TestExecuteTaskUnified_LogsBothBranches(t *testing.T) {
	seedGlobalWithDownloadDir(t)
	srv := emptyFeedServer(t)
	cfg := models.RSSConfig{Name: "job", URL: srv.URL}

	// success branch
	ok := &fakeUnifiedSite{enabled: true}
	executeTaskUnified(context.Background(), cfg, ok)
	assert.Equal(t, 1, ok.sendCalls())

	// error branch (disabled -> fetch fails)
	bad := &fakeUnifiedSite{enabled: false}
	executeTaskUnified(context.Background(), models.RSSConfig{Name: "bad", URL: "http://127.0.0.1:0/x"}, bad)
	assert.Equal(t, 0, bad.sendCalls())
}

func TestRunRSSJobUnified_SingleModeReturns(t *testing.T) {
	seedGlobalWithDownloadDir(t)
	srv := emptyFeedServer(t)
	site := &fakeUnifiedSite{enabled: true}
	cfg := models.RSSConfig{Name: "job", URL: srv.URL, IntervalMinutes: 1}
	ctx := context.WithValue(context.Background(), modeKey, "single")
	// Must return promptly in single mode (no ticker loop).
	done := make(chan struct{})
	go func() {
		runRSSJobUnified(ctx, cfg, site)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runRSSJobUnified did not return in single mode")
	}
	assert.GreaterOrEqual(t, site.sendCalls(), 1)
}

func TestRunRSSJobUnified_PersistentModeCancels(t *testing.T) {
	seedGlobalWithDownloadDir(t)
	srv := emptyFeedServer(t)
	site := &fakeUnifiedSite{enabled: true}
	cfg := models.RSSConfig{Name: "job", URL: srv.URL, IntervalMinutes: 1}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), modeKey, "persistent"))
	done := make(chan struct{})
	go func() {
		runRSSJobUnified(ctx, cfg, site)
		close(done)
	}()
	// Give the first executeTaskUnified a moment, then cancel to exit the loop.
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runRSSJobUnified did not exit on context cancel")
	}
}

func TestRunSiteJobsUnified_FanOut(t *testing.T) {
	seedGlobalWithDownloadDir(t)
	srv := emptyFeedServer(t)
	site := &fakeUnifiedSite{enabled: true}
	siteCfg := models.SiteConfig{
		RSS: []models.RSSConfig{
			{Name: "a", URL: srv.URL},
			{Name: "b", URL: srv.URL},
		},
	}
	ctx := context.WithValue(context.Background(), modeKey, "single")
	runSiteJobsUnified(ctx, siteCfg, site)
	assert.Equal(t, 2, site.sendCalls(), "both RSS jobs should push once each")
}

func TestGetInterval(t *testing.T) {
	// RSS-level interval takes precedence.
	assert.Equal(t, 5*time.Minute, getInterval(models.RSSConfig{IntervalMinutes: 5}))

	// Zero RSS interval falls back to the global default.
	seedGlobalWithDownloadDir(t)
	require.NoError(t, core.NewConfigStore(global.GlobalDB).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 7, DefaultEnabled: true,
	}))
	assert.Equal(t, 7*time.Minute, getInterval(models.RSSConfig{IntervalMinutes: 0}))
}
