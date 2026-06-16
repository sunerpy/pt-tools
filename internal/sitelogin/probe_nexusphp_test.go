package sitelogin

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// fakeSite is a minimal v2.Site implementation used solely for testing
// the ProbeNexusPHP wrapper. Only GetUserInfo carries behaviour; all other
// methods are inert.
type fakeSite struct {
	info v2.UserInfo
	err  error
}

func (f *fakeSite) ID() string                                      { return "fake" }
func (f *fakeSite) Name() string                                    { return "Fake" }
func (f *fakeSite) Kind() v2.SiteKind                               { return v2.SiteNexusPHP }
func (f *fakeSite) Login(_ context.Context, _ v2.Credentials) error { return nil }
func (f *fakeSite) Search(_ context.Context, _ v2.SearchQuery) ([]v2.TorrentItem, error) {
	return nil, nil
}

func (f *fakeSite) GetUserInfo(_ context.Context) (v2.UserInfo, error) {
	return f.info, f.err
}

func (f *fakeSite) Download(_ context.Context, _ string) ([]byte, error) { return nil, nil }
func (f *fakeSite) Close() error                                         { return nil }

func newProbeClock(t time.Time) Clock {
	return NewFakeClock(t)
}

func TestProbeNexusPHPHappyBothFields(t *testing.T) {
	clock := newProbeClock(time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))
	t1 := time.Date(2026, 5, 17, 9, 0, 0, 0, time.UTC).Unix()
	t2 := time.Date(2026, 5, 16, 8, 0, 0, 0, time.UTC).Unix()
	site := &fakeSite{info: v2.UserInfo{LastAccess: t1, LastLogin: t2}}

	res, err := ProbeNexusPHP(context.Background(), site, clock, ProbeSourceHTTPCookie)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, OK, res.Status)
	assert.Equal(t, ProbeSourceHTTPCookie, res.Source)
	require.NotNil(t, res.LastAccessAt)
	assert.Equal(t, t1, res.LastAccessAt.Unix())
	require.NotNil(t, res.LastLoginAt)
	assert.Equal(t, t2, res.LastLoginAt.Unix())
	assert.NoError(t, res.RawError)
}

func TestProbeNexusPHPRousiSource(t *testing.T) {
	clock := newProbeClock(time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))
	t1 := time.Date(2026, 5, 17, 9, 0, 0, 0, time.UTC).Unix()
	site := &fakeSite{info: v2.UserInfo{LastAccess: t1}}

	res, err := ProbeNexusPHP(context.Background(), site, clock, ProbeSourceHTTPAPIKey)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, OK, res.Status)
	assert.Equal(t, ProbeSourceHTTPAPIKey, res.Source)
}

func TestProbeNexusPHPHappyOnlyLastAccess(t *testing.T) {
	clock := newProbeClock(time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))
	t1 := time.Date(2026, 5, 17, 9, 0, 0, 0, time.UTC).Unix()
	site := &fakeSite{info: v2.UserInfo{LastAccess: t1, LastLogin: 0}}

	res, err := ProbeNexusPHP(context.Background(), site, clock, ProbeSourceHTTPCookie)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, OK, res.Status)
	require.NotNil(t, res.LastAccessAt)
	assert.Equal(t, t1, res.LastAccessAt.Unix())
	assert.Nil(t, res.LastLoginAt)
}

func TestProbeNexusPHPParseFailure(t *testing.T) {
	clock := newProbeClock(time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))
	site := &fakeSite{info: v2.UserInfo{LastAccess: 0, LastLogin: 0}}

	res, err := ProbeNexusPHP(context.Background(), site, clock, ProbeSourceHTTPCookie)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, PARSE_ERROR, res.Status)
	assert.Nil(t, res.LastAccessAt)
	assert.Nil(t, res.LastLoginAt)
}

func TestProbeNexusPHPSessionExpired(t *testing.T) {
	clock := newProbeClock(time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))
	site := &fakeSite{err: v2.ErrSessionExpired}

	res, err := ProbeNexusPHP(context.Background(), site, clock, ProbeSourceHTTPCookie)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, SESSION_EXPIRED, res.Status)
	assert.ErrorIs(t, res.RawError, v2.ErrSessionExpired)
}

func TestProbeNexusPHPSessionExpiredWrapped(t *testing.T) {
	clock := newProbeClock(time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))
	wrapped := fmt.Errorf("login: %w", v2.ErrSessionExpired)
	site := &fakeSite{err: wrapped}

	res, err := ProbeNexusPHP(context.Background(), site, clock, ProbeSourceHTTPCookie)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, SESSION_EXPIRED, res.Status)
	assert.ErrorIs(t, res.RawError, v2.ErrSessionExpired)
}

