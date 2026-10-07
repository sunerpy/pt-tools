// Package outbound 是只出站的通知适配器（Bark、Server 酱、ntfy、钉钉、飞书）共用的发送工具。
package outbound

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/utils/httpclient"
)

// PostJSON 把 payload 以 JSON 发到 endpoint，返回响应体。HTTP 4xx/5xx 算失败。
// 错误里不带请求地址：Webhook 地址、SendKey 往往就在地址里。
func PostJSON(ctx context.Context, endpoint string, payload any, opts ...httpclient.RequestOption) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}
	all := append([]httpclient.RequestOption{httpclient.WithContext(ctx), httpclient.WithContentType("application/json")}, opts...)
	resp, err := httpclient.Post(endpoint, body, all...)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	if code := resp.StatusCode(); code >= 400 {
		return resp.Bytes(), fmt.Errorf("HTTP %d: %s", code, Truncate(resp.Bytes()))
	}
	return resp.Bytes(), nil
}

// Truncate 截短响应体用于错误信息（最多 200 字节，按字符截断）。
func Truncate(b []byte) string {
	const maxLen = 200
	s := strings.TrimSpace(string(b))
	if len(s) <= maxLen {
		return s
	}
	cut := maxLen
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
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

// HTTPURL 校验并规整一个 http(s) 地址（去掉末尾的 /）；不能带用户名密码或 #。
func HTTPURL(raw, what string) (string, error) {
	s := strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Fragment != "" {
		return "", fmt.Errorf("%s要以 http:// 或 https:// 开头，不能带用户名密码或 #", what)
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
