package sitelogin

import (
	"context"
	"errors"
	"strings"
	"time"

	sitev2 "github.com/sunerpy/pt-tools/site/v2"
)

func ProbeGazelle(ctx context.Context, site sitev2.Site, clock Clock) (*ProbeResult, error) {
	if site == nil {
		return &ProbeResult{Status: UNKNOWN, Diagnostic: "site is nil"}, errors.New("site is nil")
	}

	info, err := site.GetUserInfo(ctx)
	if err != nil {
		return classifyGazelleError(err), nil
	}

	if info.LastAccess == 0 {
		return &ProbeResult{
			Status:     PARSE_ERROR,
			Source:     ProbeSourceHTTPCookie,
			Diagnostic: "gazelle response missing stats.LastAccess",
		}, nil
	}

	access := time.Unix(info.LastAccess, 0).UTC()
	return &ProbeResult{
		Status:       OK,
		Source:       ProbeSourceHTTPCookie,
		LastAccessAt: &access,
		LastLoginAt:  nil,
	}, nil
}

func classifyGazelleError(err error) *ProbeResult {
	switch {
	case errors.Is(err, sitev2.ErrSessionExpired), errors.Is(err, sitev2.ErrInvalidCredentials), errors.Is(err, sitev2.ErrAuthFailed):
		return &ProbeResult{Status: SESSION_EXPIRED, Source: ProbeSourceHTTPCookie, RawError: err, Diagnostic: err.Error()}
	case errors.Is(err, sitev2.ErrCircuitOpen), errors.Is(err, sitev2.ErrRateLimited):
		return &ProbeResult{Status: RATE_LIMITED, Source: ProbeSourceHTTPCookie, RawError: err, Diagnostic: err.Error()}
	case errors.Is(err, sitev2.ErrParseError):
		return &ProbeResult{Status: PARSE_ERROR, Source: ProbeSourceHTTPCookie, RawError: err, Diagnostic: err.Error()}
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled), errors.Is(err, sitev2.ErrNetworkError):
		return &ProbeResult{Status: NETWORK_ERROR, Source: ProbeSourceHTTPCookie, RawError: err, Diagnostic: err.Error()}
	}

	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "cloudflare"), strings.Contains(msg, "challenge"):
		return &ProbeResult{Status: CHALLENGE, Source: ProbeSourceHTTPCookie, RawError: err, Diagnostic: err.Error()}
	case strings.Contains(msg, "api key"), strings.Contains(msg, "apikey"):
		return &ProbeResult{Status: KEY_ERROR, Source: ProbeSourceHTTPCookie, RawError: err, Diagnostic: err.Error()}
	}

	return &ProbeResult{Status: UNKNOWN, Source: ProbeSourceHTTPCookie, RawError: err, Diagnostic: err.Error()}
}
