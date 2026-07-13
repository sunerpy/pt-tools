package nexusphp

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/cloakdriver"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
)

func mustReadFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", name)
	b, err := os.ReadFile(path)
	require.NoError(t, err, "read fixture %s", path)
	return string(b)
}

func TestNexusPHPCloakDriverParse(t *testing.T) {
	t.Run("happy_zh_naive_cst", func(t *testing.T) {
		html := mustReadFixture(t, "user_info.html")
		ll, la, err := parseNexusPHPUserPage(html)
		require.NoError(t, err)

		// 2026-05-15 12:34:56 in CST (UTC+8) == 2026-05-15 04:34:56 UTC
		wantLogin := time.Date(2026, 5, 15, 4, 34, 56, 0, time.UTC)
		// 2026-05-18 09:00:00 in CST            == 2026-05-18 01:00:00 UTC
		wantAccess := time.Date(2026, 5, 18, 1, 0, 0, 0, time.UTC)

		assert.True(t, ll.UTC().Equal(wantLogin),
			"last_login UTC mismatch: got %s want %s", ll.UTC(), wantLogin)
		assert.True(t, la.UTC().Equal(wantAccess),
			"last_access UTC mismatch: got %s want %s", la.UTC(), wantAccess)
	})

	t.Run("drift_zone_aware", func(t *testing.T) {
		html := mustReadFixture(t, "user_info_drift.html")
		ll, la, err := parseNexusPHPUserPage(html)
		require.NoError(t, err)

		// 2026-05-15T12:34:56+08:00 == 2026-05-15 04:34:56 UTC
		wantLogin := time.Date(2026, 5, 15, 4, 34, 56, 0, time.UTC)
		// 2026-05-18 09:00:00 (+09:00) == 2026-05-18 00:00:00 UTC
		wantAccess := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)

		assert.True(t, ll.UTC().Equal(wantLogin),
			"last_login (zone-aware) UTC mismatch: got %s want %s", ll.UTC(), wantLogin)
		assert.True(t, la.UTC().Equal(wantAccess),
			"last_access (drift zone) UTC mismatch: got %s want %s", la.UTC(), wantAccess)
	})

	t.Run("empty_returns_parse_error", func(t *testing.T) {
		html := mustReadFixture(t, "user_info_empty.html")
		ll, la, err := parseNexusPHPUserPage(html)
		require.Error(t, err, "empty fixture must return parse error")
		assert.True(t, ll.IsZero(), "ll must be zero on parse error")
		assert.True(t, la.IsZero(), "la must be zero on parse error")
	})
}

// fakeManager is a minimal cloakdriver.ManagerClient stub for unit tests.
// It records calls and returns fixed values / errors per field.
type fakeManager struct {
	launchResult *cloakdriver.ProfileLaunchResult
	launchErr    error
	stopCalls    int
}

func (f *fakeManager) LaunchProfile(_ context.Context, _ string) (*cloakdriver.ProfileLaunchResult, error) {
	if f.launchErr != nil {
		return nil, f.launchErr
	}
	if f.launchResult != nil {
		return f.launchResult, nil
	}
	return &cloakdriver.ProfileLaunchResult{ProfileID: "p", CdpURL: "ws://127.0.0.1:1/devtools/browser/x", StartedAt: time.Now()}, nil
}

func (f *fakeManager) GetProfileStatus(_ context.Context, _ string) (*cloakdriver.ProfileStatus, error) {
	return &cloakdriver.ProfileStatus{ProfileID: "p", Running: true}, nil
}

func (f *fakeManager) StopProfile(_ context.Context, _ string) error {
	f.stopCalls++
	return nil
}

func (f *fakeManager) DeleteProfile(_ context.Context, _ string) error { return nil }
func (f *fakeManager) ManagerStatus(_ context.Context) error           { return nil }
func (f *fakeManager) ManagerStatusFull(_ context.Context) (*cloakdriver.ManagerStatusInfo, error) {
	return &cloakdriver.ManagerStatusInfo{Status: "ok", Version: "0.0.4"}, nil
}

