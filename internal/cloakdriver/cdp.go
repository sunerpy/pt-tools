package cloakdriver

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
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

// NewCDPSession opens a chromedp NewRemoteAllocator connection to cdpURL
// and verifies the connection with a no-op chromedp.Run within a 5s
// timeout. The returned session must be Close()d.
func NewCDPSession(parent context.Context, cdpURL string) (*CDPSession, error) {
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(parent, cdpURL)
	taskCtx, taskCancel := chromedp.NewContext(allocCtx)

	verifyCtx, verifyCancel := context.WithTimeout(taskCtx, 5*time.Second)
	defer verifyCancel()
	if err := chromedp.Run(verifyCtx); err != nil {
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
// callers (T11-T14 schema drivers may need direct chromedp.Run access).
func (s *CDPSession) TaskContext() context.Context {
	return s.taskCtx
}

// InjectCookies enforces R35: clear all browser cookies first, then set
// the supplied cookies. This prevents stale-cookie pollution from prior
// sessions in the persistent CloakBrowser profile.
func (s *CDPSession) InjectCookies(ctx context.Context, _ string, cookies []*http.Cookie) error {
	return chromedp.Run(
		ctx,
		network.ClearBrowserCookies(),
		chromedp.ActionFunc(func(ctx context.Context) error {
			for _, c := range cookies {
				if c == nil {
					continue
				}
				err := network.SetCookie(c.Name, c.Value).
					WithDomain(c.Domain).
					WithPath(c.Path).
					WithHTTPOnly(c.HttpOnly).
					WithSecure(c.Secure).
					Do(ctx)
				if err != nil {
					return fmt.Errorf("set cookie %s: %w", c.Name, err)
				}
			}
			return nil
		}),
	)
}

// GetCookies returns all cookies currently in the browser for the given URLs.
func (s *CDPSession) GetCookies(ctx context.Context, urls ...string) ([]*network.Cookie, error) {
	var cookies []*network.Cookie
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var ferr error
		cookies, ferr = network.GetCookies().WithURLs(urls).Do(ctx)
		return ferr
	}))
	return cookies, err
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
	var html string
	err := chromedp.Run(
		ctx,
		chromedp.Navigate(targetURL),
		chromedp.WaitVisible(waitForSelector, chromedp.ByQuery),
		chromedp.OuterHTML("html", &html),
	)
	if err != nil {
		return err
	}
	return extract(html)
}
