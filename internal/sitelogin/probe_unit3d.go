package sitelogin

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// ProbeUnit3D wraps a Unit3D Site.GetUserInfo call and classifies any error
// into the 8-status ProbeStatus taxonomy. It never issues raw HTTP requests;
// all transport flows through the underlying driver (which inherits circuit
// breaking and rate limiting from site/v2/http_client.go).
func ProbeUnit3D(ctx context.Context, site v2.Site, clock Clock) (*ProbeResult, error) {
	if site == nil {
		return nil, errors.New("ProbeUnit3D: site is nil")
	}
	if clock == nil {
		clock = NewRealClock()
	}

	info, err := site.GetUserInfo(ctx)
	if err != nil {
		status, diagnostic := classifyDriverError(err)
		return &ProbeResult{
			Status:     status,
			Source:     ProbeSourceHTTPCookie,
			RawError:   err,
			Diagnostic: diagnostic,
		}, nil
	}

	if info.LastLogin == 0 && info.LastAccess == 0 {
		return &ProbeResult{
			Status:     PARSE_ERROR,
			Source:     ProbeSourceHTTPCookie,
			Diagnostic: "Unit3D driver returned UserInfo without last_login or last_action",
		}, nil
	}

	res := &ProbeResult{Status: OK, Source: ProbeSourceHTTPCookie}
	if info.LastLogin > 0 {
		t := time.Unix(info.LastLogin, 0).UTC()
		res.LastLoginAt = &t
	}
	if info.LastAccess > 0 {
		t := time.Unix(info.LastAccess, 0).UTC()
		res.LastAccessAt = &t
	}
	_ = clock
	return res, nil
}

// classifyDriverError maps a driver error to a ProbeStatus using sentinel
// errors via errors.Is wherever possible. Sentinel errors are preferred over
// string matching; the only string-based fallback is for Cloudflare/challenge
// indicators since site/v2 does not yet expose a dedicated sentinel for that.
func classifyDriverError(err error) (ProbeStatus, string) {
	switch {
	case err == nil:
		return OK, ""
	case errors.Is(err, v2.ErrSessionExpired):
		return SESSION_EXPIRED, "session expired"
	case errors.Is(err, v2.ErrInvalidCredentials):
		return KEY_ERROR, "invalid credentials"
	case errors.Is(err, v2.ErrCircuitOpen), errors.Is(err, v2.ErrRateLimited):
		return RATE_LIMITED, "rate limited or circuit open"
	case errors.Is(err, context.DeadlineExceeded),
		errors.Is(err, context.Canceled),
		errors.Is(err, v2.ErrNetworkError):
		return NETWORK_ERROR, "network error"
	case errors.Is(err, v2.ErrParseError):
		return PARSE_ERROR, "parse error"
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return NETWORK_ERROR, fmt.Sprintf("net.Error: %v", netErr)
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "cloudflare") || strings.Contains(msg, "challenge") || strings.Contains(msg, "cf-chl") {
		return CHALLENGE, "challenge detected"
	}

	return UNKNOWN, err.Error()
}