func TestNexusPHPDriverProbeArgValidation(t *testing.T) {
	cases := []struct {
		name      string
		manager   cloakdriver.ManagerClient
		indexURL  string
		profileID string
		wantErr   string
	}{
		{name: "nil_manager", manager: nil, indexURL: "https://x", profileID: "p", wantErr: "nil manager"},
		{name: "empty_url", manager: &fakeManager{}, indexURL: "", profileID: "p", wantErr: "empty indexURL"},
		{name: "empty_profile", manager: &fakeManager{}, indexURL: "https://x", profileID: "", wantErr: "empty profileID"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDriver(tc.manager)
			_, err := d.Probe(context.Background(), tc.indexURL, nil, tc.profileID)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestNexusPHPDriverProbeManagerErrorClassification(t *testing.T) {
	cases := []struct {
		name       string
		launchErr  error
		wantStatus sitelogin.ProbeStatus
	}{
		{"auth_failed", cloakdriver.ErrManagerAuthFailed, sitelogin.KEY_ERROR},
		{"not_found", cloakdriver.ErrManagerNotFound, sitelogin.UNKNOWN},
		{"server_error", cloakdriver.ErrManagerServerError, sitelogin.NETWORK_ERROR},
		{"dns_failed", cloakdriver.ErrManagerDNSFailed, sitelogin.NETWORK_ERROR},
		{"conn_refused", cloakdriver.ErrManagerConnRefused, sitelogin.NETWORK_ERROR},
		{"timeout", cloakdriver.ErrManagerTimeout, sitelogin.NETWORK_ERROR},
		{"protocol", cloakdriver.ErrManagerProtocolError, sitelogin.UNKNOWN},
		{"unrecognised", errors.New("totally unexpected"), sitelogin.UNKNOWN},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fm := &fakeManager{launchErr: tc.launchErr}
			d := NewDriver(fm)
			res, err := d.Probe(context.Background(), "https://example.test/", nil, "p1")
			require.NoError(t, err, "classification errors must be encoded into result, not returned as err")
			require.NotNil(t, res)
			assert.Equal(t, tc.wantStatus, res.Status)
			assert.Equal(t, sitelogin.ProbeSourceCloak, res.Source)
			assert.ErrorIs(t, res.RawError, tc.launchErr)
			// Manager.LaunchProfile failed before we got to defer Stop, so
			// stopCalls remains 0 — the deferred Stop is registered AFTER
			// a successful launch (so we don't try to stop a profile that
			// was never started).
			assert.Equal(t, 0, fm.stopCalls)
		})
	}
}

func TestNexusPHPDriverProbeCDPConnectFailureCleansUpProfile(t *testing.T) {
	// Use a deliberately unreachable cdp_url so NewCDPSession fails fast.
	// This proves the deferred Stop runs even when CDP connect fails.
	fm := &fakeManager{
		launchResult: &cloakdriver.ProfileLaunchResult{
			ProfileID: "p",
			CdpURL:    "ws://127.0.0.1:1/unreachable",
			StartedAt: time.Now(),
		},
	}
	d := NewDriver(fm)
	res, err := d.Probe(context.Background(), "https://example.test/", []*http.Cookie{}, "p1")
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, sitelogin.NETWORK_ERROR, res.Status)
	assert.Equal(t, sitelogin.ProbeSourceCloak, res.Source)
	assert.GreaterOrEqual(t, fm.stopCalls, 1, "profile MUST be stopped even when CDP connect fails (resource leak guard)")
}

func TestClassifyChromedpError(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus sitelogin.ProbeStatus
	}{
		{"deadline_is_challenge", context.DeadlineExceeded, sitelogin.CHALLENGE},
		{"cancel_unknown", context.Canceled, sitelogin.UNKNOWN},
		{"cloudflare_msg_challenge", errors.New("cloudflare hold"), sitelogin.CHALLENGE},
		{"challenge_msg_challenge", errors.New("challenge presented"), sitelogin.CHALLENGE},
		{"parse_error_msg", errors.New("nexusphp parse: no last_login/last_access row found"), sitelogin.PARSE_ERROR},
		{"default_network", errors.New("websocket: bad handshake"), sitelogin.NETWORK_ERROR},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := classifyChromedpError(tc.err)
			require.NotNil(t, r)
			assert.Equal(t, tc.wantStatus, r.Status)
			assert.Equal(t, sitelogin.ProbeSourceCloak, r.Source)
		})
	}
}

