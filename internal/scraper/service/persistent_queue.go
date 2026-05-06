package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
	"github.com/sunerpy/pt-tools/internal/scraper/store"
)

// 持久化任务状态常量（对应 store.ScrapeTask.State）。
const (
	stateScrapePending  = "pending"
	stateScrapeRunning  = "running"
	stateScrapeSuccess  = "success"
	stateScrapeFailed   = "failed"
	stateScrapeRetrying = "retrying"
)

// defaultRetryCheckInterval 默认 retry 轮询间隔。
const defaultRetryCheckInterval = 15 * time.Second

// PersistentQueue 在 T4 内存 Queue 基础上叠加 GORM 持久化层。
//
// 特性：
//   - Enqueue 先写 DB（state=pending），后入内存 channel
//   - 状态转换同步到 ScrapeTask 记录（running/success/failed/retrying）
//   - 重启时从 DB 恢复 pending/retrying 任务重新入队
//   - 失败重试：指数退避 NextRetryAt 持久化；retryLoop 定期扫描到期任务
//   - 超 MaxRetries 永久 Failed
//   - 通过 inFlight 去重，避免 recover + retryLoop 重复入队
type PersistentQueue struct {
	mem *Queue
	db  *gorm.DB

	// taskBuilder 从 ScrapeTask 记录构造 core.Task 实例（业务层注入）。
	taskBuilder func(record store.ScrapeTask) core.Task

	retryInterval time.Duration

	mu       sync.Mutex
	inFlight map[string]bool

	wg     sync.WaitGroup
	cancel context.CancelFunc
}

// PersistentConfig 创建 PersistentQueue 的配置。
type PersistentConfig struct {
	DB                 *gorm.DB
	BufferSize         int
	TaskBuilder        func(store.ScrapeTask) core.Task
	RetryCheckInterval time.Duration
}

// NewPersistentQueue 构造一个持久化队列。未调用 Start 之前不会消费任何任务。
func NewPersistentQueue(cfg PersistentConfig) (*PersistentQueue, error) {
	if cfg.DB == nil {
		return nil, errors.New("nil DB")
	}
	if cfg.TaskBuilder == nil {
		return nil, errors.New("nil TaskBuilder")
	}
	interval := cfg.RetryCheckInterval
	if interval <= 0 {
		interval = defaultRetryCheckInterval
	}
	return &PersistentQueue{
		mem:           NewQueue(cfg.BufferSize),
		db:            cfg.DB,
		taskBuilder:   cfg.TaskBuilder,
		retryInterval: interval,
		inFlight:      map[string]bool{},
	}, nil
}

// Start 启动内存 worker pool，恢复未完成任务并开启 retry 轮询。
func (pq *PersistentQueue) Start(ctx context.Context, workers int) error {
	if err := pq.mem.Start(ctx, workers); err != nil {
		return err
	}

	loopCtx, cancel := context.WithCancel(ctx)
	pq.cancel = cancel

	if err := pq.recover(loopCtx); err != nil {
		return fmt.Errorf("recover: %w", err)
	}

	pq.wg.Add(1)
	go pq.retryLoop(loopCtx)
	return nil
}

// Stop 取消 retry loop，关闭内存队列。可重复调用。
func (pq *PersistentQueue) Stop() {
	if pq.cancel != nil {
		pq.cancel()
	}
	pq.wg.Wait()
	pq.mem.Stop()
}

// Processed 透传内存 Queue 的计数。
func (pq *PersistentQueue) Processed() int64 { return pq.mem.Processed() }

// Failed 透传内存 Queue 的计数。
func (pq *PersistentQueue) Failed() int64 { return pq.mem.Failed() }

// Enqueue 持久化任务 + 入队。requestData 将 JSON 序列化存入 ScrapeTask.RequestData，
// 用于重启后恢复原始请求。
func (pq *PersistentQueue) Enqueue(
	ctx context.Context,
	taskType, mediaPath string,
	libraryID *uint,
	requestData any,
) (*store.ScrapeTask, error) {
	reqJSON, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	record := store.ScrapeTask{
		LibraryID:   libraryID,
		TaskType:    taskType,
		MediaPath:   mediaPath,
		State:       stateScrapePending,
		MaxRetries:  3,
		RequestData: string(reqJSON),
	}
	if err := pq.db.Create(&record).Error; err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}

	task := pq.taskBuilder(record)
	if task == nil {
		pq.db.Model(&store.ScrapeTask{}).Where("id = ?", record.ID).Update("state", stateScrapeFailed)
		return nil, errors.New("taskBuilder returned nil")
	}

	key := pq.inFlightKey(task.ID(), record.ID)
	pq.markInFlight(key)
	if err := pq.mem.Enqueue(ctx, pq.wrapTask(task, record.ID)); err != nil {
		pq.unmarkInFlight(key)
		pq.db.Model(&store.ScrapeTask{}).Where("id = ?", record.ID).Update("state", stateScrapeFailed)
		return nil, err
	}
	return &record, nil
}

