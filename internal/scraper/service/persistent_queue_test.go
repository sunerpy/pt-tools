package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
	"github.com/sunerpy/pt-tools/internal/scraper/store"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, store.Migrate(db))
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

type persistMockTask struct {
	mu         sync.Mutex
	id         string
	runs       atomic.Int32
	state      core.TaskState
	retryCount int
	maxRetries int
	lastErr    error
	runFunc    func(ctx context.Context, runIdx int32) error
}

func (m *persistMockTask) ID() string   { return m.id }
func (m *persistMockTask) Type() string { return "test" }

func (m *persistMockTask) Run(ctx context.Context) error {
	n := m.runs.Add(1)
	if m.runFunc == nil {
		return nil
	}
	return m.runFunc(ctx, n)
}

func (m *persistMockTask) State() core.TaskState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

func (m *persistMockTask) SetState(s core.TaskState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = s
}

func (m *persistMockTask) RetryCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.retryCount
}

func (m *persistMockTask) MaxRetries() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.maxRetries
}

func (m *persistMockTask) IncrementRetry() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retryCount++
}

func (m *persistMockTask) LastError() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastErr
}

func (m *persistMockTask) SetLastError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastErr = err
}

type registryBuilder struct {
	mu    sync.Mutex
	tasks map[uint]*persistMockTask
}

func newRegistryBuilder() *registryBuilder {
	return &registryBuilder{tasks: map[uint]*persistMockTask{}}
}

func (r *registryBuilder) register(dbID uint, task *persistMockTask) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[dbID] = task
}

func (r *registryBuilder) build(rec store.ScrapeTask) core.Task {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.tasks[rec.ID]; ok {
		return t
	}
	return &persistMockTask{id: "auto-" + strconv.FormatUint(uint64(rec.ID), 10), maxRetries: rec.MaxRetries}
}

