package sitelogin

import (
	"context"
	"fmt"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// Probe routes the request to the schema-specific probe function based on
// def.Schema. All I/O is performed through site.GetUserInfo, which inherits
// the project's circuit breaker and rate limiter from site/v2/http_client.go;
// dispatcher.go itself MUST NOT issue raw HTTP requests.
//
// Panics raised by individual probes are recovered and surfaced as a
// ProbeResult with Status=UNKNOWN so that a single misbehaving probe cannot
// crash the LoginReminderMonitor goroutine and stop progress for other
// sites.
//
// HDDolby and Rousi schemas share the NexusPHP probe implementation because
// their drivers expose the same UserInfo shape (LastAccess + LastLogin).
func Probe(ctx context.Context, def *v2.SiteDefinition, site v2.Site, clock Clock) (*ProbeResult, error) {
	return ProbeWithFallback(ctx, def, site, clock, HTTPTransport{}, nil)
}

// ProbeWithFallback runs the primary transport first and invokes fallback only
// for statuses that indicate transport-level blocking rather than invalid user
// credentials. It never runs both transports concurrently.
func ProbeWithFallback(ctx context.Context, def *v2.SiteDefinition, site v2.Site, clock Clock, primary, fallback Transport) (*ProbeResult, error) {
	result, err := probeWithTransport(ctx, def, site, clock, primary)
	if err != nil || result == nil || fallback == nil || !isFallbackEligible(result.Status) {
		return result, err
	}
	return probeWithTransport(ctx, def, site, clock, fallback)
}

func probeWithTransport(ctx context.Context, def *v2.SiteDefinition, site v2.Site, clock Clock, transport Transport) (result *ProbeResult, err error) {
	if def == nil {
		return &ProbeResult{Status: UNKNOWN, Diagnostic: "nil site definition"}, nil
	}
	if transport == nil {
		return &ProbeResult{Status: UNKNOWN, Diagnostic: "nil transport"}, nil
	}

	defer func() {
		if r := recover(); r != nil {
			panicErr := fmt.Errorf("probe panic: %v", r)
			result = &ProbeResult{Status: UNKNOWN, RawError: panicErr, Diagnostic: panicErr.Error()}
			err = nil
			sLogger().Errorw("probe_panic", "site_name", def.ID, "schema", string(def.Schema), "transport", transport.Name(), "panic", r)
		}
	}()

	sLogger().Infow("probe_started", "site_name", def.ID, "schema", string(def.Schema), "transport", transport.Name())

	result, err = transport.FetchUserInfo(ctx, def, site, clock)

	if result == nil {
		result = &ProbeResult{Status: UNKNOWN, Diagnostic: "probe returned nil result"}
	}
	sLogger().Infow("probe_classified", "site_name", def.ID, "schema", string(def.Schema), "transport", transport.Name(), "status", string(result.Status))
	return result, err
}

func isFallbackEligible(status ProbeStatus) bool {
	switch status {
	case RATE_LIMITED, CHALLENGE, NETWORK_ERROR:
		return true
	default:
		return false
	}
}
