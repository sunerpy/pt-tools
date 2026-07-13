package gazelle

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/cloakdriver"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
)

// launchErrManager returns a preset error from LaunchProfile so the Probe
// manager-error classification path can be exercised without a real Chrome.
type launchErrManager struct {
	err       error
	stopCalls int
}

func (m *launchErrManager) LaunchProfile(_ context.Context, _ string) (*cloakdriver.ProfileLaunchResult, error) {
	return nil, m.err
}

func (m *launchErrManager) GetProfileStatus(_ context.Context, _ string) (*cloakdriver.ProfileStatus, error) {
	return &cloakdriver.ProfileStatus{ProfileID: "p", Running: false}, nil
}

func (m *launchErrManager) StopProfile(_ context.Context, _ string) error {
	m.stopCalls++
	return nil
}
func (m *launchErrManager) DeleteProfile(_ context.Context, _ string) error { return nil }
func (m *launchErrManager) ManagerStatus(_ context.Context) error           { return nil }
func (m *launchErrManager) ManagerStatusFull(_ context.Context) (*cloakdriver.ManagerStatusInfo, error) {
	return &cloakdriver.ManagerStatusInfo{Status: "ok"}, nil
}

func TestGazelleArgValidation(t *testing.T) {
	cases := []struct {
		name      string
		manager   cloakdriver.ManagerClient
		indexURL  string
		profileID string
		wantErr   string
	}{
		{"nil_manager", nil, "https://x/user.php?id=1", "p", "nil manager"},
		{"empty_url", &launchErrManager{}, "", "p", "empty indexURL"},
		{"empty_profile", &launchErrManager{}, "https://x", "", "empty profileID"},
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

func TestGazelleProbeManagerErrorPaths(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus sitelogin.ProbeStatus
	}{
		{"auth", cloakdriver.ErrManagerAuthFailed, sitelogin.KEY_ERROR},
		{"notfound", cloakdriver.ErrManagerNotFound, sitelogin.UNKNOWN},
		{"server", cloakdriver.ErrManagerServerError, sitelogin.NETWORK_ERROR},
		{"dns", cloakdriver.ErrManagerDNSFailed, sitelogin.NETWORK_ERROR},
		{"connrefused", cloakdriver.ErrManagerConnRefused, sitelogin.NETWORK_ERROR},
		{"timeout", cloakdriver.ErrManagerTimeout, sitelogin.NETWORK_ERROR},
		{"protocol", cloakdriver.ErrManagerProtocolError, sitelogin.UNKNOWN},
		{"other", errors.New("boom"), sitelogin.UNKNOWN},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &launchErrManager{err: tc.err}
			d := NewDriver(m)
			res, err := d.Probe(
				context.Background(),
				"https://x/user.php?id=1",
				[]*http.Cookie{{Name: "session", Value: "v"}},
				"p1",
			)
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.Equal(t, tc.wantStatus, res.Status)
			assert.Equal(t, sitelogin.ProbeSourceCloak, res.Source)
			assert.ErrorIs(t, res.RawError, tc.err)
		})
	}
}

type okLaunchManager struct{ stopCalls int }

func (m *okLaunchManager) LaunchProfile(_ context.Context, _ string) (*cloakdriver.ProfileLaunchResult, error) {
	return &cloakdriver.ProfileLaunchResult{ProfileID: "p", CdpURL: "ws://127.0.0.1:1/devtools", StartedAt: time.Now()}, nil
}

func (m *okLaunchManager) GetProfileStatus(_ context.Context, _ string) (*cloakdriver.ProfileStatus, error) {
	return &cloakdriver.ProfileStatus{ProfileID: "p", Running: true}, nil
}

func (m *okLaunchManager) StopProfile(_ context.Context, _ string) error   { m.stopCalls++; return nil }
func (m *okLaunchManager) DeleteProfile(_ context.Context, _ string) error { return nil }
func (m *okLaunchManager) ManagerStatus(_ context.Context) error           { return nil }
func (m *okLaunchManager) ManagerStatusFull(_ context.Context) (*cloakdriver.ManagerStatusInfo, error) {
	return &cloakdriver.ManagerStatusInfo{Status: "ok"}, nil
}

