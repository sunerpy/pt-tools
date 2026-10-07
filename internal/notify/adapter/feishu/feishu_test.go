package feishu

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

// 向量按飞书文档的说明独立计算：hmac.new(f"{ts}\n{secret}", b"", sha256) → base64。
func TestSign(t *testing.T) {
	assert.Equal(t, "OrBzY1Y01Gq+HgJsl+7OfWcMVwc7YocohQm5iiZwjhU=", Sign(1700000000, "feishu-secret"))
}

func TestFeishuInit(t *testing.T) {
	c := &Channel{}
	assert.Error(t, c.Init(context.Background(), nil))
	for _, bad := range []string{
		`{}`, `{"webhook_url":"ftp://x"}`,
		`{"webhook_url":"http://open.feishu.cn/open-apis/bot/v2/hook/abc"}`, // 只能 https
		`{"webhook_url":"https://example.com/open-apis/bot/v2/hook/abc"}`,
		`{"webhook_url":"https://open.feishu.cn.evil.com/open-apis/bot/v2/hook/abc"}`,
		`{"webhook_url":"https://open.feishu.cn/open-apis/bot/v2/hook/"}`, // 没有 id
		`{"webhook_url":"https://open.feishu.cn/open-apis/bot/v2/hook/abc/def"}`,
		`{"webhook_url":"https://open.feishu.cn/open-apis/bot/v2/hook/abc?x=1"}`,
		`{"webhook_url":"https://oapi.dingtalk.com/robot/send?access_token=t"}`, // 钉钉的地址
		`{"webhook_url":"http://127.0.0.1:8080/open-apis/bot/v2/hook/abc"}`,
	} {
		assert.Error(t, c.CheckConfig(&models.NotificationConf{ConfigJSON: bad}), bad)
	}
	for in, want := range map[string]string{
		"https://open.feishu.cn/open-apis/bot/v2/hook/abc":      "https://open.feishu.cn/open-apis/bot/v2/hook/abc",
		" https://OPEN.larksuite.com/open-apis/bot/v2/hook/x1 ": "https://open.larksuite.com/open-apis/bot/v2/hook/x1",
	} {
		u, _, err := WebhookURL(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, u)
	}
	assert.Equal(t, Type, c.Type())
	assert.False(t, c.SupportsInbound())
	assert.True(t, c.Healthy())
	assert.NoError(t, c.Close(context.Background()))
	c.OnInbound(nil)
	assert.Contains(t, notify.DefaultRegistry().Types(), Type)
	var _ notify.ConfigChecker = c
}

func TestFeishuSend(t *testing.T) {
	var got map[string]any
	var host string
	status, resp := http.StatusOK, `{"code":0,"msg":"success"}`
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host = r.Host + r.URL.Path
		b, _ := io.ReadAll(r.Body)
		got = map[string]any{}
		_ = json.Unmarshal(b, &got)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(resp))
	}))
	defer srv.Close()
	// 请求发到官方地址（测试里改连到本地 TLS 收件端）
	client := outbound.NewClient(outbound.Options{Divert: srv.Listener.Addr().String()})
	c := &Channel{now: func() time.Time { return time.Unix(1700000000, 0) }, client: client}
	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ConfigJSON: `{"webhook_url":"https://open.feishu.cn/open-apis/bot/v2/hook/hook-abc","secret":"feishu-secret"}`}))
	require.NoError(t, c.Send(context.Background(), notify.Notification{Title: "标题", Text: "正文"}))
	assert.Equal(t, "open.feishu.cn/open-apis/bot/v2/hook/hook-abc", host)
	assert.Equal(t, map[string]any{
		"msg_type": "text", "content": map[string]any{"text": "标题\n正文"},
		"timestamp": "1700000000", "sign": "OrBzY1Y01Gq+HgJsl+7OfWcMVwc7YocohQm5iiZwjhU=",
	}, got)

	nosign := &Channel{client: client}
	require.NoError(t, nosign.Init(context.Background(), &models.NotificationConf{ConfigJSON: `{"webhook_url":"https://open.larksuite.com/open-apis/bot/v2/hook/hook-abc"}`}))
	require.NoError(t, nosign.Send(context.Background(), notify.Notification{Text: "正文"}))
	assert.Equal(t, "open.larksuite.com/open-apis/bot/v2/hook/hook-abc", host)
	assert.NotContains(t, got, "sign")

	// code 非 0：报 code 与 msg，回显的 hook id 与 secret 换成 ***
	resp = `{"code":19021,"msg":"sign match fail for hook-abc with feishu-secret"}`
	assert.EqualError(t, c.Send(context.Background(), notify.Notification{Title: "t"}), "飞书推送失败: code=19021, msg=sign match fail for *** with ***")
	// 旧接口：StatusCode 为 0 才算成功
	resp = `{"Extra":null,"StatusCode":0,"StatusMessage":"success"}`
	require.NoError(t, c.Send(context.Background(), notify.Notification{Title: "t"}))
	resp = `{"StatusCode":9499,"StatusMessage":"Bad Request"}`
	assert.EqualError(t, c.Send(context.Background(), notify.Notification{Title: "t"}), "飞书推送失败: code=9499, msg=Bad Request")

	// 既没有 code 也没有 StatusCode（比如钉钉的响应）算失败；HTTP 错误没有 JSON 时只报状态码
	resp = `{"errcode":310000}`
	assert.ErrorContains(t, c.Send(context.Background(), notify.Notification{Title: "t"}), "响应里没有 code")
	status, resp = http.StatusBadRequest, `hook-abc`
	assert.EqualError(t, c.Send(context.Background(), notify.Notification{Title: "t"}), "飞书推送失败: HTTP 400")
}