// recover 重启后把 DB 中 pending / retrying 任务重新入队。
// 对于 retrying 任务，仅恢复 next_retry_at 已到期（或未设置）的。
// 幂等：若某 id 已在 inFlight 中则跳过。
func (pq *PersistentQueue) recover(ctx context.Context) error {
	var records []store.ScrapeTask
	now := time.Now()
	if err := pq.db.
		Where("state = ? OR (state = ? AND (next_retry_at IS NULL OR next_retry_at <= ?))",
			stateScrapePending, stateScrapeRetrying, now).
		Find(&records).Error; err != nil {
		return err
	}
	for _, r := range records {
		task := pq.taskBuilder(r)
		if task == nil {
			continue
		}
		key := pq.inFlightKey(task.ID(), r.ID)
		if pq.isInFlight(key) {
			continue
		}
		pq.markInFlight(key)
		// 软入队：若 channel 满 / ctx 取消，不阻塞启动流程
		if err := pq.mem.Enqueue(ctx, pq.wrapTask(task, r.ID)); err != nil {
			pq.unmarkInFlight(key)
		}
	}
	return nil
}

// retryLoop 周期扫描 next_retry_at <= now 的 retrying 任务，重新入队。
func (pq *PersistentQueue) retryLoop(ctx context.Context) {
	defer pq.wg.Done()
	t := time.NewTicker(pq.retryInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pq.scanDueRetries(ctx)
		}
	}
}

// scanDueRetries 单次扫描；与 retryLoop 拆分便于测试触发。
func (pq *PersistentQueue) scanDueRetries(ctx context.Context) {
	var due []store.ScrapeTask
	if err := pq.db.
		Where("state = ? AND next_retry_at <= ?", stateScrapeRetrying, time.Now()).
		Find(&due).Error; err != nil {
		return
	}
	for _, r := range due {
		task := pq.taskBuilder(r)
		if task == nil {
			continue
		}
		key := pq.inFlightKey(task.ID(), r.ID)
		if pq.isInFlight(key) {
			continue
		}
		pq.markInFlight(key)
		if err := pq.mem.Enqueue(ctx, pq.wrapTask(task, r.ID)); err != nil {
			pq.unmarkInFlight(key)
		}
	}
}

// wrapTask 用 persistentTask 包装原 core.Task，在 Run 前后更新 DB 状态。
func (pq *PersistentQueue) wrapTask(t core.Task, dbID uint) core.Task {
	return &persistentTask{Task: t, dbID: dbID, pq: pq}
}

func (pq *PersistentQueue) inFlightKey(taskID string, dbID uint) string {
	return taskID + "#" + strconv.FormatUint(uint64(dbID), 10)
}

func (pq *PersistentQueue) markInFlight(key string) {
	pq.mu.Lock()
	pq.inFlight[key] = true
	pq.mu.Unlock()
}

func (pq *PersistentQueue) unmarkInFlight(key string) {
	pq.mu.Lock()
	delete(pq.inFlight, key)
	pq.mu.Unlock()
}

func (pq *PersistentQueue) isInFlight(key string) bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	return pq.inFlight[key]
}

// persistentTask 包装 core.Task，Run 前后更新 DB。
type persistentTask struct {
	core.Task
	dbID uint
	pq   *PersistentQueue
}

// Run 更新 DB 状态为 running → 执行 → 成功 / 失败 / 重试。
func (p *persistentTask) Run(ctx context.Context) error {
	now := time.Now()
	p.pq.db.Model(&store.ScrapeTask{}).Where("id = ?", p.dbID).Updates(map[string]any{
		"state":      stateScrapeRunning,
		"started_at": &now,
	})

	err := p.Task.Run(ctx)

	key := p.pq.inFlightKey(p.ID(), p.dbID)

	if err == nil {
		completed := time.Now()
		p.pq.db.Model(&store.ScrapeTask{}).Where("id = ?", p.dbID).Updates(map[string]any{
			"state":        stateScrapeSuccess,
			"completed_at": &completed,
			"progress":     float64(100),
		})
		p.pq.unmarkInFlight(key)
		return nil
	}

	// 失败：判断是否继续重试
	if p.RetryCount() >= p.MaxRetries() {
		completed := time.Now()
		p.pq.db.Model(&store.ScrapeTask{}).Where("id = ?", p.dbID).Updates(map[string]any{
			"state":        stateScrapeFailed,
			"completed_at": &completed,
			"last_error":   err.Error(),
		})
		p.pq.unmarkInFlight(key)
		return err
	}

	// 安排重试
	nextAttempt := p.RetryCount() + 1
	backoff := ExponentialBackoff(nextAttempt)
	nextAt := time.Now().Add(backoff)
	p.pq.db.Model(&store.ScrapeTask{}).Where("id = ?", p.dbID).Updates(map[string]any{
		"state":         stateScrapeRetrying,
		"retry_count":   nextAttempt,
		"next_retry_at": &nextAt,
		"last_error":    err.Error(),
	})
	p.pq.unmarkInFlight(key)
	return err
}
