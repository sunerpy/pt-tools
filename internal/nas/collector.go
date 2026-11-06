package nas

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/internal/redact"
)

// LocalID 是本机来源在列表与设置里的编号（数据库里的来源是数字编号）。
const LocalID = "local"

// LocalKind 是本机来源的类型名。
const LocalKind = "local"

// 来源的状态（Status.State）。
const (
	StateOK            = "ok"
	StateConnecting    = "connecting"
	StateError         = "error"
	StateAuthFailed    = "auth_failed"
	StateCertUntrusted = "cert_untrusted"
	StateCertChanged   = "cert_changed"
	StateDisabled      = "disabled"
	StateUnsupported   = "unsupported"
)

const (
	defaultLight   = 30 * time.Second
	defaultFull    = 5 * time.Minute
	defaultTimeout = 10 * time.Second
	// HistoryLen 是内存里留几拍历史（30 秒一拍，约 30 分钟）
	HistoryLen = 60
)

// defaultBackoff 是网络、超时与协议错误后的重试间隔：30 秒、1、2、5 分钟，之后一直 5 分钟；成功一次清零。
var defaultBackoff = []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute}

// Status 是一个来源现在的情况（只读内存）：State 见 State* 常量，Message 是给人看的原因（已去掉凭证），
// Snapshot 是最近一次成功采集合并出的数据（出错时还是上一份，界面写「这是 N 分钟前的数据」），History 是最近的几拍。
type Status struct {
	ID           string
	Name         string
	Kind         string
	Enabled      bool
	Experimental bool
	URL          string
	HasSecret    bool
	PinSHA256    string
	InsecureAck  bool
	State        string
	Message      string
	LastOKAt     *time.Time
	NextTryAt    *time.Time
	Snapshot     *Snapshot
	History      []Sample
}

// Collector 定时采集各个来源：每个启用的来源一个循环，第一次完整采集，之后每 30 秒轻量采集、每 5 分钟完整采集，
// 结果合并成最新快照并留最近 60 拍的历史。网络等错误按 30 秒、1、2、5 分钟退避；认证失败、证书不受信任或换了以后
// 停住（熔断），直到这个来源的设置改了（Reload 带来新的 revision）或用户点「重新连接」（Retry，只试一次）。
//
// Load 读全部设置（含停用的来源）；SaveExtra 把来源刷新过的长期凭据写回（只在库里的 revision 没变时写）；
// Factory 建一个来源（为空时按注册表的 Kind.New）；NewLocal 建本机来源（为空时 NewLocalSource）。
type Collector struct {
	Load      func(ctx context.Context) (Config, error)
	SaveExtra func(ctx context.Context, id uint, revision int64, extra map[string]string) error
	Factory   func(cfg SourceConfig) (Source, error)
	NewLocal  func() Source
	Now       func() time.Time
	Logf      func(format string, args ...any)
	// Light、Full、Timeout、Backoff 为零时用默认值（30 秒、5 分钟、10 秒、30 秒/1/2/5 分钟）；测试里调小
	Light, Full, Timeout time.Duration
	Backoff              []time.Duration

	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	stopped bool
	entries map[string]*entry
	order   []string
	wg      sync.WaitGroup
}

// entry 是一个来源的循环与它的状态。设置变了就换一个新的 entry：旧循环写的是旧 entry，不会盖掉新的。
type entry struct {
	id           string
	cfg          SourceConfig
	experimental bool

	mu      sync.Mutex
	state   string
	message string
	lastOK  *time.Time
	nextTry *time.Time
	snap    *Snapshot
	history []Sample

	cancel context.CancelFunc
	retry  chan chan struct{}
}

func (c *Collector) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Collector) light() time.Duration { return orDefault(c.Light, defaultLight) }
func (c *Collector) full() time.Duration  { return orDefault(c.Full, defaultFull) }

func (c *Collector) timeout() time.Duration { return orDefault(c.Timeout, defaultTimeout) }

func orDefault(d, def time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return def
}

func (c *Collector) backoff(failures int) time.Duration {
	steps := c.Backoff
	if len(steps) == 0 {
		steps = defaultBackoff
	}
	if failures >= len(steps) {
		return steps[len(steps)-1]
	}
	return steps[failures]
}