func TestProbeNexusPHPCircuitOpen(t *testing.T) {
	clock := newProbeClock(time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))
	site := &fakeSite{err: v2.ErrCircuitOpen}

	res, err := ProbeNexusPHP(context.Background(), site, clock, ProbeSourceHTTPCookie)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, RATE_LIMITED, res.Status)
	assert.ErrorIs(t, res.RawError, v2.ErrCircuitOpen)
}

func TestProbeNexusPHPNetworkTimeout(t *testing.T) {
	clock := newProbeClock(time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))
	site := &fakeSite{err: context.DeadlineExceeded}

	res, err := ProbeNexusPHP(context.Background(), site, clock, ProbeSourceHTTPCookie)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, NETWORK_ERROR, res.Status)
	assert.ErrorIs(t, res.RawError, context.DeadlineExceeded)
}

func TestProbeNexusPHPCloudflareChallenge(t *testing.T) {
	clock := newProbeClock(time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))
	site := &fakeSite{err: errors.New("Cloudflare 403 Forbidden")}

	res, err := ProbeNexusPHP(context.Background(), site, clock, ProbeSourceHTTPCookie)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, CHALLENGE, res.Status)
	require.Error(t, res.RawError)
}

func TestProbeNexusPHPUnknown(t *testing.T) {
	t.Run("unknown error preserves original message", func(t *testing.T) {
		clock := newProbeClock(time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))
		site := &fakeSite{err: errors.New("weird")}

		res, err := ProbeNexusPHP(context.Background(), site, clock, ProbeSourceHTTPCookie)

		require.NoError(t, err)
		require.NotNil(t, res)
		assert.Equal(t, UNKNOWN, res.Status)
		require.Error(t, res.RawError)
		assert.Contains(t, res.RawError.Error(), "weird")
		assert.Equal(t, "weird", res.Diagnostic)
	})

	t.Run("401 unauthorized error suggests cookie refresh", func(t *testing.T) {
		clock := newProbeClock(time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))
		site := &fakeSite{err: errors.New("HTTP 401 Unauthorized")}

		res, err := ProbeNexusPHP(context.Background(), site, clock, ProbeSourceHTTPCookie)

		require.NoError(t, err)
		require.NotNil(t, res)
		assert.Equal(t, UNKNOWN, res.Status)
		require.Error(t, res.RawError)
		assert.Contains(t, res.Diagnostic, "Cookie")
		assert.Contains(t, res.Diagnostic, "未配置或已失效")
	})

	t.Run("403 forbidden error suggests cookie refresh", func(t *testing.T) {
		clock := newProbeClock(time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))
		site := &fakeSite{err: errors.New("HTTP 403 Forbidden")}

		res, err := ProbeNexusPHP(context.Background(), site, clock, ProbeSourceHTTPCookie)

		require.NoError(t, err)
		require.NotNil(t, res)
		assert.Equal(t, UNKNOWN, res.Status)
		require.Error(t, res.RawError)
		assert.Contains(t, res.Diagnostic, "Cookie")
		assert.Contains(t, res.Diagnostic, "未配置或已失效")
	})

	t.Run("cookie in error message suggests refresh", func(t *testing.T) {
		clock := newProbeClock(time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))
		site := &fakeSite{err: errors.New("invalid cookie format")}

		res, err := ProbeNexusPHP(context.Background(), site, clock, ProbeSourceHTTPCookie)

		require.NoError(t, err)
		require.NotNil(t, res)
		assert.Equal(t, UNKNOWN, res.Status)
		require.Error(t, res.RawError)
		assert.Contains(t, res.Diagnostic, "Cookie")
		assert.Contains(t, res.Diagnostic, "未配置或已失效")
	})

	t.Run("unauthorized keyword suggests cookie refresh", func(t *testing.T) {
		clock := newProbeClock(time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))
		site := &fakeSite{err: errors.New("Request unauthorized")}

		res, err := ProbeNexusPHP(context.Background(), site, clock, ProbeSourceHTTPCookie)

		require.NoError(t, err)
		require.NotNil(t, res)
		assert.Equal(t, UNKNOWN, res.Status)
		require.Error(t, res.RawError)
		assert.Contains(t, res.Diagnostic, "Cookie")
		assert.Contains(t, res.Diagnostic, "未配置或已失效")
	})

	t.Run("login keyword suggests cookie refresh", func(t *testing.T) {
		clock := newProbeClock(time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))
		site := &fakeSite{err: errors.New("please login first")}

		res, err := ProbeNexusPHP(context.Background(), site, clock, ProbeSourceHTTPCookie)

		require.NoError(t, err)
		require.NotNil(t, res)
		assert.Equal(t, UNKNOWN, res.Status)
		require.Error(t, res.RawError)
		assert.Contains(t, res.Diagnostic, "Cookie")
		assert.Contains(t, res.Diagnostic, "未配置或已失效")
	})
}
