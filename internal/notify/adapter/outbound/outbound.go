// Package outbound 是只出站的通知适配器（Bark、Server 酱、ntfy、钉钉、飞书、企业微信群机器人）共用的发送工具。
//
// 发送走自己的 http.Client：请求绑定调用方的 ctx（超时就取消，不会超时之后才送达、再被重试送一遍）；
// 不走环境代理、不跟随跳转；连接前检查目标地址（链路本地、组播与保留地址一律不连，本机与内网只在允许时连）；
// 响应最多读 64 KiB；HTTP 错误只报状态码，响应里的错误信息由各适配器挑字段、用 Clean 脱敏后再报。
package outbound

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/sunerpy/pt-tools/internal/notify"
)

// MaxBody 是响应最多读多少字节。
const MaxBody = 64 << 10

// ErrTooLarge 表示响应超过 MaxBody。
var ErrTooLarge = errors.New("响应超过 64 KiB")

// HTTPError 表示 HTTP 状态码不是 2xx。只带状态码：响应体可能回显请求里的密钥。
type HTTPError struct{ Status int }

func (e *HTTPError) Error() string {
	if e.Status >= 300 && e.Status < 400 {
		return fmt.Sprintf("HTTP %d：服务器要求跳转，通知不跟随跳转，请填写跳转后的地址", e.Status)
	}
	return fmt.Sprintf("HTTP %d", e.Status)
}

// AddrError 表示连接前检查目标地址没通过。
type AddrError struct {
	Addr string
	// Private 为 true 时是本机或内网地址（允许内网时能连）；为 false 时是链路本地、组播或保留地址，一律不连。
	Private bool
}

func (e *AddrError) Error() string {
	if e.Private {
		return e.Addr + " 是本机或内网地址"
	}
	return e.Addr + " 是链路本地、组播或保留地址，不用于通知"
}

// PrivateHint 在 err 是「本机或内网地址」时给出打开「允许内网地址」的提示，否则为空串（给自建服务器的通道用）。
func PrivateHint(err error) string {
	var ae *AddrError
	if errors.As(err, &ae) && ae.Private {
		return "（自建在本机或内网的服务器请打开「允许内网地址」）"
	}
	return ""
}

var (
	// alwaysBlocked 一律不连：0.0.0.0/8、链路本地（含云厂商的元数据地址 169.254.169.254）、组播、保留地址。
	alwaysBlocked = []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/8"),
		netip.MustParsePrefix("169.254.0.0/16"),
		netip.MustParsePrefix("224.0.0.0/4"),
		netip.MustParsePrefix("240.0.0.0/4"),
		netip.MustParsePrefix("::/128"),
		netip.MustParsePrefix("fe80::/10"),
		netip.MustParsePrefix("ff00::/8"),
	}
	// privateNets 只在允许内网时连：本机、内网、运营商级 NAT（100.64.0.0/10，Tailscale 也用它）、IPv6 唯一本地地址。
	privateNets = []netip.Prefix{
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("172.16.0.0/12"),
		netip.MustParsePrefix("192.168.0.0/16"),
		netip.MustParsePrefix("100.64.0.0/10"),
		netip.MustParsePrefix("::1/128"),
		netip.MustParsePrefix("fc00::/7"),
	}
	nat64 = netip.MustParsePrefix("64:ff9b::/96")
)

// CheckIP 检查一个要连的 IP（规则见 alwaysBlocked 与 privateNets）。
func CheckIP(ip netip.Addr, allowPrivate bool) error {
	// 带 zone 的地址不匹配任何网段，要先去掉；IPv4 映射与 NAT64 地址按里面的 IPv4 判断
	ip = ip.WithZone("").Unmap()
	if nat64.Contains(ip) {
		b := ip.As16()
		ip = netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]})
	}
	if !ip.IsValid() {
		return &AddrError{Addr: "空地址"}
	}
	for _, p := range alwaysBlocked {
		if p.Contains(ip) {
			return &AddrError{Addr: ip.String()}
		}
	}
	if !allowPrivate {
		for _, p := range privateNets {
			if p.Contains(ip) {
				return &AddrError{Addr: ip.String(), Private: true}
			}
		}
	}
	return nil
}

// CheckHost 在保存配置时检查地址里的主机：是 IP 或 localhost 时按 CheckIP 的规则检查；
// 其余主机名不查 DNS（不做 I/O），连接时再按解析出的 IP 检查。
func CheckHost(host string, allowPrivate bool) error {
	h := strings.TrimSuffix(strings.ToLower(host), ".")
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		if allowPrivate {
			return nil
		}
		return &AddrError{Addr: host, Private: true}
	}
	ip, err := netip.ParseAddr(h)
	if err != nil {
		return nil
	}
	return CheckIP(ip, allowPrivate)
}

// Options 是 Client 的选项。
type Options struct {
	// Divert 非空时（host:port），主机名不是 IP 也不是 localhost 的连接都改连到这里，且不校验证书。
	// 只给测试与 qa 构建用：让发往各家官方地址的请求落到本地的收件端。
	Divert string
}

// Client 发通知请求。用 NewClient 或 Default 取得。
type Client struct {
	// 允许与不允许内网各用一套连接池：同一个地址的连接不能被另一种策略的请求复用
	public, private *http.Client
}