func (c *Collector) logf(format string, args ...any) {
	if c.Logf != nil {
		c.Logf(format, args...)
	}
}

// Start 读设置并启动各个来源的循环；ctx 结束或 Stop 以后循环退出。
func (c *Collector) Start(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ctx != nil || c.stopped {
		return
	}
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.entries = map[string]*entry{}
	cfg, err := c.load(c.ctx)
	if err != nil {
		c.logf("NAS 监控读取设置失败，先不采集: %v", err)
		return
	}
	c.apply(cfg, "")
}

// Stop 停掉全部循环并等它们退出（最多等到 ctx 结束）。
func (c *Collector) Stop(ctx context.Context) error {
	c.mu.Lock()
	c.stopped = true
	if c.cancel != nil {
		c.cancel()
	}
	c.mu.Unlock()
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("等 NAS 监控的采集退出超时: %w", ctx.Err())
	}
}

// Close 是关闭流程用的 Stop（cmd 的 shutdownPlan）。
func (c *Collector) Close(ctx context.Context) error { return c.Stop(ctx) }

func (c *Collector) load(ctx context.Context) (Config, error) {
	if c.Load == nil {
		return Config{}, errors.New("没有设置读取函数")
	}
	return c.Load(ctx)
}

// Reload 在保存、删除来源或改了本机来源的开关以后调用：重新读设置，只重启 id 这一个来源（id 是 LocalID 或数字编号）。
// 设置没变（同一个 revision、开关一样）时什么也不做，数据与历史照旧。
func (c *Collector) Reload(ctx context.Context, id string) error {
	c.mu.Lock()
	running := c.ctx != nil && !c.stopped
	c.mu.Unlock()
	if !running {
		return nil
	}
	cfg, err := c.load(ctx)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ctx == nil || c.stopped {
		return nil
	}
	c.apply(cfg, id)
	return nil
}

// apply 按读到的设置建立或更新各个 entry：only 为空时全部，否则只动这一个。调用时持有 c.mu。
func (c *Collector) apply(cfg Config, only string) {
	want := map[string]SourceConfig{}
	order := []string{LocalID}
	want[LocalID] = SourceConfig{Name: "本机", Kind: LocalKind, Enabled: cfg.LocalEnabled}
	for _, s := range cfg.Sources {
		id := strconv.FormatUint(uint64(s.ID), 10)
		want[id] = s
		order = append(order, id)
	}
	c.order = order
	for id, sc := range want {
		if only != "" && id != only {
			continue
		}
		if old, ok := c.entries[id]; ok && sameConfig(old.cfg, sc) {
			continue
		}
		c.replace(id, sc)
	}
	for id, old := range c.entries {
		if _, ok := want[id]; !ok && (only == "" || id == only) {
			if old.cancel != nil {
				old.cancel()
			}
			delete(c.entries, id)
		}
	}
}

// sameConfig 是设置没变：同一个 revision（数据库里的来源每次保存加一）、同样的开关与可用性。
func sameConfig(a, b SourceConfig) bool {
	return a.Enabled == b.Enabled && a.Revision == b.Revision && a.Broken == b.Broken && a.Kind == b.Kind
}

// replace 停掉 id 原来的循环，按 sc 建一个新的 entry（启用的来源开始循环）。调用时持有 c.mu。
func (c *Collector) replace(id string, sc SourceConfig) {
	if old, ok := c.entries[id]; ok && old.cancel != nil {
		old.cancel()
	}
	e := &entry{id: id, cfg: sc, retry: make(chan chan struct{})}
	c.entries[id] = e
	if id != LocalID {
		if k, ok := LookupKind(sc.Kind); ok {
			e.experimental = k.Experimental
		}
	}
	switch {
	case !sc.Enabled:
		e.state = StateDisabled
		return
	case sc.Broken != "":
		e.state, e.message = StateError, sc.Broken
		return
	}
	src, state, err := c.newSource(id, sc)
	if err != nil {
		e.state, e.message = state, scrubSecrets(redact.Secrets(err.Error()), sc)
		return
	}
	e.state = StateConnecting
	ctx, cancel := context.WithCancel(c.ctx)
	e.cancel = cancel
	c.wg.Go(func() { c.loop(ctx, e, src) })
}

