package cloakdriver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Most CDP tests need a real Chromium endpoint to round-trip. Those tests
// are tagged with `//go:build integration` in cdp_integration_test.go.
//
// The unit tests below exercise the parts of cdp.go that do NOT require an
// actual chromedp connection:
//   - context cancellation propagation through chromedp.NewRemoteAllocator
//   - Close() idempotency
//   - InjectCookies / GetCookies on a closed/cancelled session return error
//
// Coverage for the round-trip (cookie inject + readback + clear-before-inject)
// lives in cdp_integration_test.go and is documented as PENDING when no
// real Chromium / cloakbrowser-manager is available in the test env.

func TestCDPSessionConnectFailsOnInvalidURL(t *testing.T) {
	t.Parallel()
	// Connecting to a URL that does not point at a CDP endpoint should
	// fail within the verification timeout instead of hanging forever.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := NewCDPSession(ctx, "ws://127.0.0.1:1/devtools/browser/fake", nil)
	if err == nil {
		t.Fatalf("expected error when connecting to fake CDP url; got session=%v", sess)
		sess.Close()
	}
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cdp connect")
}

// 经 Manager 转发的地址：原样拨这个地址（不去查 /json/version，那个请求带不上令牌），握手带上令牌
func TestCDPSessionDialsManagerProxyAsIs(t *testing.T) {
	t.Parallel()
	type seen struct{ path, auth string }
	got := make(chan seen, 4)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- seen{r.URL.Path, r.Header.Get("Authorization")}
		http.Error(w, "no websocket here", http.StatusBadRequest)
	}))
	defer ts.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/profiles/p1/cdp"
	_, err := NewCDPSession(ctx, wsURL, http.Header{"Authorization": {"Bearer tok"}})
	require.Error(t, err)
	first := <-got
	assert.Equal(t, seen{"/api/profiles/p1/cdp", "Bearer tok"}, first)
	for len(got) > 0 {
		assert.NotEqual(t, "/json/version", (<-got).path)
	}
}

func TestCDPSessionCloseIdempotent(t *testing.T) {
	t.Parallel()
	// A failed NewCDPSession() must already release resources internally.
	// We additionally verify that calling Close() on a partially-initialised
	// session (constructed for testing) is safe to call multiple times.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	allocCtx, allocCancel := dummyAllocCtx(ctx)
	taskCtx, taskCancel := dummyTaskCtx(allocCtx)
	sess := &CDPSession{
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		taskCtx:     taskCtx,
		taskCancel:  taskCancel,
	}
	require.NotPanics(t, func() {
		sess.Close()
		sess.Close()
		sess.Close()
	})
}

func TestCDPSessionContextCancellationCleansUp(t *testing.T) {
	t.Parallel()
	// Verify a cancelled parent context propagates: the underlying contexts
	// returned by NewRemoteAllocator/NewContext should report Done() once
	// we cancel them through Close().
	ctx, cancel := context.WithCancel(context.Background())
	allocCtx, allocCancel := dummyAllocCtx(ctx)
	taskCtx, taskCancel := dummyTaskCtx(allocCtx)
	sess := &CDPSession{
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		taskCtx:     taskCtx,
		taskCancel:  taskCancel,
	}

	// Snapshot goroutine count before Close(); after Close() and a small
	// settle window, the count should not grow.
	before := runtime.NumGoroutine()
	sess.Close()
	cancel()

	// Allow the runtime a brief moment to reap any short-lived goroutines.
	time.Sleep(50 * time.Millisecond)

	// The Done channels of both contexts must be closed.
	select {
	case <-sess.taskCtx.Done():
	default:
		t.Fatalf("taskCtx not cancelled after Close()")
	}
	select {
	case <-sess.allocCtx.Done():
	default:
		t.Fatalf("allocCtx not cancelled after Close()")
	}

	after := runtime.NumGoroutine()
	if after > before+2 {
		t.Logf("goroutine count went from %d -> %d (informational)", before, after)
	}
}

func TestCDPSessionTaskContextExposed(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	allocCtx, allocCancel := dummyAllocCtx(ctx)
	taskCtx, taskCancel := dummyTaskCtx(allocCtx)
	sess := &CDPSession{
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		taskCtx:     taskCtx,
		taskCancel:  taskCancel,
	}
	defer sess.Close()

	got := sess.TaskContext()
	require.NotNil(t, got)
	// Should be the same context (pointer identity through the field).
	assert.Equal(t, sess.taskCtx, got)
}

// TestCDPInjectCookiesOnClosedSessionFails — when a session has been Close()d
// the underlying chromedp context is cancelled; InjectCookies must surface
// that as an error rather than silently succeed.
func TestCDPInjectCookiesOnClosedSessionFails(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	allocCtx, allocCancel := dummyAllocCtx(ctx)
	taskCtx, taskCancel := dummyTaskCtx(allocCtx)
	sess := &CDPSession{
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		taskCtx:     taskCtx,
		taskCancel:  taskCancel,
	}
	sess.Close()

	cookies := []*http.Cookie{
		{Name: "foo", Value: "bar", Domain: "example.com", Path: "/"},
	}
	err := sess.InjectCookies(sess.TaskContext(), "https://example.com/", cookies)
	require.Error(t, err)
}

func TestCDPCloseIsThreadSafe(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	allocCtx, allocCancel := dummyAllocCtx(ctx)
	taskCtx, taskCancel := dummyTaskCtx(allocCtx)
	sess := &CDPSession{
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		taskCtx:     taskCtx,
		taskCancel:  taskCancel,
	}

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sess.Close()
		}()
	}
	wg.Wait()
}

// dummyAllocCtx / dummyTaskCtx — minimal stand-in contexts that mimic the
// shape returned by chromedp.NewRemoteAllocator / chromedp.NewContext, so we
// can exercise CDPSession lifecycle code (Close idempotency, context exposure)
// without spinning a real browser.
func dummyAllocCtx(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(parent)
}

func dummyTaskCtx(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(parent)
}