var defaultClient = NewClient(Options{Divert: qaDivert()})

// Default 是正式发送用的 Client。
func Default() *Client { return defaultClient }

// Use 返回 c；c 为空时返回 Default()。
func Use(c *Client) *Client {
	if c != nil {
		return c
	}
	return defaultClient
}

// NewClient 建一个 Client。
func NewClient(o Options) *Client {
	return &Client{public: newHTTPClient(o, false), private: newHTTPClient(o, true)}
}

func newHTTPClient(o Options, allowPrivate bool) *http.Client {
	checked := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		// Control 拿到的是解析之后真正要连的 IP：DNS 换了解析结果也逃不过这一步
		Control: func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil {
				return &AddrError{Addr: address}
			}
			return CheckIP(ap.Addr(), allowPrivate)
		},
	}
	plain := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	tr := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if o.Divert != "" && diverted(addr) {
				return plain.DialContext(ctx, network, o.Divert)
			}
			return checked.DialContext(ctx, network, addr)
		},
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        16,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	if o.Divert != "" {
		// 只在测试与 qa 构建里：本地收件端用的是自签证书
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &http.Client{
		Transport: tr,
		// 调用方都带了更短的 ctx 超时；这里兜底，防止有人传进不会到期的 ctx
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// diverted 判断 Divert 是否接管这个连接：主机名不是 IP 也不是 localhost。
func diverted(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return false
	}
	_, err = netip.ParseAddr(host)
	return err != nil
}

// Request 是一次发送。
type Request struct {
	URL     string
	Payload any
	Header  map[string]string
	// AllowPrivate 允许连本机与内网地址（自建在内网的 Bark、ntfy 由用户打开）。
	AllowPrivate bool
}

// Response 是 HTTP 状态码与响应体。
type Response struct {
	Status int
	Body   []byte
}

// PostJSON 把 r.Payload 以 JSON POST 到 r.URL。状态码不是 2xx 时返回 *HTTPError，响应体照样返回
// （给适配器挑错误字段）。错误里不带请求地址：Webhook 地址、SendKey 往往就在地址里。
func (c *Client) PostJSON(ctx context.Context, r Request) (Response, error) {
	body, err := json.Marshal(r.Payload)
	if err != nil {
		return Response{}, fmt.Errorf("序列化请求失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL, bytes.NewReader(body))
	if err != nil {
		// 解析错误会带上整个地址，不往外传
		return Response{}, errors.New("请求地址格式不对")
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range r.Header {
		req.Header.Set(k, v)
	}
	hc := c.public
	if r.AllowPrivate {
		hc = c.private
	}
	resp, err := hc.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return Response{}, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxBody+1))
	if err != nil {
		return Response{Status: resp.StatusCode}, fmt.Errorf("读取响应失败: %w", err)
	}
	if len(data) > MaxBody {
		return Response{Status: resp.StatusCode}, ErrTooLarge
	}
	out := Response{Status: resp.StatusCode, Body: data}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return out, &HTTPError{Status: resp.StatusCode}
	}
	return out, nil
}

// Clean 整理服务端返回的错误信息再放进错误：把 secrets（配置里的密钥、token）换成 ***、
// 空白压成一个空格、最多 120 个字符。
func Clean(msg string, secrets ...string) string {
	s := msg
	for _, sec := range secrets {
		if sec = strings.TrimSpace(sec); len(sec) >= 3 {
			s = strings.ReplaceAll(s, sec, "***")
		}
	}
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 120 {
		s = string(r[:120]) + "…"
	}
	return s
}

// Text 是标题和正文拼成的纯文本（标题为空时只有正文），链接附在最后。
func Text(n notify.Notification) string {
	parts := []string{}
	if t := strings.TrimSpace(n.Title); t != "" {
		parts = append(parts, t)
	}
	if t := strings.TrimSpace(n.Text); t != "" {
		parts = append(parts, t)
	}
	if l := strings.TrimSpace(n.Link); l != "" {
		parts = append(parts, l)
	}
	return strings.Join(parts, "\n")
}

// HTTPURL 校验并规整一个 http(s) 地址（去掉末尾的 /）；不能带用户名密码、? 或 #（后面还要拼路径）。
func HTTPURL(raw, what string) (string, error) {
	s := strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", fmt.Errorf("%s要以 http:// 或 https:// 开头，不能带用户名密码、? 或 #", what)
	}
	return s, nil
}

// ServerURL 是自建服务器地址：HTTPURL 的规则，再加上主机是 IP 或 localhost 时按 CheckHost 检查。
func ServerURL(raw, what string, allowPrivate bool) (string, error) {
	s, err := HTTPURL(raw, what)
	if err != nil {
		return "", err
	}
	u, _ := url.Parse(s)
	if err := CheckHost(u.Hostname(), allowPrivate); err != nil {
		return "", fmt.Errorf("%s不能用：%w%s", what, err, PrivateHint(err))
	}
	return s, nil
}

// Config 解析 ConfigJSON 到 dst。
func Config(conf string, dst any, typ string) error {
	if strings.TrimSpace(conf) == "" {
		conf = "{}"
	}
	if err := json.Unmarshal([]byte(conf), dst); err != nil {
		return fmt.Errorf("解析 %s 配置失败: %w", typ, err)
	}
	return nil
}
