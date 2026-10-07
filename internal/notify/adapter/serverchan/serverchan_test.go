package serverchan

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/notify/adapter/outbound"
	"github.com/sunerpy/pt-tools/models"
)

func TestEndpoint(t *testing.T) {
	ep, err := Endpoint(" SCT123abc ")
	require.NoError(t, err)
	assert.Equal(t, "https://sctapi.ftqq.com/SCT123abc.send", ep, "Turbo 版")
	ep, err = Endpoint("sctp12345tABCdef")
	require.NoError(t, err)
	assert.Equal(t, "https://12345.push.ft07.com/send/sctp12345tABCdef.send", ep, "Server 酱³：uid 来自 sctp<uid>t")
	for _, bad := range []string{"", "a/b", "a?b", "a b"} {
		_, err := Endpoint(bad)
		assert.Error(t, err, bad)
	}
}

func TestServerChan(t *testing.T) {
	c := &Channel{}
	assert.Error(t, c.Init(context.Background(), nil))
	assert.ErrorContains(t, c.Init(context.Background(), &models.NotificationConf{ConfigJSON: `{}`}), "send_key")
	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ConfigJSON: `{"send_key":"SCTkey"}`}))
	assert.Equal(t, Type, c.Type())
	assert.False(t, c.SupportsInbound())
	assert.True(t, c.Healthy())
	assert.NoError(t, c.Close(context.Background()))
	c.OnInbound(nil)
	assert.Contains(t, notify.DefaultRegistry().Types(), Type)

	var _ notify.ConfigChecker = c
	assert.ErrorContains(t, c.CheckConfig(&models.NotificationConf{ConfigJSON: `{"send_key":"a/b"}`}), "格式不对")

	// 请求发到官方地址（测试里改连到本地 TLS 收件端）
	var got map[string]string
	var host string
	status, resp := http.StatusOK, `{"code":0,"message":""}`
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host = r.Host + r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(resp))
	}))
	defer srv.Close()
	c.client = outbound.NewClient(outbound.Options{Divert: srv.Listener.Addr().String()})
	long := strings.Repeat("长", 40)
	require.NoError(t, c.Send(context.Background(), notify.Notification{Title: long, Text: "正文", Link: "https://x"}))
	assert.Equal(t, "sctapi.ftqq.com/SCTkey.send", host)
	assert.Equal(t, 32, len([]rune(got["title"])), "标题最多 32 个字")
	assert.True(t, strings.HasPrefix(got["desp"], long), "完整标题放进正文")
	assert.Contains(t, got["desp"], "https://x")

	resp = `{"code":40001,"message":"bad key"}`
	assert.ErrorContains(t, c.Send(context.Background(), notify.Notification{Title: "t"}), "code=40001")

	// 真实服务对错误的 Key 回 HTTP 400 + JSON：报 code 与 message（回显的 SendKey 换成 ***），不报整段响应
	status, resp = http.StatusBadRequest, `{"message":"[AUTH]\u9519\u8bef\u7684Key SCTkey","code":40001,"info":"SCTkey"}`
	err := c.Send(context.Background(), notify.Notification{Title: "t"})
	assert.EqualError(t, err, "Server 酱推送失败: code=40001, message=[AUTH]错误的Key ***")

	// 没有 code、或 HTTP 错误没有 JSON：算失败，错误里只有状态码
	status, resp = http.StatusOK, `{}`
	assert.ErrorContains(t, c.Send(context.Background(), notify.Notification{Title: "t"}), "响应里没有 code")
	status, resp = http.StatusInternalServerError, `SCTkey`
	assert.EqualError(t, c.Send(context.Background(), notify.Notification{Title: "t"}), "Server 酱推送失败: HTTP 500")
}
