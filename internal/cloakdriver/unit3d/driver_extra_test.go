package unit3d

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
