package sitelogin

import "time"

type ProbeStatus string

const (
	OK              ProbeStatus = "OK"
	SESSION_EXPIRED ProbeStatus = "SESSION_EXPIRED"
	CHALLENGE       ProbeStatus = "CHALLENGE"
	RATE_LIMITED    ProbeStatus = "RATE_LIMITED"
	NETWORK_ERROR   ProbeStatus = "NETWORK_ERROR"
	PARSE_ERROR     ProbeStatus = "PARSE_ERROR"
	KEY_ERROR       ProbeStatus = "KEY_ERROR"
	UNKNOWN         ProbeStatus = "UNKNOWN"
	// NOT_APPLICABLE marks a probe path that was deliberately skipped
	// (e.g. mTorrent CloakBrowser cookie path when no cookie is configured).
	// No Manager call is made and no error is recorded.
	NOT_APPLICABLE ProbeStatus = "NOT_APPLICABLE"
)

func (s ProbeStatus) String() string {
	return string(s)
}

// ProbeSource identifies which authentication path produced a probe's
// last-login timestamp. The caller (LoginReminderMonitor) uses this to
// dispatch the timestamp into either ApiLastLoginAt (API key) or
// CookieLastLoginAt (cookie session).
type ProbeSource string

const (
	ProbeSourceHTTPCookie ProbeSource = "http_cookie"
	ProbeSourceHTTPAPIKey ProbeSource = "http_api_key"
	ProbeSourceCloak      ProbeSource = "cloak"
)

type ProbeResult struct {
	Status       ProbeStatus
	LastLoginAt  *time.Time
	LastAccessAt *time.Time
	Source       ProbeSource
	RawError     error
	Diagnostic   string
}
