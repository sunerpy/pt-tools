package scheduler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func statusGlobal(autoStart bool) models.SettingsGlobal {
	return models.SettingsGlobal{DownloadDir: "downloads", AutoStart: autoStart}
}

var oneRSS = []models.RSSConfig{{Name: "r1", URL: "https://example.com/rss"}}

// 程序启动后没开自动启动、也没人点过启动：RSS 任务是「未启动」，不是「已停止」。
func TestRSSStatus_ManualBeforeAnyStart(t *testing.T) {
	m := newTestManager(t)
	st := m.RSSStatus(statusGlobal(false), oneRSS)
	assert.Equal(t, RSSManual, st.State)
	assert.Equal(t, 0, st.Jobs)
	assert.Equal(t, 1, st.Subscriptions)
	assert.False(t, st.AutoStart)
	assert.True(t, st.StartedAt.IsZero())
}

// 开着自动启动、启动时的那次重载还没做完：显示「启动中」，不能报成未启动。
func TestRSSStatus_AutoStartBeforeFirstReloadIsStarting(t *testing.T) {
	m := newTestManager(t)
	st := m.RSSStatus(statusGlobal(true), oneRSS)
	assert.Equal(t, RSSStarting, st.State)
	assert.True(t, st.AutoStart)
}

func TestRSSStatus_RunningJobs(t *testing.T) {
	m := newTestManager(t)
	before := time.Now()
	for _, name := range []string{"r1", "r2"} {
		m.Start(models.SiteGroup("s"), models.RSSConfig{Name: name}, func(ctx context.Context) { <-ctx.Done() })
	}
	st := m.RSSStatus(statusGlobal(false), oneRSS)
	assert.Equal(t, RSSRunning, st.State)
	assert.Equal(t, 2, st.Jobs)
	assert.False(t, st.StartedAt.Before(before), "StartedAt 是最早一个任务的启动时间")
}

func TestRSSStatus_PausedAfterStopJobs(t *testing.T) {
	m := newTestManager(t)
	m.Start(models.SiteGroup("s"), models.RSSConfig{Name: "r1"}, func(ctx context.Context) { <-ctx.Done() })
	m.StopJobs()
	// 开着自动启动也一样：手动停止以后配置变更不会把任务拉起来
	assert.Equal(t, RSSPaused, m.RSSStatus(statusGlobal(true), oneRSS).State)
	assert.Equal(t, RSSPaused, m.RSSStatus(statusGlobal(false), oneRSS).State)
}

// 示例订阅、空地址、无效地址不算可以运行的订阅（和 Reload 跳过的口径一致）。
func TestRSSStatus_NoRunnableSubscriptions(t *testing.T) {
	m := newTestManager(t)
	rss := []models.RSSConfig{
		{Name: "example", URL: "https://example.com/rss", IsExample: true},
		{Name: "empty"},
		{Name: "bad", URL: "ftp://example.com/rss"},
	}
	st := m.RSSStatus(statusGlobal(false), rss)
	assert.Equal(t, RSSNoSubscriptions, st.State)
	assert.Equal(t, 0, st.Subscriptions)
}

func TestRSSStatus_NotReadyWithoutDownloadDir(t *testing.T) {
	m := newTestManager(t)
	st := m.RSSStatus(models.SettingsGlobal{DownloadDir: "  ", AutoStart: true}, oneRSS)
	assert.Equal(t, RSSNotReady, st.State)
}

func TestRSSStatus_StartingWhileReloading(t *testing.T) {
	m := newTestManager(t)
	m.mu.Lock()
	m.reloading++
	m.mu.Unlock()
	assert.Equal(t, RSSStarting, m.RSSStatus(statusGlobal(false), oneRSS).State)
	m.mu.Lock()
	m.reloading--
	m.mu.Unlock()
	assert.Equal(t, RSSManual, m.RSSStatus(statusGlobal(false), oneRSS).State)
}

