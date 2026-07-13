// MIT License
// Copyright (c) 2025 pt-tools

package scheduler

import (
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
	v2 "github.com/sunerpy/pt-tools/site/v2"
	_ "github.com/sunerpy/pt-tools/site/v2/definitions"
)

type procRSSStub struct {
	fetchErr error
	sendErr  error
	sends    int
}

func (s *procRSSStub) GetTorrentDetails(item *gofeed.Item) (*v2.TorrentItem, error) {
	return &v2.TorrentItem{Title: item.Title}, nil
}
func (s *procRSSStub) IsEnabled() bool                                { return true }
func (s *procRSSStub) DownloadTorrent(_, _, _ string) (string, error) { return "h", nil }
func (s *procRSSStub) MaxRetries() int                                { return 1 }
func (s *procRSSStub) RetryDelay() time.Duration                      { return 0 }
func (s *procRSSStub) Context() context.Context                       { return context.Background() }

func (s *procRSSStub) SiteGroup() models.SiteGroup { return models.SiteGroup("springsunday") }

func (s *procRSSStub) SendTorrentToDownloader(_ context.Context, _ models.RSSConfig) error {
	s.sends++
	return s.sendErr
}

func TestProcessRSSUnified_FetchThenSendSucceeds(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true,
	}))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>t</title><link>http://x</link><description>d</description></channel></rss>`))
	}))
	t.Cleanup(srv.Close)

	stub := &procRSSStub{}
	err = processRSSUnified(context.Background(), models.RSSConfig{Name: "r", URL: srv.URL}, stub)
	require.NoError(t, err)
	assert.Equal(t, 1, stub.sends)
}

func TestProcessRSSUnified_SendErrorSurfaced(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true,
	}))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>t</title><link>http://x</link><description>d</description></channel></rss>`))
	}))
	t.Cleanup(srv.Close)

	stub := &procRSSStub{sendErr: errSendBoom}
	err = processRSSUnified(context.Background(), models.RSSConfig{Name: "r", URL: srv.URL}, stub)
	require.Error(t, err)
}

func TestExecuteTaskUnified_LogsErrorNoPanic(t *testing.T) {
	global.InitLogger(zap.NewNop())
	global.GlobalDB = nil
	stub := &procRSSStub{}
	require.NotPanics(t, func() {
		executeTaskUnified(context.Background(), models.RSSConfig{Name: "r", URL: ""}, stub)
	})
}

func TestStartAll_ValidSiteRegistersJob(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })

	s := core.NewConfigStore(db)
	e := true
	_, _ = s.UpsertSite(models.SiteGroup("springsunday"), models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})

	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			"springsunday": {Enabled: &e, RSS: []models.RSSConfig{{Name: "r1", URL: "https://example.com/rss", IntervalMinutes: 1}}},
		},
	}
	m.StartAll(cfg)
	jobs := m.ListJobs()
	assert.GreaterOrEqual(t, len(jobs), 1)
	time.Sleep(100 * time.Millisecond) // 让 runner 先执行 wg.Add(1)，再 StopAll wg.Wait()，避免 -race 竞态
	m.StopAll()
}

func TestStartAll_SkipsSampleRSS(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })

	s := core.NewConfigStore(db)
	e := true
	_, _ = s.UpsertSite(models.SiteGroup("springsunday"), models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})

	m := newTestManager(t)
	cfg := &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true},
		Sites: map[models.SiteGroup]models.SiteConfig{
			"springsunday": {Enabled: &e, RSS: []models.RSSConfig{{Name: "sample", URL: ""}}},
		},
	}
	m.StartAll(cfg)
	assert.Empty(t, m.ListJobs())
	m.StopAll()
}

var errSendBoom = &schedGenericErr{"send boom"}
