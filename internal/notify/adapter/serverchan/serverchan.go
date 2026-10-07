// Package serverchan 是 Server 酱出站通道。SendKey 以 sctp 开头的是 Server 酱³（https://<uid>.push.ft07.com/send/<key>.send），
// 其余按 Turbo 版（https://sctapi.ftqq.com/<key>.send）；POST JSON {title, desp}，返回 code 非 0 算失败。
package serverchan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/notify/adapter/outbound"
	"github.com/sunerpy/pt-tools/models"
)

// Type 是通道类型名。
const Type = "serverchan"

// titleMax 是标题最多几个字（Server 酱的标题上限是 32 个字，超出的放进正文）。
const titleMax = 32

var sc3Key = regexp.MustCompile(`^sctp(\d+)t`)

// Channel 是 Server 酱通道（只出站）。
type Channel struct {
	endpoint string
}

// Type 返回通道类型名。
func (c *Channel) Type() string { return Type }

// Endpoint 是 SendKey 对应的发送地址。
func Endpoint(sendKey string) (string, error) {
	key := strings.TrimSpace(sendKey)
	if key == "" {
		return "", errors.New("Server 酱 send_key 为空")
	}
	if strings.ContainsAny(key, "/?#% ") {
		return "", errors.New("Server 酱 send_key 格式不对")
	}
	if m := sc3Key.FindStringSubmatch(key); m != nil {
		return fmt.Sprintf("https://%s.push.ft07.com/send/%s.send", m[1], url.PathEscape(key)), nil
	}
	return fmt.Sprintf("https://sctapi.ftqq.com/%s.send", url.PathEscape(key)), nil
}

// Init 解析配置：send_key 必填。
func (c *Channel) Init(_ context.Context, conf *models.NotificationConf) error {
	if conf == nil {
		return errors.New("notification conf is nil")
	}
	var cfg struct {
		SendKey string `json:"send_key"`
	}
	if err := outbound.Config(conf.ConfigJSON, &cfg, "Server 酱"); err != nil {
		return err
	}
	ep, err := Endpoint(cfg.SendKey)
	if err != nil {
		return err
	}
	c.endpoint = ep
	return nil
}

// Send 推送一条通知：标题超过 32 个字时截断，完整标题放进正文第一行。
func (c *Channel) Send(ctx context.Context, n notify.Notification) error {
	title := strings.TrimSpace(n.Title)
	desp := n.Text
	if r := []rune(title); len(r) > titleMax {
		title = string(r[:titleMax-1]) + "…"
		desp = n.Title + "\n\n" + desp
	}
	if title == "" {
		title = "pt-tools"
	}
	if n.Link != "" {
		desp += "\n\n" + n.Link
	}
	body, err := outbound.PostJSON(ctx, c.endpoint, map[string]any{"title": title, "desp": desp})
	if err != nil {
		return fmt.Errorf("Server 酱推送失败: %w", err)
	}
	var res struct {
		Code    *int   `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &res) == nil && res.Code != nil && *res.Code != 0 {
		return fmt.Errorf("Server 酱推送失败: code=%d, message=%s", *res.Code, res.Message)
	}
	return nil
}

// SupportsInbound 恒为 false。
func (c *Channel) SupportsInbound() bool { return false }

// OnInbound 不支持入站。
func (c *Channel) OnInbound(notify.InboundHandler) {}

// Close 没有要释放的资源。
func (c *Channel) Close(context.Context) error { return nil }

// Healthy 恒为 true。
func (c *Channel) Healthy() bool { return true }

func init() {
	notify.RegisterChannel(Type, func() notify.Channel { return &Channel{} })
}
