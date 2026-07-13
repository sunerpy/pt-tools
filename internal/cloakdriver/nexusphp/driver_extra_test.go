package nexusphp

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/cloakdriver"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
)

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
