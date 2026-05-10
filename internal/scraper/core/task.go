package core

import "context"

// TaskState 任务生命周期状态。
type TaskState int

const (
	// TaskPending 任务已入队，等待被 worker 取走。
	TaskPending TaskState = iota
	// TaskRunning 任务正在执行。
	TaskRunning
	// TaskSuccess 任务执行成功。
	TaskSuccess
	// TaskFailed 任务最终失败（已达最大重试次数或不可重试错误）。
	TaskFailed
	// TaskCanceled 任务被 context 取消。
	TaskCanceled
	// TaskRetrying 任务正在等待重试。
	TaskRetrying
)

// String 返回 TaskState 的可读字符串。
func (s TaskState) String() string {
	switch s {
	case TaskPending:
		return "pending"
	case TaskRunning:
		return "running"
	case TaskSuccess:
		return "success"
	case TaskFailed:
		return "failed"
	case TaskCanceled:
		return "canceled"
	case TaskRetrying:
		return "retrying"
	}
	return "unknown"
}

// Task 可调度任务的抽象。
//
// 具体 scraper 任务（movie/tv/episode/bulk）实现此接口。
// 方法的并发安全性由实现方保证（Queue 可能从多个 goroutine 调用状态方法）。
type Task interface {
	// ID 返回任务的唯一标识。调用方负责生成（通常 uuid 或 DB 自增）。
	ID() string
	// Type 返回任务业务类型，如 "movie"/"tv"/"episode"/"bulk"。
	Type() string
	// Run 执行任务主逻辑。Queue 会在 worker goroutine 中调用。
	Run(ctx context.Context) error
	// State 返回当前任务状态。
	State() TaskState
	// RetryCount 返回已重试次数（初始为 0）。
	RetryCount() int
	// MaxRetries 返回最大允许重试次数。
	MaxRetries() int
	// SetState 由 Queue 内部调用更新任务状态。
	SetState(s TaskState)
	// IncrementRetry 将 RetryCount +1。
	IncrementRetry()
	// LastError 返回最后一次 Run 返回的错误（如果有）。
	LastError() error
	// SetLastError 保存最后一次 Run 的错误。
	SetLastError(err error)
}
