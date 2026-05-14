package wecom

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/utils/httpclient"
)

func (w *WeComChannel) sendNotification(ctx context.Context, n notify.Notification) error {
	endpoint := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=%s", w.webhookKey)

	var payload interface{}
	switch w.msgType {
	case "markdown":
		payload = w.buildMarkdownPayload(n)
	case "text":
		payload = w.buildTextPayload(n)
	default:
		payload = w.buildMarkdownPayload(n)
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("wecom 序列化 payload 失败: %w", err)
	}

	resp, err := httpclient.Post(endpoint, bodyBytes,
		httpclient.WithContext(ctx),
		httpclient.WithContentType("application/json"),
	)
	if err != nil {
		return fmt.Errorf("wecom webhook 请求失败: %w", err)
	}

	if resp.StatusCode() >= 400 && resp.StatusCode() < 500 {
		return fmt.Errorf("wecom webhook 返回 4xx: %d, body: %s", resp.StatusCode(), string(resp.Bytes()))
	}

	if resp.StatusCode() >= 500 {
		return fmt.Errorf("wecom webhook 返回 5xx: %d, body: %s", resp.StatusCode(), string(resp.Bytes()))
	}

	return nil
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