func (c *Collector) newSource(id string, sc SourceConfig) (Source, string, error) {
	if id == LocalID {
		if c.NewLocal != nil {
			return c.NewLocal(), "", nil
		}
		return NewLocalSource(), "", nil
	}
	if c.Factory != nil {
		src, err := c.Factory(sc)
		return src, StateError, err
	}
	k, ok := LookupKind(sc.Kind)
	if !ok {
		return nil, StateUnsupported, fmt.Errorf("这个版本不支持「%s」来源", sc.Kind)
	}
	src, err := k.New(sc)
	return src, StateError, err
}

type outcome int

const (
	outcomeOK outcome = iota
	outcomeRetryLater
	outcomeParked
	outcomeStopped
)

// loop 是一个来源的采集循环：自己持有 src，退出时关掉它。
func (c *Collector) loop(ctx context.Context, e *entry, src Source) {
	defer func() { _ = src.Close() }()
	var lastFull time.Time
	failures := 0
	full := true
	var reply chan struct{}
	for {
		res := c.attempt(ctx, e, src, full)
		if reply != nil {
			close(reply)
			reply = nil
		}
		if res == outcomeStopped || ctx.Err() != nil {
			return
		}
		delay := time.Duration(-1)
		switch res {
		case outcomeOK:
			failures = 0
			if full {
				lastFull = c.now()
			}
			delay = c.light()
		case outcomeRetryLater:
			delay = c.backoff(failures)
			failures++
			next := c.now().Add(delay)
			e.mu.Lock()
			e.nextTry = &next
			e.mu.Unlock()
		}
		var tick <-chan time.Time
		var timer *time.Timer
		if delay >= 0 {
			timer = time.NewTimer(delay)
			tick = timer.C
		}
		select {
		case <-ctx.Done():
			stopTimer(timer)
			return
		case <-tick:
			full = lastFull.IsZero() || c.now().Sub(lastFull) >= c.full()
		case reply = <-e.retry:
			stopTimer(timer)
			full = true
		}
	}
}

// humanDuration 是给人看的时长：整秒写「N 秒」，不到一秒写毫秒。
func humanDuration(d time.Duration) string {
	if d >= time.Second {
		return fmt.Sprintf("%d 秒", int(d/time.Second))
	}
	return fmt.Sprintf("%d 毫秒", d.Milliseconds())
}

func stopTimer(t *time.Timer) {
	if t != nil {
		t.Stop()
	}
}

// attempt 采集一次并记下结果。
func (c *Collector) attempt(ctx context.Context, e *entry, src Source, full bool) outcome {
	callCtx, cancel := context.WithTimeout(ctx, c.timeout())
	snap, err := src.Collect(callCtx, full)
	cancel()
	if ctx.Err() != nil {
		return outcomeStopped
	}
	if err == nil && snap == nil {
		err = errors.New("来源没有返回数据")
	}
	if err != nil {
		state, msg, parked := classify(err, c.timeout())
		msg = scrubSecrets(msg, e.cfg)
		e.mu.Lock()
		e.state, e.message, e.nextTry = state, msg, nil
		e.mu.Unlock()
		if parked {
			return outcomeParked
		}
		return outcomeRetryLater
	}
	now := c.now()
	if snap.At.IsZero() {
		snap.At = now
	}
	e.mu.Lock()
	if full {
		e.snap = snap.clone()
	} else {
		e.snap = mergeLight(e.snap, snap)
	}
	e.history = append(e.history, e.snap.sample())
	if len(e.history) > HistoryLen {
		e.history = append([]Sample(nil), e.history[len(e.history)-HistoryLen:]...)
	}
	e.state, e.message, e.nextTry = StateOK, "", nil
	e.lastOK = &now
	e.mu.Unlock()
	if xs, ok := src.(ExtraSource); ok && e.id != LocalID && c.SaveExtra != nil {
		if extra := xs.TakeExtra(); extra != nil {
			if err := c.SaveExtra(ctx, e.cfg.ID, e.cfg.Revision, extra); err != nil {
				msg := scrubSecrets(redact.Secrets(err.Error()), SourceConfig{Secret: e.cfg.Secret, Extra: extra})
				c.logf("NAS 来源 %s 保存刷新的凭据失败: %v", e.cfg.Name, msg)
			}
		}
	}
	return outcomeOK
}

