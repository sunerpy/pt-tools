package scheduler

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// countSettingsQueries 统计读取全局设置的次数：监控每一轮都从这里开始。
func countSettingsQueries(t *testing.T, db *gorm.DB, block <-chan struct{}) *atomic.Int32 {
	t.Helper()
	var n atomic.Int32
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("lifecycle:count_settings", func(tx *gorm.DB) {
		if tx.Statement.Table == "settings_globals" {
			n.Add(1)
			if block != nil {
				<-block
			}
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove("lifecycle:count_settings") })
	return &n
}

// 启动后的等待期内停止，监控不会在之后醒来再跑一轮。
func TestCleanupAndPeerRatioMonitors_StopDuringStartDelay(t *testing.T) {
	db := setupLifecycleDB(t)
	queries := countSettingsQueries(t, db.DB, nil)

	cm := NewCleanupMonitor(db.DB, downloader.NewDownloaderManager())
	cm.startDelay = 30 * time.Millisecond
	require.NoError(t, cm.Start())
	cm.Stop()

	pm := NewPeerRatioMonitor(db.DB, downloader.NewDownloaderManager())
	pm.startDelay = 30 * time.Millisecond
	require.NoError(t, pm.Start())
	pm.Stop()

	time.Sleep(150 * time.Millisecond)
	assert.Zero(t, queries.Load(), "停止之后不再执行检查")
}

// Stop 要等正在执行的这一轮结束才返回，被替换的旧实例不会和新实例同时操作下载器。
func TestCleanupMonitor_StopWaitsForRunningIteration(t *testing.T) {
	db := setupLifecycleDB(t)
	release := make(chan struct{})
	queries := countSettingsQueries(t, db.DB, release)

	cm := NewCleanupMonitor(db.DB, downloader.NewDownloaderManager())
	cm.startDelay = 0
	require.NoError(t, cm.Start())
	require.Eventually(t, func() bool { return queries.Load() > 0 }, 2*time.Second, 5*time.Millisecond)

	stopped := make(chan struct{})
	go func() {
		cm.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("这一轮还没结束，Stop 不应返回")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("这一轮结束后 Stop 应返回")
	}
}
