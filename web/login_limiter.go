package web

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	// 同一 IP 在 loginFailureWindow 内失败 loginMaxFailures 次后，暂停该 IP 登录 loginLockout。
	loginMaxFailures   = 10
	loginFailureWindow = 15 * time.Minute
	loginLockout       = 15 * time.Minute
	// loginVerifyConcurrency 限制同时进行的口令校验数：每次校验是 10 万次 SHA-256，
	// 不设上限时并发提交错误口令就能把 CPU 占满。
	loginVerifyConcurrency = 4
	// loginMaxTrackedIPs 是记录失败次数的 IP 上限，超出时先清理过期记录，仍超出就整体重置。
	loginMaxTrackedIPs = 4096
	// loginMaxBodyBytes 是登录请求体上限。
	loginMaxBodyBytes = 64 << 10
)

// loginLimiter 按客户端 IP 统计登录失败次数，并限制口令校验的并发数。nil 时不做任何限制。
type loginLimiter struct {
	mu       sync.Mutex
	failures map[string]*loginFailure
	now      func() time.Time
	verify   chan struct{}
}

type loginFailure struct {
	count       int
	windowStart time.Time
	lockedUntil time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{
		failures: map[string]*loginFailure{},
		now:      time.Now,
		verify:   make(chan struct{}, loginVerifyConcurrency),
	}
}

// blocked 返回该 IP 当前是否被暂停登录，以及还要等多久。
func (l *loginLimiter) blocked(ip string) (time.Duration, bool) {
	if l == nil {
		return 0, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	f, ok := l.failures[ip]
	if !ok {
		return 0, false
	}
	if wait := f.lockedUntil.Sub(l.now()); wait > 0 {
		return wait, true
	}
	return 0, false
}

// fail 记一次失败；达到上限时开始暂停。
func (l *loginLimiter) fail(ip string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	f, ok := l.failures[ip]
	if !ok {
		if len(l.failures) >= loginMaxTrackedIPs {
			l.pruneLocked(now)
			if len(l.failures) >= loginMaxTrackedIPs {
				l.failures = map[string]*loginFailure{}
			}
		}
		f = &loginFailure{windowStart: now}
		l.failures[ip] = f
	}
	if now.Sub(f.windowStart) > loginFailureWindow {
		f.count, f.windowStart = 0, now
	}
	f.count++
	if f.count >= loginMaxFailures {
		f.lockedUntil = now.Add(loginLockout)
		f.count, f.windowStart = 0, now
	}
}

// succeed 登录成功后清掉该 IP 的失败记录。
func (l *loginLimiter) succeed(ip string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, ip)
}

// acquireVerify 占用一个口令校验名额，请求取消时放弃等待。
func (l *loginLimiter) acquireVerify(ctx context.Context) (func(), error) {
	if l == nil {
		return func() {}, nil
	}
	select {
	case l.verify <- struct{}{}:
		return func() { <-l.verify }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (l *loginLimiter) pruneLocked(now time.Time) {
	for ip, f := range l.failures {
		if now.Sub(f.windowStart) > loginFailureWindow && !now.Before(f.lockedUntil) {
			delete(l.failures, ip)
		}
	}
}

// clientIP 取连接的对端地址。不读 X-Forwarded-For：没有反向代理时这个头可以随意伪造。
// 部署在反向代理之后时，所有请求都来自代理地址，失败次数也会合并计算。
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
