package sitelogin

import (
	"context"
	"errors"
	"strings"
	"time"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func ProbeMTorrent(ctx context.Context, site v2.Site, clock Clock) (*ProbeResult, error) {
	_ = clock
	info, err := site.GetUserInfo(ctx)
	return classifyMTorrentResult(info, err), nil
}

func classifyMTorrentResult(info v2.UserInfo, err error) *ProbeResult {
	if err != nil {
		return classifyMTorrentError(err)
	}

	if info.LastAccess <= 0 {
		return &ProbeResult{
			Status:     PARSE_ERROR,
			Source:     ProbeSourceHTTPAPIKey,
			Diagnostic: "M-Team profile returned no lastModifiedDate / lastBrowse",
		}
	}

	accessAt := time.Unix(info.LastAccess, 0).UTC()
	result := &ProbeResult{
		Status:       OK,
		Source:       ProbeSourceHTTPAPIKey,
		LastAccessAt: &accessAt,
	}
	if info.LastLogin > 0 {
		loginAt := time.Unix(info.LastLogin, 0).UTC()
		result.LastLoginAt = &loginAt
	}
	return result
}

func classifyMTorrentError(err error) *ProbeResult {
	switch {
	case errors.Is(err, v2.ErrCloudflareChallenge):
		return &ProbeResult{Status: CHALLENGE, Source: ProbeSourceHTTPAPIKey, RawError: err}
	case errors.Is(err, v2.ErrSessionExpired):
		return &ProbeResult{Status: SESSION_EXPIRED, Source: ProbeSourceHTTPAPIKey, RawError: err}
	case errors.Is(err, v2.ErrInvalidCredentials):
		// M-Team 用 API Key 认证，401/403 是密钥失效
		return &ProbeResult{Status: KEY_ERROR, Source: ProbeSourceHTTPAPIKey, RawError: err}
	case errors.Is(err, v2.ErrCircuitOpen), errors.Is(err, v2.ErrRateLimited):
		return &ProbeResult{Status: RATE_LIMITED, Source: ProbeSourceHTTPAPIKey, RawError: err}
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, v2.ErrNetworkError):
		return &ProbeResult{Status: NETWORK_ERROR, Source: ProbeSourceHTTPAPIKey, RawError: err}
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "cloudflare") || strings.Contains(msg, "challenge") {
		return &ProbeResult{Status: CHALLENGE, Source: ProbeSourceHTTPAPIKey, RawError: err}
	}

	return &ProbeResult{Status: UNKNOWN, Source: ProbeSourceHTTPAPIKey, RawError: err}
}