// 决定了要运行、却没有任务在跑（例如站点都没注册）：不能报成运行中，也不能报成没有订阅。
func TestRSSStatus_DecidedToRunButNothingRunning(t *testing.T) {
	m := newTestManager(t)
	m.mu.Lock()
	m.lastDecision = RSSRunning
	m.mu.Unlock()
	assert.Equal(t, RSSStopped, m.RSSStatus(statusGlobal(true), oneRSS).State)
}

// Reload 没有启动任务时记下原因，状态查询照实报出来。
func TestReload_RecordsWhyJobsDidNotStart(t *testing.T) {
	t.Run("没开自动启动", func(t *testing.T) {
		db := setupLifecycleDB(t)
		cfg := lifecycleConfig(t, db, false)
		m := newTestManager(t)
		m.Reload(cfg)
		require.Empty(t, m.ListJobs())
		assert.Equal(t, RSSManual, m.RSSStatus(cfg.Global, oneRSS).State)
		// 随后打开自动启动：配置变更引起的那次重载还没做完，显示「启动中」
		assert.Equal(t, RSSStarting, m.RSSStatus(statusGlobal(true), oneRSS).State)
	})

	t.Run("手动停止", func(t *testing.T) {
		db := setupLifecycleDB(t)
		cfg := lifecycleConfig(t, db, true)
		m := newTestManager(t)
		m.StopJobs()
		m.Reload(cfg)
		require.Empty(t, m.ListJobs())
		assert.Equal(t, RSSPaused, m.RSSStatus(cfg.Global, oneRSS).State)
	})

	t.Run("没有默认下载器", func(t *testing.T) {
		setupLifecycleDB(t)
		m := newTestManager(t)
		cfg := &models.Config{Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true}}
		m.Reload(cfg)
		require.Empty(t, m.ListJobs())
		assert.Equal(t, RSSNoDownloader, m.RSSStatus(cfg.Global, oneRSS).State)
	})

	t.Run("默认下载器连不上", func(t *testing.T) {
		db := setupLifecycleDB(t)
		// 登录成功，其余接口（包括健康检查用的 app/version）都回 500
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v2/auth/login" {
				_, _ = w.Write([]byte("Ok."))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)
		require.NoError(t, db.DB.Create(&models.DownloaderSetting{
			Name: "qb-down", Type: "qbittorrent", URL: srv.URL, Username: "admin", Password: "admin",
			Enabled: true, IsDefault: true,
		}).Error)
		m := newTestManager(t)
		cfg := &models.Config{Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true}}
		m.Reload(cfg)
		require.Empty(t, m.ListJobs())
		assert.Equal(t, RSSDownloaderUnhealthy, m.RSSStatus(cfg.Global, oneRSS).State)
	})

	t.Run("启动成功", func(t *testing.T) {
		db := setupLifecycleDB(t)
		cfg := lifecycleConfig(t, db, true)
		m := newTestManager(t)
		m.Reload(cfg)
		require.Len(t, m.ListJobs(), 1)
		st := m.RSSStatus(cfg.Global, cfg.Sites["springsunday"].RSS)
		assert.Equal(t, RSSRunning, st.State)
		assert.Equal(t, 1, st.Jobs)
		assert.Equal(t, 1, st.Subscriptions)
	})
}

// 手动启动以后，之前记下的原因（例如下载器连不上）不再算数。
func TestStartAll_ClearsSkipReason(t *testing.T) {
	setupLifecycleDB(t)
	m := newTestManager(t)
	cfg := &models.Config{Global: models.SettingsGlobal{DownloadDir: t.TempDir(), AutoStart: true}}
	m.Reload(cfg)
	require.Equal(t, RSSNoDownloader, m.RSSStatus(cfg.Global, oneRSS).State)
	m.StartAll(cfg) // 配置里没有站点，一个任务也起不来
	assert.Equal(t, RSSStopped, m.RSSStatus(cfg.Global, oneRSS).State)
	assert.Equal(t, RSSNoSubscriptions, m.RSSStatus(cfg.Global, nil).State)
}
