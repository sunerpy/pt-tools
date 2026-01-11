// Package httpclient 提供基于 github.com/sunerpy/requests 的 HTTP 客户端连接池管理
package httpclient

import (
	"context"
	"sync"
	"time"

	"github.com/sunerpy/requests"
)

// PoolConfig 连接池配置
type PoolConfig struct {
	// MaxIdleConns 最大空闲连接数
	MaxIdleConns int
	// MaxIdleConnsPerHost 每个主机的最大空闲连接数
	MaxIdleConnsPerHost int
	// IdleTimeout 空闲连接超时时间
	IdleTimeout time.Duration
	// Timeout 请求超时时间
	Timeout time.Duration
	// ConnectTimeout 连接超时时间
	ConnectTimeout time.Duration
	// EnableKeepAlive 是否启用 Keep-Alive
	EnableKeepAlive bool
	// EnableHTTP2 是否启用 HTTP/2
	EnableHTTP2 bool
	// MaxRetries 最大重试次数
	MaxRetries int
	// RetryDelay 初始重试延迟
	RetryDelay time.Duration
	// MaxRetryDelay 最大重试延迟
	MaxRetryDelay time.Duration
}

// DefaultPoolConfig 返回默认配置
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleTimeout:         90 * time.Second,
		Timeout:             30 * time.Second,
		ConnectTimeout:      10 * time.Second,
		EnableKeepAlive:     true,
		EnableHTTP2:         false,
		MaxRetries:          3,
		RetryDelay:          100 * time.Millisecond,
		MaxRetryDelay:       10 * time.Second,
	}
}

// Pool HTTP 客户端连接池，基于 requests 库
type Pool struct {
	config  PoolConfig
	session requests.Session
	mu      sync.RWMutex
}

var (
	defaultPool     *Pool
	defaultPoolOnce sync.Once
)

// NewPool 创建新的连接池
func NewPool(config PoolConfig) *Pool {
	session := requests.NewSession().
		WithTimeout(config.Timeout).
		WithIdleTimeout(config.IdleTimeout).
		WithMaxIdleConns(config.MaxIdleConns).
		WithKeepAlive(config.EnableKeepAlive).
		WithHTTP2(config.EnableHTTP2)

	return &Pool{
		config:  config,
		session: session,
	}
}

// NewPoolWithDefaults 使用默认配置创建连接池
func NewPoolWithDefaults() *Pool {
	return NewPool(DefaultPoolConfig())
}

// GetDefaultPool 获取默认连接池（单例）
func GetDefaultPool() *Pool {
	defaultPoolOnce.Do(func() {
		defaultPool = NewPool(DefaultPoolConfig())
	})
	return defaultPool
}

// GetSession 获取底层 Session
func (p *Pool) GetSession() requests.Session {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.session
}

// GetConfig 获取连接池配置
func (p *Pool) GetConfig() PoolConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config
}

// AcquireSession 从对象池获取 Session（高性能场景）
// 使用完毕后需调用 ReleaseSession
func AcquireSession() requests.Session {
	return requests.AcquireSession()
}

// ReleaseSession 释放 Session 回对象池
func ReleaseSession(sess requests.Session) {
	requests.ReleaseSession(sess)
}

// Close 关闭连接池
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.session != nil {
		return p.session.Close()
	}
	return nil
}

// GetStats 获取连接池统计信息
func (p *Pool) GetStats() map[string]any {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return map[string]any{
		"max_idle_conns":          p.config.MaxIdleConns,
		"max_idle_conns_per_host": p.config.MaxIdleConnsPerHost,
		"idle_timeout":            p.config.IdleTimeout.String(),
		"timeout":                 p.config.Timeout.String(),
		"connect_timeout":         p.config.ConnectTimeout.String(),
		"enable_keep_alive":       p.config.EnableKeepAlive,
		"enable_http2":            p.config.EnableHTTP2,
		"max_retries":             p.config.MaxRetries,
		"retry_delay":             p.config.RetryDelay.String(),
		"max_retry_delay":         p.config.MaxRetryDelay.String(),
	}
}

// 全局便捷函数

// Close 关闭默认连接池
func Close() error {
	if defaultPool != nil {
		return defaultPool.Close()
	}
	return nil
}

