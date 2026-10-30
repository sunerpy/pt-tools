package scheduler

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/internal/reseed"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
)

const (
	reseedTick         = 10 * time.Minute
	reseedStartupDelay = 3 * time.Minute
	// reseedRunTimeout 是一轮辅种的时限（暂停的时间不算；站点下载种子受限速约束，可能比较慢）
	reseedRunTimeout = 30 * time.Minute
	// reseedMaxPause 是一次暂停最长多久：超过就停下这一轮，免得忘了继续以后定时的轮次一直跑不了
	reseedMaxPause = 24 * time.Hour
)

var (
	// ErrReseedBusy 表示已经有一轮辅种在跑。
	ErrReseedBusy = errors.New("辅种正在运行，请等这一轮结束")
	// ErrReseedNotCurrent 表示要暂停、继续或停止的不是正在跑的那一轮。
	ErrReseedNotCurrent = errors.New("这一轮已经结束，或者不是正在运行的那一轮，刷新后再试")
)

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
	// RunTimeout 是一轮的时限（默认 30 分钟，暂停的时间不算）；MaxPause 是一次暂停最长多久（默认 24 小时）。
	RunTimeout time.Duration
	MaxPause   time.Duration
	// NewTimer 是计时器（测试里换成可控的），默认 time.NewTimer。
	NewTimer func(d time.Duration) reseed.Timer
}

