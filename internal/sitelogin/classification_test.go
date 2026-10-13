package sitelogin

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// NexusPHP 系（含共用这条分类的 HDDolby、Rousi）对驱动哨兵错误的归类：
// 401/403 凭证无效、429 限流、传输层失败此前都落成 UNKNOWN，提醒策略和后备探测都用错了状态。
func TestClassifyNexusPHPResult_DriverSentinels(t *testing.T) {
	clock := NewFakeClock(time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC))
	cases := []struct {
		name   string
		err    error
		source ProbeSource
		want   ProbeStatus
	}{
		{"cookie invalid", v2.ErrInvalidCredentials, ProbeSourceHTTPCookie, SESSION_EXPIRED},
		{"auth failed", v2.ErrAuthFailed, ProbeSourceHTTPCookie, SESSION_EXPIRED},
		{"api key invalid", v2.ErrInvalidCredentials, ProbeSourceHTTPAPIKey, KEY_ERROR},
		{"rate limited", fmt.Errorf("HTTP 429: %w", v2.ErrRateLimited), ProbeSourceHTTPCookie, RATE_LIMITED},
		{"network", fmt.Errorf("execute request: %w", fmt.Errorf("%w: dial tcp: connection refused", v2.ErrNetworkError)), ProbeSourceHTTPCookie, NETWORK_ERROR},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := classifyNexusPHPResult(v2.UserInfo{}, tc.err, clock, tc.source)
			assert.NoError(t, err)
			assert.Equal(t, tc.want, res.Status)
		})
	}
}

// M-Team 用 API Key 认证，401/403 是密钥失效；429 是限流；连接失败是网络错误。
func TestClassifyMTorrentError_DriverSentinels(t *testing.T) {
	assert.Equal(t, KEY_ERROR, classifyMTorrentError(v2.ErrInvalidCredentials).Status)
	assert.Equal(t, RATE_LIMITED, classifyMTorrentError(fmt.Errorf("HTTP 429: %w", v2.ErrRateLimited)).Status)
	assert.Equal(t, NETWORK_ERROR, classifyMTorrentError(fmt.Errorf("%w: EOF", v2.ErrNetworkError)).Status)
}

// 登录后的页面取到了用户名、只是没有「最近动向」：会话是好的，按这次探测的时间记最近访问，不报解析失败。
// 连用户名都没有时照旧是解析失败（多半不是登录后的页面）。
func TestClassifyNexusPHPResult_NoLastAccessButLoggedIn(t *testing.T) {
	now := time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC)
	clock := NewFakeClock(now)
	res, err := classifyNexusPHPResult(v2.UserInfo{Username: "someone"}, nil, clock, ProbeSourceHTTPCookie)
	require.NoError(t, err)
	assert.Equal(t, OK, res.Status)
	require.NotNil(t, res.LastAccessAt)
	assert.Equal(t, now, *res.LastAccessAt)
	assert.Contains(t, res.Diagnostic, "最近动向")

	res, err = classifyNexusPHPResult(v2.UserInfo{}, nil, clock, ProbeSourceHTTPCookie)
	require.NoError(t, err)
	assert.Equal(t, PARSE_ERROR, res.Status)
	assert.Nil(t, res.LastAccessAt)
}
