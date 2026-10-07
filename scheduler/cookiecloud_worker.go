package scheduler

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/internal/cookiecloud"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
)

const (
	cookieCloudTick         = 10 * time.Minute
	cookieCloudStartupDelay = 2 * time.Minute
	cookieCloudRunTimeout   = 2 * time.Minute
)

// CookieCloudWorkerConfig 是 CookieCloud 定时同步的依赖。
type CookieCloudWorkerConfig struct {
	Service *cookiecloud.Service
	Clock   sitelogin.Clock
	Logger  *zap.SugaredLogger
	Tick    time.Duration
	// StartupDelay 是启动后第一次检查前的等待（默认 2 分钟）。
	StartupDelay time.Duration
}

// CookieCloudWorker 按设置的间隔从 CookieCloud 同步已启用站点的 Cookie。
type CookieCloudWorker struct {
	cfg CookieCloudWorkerConfig

	mu      sync.Mutex
	running bool
	stopped bool
	root    context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewCookieCloudWorker 构造后台，不启动。
func NewCookieCloudWorker(cfg CookieCloudWorkerConfig) *CookieCloudWorker {
	if cfg.Clock == nil {
		cfg.Clock = sitelogin.NewRealClock()
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop().Sugar()
	}
	if cfg.Tick <= 0 {
		cfg.Tick = cookieCloudTick
	}
	if cfg.StartupDelay <= 0 {
		cfg.StartupDelay = cookieCloudStartupDelay
	}
	root, cancel := context.WithCancel(context.Background())
	return &CookieCloudWorker{cfg: cfg, root: root, cancel: cancel}
}

// Service 返回 CookieCloud 服务。
func (w *CookieCloudWorker) Service() *cookiecloud.Service {
	if w == nil {
		return nil
	}
	return w.cfg.Service
}

// Start 启动定时循环；重复调用无效果。
func (w *CookieCloudWorker) Start() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running || w.stopped {
		return
	}
	w.running = true
	w.wg.Go(w.loop)
}

// Stop 停止循环并等正在进行的同步退出。停止后不能再启动。
func (w *CookieCloudWorker) Stop() {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.running, w.stopped = false, true
	w.mu.Unlock()
	w.cancel()
	w.wg.Wait()
}

func (w *CookieCloudWorker) loop() {
	select {
	case <-w.root.Done():
		return
	case <-time.After(w.cfg.StartupDelay):
	}
	ticker := time.NewTicker(w.cfg.Tick)
	defer ticker.Stop()
	for {
		if w.cfg.Service != nil && w.cfg.Service.Due(w.root) {
			w.syncOnce()
		}
		select {
		case <-w.root.Done():
			return
		case <-ticker.C:
		}
	}
}

// syncOnce 同步一次并记下结果。手动预览或导入正在进行时跳过，下一次再来。
func (w *CookieCloudWorker) syncOnce() {
	ctx, cancel := context.WithTimeout(w.root, cookieCloudRunTimeout)
	defer cancel()
	res, err := w.cfg.Service.Sync(ctx)
	if errors.Is(err, cookiecloud.ErrBusy) || w.root.Err() != nil {
		return
	}
	summary := res.Summary()
	if err != nil {
		summary = "同步失败：" + err.Error()
		w.cfg.Logger.Warnf("[CookieCloud] %v", err)
	}
	if rerr := w.cfg.Service.RecordSync(ctx, w.cfg.Clock.Now(), summary); rerr != nil {
		w.cfg.Logger.Warnf("[CookieCloud] 记录结果失败: %v", rerr)
	}
}
