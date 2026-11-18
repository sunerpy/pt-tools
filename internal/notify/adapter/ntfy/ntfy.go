// Package ntfy 是 ntfy 出站通道：按 JSON 发布，POST 到服务器根地址 {topic, title, message, priority, click}；
// 可选 access token（Bearer）或用户名密码（Basic）。服务器回发布出来的消息（带 id）才算成功。
package ntfy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/notify/adapter/outbound"
	"github.com/sunerpy/pt-tools/models"
)

// Type 是通道类型名。
const Type = "ntfy"

// DefaultServer 是公共服务器；自建的填自己的地址。
const DefaultServer = "https://ntfy.sh"

var topicRe = regexp.MustCompile(`^[-_A-Za-z0-9]{1,64}$`)

// Channel 是 ntfy 通道（只出站）。
type Channel struct {
	server       string
	topic        string
	token        string
	username     string
	password     string
	priority     int
	allowPrivate bool
	client       *outbound.Client // 为空时用 outbound.Default()
}

// Type 返回通道类型名。
func (c *Channel) Type() string { return Type }

// Init 解析配置：topic 必填（字母、数字、-、_，最多 64 个）；server 默认公共服务器；priority 0 表示默认，1–5；
// allow_private 打开后才能用本机或内网的服务器。
func (c *Channel) Init(_ context.Context, conf *models.NotificationConf) error {
	if conf == nil {
		return errors.New("notification conf is nil")
	}
	var cfg struct {
		ServerURL    string `json:"server_url"`
		Topic        string `json:"topic"`
		Token        string `json:"token"`
		Username     string `json:"username"`
		Password     string `json:"password"`
		Priority     int    `json:"priority"`
		AllowPrivate bool   `json:"allow_private"`
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
	s, err := outbound.ServerURL(server, "ntfy 服务器地址", cfg.AllowPrivate)
	if err != nil {
		return err
	}
	c.server, c.token, c.username, c.password = s, strings.TrimSpace(cfg.Token), strings.TrimSpace(cfg.Username), cfg.Password
	c.priority, c.allowPrivate = cfg.Priority, cfg.AllowPrivate
	return nil
}

// CheckConfig 只检查配置、不发请求：保存配置前调用。
func (c *Channel) CheckConfig(conf *models.NotificationConf) error {
	return (&Channel{}).Init(context.Background(), conf)
}

// authorization 是鉴权头的值：有 token 用 Bearer，否则有用户名时用 Basic，都没有时为空。
func (c *Channel) authorization() string {
	switch {
	case c.token != "":
		return "Bearer " + c.token
	case c.username != "":
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(c.username+":"+c.password))
	}
	return ""
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
	var header map[string]string
	auth := c.authorization()
	if auth != "" {
		header = map[string]string{"Authorization": auth}
	}
	resp, err := outbound.Use(c.client).PostJSON(ctx, outbound.Request{
		URL: c.server + "/", Payload: payload, Header: header, AllowPrivate: c.allowPrivate,
	})
	// 成功时回发布出来的消息（带 id）；失败时回 {"code":40101,"http":401,"error":"unauthorized",…}
	var res struct {
		ID    string `json:"id"`
		Code  int    `json:"code"`
		Error string `json:"error"`
	}
	parsed := json.Unmarshal(resp.Body, &res) == nil
	switch {
	case err == nil && parsed && res.ID != "":
		return nil
	case parsed && res.Error != "":
		return fmt.Errorf("ntfy 推送失败: HTTP %d, code=%d, error=%s", resp.Status, res.Code,
			outbound.Clean(res.Error, c.token, c.password, strings.TrimPrefix(auth, "Basic ")))
	case err != nil:
		return fmt.Errorf("ntfy 推送失败: %w%s", err, outbound.PrivateHint(err))
	default:
		return errors.New("ntfy 推送失败: 响应里没有消息 id，服务器地址可能不是 ntfy 服务器")
	}
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
