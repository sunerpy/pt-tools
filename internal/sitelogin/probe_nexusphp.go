package sitelogin

import (
	"context"
	"errors"
	"strings"
	"time"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// ProbeNexusPHP wraps Site.GetUserInfo() for NexusPHP-schema sites.
// All I/O is delegated to the driver; this function performs only classification.
func ProbeNexusPHP(ctx context.Context, site v2.Site, clock Clock, source ProbeSource) (*ProbeResult, error) {
	info, err := site.GetUserInfo(ctx)
	return classifyNexusPHPResult(info, err, clock, source)
}

func classifyNexusPHPResult(info v2.UserInfo, err error, _ Clock, source ProbeSource) (*ProbeResult, error) {
	result := &ProbeResult{Source: source}

	if err == nil {
		if info.LastAccess > 0 {
			result.Status = OK
			access := time.Unix(info.LastAccess, 0).UTC()
			result.LastAccessAt = &access
			if info.LastLogin > 0 {
				login := time.Unix(info.LastLogin, 0).UTC()
				result.LastLoginAt = &login
			}
			return result, nil
		}
		result.Status = PARSE_ERROR
		// NexusPHP 读到了用户名时由 withNexusPHPAccessFallback 按探测时间记；走到这里的 NexusPHP 是连用户名都没有
		if strings.TrimSpace(info.Username) == "" {
			result.Diagnostic = diagNoUsername
		} else {
			result.Diagnostic = diagNoLastAccess
		}
		return result, nil
	}

	result.RawError = err

	switch {
	case errors.Is(err, v2.ErrSessionExpired):
		result.Status = SESSION_EXPIRED
	case errors.Is(err, v2.ErrInvalidCredentials), errors.Is(err, v2.ErrAuthFailed):
		// 驱动把 401/403 映射成凭证无效：Cookie 来源算会话过期，API Key 来源（HDDolby、Rousi）算密钥错误
		if source == ProbeSourceHTTPAPIKey {
			result.Status = KEY_ERROR
		} else {
			result.Status = SESSION_EXPIRED
		}
	case errors.Is(err, v2.ErrCircuitOpen), errors.Is(err, v2.ErrRateLimited):
		result.Status = RATE_LIMITED
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, v2.ErrNetworkError):
		result.Status = NETWORK_ERROR
	case isChallengeError(err):
		result.Status = CHALLENGE
	default:
		result.Status = UNKNOWN
		if isAuthError(err) {
			result.Diagnostic = "Cookie 未配置或已失效，请用浏览器扩展同步 Cookie 后重试"
		} else {
			result.Diagnostic = err.Error()
		}
	}

	return result, nil
}

// withNexusPHPAccessFallback 只用于 NexusPHP（HDDolby、Rousi 走各自的 API，不用它）：
// 登录后的页面取到了用户名，只是页面上没有「最近动向」时，会话是好的，而这次取用户信息本身就是一次访问
// （NexusPHP 每次登录后的请求都更新最近访问时间），所以按这次探测的时间记，不报解析失败。
func withNexusPHPAccessFallback(result *ProbeResult, info v2.UserInfo, err error, clock Clock) *ProbeResult {
	if result == nil || err != nil || result.Status != PARSE_ERROR || info.LastAccess != 0 || strings.TrimSpace(info.Username) == "" {
		return result
	}
	access := clock.Now().UTC()
	result.Status = OK
	result.LastAccessAt = &access
	result.Diagnostic = "页面上没有找到「最近动向」，按这次探测访问的时间记"
	return result
}

func isChallengeError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, v2.ErrCloudflareChallenge) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "cloudflare") || strings.Contains(msg, "challenge")
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "cookie") ||
		strings.Contains(msg, "401") ||
		strings.Contains(msg, "403") ||
		strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "forbidden") ||
		strings.Contains(msg, "login")
}
