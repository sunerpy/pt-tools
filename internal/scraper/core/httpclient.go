package core

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// HTTPClientConfig 构造 http.Client 的配置。
// 字段均可选；零值使用合理默认。
type HTTPClientConfig struct {
	// ProxyURL 显式代理 URL（含认证），优先于 env 变量。
	// 示例: "http://user:pass@host:3128"
	ProxyURL string

	// Timeout 单个请求超时，默认 15s。
	Timeout time.Duration

	// MaxIdleConns 连接池大小，默认 10。
	MaxIdleConns int

	// IdleConnTimeout 空闲连接超时，默认 30s。
	IdleConnTimeout time.Duration

	// UserAgent 覆盖默认 User-Agent（可选）。
	UserAgent string

	// SkipTLSVerify 跳过 TLS 校验（**仅用于测试**）。
	SkipTLSVerify bool
}

const (
	defaultTimeout         = 15 * time.Second
	defaultMaxIdleConns    = 10
	defaultIdleConnTimeout = 30 * time.Second
)

// NewHTTPClient 根据配置构造 http.Client。
//
// 代理优先级：
//  1. cfg.ProxyURL（显式 URL）
//  2. HTTP_PROXY / HTTPS_PROXY / NO_PROXY 环境变量（http.ProxyFromEnvironment）
//
// 返回的 Client 可安全并发使用。
func NewHTTPClient(cfg HTTPClientConfig) (*http.Client, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = defaultMaxIdleConns
	}
	idleTO := cfg.IdleConnTimeout
	if idleTO <= 0 {
		idleTO = defaultIdleConnTimeout
	}

	transport := &http.Transport{
		MaxIdleConns:    maxIdle,
		IdleConnTimeout: idleTO,
		// 默认走 env（ProxyURL 为空时继续生效）
		Proxy: http.ProxyFromEnvironment,
	}

	if cfg.ProxyURL != "" {
		u, err := url.Parse(cfg.ProxyURL)
		if err != nil || u.Host == "" {
			return nil, fmt.Errorf("invalid proxy URL %q: %w", cfg.ProxyURL, err)
		}
		// 显式 URL 覆盖 env
		transport.Proxy = http.ProxyURL(u)
	}

	if cfg.SkipTLSVerify {
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = newInsecureTLSConfig()
		}
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: newUARoundTripper(transport, cfg.UserAgent),
	}
	return client, nil
}

func newInsecureTLSConfig() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true} //nolint:gosec // test only
}

// uaRoundTripper 在 User-Agent 缺失时注入默认值。
type uaRoundTripper struct {
	base http.RoundTripper
	ua   string
}

func newUARoundTripper(base http.RoundTripper, ua string) http.RoundTripper {
	if ua == "" {
		return base
	}
	return &uaRoundTripper{base: base, ua: ua}
}

func (r *uaRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", r.ua)
	}
	return r.base.RoundTrip(req)
}
