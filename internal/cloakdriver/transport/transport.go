// Package transport 把 CloakBrowser 的各架构驱动接成登录探测的后备通道（sitelogin.Transport）。
//
// 放在 cloakdriver 一侧而不是 sitelogin：各架构驱动已经依赖 sitelogin，反过来会循环引用。
// scheduler 也不直接依赖这里，由 cmd 接线时经 LoginReminderConfig.Fallback 注入 Provider。
package transport

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/cloakdriver"
	"github.com/sunerpy/pt-tools/internal/cloakdriver/gazelle"
	"github.com/sunerpy/pt-tools/internal/cloakdriver/mtorrent"
	"github.com/sunerpy/pt-tools/internal/cloakdriver/nexusphp"
	"github.com/sunerpy/pt-tools/internal/cloakdriver/unit3d"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// Prober 是各架构 CloakBrowser 驱动的 Probe。
type Prober func(ctx context.Context, url string, cookies []*http.Cookie, profileID string) (*sitelogin.ProbeResult, error)

// cloakSlot 让同一时间只跑一个 CloakBrowser 探测：所有站点共用同一个 profile，并发启动会互相打断。
var cloakSlot = make(chan struct{}, 1)

// stopReserve 是各驱动在 defer 里停止 profile 的时间上限（驱动的 stopTimeout）。停止用的是独立的 ctx，
// 不受探测预算约束，所以交给驱动的 ctx 提前这么久到期，整次探测才能在预算内返回。
const stopReserve = 10 * time.Second

// Transport 经 CloakBrowser 打开站点的个人页探测登录状态。
type Transport struct {
	URL       string
	Cookies   []*http.Cookie
	ProfileID string
	Probe     Prober
}

func (t *Transport) Name() string { return "cloak" }

// FetchUserInfo 排队拿到 profile 后调用驱动；排队同样受 ctx（探测预算）约束。
func (t *Transport) FetchUserInfo(ctx context.Context, _ *v2.SiteDefinition, _ v2.Site, _ sitelogin.Clock) (*sitelogin.ProbeResult, error) {
	select {
	case cloakSlot <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-cloakSlot }()

	probeCtx := ctx
	if deadline, ok := ctx.Deadline(); ok {
		var cancel context.CancelFunc
		probeCtx, cancel = context.WithDeadline(ctx, deadline.Add(-stopReserve))
		defer cancel()
	}
	res, err := t.Probe(probeCtx, t.URL, t.Cookies, t.ProfileID)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return &sitelogin.ProbeResult{Status: sitelogin.UNKNOWN, Source: sitelogin.ProbeSourceCloak, Diagnostic: "cloak: empty result"}, nil
	}
	res.Source = sitelogin.ProbeSourceCloak
	return res, nil
}

// Settings 是 CloakBrowser 的三项配置，Token 是解密后的明文。
type Settings struct {
	Endpoint  string
	Token     string
	ProfileID string
}

// Identity 是站点上的用户身份，来自最近一次成功探测保存的用户信息。
type Identity struct {
	UserID   string
	Username string
}

// Provider 按站点构造本次可用的后备通道，实现 scheduler.FallbackProvider。
type Provider struct {
	// Settings 每次探测都读一次，改配置不需要重启。
	Settings func() (Settings, error)
	// Identity 返回站点的用户 ID 与用户名；还没有成功探测过时返回 false。
	Identity func(ctx context.Context, siteName string) (Identity, bool)
	// Cookie 返回解密后的站点 Cookie。
	Cookie func(setting models.SiteSetting) (string, error)
	// NewManager 默认 cloakdriver.NewManagerClient。
	NewManager func(endpoint, token string) cloakdriver.ManagerClient
	// Drivers 按架构返回驱动的 Probe，默认用 internal/cloakdriver 下的各驱动。
	Drivers func(schema v2.Schema, manager cloakdriver.ManagerClient) Prober
}

