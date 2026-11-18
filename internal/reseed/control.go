package reseed

import (
	"context"
	"errors"
	"sync"
	"time"
)

// 一轮被提前停下的原因（这一轮 ctx 的 cause）。
var (
	// ErrRunStopped 是手动停止。
	ErrRunStopped = errors.New("这一轮被停止")
	// ErrRunTimeout 是这一轮用完了时限（暂停的时间不算）。
	ErrRunTimeout = errors.New("这一轮超过时限，已停下")
	// ErrPausedTooLong 是暂停太久，自动停止（免得忘了继续以后定时的轮次一直跑不了）。
	ErrPausedTooLong = errors.New("暂停太久，已自动停止")
)

// Timer 是 Control 用的计时器（测试里换成可控的）。
type Timer interface {
	C() <-chan time.Time
	Stop() bool
}

type realTimer struct{ t *time.Timer }

func (r realTimer) C() <-chan time.Time { return r.t.C }
func (r realTimer) Stop() bool          { return r.t.Stop() }

// ControlConfig 是 Control 的设置。
type ControlConfig struct {
	// Budget 是这一轮的时限，暂停的时间不算；0 表示不限。
	Budget time.Duration
	// MaxPause 是一次暂停最长多久，超过就停下这一轮；0 表示不限。
	MaxPause time.Duration
	// Cancel 在超时（ErrRunTimeout）或暂停太久（ErrPausedTooLong）时取消这一轮。
	Cancel context.CancelCauseFunc
	Now    func() time.Time
	// NewTimer 建计时器，默认用 time.NewTimer。
	NewTimer func(d time.Duration) Timer
}

// Control 暂停、继续一轮辅种，并管这一轮的时限（暂停的时间不算）与一次暂停的最长时间。
// 这一轮在检查点（每次查询 IYUU、每个候选之前）调用 Wait：暂停时停在那里，继续以后接着跑。
type Control struct {
	cfg ControlConfig

	mu     sync.Mutex
	paused bool
	since  time.Time     // 这一次暂停的开始
	total  time.Duration // 已经暂停过的总时长（不含这一次）
	// resumed 在暂停时新建，继续时关闭（Wait 等它）
	resumed chan struct{}
	// changed 在每次暂停或继续时关闭并换新（计时的 goroutine 等它，重新算时限）
	changed chan struct{}
	// onWait 在 Wait 开始等继续时调用（测试用）
	onWait func()
}

// NewControl 构造控制器；调用 Start 以后才开始计时。
func NewControl(cfg ControlConfig) *Control {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.NewTimer == nil {
		cfg.NewTimer = func(d time.Duration) Timer { return realTimer{time.NewTimer(d)} }
	}
	return &Control{cfg: cfg, changed: make(chan struct{})}
}

// Start 开始计时，直到 ctx 结束。
func (c *Control) Start(ctx context.Context) {
	if c.cfg.Cancel == nil || (c.cfg.Budget <= 0 && c.cfg.MaxPause <= 0) {
		return
	}
	go c.watch(ctx)
}

// watch 是计时：没暂停时按剩下的时限计时，暂停时按一次暂停的最长时间计时；每次暂停或继续重新算。
func (c *Control) watch(ctx context.Context) {
	remaining := c.cfg.Budget
	for {
		c.mu.Lock()
		paused, changed := c.paused, c.changed
		c.mu.Unlock()

		var t Timer
		var fire <-chan time.Time
		var cause error
		started := c.cfg.Now()
		switch {
		case !paused && c.cfg.Budget > 0:
			if remaining <= 0 {
				c.cfg.Cancel(ErrRunTimeout)
				return
			}
			t, cause = c.cfg.NewTimer(remaining), ErrRunTimeout
		case paused && c.cfg.MaxPause > 0:
			t, cause = c.cfg.NewTimer(c.cfg.MaxPause), ErrPausedTooLong
		}
		if t != nil {
			fire = t.C()
		}
		select {
		case <-ctx.Done():
			if t != nil {
				t.Stop()
			}
			return
		case <-fire:
			c.cfg.Cancel(cause)
			return
		case <-changed:
			if t != nil {
				t.Stop()
			}
			if !paused {
				remaining -= c.cfg.Now().Sub(started)
			}
		}
	}
}

// notifyLocked 告诉计时的 goroutine 状态变了。调用时持有 mu。
func (c *Control) notifyLocked() {
	close(c.changed)
	c.changed = make(chan struct{})
}

// Pause 暂停（正在下载的那一个做完以后，在下一个检查点停下）；已经暂停时返回 false。
func (c *Control) Pause() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.paused {
		return false
	}
	c.paused, c.since, c.resumed = true, c.cfg.Now(), make(chan struct{})
	c.notifyLocked()
	return true
}

// Resume 继续，返回这一次暂停了多久；没在暂停时返回 false。
func (c *Control) Resume() (time.Duration, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.paused {
		return 0, false
	}
	d := c.cfg.Now().Sub(c.since)
	c.total += d
	c.paused = false
	close(c.resumed)
	c.notifyLocked()
	return d, true
}

// State 报告是不是在暂停、这一次暂停从什么时候开始，以及一共暂停过多久（含正在进行的这一次）。
func (c *Control) State() (paused bool, since time.Time, total time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	total = c.total
	if c.paused {
		total += c.cfg.Now().Sub(c.since)
	}
	return c.paused, c.since, total
}

// Wait 是检查点：暂停时等到继续或者这一轮被取消。返回在这里等了多久，以及 ctx 的错误（被取消时）。
func (c *Control) Wait(ctx context.Context) (time.Duration, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	c.mu.Lock()
	if !c.paused {
		c.mu.Unlock()
		return 0, nil
	}
	start, resumed, onWait := c.cfg.Now(), c.resumed, c.onWait
	c.mu.Unlock()
	if onWait != nil {
		onWait()
	}
	select {
	case <-resumed:
		return c.cfg.Now().Sub(start), ctx.Err()
	case <-ctx.Done():
		return c.cfg.Now().Sub(start), ctx.Err()
	}
}
