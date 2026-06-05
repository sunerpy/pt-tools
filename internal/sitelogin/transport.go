package sitelogin

import (
	"context"
	"fmt"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// Transport abstracts how a probe fetches user info from a PT site.
//
// Metis AP-3: dispatcher uses a transport-pluggable single probe, not parallel
// dual probes. ProbeWithFallback selects at most one fallback after the primary
// result is classified as fallback-eligible.
type Transport interface {
	Name() string
	FetchUserInfo(ctx context.Context, def *v2.SiteDefinition, site v2.Site, clock Clock) (*ProbeResult, error)
}

// HTTPTransport delegates to the existing schema-specific probe functions.
type HTTPTransport struct{}

func (HTTPTransport) Name() string { return "http" }

func (HTTPTransport) FetchUserInfo(ctx context.Context, def *v2.SiteDefinition, site v2.Site, clock Clock) (*ProbeResult, error) {
	if def == nil {
		return &ProbeResult{Status: UNKNOWN, Diagnostic: "nil site definition"}, nil
	}

	switch def.Schema {
	case v2.SchemaNexusPHP, v2.SchemaHDDolby:
		return ProbeNexusPHP(ctx, site, clock, ProbeSourceHTTPCookie)
	case v2.SchemaRousi:
		return ProbeNexusPHP(ctx, site, clock, ProbeSourceHTTPAPIKey)
	case v2.SchemaMTorrent:
		return ProbeMTorrent(ctx, site, clock)
	case v2.SchemaGazelle:
		return ProbeGazelle(ctx, site, clock)
	case v2.SchemaUnit3D:
		return ProbeUnit3D(ctx, site, clock)
	default:
		return &ProbeResult{Status: UNKNOWN, Diagnostic: fmt.Sprintf("unknown schema: %s", def.Schema)}, nil
	}
}

// CloakTransport is a placeholder for T11-T14, when concrete CloakBrowser
// schema drivers are wired under internal/cloakdriver/<schema>/.
type CloakTransport struct{}

func (CloakTransport) Name() string { return "cloak" }

func (CloakTransport) FetchUserInfo(context.Context, *v2.SiteDefinition, v2.Site, Clock) (*ProbeResult, error) {
	return &ProbeResult{Status: UNKNOWN, Diagnostic: "cloak transport pending T11-T14 implementation"}, nil
}
