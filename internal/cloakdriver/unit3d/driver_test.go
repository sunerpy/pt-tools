package unit3d

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

func TestUnit3DDriverParse(t *testing.T) {
	html := mustReadFixture(t, "user_info.html")
	ll, la, err := parseUnit3DUserPage(html)
	require.NoError(t, err)

	// 2026-05-15T12:34:56Z UTC
	wantLogin := time.Date(2026, 5, 15, 12, 34, 56, 0, time.UTC)
	// 2026-05-18T09:00:00Z UTC
	wantAccess := time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC)

	assert.True(t, ll.UTC().Equal(wantLogin),
		"last_login UTC mismatch: got %s want %s", ll.UTC(), wantLogin)
	assert.True(t, la.UTC().Equal(wantAccess),
		"last_action UTC mismatch: got %s want %s", la.UTC(), wantAccess)
}

func TestUnit3DDriverParseSessionExpired(t *testing.T) {
	html := mustReadFixture(t, "login_redirect.html")
	expired := isUnit3DLoginPage(html)
	assert.True(t, expired, "login_redirect.html must be detected as session-expired login page")
}

func TestUnit3DDriverParseEmpty(t *testing.T) {
	html := mustReadFixture(t, "user_info_empty.html")
	ll, la, err := parseUnit3DUserPage(html)
	require.Error(t, err, "empty fixture must return parse error")
	assert.True(t, ll.IsZero(), "ll must be zero on parse error")
	assert.True(t, la.IsZero(), "la must be zero on parse error")
}

// fakeManager is a minimal cloakdriver.ManagerClient stub for unit tests.
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

func TestUnit3DDriverProbeArgValidation(t *testing.T) {
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

func TestUnit3DDriverProbeManagerErrorClassification(t *testing.T) {
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
			assert.Equal(t, 0, fm.stopCalls)
		})
	}
}

func TestUnit3DDriverProbeCDPConnectFailureCleansUpProfile(t *testing.T) {
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

func TestUnit3DClassifyChromedpError(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus sitelogin.ProbeStatus
	}{
		{"deadline_is_challenge", context.DeadlineExceeded, sitelogin.CHALLENGE},
		{"cancel_unknown", context.Canceled, sitelogin.UNKNOWN},
		{"cloudflare_msg_challenge", errors.New("cloudflare hold"), sitelogin.CHALLENGE},
		{"challenge_msg_challenge", errors.New("challenge presented"), sitelogin.CHALLENGE},
		{"parse_error_msg", errors.New("unit3d parse: no last_login/last_action found"), sitelogin.PARSE_ERROR},
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

func TestUnit3DClassifyManagerErrorDefault(t *testing.T) {
	r := classifyManagerError(assert.AnError)
	assert.Equal(t, "cloak: manager error", r.Diagnostic)
}

func TestUnit3DIsLoginPageByTitle(t *testing.T) {
	assert.True(t, isUnit3DLoginPage(`<html><head><title>Login</title></head><body></body></html>`))
}

func TestUnit3DIsLoginPageByFormAction(t *testing.T) {
	cases := []struct {
		name string
		html string
		want bool
	}{
		{"absolute_login", `<form action="https://x.test/login/"></form>`, true},
		{"root_login", `<form action="/login"></form>`, true},
		{"other_form", `<form action="/search"></form>`, false},
		{"no_form_no_title", `<div>user page</div>`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isUnit3DLoginPage(tc.html))
		})
	}
}

func TestUnit3DParseViaLiFallback(t *testing.T) {
	html := `<html><body><div class="user-info">
	<li><span class="user-info__label">Last Login</span><time datetime="2026-05-15T12:34:56Z"></time></li>
	<li><span class="user-info__label">Last Action</span><time datetime="2026-05-18T09:00:00Z"></time></li>
	</div></body></html>`
	ll, la, err := parseUnit3DUserPage(html)
	require.NoError(t, err)
	assert.True(t, ll.Equal(time.Date(2026, 5, 15, 12, 34, 56, 0, time.UTC)))
	assert.True(t, la.Equal(time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC)))
}

func TestUnit3DParseLiFallbackLastSeen(t *testing.T) {
	html := `<html><body><div class="user-info">
	<li><span class="user-info__label">Last Seen</span><time datetime="2026-01-02T03:04:05Z"></time></li>
	</div></body></html>`
	_, la, err := parseUnit3DUserPage(html)
	require.NoError(t, err)
	assert.True(t, la.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)))
}

func TestUnit3DReadTimeNodeTextFallback(t *testing.T) {
	html := `<html><body>
	<span class="user-info__last-login">2026-05-15 12:34:56</span>
	</body></html>`
	ll, _, err := parseUnit3DUserPage(html)
	require.NoError(t, err)
	assert.True(t, ll.Equal(time.Date(2026, 5, 15, 12, 34, 56, 0, time.UTC)))
}

func TestUnit3DParseTimestampLayouts(t *testing.T) {
	cases := []struct {
		in string
		ok bool
	}{
		{"2026-05-15T12:34:56Z", true},
		{"2026-05-15T12:34:56", true},
		{"2026-05-15 12:34:56", true},
		{"2026-05-15 12:34:56 (+08:00)", true},
		{"", false},
		{"garbage", false},
	}
	for _, c := range cases {
		_, ok := parseTimestamp(c.in)
		assert.Equal(t, c.ok, ok, "input=%q", c.in)
	}
}

func TestUnit3DReadTimeNodeEmptySelection(t *testing.T) {
	html := `<html><body><div class="user-info__last-login"></div></body></html>`
	_, _, err := parseUnit3DUserPage(html)
	require.Error(t, err, "empty time node yields no timestamps → parse error")
}

func TestUnit3DParseLiEmptyLabelAndBadTimeSkipped(t *testing.T) {
	html := `<html><body><div class="user-info">
	<li><span class="user-info__label"></span><time datetime="2026-05-15T12:34:56Z"></time></li>
	<li><span class="user-info__label">Last Login</span><time datetime="bogus"></time></li>
	<li><span class="user-info__label">Last Login</span><time datetime="2026-05-15T12:34:56Z"></time></li>
	</div></body></html>`
	ll, _, err := parseUnit3DUserPage(html)
	require.NoError(t, err)
	assert.True(t, ll.Equal(time.Date(2026, 5, 15, 12, 34, 56, 0, time.UTC)),
		"empty-label and unparsable-time li rows must be skipped, third row wins")
}