// NewRequestBuilder 创建请求构建器
func NewRequestBuilder(method requests.Method, url string) *requests.RequestBuilder {
	return requests.NewRequestBuilder(method, url)
}

// SetHTTP2Enabled 全局设置是否启用 HTTP/2
func SetHTTP2Enabled(enabled bool) {
	requests.SetHTTP2Enabled(enabled)
}

// IsHTTP2Enabled 检查全局是否启用 HTTP/2
func IsHTTP2Enabled() bool {
	return requests.IsHTTP2Enabled()
}

// NewGet 创建 GET 请求构建器
func NewGet(url string) *requests.RequestBuilder {
	return requests.NewGet(url)
}

// NewPost 创建 POST 请求构建器
func NewPost(url string) *requests.RequestBuilder {
	return requests.NewPost(url)
}

// NewPut 创建 PUT 请求构建器
func NewPut(url string) *requests.RequestBuilder {
	return requests.NewPut(url)
}

// NewDelete 创建 DELETE 请求构建器
func NewDelete(url string) *requests.RequestBuilder {
	return requests.NewDeleteBuilder(url)
}

// NewPatch 创建 PATCH 请求构建器
func NewPatch(url string) *requests.RequestBuilder {
	return requests.NewPatch(url)
}

// ============================================================================
// 请求选项类型导出
// ============================================================================

// RequestOption 请求选项类型
type RequestOption = requests.RequestOption

// ============================================================================
// 请求执行函数
// ============================================================================

// Response 响应接口 - 封装 requests 库的响应
type Response interface {
	StatusCode() int
	Bytes() []byte
	Text() string
	IsSuccess() bool
	IsError() bool
}

// responseWrapper 包装 requests 库的响应
type responseWrapper struct {
	statusCode int
	body       []byte
}

func (r *responseWrapper) StatusCode() int { return r.statusCode }
func (r *responseWrapper) Bytes() []byte   { return r.body }
func (r *responseWrapper) Text() string    { return string(r.body) }
func (r *responseWrapper) IsSuccess() bool { return r.statusCode >= 200 && r.statusCode < 300 }
func (r *responseWrapper) IsError() bool   { return r.statusCode >= 400 }

// Get 发送 GET 请求
func Get(url string, opts ...RequestOption) (Response, error) {
	resp, err := requests.Get(url, opts...)
	if err != nil {
		return nil, err
	}
	return &responseWrapper{statusCode: resp.StatusCode, body: resp.Bytes()}, nil
}

// Post 发送 POST 请求
func Post(url string, body any, opts ...RequestOption) (Response, error) {
	resp, err := requests.Post(url, body, opts...)
	if err != nil {
		return nil, err
	}
	return &responseWrapper{statusCode: resp.StatusCode, body: resp.Bytes()}, nil
}

// Put 发送 PUT 请求
func Put(url string, body any, opts ...RequestOption) (Response, error) {
	resp, err := requests.Put(url, body, opts...)
	if err != nil {
		return nil, err
	}
	return &responseWrapper{statusCode: resp.StatusCode, body: resp.Bytes()}, nil
}

// Delete 发送 DELETE 请求
func Delete(url string, opts ...RequestOption) (Response, error) {
	resp, err := requests.Delete(url, opts...)
	if err != nil {
		return nil, err
	}
	return &responseWrapper{statusCode: resp.StatusCode, body: resp.Bytes()}, nil
}

// Patch 发送 PATCH 请求
func Patch(url string, body any, opts ...RequestOption) (Response, error) {
	resp, err := requests.Patch(url, body, opts...)
	if err != nil {
		return nil, err
	}
	return &responseWrapper{statusCode: resp.StatusCode, body: resp.Bytes()}, nil
}

// ============================================================================
// JSON 请求执行函数（泛型）
// ============================================================================

// Result 泛型结果类型
type Result[T any] struct {
	data       T
	statusCode int
	success    bool
}

// Data 获取解析后的数据
func (r Result[T]) Data() T { return r.data }

// StatusCode 获取状态码
func (r Result[T]) StatusCode() int { return r.statusCode }

// IsSuccess 检查是否成功
func (r Result[T]) IsSuccess() bool { return r.success }

