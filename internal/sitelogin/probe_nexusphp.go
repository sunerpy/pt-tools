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

func classifyNexusPHPResult(info v2.UserInfo, err error, clock Clock, source ProbeSource) (*ProbeResult, error) {
	now := clock.Now()
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
		result.Diagnostic = "driver returned UserInfo with zero LastAccess at " + now.UTC().Format(time.RFC3339)
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
