//go:build integration
// +build integration

package cloakdriver

import (
	"context"
	"net/http"
	"os"
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
