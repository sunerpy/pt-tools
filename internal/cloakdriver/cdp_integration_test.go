//go:build integration
// +build integration

package cloakdriver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests require a real Chromium endpoint reachable at
// CLOAKDRIVER_CDP_URL (e.g. ws://localhost:9222/devtools/browser/<id> or
// the cdp_url returned by cloakhq/cloakbrowser-manager:0.0.4 launch API).
// Run with: go test -tags integration ./internal/cloakdriver/...

func cdpURLOrSkip(t *testing.T) string {
	t.Helper()
	u := os.Getenv("CLOAKDRIVER_CDP_URL")
	if u == "" {
		t.Skip("CLOAKDRIVER_CDP_URL not set; skipping integration test")
	}
	return u
}

func TestCDPCookieInjectionRoundTrip(t *testing.T) {
	cdpURL := cdpURLOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sess, err := NewCDPSession(ctx, cdpURL)
	require.NoError(t, err)
	defer sess.Close()

	cookies := []*http.Cookie{
		{Name: "PHPSESSID", Value: "abc123", Domain: "example.com", Path: "/", HttpOnly: true},
		{Name: "remember", Value: "yes", Domain: "example.com", Path: "/", Secure: true},
	}
	require.NoError(t, sess.InjectCookies(sess.TaskContext(), "https://example.com/", cookies))

	got, err := sess.GetCookies(sess.TaskContext(), "https://example.com/")
	require.NoError(t, err)

	byName := map[string]string{}
	httpOnlyByName := map[string]bool{}
	for _, c := range got {
		byName[c.Name] = c.Value
		httpOnlyByName[c.Name] = c.HTTPOnly
	}
	assert.Equal(t, "abc123", byName["PHPSESSID"])
	assert.Equal(t, "yes", byName["remember"])
	assert.True(t, httpOnlyByName["PHPSESSID"], "PHPSESSID HttpOnly must be preserved")
}

func TestCDPClearBeforeInject(t *testing.T) {
	cdpURL := cdpURLOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sess, err := NewCDPSession(ctx, cdpURL)
	require.NoError(t, err)
	defer sess.Close()

	stale := []*http.Cookie{
		{Name: "PHPSESSID", Value: "old", Domain: "example.com", Path: "/", HttpOnly: true},
	}
	require.NoError(t, sess.InjectCookies(sess.TaskContext(), "https://example.com/", stale))

	fresh := []*http.Cookie{
		{Name: "PHPSESSID", Value: "new", Domain: "example.com", Path: "/", HttpOnly: true},
	}
	require.NoError(t, sess.InjectCookies(sess.TaskContext(), "https://example.com/", fresh))

	got, err := sess.GetCookies(sess.TaskContext(), "https://example.com/")
	require.NoError(t, err)

	var values []string
	for _, c := range got {
		if c.Name == "PHPSESSID" {
			values = append(values, c.Value)
		}
	}
	assert.Contains(t, values, "new", "new cookie must be present")
	assert.NotContains(t, values, "old", "stale cookie must have been cleared (R35)")
}

func TestCDPContextCancellationIntegration(t *testing.T) {
	cdpURL := cdpURLOrSkip(t)
	parent, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sess, err := NewCDPSession(parent, cdpURL)
	require.NoError(t, err)
	defer sess.Close()

	deadlined, dlCancel := context.WithTimeout(sess.TaskContext(), 1*time.Second)
	defer dlCancel()

	err = sess.LoadPageAndExtract(deadlined, "https://example.com/", "body", func(html string) error {
		return nil
	})
	if err == nil {
		t.Logf("page loaded under 1s; this is acceptable but cannot verify deadline propagation")
		return
	}
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

// M1d：后备注入的 Cookie 来自站点配置里的 Cookie 串（http.ParseCookie），没有 Domain 与 Path，
// 必须按要打开的地址设置，否则 Chrome 拒绝 Network.setCookie。
func TestCDPCookieInjectionFromCookieHeader(t *testing.T) {
	cdpURL := cdpURLOrSkip(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sess, err := NewCDPSession(ctx, cdpURL)
	require.NoError(t, err)
	defer sess.Close()

	cookies, err := http.ParseCookie("c_secure_uid=93012; c_secure_pass=abc")
	require.NoError(t, err)
	target := "https://tracker.example/userdetails.php?id=93012"
	require.NoError(t, sess.InjectCookies(sess.TaskContext(), target, cookies))

	got, err := sess.GetCookies(sess.TaskContext(), target)
	require.NoError(t, err)
	byName := map[string]string{}
	for _, c := range got {
		byName[c.Name] = c.Value
		assert.Equal(t, "tracker.example", c.Domain, c.Name)
		assert.Equal(t, "/", c.Path, c.Name)
	}
	assert.Equal(t, "93012", byName["c_secure_uid"])
	assert.Equal(t, "abc", byName["c_secure_pass"])
}

// 打开页面、等选择器出现、取整页 HTML，并且请求带上注入的 Cookie：探测驱动走的就是这条路
// （chromedp v0.21 换成泛型接口以后 Navigate、WaitVisible、OuterHTML 都改了写法）。
func TestCDPLoadPageAndExtractSendsInjectedCookies(t *testing.T) {
	cdpURL := cdpURLOrSkip(t)
	gotCookie := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/favicon.ico" {
			http.NotFound(w, r)
			return
		}
		gotCookie <- r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><div id="info">用户 93012</div></body></html>`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	sess, err := NewCDPSession(ctx, cdpURL)
	require.NoError(t, err)
	defer sess.Close()

	cookies, err := http.ParseCookie("c_secure_uid=93012; c_secure_pass=abc")
	require.NoError(t, err)
	target := srv.URL + "/userdetails.php?id=93012"
	require.NoError(t, sess.InjectCookies(sess.TaskContext(), target, cookies))

	var html string
	require.NoError(t, sess.LoadPageAndExtract(sess.TaskContext(), target, "#info", func(h string) error {
		html = h
		return nil
	}))
	assert.Contains(t, html, `<div id="info">用户 93012</div>`)
	cookie := <-gotCookie
	assert.True(t, strings.Contains(cookie, "c_secure_uid=93012") && strings.Contains(cookie, "c_secure_pass=abc"), cookie)
}
