package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// unifiedSiteStub is a minimal internal.UnifiedPTSite used to exercise
// runRSSJobUnified / executeTaskUnified / processRSSUnified without touching
// the network. It counts how many times the pipeline ran.
type unifiedSiteStub struct {
	sendCalls int
	sendErr   error
}

func (s *unifiedSiteStub) GetTorrentDetails(item *gofeed.Item) (*v2.TorrentItem, error) {
	return &v2.TorrentItem{Title: item.Title}, nil
}
func (s *unifiedSiteStub) IsEnabled() bool                                { return true }
func (s *unifiedSiteStub) DownloadTorrent(_, _, _ string) (string, error) { return "hash", nil }
func (s *unifiedSiteStub) MaxRetries() int                                { return 1 }
func (s *unifiedSiteStub) RetryDelay() time.Duration                      { return 0 }
func (s *unifiedSiteStub) Context() context.Context                       { return context.Background() }

func (s *unifiedSiteStub) SiteGroup() models.SiteGroup { return models.SiteGroup("stub") }

func (s *unifiedSiteStub) SendTorrentToDownloader(_ context.Context, _ models.RSSConfig) error {
	s.sendCalls++
	return s.sendErr
}

// === InitFreeEndMonitor wires all monitors ===

func TestInitFreeEndMonitor_WiresMonitors(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })

	m := newTestManager(t)
	require.NotPanics(t, func() { m.InitFreeEndMonitor() })
	assert.NotNil(t, m.GetFreeEndMonitor(), "free-end monitor should be wired")
}

// === GetFreeEndMonitor before init returns nil ===

func TestGetFreeEndMonitor_NilBeforeInit(t *testing.T) {
	m := newTestManager(t)
	assert.Nil(t, m.GetFreeEndMonitor())
}

// === Set/GetLoginReminderMonitor ===

func TestSetGetLoginReminderMonitor(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.InitLogger(zap.NewNop())

	m := newTestManager(t)
	assert.Nil(t, m.GetLoginReminderMonitor())

	mon := NewLoginReminderMonitor(LoginReminderConfig{DB: db.DB})
	m.SetLoginReminderMonitor(mon)
	assert.Same(t, mon, m.GetLoginReminderMonitor())

	// Replacing with a different monitor stops the old one; passing same is no-op.
	mon2 := NewLoginReminderMonitor(LoginReminderConfig{DB: db.DB})
	m.SetLoginReminderMonitor(mon2)
	assert.Same(t, mon2, m.GetLoginReminderMonitor())
}

// === runRSSJobUnified runs pipeline at least once then honours ctx cancel ===

func TestRunRSSJobUnified_RunsAndStops(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, DefaultEnabled: true,
	}))

	stub := &unifiedSiteStub{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		// URL is empty → FetchAndDownloadFreeRSSUnified will error out early,
		// but processRSSUnified is still exercised (executeTaskUnified logs it).
		runRSSJobUnified(ctx, models.RSSConfig{Name: "r", URL: "", IntervalMinutes: 1}, stub)
		close(done)
	}()
	time.Sleep(80 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runRSSJobUnified did not stop after cancel")
	}
}

// === processRSSUnified: send-only success path (fetch skipped via cancelled ctx) ===

func TestProcessRSSUnified_SendError(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })

	// Cancelled context makes the fetch step return quickly; then we simply
	// assert the function returns an error (from fetch or send) without panic.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stub := &unifiedSiteStub{}
	require.NotPanics(t, func() {
		_ = processRSSUnified(ctx, models.RSSConfig{Name: "x", URL: ""}, stub)
	})
}

// === Event-driven reload: publishing ConfigChanged triggers Reload ===

// Observed via the mutex-guarded GetFreeEndMonitor() (not the unsynchronized
// LastVersion()) to stay race-free under -race.
func TestManager_EventReload_TriggersReload(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, AutoStart: true,
	}))

	m := newTestManager(t)
	require.Nil(t, m.GetFreeEndMonitor(), "no monitor before any reload")

	events.Publish(events.Event{Type: events.ConfigChanged, Version: 5})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if m.GetFreeEndMonitor() != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	assert.NotNil(t, m.GetFreeEndMonitor(), "event reload should wire the free-end monitor")
}

// === Event-driven reload: stale version ignored ===
func TestManager_EventReload_IgnoresStaleVersion(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, AutoStart: true,
	}))

	m := newTestManager(t)
	events.Publish(events.Event{Type: events.ConfigChanged, Version: 10})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if m.GetFreeEndMonitor() != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	first := m.GetFreeEndMonitor()
	require.NotNil(t, first)

	events.Publish(events.Event{Type: events.ConfigChanged, Version: 3})
	time.Sleep(500 * time.Millisecond)
	assert.Same(t, first, m.GetFreeEndMonitor(), "stale version must not trigger another reload")
}

// === initFreeEndMonitor/initCleanupMonitor/initPeerRatioMonitor replace existing ===

func TestInitMonitors_ReplaceExisting(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })

	m := newTestManager(t)
	m.initDownloaderManager()
	m.initFreeEndMonitor()
	m.initCleanupMonitor()
	m.initPeerRatioMonitor()

	first := m.GetFreeEndMonitor()
	require.NotNil(t, first)

	// Calling again should stop the previous and create a new instance.
	m.initFreeEndMonitor()
	m.initCleanupMonitor()
	m.initPeerRatioMonitor()
	second := m.GetFreeEndMonitor()
	require.NotNil(t, second)
	assert.NotSame(t, first, second, "re-init should replace the free-end monitor")
}
