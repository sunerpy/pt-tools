package mtorrent

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
	b, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return string(b)
}

type recordingManager struct {
	launchCalls int
	stopCalls   int
}

func (r *recordingManager) LaunchProfile(_ context.Context, _ string) (*cloakdriver.ProfileLaunchResult, error) {
	r.launchCalls++
	return &cloakdriver.ProfileLaunchResult{ProfileID: "p", CdpURL: "ws://127.0.0.1:1/x", StartedAt: time.Now()}, nil
}

func (r *recordingManager) GetProfileStatus(_ context.Context, _ string) (*cloakdriver.ProfileStatus, error) {
	return &cloakdriver.ProfileStatus{ProfileID: "p", Running: true}, nil
}

func (r *recordingManager) StopProfile(_ context.Context, _ string) error {
	r.stopCalls++
	return nil
}
func (r *recordingManager) DeleteProfile(_ context.Context, _ string) error { return nil }
func (r *recordingManager) ManagerStatus(_ context.Context) error           { return nil }
func (r *recordingManager) ManagerStatusFull(_ context.Context) (*cloakdriver.ManagerStatusInfo, error) {
	return &cloakdriver.ManagerStatusInfo{Status: "ok", Version: "0.0.4"}, nil
}

func TestMTorrentDriverWithCookieParse(t *testing.T) {
	html := mustReadFixture(t, "profile_page.html")
	la, err := parseMTorrentProfilePage(html)
	require.NoError(t, err)

	want := time.Date(2026, 5, 18, 1, 30, 0, 0, time.UTC)
	assert.True(t, la.UTC().Equal(want),
		"lastModifiedDate UTC mismatch: got %s want %s", la.UTC(), want)
}

func TestMTorrentDriverParseEmpty(t *testing.T) {
	html := mustReadFixture(t, "profile_page_empty.html")
	la, err := parseMTorrentProfilePage(html)
	require.Error(t, err)
	assert.True(t, la.IsZero())
}

func TestMTorrentDriverNoCookieReturnsNotApplicable(t *testing.T) {
	rm := &recordingManager{}
	d := NewDriver(rm)

	res, err := d.Probe(context.Background(), "https://kp.m-team.cc/profile", nil, "p1")
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, sitelogin.NOT_APPLICABLE, res.Status)
	assert.Equal(t, sitelogin.ProbeSourceCloak, res.Source)
	assert.Nil(t, res.LastLoginAt, "M-Team has no separate last_login")
	assert.Nil(t, res.LastAccessAt)
	assert.Equal(t, 0, rm.launchCalls, "Manager MUST NOT be invoked when cookie is empty")
	assert.Equal(t, 0, rm.stopCalls)
}

func TestMTorrentDriverNoCookieEmptySliceReturnsNotApplicable(t *testing.T) {
	rm := &recordingManager{}
	d := NewDriver(rm)

	res, err := d.Probe(context.Background(), "https://kp.m-team.cc/profile", []*http.Cookie{}, "p1")
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, sitelogin.NOT_APPLICABLE, res.Status)
	assert.Equal(t, 0, rm.launchCalls)
}