// ReseedWorker 按设置的间隔运行辅种；「立即运行」「继续」「重试」在后台跑一轮，同一时间只有一轮。
// 正在跑的那一轮可以暂停、继续、停止。
type ReseedWorker struct {
	cfg ReseedWorkerConfig
	// created 是后台建立的时间：早于它开始、还写着进行中的轮次是上一个进程留下的
	created time.Time

	mu      sync.Mutex
	running bool
	stopped bool
	cur     *reseedActive
	root    context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// reseedActive 是正在跑的那一轮。id 为 0 时还在写轮次记录。
type reseedActive struct {
	id            uint
	trigger       string
	from          *uint
	started       time.Time
	ctrl          *reseed.Control
	cancel        context.CancelCauseFunc
	progress      reseed.RunResult
	cancelPending bool
}

// ReseedCurrentRun 是正在跑的那一轮（页面上的实时进度）。
type ReseedCurrentRun struct {
	ID            uint       `json:"id"`
	Trigger       string     `json:"trigger"`
	ContinuedFrom *uint      `json:"continued_from,omitempty"`
	Status        string     `json:"status"`
	StartedAt     time.Time  `json:"started_at"`
	PausedAt      *time.Time `json:"paused_at,omitempty"`
	Hashes        int        `json:"hashes"`
	Batches       int        `json:"batches"`
	BatchesDone   int        `json:"batches_done"`
	Candidates    int        `json:"candidates"`
	Created       int        `json:"created"`
	Failed        int        `json:"failed"`
	Gone          int        `json:"gone"`
	Skipped       int        `json:"skipped"`
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
	if cfg.RunTimeout <= 0 {
		cfg.RunTimeout = reseedRunTimeout
	}
	if cfg.MaxPause <= 0 {
		cfg.MaxPause = reseedMaxPause
	}
	root, cancel := context.WithCancel(context.Background())
	return &ReseedWorker{cfg: cfg, created: cfg.Clock.Now(), root: root, cancel: cancel}
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

// Stop 停止循环并等正在跑的一轮退出（这一轮记成没跑完，下一次接着查）。停止后不能再启动。
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

// Running 报告是不是有一轮在跑（含暂停中的）。
func (w *ReseedWorker) Running() bool {
	if w == nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cur != nil
}

// Current 返回正在跑的那一轮；没有时为 nil。
func (w *ReseedWorker) Current() *ReseedCurrentRun {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	a := w.cur
	if a == nil || a.id == 0 {
		w.mu.Unlock()
		return nil
	}
	p := a.progress
	out := &ReseedCurrentRun{
		ID: a.id, Trigger: a.trigger, ContinuedFrom: a.from, Status: models.ReseedRunRunning, StartedAt: a.started,
		Hashes: p.Hashes, Batches: p.Batches, BatchesDone: p.BatchesDone, Candidates: p.Candidates,
		Created: p.Created, Failed: p.Failed, Gone: p.Gone, Skipped: p.SkippedTotal(),
	}
	ctrl := a.ctrl
	w.mu.Unlock()
	if paused, since, _ := ctrl.State(); paused {
		out.Status, out.PausedAt = models.ReseedRunPaused, &since
	}
	return out
}

func (w *ReseedWorker) loop() {
	if w.cfg.Service != nil {
		if runs, atts, err := w.cfg.Service.MarkInterrupted(w.root, w.created); err != nil {
			w.cfg.Logger.Warnf("[辅种] %v", err)
		} else if runs+atts > 0 {
			w.cfg.Logger.Infof("[辅种] 上次退出时有 %d 轮、%d 次尝试没有完成，已记为被打断", runs, atts)
		}
	}
	select {
	case <-w.root.Done():
		return
	case <-time.After(w.cfg.StartupDelay):
	}
	ticker := time.NewTicker(w.cfg.Tick)
	defer ticker.Stop()
	for {
		if w.cfg.Service != nil && w.cfg.Service.Due(w.root) {
			_, err := w.start(models.ReseedTriggerSchedule, w.cfg.Service.ContinuePoint, w.cfg.Service.Run)
			if err != nil && !errors.Is(err, ErrReseedBusy) {
				w.cfg.Logger.Warnf("[辅种] 定时运行没有开始: %v", err)
			}
		}
		select {
		case <-w.root.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunNow 在后台马上从头跑一轮（不等它结束），返回轮次编号；已经有一轮在跑时返回 ErrReseedBusy，
// IYUU 限流冷却中或今天的请求到了上限时返回对应的错误（不开始）。
func (w *ReseedWorker) RunNow() (uint, error) {
	if w == nil || w.cfg.Service == nil {
		return 0, errors.New("辅种服务没有启动")
	}
	svc := w.cfg.Service
	return w.start(models.ReseedTriggerManual, func(ctx context.Context) (string, *uint, error) {
		return "", nil, svc.IYUUReady(ctx)
	}, svc.Run)
}

// Continue 接着 id 这一轮往下查（只有最近一个提前停下的轮次可以），返回新轮次的编号。
func (w *ReseedWorker) Continue(id uint) (uint, error) {
	if w == nil || w.cfg.Service == nil {
		return 0, errors.New("辅种服务没有启动")
	}
	return w.start(models.ReseedTriggerContinue, func(ctx context.Context) (string, *uint, error) {
		after, err := w.cfg.Service.CanContinue(ctx, id)
		if err != nil {
			return "", nil, err
		}
		if err := w.cfg.Service.IYUUReady(ctx); err != nil {
			return "", nil, err
		}
		from := id
		return after, &from, nil
	}, w.cfg.Service.Run)
}

// Retry 在后台重试选中的辅种记录（触发方式为 retry 的一轮，不查 IYUU），返回轮次编号。
// 选中的记录都不能重试时返回 reseed.ErrNothingToRetry；已经有一轮在跑时返回 ErrReseedBusy。
func (w *ReseedWorker) Retry(ids []uint) (uint, error) {
	if w == nil || w.cfg.Service == nil {
		return 0, errors.New("辅种服务没有启动")
	}
	svc := w.cfg.Service
	return w.start(models.ReseedTriggerRetry, func(ctx context.Context) (string, *uint, error) {
		return "", nil, svc.CheckRetry(ctx, ids)
	}, func(ctx context.Context, opts reseed.RunOptions) (reseed.RunResult, error) {
		return svc.Retry(ctx, opts, ids)
	})
}

// reseedPlan 在占住「正在跑」以后算出这一轮的起点（与别的轮次不会交错）。
type reseedPlan func(ctx context.Context) (after string, from *uint, err error)

// reseedExec 是这一轮要跑的东西（普通轮次是 Service.Run）。
type reseedExec func(ctx context.Context, opts reseed.RunOptions) (reseed.RunResult, error)

// start 占住「正在跑」、写轮次记录，然后在后台跑这一轮；已经在跑或已经停止时返回错误。
func (w *ReseedWorker) start(trigger string, plan reseedPlan, exec reseedExec) (uint, error) {
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return 0, errors.New("辅种服务已停止")
	}
	if w.cur != nil {
		w.mu.Unlock()
		return 0, ErrReseedBusy
	}
	a := &reseedActive{trigger: trigger, started: w.cfg.Clock.Now()}
	w.cur = a
	w.wg.Add(1)
	w.mu.Unlock()
	release := func() {
		w.mu.Lock()
		if w.cur == a {
			w.cur = nil
		}
		w.mu.Unlock()
		w.wg.Done()
	}
	after, from, err := plan(w.root)
	if err != nil {
		release()
		return 0, err
	}
	run, err := w.cfg.Service.StartRun(w.root, trigger, after, from)
	if err != nil {
		release()
		return 0, err
	}
	ctx, cancel := context.WithCancelCause(w.root)
	ctrl := reseed.NewControl(reseed.ControlConfig{
		Budget: w.cfg.RunTimeout, MaxPause: w.cfg.MaxPause, Cancel: cancel, Now: w.cfg.Clock.Now, NewTimer: w.cfg.NewTimer,
	})
	w.mu.Lock()
	a.id, a.from, a.started, a.ctrl, a.cancel = run.ID, from, run.StartedAt, ctrl, cancel
	w.mu.Unlock()
	ctrl.Start(ctx)
	go w.execute(ctx, a, after, exec)
	return run.ID, nil
}

// execute 跑这一轮并记下结果；调用前必须已经 start。
func (w *ReseedWorker) execute(ctx context.Context, a *reseedActive, after string, exec reseedExec) {
	defer w.wg.Done()
	defer func() {
		w.mu.Lock()
		if w.cur == a {
			w.cur = nil
		}
		w.mu.Unlock()
		a.cancel(nil)
	}()
	svc := w.cfg.Service
	if err := svc.PruneRuns(ctx); err != nil {
		w.cfg.Logger.Warnf("[辅种] %v", err)
	}
	opts := reseed.RunOptions{RunID: a.id, After: after, Control: a.ctrl, Progress: func(r reseed.RunResult) {
		w.mu.Lock()
		a.progress = r
		w.mu.Unlock()
	}}
	res, err := exec(ctx, opts)
	status, errText := reseedRunStatus(ctx, res, err)
	summary := res.Summary()
	if err != nil {
		summary = "运行失败：" + errText
		if status == models.ReseedRunLimited {
			summary = "提前停下：" + errText
		}
		w.cfg.Logger.Warnf("[辅种] 第 %d 轮：%s", a.id, errText)
	} else {
		w.cfg.Logger.Infof("[辅种] 第 %d 轮：%s", a.id, summary)
	}
	_, _, paused := a.ctrl.State()
	done := context.WithoutCancel(ctx)
	if ferr := svc.FinishRun(done, a.id, res, status, errText, paused); ferr != nil {
		w.cfg.Logger.Warnf("[辅种] %v", ferr)
	}
	// 重试轮次不查 IYUU：不算进定时运行的间隔，「上次运行」也只写普通轮次
	if a.trigger != models.ReseedTriggerRetry {
		if rerr := svc.RecordRun(done, w.cfg.Clock.Now(), summary); rerr != nil {
			w.cfg.Logger.Warnf("[辅种] 记录结果失败: %v", rerr)
		}
	}
	w.mu.Lock()
	cancelPending := a.cancelPending
	w.mu.Unlock()
	if cancelPending {
		ids, jerr := svc.RunJobsWaiting(done, a.id)
		if jerr != nil {
			w.cfg.Logger.Warnf("[辅种] %v", jerr)
		} else if n := svc.CancelJobs(done, ids); n > 0 {
			w.cfg.Logger.Infof("[辅种] 第 %d 轮停止时取消了 %d 个还没加入下载器的任务", a.id, n)
		}
	}
	if res.Created > 0 && w.cfg.Transfer != nil {
		w.cfg.Transfer.Trigger()
	}
}

// reseedRunStatus 是这一轮的结果状态与原因。
func reseedRunStatus(ctx context.Context, res reseed.RunResult, err error) (string, string) {
	if err != nil && ctx.Err() != nil {
		// 还没开始查就被停下了（读站点列表、报告站点时）
		switch cause := context.Cause(ctx); {
		case errors.Is(cause, reseed.ErrRunStopped), errors.Is(cause, reseed.ErrPausedTooLong):
			return models.ReseedRunStopped, cause.Error()
		case errors.Is(cause, reseed.ErrRunTimeout):
			return models.ReseedRunLimited, cause.Error()
		default:
			return models.ReseedRunInterrupted, "pt-tools 正在退出，这一轮没有跑完"
		}
	}
	if err != nil {
		if reseed.IYUULimited(err) {
			// 读站点列表、报告站点时遇到限流：提前停下，下一轮接着从这一轮的起点查
			return models.ReseedRunLimited, err.Error()
		}
		return models.ReseedRunFailed, err.Error()
	}
	switch res.End {
	case reseed.EndStopped:
		return models.ReseedRunStopped, res.Stopped
	case reseed.EndLimited:
		return models.ReseedRunLimited, res.Stopped
	case reseed.EndShutdown:
		return models.ReseedRunInterrupted, res.Stopped
	}
	if len(res.Errors) > 0 {
		return models.ReseedRunPartial, res.Errors[0]
	}
	return models.ReseedRunOK, ""
}

// active 返回编号为 id 的正在跑的那一轮。
func (w *ReseedWorker) active(id uint) (*reseedActive, error) {
	if w == nil {
		return nil, ErrReseedNotCurrent
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if a := w.cur; a != nil && a.id == id && a.id != 0 {
		return a, nil
	}
	return nil, ErrReseedNotCurrent
}

// Pause 暂停 id 这一轮（正在下载的那一个做完以后停下）；已经暂停时什么都不做。
func (w *ReseedWorker) Pause(id uint) error {
	a, err := w.active(id)
	if err != nil {
		return err
	}
	if !a.ctrl.Pause() {
		return nil
	}
	_, since, total := a.ctrl.State()
	w.cfg.Logger.Infof("[辅种] 第 %d 轮已暂停", id)
	return w.cfg.Service.SetRunPaused(w.root, id, true, since, total)
}

// Resume 继续 id 这一轮；没在暂停时什么都不做。
func (w *ReseedWorker) Resume(id uint) error {
	a, err := w.active(id)
	if err != nil {
		return err
	}
	d, ok := a.ctrl.Resume()
	if !ok {
		return nil
	}
	_, _, total := a.ctrl.State()
	w.cfg.Logger.Infof("[辅种] 第 %d 轮暂停了 %s 以后继续", id, d.Round(time.Second))
	return w.cfg.Service.SetRunPaused(w.root, id, false, time.Time{}, total)
}

// StopRun 停止 id 这一轮；cancelPending 时这一轮结束以后再取消它建的、还没加入下载器的任务。
func (w *ReseedWorker) StopRun(id uint, cancelPending bool) error {
	a, err := w.active(id)
	if err != nil {
		return err
	}
	w.mu.Lock()
	a.cancelPending = a.cancelPending || cancelPending
	w.mu.Unlock()
	a.cancel(reseed.ErrRunStopped)
	w.cfg.Logger.Infof("[辅种] 第 %d 轮被停止", id)
	return nil
}
