package mtorrent

import (
	"context"
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
