package remote

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// 配对链接（v1）：
//
//	pttools://pair?v=1&h=<hostId>&k=<主机 X25519 公钥>&s=<配对密钥>&r=<relay 地址，逗号分隔>&d=<直连地址>
//
// k、s 是 base64url（无填充）的 32 字节；r、d 可选，但至少要有一个。参数按这个顺序写，值按查询串转义。
// 不认识的参数忽略（以后加参数不用升级链接版本），认识的参数重复出现时整个链接作废。
const (
	LinkVersion = 1
	linkScheme  = "pttools"
	linkHost    = "pair"
	// MaxRelays 是一台主机最多配置的 relay 数
	MaxRelays = 4
	// maxURLLen 是 relay 地址与直连地址的长度上限
	maxURLLen = 256
)

var (
	// ErrLink 是配对链接格式不对
	ErrLink = errors.New("配对链接不对")
	// ErrLinkVersion 是配对链接的版本不认识（App 要提示升级）
	ErrLinkVersion = errors.New("配对链接的版本不认识")
	// ErrURL 是 relay 地址或直连地址不对
	ErrURL = errors.New("地址不对")
)

// PairingLink 是配对二维码与链接里的内容。
type PairingLink struct {
	HostID  string
	HostKey []byte
	Secret  []byte
	Relays  []string
	Direct  string
}

// String 写出链接。
func (l PairingLink) String() string {
	s := linkScheme + "://" + linkHost + "?v=" + strconv.Itoa(LinkVersion) +
		"&h=" + l.HostID + "&k=" + EncodeKey(l.HostKey) + "&s=" + EncodeKey(l.Secret)
	if len(l.Relays) > 0 {
		s += "&r=" + url.QueryEscape(strings.Join(l.Relays, ","))
	}
	if l.Direct != "" {
		s += "&d=" + url.QueryEscape(l.Direct)
	}
	return s
}

// ParseLink 读配对链接并检查每一项。
func ParseLink(raw string) (PairingLink, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return PairingLink{}, fmt.Errorf("%w: %v", ErrLink, err)
	}
	if u.Scheme != linkScheme || u.Host != linkHost || (u.Path != "" && u.Path != "/") || u.User != nil || u.Fragment != "" {
		return PairingLink{}, fmt.Errorf("%w: 不是 pttools://pair 链接", ErrLink)
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return PairingLink{}, fmt.Errorf("%w: %v", ErrLink, err)
	}
	for _, k := range []string{"v", "h", "k", "s", "r", "d"} {
		if len(q[k]) > 1 {
			return PairingLink{}, fmt.Errorf("%w: 参数 %s 重复", ErrLink, k)
		}
	}
	if v := q.Get("v"); v != strconv.Itoa(LinkVersion) {
		if v == "" {
			return PairingLink{}, fmt.Errorf("%w: 没有版本", ErrLink)
		}
		return PairingLink{}, fmt.Errorf("%w: %s", ErrLinkVersion, v)
	}
	l := PairingLink{HostID: q.Get("h")}
	if !ValidHostID(l.HostID) {
		return PairingLink{}, fmt.Errorf("%w: hostId 不对", ErrLink)
	}
	if l.HostKey, err = DecodeKey(q.Get("k")); err != nil {
		return PairingLink{}, fmt.Errorf("%w: 主机公钥不对", ErrLink)
	}
	if l.Secret, err = DecodeKey(q.Get("s")); err != nil {
		return PairingLink{}, fmt.Errorf("%w: 配对密钥不对", ErrLink)
	}
	if r := q.Get("r"); r != "" {
		for part := range strings.SplitSeq(r, ",") {
			relay, rerr := NormalizeRelayURL(part)
			if rerr != nil {
				return PairingLink{}, fmt.Errorf("%w: %v", ErrLink, rerr)
			}
			if !slices.Contains(l.Relays, relay) {
				l.Relays = append(l.Relays, relay)
			}
		}
		if len(l.Relays) > MaxRelays {
			return PairingLink{}, fmt.Errorf("%w: relay 超过 %d 个", ErrLink, MaxRelays)
		}
	}
	if d := q.Get("d"); d != "" {
		if l.Direct, err = NormalizeDirectURL(d); err != nil {
			return PairingLink{}, fmt.Errorf("%w: %v", ErrLink, err)
		}
	}
	if len(l.Relays) == 0 && l.Direct == "" {
		return PairingLink{}, fmt.Errorf("%w: 既没有 relay 也没有直连地址", ErrLink)
	}
	return l, nil
}

// NormalizeRelayURL 检查并规整 relay 地址：ws:// 或 wss://，可以带路径前缀；去掉末尾的 /。
func NormalizeRelayURL(raw string) (string, error) {
	return normalizeURL(raw, "relay 地址", "ws", "wss")
}

// NormalizeDirectURL 检查并规整直连地址：http:// 或 https://，可以带反向代理的子路径；去掉末尾的 /。
func NormalizeDirectURL(raw string) (string, error) {
	return normalizeURL(raw, "直连地址", "http", "https")
}

// normalizeURL 只接受「协议://主机[:端口][/路径]」：没有用户信息、查询串、片段与百分号转义；逗号留给 relay 列表做分隔。
func normalizeURL(raw, what string, schemes ...string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" || len(s) > maxURLLen {
		return "", fmt.Errorf("%w: %s的长度要在 1 到 %d 个字符之间", ErrURL, what, maxURLLen)
	}
	if strings.ContainsAny(s, ",%?# \t\r\n\\") {
		return "", fmt.Errorf("%w: %s里不能有逗号、百分号、问号、井号、空白与反斜杠", ErrURL, what)
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %v", ErrURL, what, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if !slices.Contains(schemes, scheme) {
		return "", fmt.Errorf("%w: %s要以 %s:// 或 %s:// 开头", ErrURL, what, schemes[0], schemes[1])
	}
	if u.Opaque != "" || u.User != nil || u.Hostname() == "" {
		return "", fmt.Errorf("%w: %s要写成 %s://主机[:端口][/路径]", ErrURL, what, schemes[1])
	}
	if p := u.Port(); p != "" {
		if n, err := strconv.Atoi(p); err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("%w: %s的端口不对", ErrURL, what)
		}
	}
	out := scheme + "://" + strings.ToLower(u.Host) + strings.TrimRight(u.Path, "/")
	return out, nil
}
