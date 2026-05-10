package core

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

// HTTPClientConfig 构造 http.Client 的配置。
// 字段均可选；零值使用合理默认。
type HTTPClientConfig struct {
	// ProxyURL 显式代理 URL（含认证），优先于 env 变量。
	// 支持的 scheme:
	//   - http / https:  普通 HTTP(S) 代理（通过 CONNECT 隧道）
	//   - socks5:        SOCKS5 代理（DNS 本地解析）
	//   - socks5h:       SOCKS5 代理（DNS 远端解析，隐私更好）
	// 示例: "http://user:pass@host:3128", "socks5://127.0.0.1:1080"
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
	defaultDialTimeout     = 10 * time.Second
	defaultKeepAlive       = 30 * time.Second
)

// NewHTTPClient 根据配置构造 http.Client。
//
// 代理优先级：
//  1. cfg.ProxyURL（显式 URL，支持 http/https/socks5/socks5h）
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

	baseDialer := &net.Dialer{Timeout: defaultDialTimeout, KeepAlive: defaultKeepAlive}

	transport := &http.Transport{
		MaxIdleConns:    maxIdle,
		IdleConnTimeout: idleTO,
		DialContext:     baseDialer.DialContext,
		// 默认走 env（ProxyURL 为空时继续生效）
		Proxy: http.ProxyFromEnvironment,
	}

	if cfg.ProxyURL != "" {
		if err := applyProxy(transport, baseDialer, cfg.ProxyURL); err != nil {
			return nil, err
		}
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

// applyProxy 根据 proxyURL 的 scheme 调整 transport 的 Proxy 或 DialContext。
// HTTP/HTTPS 代理通过 transport.Proxy（CONNECT 隧道）。
// SOCKS5 代理通过 transport.DialContext（golang.org/x/net/proxy 实现）。
func applyProxy(transport *http.Transport, baseDialer *net.Dialer, proxyURL string) error {
	u, err := url.Parse(proxyURL)
	if err != nil || u.Host == "" {
		return fmt.Errorf("invalid proxy URL %q: %w", proxyURL, err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		transport.Proxy = http.ProxyURL(u)
		return nil
	case "socks5", "socks5h":
		var auth *proxy.Auth
		if u.User != nil {
			pw, _ := u.User.Password()
			auth = &proxy.Auth{User: u.User.Username(), Password: pw}
		}
		socksDialer, err := proxy.SOCKS5("tcp", u.Host, auth, baseDialer)
		if err != nil {
			return fmt.Errorf("create socks5 dialer: %w", err)
		}
		// proxy.ContextDialer 是可选接口（golang.org/x/net 最新版已实现）。
		if cd, ok := socksDialer.(proxy.ContextDialer); ok {
			transport.DialContext = cd.DialContext
		} else {
			transport.DialContext = func(_ context.Context, network, addr string) (net.Conn, error) {
				return socksDialer.Dial(network, addr)
			}
		}
		// SOCKS5 已在拨号层处理，需显式关闭 HTTP 代理路径避免双重代理。
		transport.Proxy = nil
		return nil
	default:
		return fmt.Errorf("unsupported proxy scheme %q (use http/https/socks5/socks5h)", u.Scheme)
	}
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
