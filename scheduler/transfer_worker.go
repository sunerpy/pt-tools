package scheduler

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/internal/transfer"
	"github.com/sunerpy/pt-tools/models"
)

const (
	transferTick = 15 * time.Second
	// transferRunTimeout 是一轮（推进全部任务、跑到期的规则）的总预算
	transferRunTimeout = 5 * time.Minute
)

// TransferWorkerConfig 是转移做种后台的依赖。
type TransferWorkerConfig struct {
	Service *transfer.Service
	DB      *gorm.DB
	Clock   sitelogin.Clock
	Logger  *zap.SugaredLogger
	Tick    time.Duration
}

// TransferWorker 定时推进转移做种任务，并运行到期的定时规则。建了新任务后可以 Trigger 立即跑一轮。
type TransferWorker struct {
	cfg     TransferWorkerConfig
	trigger chan struct{}

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	runMu   sync.Mutex // 同一时间只跑一轮
	// rulesSem 让后台的定时运行与「立即运行」轮流跑规则：同一时间只有一处在挑种子建任务
	rulesSem chan struct{}
}

// TransferRuleRun 是「立即运行」一条规则的结果；RunError 不为空表示这一轮没跑完（下载器不可用等）。
type TransferRuleRun struct {
	Result   transfer.RuleResult `json:"result"`
	Summary  string              `json:"summary"`
	RunError string              `json:"error,omitempty"`
}

// NewTransferWorker 构造后台，不启动。
func NewTransferWorker(cfg TransferWorkerConfig) *TransferWorker {
	if cfg.Clock == nil {
		cfg.Clock = sitelogin.NewRealClock()
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop().Sugar()
	}
	if cfg.Tick <= 0 {
		cfg.Tick = transferTick
	}
	return &TransferWorker{cfg: cfg, trigger: make(chan struct{}, 1), rulesSem: make(chan struct{}, 1)}
}

// Service 返回转移做种服务。
func (w *TransferWorker) Service() *transfer.Service {
	if w == nil {
		return nil
	}
	return w.cfg.Service
}

// Start 启动循环；重复调用无效果。
func (w *TransferWorker) Start() {
	if w == nil {
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
func (w *TransferWorker) Stop() {
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
func (w *TransferWorker) Trigger() {
	if w == nil {
		return
	}
	select {
	case w.trigger <- struct{}{}:
	default:
	}
}

func (w *TransferWorker) loop(ctx context.Context) {
	ticker := time.NewTicker(w.cfg.Tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-w.trigger:
		}
		w.RunOnce(ctx)
	}
}

// RunOnce 推进全部未结束的任务一步，再跑到期的定时规则；返回状态有变化的任务数与新建的任务数。
func (w *TransferWorker) RunOnce(ctx context.Context) (stepped, created int) {
	if w == nil || w.cfg.Service == nil {
		return 0, 0
	}
	w.runMu.Lock()
	defer w.runMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, transferRunTimeout)
	defer cancel()
	stepped = w.cfg.Service.RunOnce(ctx)
	created = w.runRules(ctx)
	if created > 0 {
		// 新建的任务马上导出，不用等下一个间隔
		stepped += w.cfg.Service.RunOnce(ctx)
	}
	return stepped, created
}

// lockRules 占住规则运行；ctx 先到期时返回 false。
func (w *TransferWorker) lockRules(ctx context.Context) bool {
	select {
	case w.rulesSem <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (w *TransferWorker) unlockRules() { <-w.rulesSem }

// RunRuleNow 立即运行一条规则并记下结果；与后台的定时运行轮流进行，不会同时挑种子。
func (w *TransferWorker) RunRuleNow(ctx context.Context, id uint) (TransferRuleRun, error) {
	if w == nil || w.cfg.Service == nil {
		return TransferRuleRun{}, errors.New("转移做种服务没有启动")
	}
	if !w.lockRules(ctx) {
		return TransferRuleRun{}, ctx.Err()
	}
	defer w.unlockRules()
	rule, err := w.cfg.Service.GetRule(ctx, id)
	if err != nil {
		return TransferRuleRun{}, err
	}
	res, runErr := w.cfg.Service.RunRule(ctx, rule)
	out := TransferRuleRun{Result: res, Summary: res.Summary()}
	if runErr != nil {
		out.RunError = runErr.Error()
		out.Summary = "运行失败：" + runErr.Error()
	}
	if err := w.cfg.Service.RecordRuleRun(ctx, id, w.cfg.Clock.Now(), out.Summary); err != nil {
		return out, err
	}
	if res.Created > 0 {
		w.Trigger()
	}
	return out, nil
}

// runRules 运行到期的规则，记下运行时间和结果。
func (w *TransferWorker) runRules(ctx context.Context) int {
	if w.cfg.DB == nil {
		return 0
	}
	if !w.lockRules(ctx) {
		return 0
	}
	defer w.unlockRules()
	var rules []models.TransferRule
	if err := w.cfg.DB.WithContext(ctx).Where("enabled = ?", true).Order("id").Find(&rules).Error; err != nil {
		w.cfg.Logger.Warnf("[转移做种] 读取规则失败: %v", err)
		return 0
	}
	created := 0
	for _, r := range rules {
		if ctx.Err() != nil {
			break
		}
		now := w.cfg.Clock.Now()
		if !transfer.RuleDue(r, now) {
			continue
		}
		res, err := w.cfg.Service.RunRule(ctx, r)
		summary := res.Summary()
		if err != nil {
			summary = "运行失败：" + err.Error()
			w.cfg.Logger.Warnf("[转移做种] 规则 %s 运行失败: %v", r.Name, err)
		}
		created += res.Created
		if err := w.cfg.DB.WithContext(context.WithoutCancel(ctx)).Model(&models.TransferRule{}).Where("id = ?", r.ID).
			Updates(map[string]any{"last_run_at": now, "last_result": summary}).Error; err != nil {
			w.cfg.Logger.Warnf("[转移做种] 记录规则 %s 的运行结果失败: %v", r.Name, err)
		}
	}
	return created
}
