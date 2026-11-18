package feishuapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"unicode"

	lark "github.com/larksuite/oapi-sdk-go/v3"
)

// 平台：飞书与 Lark 国际版是同一种通道，按 domain 切换开放平台的域名。
const (
	DomainFeishu = "feishu"
	DomainLark   = "lark"
)

// Config 是 NotificationConf.ConfigJSON 解密后的配置，已经校验、规整过。
type Config struct {
	AppID     string
	AppSecret string
	Domain    string
	// AdminUsers / AllowedUsers 是 open_id（ou_ 开头）名单；两个都空时谁都不放行
	AdminUsers   []string
	AllowedUsers []string
	// DefaultChatID 是没有指定收件人的通知发到哪：oc_（群或单聊会话）、ou_（个人）或 on_（union_id）
	DefaultChatID string
	// ProxyURL 是通道级代理（http 或 socks5）；为空时跟 HTTPS_PROXY / NO_PROXY
	ProxyURL string
}

type rawConfig struct {
	AppID         string          `json:"app_id"`
	AppSecret     string          `json:"app_secret"`
	Domain        string          `json:"domain"`
	AdminUsers    json.RawMessage `json:"admin_users"`
	AllowedUsers  json.RawMessage `json:"allowed_users"`
	DefaultChatID json.RawMessage `json:"default_chat_id"`
	ProxyURL      string          `json:"proxy_url"`
}

// parseConfig 解析并校验明文配置，不做任何 I/O：Init 与 CheckConfig（保存前）用同一套规则。
func parseConfig(raw string) (*Config, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("配置为空：要填 App ID 与 App Secret")
	}
	var rc rawConfig
	if err := json.Unmarshal([]byte(raw), &rc); err != nil {
		return nil, fmt.Errorf("配置不是合法的 JSON: %w", err)
	}
	cfg := &Config{
		AppID:     strings.TrimSpace(rc.AppID),
		AppSecret: strings.TrimSpace(rc.AppSecret),
		Domain:    strings.ToLower(strings.TrimSpace(rc.Domain)),
		ProxyURL:  strings.TrimSpace(rc.ProxyURL),
	}
	if cfg.AppID == "" {
		return nil, errors.New("App ID 不能为空")
	}
	if strings.IndexFunc(cfg.AppID, unicode.IsSpace) >= 0 {
		return nil, errors.New("App ID 中间不能有空白")
	}
	if cfg.AppSecret == "" {
		return nil, errors.New("App Secret 不能为空")
	}
	switch cfg.Domain {
	case "":
		cfg.Domain = DomainFeishu
	case DomainFeishu, DomainLark:
	default:
		return nil, errors.New("平台只能是 feishu（飞书）或 lark（Lark 国际版）")
	}
	var err error
	if cfg.AdminUsers, err = openIDList(rc.AdminUsers, "管理员 open_id"); err != nil {
		return nil, err
	}
	if cfg.AllowedUsers, err = openIDList(rc.AllowedUsers, "允许的用户 open_id"); err != nil {
		return nil, err
	}
	if cfg.DefaultChatID, err = defaultChatID(rc.DefaultChatID); err != nil {
		return nil, err
	}
	if cfg.ProxyURL != "" {
		if err := checkProxyURL(cfg.ProxyURL); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

// openIDList 读名单：字符串数组，去掉首尾空白与空项、去重。只认 ou_ 开头的 open_id，不对就拒绝保存——
// 静默跳过的话，用户填了 user_id 或 union_id，保存以后就把自己关在门外了。
func openIDList(raw json.RawMessage, what string) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var items []string
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("%s要是字符串数组", what)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item)
		if id == "" || slices.Contains(out, id) {
			continue
		}
		if !strings.HasPrefix(id, "ou_") {
			return nil, fmt.Errorf("名单里要填 open_id，以 ou_ 开头：%q 不是", shorten(id, 64))
		}
		out = append(out, id)
	}
	return out, nil
}

func defaultChatID(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	bad := errors.New("默认会话 ID 要以 oc_（群或单聊会话）、ou_（个人 open_id）或 on_（union_id）开头")
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", bad
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if _, err := receiveIDType(s); err != nil {
		return "", bad
	}
	return s, nil
}

// checkProxyURL 只认 http 与 socks5：长连接用的 gorilla/websocket 不支持 https 协议的代理。
// 地址里可能带用户名与密码，所以错误里不写它。
func checkProxyURL(raw string) error {
	bad := errors.New("代理地址要以 http:// 或 socks5:// 开头，并带上主机与端口")
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "socks5") {
		return bad
	}
	return nil
}

// proxyFunc 是 HTTP 请求与长连接共用的代理：通道填了 proxy_url 就用它，否则跟环境变量。
func proxyFunc(cfg *Config) func(*http.Request) (*url.URL, error) {
	if cfg.ProxyURL == "" {
		return http.ProxyFromEnvironment
	}
	u, err := url.Parse(cfg.ProxyURL)
	if err != nil {
		return http.ProxyFromEnvironment
	}
	return http.ProxyURL(u)
}

// baseURL 是开放平台的地址：正式构建只认飞书与 Lark 两个官方域名，qa 构建可以改连本地的假服务。
func baseURL(cfg *Config) string {
	if u := qaBaseURL(); u != "" {
		return strings.TrimRight(u, "/")
	}
	if cfg.Domain == DomainLark {
		return lark.LarkBaseUrl
	}
	return lark.FeishuBaseUrl
}

// permitted 报告这个 open_id 在不在管理员或允许名单里（两个都空时谁都不在）。
func (c *Config) permitted(openID string) bool {
	if openID == "" {
		return false
	}
	return slices.Contains(c.AdminUsers, openID) || slices.Contains(c.AllowedUsers, openID)
}

func shorten(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
