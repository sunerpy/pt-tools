package reseed

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ctlTimer struct {
	d time.Duration
	c chan time.Time
}

func (t *ctlTimer) C() <-chan time.Time { return t.c }
func (t *ctlTimer) Stop() bool          { return true }

type ctlClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *ctlClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *ctlClock) add(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// Wait 在暂停时等到继续，返回等了多久；没暂停时马上返回；这一轮被取消时返回 ctx 的错误。
func TestControlWait(t *testing.T) {
	clock := &ctlClock{now: time.Date(2026, 10, 30, 10, 0, 0, 0, time.Local)}
	c := NewControl(ControlConfig{Now: clock.Now})
	ctx, cancel := context.WithCancelCause(context.Background())
	d, err := c.Wait(ctx)
	require.NoError(t, err)
	assert.Zero(t, d)

	assert.True(t, c.Pause())
	assert.False(t, c.Pause(), "已经暂停")
	entered := make(chan struct{}, 2)
	c.onWait = func() { entered <- struct{}{} }
	done := make(chan time.Duration)
	go func() {
		d, err := c.Wait(ctx)
		assert.NoError(t, err)
		done <- d
	}()
	<-entered // 已经在等继续
	select {
	case <-done:
		t.Fatal("暂停时不应该返回")
	default:
	}
	clock.add(3 * time.Minute)
	got, ok := c.Resume()
	require.True(t, ok)
	assert.Equal(t, 3*time.Minute, got)
	assert.Equal(t, 3*time.Minute, <-done)
	_, ok = c.Resume()
	assert.False(t, ok, "没在暂停")
	paused, _, total := c.State()
	assert.False(t, paused)
	assert.Equal(t, 3*time.Minute, total)

	c.Pause()
	go func() {
		_, err := c.Wait(ctx)
		assert.ErrorIs(t, err, context.Canceled)
		done <- 0
	}()
	<-entered
	cancel(ErrRunStopped)
	<-done
	assert.ErrorIs(t, context.Cause(ctx), ErrRunStopped)
}

// 时限不算暂停的时间：暂停前用掉的时间从时限里扣掉，继续以后按剩下的计时；暂停超过最长时间就停下这一轮。
func TestControlBudgetExcludesPause(t *testing.T) {
	clock := &ctlClock{now: time.Date(2026, 10, 30, 10, 0, 0, 0, time.Local)}
	timers := make(chan *ctlTimer, 8)
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	c := NewControl(ControlConfig{
		Budget: 30 * time.Minute, MaxPause: 24 * time.Hour, Cancel: cancel, Now: clock.Now,
		NewTimer: func(d time.Duration) Timer {
			tm := &ctlTimer{d: d, c: make(chan time.Time, 1)}
			timers <- tm
			return tm
		},
	})
	c.Start(ctx)
	first := <-timers
	assert.Equal(t, 30*time.Minute, first.d)

	clock.add(10 * time.Minute)
	c.Pause()
	pause := <-timers
	assert.Equal(t, 24*time.Hour, pause.d, "暂停时按一次暂停的最长时间计时")
	clock.add(5 * time.Hour)
	c.Resume()
	rest := <-timers
	assert.Equal(t, 20*time.Minute, rest.d, "暂停的 5 小时不算，剩下 20 分钟")

	rest.c <- clock.Now()
	<-ctx.Done()
	assert.ErrorIs(t, context.Cause(ctx), ErrRunTimeout)

	// 暂停太久
	ctx2, cancel2 := context.WithCancelCause(context.Background())
	defer cancel2(nil)
	c2 := NewControl(ControlConfig{
		Budget: 30 * time.Minute, MaxPause: time.Hour, Cancel: cancel2, Now: clock.Now,
		NewTimer: func(d time.Duration) Timer {
			tm := &ctlTimer{d: d, c: make(chan time.Time, 1)}
			timers <- tm
			return tm
		},
	})
	c2.Start(ctx2)
	<-timers
	c2.Pause()
	p := <-timers
	p.c <- clock.Now()
	<-ctx2.Done()
	assert.ErrorIs(t, context.Cause(ctx2), ErrPausedTooLong)
}
