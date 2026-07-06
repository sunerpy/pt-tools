package cmd

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// **Validates: Requirements 1.1, 2.1, 2.2**

// blockingReloader 是一个 startupReloader 实现，其 Reload 会阻塞直到 release
// 被关闭，用于模拟下载器不可达时 mgr.Reload 长时间卡在 createWithRetry 的场景。
type blockingReloader struct {
	started     chan struct{}
	release     chan struct{}
	reloadCount atomic.Int32
	lastCfg     atomic.Pointer[models.Config]
}

func newBlockingReloader() *blockingReloader {
	return &blockingReloader{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
}

func (r *blockingReloader) Reload(cfg *models.Config) {
	r.lastCfg.Store(cfg)
	select {
	case r.started <- struct{}{}:
	default:
	}
	<-r.release
	r.reloadCount.Add(1)
}

func (r *blockingReloader) getReloadCount() int {
	return int(r.reloadCount.Load())
}

func autoStartConfig(dir string) *models.Config {
	global.InitLogger(zap.NewNop())
	cfg := &models.Config{}
	cfg.Global.AutoStart = true
	cfg.Global.DownloadDir = dir
	return cfg
}

// TestStartupAutoReload_DispatchesAsynchronously 驱动真实的 maybeAutoStartReload，
// 注入一个会一直阻塞的 reloader，验证该函数不会等待 Reload 完成即快速返回，
// 从而证明自动启动重载是异步派发（对应生产代码 `go mgr.Reload(cfg)`）。
//
// 若生产代码退回同步 `mgr.Reload(cfg)`，maybeAutoStartReload 会阻塞在
// blockingReloader.Reload 里（release 永不关闭），下面的返回耗时断言即失败。
func TestStartupAutoReload_DispatchesAsynchronously(t *testing.T) {
	const blockTimeout = 10 * time.Second

	reloader := newBlockingReloader()
	cfg := autoStartConfig("/data/downloads")

	done := make(chan bool, 1)
	start := time.Now()
	go func() {
		done <- maybeAutoStartReload(reloader, cfg)
	}()

	select {
	case dispatched := <-done:
		elapsed := time.Since(start)
		assert.True(t, dispatched, "AutoStart=true 且目录非空时应派发重载")
		assert.Less(t, elapsed, time.Second,
			"maybeAutoStartReload 必须异步派发并立即返回；同步实现会阻塞至 release 关闭")
	case <-time.After(blockTimeout):
		close(reloader.release)
		t.Fatalf("maybeAutoStartReload 未在 %v 内返回：重载被同步执行，端口将被阻塞", blockTimeout)
	}

	// 确认异步的 Reload 确实被启动，然后放行让其结束。
	select {
	case <-reloader.started:
	case <-time.After(2 * time.Second):
		close(reloader.release)
		t.Fatal("异步 Reload 未被调用")
	}
	close(reloader.release)

	require.Eventually(t, func() bool {
		return reloader.getReloadCount() == 1
	}, 2*time.Second, 10*time.Millisecond, "后台 Reload 应最终完成一次")
}

// TestStartupAutoReload_PortOpensDespiteBlockingReload 是一个更贴近集成的用例：
// 在 maybeAutoStartReload 之后立即模拟 srv.Serve 打开监听端口，验证即便重载一直
// 阻塞，端口仍能迅速就绪。同步实现下 Listen 会被推迟到重载结束之后。
func TestStartupAutoReload_PortOpensDespiteBlockingReload(t *testing.T) {
	reloader := newBlockingReloader()
	cfg := autoStartConfig("/data/downloads")
	defer close(reloader.release)

	addr := "127.0.0.1:0"
	ready := make(chan string, 1)

	go func() {
		maybeAutoStartReload(reloader, cfg)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			ready <- ""
			return
		}
		defer ln.Close()
		ready <- ln.Addr().String()
		time.Sleep(500 * time.Millisecond)
	}()

	select {
	case got := <-ready:
		require.NotEmpty(t, got, "监听端口应成功打开")
	case <-time.After(3 * time.Second):
		t.Fatal("端口未在 3s 内就绪：重载同步阻塞了 srv.Serve")
	}
}
