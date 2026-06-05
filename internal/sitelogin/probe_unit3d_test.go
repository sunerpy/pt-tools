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

// mockUnit3DSite implements v2.Site for testing ProbeUnit3D. Only GetUserInfo is
// exercised; other methods return zero values / nil to satisfy the interface.
type mockUnit3DSite struct {
	info v2.UserInfo
	err  error
}

func (m *mockUnit3DSite) ID() string                                      { return "mock-unit3d" }
func (m *mockUnit3DSite) Name() string                                    { return "MockUnit3D" }
func (m *mockUnit3DSite) Kind() v2.SiteKind                               { return v2.SiteUnit3D }
func (m *mockUnit3DSite) Login(_ context.Context, _ v2.Credentials) error { return nil }
func (m *mockUnit3DSite) Search(_ context.Context, _ v2.SearchQuery) ([]v2.TorrentItem, error) {
	return nil, nil
}

func (m *mockUnit3DSite) GetUserInfo(_ context.Context) (v2.UserInfo, error) {
	return m.info, m.err
}
func (m *mockUnit3DSite) Download(_ context.Context, _ string) ([]byte, error) { return nil, nil }
func (m *mockUnit3DSite) Close() error                                         { return nil }

func TestProbeUnit3DHappy(t *testing.T) {
	// Unit3D last_login=2026-05-15T12:00:00Z, last_action=2026-05-16T08:00:00Z
	loginTs := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC).Unix()
	accessTs := time.Date(2026, 5, 16, 8, 0, 0, 0, time.UTC).Unix()
	site := &mockUnit3DSite{
		info: v2.UserInfo{
			Site:       "mock-unit3d",
			Username:   "tester",
			LastLogin:  loginTs,
			LastAccess: accessTs,
		},
	}
	clk := NewFakeClock(time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC))
	res, err := ProbeUnit3D(context.Background(), site, clk)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, OK, res.Status)
	require.NotNil(t, res.LastLoginAt)
	assert.Equal(t, loginTs, res.LastLoginAt.Unix())
	require.NotNil(t, res.LastAccessAt)
	assert.Equal(t, accessTs, res.LastAccessAt.Unix())
}

func TestProbeUnit3DHappyNoLastLogin(t *testing.T) {
	// Some Unit3D variants only expose last_action; LastLogin remains 0.
	accessTs := time.Date(2026, 5, 16, 8, 0, 0, 0, time.UTC).Unix()
	site := &mockUnit3DSite{
		info: v2.UserInfo{
			Username:   "tester",
			LastLogin:  0,
			LastAccess: accessTs,
		},
	}
	res, err := ProbeUnit3D(context.Background(), site, NewRealClock())
	require.NoError(t, err)
	assert.Equal(t, OK, res.Status)
	assert.Nil(t, res.LastLoginAt, "LastLogin=0 must yield nil pointer")
	require.NotNil(t, res.LastAccessAt)
	assert.Equal(t, accessTs, res.LastAccessAt.Unix())
}

func TestProbeUnit3DParseError(t *testing.T) {
	// Driver returned no error but neither timestamp—parse failed silently upstream.
	site := &mockUnit3DSite{
		info: v2.UserInfo{Username: "tester"},
	}
	res, err := ProbeUnit3D(context.Background(), site, NewRealClock())
	require.NoError(t, err)
	assert.Equal(t, PARSE_ERROR, res.Status)
}

func TestProbeUnit3DSessionExpired(t *testing.T) {
	site := &mockUnit3DSite{err: fmt.Errorf("auth check: %w", v2.ErrSessionExpired)}
	res, err := ProbeUnit3D(context.Background(), site, NewRealClock())
	require.NoError(t, err)
	assert.Equal(t, SESSION_EXPIRED, res.Status)
	assert.ErrorIs(t, res.RawError, v2.ErrSessionExpired)
}

func TestProbeUnit3DRateLimited(t *testing.T) {
	site := &mockUnit3DSite{err: fmt.Errorf("rate limited: %w", v2.ErrCircuitOpen)}
	res, err := ProbeUnit3D(context.Background(), site, NewRealClock())
	require.NoError(t, err)
	assert.Equal(t, RATE_LIMITED, res.Status)
}

func TestProbeUnit3DRateLimitedSentinel(t *testing.T) {
	site := &mockUnit3DSite{err: fmt.Errorf("upstream throttled: %w", v2.ErrRateLimited)}
	res, err := ProbeUnit3D(context.Background(), site, NewRealClock())
	require.NoError(t, err)
	assert.Equal(t, RATE_LIMITED, res.Status)
}

func TestProbeUnit3DNetworkError(t *testing.T) {
	site := &mockUnit3DSite{err: context.DeadlineExceeded}
	res, err := ProbeUnit3D(context.Background(), site, NewRealClock())
	require.NoError(t, err)
	assert.Equal(t, NETWORK_ERROR, res.Status)
}

func TestProbeUnit3DNetworkErrorWrapped(t *testing.T) {
	site := &mockUnit3DSite{err: fmt.Errorf("net dial: %w", v2.ErrNetworkError)}
	res, err := ProbeUnit3D(context.Background(), site, NewRealClock())
	require.NoError(t, err)
	assert.Equal(t, NETWORK_ERROR, res.Status)
}

func TestProbeUnit3DChallenge(t *testing.T) {
	site := &mockUnit3DSite{err: errors.New("got cloudflare challenge page (cf-chl)")}
	res, err := ProbeUnit3D(context.Background(), site, NewRealClock())
	require.NoError(t, err)
	assert.Equal(t, CHALLENGE, res.Status)
}

func TestProbeUnit3DKeyError(t *testing.T) {
	site := &mockUnit3DSite{err: fmt.Errorf("driver init: %w", v2.ErrInvalidCredentials)}
	res, err := ProbeUnit3D(context.Background(), site, NewRealClock())
	require.NoError(t, err)
	assert.Equal(t, KEY_ERROR, res.Status)
}

func TestProbeUnit3DUnknown(t *testing.T) {
	site := &mockUnit3DSite{err: errors.New("totally unexpected boom")}
	res, err := ProbeUnit3D(context.Background(), site, NewRealClock())
	require.NoError(t, err)
	assert.Equal(t, UNKNOWN, res.Status)
	require.NotNil(t, res.RawError)
}

func TestProbeUnit3DNilSite(t *testing.T) {
	res, err := ProbeUnit3D(context.Background(), nil, NewRealClock())
	require.Error(t, err)
	assert.Nil(t, res)
}

func TestProbeUnit3DNilClock(t *testing.T) {
	site := &mockUnit3DSite{
		info: v2.UserInfo{LastAccess: time.Now().Unix()},
	}
	// Nil clock should not panic; impl must use a real clock fallback.
	res, err := ProbeUnit3D(context.Background(), site, nil)
	require.NoError(t, err)
	assert.Equal(t, OK, res.Status)
}
