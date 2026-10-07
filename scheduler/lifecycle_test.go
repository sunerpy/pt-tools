package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// 启动任务后立刻停止，也要等任务真正退出：WaitGroup 必须在启动协程之前登记，
// 否则 Wait 可能在计数还是 0 时返回，旧任务随后才开始运行。
func TestManager_StopWaitsForJobStartedJustBefore(t *testing.T) {
	m := newTestManager(t)
	var finished atomic.Bool
	m.Start(models.SiteGroup("s"), models.RSSConfig{Name: "r"}, func(ctx context.Context) {
		<-ctx.Done()
		time.Sleep(50 * time.Millisecond)
		finished.Store(true)
	})
	m.StopAll()
	assert.True(t, finished.Load(), "StopAll 返回时任务已经退出")
}

func setupLifecycleDB(t *testing.T) *models.TorrentDB {
	t.Helper()
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })
	return db
}

// 「停止所有任务」只停 RSS 任务：监控器和配置热重载继续运行，「启动所有任务」能恢复。
func TestManager_StopJobsKeepsMonitorsRunning(t *testing.T) {
	setupLifecycleDB(t)
	m := newTestManager(t)
	m.InitFreeEndMonitor()
	require.NotNil(t, m.GetFreeEndMonitor())

	stopped := make(chan struct{})
	m.Start(models.SiteGroup("s"), models.RSSConfig{Name: "r"}, func(ctx context.Context) {
		<-ctx.Done()
		close(stopped)
	})
	m.StopJobs()

	select {
	case <-stopped:
	default:
		t.Fatal("StopJobs 返回时 RSS 任务应已退出")
	}
	assert.Empty(t, m.ListJobs())
	assert.NotNil(t, m.GetFreeEndMonitor(), "免费到期监控继续运行")
	m.mu.Lock()
	assert.NotNil(t, m.cleanupMonitor)
	assert.NotNil(t, m.peerRatioMonitor)
	assert.False(t, m.stopped, "配置事件订阅仍然有效")
	m.mu.Unlock()
}

// 下载目录被清空时，Reload 要停掉带着旧配置运行的任务，而不是提前返回把它们留着。
func TestReload_StopsRunningJobsWhenConfigNotReady(t *testing.T) {
	setupLifecycleDB(t)
	m := newTestManager(t)
	stopped := make(chan struct{})
	m.Start(models.SiteGroup("s"), models.RSSConfig{Name: "r"}, func(ctx context.Context) {
		<-ctx.Done()
		close(stopped)
	})

	m.Reload(&models.Config{Global: models.SettingsGlobal{DownloadDir: "", AutoStart: true}})

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("旧任务没有被停止")
	}
	assert.Empty(t, m.ListJobs())
}

func lifecycleConfig(t *testing.T, db *models.TorrentDB, autoStart bool) *models.Config {
	t.Helper()
	srv := createMockQBitServer(t, 0.5, false)
	t.Cleanup(srv.Close)
	require.NoError(t, db.DB.Create(&models.DownloaderSetting{
		Name: "qb-live", Type: "qbittorrent", URL: srv.URL, Username: "admin", Password: "admin",
		Enabled: true, IsDefault: true,
	}).Error)
	e := true
	_, err := core.NewConfigStore(db).UpsertSite(models.SiteGroup("springsunday"), models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
	require.NoError(t, err)
	return &models.Config{
		Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: autoStart},
		Sites: map[models.SiteGroup]models.SiteConfig{
			"springsunday": {Enabled: &e, RSS: []models.RSSConfig{{Name: "r1", URL: "https://example.com/rss", IntervalMinutes: 60}}},
		},
	}
}

// 没开自动启动、但用户手动启动过任务时，配置变更按新配置重启任务；没启动过的不会被拉起。
func TestReload_ManualStartSurvivesConfigChange(t *testing.T) {
	db := setupLifecycleDB(t)
	cfg := lifecycleConfig(t, db, false)
	m := newTestManager(t)

	m.Reload(cfg)
	assert.Empty(t, m.ListJobs(), "未开启自动启动、也没手动启动过时不启动任务")

	m.StartAll(cfg)
	require.Len(t, m.ListJobs(), 1)
	m.Reload(cfg)
	assert.Len(t, m.ListJobs(), 1, "手动启动过的任务在配置变更后按新配置重启")
}

// 用户在调度器里停止任务后，配置变更（即使开着自动启动）也不会把任务拉起来，直到再次手动启动。
func TestReload_RespectsManualStop(t *testing.T) {
	db := setupLifecycleDB(t)
	cfg := lifecycleConfig(t, db, true)
	m := newTestManager(t)

	m.Reload(cfg)
	require.Len(t, m.ListJobs(), 1)

	m.StopJobs()
	m.Reload(cfg)
	assert.Empty(t, m.ListJobs(), "手动停止后配置变更不重启任务")

	m.StartAll(cfg)
	assert.Len(t, m.ListJobs(), 1)
}

// 配置变更的重载在别的 goroutine 里换监控，可能和 StopAll 同时进行：不能有数据竞争（-race 下检查），
// StopAll 以后也不再起新的监控。
func TestManager_InitMonitorsConcurrentWithStopAll(t *testing.T) {
	setupLifecycleDB(t)
	for range 20 {
		m := NewManager()
		done := make(chan struct{})
		go func() {
			defer close(done)
			m.initFreeEndMonitor()
			m.initCleanupMonitor()
			m.initPeerRatioMonitor()
		}()
		m.StopAll()
		<-done
		m.mu.Lock()
		assert.Nil(t, m.freeEndMonitor)
		assert.Nil(t, m.cleanupMonitor)
		assert.Nil(t, m.peerRatioMonitor)
		m.mu.Unlock()
	}
}

// 换监控的过程（持 monMu）还没完时，StopAll 等它完了再停，返回时不会有正在停的旧监控被漏掉
func TestManager_StopAllWaitsForMonitorSwap(t *testing.T) {
	setupLifecycleDB(t)
	m := NewManager()
	m.monMu.Lock()
	done := make(chan struct{})
	go func() {
		m.StopAll()
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("StopAll 没等正在换监控的那次")
	case <-time.After(100 * time.Millisecond):
	}
	m.monMu.Unlock()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("换完以后 StopAll 没有返回")
	}
	m.initCleanupMonitor()
	m.mu.Lock()
	assert.Nil(t, m.cleanupMonitor, "StopAll 以后不再起新的")
	m.mu.Unlock()
}
