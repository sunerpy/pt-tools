// Package bark 是 Bark（iOS 推送）出站通道：POST {server}/push，JSON 里带 device_key；返回 code 为 200 才算成功。
package bark

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/notify/adapter/outbound"
	"github.com/sunerpy/pt-tools/models"
)

// Type 是通道类型名。
const Type = "bark"

// DefaultServer 是官方服务器；自建的 bark-server 填自己的地址。
const DefaultServer = "https://api.day.app"

// Channel 是 Bark 通道（只出站）。
type Channel struct {
	server       string
	deviceKey    string
	group        string
	sound        string
	allowPrivate bool
	client       *outbound.Client // 为空时用 outbound.Default()
}

// Type 返回通道类型名。
func (c *Channel) Type() string { return Type }

// Init 解析配置：device_key 必填；server 默认官方服务器；group、sound 可选；
// allow_private 打开后才能用本机或内网的服务器。
func (c *Channel) Init(_ context.Context, conf *models.NotificationConf) error {
	if conf == nil {
		return errors.New("notification conf is nil")
	}
	var cfg struct {
		ServerURL    string `json:"server_url"`
		DeviceKey    string `json:"device_key"`
		Group        string `json:"group"`
		Sound        string `json:"sound"`
		AllowPrivate bool   `json:"allow_private"`
	}
	if err := outbound.Config(conf.ConfigJSON, &cfg, "Bark"); err != nil {
		return err
	}
	c.deviceKey = strings.TrimSpace(cfg.DeviceKey)
	if c.deviceKey == "" {
		return errors.New("Bark device_key 为空")
	}
	server := strings.TrimSpace(cfg.ServerURL)
	if server == "" {
		server = DefaultServer
	}
	s, err := outbound.ServerURL(server, "Bark 服务器地址", cfg.AllowPrivate)
	if err != nil {
		return err
	}
	c.server, c.group, c.sound, c.allowPrivate = s, strings.TrimSpace(cfg.Group), strings.TrimSpace(cfg.Sound), cfg.AllowPrivate
	return nil
}

// CheckConfig 只检查配置、不发请求：保存配置前调用。
func (c *Channel) CheckConfig(conf *models.NotificationConf) error {
	return (&Channel{}).Init(context.Background(), conf)
}

// Send 推送一条通知。
func (c *Channel) Send(ctx context.Context, n notify.Notification) error {
	payload := map[string]any{"device_key": c.deviceKey, "title": n.Title, "body": n.Text}
	if n.Link != "" {
		payload["url"] = n.Link
	}
	if c.group != "" {
		payload["group"] = c.group
	}
	if c.sound != "" {
		payload["sound"] = c.sound
	}
	resp, err := outbound.Use(c.client).PostJSON(ctx, outbound.Request{
		URL: c.server + "/push", Payload: payload, AllowPrivate: c.allowPrivate,
	})
	// device_key 不对等错误也会带 HTTP 4xx：响应里有 code 时报 code 与 message（去掉 device_key）
	var res struct {
		Code    *int   `json:"code"`
		Message string `json:"message"`
	}
	parsed := json.Unmarshal(resp.Body, &res) == nil && res.Code != nil
	switch {
	case err == nil && parsed && *res.Code == 200:
		return nil
	case parsed && *res.Code != 200:
		return fmt.Errorf("Bark 推送失败: code=%d, message=%s", *res.Code, outbound.Clean(res.Message, c.deviceKey))
	case err != nil:
		return fmt.Errorf("Bark 推送失败: %w%s", err, outbound.PrivateHint(err))
	default:
		return errors.New("Bark 推送失败: 响应里没有 code，服务器地址可能不是 Bark 服务器")
	}
}

// SupportsInbound 恒为 false：Bark 只出站。
func (c *Channel) SupportsInbound() bool { return false }

// OnInbound 不支持入站，什么也不做。
func (c *Channel) OnInbound(notify.InboundHandler) {}

// Close 没有要释放的资源。
func (c *Channel) Close(context.Context) error { return nil }

// Healthy 恒为 true：无状态，每次发送单独请求。
func (c *Channel) Healthy() bool { return true }

func init() {
	notify.RegisterChannel(Type, func() notify.Channel { return &Channel{} })
}
