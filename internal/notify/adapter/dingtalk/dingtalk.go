// Package dingtalk 是钉钉群自定义机器人出站通道。加签时 timestamp 是毫秒，签名是以 secret 为 key 对
// "timestamp\nsecret" 做 HmacSHA256 后 Base64，拼在 Webhook 地址后面（&timestamp=…&sign=…）；返回 errcode 非 0 算失败。
package dingtalk

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/notify/adapter/outbound"
	"github.com/sunerpy/pt-tools/models"
)

// Type 是通道类型名。
const Type = "dingtalk"

// Channel 是钉钉群机器人通道（只出站）。
type Channel struct {
	webhook string
	secret  string
	msgType string
	now     func() time.Time
}

// Type 返回通道类型名。
func (c *Channel) Type() string { return Type }

// Init 解析配置：webhook_url 必填；secret（加签密钥，SEC 开头）可选；msg_type 为 markdown（默认）或 text。
func (c *Channel) Init(_ context.Context, conf *models.NotificationConf) error {
	if conf == nil {
		return errors.New("notification conf is nil")
	}
	var cfg struct {
		WebhookURL string `json:"webhook_url"`
		Secret     string `json:"secret"`
		MsgType    string `json:"msg_type"`
	}
	if err := outbound.Config(conf.ConfigJSON, &cfg, "钉钉"); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.WebhookURL) == "" {
		return errors.New("钉钉 webhook_url 为空")
	}
	u, err := outbound.HTTPURL(cfg.WebhookURL, "钉钉 Webhook 地址")
	if err != nil {
		return err
	}
	c.msgType = strings.TrimSpace(cfg.MsgType)
	if c.msgType == "" {
		c.msgType = "markdown"
	}
	if c.msgType != "markdown" && c.msgType != "text" {
		return fmt.Errorf("钉钉 msg_type 无效: %s", c.msgType)
	}
	c.webhook, c.secret = u, strings.TrimSpace(cfg.Secret)
	if c.now == nil {
		c.now = time.Now
	}
	return nil
}

// Sign 是钉钉加签：以 secret 为 key 对 "timestamp\nsecret" 做 HmacSHA256，再 Base64（拼进地址时再 URL 编码）。
func Sign(timestampMillis int64, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(timestampMillis, 10) + "\n" + secret))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// endpoint 是这一次要发的地址：配置了 secret 时带上 timestamp 与 sign。
func (c *Channel) endpoint() (string, error) {
	if c.secret == "" {
		return c.webhook, nil
	}
	u, err := url.Parse(c.webhook)
	if err != nil {
		return "", err
	}
	ts := c.now().UnixMilli()
	q := u.Query()
	q.Set("timestamp", strconv.FormatInt(ts, 10))
	q.Set("sign", Sign(ts, c.secret))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// Send 推送一条通知。
func (c *Channel) Send(ctx context.Context, n notify.Notification) error {
	var payload map[string]any
	if c.msgType == "text" {
		payload = map[string]any{"msgtype": "text", "text": map[string]string{"content": outbound.Text(n)}}
	} else {
		title := strings.TrimSpace(n.Title)
		if title == "" {
			title = "pt-tools"
		}
		text := "### " + title + "\n\n" + n.Text
		if n.Link != "" {
			text += "\n\n[查看](" + n.Link + ")"
		}
		payload = map[string]any{"msgtype": "markdown", "markdown": map[string]string{"title": title, "text": text}}
	}
	ep, err := c.endpoint()
	if err != nil {
		return fmt.Errorf("钉钉推送失败: %w", err)
	}
	body, err := outbound.PostJSON(ctx, ep, payload)
	if err != nil {
		return fmt.Errorf("钉钉推送失败: %w", err)
	}
	var res struct {
		ErrCode *int   `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if json.Unmarshal(body, &res) == nil && res.ErrCode != nil && *res.ErrCode != 0 {
		return fmt.Errorf("钉钉推送失败: errcode=%d, errmsg=%s", *res.ErrCode, res.ErrMsg)
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
