package wecom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/notify/adapter/outbound"
)

// webhookEndpoint 是企业微信群机器人的发送地址（key 拼在查询参数里）。
const webhookEndpoint = "https://qyapi.weixin.qq.com/cgi-bin/webhook/send"

func (w *WeComChannel) sendNotification(ctx context.Context, n notify.Notification) error {
	var payload any
	switch w.msgType {
	case "text":
		payload = w.buildTextPayload(n)
	default:
		payload = w.buildMarkdownPayload(n)
	}
	resp, err := outbound.Use(w.client).PostJSON(ctx, outbound.Request{
		URL: webhookEndpoint + "?key=" + url.QueryEscape(w.webhookKey), Payload: payload,
	})
	// key 无效、频率超限、内容违规等错误都以 HTTP 200 返回，靠 errcode 区分；errcode 为 0 才算成功
	var res struct {
		ErrCode *int   `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	parsed := json.Unmarshal(resp.Body, &res) == nil && res.ErrCode != nil
	switch {
	case err == nil && parsed && *res.ErrCode == 0:
		return nil
	case parsed && *res.ErrCode != 0:
		return fmt.Errorf("wecom webhook 返回错误: errcode=%d, errmsg=%s", *res.ErrCode, outbound.Clean(res.ErrMsg, w.webhookKey))
	case err != nil:
		return fmt.Errorf("wecom webhook 请求失败: %w", err)
	default:
		return errors.New("wecom webhook 返回无法识别的响应：没有 errcode")
	}
}

func (w *WeComChannel) buildMarkdownPayload(n notify.Notification) map[string]interface{} {
	content := fmt.Sprintf("# %s\n\n%s", n.Title, n.Text)
	return map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"content": content,
		},
	}
}

func (w *WeComChannel) buildTextPayload(n notify.Notification) map[string]interface{} {
	content := fmt.Sprintf("%s\n%s", n.Title, n.Text)
	return map[string]interface{}{
		"msgtype": "text",
		"text": map[string]string{
			"content": content,
		},
	}
}
