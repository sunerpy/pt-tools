// Package feishu 是飞书（Lark）群自定义机器人出站通道。Webhook 地址只能是
// https://open.feishu.cn/open-apis/bot/v2/hook/…（Lark 国际版是 open.larksuite.com）；签名校验时 timestamp 是秒，
// 签名是以 "timestamp\nsecret" 为 key 对空数据做 HmacSHA256 后 Base64，与 timestamp 一起放在请求体里；
// 返回 code 为 0（旧接口是 StatusCode 为 0）才算成功。
package feishu

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/notify/adapter/outbound"
	"github.com/sunerpy/pt-tools/models"
)

// Type 是通道类型名。
const Type = "feishu"

// HookPath 是自定义机器人地址的路径前缀。
const HookPath = "/open-apis/bot/v2/hook/"

// Hosts 是飞书与 Lark 的开放平台域名。
var Hosts = []string{"open.feishu.cn", "open.larksuite.com"}

// Channel 是飞书群机器人通道（只出站）。
type Channel struct {
	webhook string
	hookID  string
	secret  string
	now     func() time.Time
	client  *outbound.Client // 为空时用 outbound.Default()
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
	u, id, err := WebhookURL(cfg.WebhookURL)
	if err != nil {
		return err
	}
	c.webhook, c.hookID, c.secret = u, id, strings.TrimSpace(cfg.Secret)
	if c.now == nil {
		c.now = time.Now
	}
	return nil
}

// CheckConfig 只检查配置、不发请求：保存配置前调用。
func (c *Channel) CheckConfig(conf *models.NotificationConf) error {
	return (&Channel{}).Init(context.Background(), conf)
}

// WebhookURL 校验飞书 Webhook 地址：只能是 https://open.feishu.cn 或 https://open.larksuite.com 下的
// /open-apis/bot/v2/hook/<id>；返回规整后的地址与 id。
func WebhookURL(raw string) (string, string, error) {
	bad := errors.New("飞书 Webhook 地址要是 https://open.feishu.cn/open-apis/bot/v2/hook/… 的形式（Lark 是 open.larksuite.com）")
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", "", bad
	}
	host := strings.ToLower(u.Host)
	if !slices.Contains(Hosts, host) {
		return "", "", bad
	}
	id, ok := strings.CutPrefix(u.Path, HookPath)
	if !ok || id == "" || strings.Contains(id, "/") {
		return "", "", bad
	}
	return "https://" + host + HookPath + id, id, nil
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
	resp, err := outbound.Use(c.client).PostJSON(ctx, outbound.Request{URL: c.webhook, Payload: payload})
	var res struct {
		Code          *int   `json:"code"`
		Msg           string `json:"msg"`
		StatusCode    *int   `json:"StatusCode"`
		StatusMessage string `json:"StatusMessage"`
	}
	parsed := json.Unmarshal(resp.Body, &res) == nil
	code, msg := res.Code, res.Msg
	if code == nil {
		code, msg = res.StatusCode, res.StatusMessage
	}
	switch {
	case err == nil && parsed && code != nil && *code == 0:
		return nil
	case parsed && code != nil && *code != 0:
		return fmt.Errorf("飞书推送失败: code=%d, msg=%s", *code, outbound.Clean(msg, c.hookID, c.secret))
	case err != nil:
		return fmt.Errorf("飞书推送失败: %w", err)
	default:
		return errors.New("飞书推送失败: 响应里没有 code")
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
