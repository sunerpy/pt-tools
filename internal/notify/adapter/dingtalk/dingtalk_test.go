package dingtalk

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/notify/adapter/outbound"
	"github.com/sunerpy/pt-tools/models"
)

// 向量按钉钉文档的 Python 示例独立计算：hmac.new(secret, f"{ts}\n{secret}", sha256) → base64。
func TestSign(t *testing.T) {
	assert.Equal(t, "TSZbRFUuvaSQaRKUpF970OPCb2/LcQAP3wOvwZIzBZk=", Sign(1700000000000, "SEC0123456789abcdef"))
}

func TestDingTalkInit(t *testing.T) {
	c := &Channel{}
	assert.Error(t, c.Init(context.Background(), nil))
	for _, bad := range []string{
		`{}`, `{"webhook_url":"ftp://x"}`,
		`{"webhook_url":"https://oapi.dingtalk.com/robot/send?access_token=t","msg_type":"card"}`,
		`{"webhook_url":"http://oapi.dingtalk.com/robot/send?access_token=t"}`, // 只能 https
		`{"webhook_url":"https://example.com/robot/send?access_token=t"}`,      // 只能是官方域名
		`{"webhook_url":"https://oapi.dingtalk.com.evil.com/robot/send?access_token=t"}`,
		`{"webhook_url":"https://oapi.dingtalk.com:8443/robot/send?access_token=t"}`,
		`{"webhook_url":"https://oapi.dingtalk.com/robot/other?access_token=t"}`,
		`{"webhook_url":"https://oapi.dingtalk.com/robot/send"}`, // 没有 access_token
		`{"webhook_url":"https://u:p@oapi.dingtalk.com/robot/send?access_token=t"}`,
		`{"webhook_url":"http://127.0.0.1:8080/robot/send?access_token=t"}`,
	} {
		assert.Error(t, c.CheckConfig(&models.NotificationConf{ConfigJSON: bad}), bad)
	}
	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ConfigJSON: `{"webhook_url":"https://OAPI.dingtalk.com/robot/send?access_token=t"}`}))
	assert.Equal(t, "https://oapi.dingtalk.com/robot/send?access_token=t", c.webhook)
	assert.Equal(t, "markdown", c.msgType)
	assert.Equal(t, Type, c.Type())
	assert.False(t, c.SupportsInbound())
	assert.True(t, c.Healthy())
	assert.NoError(t, c.Close(context.Background()))
	c.OnInbound(nil)
	assert.Contains(t, notify.DefaultRegistry().Types(), Type)
	var _ notify.ConfigChecker = c
}

func TestDingTalkSend(t *testing.T) {
	var query map[string]string
	var got map[string]any
	var host string
	status, resp := http.StatusOK, `{"errcode":0,"errmsg":"ok"}`
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host = r.Host + r.URL.Path
		q := r.URL.Query()
		query = map[string]string{"access_token": q.Get("access_token"), "timestamp": q.Get("timestamp"), "sign": q.Get("sign")}
		b, _ := io.ReadAll(r.Body)
		got = map[string]any{}
		_ = json.Unmarshal(b, &got)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(resp))
	}))
	defer srv.Close()
	// 请求发到官方地址（测试里改连到本地 TLS 收件端）
	client := outbound.NewClient(outbound.Options{Divert: srv.Listener.Addr().String()})
	c := &Channel{now: func() time.Time { return time.UnixMilli(1700000000000) }, client: client}
	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ConfigJSON: `{"webhook_url":"https://oapi.dingtalk.com/robot/send?access_token=tok-123","secret":"SEC0123456789abcdef"}`}))
	require.NoError(t, c.Send(context.Background(), notify.Notification{Title: "标题", Text: "正文", Link: "https://x"}))
	assert.Equal(t, "oapi.dingtalk.com/robot/send", host)
	assert.Equal(t, map[string]string{"access_token": "tok-123", "timestamp": "1700000000000", "sign": "TSZbRFUuvaSQaRKUpF970OPCb2/LcQAP3wOvwZIzBZk="}, query, "加签拼在地址后面")
	assert.Equal(t, "markdown", got["msgtype"])
	md := got["markdown"].(map[string]any)
	assert.Equal(t, "标题", md["title"])
	assert.Equal(t, "### 标题\n\n正文\n\n[查看](https://x)", md["text"])

	text := &Channel{client: client}
	require.NoError(t, text.Init(context.Background(), &models.NotificationConf{ConfigJSON: `{"webhook_url":"https://oapi.dingtalk.com/robot/send?access_token=tok-123","msg_type":"text"}`}))
	require.NoError(t, text.Send(context.Background(), notify.Notification{Title: "标题", Text: "正文"}))
	assert.Equal(t, map[string]any{"msgtype": "text", "text": map[string]any{"content": "标题\n正文"}}, got)
	assert.Empty(t, query["sign"], "没配 secret 不加签")

	// errcode 非 0：报 errcode 与 errmsg，回显的 access_token 与 secret 换成 ***
	resp = `{"errcode":310000,"errmsg":"sign not match, token tok-123 secret SEC0123456789abcdef"}`
	err := c.Send(context.Background(), notify.Notification{Title: "t"})
	assert.EqualError(t, err, "钉钉推送失败: errcode=310000, errmsg=sign not match, token *** secret ***")

	// 没有 errcode（比如飞书的响应）算失败；HTTP 错误没有 JSON 时只报状态码
	resp = `{"code":0,"msg":"success"}`
	assert.ErrorContains(t, c.Send(context.Background(), notify.Notification{Title: "t"}), "响应里没有 errcode")
	status, resp = http.StatusForbidden, `token tok-123`
	assert.EqualError(t, c.Send(context.Background(), notify.Notification{Title: "t"}), "钉钉推送失败: HTTP 403")
}
