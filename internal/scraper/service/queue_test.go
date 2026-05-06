package service

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

type mockTask struct {
	mu         sync.Mutex
	id         string
	ttype      string
	runs       atomic.Int32
	state      core.TaskState
	retryCount int
	maxRetries int
	lastErr    error
	runFunc    func(ctx context.Context, runIdx int32) error
}

func (m *mockTask) ID() string   { return m.id }
func (m *mockTask) Type() string { return m.ttype }

func (m *mockTask) Run(ctx context.Context) error {
	n := m.runs.Add(1)
	if m.runFunc == nil {
		return nil
	}
	return m.runFunc(ctx, n)
}

func (m *mockTask) State() core.TaskState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

func (m *mockTask) SetState(s core.TaskState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = s
}

func (m *mockTask) RetryCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.retryCount
}

func (m *mockTask) MaxRetries() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.maxRetries
}

func (m *mockTask) IncrementRetry() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retryCount++
}

func (m *mockTask) LastError() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastErr
}

func (m *mockTask) SetLastError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastErr = err
}

func (m *mockTask) Runs() int32 { return m.runs.Load() }

func newMockTask(id string, maxRetries int, fn func(ctx context.Context, runIdx int32) error) *mockTask {
	return &mockTask{
		id:         id,
		ttype:      "test",
		maxRetries: maxRetries,
		runFunc:    fn,
	}
}

func TestQueueBasicProcessing(t *testing.T) {
	q := NewQueue(32)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, q.Start(ctx, 3))

	const n = 10
	tasks := make([]*mockTask, n)
	for i := 0; i < n; i++ {
		tasks[i] = newMockTask(fmt.Sprintf("t-%d", i), 0, func(ctx context.Context, _ int32) error {
			select {
			case <-time.After(100 * time.Millisecond):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}

	start := time.Now()
	for _, mt := range tasks {
		require.NoError(t, q.Enqueue(ctx, mt))
	}

	require.Eventually(t, func() bool {
		return q.Processed() == int64(n)
	}, 3*time.Second, 10*time.Millisecond, "processed should reach %d", n)
	elapsed := time.Since(start)

	q.Stop()

	assert.Equal(t, int64(n), q.Processed())
	assert.Equal(t, int64(0), q.Failed())
	for _, mt := range tasks {
		assert.Equal(t, core.TaskSuccess, mt.State(), "task %s", mt.ID())
	}

	assert.Less(t, elapsed, 1*time.Second, "10 tasks @100ms on 3 workers should finish under 1s, got %v", elapsed)
	t.Logf("10 tasks / 3 workers took %v", elapsed)
}

func TestQueueRetrySuccess(t *testing.T) {
	q := NewQueue(8)
	q.SetBackoff(func(int) time.Duration { return 0 })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, q.Start(ctx, 1))
	defer q.Stop()

	mt := newMockTask("retry-ok", 3, func(_ context.Context, runIdx int32) error {
		if runIdx < 3 {
			return fmt.Errorf("transient failure run=%d", runIdx)
		}
		return nil
	})

	require.NoError(t, q.Enqueue(ctx, mt))

	require.Eventually(t, func() bool {
		return mt.State() == core.TaskSuccess
	}, 2*time.Second, 5*time.Millisecond)

	assert.Equal(t, int32(3), mt.Runs())
	assert.Equal(t, 2, mt.RetryCount())
	assert.Equal(t, int64(1), q.Processed())
	assert.Equal(t, int64(0), q.Failed())
}

func TestQueueMaxRetriesFailed(t *testing.T) {
	q := NewQueue(8)
	q.SetBackoff(func(int) time.Duration { return 0 })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, q.Start(ctx, 1))
	defer q.Stop()

	mt := newMockTask("always-fail", 2, func(_ context.Context, runIdx int32) error {
		return fmt.Errorf("boom run=%d", runIdx)
	})

	require.NoError(t, q.Enqueue(ctx, mt))

	require.Eventually(t, func() bool {
		return mt.State() == core.TaskFailed
	}, 2*time.Second, 5*time.Millisecond)

	assert.Equal(t, int32(3), mt.Runs(), "should run initial + 2 retries")
	assert.Equal(t, 2, mt.RetryCount())
	assert.Equal(t, int64(1), q.Failed())
	assert.Equal(t, int64(0), q.Processed())
	assert.Error(t, mt.LastError())
}

func TestQueueContextCancel(t *testing.T) {
	q := NewQueue(8)
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, q.Start(ctx, 2))

	mt := newMockTask("long-running", 0, func(runCtx context.Context, _ int32) error {
		select {
		case <-runCtx.Done():
			return runCtx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	})

	require.NoError(t, q.Enqueue(context.Background(), mt))

	require.Eventually(t, func() bool {
		return mt.State() == core.TaskRunning
	}, 1*time.Second, 5*time.Millisecond)

	cancel()

	require.Eventually(t, func() bool {
		return mt.State() == core.TaskCanceled
	}, 2*time.Second, 5*time.Millisecond)

	q.Stop()

	assert.Equal(t, core.TaskCanceled, mt.State())
}

func TestQueueGracefulShutdown(t *testing.T) {
	q := NewQueue(8)
	ctx := context.Background()
	require.NoError(t, q.Start(ctx, 3))

	baseGoroutines := runtime.NumGoroutine()

	for i := 0; i < 3; i++ {
		mt := newMockTask(fmt.Sprintf("g-%d", i), 0, func(_ context.Context, _ int32) error {
			time.Sleep(20 * time.Millisecond)
			return nil
		})
		require.NoError(t, q.Enqueue(ctx, mt))
	}

	stopDone := make(chan struct{})
	go func() {
		q.Stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
	case <-time.After(1 * time.Second):
		t.Fatal("Stop() did not return within 1s")
	}

	time.Sleep(50 * time.Millisecond)
	after := runtime.NumGoroutine()
	assert.LessOrEqual(t, after, baseGoroutines, "worker goroutines should have exited (base=%d, after=%d)", baseGoroutines, after)
}

func TestQueueEnqueueAfterStop(t *testing.T) {
	q := NewQueue(4)
	ctx := context.Background()
	require.NoError(t, q.Start(ctx, 1))
	q.Stop()

	mt := newMockTask("after-stop", 0, nil)
	err := q.Enqueue(ctx, mt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stopped")
}

func TestQueueStartTwice(t *testing.T) {
	q := NewQueue(4)
	ctx := context.Background()
	require.NoError(t, q.Start(ctx, 1))
	defer q.Stop()

	err := q.Start(ctx, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already started")
}

func TestExponentialBackoff(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{-1, 0},
		{0, 0},
		{1, 5 * time.Second},
		{2, 30 * time.Second},
		{3, 3 * time.Minute},
		{4, 3 * time.Minute},
		{10, 3 * time.Minute},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("attempt=%d", tc.attempt), func(t *testing.T) {
			assert.Equal(t, tc.want, ExponentialBackoff(tc.attempt))
		})
	}
}

func TestShouldRetry(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name       string
		err        error
		attempt    int
		maxRetries int
		want       bool
	}{
		{"nil error never retries", nil, 0, 3, false},
		{"within budget retries", boom, 0, 3, true},
		{"exactly at max does not retry", boom, 3, 3, false},
		{"over max does not retry", boom, 5, 3, false},
		{"zero max never retries", boom, 0, 0, false},
		{"one retry allowed", boom, 0, 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ShouldRetry(tc.err, tc.attempt, tc.maxRetries))
		})
	}
}