func TestPersistQueue_EnqueuePersists(t *testing.T) {
	db := openTestDB(t)
	reg := newRegistryBuilder()

	pq, err := NewPersistentQueue(PersistentConfig{
		DB:                 db,
		BufferSize:         8,
		TaskBuilder:        reg.build,
		RetryCheckInterval: 24 * time.Hour,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, pq.Start(ctx, 1))
	defer pq.Stop()

	blockCh := make(chan struct{})
	mt := &persistMockTask{id: "t-1", maxRetries: 3, runFunc: func(ctx context.Context, _ int32) error {
		select {
		case <-blockCh:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}

	// 注册：taskBuilder 将根据 DB 记录 ID 返回该 mock
	reg.register(0, mt)

	var savedID uint
	pq.taskBuilder = func(rec store.ScrapeTask) core.Task {
		savedID = rec.ID
		return mt
	}

	rec, err := pq.Enqueue(ctx, "movie", "/tmp/x", nil, map[string]any{"foo": "bar"})
	require.NoError(t, err)
	require.NotNil(t, rec)
	assert.NotZero(t, rec.ID)
	assert.Equal(t, savedID, rec.ID)

	var stored store.ScrapeTask
	require.NoError(t, db.First(&stored, rec.ID).Error)
	assert.Equal(t, "movie", stored.TaskType)
	assert.Equal(t, "/tmp/x", stored.MediaPath)
	// state 可能是 pending 或 running（取决于 worker 调度时机）
	assert.Contains(t, []string{stateScrapePending, stateScrapeRunning}, stored.State)
	assert.Contains(t, stored.RequestData, `"foo":"bar"`)
	assert.Equal(t, 3, stored.MaxRetries)

	close(blockCh)
}

func TestPersistQueue_RunSuccessUpdatesDB(t *testing.T) {
	db := openTestDB(t)
	reg := newRegistryBuilder()

	pq, err := NewPersistentQueue(PersistentConfig{
		DB:                 db,
		BufferSize:         4,
		TaskBuilder:        reg.build,
		RetryCheckInterval: 24 * time.Hour,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, pq.Start(ctx, 1))
	defer pq.Stop()

	mt := &persistMockTask{id: "ok-1", maxRetries: 3, runFunc: func(context.Context, int32) error { return nil }}
	pq.taskBuilder = func(rec store.ScrapeTask) core.Task { return mt }

	rec, err := pq.Enqueue(ctx, "movie", "/a", nil, nil)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		var r store.ScrapeTask
		if err := db.First(&r, rec.ID).Error; err != nil {
			return false
		}
		return r.State == stateScrapeSuccess
	}, 2*time.Second, 10*time.Millisecond)

	var final store.ScrapeTask
	require.NoError(t, db.First(&final, rec.ID).Error)
	assert.Equal(t, stateScrapeSuccess, final.State)
	assert.NotNil(t, final.CompletedAt)
	assert.InDelta(t, 100.0, final.Progress, 0.0001)
	assert.Equal(t, int32(1), mt.runs.Load())
}

func TestPersistQueue_RunFailRetryUpdatesDB(t *testing.T) {
	db := openTestDB(t)
	reg := newRegistryBuilder()

	pq, err := NewPersistentQueue(PersistentConfig{
		DB:                 db,
		BufferSize:         4,
		TaskBuilder:        reg.build,
		RetryCheckInterval: 24 * time.Hour,
	})
	require.NoError(t, err)

	pq.mem.SetBackoff(func(int) time.Duration { return 100 * time.Millisecond })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, pq.Start(ctx, 1))
	defer pq.Stop()

	mt := &persistMockTask{id: "fail-once", maxRetries: 5, runFunc: func(_ context.Context, idx int32) error {
		return fmt.Errorf("boom-%d", idx)
	}}
	pq.taskBuilder = func(rec store.ScrapeTask) core.Task { return mt }

	rec, err := pq.Enqueue(ctx, "movie", "/b", nil, nil)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		var r store.ScrapeTask
		if err := db.First(&r, rec.ID).Error; err != nil {
			return false
		}
		return r.State == stateScrapeRetrying && r.RetryCount >= 1 && r.NextRetryAt != nil
	}, 2*time.Second, 10*time.Millisecond)

	var r store.ScrapeTask
	require.NoError(t, db.First(&r, rec.ID).Error)
	assert.Equal(t, stateScrapeRetrying, r.State)
	assert.GreaterOrEqual(t, r.RetryCount, 1)
	require.NotNil(t, r.NextRetryAt)
	assert.True(t, r.NextRetryAt.After(time.Now().Add(-1*time.Second)))
	assert.Contains(t, r.LastError, "boom")
}

func TestPersistQueue_MaxRetriesExceededFailed(t *testing.T) {
	db := openTestDB(t)
	reg := newRegistryBuilder()

	pq, err := NewPersistentQueue(PersistentConfig{
		DB:                 db,
		BufferSize:         4,
		TaskBuilder:        reg.build,
		RetryCheckInterval: 24 * time.Hour,
	})
	require.NoError(t, err)
	pq.mem.SetBackoff(func(int) time.Duration { return 0 })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, pq.Start(ctx, 1))
	defer pq.Stop()

	mt := &persistMockTask{id: "doomed", maxRetries: 1, runFunc: func(context.Context, int32) error {
		return errors.New("always-fails")
	}}
	pq.taskBuilder = func(rec store.ScrapeTask) core.Task { return mt }

	rec, err := pq.Enqueue(ctx, "movie", "/c", nil, nil)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		var r store.ScrapeTask
		if err := db.First(&r, rec.ID).Error; err != nil {
			return false
		}
		return r.State == stateScrapeFailed
	}, 3*time.Second, 10*time.Millisecond)

	var final store.ScrapeTask
	require.NoError(t, db.First(&final, rec.ID).Error)
	assert.Equal(t, stateScrapeFailed, final.State)
	assert.NotNil(t, final.CompletedAt)
	assert.Contains(t, final.LastError, "always-fails")
	assert.GreaterOrEqual(t, int(mt.runs.Load()), 2)
}