func TestMTorrentDriverArgValidation(t *testing.T) {
	cases := []struct {
		name      string
		manager   cloakdriver.ManagerClient
		indexURL  string
		profileID string
		cookies   []*http.Cookie
		wantErr   string
	}{
		{
			name:      "nil_manager",
			manager:   nil,
			indexURL:  "https://x",
			profileID: "p",
			cookies:   []*http.Cookie{{Name: "tp", Value: "v"}},
			wantErr:   "nil manager",
		},
		{
			name:      "empty_url",
			manager:   &recordingManager{},
			indexURL:  "",
			profileID: "p",
			cookies:   []*http.Cookie{{Name: "tp", Value: "v"}},
			wantErr:   "empty indexURL",
		},
		{
			name:      "empty_profile",
			manager:   &recordingManager{},
			indexURL:  "https://x",
			profileID: "",
			cookies:   []*http.Cookie{{Name: "tp", Value: "v"}},
			wantErr:   "empty profileID",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDriver(tc.manager)
			_, err := d.Probe(context.Background(), tc.indexURL, tc.cookies, tc.profileID)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

type errLaunchManager struct {
	launchErr error
	stopCalls int
}

func (m *errLaunchManager) LaunchProfile(_ context.Context, _ string) (*cloakdriver.ProfileLaunchResult, error) {
	if m.launchErr != nil {
		return nil, m.launchErr
	}
	return &cloakdriver.ProfileLaunchResult{ProfileID: "p", CdpURL: "ws://127.0.0.1:1/x", StartedAt: time.Now()}, nil
}

func (m *errLaunchManager) GetProfileStatus(_ context.Context, _ string) (*cloakdriver.ProfileStatus, error) {
	return &cloakdriver.ProfileStatus{ProfileID: "p", Running: true}, nil
}

func (m *errLaunchManager) StopProfile(_ context.Context, _ string) error   { m.stopCalls++; return nil }
func (m *errLaunchManager) DeleteProfile(_ context.Context, _ string) error { return nil }
func (m *errLaunchManager) ManagerStatus(_ context.Context) error           { return nil }
func (m *errLaunchManager) ManagerStatusFull(_ context.Context) (*cloakdriver.ManagerStatusInfo, error) {
	return &cloakdriver.ManagerStatusInfo{Status: "ok"}, nil
}

func TestMTorrentProbeLaunchErrorWithCookie(t *testing.T) {
	m := &errLaunchManager{launchErr: cloakdriver.ErrManagerAuthFailed}
	d := NewDriver(m)
	res, err := d.Probe(
		context.Background(),
		"https://kp.m-team.cc/profile",
		[]*http.Cookie{{Name: "Q-detail-1", Value: "a"}},
		"p1",
	)
	require.NoError(t, err)
	assert.Equal(t, sitelogin.KEY_ERROR, res.Status)
	assert.Equal(t, 0, m.stopCalls, "stop not registered before successful launch")
}

func TestMTorrentProbeCDPFailureCleansUp(t *testing.T) {
	m := &errLaunchManager{}
	d := NewDriver(m)
	res, err := d.Probe(
		context.Background(),
		"https://kp.m-team.cc/profile",
		[]*http.Cookie{{Name: "Q-detail-1", Value: "a"}},
		"p1",
	)
	require.NoError(t, err)
	assert.Equal(t, sitelogin.NETWORK_ERROR, res.Status)
	assert.GreaterOrEqual(t, m.stopCalls, 1, "profile stopped even when CDP connect fails")
}

func TestMTorrentClassifyManagerError(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus sitelogin.ProbeStatus
	}{
		{"auth", cloakdriver.ErrManagerAuthFailed, sitelogin.KEY_ERROR},
		{"notfound", cloakdriver.ErrManagerNotFound, sitelogin.UNKNOWN},
		{"server", cloakdriver.ErrManagerServerError, sitelogin.NETWORK_ERROR},
		{"dns", cloakdriver.ErrManagerDNSFailed, sitelogin.NETWORK_ERROR},
		{"conn", cloakdriver.ErrManagerConnRefused, sitelogin.NETWORK_ERROR},
		{"timeout", cloakdriver.ErrManagerTimeout, sitelogin.NETWORK_ERROR},
		{"protocol", cloakdriver.ErrManagerProtocolError, sitelogin.UNKNOWN},
		{"default", errors.New("weird"), sitelogin.UNKNOWN},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := classifyManagerError(tc.err)
			assert.Equal(t, tc.wantStatus, r.Status)
			assert.Equal(t, sitelogin.ProbeSourceCloak, r.Source)
			assert.ErrorIs(t, r.RawError, tc.err)
		})
	}
}

func TestMTorrentClassifyChromedpError(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus sitelogin.ProbeStatus
	}{
		{"deadline", context.DeadlineExceeded, sitelogin.CHALLENGE},
		{"canceled", context.Canceled, sitelogin.UNKNOWN},
		{"parse", errors.New("mtorrent: no lastModifiedDate found"), sitelogin.PARSE_ERROR},
		{"challenge", errors.New("cloudflare challenge"), sitelogin.CHALLENGE},
		{"network", errors.New("bad handshake"), sitelogin.NETWORK_ERROR},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := classifyChromedpError(tc.err)
			assert.Equal(t, tc.wantStatus, r.Status)
			assert.Equal(t, sitelogin.ProbeSourceCloak, r.Source)
		})
	}
}

