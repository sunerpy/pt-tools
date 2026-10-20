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
	// NOT_CONFIGURED 表示站点缺少其认证方式要求的凭证，本次没有向站点发请求，不计失败、不提醒。
	NOT_CONFIGURED ProbeStatus = "NOT_CONFIGURED"
	// UNSUPPORTED 表示站点没有内置定义（动态站点），无法构造探测，不发请求、不计失败、不提醒。
	UNSUPPORTED ProbeStatus = "UNSUPPORTED"
)

// IsCredentialFailure 报告该状态是否说明凭证本身失效（需要用户重新同步），而不是暂时性故障。
func (s ProbeStatus) IsCredentialFailure() bool {
	return s == SESSION_EXPIRED || s == KEY_ERROR
}

// IsFailure 报告该状态是否计入失败连续段。OK、NOT_CONFIGURED、UNSUPPORTED、NOT_APPLICABLE 都不算。
func (s ProbeStatus) IsFailure() bool {
	switch s {
	case OK, NOT_CONFIGURED, UNSUPPORTED, NOT_APPLICABLE, "":
		return false
	default:
		return true
	}
}

func (s ProbeStatus) String() string {
	return string(s)
}

// 解析失败时写给用户看的说明：缺了什么，下一步做什么。
const (
	// diagNoUsername：页面能打开，但上面没有用户名，多半不是登录后的页面。
	diagNoUsername = "取回的页面上没有用户名，多半不是登录后的页面：Cookie 失效时有的站点不跳登录页，而是回一个别的页面；" +
		"也可能是站点改了页面。先在浏览器里打开站点确认能登录，再用浏览器扩展重新同步 Cookie；仍然这样请反馈站点名"
	// diagNoLastAccess：取到了用户信息，但里面没有最近访问时间（站点接口或页面改了）。
	diagNoLastAccess = "取回了用户信息，但里面没有最近访问时间，可能是站点改了接口或页面；请反馈站点名"
)

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
	// FallbackNote 记录后备通道没有成功时的说明（保留主通道结果时附带），写进 LastProbeError。
	FallbackNote string
	Source       ProbeSource
	RawError     error
	Diagnostic   string
}