// classify 是采集出错时的状态与原因；parked 为真是停住（熔断）、不按退避重试。timeout 是单次采集的时限（写进超时的原因）。
func classify(err error, timeout time.Duration) (state, msg string, parked bool) {
	var ae *AuthError
	switch {
	case errors.Is(err, ErrUnsupported):
		return StateUnsupported, ErrUnsupported.Error(), true
	case errors.As(err, &ae):
		return StateAuthFailed, redact.Secrets(ae.Error()), true
	case errors.Is(err, ErrCertChanged):
		return StateCertChanged, ErrCertChanged.Error(), true
	case errors.Is(err, ErrCertUntrusted):
		return StateCertUntrusted, ErrCertUntrusted.Error(), true
	case errors.Is(err, context.DeadlineExceeded):
		return StateError, fmt.Sprintf("%s内没有回应", humanDuration(timeout)), false
	default:
		return StateError, redact.Secrets(err.Error()), false
	}
}

// scrubSecrets 把来源自己的密钥与长期凭据的原文从原因里换成 ***（来源返回的错误里本不该有它们，这里再挡一道）。
// 短于 4 个字的不换，免得把普通文字也换掉。
func scrubSecrets(msg string, cfg SourceConfig) string {
	vals := make([]string, 0, 1+len(cfg.Extra))
	vals = append(vals, cfg.Secret)
	for _, v := range cfg.Extra {
		vals = append(vals, v)
	}
	for _, v := range vals {
		if len(v) >= 4 {
			msg = strings.ReplaceAll(msg, v, "***")
		}
	}
	return msg
}

// ErrNoLoop 是这个来源没有在采集（停用了、设置有问题或这个版本不支持），「重新连接」没有用。
var ErrNoLoop = errors.New("这个来源没有在采集（停用了或设置有问题），改好设置以后会自动重连")

// ErrNotFound 是没有这个来源。
var ErrNotFound = errors.New("没有这个来源")

// Retry 立即试一次 id 这个来源（「重新连接」）：等这一次试完（最多等到 ctx 结束），返回它现在的情况。
// 失败了照旧：熔断的继续停住，别的按退避重试。
func (c *Collector) Retry(ctx context.Context, id string) (Status, error) {
	c.mu.Lock()
	e, ok := c.entries[id]
	c.mu.Unlock()
	if !ok {
		return Status{}, ErrNotFound
	}
	if e.cancel == nil {
		return e.status(), ErrNoLoop
	}
	reply := make(chan struct{})
	select {
	case e.retry <- reply:
	case <-ctx.Done():
		return e.status(), ctx.Err()
	}
	select {
	case <-reply:
	case <-ctx.Done():
		return e.status(), ctx.Err()
	}
	return e.status(), nil
}

// List 是各个来源现在的情况：本机在最前，其余按编号。只读内存，不对外发请求。
func (c *Collector) List() []Status {
	c.mu.Lock()
	es := make([]*entry, 0, len(c.order))
	for _, id := range c.order {
		if e, ok := c.entries[id]; ok {
			es = append(es, e)
		}
	}
	c.mu.Unlock()
	out := make([]Status, 0, len(es))
	for _, e := range es {
		out = append(out, e.status())
	}
	return out
}

func (e *entry) status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := Status{
		ID: e.id, Name: e.cfg.Name, Kind: e.cfg.Kind, Enabled: e.cfg.Enabled, Experimental: e.experimental,
		URL: e.cfg.URL, HasSecret: e.cfg.Secret != "", PinSHA256: e.cfg.PinSHA256, InsecureAck: e.cfg.InsecureAck,
		State: e.state, Message: e.message, Snapshot: e.snap.clone(), History: append([]Sample(nil), e.history...),
	}
	if e.lastOK != nil {
		t := *e.lastOK
		st.LastOKAt = &t
	}
	if e.nextTry != nil {
		t := *e.nextTry
		st.NextTryAt = &t
	}
	return st
}
