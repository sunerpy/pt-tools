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

// fakeMTorrentSite is a stub implementation of v2.Site used solely to drive
// ProbeMTorrent through every classification branch.
type fakeMTorrentSite struct {
	info v2.UserInfo
	err  error
}

func (f *fakeMTorrentSite) ID() string                                        { return "mteam" }
func (f *fakeMTorrentSite) Name() string                                      { return "M-Team" }
func (f *fakeMTorrentSite) Kind() v2.SiteKind                                 { return v2.SiteMTorrent }
func (f *fakeMTorrentSite) Login(ctx context.Context, _ v2.Credentials) error { return nil }
func (f *fakeMTorrentSite) Search(_ context.Context, _ v2.SearchQuery) ([]v2.TorrentItem, error) {
	return nil, nil
}

func (f *fakeMTorrentSite) GetUserInfo(_ context.Context) (v2.UserInfo, error) {
	return f.info, f.err
}

func (f *fakeMTorrentSite) Download(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (f *fakeMTorrentSite) Close() error { return nil }

func TestProbeMTorrentHappy(t *testing.T) {
	// API returned a valid lastModifiedDate -> LastAccess populated.
	accessTS := time.Date(2026, 5, 15, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
	site := &fakeMTorrentSite{
		info: v2.UserInfo{
			Username:   "tester",
			LastAccess: accessTS.Unix(),
			// M-Team has no last_login concept; LastLogin must remain 0.
			LastLogin: 0,
		},
	}

	clock := NewFakeClock(time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))
	res, err := ProbeMTorrent(context.Background(), site, clock)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, OK, res.Status)
	require.NotNil(t, res.LastAccessAt, "LastAccessAt must be non-nil")
	assert.Equal(t, accessTS.Unix(), res.LastAccessAt.Unix())
	assert.Nil(t, res.LastLoginAt, "M-Team has no last_login; LastLoginAt must be nil")
}

func TestProbeMTorrentClassification(t *testing.T) {
	clock := NewFakeClock(time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))

	cases := []struct {
		name   string
		info   v2.UserInfo
		err    error
		status ProbeStatus
	}{
		{
			name:   "session_expired_api_key_invalid",
			err:    v2.ErrSessionExpired,
			status: SESSION_EXPIRED,
		},
		{
			name:   "session_expired_wrapped",
			err:    fmt.Errorf("driver wrapped: %w", v2.ErrSessionExpired),
			status: SESSION_EXPIRED,
		},
		{
			name:   "circuit_open_rate_limited",
			err:    v2.ErrCircuitOpen,
			status: RATE_LIMITED,
		},
		{
			name:   "rate_limited_sentinel",
			err:    v2.ErrRateLimited,
			status: RATE_LIMITED,
		},
		{
			name:   "deadline_exceeded_network",
			err:    context.DeadlineExceeded,
			status: NETWORK_ERROR,
		},
		{
			name:   "network_error_sentinel",
			err:    fmt.Errorf("connect: %w", v2.ErrNetworkError),
			status: NETWORK_ERROR,
		},
		{
			name:   "challenge_cloudflare",
			err:    errors.New("cloudflare challenge encountered"),
			status: CHALLENGE,
		},
		{
			name:   "parse_error_no_last_access",
			info:   v2.UserInfo{Username: "tester"}, // LastAccess = 0
			err:    nil,
			status: PARSE_ERROR,
		},
		{
			name:   "unknown_error",
			err:    errors.New("something weird happened"),
			status: UNKNOWN,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			site := &fakeMTorrentSite{info: tc.info, err: tc.err}
			res, err := ProbeMTorrent(context.Background(), site, clock)
			require.NoError(t, err, "ProbeMTorrent itself must not return error; classification goes via ProbeResult")
			require.NotNil(t, res)
			assert.Equal(t, tc.status, res.Status, "unexpected status for case %s", tc.name)
			if tc.err != nil {
				assert.NotNil(t, res.RawError, "RawError should preserve original error")
			}
			// M-Team must never fill LastLoginAt.
			assert.Nil(t, res.LastLoginAt, "LastLoginAt must always be nil for M-Team")
		})
	}
}