func TestGazelleProbeCDPOpenFailure(t *testing.T) {
	m := &okLaunchManager{}
	d := NewDriver(m)
	res, err := d.Probe(
		context.Background(),
		"https://x/user.php?id=1",
		[]*http.Cookie{{Name: "session", Value: "v"}},
		"p1",
	)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, sitelogin.NETWORK_ERROR, res.Status)
	assert.Equal(t, sitelogin.ProbeSourceCloak, res.Source)
	assert.NotNil(t, res.RawError)
	assert.Equal(t, 1, m.stopCalls, "StopProfile must run via defer after launch")
}

func TestGazelleClassifyChromedpError(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus sitelogin.ProbeStatus
	}{
		{"deadline", context.DeadlineExceeded, sitelogin.CHALLENGE},
		{"canceled", context.Canceled, sitelogin.UNKNOWN},
		{"parse", errors.New("cloak: no lastaccess row found"), sitelogin.PARSE_ERROR},
		{"challenge", errors.New("cloudflare challenge detected"), sitelogin.CHALLENGE},
		{"network", errors.New("some chromedp failure"), sitelogin.NETWORK_ERROR},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := classifyChromedpError(tc.err)
			assert.Equal(t, tc.wantStatus, r.Status)
			assert.Equal(t, sitelogin.ProbeSourceCloak, r.Source)
			assert.Equal(t, tc.err, r.RawError)
		})
	}
}

func TestGazelleIsLastAccessLabel(t *testing.T) {
	assert.True(t, isLastAccessLabel("last seen: 2 hours ago"))
	assert.True(t, isLastAccessLabel("last access foo"))
	assert.True(t, isLastAccessLabel("last visit bar"))
	assert.False(t, isLastAccessLabel("uploaded: 1 TB"))
	assert.False(t, isLastAccessLabel(""))
}

func TestGazelleParseTimestamp(t *testing.T) {
	t.Run("space_layout_utc", func(t *testing.T) {
		got, ok := parseGazelleTimestamp("2026-05-18 09:00:00")
		require.True(t, ok)
		assert.True(t, got.Equal(time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC)))
	})
	t.Run("iso_z", func(t *testing.T) {
		got, ok := parseGazelleTimestamp("2026-05-18T09:00:00Z")
		require.True(t, ok)
		assert.True(t, got.Equal(time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC)))
	})
	t.Run("empty", func(t *testing.T) {
		_, ok := parseGazelleTimestamp("   ")
		assert.False(t, ok)
	})
	t.Run("garbage", func(t *testing.T) {
		_, ok := parseGazelleTimestamp("not-a-date")
		assert.False(t, ok)
	})
}

func TestGazelleParseUserPageTitleFallbackToText(t *testing.T) {
	// span.time has no title attr — parser must fall back to the element text.
	html := `<html><body><ul class="stats">
	<li>Last seen: <span class="time">2026-05-18 09:00:00</span></li>
	</ul></body></html>`
	la, err := parseGazelleUserPage(html)
	require.NoError(t, err)
	assert.True(t, la.Equal(time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC)))
}

func TestGazelleParseUserPageUnparsableTitleSkipped(t *testing.T) {
	html := `<html><body><ul class="stats">
	<li>Last seen: <span class="time" title="bogus"></span></li>
	</ul></body></html>`
	_, err := parseGazelleUserPage(html)
	require.Error(t, err)
}

func TestGazelleParseUserPageEmptyTitleAndTextSkipped(t *testing.T) {
	html := `<html><body><ul class="stats">
	<li>Last seen: <span class="time" title=""></span></li>
	</ul></body></html>`
	_, err := parseGazelleUserPage(html)
	require.Error(t, err, "empty title and empty text → no match → parse error")
}
