// Package ntfy 是 ntfy 出站通道：按 JSON 发布，POST 到服务器根地址 {topic, title, message, priority, click}；
// 可选 access token（Bearer）或用户名密码（Basic）。
package ntfy

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/notify/adapter/outbound"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/utils/httpclient"
)

// Type 是通道类型名。
const Type = "ntfy"

// DefaultServer 是公共服务器；自建的填自己的地址。
const DefaultServer = "https://ntfy.sh"

var topicRe = regexp.MustCompile(`^[-_A-Za-z0-9]{1,64}$`)

// Channel 是 ntfy 通道（只出站）。
type Channel struct {
	server   string
	topic    string
	token    string
	username string
	password string
	priority int
}

// Type 返回通道类型名。
func (c *Channel) Type() string { return Type }

// Init 解析配置：topic 必填（字母、数字、-、_，最多 64 个）；server 默认公共服务器；priority 0 表示默认，1–5。
func (c *Channel) Init(_ context.Context, conf *models.NotificationConf) error {
	if conf == nil {
		return errors.New("notification conf is nil")
	}
	var cfg struct {
		ServerURL string `json:"server_url"`
		Topic     string `json:"topic"`
		Token     string `json:"token"`
		Username  string `json:"username"`
		Password  string `json:"password"`
		Priority  int    `json:"priority"`
	}
	if err := outbound.Config(conf.ConfigJSON, &cfg, "ntfy"); err != nil {
		return err
	}
	c.topic = strings.TrimSpace(cfg.Topic)
	if !topicRe.MatchString(c.topic) {
		return errors.New("ntfy topic 只能是字母、数字、- 和 _，最多 64 个字符")
	}
	if cfg.Priority < 0 || cfg.Priority > 5 {
		return errors.New("ntfy priority 要在 1 到 5 之间（0 表示默认）")
	}
	server := strings.TrimSpace(cfg.ServerURL)
	if server == "" {
		server = DefaultServer
	}
	s, err := outbound.HTTPURL(server, "ntfy 服务器地址")
	if err != nil {
		return err
	}
	c.server, c.token, c.username, c.password, c.priority = s, strings.TrimSpace(cfg.Token), strings.TrimSpace(cfg.Username), cfg.Password, cfg.Priority
	return nil
}

// Send 发布一条消息。
func (c *Channel) Send(ctx context.Context, n notify.Notification) error {
	payload := map[string]any{"topic": c.topic, "title": n.Title, "message": n.Text}
	if strings.TrimSpace(n.Text) == "" {
		payload["message"] = n.Title
	}
	if c.priority > 0 {
		payload["priority"] = c.priority
	}
	if n.Link != "" {
		payload["click"] = n.Link
	}
	// 鉴权头自己拼：请求选项里的 BearerToken / BasicAuth 不会写进请求
	var opts []httpclient.RequestOption
	switch {
	case c.token != "":
		opts = append(opts, httpclient.WithHeader("Authorization", "Bearer "+c.token))
	case c.username != "":
		basic := base64.StdEncoding.EncodeToString([]byte(c.username + ":" + c.password))
		opts = append(opts, httpclient.WithHeader("Authorization", "Basic "+basic))
	}
	if _, err := outbound.PostJSON(ctx, c.server+"/", payload, opts...); err != nil {
		return fmt.Errorf("ntfy 推送失败: %w", err)
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
