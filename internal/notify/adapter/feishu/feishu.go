// Package feishu 是飞书（Lark）群自定义机器人出站通道。签名校验时 timestamp 是秒，签名是以 "timestamp\nsecret" 为 key
// 对空数据做 HmacSHA256 后 Base64，与 timestamp 一起放在请求体里；返回 code 非 0 算失败。
package feishu

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/notify/adapter/outbound"
	"github.com/sunerpy/pt-tools/models"
)

// Type 是通道类型名。
const Type = "feishu"

// Channel 是飞书群机器人通道（只出站）。
type Channel struct {
	webhook string
	secret  string
	now     func() time.Time
}

// Type 返回通道类型名。
func (c *Channel) Type() string { return Type }

// Init 解析配置：webhook_url 必填；secret（签名校验密钥）可选。
func (c *Channel) Init(_ context.Context, conf *models.NotificationConf) error {
	if conf == nil {
		return errors.New("notification conf is nil")
	}
	var cfg struct {
		WebhookURL string `json:"webhook_url"`
		Secret     string `json:"secret"`
	}
	if err := outbound.Config(conf.ConfigJSON, &cfg, "飞书"); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.WebhookURL) == "" {
		return errors.New("飞书 webhook_url 为空")
	}
	u, err := outbound.HTTPURL(cfg.WebhookURL, "飞书 Webhook 地址")
	if err != nil {
		return err
	}
	c.webhook, c.secret = u, strings.TrimSpace(cfg.Secret)
	if c.now == nil {
		c.now = time.Now
	}
	return nil
}

// Sign 是飞书签名：以 "timestamp\nsecret" 为 key 对空数据做 HmacSHA256，再 Base64。
func Sign(timestampSeconds int64, secret string) string {
	mac := hmac.New(sha256.New, []byte(strconv.FormatInt(timestampSeconds, 10)+"\n"+secret))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// Send 推送一条文本消息。
func (c *Channel) Send(ctx context.Context, n notify.Notification) error {
	payload := map[string]any{"msg_type": "text", "content": map[string]string{"text": outbound.Text(n)}}
	if c.secret != "" {
		ts := c.now().Unix()
		payload["timestamp"] = strconv.FormatInt(ts, 10)
		payload["sign"] = Sign(ts, c.secret)
	}
	body, err := outbound.PostJSON(ctx, c.webhook, payload)
	if err != nil {
		return fmt.Errorf("飞书推送失败: %w", err)
	}
	var res struct {
		Code       *int   `json:"code"`
		Msg        string `json:"msg"`
		StatusCode *int   `json:"StatusCode"`
	}
	if json.Unmarshal(body, &res) == nil {
		if res.Code != nil && *res.Code != 0 {
			return fmt.Errorf("飞书推送失败: code=%d, msg=%s", *res.Code, res.Msg)
		}
		if res.Code == nil && res.StatusCode != nil && *res.StatusCode != 0 {
			return fmt.Errorf("飞书推送失败: StatusCode=%d", *res.StatusCode)
		}
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