// FallbackFor 在三项配置齐全、架构支持、站点有 Cookie 且需要的身份信息已知时返回后备通道，否则返回 nil。
func (p *Provider) FallbackFor(ctx context.Context, setting models.SiteSetting, def *v2.SiteDefinition) sitelogin.Transport {
	if p == nil || def == nil || p.Settings == nil {
		return nil
	}
	cfg, err := p.Settings()
	if err != nil || strings.TrimSpace(cfg.Endpoint) == "" || strings.TrimSpace(cfg.Token) == "" || strings.TrimSpace(cfg.ProfileID) == "" {
		return nil
	}
	cookies := p.cookies(setting)
	if len(cookies) == 0 {
		return nil
	}
	var id Identity
	if needsIdentity(def.Schema) {
		if p.Identity == nil {
			return nil
		}
		got, ok := p.Identity(ctx, setting.Name)
		if !ok {
			return nil
		}
		id = got
	}
	target, ok := NavigationURL(setting, def, id)
	if !ok {
		return nil
	}
	newManager := p.NewManager
	if newManager == nil {
		newManager = defaultManager
	}
	drivers := p.Drivers
	if drivers == nil {
		drivers = defaultDriver
	}
	probe := drivers(def.Schema, newManager(cfg.Endpoint, cfg.Token))
	if probe == nil {
		return nil
	}
	return &Transport{URL: target, Cookies: cookies, ProfileID: cfg.ProfileID, Probe: probe}
}

func (p *Provider) cookies(setting models.SiteSetting) []*http.Cookie {
	if p.Cookie == nil {
		return nil
	}
	raw, err := p.Cookie(setting)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	cookies, err := http.ParseCookie(strings.TrimSpace(raw))
	if err != nil {
		return nil
	}
	return cookies
}

func needsIdentity(schema v2.Schema) bool {
	switch schema {
	case v2.SchemaNexusPHP, v2.SchemaUnit3D, v2.SchemaGazelle:
		return true
	default:
		return false
	}
}

// NavigationURL 按架构构造个人页地址。站点地址与主通道的取法一致：SiteSetting.APIUrl 优先，
// 为空时用定义的默认地址（与 SiteRegistry.CreateSite 的 customBaseURL 规则相同），否则两条通道会访问不同域名，
// Cookie 也对不上。M-Team 的 APIUrl 与 URLs[0] 都是 API 地址，网页固定用 WebURL。HDDolby、Rousi 不启用后备。
func NavigationURL(setting models.SiteSetting, def *v2.SiteDefinition, id Identity) (string, bool) {
	if def == nil {
		return "", false
	}
	base := strings.TrimRight(strings.TrimSpace(setting.APIUrl), "/")
	if base == "" && len(def.URLs) > 0 {
		base = strings.TrimRight(def.URLs[0], "/")
	}
	switch def.Schema {
	case v2.SchemaNexusPHP:
		if base == "" || id.UserID == "" {
			return "", false
		}
		return base + "/userdetails.php?id=" + url.QueryEscape(id.UserID), true
	case v2.SchemaUnit3D:
		if base == "" || id.Username == "" {
			return "", false
		}
		return base + "/users/" + url.PathEscape(id.Username), true
	case v2.SchemaGazelle:
		if base == "" || id.UserID == "" {
			return "", false
		}
		return base + "/user.php?id=" + url.QueryEscape(id.UserID), true
	case v2.SchemaMTorrent:
		web := strings.TrimRight(def.WebURL, "/")
		if web == "" {
			return "", false
		}
		return web + "/profile", true
	default:
		return "", false
	}
}

func defaultManager(endpoint, token string) cloakdriver.ManagerClient {
	return cloakdriver.NewManagerClient(endpoint, token, 0)
}

func defaultDriver(schema v2.Schema, manager cloakdriver.ManagerClient) Prober {
	switch schema {
	case v2.SchemaNexusPHP:
		return nexusphp.NewDriver(manager).Probe
	case v2.SchemaUnit3D:
		return unit3d.NewDriver(manager).Probe
	case v2.SchemaGazelle:
		return gazelle.NewDriver(manager).Probe
	case v2.SchemaMTorrent:
		return mtorrent.NewDriver(manager).Probe
	default:
		return nil
	}
}

// ErrNotConfigured 表示 CloakBrowser 的三项配置不齐。
var ErrNotConfigured = errors.New("cloakbrowser fallback not configured")
