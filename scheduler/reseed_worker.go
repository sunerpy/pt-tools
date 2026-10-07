package scheduler

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/internal/reseed"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
)

const (
	reseedTick         = 10 * time.Minute
	reseedStartupDelay = 3 * time.Minute
	// reseedRunTimeout 是一轮辅种的总预算（站点下载种子受限速约束，可能比较慢）
	reseedRunTimeout = 30 * time.Minute
)

// ErrReseedBusy 表示已经有一轮辅种在跑。
var ErrReseedBusy = errors.New("辅种正在运行，请等这一轮结束")

// ReseedWorkerConfig 是辅种后台的依赖。
type ReseedWorkerConfig struct {
	Service *reseed.Service
	// Transfer 在建了新的辅种任务后唤醒，让任务马上开始加入与校验
	Transfer *TransferWorker
	Clock    sitelogin.Clock
	Logger   *zap.SugaredLogger
	Tick     time.Duration
	// StartupDelay 是启动后第一次检查前的等待（默认 3 分钟）。
	StartupDelay time.Duration
}

// ReseedWorker 按设置的间隔运行辅种；「立即运行」在后台跑一轮，同一时间只有一轮。
type ReseedWorker struct {
	cfg ReseedWorkerConfig

	mu      sync.Mutex
	running bool
	busy    bool
	stopped bool
	root    context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewReseedWorker 构造后台，不启动。
func NewReseedWorker(cfg ReseedWorkerConfig) *ReseedWorker {
	if cfg.Clock == nil {
		cfg.Clock = sitelogin.NewRealClock()
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop().Sugar()
	}
	if cfg.Tick <= 0 {
		cfg.Tick = reseedTick
	}
	if cfg.StartupDelay <= 0 {
		cfg.StartupDelay = reseedStartupDelay
	}
	root, cancel := context.WithCancel(context.Background())
	return &ReseedWorker{cfg: cfg, root: root, cancel: cancel}
}

// Service 返回辅种服务。
func (w *ReseedWorker) Service() *reseed.Service {
	if w == nil {
		return nil
	}
	return w.cfg.Service
}

// Start 启动定时循环；重复调用无效果。
func (w *ReseedWorker) Start() {
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

// Stop 停止循环并等正在跑的一轮（包括「立即运行」）退出。停止后不能再启动。
func (w *ReseedWorker) Stop() {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.running, w.stopped = false, true
	w.mu.Unlock()
	w.cancel()
	w.wg.Wait()
}

// Running 报告是不是有一轮在跑。
func (w *ReseedWorker) Running() bool {
	if w == nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.busy
}

func (w *ReseedWorker) loop() {
	select {
	case <-w.root.Done():
		return
	case <-time.After(w.cfg.StartupDelay):
	}
	ticker := time.NewTicker(w.cfg.Tick)
	defer ticker.Stop()
	for {
		if w.cfg.Service != nil && w.cfg.Service.Due(w.root) {
			if err := w.claim(); err == nil {
				w.runOne()
			}
		}
		select {
		case <-w.root.Done():
			return
		case <-ticker.C:
		}
	}
}

// claim 占住「正在跑」并把这一轮记进 wg（与 Stop 在同一把锁下，停止之后不会再记）；已经在跑或已经停止时返回错误。
func (w *ReseedWorker) claim() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return errors.New("辅种服务已停止")
	}
	if w.busy {
		return ErrReseedBusy
	}
	w.busy = true
	w.wg.Add(1)
	return nil
}

// RunNow 在后台马上跑一轮（不等它结束）；已经有一轮在跑时返回 ErrReseedBusy。
func (w *ReseedWorker) RunNow() error {
	if w == nil || w.cfg.Service == nil {
		return errors.New("辅种服务没有启动")
	}
	if err := w.claim(); err != nil {
		return err
	}
	go w.runOne()
	return nil
}

// runOne 跑一轮并记下结果；调用前必须已经 claim。
func (w *ReseedWorker) runOne() {
	defer w.wg.Done()
	defer func() {
		w.mu.Lock()
		w.busy = false
		w.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(w.root, reseedRunTimeout)
	defer cancel()
	res, err := w.cfg.Service.Run(ctx)
	summary := res.Summary()
	if err != nil {
		summary = "运行失败：" + err.Error()
		w.cfg.Logger.Warnf("[辅种] %v", err)
	} else {
		w.cfg.Logger.Infof("[辅种] %s", summary)
	}
	if rerr := w.cfg.Service.RecordRun(ctx, w.cfg.Clock.Now(), summary); rerr != nil {
		w.cfg.Logger.Warnf("[辅种] 记录结果失败: %v", rerr)
	}
	if res.Created > 0 {
		w.cfg.Transfer.Trigger()
	}
}
