package service

import "time"

// ExponentialBackoff 三级退避：5s / 30s / 3min。
//
// attempt=0 → 0, attempt=1 → 5s, attempt=2 → 30s, attempt>=3 → 3min。
func ExponentialBackoff(attempt int) time.Duration {
	switch {
	case attempt <= 0:
		return 0
	case attempt == 1:
		return 5 * time.Second
	case attempt == 2:
		return 30 * time.Second
	default:
		return 3 * time.Minute
	}
}

// ShouldRetry 判断一次失败后是否应继续重试。
//
// 不重试的情况：err 为 nil（没有错误），或 currentAttempt 已达 maxRetries。
// provider 特定的不可重试错误（如 auth failed）由 Task.Run 自己处理。
func ShouldRetry(err error, currentAttempt, maxRetries int) bool {
	if err == nil {
		return false
	}
	if currentAttempt >= maxRetries {
		return false
	}
	return true
}