// GetJSON 发送 GET 请求并解析 JSON 响应
func GetJSON[T any](url string, opts ...RequestOption) (Result[T], error) {
	result, err := requests.GetJSON[T](url, opts...)
	if err != nil {
		var zero Result[T]
		return zero, err
	}
	return Result[T]{
		data:       result.Data(),
		statusCode: result.StatusCode(),
		success:    result.IsSuccess(),
	}, nil
}

// PostJSON 发送 POST 请求并解析 JSON 响应
func PostJSON[T any](url string, data any, opts ...RequestOption) (Result[T], error) {
	result, err := requests.PostJSON[T](url, data, opts...)
	if err != nil {
		var zero Result[T]
		return zero, err
	}
	return Result[T]{
		data:       result.Data(),
		statusCode: result.StatusCode(),
		success:    result.IsSuccess(),
	}, nil
}

// PutJSON 发送 PUT 请求并解析 JSON 响应
func PutJSON[T any](url string, data any, opts ...RequestOption) (Result[T], error) {
	result, err := requests.PutJSON[T](url, data, opts...)
	if err != nil {
		var zero Result[T]
		return zero, err
	}
	return Result[T]{
		data:       result.Data(),
		statusCode: result.StatusCode(),
		success:    result.IsSuccess(),
	}, nil
}

// DeleteJSON 发送 DELETE 请求并解析 JSON 响应
func DeleteJSON[T any](url string, opts ...RequestOption) (Result[T], error) {
	result, err := requests.DeleteJSON[T](url, opts...)
	if err != nil {
		var zero Result[T]
		return zero, err
	}
	return Result[T]{
		data:       result.Data(),
		statusCode: result.StatusCode(),
		success:    result.IsSuccess(),
	}, nil
}

// PatchJSON 发送 PATCH 请求并解析 JSON 响应
func PatchJSON[T any](url string, data any, opts ...RequestOption) (Result[T], error) {
	result, err := requests.PatchJSON[T](url, data, opts...)
	if err != nil {
		var zero Result[T]
		return zero, err
	}
	return Result[T]{
		data:       result.Data(),
		statusCode: result.StatusCode(),
		success:    result.IsSuccess(),
	}, nil
}

// ============================================================================
// 请求选项导出
// ============================================================================

// 导出常用请求选项
var (
	WithTimeout     = requests.WithTimeout
	WithHeader      = requests.WithHeader
	WithHeaders     = requests.WithHeaders
	WithQuery       = requests.WithQuery
	WithQueryParams = requests.WithQueryParams
	WithBasicAuth   = requests.WithBasicAuth
	WithBearerToken = requests.WithBearerToken
	WithContext     = requests.WithContext
	WithContentType = requests.WithContentType
	WithAccept      = requests.WithAccept
)

// ============================================================================
// 带重试的请求执行
// ============================================================================

// RetryExecutor 重试执行器
type RetryExecutor struct {
	maxRetries    int
	retryDelay    time.Duration
	maxRetryDelay time.Duration
}

// NewRetryExecutor 创建重试执行器
func NewRetryExecutor(maxRetries int, retryDelay, maxRetryDelay time.Duration) *RetryExecutor {
	return &RetryExecutor{
		maxRetries:    maxRetries,
		retryDelay:    retryDelay,
		maxRetryDelay: maxRetryDelay,
	}
}

// NewRetryExecutorFromConfig 从配置创建重试执行器
func NewRetryExecutorFromConfig(config PoolConfig) *RetryExecutor {
	return NewRetryExecutor(config.MaxRetries, config.RetryDelay, config.MaxRetryDelay)
}

// Execute 执行带重试的请求
func (r *RetryExecutor) Execute(ctx context.Context, fn func() (Response, error)) (Response, error) {
	var lastErr error
	delay := r.retryDelay

	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if attempt > 0 {
			// 检查上下文是否已取消
			if ctx != nil {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(delay):
				}
			} else {
				time.Sleep(delay)
			}
			// 指数退避
			delay = delay * 2
			if delay > r.maxRetryDelay {
				delay = r.maxRetryDelay
			}
		}

		resp, err := fn()
		if err == nil && resp != nil && resp.IsSuccess() {
			return resp, nil
		}
		if err != nil {
			lastErr = err
		}

		// 检查是否为可重试错误
		if !isRetryableError(err, resp) {
			return resp, err
		}
	}

	return nil, lastErr
}

