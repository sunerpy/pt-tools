package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

// Queue 内存 FIFO 任务队列 + worker pool。
//
// 不做持久化（Task 17 会添加 DB 持久化层）。
// 典型用法：
//
//	q := NewQueue(128)
//	q.Start(ctx, 3)
//	q.Enqueue(ctx, task)
//	...
//	q.Stop()
type Queue struct {
	tasks   chan core.Task
	workers int
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc

	started atomic.Bool
	stopped atomic.Bool

	processed atomic.Int64
	failed    atomic.Int64

	backoff func(attempt int) time.Duration
}

// NewQueue 创建一个容量为 bufferSize 的队列。bufferSize<=0 时回退到 64。
// 默认使用 ExponentialBackoff 作为重试退避函数。
func NewQueue(bufferSize int) *Queue {
	if bufferSize <= 0 {
		bufferSize = 64
	}
	return &Queue{
		tasks:   make(chan core.Task, bufferSize),
		backoff: ExponentialBackoff,
	}
}

// SetBackoff 覆盖默认的退避函数。必须在 Start 之前调用。主要用于测试。
func (q *Queue) SetBackoff(fn func(attempt int) time.Duration) {
	if fn == nil {
		fn = ExponentialBackoff
	}
	q.backoff = fn
}

// Start 启动 workers 个 worker goroutine。只能成功调用一次。
// workers<=0 时回退到 3。
func (q *Queue) Start(ctx context.Context, workers int) error {
	if !q.started.CompareAndSwap(false, true) {
		return errors.New("queue already started")
	}
	if workers <= 0 {
		workers = 3
	}
	q.workers = workers
	q.ctx, q.cancel = context.WithCancel(ctx)

	for i := 0; i < workers; i++ {
		q.wg.Add(1)
		go q.worker(i)
	}
	return nil
}

// Enqueue 提交任务。队列满时阻塞，直到有 worker 取走、传入的 ctx 或 Queue 自身 ctx 取消。
func (q *Queue) Enqueue(ctx context.Context, t core.Task) error {
	if q.stopped.Load() {
		return errors.New("queue stopped")
	}
	if t == nil {
		return errors.New("nil task")
	}
	if !q.started.Load() {
		return errors.New("queue not started")
	}
	t.SetState(core.TaskPending)
	select {
	case q.tasks <- t:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-q.ctx.Done():
		return q.ctx.Err()
	}
}

// Stop 优雅关闭：取消内部 ctx，关闭 channel，等待所有 worker 退出。可重复调用。
func (q *Queue) Stop() {
	if !q.stopped.CompareAndSwap(false, true) {
		return
	}
	if q.cancel != nil {
		q.cancel()
	}
	close(q.tasks)
	q.wg.Wait()
}

// Processed 返回成功处理的任务数。
func (q *Queue) Processed() int64 { return q.processed.Load() }

// Failed 返回最终失败的任务数（不含重试中）。
func (q *Queue) Failed() int64 { return q.failed.Load() }

func (q *Queue) worker(_ int) {
	defer q.wg.Done()
	for {
		select {
		case <-q.ctx.Done():
			return
		case t, ok := <-q.tasks:
			if !ok {
				return
			}
			q.runTask(t)
		}
	}
}

func (q *Queue) runTask(t core.Task) {
	t.SetState(core.TaskRunning)
	err := t.Run(q.ctx)

	if err == nil {
		t.SetState(core.TaskSuccess)
		q.processed.Add(1)
		return
	}

	t.SetLastError(err)

	if errors.Is(err, context.Canceled) || errors.Is(q.ctx.Err(), context.Canceled) {
		t.SetState(core.TaskCanceled)
		return
	}

	if t.RetryCount() >= t.MaxRetries() {
		t.SetState(core.TaskFailed)
		q.failed.Add(1)
		return
	}

	t.IncrementRetry()
	t.SetState(core.TaskRetrying)

	backoff := q.backoff(t.RetryCount())
	if backoff > 0 {
		timer := time.NewTimer(backoff)
		select {
		case <-timer.C:
		case <-q.ctx.Done():
			timer.Stop()
			t.SetState(core.TaskCanceled)
			return
		}
	}

	select {
	case <-q.ctx.Done():
		t.SetState(core.TaskCanceled)
		return
	case q.tasks <- t:
		t.SetState(core.TaskPending)
	default:
		t.SetState(core.TaskFailed)
		q.failed.Add(1)
	}
}