func TestNexusProbeManagerErrorPaths(t *testing.T) {
	cases := []struct {
		name       string
		launchErr  error
		wantStatus sitelogin.ProbeStatus
	}{
		{"auth", cloakdriver.ErrManagerAuthFailed, sitelogin.KEY_ERROR},
		{"server", cloakdriver.ErrManagerServerError, sitelogin.NETWORK_ERROR},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fm := &fakeManager{launchErr: tc.launchErr}
			d := NewDriver(fm)
			res, err := d.Probe(context.Background(), "https://x", []*http.Cookie{{Name: "c", Value: "v"}}, "p1")
			require.NoError(t, err)
			assert.Equal(t, tc.wantStatus, res.Status)
			assert.Equal(t, 0, fm.stopCalls)
		})
	}
}

func TestNexusProbeCDPFailureCleansUp(t *testing.T) {
	fm := &fakeManager{launchResult: &cloakdriver.ProfileLaunchResult{ProfileID: "p", CdpURL: "ws://127.0.0.1:1/x", StartedAt: time.Now()}}
	d := NewDriver(fm)
	res, err := d.Probe(context.Background(), "https://x", []*http.Cookie{{Name: "c", Value: "v"}}, "p1")
	require.NoError(t, err)
	assert.Equal(t, sitelogin.NETWORK_ERROR, res.Status)
	assert.GreaterOrEqual(t, fm.stopCalls, 1)
}

func TestNexusMatchHeader(t *testing.T) {
	assert.True(t, matchHeader("上次登录", lastLoginHeaders))
	assert.True(t, matchHeader("Last Access", lastAccessHeaders))
	assert.False(t, matchHeader("上传量", lastLoginHeaders))
}

func TestNexusParseTimestampLayouts(t *testing.T) {
	cases := []struct {
		in string
		ok bool
	}{
		{"2026-05-18 09:30:00 (+08:00)", true},
		{"2026-05-18T09:30:00+08:00", true},
		{"2026-05-18 09:30:00", true},
		{"2026/05/18 09:30:00", true},
		{"", false},
		{"nope", false},
	}
	for _, c := range cases {
		_, ok := parseTimestamp(c.in)
		assert.Equal(t, c.ok, ok, "input=%q", c.in)
	}
}

func TestNexusParseTimestampCSTFallback(t *testing.T) {
	got, ok := parseTimestamp("2026-05-18 09:30:00")
	require.True(t, ok)
	assert.True(t, got.UTC().Equal(time.Date(2026, 5, 18, 1, 30, 0, 0, time.UTC)),
		"bare NexusPHP timestamp must be interpreted as CST (UTC+8)")
}

func TestNexusParseUserPageEnglishHeaders(t *testing.T) {
	html := `<html><body><table>
	<tr><td class="rowhead">Last Login</td><td class="rowfollow">2026-05-18 01:30:00</td></tr>
	<tr><td class="rowhead">Last Access</td><td class="rowfollow">2026-05-18 02:30:00</td></tr>
	</table></body></html>`
	ll, la, err := parseNexusPHPUserPage(html)
	require.NoError(t, err)
	assert.False(t, ll.IsZero())
	assert.False(t, la.IsZero())
}

func TestNexusParseUserPageSkipsEmptyCells(t *testing.T) {
	html := `<html><body><table>
	<tr><td class="rowhead"></td><td class="rowfollow"></td></tr>
	<tr><td class="rowhead">上次登录</td><td class="rowfollow">2026-05-18 01:30:00</td></tr>
	</table></body></html>`
	ll, _, err := parseNexusPHPUserPage(html)
	require.NoError(t, err)
	assert.False(t, ll.IsZero())
}