// GetAttemptCount 获取最大尝试次数（初始 + 重试）
func (r *RetryExecutor) GetAttemptCount() int {
	return r.maxRetries + 1
}

// GetMaxRetries 获取最大重试次数
func (r *RetryExecutor) GetMaxRetries() int {
	return r.maxRetries
}

// isRetryableError 判断是否为可重试错误
func isRetryableError(err error, resp Response) bool {
	if err != nil {
		// 网络错误通常可重试
		return true
	}
	if resp != nil {
		// 5xx 服务器错误可重试
		statusCode := resp.StatusCode()
		if statusCode >= 500 && statusCode < 600 {
			return true
		}
	}
	return false
}

// ============================================================================
// 中间件和钩子支持
// ============================================================================

// Middleware 中间件类型
type Middleware = requests.Middleware

// Handler 请求处理器类型
type Handler = requests.Handler

// MiddlewareChain 中间件链类型
type MiddlewareChain = requests.MiddlewareChain

// Hooks 请求钩子类型
type Hooks = requests.Hooks

// 导出中间件和钩子构造函数
var (
	NewMiddlewareChain = requests.NewMiddlewareChain
	NewHooks           = requests.NewHooks
)

// ============================================================================
// 带上下文的请求函数
// ============================================================================

// GetWithContext 发送带上下文的 GET 请求
func GetWithContext(ctx context.Context, url string, opts ...RequestOption) (Response, error) {
	opts = append(opts, requests.WithContext(ctx))
	return Get(url, opts...)
}

// PostWithContext 发送带上下文的 POST 请求
func PostWithContext(ctx context.Context, url string, body any, opts ...RequestOption) (Response, error) {
	opts = append(opts, requests.WithContext(ctx))
	return Post(url, body, opts...)
}

// PutWithContext 发送带上下文的 PUT 请求
func PutWithContext(ctx context.Context, url string, body any, opts ...RequestOption) (Response, error) {
	opts = append(opts, requests.WithContext(ctx))
	return Put(url, body, opts...)
}

// DeleteWithContext 发送带上下文的 DELETE 请求
func DeleteWithContext(ctx context.Context, url string, opts ...RequestOption) (Response, error) {
	opts = append(opts, requests.WithContext(ctx))
	return Delete(url, opts...)
}

// PatchWithContext 发送带上下文的 PATCH 请求
func PatchWithContext(ctx context.Context, url string, body any, opts ...RequestOption) (Response, error) {
	opts = append(opts, requests.WithContext(ctx))
	return Patch(url, body, opts...)
}

// ============================================================================
// 重试策略导出
// ============================================================================

// 导出重试策略函数
var (
	NoRetryPolicy          = requests.NoRetryPolicy
	LinearRetryPolicy      = requests.LinearRetryPolicy
	ExponentialRetryPolicy = requests.ExponentialRetryPolicy
	RetryOn5xx             = requests.RetryOn5xx
	RetryOnNetworkError    = requests.RetryOnNetworkError
	RetryOnStatusCodes     = requests.RetryOnStatusCodes
	CombineRetryConditions = requests.CombineRetryConditions
)

// ============================================================================
// 错误检查函数导出
// ============================================================================

// 导出错误检查函数
var (
	IsTimeout         = requests.IsTimeout
	IsConnectionError = requests.IsConnectionError
	IsResponseError   = requests.IsResponseError
	IsTemporary       = requests.IsTemporary
)

// ============================================================================
// 类型导出
// ============================================================================

// RequestBuilder 请求构建器类型
type RequestBuilder = requests.RequestBuilder

// Method HTTP 方法类型
type Method = requests.Method

// HTTP 方法常量
const (
	MethodGet     = requests.MethodGet
	MethodPost    = requests.MethodPost
	MethodPut     = requests.MethodPut
	MethodDelete  = requests.MethodDelete
	MethodPatch   = requests.MethodPatch
	MethodHead    = requests.MethodHead
	MethodOptions = requests.MethodOptions
)
