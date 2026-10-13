package cloakdriver

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/remote"
)

// CDPSession wraps a chromedp browser context bound to a CloakBrowser
// profile's CDP WebSocket URL (returned by Manager's POST
// /api/profiles/{id}/launch). Sessions are stateful; callers must Close()
// to release allocator and task goroutines.
type CDPSession struct {
	allocCtx    context.Context
	allocCancel context.CancelFunc
	taskCtx     context.Context
	taskCancel  context.CancelFunc

	closeOnce sync.Once
}

// NewCDPSession opens a chromedp remote allocator connection to cdpURL
// and verifies the connection with a no-op chromedp.Do within a 5s
// timeout. The returned session must be Close()d.
func NewCDPSession(parent context.Context, cdpURL string) (*CDPSession, error) {
	allocCtx, allocCancel := remote.NewAllocator(parent, cdpURL)
	taskCtx, taskCancel := chromedp.NewContext(allocCtx)

	// chromedp 把第一次执行用的 ctx 当作标签页的生命周期：在它上面套超时并在返回时取消，
	// 会把刚建好的标签页一起关掉，之后每次执行都报 context canceled。所以第一次 Do 直接用 taskCtx，
	// 连接时限由计时器负责。
	connected := make(chan error, 1)
	go func() { connected <- chromedp.Do(taskCtx) }()
	timer := time.NewTimer(cdpConnectTimeout)
	defer timer.Stop()
	var err error
	select {
	case err = <-connected:
	case <-timer.C:
		err = context.DeadlineExceeded
	}
	if err != nil {
		taskCancel()
		allocCancel()
		return nil, fmt.Errorf("cdp connect failed: %w", err)
	}
	return &CDPSession{
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		taskCtx:     taskCtx,
		taskCancel:  taskCancel,
	}, nil
}

// cdpConnectTimeout 是 NewCDPSession 连上浏览器并建好标签页的时限。
const cdpConnectTimeout = 5 * time.Second

// Close releases allocator and task contexts. Safe to call multiple times.
func (s *CDPSession) Close() {
	s.closeOnce.Do(func() {
		if s.taskCancel != nil {
			s.taskCancel()
		}
		if s.allocCancel != nil {
			s.allocCancel()
		}
	})
}

// TaskContext returns the underlying chromedp task context for advanced
// callers (T11-T14 schema drivers may need direct chromedp.Do access).
func (s *CDPSession) TaskContext() context.Context {
	return s.taskCtx
}

// InjectCookies enforces R35: clear all browser cookies first, then set
// the supplied cookies. This prevents stale-cookie pollution from prior
// sessions in the persistent CloakBrowser profile.
//
// 从站点配置的 Cookie 串解析出的 Cookie 没有 Domain 与 Path，这类 Cookie 按 targetURL 设置
// （Chrome 由 url 推出域名），Path 取 "/"；否则 Network.setCookie 会因缺少 url 与 domain 而失败。
func (s *CDPSession) InjectCookies(ctx context.Context, targetURL string, cookies []*http.Cookie) error {
	return chromedp.Do(ctx, chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
		if _, err := cdp.Call(ctx, t, network.ClearBrowserCookies, cdp.Empty{}); err != nil {
			return fmt.Errorf("clear cookies: %w", err)
		}
		for _, c := range cookies {
			if c == nil {
				continue
			}
			path := c.Path
			if path == "" {
				path = "/"
			}
			httpOnly, secure := c.HttpOnly, c.Secure
			params := network.SetCookieParams{
				Name:     c.Name,
				Value:    c.Value,
				Path:     path,
				HTTPOnly: &httpOnly,
				Secure:   &secure,
			}
			if c.Domain != "" {
				params.Domain = c.Domain
			} else {
				params.URL = targetURL
			}
			if _, err := cdp.Call(ctx, t, network.SetCookie, params); err != nil {
				return fmt.Errorf("set cookie %s: %w", c.Name, err)
			}
		}
		return nil
	}))
}

// GetCookies returns all cookies currently in the browser for the given URLs.
func (s *CDPSession) GetCookies(ctx context.Context, urls ...string) ([]*network.Cookie, error) {
	res, err := chromedp.Call(ctx, network.GetCookies, network.GetCookiesParams{URLs: urls})
	if err != nil {
		return nil, err
	}
	return res.Cookies, nil
}

// LoadPageAndExtract navigates to targetURL, waits for waitForSelector to
// become visible, and runs extract with the rendered page's outer HTML.
// Used by schema-specific drivers (T11-T14) to parse user/profile pages.
func (s *CDPSession) LoadPageAndExtract(
	ctx context.Context,
	targetURL string,
	waitForSelector string,
	extract func(htmlOuter string) error,
) error {
	if err := chromedp.Do(ctx,
		chromedp.Navigate(targetURL),
		chromedp.WaitVisible(chromedp.CSS(waitForSelector)),
	); err != nil {
		return err
	}
	html, err := chromedp.Run(ctx, chromedp.OuterHTML(chromedp.CSS("html")))
	if err != nil {
		return err
	}
	return extract(html)
}
