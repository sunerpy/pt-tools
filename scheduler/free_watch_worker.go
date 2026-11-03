package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/internal/freewatch"
)

const (
	freeWatchTick = 5 * time.Minute
	// freeWatchRunTimeout 是一轮的总预算（每个站点最多读 10 个种子，读不完的下一轮接着读）
	freeWatchRunTimeout = 4 * time.Minute
)

// FreeWatchWorker 定时检查等免费的种子（见 internal/freewatch）：每 5 分钟一轮，新建以后可以 Trigger 马上跑一轮。
type FreeWatchWorker struct {
	svc     *freewatch.Service
	tick    time.Duration
	trigger chan struct{}

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	runMu   sync.Mutex // 同一时间只跑一轮
}

// NewFreeWatchWorker 构造后台，不启动。tick 不大于 0 时是 5 分钟。
func NewFreeWatchWorker(svc *freewatch.Service, tick time.Duration) *FreeWatchWorker {
	if tick <= 0 {
		tick = freeWatchTick
	}
	return &FreeWatchWorker{svc: svc, tick: tick, trigger: make(chan struct{}, 1)}
}

// Service 返回等免费服务。
func (w *FreeWatchWorker) Service() *freewatch.Service {
	if w == nil {
		return nil
	}
	return w.svc
}

// Start 启动循环；重复调用无效果。
func (w *FreeWatchWorker) Start() {
	if w == nil || w.svc == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel, w.running = cancel, true
	w.wg.Go(func() { w.loop(ctx) })
}

// Stop 停止循环并等正在跑的一轮结束。
func (w *FreeWatchWorker) Stop() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	cancel := w.cancel
	w.mu.Unlock()
	cancel()
	w.wg.Wait()
}

// Trigger 让循环马上跑一轮（不等下一个间隔）；不阻塞。
func (w *FreeWatchWorker) Trigger() {
	if w == nil {
		return
	}
	select {
	case w.trigger <- struct{}{}:
	default:
	}
}

func (w *FreeWatchWorker) loop(ctx context.Context) {
	ticker := time.NewTicker(w.tick)
	defer ticker.Stop()
	for {
		w.RunOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-w.trigger:
		}
	}
}

// RunOnce 跑一轮检查，返回读了几个种子。
func (w *FreeWatchWorker) RunOnce(ctx context.Context) int {
	if w == nil || w.svc == nil {
		return 0
	}
	w.runMu.Lock()
	defer w.runMu.Unlock()
	if ctx.Err() != nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(ctx, freeWatchRunTimeout)
	defer cancel()
	return w.svc.RunOnce(ctx)
}