func TestPersistQueue_Recover(t *testing.T) {
	db := openTestDB(t)

	pending := store.ScrapeTask{TaskType: "movie", MediaPath: "/p", State: stateScrapePending, MaxRetries: 3}
	retrying := store.ScrapeTask{TaskType: "movie", MediaPath: "/r", State: stateScrapeRetrying, MaxRetries: 3}
	success := store.ScrapeTask{TaskType: "movie", MediaPath: "/s", State: stateScrapeSuccess, MaxRetries: 3}
	failed := store.ScrapeTask{TaskType: "movie", MediaPath: "/f", State: stateScrapeFailed, MaxRetries: 3}
	require.NoError(t, db.Create(&pending).Error)
	require.NoError(t, db.Create(&retrying).Error)
	require.NoError(t, db.Create(&success).Error)
	require.NoError(t, db.Create(&failed).Error)

	var seen sync.Map
	builder := func(rec store.ScrapeTask) core.Task {
		return &persistMockTask{
			id:         "rec-" + strconv.FormatUint(uint64(rec.ID), 10),
			maxRetries: rec.MaxRetries,
			runFunc: func(context.Context, int32) error {
				seen.Store(rec.ID, true)
				return nil
			},
		}
	}

	pq, err := NewPersistentQueue(PersistentConfig{
		DB:                 db,
		BufferSize:         8,
		TaskBuilder:        builder,
		RetryCheckInterval: 24 * time.Hour,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, pq.Start(ctx, 2))
	defer pq.Stop()

	require.Eventually(t, func() bool {
		_, okP := seen.Load(pending.ID)
		_, okR := seen.Load(retrying.ID)
		return okP && okR
	}, 2*time.Second, 10*time.Millisecond)

	// success / failed 任务不应被 recover 重新入队
	_, okS := seen.Load(success.ID)
	_, okF := seen.Load(failed.ID)
	assert.False(t, okS, "success task should not be recovered")
	assert.False(t, okF, "failed task should not be recovered")

	require.Eventually(t, func() bool {
		var p, r store.ScrapeTask
		if err := db.First(&p, pending.ID).Error; err != nil {
			return false
		}
		if err := db.First(&r, retrying.ID).Error; err != nil {
			return false
		}
		return p.State == stateScrapeSuccess && r.State == stateScrapeSuccess
	}, 2*time.Second, 10*time.Millisecond)
}

func TestPersistQueue_RetryLoop_WaitsForBackoff(t *testing.T) {
	db := openTestDB(t)

	future := time.Now().Add(1 * time.Hour)
	rec := store.ScrapeTask{
		TaskType:    "movie",
		MediaPath:   "/future",
		State:       stateScrapeRetrying,
		MaxRetries:  3,
		RetryCount:  1,
		NextRetryAt: &future,
	}
	require.NoError(t, db.Create(&rec).Error)

	var ran atomic.Int32
	builder := func(r store.ScrapeTask) core.Task {
		return &persistMockTask{
			id:         "retry-wait",
			maxRetries: r.MaxRetries,
			runFunc: func(context.Context, int32) error {
				ran.Add(1)
				return nil
			},
		}
	}

	pq, err := NewPersistentQueue(PersistentConfig{
		DB:                 db,
		BufferSize:         4,
		TaskBuilder:        builder,
		RetryCheckInterval: 50 * time.Millisecond,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, pq.Start(ctx, 1))
	defer pq.Stop()

	// next_retry_at 在未来：retryLoop tick 时应当跳过该任务
	time.Sleep(300 * time.Millisecond)
	assert.Equal(t, int32(0), ran.Load(), "task with future next_retry_at must not run")

	past := time.Now().Add(-1 * time.Minute)
	require.NoError(t, db.Model(&store.ScrapeTask{}).Where("id = ?", rec.ID).Update("next_retry_at", &past).Error)

	require.Eventually(t, func() bool { return ran.Load() >= 1 }, 2*time.Second, 20*time.Millisecond)

	require.Eventually(t, func() bool {
		var r store.ScrapeTask
		if err := db.First(&r, rec.ID).Error; err != nil {
			return false
		}
		return r.State == stateScrapeSuccess
	}, 2*time.Second, 20*time.Millisecond)
}
