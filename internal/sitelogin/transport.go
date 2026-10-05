package sitelogin

import (
	"context"
	"errors"
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

// ClassifyUserInfo 按站点架构把一次用户信息获取的结果归类，HTTP 主通道与 UserInfoService 主通道共用。
func ClassifyUserInfo(def *v2.SiteDefinition, info v2.UserInfo, err error, clock Clock) *ProbeResult {
	if def == nil {
		return &ProbeResult{Status: UNKNOWN, Diagnostic: "nil site definition"}
	}
	switch def.Schema {
	case v2.SchemaNexusPHP, v2.SchemaHDDolby:
		result, _ := classifyNexusPHPResult(info, err, clock, ProbeSourceHTTPCookie)
		return result
	case v2.SchemaRousi:
		result, _ := classifyNexusPHPResult(info, err, clock, ProbeSourceHTTPAPIKey)
		return result
	case v2.SchemaMTorrent:
		return classifyMTorrentResult(info, err)
	case v2.SchemaGazelle:
		return classifyGazelleResult(info, err)
	case v2.SchemaUnit3D:
		return classifyUnit3DResult(info, err)
	default:
		return &ProbeResult{Status: UNKNOWN, Diagnostic: fmt.Sprintf("unknown schema: %s", def.Schema)}
	}
}

// UserInfoFetcher 取回并保存站点的用户信息，即 v2.UserInfoService 的 FetchAndSave。
type UserInfoFetcher interface {
	FetchAndSave(ctx context.Context, siteID string) (v2.UserInfo, error)
}

// UserInfoServiceTransport 经 UserInfoService 探测：用与搜索共用的站点实例和限速器取用户信息，
// 取到的数据同时写入用户统计。它忽略 FetchUserInfo 的 site 参数，按 SiteID 取已注册的实例。
type UserInfoServiceTransport struct {
	Service UserInfoFetcher
	SiteID  string
}

func (UserInfoServiceTransport) Name() string { return "userinfo" }

func (t UserInfoServiceTransport) FetchUserInfo(ctx context.Context, def *v2.SiteDefinition, _ v2.Site, clock Clock) (*ProbeResult, error) {
	if t.Service == nil {
		return &ProbeResult{Status: UNKNOWN, Diagnostic: "user info service is nil"}, nil
	}
	info, err := t.Service.FetchAndSave(ctx, t.SiteID)
	switch {
	case errors.Is(err, v2.ErrUserInfoPersist):
		// 站点可达，只是统计记录没保存成功：按取回的数据判断站点状态。先于 ErrSiteNotFound 判断，
		// 因为仓库报的「找不到站点」也可能包在保存失败的错误链里。
		sLogger().Warnw("probe_userinfo_persist_failed", "site_name", t.SiteID, "err", err)
		err = nil
	case errors.Is(err, v2.ErrSiteNotFound):
		return &ProbeResult{Status: NOT_CONFIGURED, Diagnostic: "站点没有注册到用户信息服务：凭证缺失或站点实例创建失败"}, nil
	case errors.Is(err, v2.ErrEmptyUsername):
		return &ProbeResult{Status: PARSE_ERROR, Diagnostic: "取回的用户信息没有用户名，可能不是登录后的页面"}, nil
	}
	return ClassifyUserInfo(def, info, err, clock), nil
}

// CloakTransport is a placeholder for T11-T14, when concrete CloakBrowser
// schema drivers are wired under internal/cloakdriver/<schema>/.
type CloakTransport struct{}

func (CloakTransport) Name() string { return "cloak" }

func (CloakTransport) FetchUserInfo(context.Context, *v2.SiteDefinition, v2.Site, Clock) (*ProbeResult, error) {
	return &ProbeResult{Status: UNKNOWN, Diagnostic: "cloak transport pending T11-T14 implementation"}, nil
}
