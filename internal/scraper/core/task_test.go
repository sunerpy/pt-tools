package core

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskState_String(t *testing.T) {
	tests := []struct {
		val  TaskState
		want string
	}{
		{TaskPending, "pending"},
		{TaskRunning, "running"},
		{TaskSuccess, "success"},
		{TaskFailed, "failed"},
		{TaskCanceled, "canceled"},
		{TaskRetrying, "retrying"},
		{TaskState(99), "unknown"},
		{TaskState(-1), "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.val.String())
		})
	}
}

type fakeTask struct {
	id         string
	typ        string
	state      TaskState
	retryCount int
	maxRetries int
	lastErr    error
	runErr     error
	runCalls   int
}

func (f *fakeTask) ID() string             { return f.id }
func (f *fakeTask) Type() string           { return f.typ }
func (f *fakeTask) State() TaskState       { return f.state }
func (f *fakeTask) RetryCount() int        { return f.retryCount }
func (f *fakeTask) MaxRetries() int        { return f.maxRetries }
func (f *fakeTask) SetState(s TaskState)   { f.state = s }
func (f *fakeTask) IncrementRetry()        { f.retryCount++ }
func (f *fakeTask) LastError() error       { return f.lastErr }
func (f *fakeTask) SetLastError(err error) { f.lastErr = err }
func (f *fakeTask) Run(ctx context.Context) error {
	f.runCalls++
	return f.runErr
}

func TestTaskInterface_Implemented(t *testing.T) {
	var task Task = &fakeTask{
		id:         "task-1",
		typ:        "movie",
		state:      TaskPending,
		maxRetries: 3,
	}

	assert.Equal(t, "task-1", task.ID())
	assert.Equal(t, "movie", task.Type())
	assert.Equal(t, TaskPending, task.State())
	assert.Equal(t, 0, task.RetryCount())
	assert.Equal(t, 3, task.MaxRetries())
	assert.NoError(t, task.LastError())

	task.SetState(TaskRunning)
	assert.Equal(t, TaskRunning, task.State())

	task.IncrementRetry()
	task.IncrementRetry()
	assert.Equal(t, 2, task.RetryCount())

	boom := errors.New("boom")
	task.SetLastError(boom)
	assert.True(t, errors.Is(task.LastError(), boom))

	require.NoError(t, task.Run(context.Background()))
}

func TestTaskInterface_RunPropagatesError(t *testing.T) {
	want := errors.New("scrape failed")
	var task Task = &fakeTask{runErr: want}

	err := task.Run(context.Background())
	assert.ErrorIs(t, err, want)
}

func TestTaskInterface_RunContextCancellable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ft := &fakeTask{}
	var task Task = ft
	require.NoError(t, task.Run(ctx))
	assert.Equal(t, 1, ft.runCalls)
}
