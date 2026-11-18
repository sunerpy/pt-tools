package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/redact"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// FeatureCredentialVerify 写在 /api/ping 的 features 里：扩展看到它才会把认不出格式的 Cookie 带上 verify_cookie 推过来。
// 旧版 pt-tools 不认 verify_cookie，会不验证直接保存，可能拿一组没登录的 Cookie 换掉能用的那组。
const FeatureCredentialVerify = "site_credential_verify"

// cookieVerifyBudget 是验证一组 Cookie 的时间上限：扩展的弹窗在等，比登录探测的预算（150 秒）短。
const cookieVerifyBudget = 60 * time.Second

// cookieVerifier 拿一组 Cookie 实际访问站点一次，返回登录探测的结果和要存的那组 Cookie（访问时站点续期换发了令牌就是换发后的，
// 否则就是原样）。默认是 probeSiteCookie，测试里替换。
type cookieVerifier func(ctx context.Context, siteName string, sc models.SiteConfig) (*sitelogin.ProbeResult, string)

// probeSiteCookie 用 sc 里的凭据（Cookie 是这次推来的，API Key、Passkey 是库里的）新建一个站点实例，
// 按登录探测的取法取一次用户信息并判定：和定时的登录探测同一套页面与规则，只是不用已注册的实例（那上面是旧 Cookie），
// 也不走 CloakBrowser 后备（后备用的是库里的 Cookie）。会续期的站点（BYR）访问时要是换发了令牌，返回换发后的那组。
func probeSiteCookie(ctx context.Context, siteName string, sc models.SiteConfig) (*sitelogin.ProbeResult, string) {
	def, ok := v2.GetDefinitionRegistry().Get(siteName)
	if !ok || def == nil {
		return &sitelogin.ProbeResult{Status: sitelogin.UNSUPPORTED, Diagnostic: "站点没有内置定义"}, sc.Cookie
	}
	registry := GetSiteRegistry()
	if registry == nil {
		return &sitelogin.ProbeResult{Status: sitelogin.UNKNOWN, Diagnostic: "站点服务还没有初始化"}, sc.Cookie
	}
	site, err := registry.CreateSite(siteName, v2.SiteCredentials{
		Cookie:  sc.Cookie,
		APIKey:  sc.APIKey,
		Passkey: sc.Passkey,
	}, sc.APIUrl)
	if err != nil {
		return &sitelogin.ProbeResult{Status: sitelogin.UNKNOWN, RawError: err, Diagnostic: "创建站点实例失败：" + err.Error()}, sc.Cookie
	}
	defer site.Close()
	ctx, cancel := context.WithTimeout(ctx, cookieVerifyBudget)
	defer cancel()
	result, _ := sitelogin.ProbeWithFallback(ctx, def, site, sitelogin.NewRealClock(), sitelogin.HTTPTransport{}, nil)
	if result == nil {
		result = &sitelogin.ProbeResult{Status: sitelogin.UNKNOWN, Diagnostic: "探测没有结果"}
	}
	cookie := sc.Cookie
	if renewed, ok := v2.CurrentSessionCookie(site); ok && renewed != "" {
		cookie = renewed
	}
	return result, cookie
}

// verifyCookieOrReject 先用 sc 里这次推来的 Cookie 实际访问站点一次：登录上了返回要存的那组 Cookie 与 true，调用方接着照常保存
// （站点访问时续期换发了令牌就是换发后的那组）；没登录上、或者没法确认时回 422 和原因并返回 false，什么也不保存，
// pt-tools 里原来的凭据不动。和登录探测共用单站锁：同一个站点正在探测时回 409，免得两拨请求挤在一起被站点限流。
func (s *Server) verifyCookieOrReject(ctx context.Context, w http.ResponseWriter, siteName string, sc models.SiteConfig) (string, bool) {
	if s.mgr != nil {
		if mon := s.mgr.GetLoginReminderMonitor(); mon != nil {
			release, ok := mon.TryAcquireProbeLock(siteName)
			if !ok {
				http.Error(w, "这个站点正在做登录检查，过一会儿再同步一次", http.StatusConflict)
				return "", false
			}
			defer release()
		}
	}
	verify := s.verifyCookie
	if verify == nil {
		verify = probeSiteCookie
	}
	started := time.Now()
	result, cookie := verify(ctx, siteName, sc)
	if result == nil {
		result = &sitelogin.ProbeResult{Status: sitelogin.UNKNOWN}
	}
	global.GetSlogger().Infof("[Site] 验证扩展推来的 Cookie: site=%s status=%s elapsed=%s",
		siteName, result.Status, time.Since(started).Round(time.Millisecond))
	if result.Status == sitelogin.OK {
		if strings.TrimSpace(cookie) == "" {
			cookie = sc.Cookie
		}
		return cookie, true
	}
	http.Error(w, cookieVerifyFailure(siteName, result), http.StatusUnprocessableEntity)
	return "", false
}

// cookieVerifyFailure 是验证没通过时给用户看的话：哪里不行，后面带上探测的说明（遮掉凭证），最后说明原来的 Cookie 没动。
func cookieVerifyFailure(siteName string, result *sitelogin.ProbeResult) string {
	name := siteLabel(siteName)
	var reason string
	switch result.Status {
	case sitelogin.SESSION_EXPIRED, sitelogin.KEY_ERROR:
		reason = "用这组 Cookie 没能登录 " + name
	case sitelogin.PARSE_ERROR:
		reason = fmt.Sprintf("用这组 Cookie 打开 %s，页面上没有用户名，多半没有登录", name)
	case sitelogin.CHALLENGE:
		reason = name + " 要求人机验证（如 Cloudflare），pt-tools 的服务器过不去"
	case sitelogin.RATE_LIMITED:
		reason = name + " 限制了访问频率，过一会儿再同步一次"
	case sitelogin.NETWORK_ERROR:
		reason = "pt-tools 的服务器连不上 " + name
	case sitelogin.UNSUPPORTED:
		reason = name + " 没有内置的站点定义，没法在线验证"
	default:
		reason = "没能确认这组 Cookie 能不能登录 " + name
	}
	msg := "没有保存：" + reason
	if detail := probeDetail(result); detail != "" {
		msg += "（" + detail + "）"
	}
	return msg + "。pt-tools 里原来的 Cookie 没有改动"
}

// probeDetail 是探测结果里能给用户看的说明：优先原始错误，401/403 换成一句话；页面上没有用户名时不重复。
func probeDetail(result *sitelogin.ProbeResult) string {
	if result.Status == sitelogin.PARSE_ERROR {
		return ""
	}
	var detail string
	switch err := result.RawError; {
	case err != nil && (errors.Is(err, v2.ErrInvalidCredentials) || errors.Is(err, v2.ErrAuthFailed)):
		return "站点回了 401 或 403：没有权限访问，或者这组 Cookie 没有登录"
	case err != nil:
		detail = err.Error()
	default:
		detail = result.Diagnostic
	}
	detail = redact.Secrets(strings.Join(strings.Fields(detail), " "))
	if r := []rune(detail); len(r) > 200 {
		detail = string(r[:200]) + "…"
	}
	return detail
}

// siteLabel 是站点给人看的名字：内置定义里的名字，没有定义时用配置名。
func siteLabel(siteName string) string {
	if def, ok := v2.GetDefinitionRegistry().Get(siteName); ok && def.Name != "" {
		return def.Name
	}
	return siteName
}
