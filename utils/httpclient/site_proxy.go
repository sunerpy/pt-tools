package httpclient

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
)

// 站点代理：全局设置里的「站点代理」决定哪些站点的请求经代理发出。设置开启时，站点请求按所属站点的 ID 判定，
// 不再看环境变量（这样「这个站点不走代理」才真的不走）；设置关闭、或者请求不知道是哪个站点时，照旧按环境变量。
// 判定每个请求做一次，换了设置不用重建站点实例。

// 站点代理的模式。
const (
	SiteProxyAll     = "all"     // 全部站点
	SiteProxyInclude = "include" // 只有选中的站点
	SiteProxyExclude = "exclude" // 除了选中的站点
)

// SiteProxyPolicy 是站点代理设置的只读快照（换设置时整个换掉，不要改它的字段）。
type SiteProxyPolicy struct {
	Enabled bool
	// Proxy 是校验过的代理地址，可能带用户名密码：只在内存里，不要写进日志
	Proxy string
	Mode  string
	// Sites 是选中的站点 ID（NormalizeSiteID 以后的）
	Sites map[string]bool
}

var siteProxy atomic.Pointer[SiteProxyPolicy]

// SetSiteProxyPolicy 换上新的快照，之后发出的站点请求按它判定；nil 表示关闭。
func SetSiteProxyPolicy(p *SiteProxyPolicy) { siteProxy.Store(p) }

// NormalizeSiteID 是站点 ID 的规范写法：去掉首尾空白、转小写（站点设置里的名字就是注册表 ID 的小写）。
func NormalizeSiteID(id string) string { return strings.ToLower(strings.TrimSpace(id)) }

// ResolveSiteProxy 判断站点 siteID 的请求走不走代理。decided 为假表示这里不管（设置关闭，或者没有站点 ID），
// 调用方照旧按环境变量；decided 为真时 proxy 为空表示直连。
func ResolveSiteProxy(siteID string) (proxy string, decided bool) {
	p := siteProxy.Load()
	if p == nil || !p.Enabled || p.Proxy == "" {
		return "", false
	}
	id := NormalizeSiteID(siteID)
	if id == "" {
		return "", false
	}
	use := true
	switch p.Mode {
	case SiteProxyInclude:
		use = p.Sites[id]
	case SiteProxyExclude:
		use = !p.Sites[id]
	}
	if use {
		return p.Proxy, true
	}
	return "", true
}

// envProxyFuncs 是「按环境变量取代理」的两种回退：requests 用的（认 ALL_PROXY）与 http.Transport 用的（Go 自带，不认 ALL_PROXY）。
type envProxyFuncs struct {
	forURL     func(rawURL string) string
	forRequest func(*http.Request) (*url.URL, error)
}

var envProxy atomic.Pointer[envProxyFuncs]

func init() {
	envProxy.Store(&envProxyFuncs{forURL: ResolveProxyFromEnvironment, forRequest: http.ProxyFromEnvironment})
}

// ResolveSiteProxyOrEnv 是 requests 发站点请求时用的代理地址：站点代理判定了就用它，否则按环境变量（认 ALL_PROXY）。
// 空串表示直连。调用方无论结果是不是空都要 WithProxy（清掉 Transport 池里残留的代理，见 NewSession）。
func ResolveSiteProxyOrEnv(siteID, rawURL string) string {
	if proxy, ok := ResolveSiteProxy(siteID); ok {
		return proxy
	}
	return envProxy.Load().forURL(rawURL)
}

// SiteTransportProxy 是 http.Transport 发站点请求时用的代理：站点代理判定了就用它（空串直连），
// 否则按 http.ProxyFromEnvironment（不认 ALL_PROXY）。
func SiteTransportProxy(siteID string, req *http.Request) (*url.URL, error) {
	if proxy, ok := ResolveSiteProxy(siteID); ok {
		if proxy == "" {
			return nil, nil
		}
		return url.Parse(proxy)
	}
	return envProxy.Load().forRequest(req)
}

type siteIDKey struct{}

// WithSiteID 把站点 ID 放进请求的 context：SiteTransport 的 Proxy 按它判定。
func WithSiteID(ctx context.Context, siteID string) context.Context {
	return context.WithValue(ctx, siteIDKey{}, siteID)
}

// SiteIDFrom 取出 WithSiteID 放进去的站点 ID，没有时是空串。
func SiteIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(siteIDKey{}).(string)
	return id
}

var (
	siteTransportOnce sync.Once
	siteTransport     *http.Transport
)

// SiteTransport 是按站点代理判定的 http.Transport（默认 Transport 的副本，Proxy 换成 SiteTransportProxy）：
// 请求的 context 里用 WithSiteID 放上站点 ID。所有调用方共用这一个，连接池也共用。
func SiteTransport() *http.Transport {
	siteTransportOnce.Do(func() {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.Proxy = func(req *http.Request) (*url.URL, error) {
			return SiteTransportProxy(SiteIDFrom(req.Context()), req)
		}
		siteTransport = tr
	})
	return siteTransport
}

// SetEnvProxyForTest 让「按环境变量取代理」的两种回退都返回 proxy（空串表示直连），返回还原函数。只给测试用：
// Go 的 http.ProxyFromEnvironment 第一次调用以后就缓存环境变量，而且对回环地址一律直连，改环境变量证明不了回退。
// 替换期间所有走回退的请求都得到这个地址，用到它的测试不要并行。
func SetEnvProxyForTest(proxy string) (restore func()) {
	fake := &envProxyFuncs{
		forURL: func(string) string { return proxy },
		forRequest: func(*http.Request) (*url.URL, error) {
			if proxy == "" {
				return nil, nil
			}
			return url.Parse(proxy)
		},
	}
	prev := envProxy.Swap(fake)
	return func() { envProxy.Store(prev) }
}

// ParseProxyURL 校验代理地址：http、https、socks5、socks5h，要有主机（和 TMDB、AI 助手的代理同样的规则）。
func ParseProxyURL(s string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Host == "" {
		return nil, errors.New("代理地址格式不对")
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
		return u, nil
	}
	return nil, errors.New("代理地址要以 http://、https://、socks5:// 或 socks5h:// 开头")
}

// MaskProxyURL 把代理地址里的密码换成 ***（给界面看）；没有用户信息时原样返回。解析不了时整个换成 ***
// （存进去的都校验过，正常走不到；万一走到也不把原文交出去）。
func MaskProxyURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "***"
	}
	if u.User == nil {
		return raw
	}
	if _, ok := u.User.Password(); ok {
		u.User = url.UserPassword(u.User.Username(), "***")
	}
	return strings.Replace(u.String(), "%2A%2A%2A", "***", 1)
}