func TestMTorrentParseTimestampLayouts(t *testing.T) {
	cases := []struct {
		in string
		ok bool
	}{
		{"2026-05-18T01:30:00Z", true},
		{"2026-05-18 09:30:00+08:00", true},
		{"2026-05-18 09:30:00", true},
		{"", false},
		{"not-a-time", false},
	}
	for _, c := range cases {
		_, ok := parseTimestamp(c.in)
		assert.Equal(t, c.ok, ok, "input=%q", c.in)
	}
}

func TestMTorrentParseTimestampCSTFallback(t *testing.T) {
	got, ok := parseTimestamp("2026-05-18 09:30:00")
	assert.True(t, ok)
	assert.True(t, got.UTC().Equal(time.Date(2026, 5, 18, 1, 30, 0, 0, time.UTC)),
		"bare layout must be interpreted as CST (UTC+8): got %s", got.UTC())
}

func TestMTorrentExtractLastModifiedFromJSONLD(t *testing.T) {
	t.Run("top_level_lastModified", func(t *testing.T) {
		obj := map[string]any{"lastModifiedDate": "2026-05-18T01:30:00Z"}
		got, ok := extractLastModifiedFromJSONLD(obj)
		assert.True(t, ok)
		assert.True(t, got.Equal(time.Date(2026, 5, 18, 1, 30, 0, 0, time.UTC)))
	})
	t.Run("dateModified", func(t *testing.T) {
		obj := map[string]any{"dateModified": "2026-05-18T01:30:00Z"}
		_, ok := extractLastModifiedFromJSONLD(obj)
		assert.True(t, ok)
	})
	t.Run("nested_mainEntity", func(t *testing.T) {
		obj := map[string]any{"mainEntity": map[string]any{"lastModifiedDate": "2026-05-18T01:30:00Z"}}
		_, ok := extractLastModifiedFromJSONLD(obj)
		assert.True(t, ok)
	})
	t.Run("none", func(t *testing.T) {
		_, ok := extractLastModifiedFromJSONLD(map[string]any{"foo": "bar"})
		assert.False(t, ok)
	})
	t.Run("unparsable_value", func(t *testing.T) {
		_, ok := extractLastModifiedFromJSONLD(map[string]any{"lastModifiedDate": "bogus"})
		assert.False(t, ok)
	})
}

func TestMTorrentParseProfileCSSFallback(t *testing.T) {
	html := `<html><body><section class="profile-card">
	<dd class="lastModifiedDate">2026-05-18 09:30:00</dd>
	</section></body></html>`
	got, err := parseMTorrentProfilePage(html)
	assert.NoError(t, err)
	assert.True(t, got.UTC().Equal(time.Date(2026, 5, 18, 1, 30, 0, 0, time.UTC)))
}

func TestMTorrentParseProfileLastBrowseFallback(t *testing.T) {
	html := `<html><body><section class="profile-card">
	<dd class="lastBrowse">2026-05-18 09:30:00</dd>
	</section></body></html>`
	got, err := parseMTorrentProfilePage(html)
	assert.NoError(t, err)
	assert.False(t, got.IsZero())
}

func TestMTorrentParseProfileEmptyScriptSkipped(t *testing.T) {
	html := `<html><body>
	<script type="application/ld+json">   </script>
	<script type="application/ld+json">{"@type":"Person"}</script>
	<dd class="lastModifiedDate">2026-05-18T01:30:00Z</dd>
	</body></html>`
	got, err := parseMTorrentProfilePage(html)
	assert.NoError(t, err)
	assert.True(t, got.Equal(time.Date(2026, 5, 18, 1, 30, 0, 0, time.UTC)))
}

func TestMTorrentParseProfileNoMatch(t *testing.T) {
	html := `<html><body><section class="profile-card">no timestamps here</section></body></html>`
	_, err := parseMTorrentProfilePage(html)
	assert.Error(t, err)
}

func TestMTorrentParseProfileIgnoresBadJSONLD(t *testing.T) {
	html := `<html><body>
	<script type="application/ld+json">{ not valid json }</script>
	<dd class="lastModifiedDate">2026-05-18T01:30:00Z</dd>
	</body></html>`
	got, err := parseMTorrentProfilePage(html)
	assert.NoError(t, err)
	assert.True(t, got.Equal(time.Date(2026, 5, 18, 1, 30, 0, 0, time.UTC)))
}
