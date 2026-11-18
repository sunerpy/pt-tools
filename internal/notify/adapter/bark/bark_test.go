package bark

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/notify/adapter/outbound"
	"github.com/sunerpy/pt-tools/models"
)

func newChannel(t *testing.T, cfg string) *Channel {
	t.Helper()
	c := &Channel{}
	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ConfigJSON: cfg}))
	return c
}

func TestBarkInit(t *testing.T) {
	c := &Channel{}
	assert.Error(t, c.Init(context.Background(), nil))
	assert.ErrorContains(t, c.Init(context.Background(), &models.NotificationConf{ConfigJSON: `{}`}), "device_key")
	assert.Error(t, c.Init(context.Background(), &models.NotificationConf{ConfigJSON: `{"device_key":"k","server_url":"ftp://x"}`}))
	assert.Equal(t, DefaultServer, newChannel(t, `{"device_key":"k"}`).server)
	assert.Equal(t, Type, c.Type())
	assert.False(t, c.SupportsInbound())
	assert.True(t, c.Healthy())
	assert.NoError(t, c.Close(context.Background()))
	c.OnInbound(nil)
	assert.Contains(t, notify.DefaultRegistry().Types(), Type)

	// 本机与内网的服务器要打开 allow_private；保存前的检查与 Init 同一套规则
	inner := &models.NotificationConf{ConfigJSON: `{"device_key":"k","server_url":"http://192.168.1.5:8080"}`}
	assert.ErrorContains(t, c.CheckConfig(inner), "允许内网地址")
	assert.NoError(t, c.CheckConfig(&models.NotificationConf{ConfigJSON: `{"device_key":"k","server_url":"http://192.168.1.5:8080","allow_private":true}`}))
	var _ notify.ConfigChecker = c
}

func TestBarkSend(t *testing.T) {
	var path string
	var got map[string]any
	status, resp := http.StatusOK, `{"code":200,"message":"success"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		got = map[string]any{}
		_ = json.Unmarshal(b, &got)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(resp))
	}))
	defer srv.Close()
	c := newChannel(t, `{"device_key":"dev-key-1","server_url":"`+srv.URL+`/","group":"pt","sound":"bell","allow_private":true}`)
	n := notify.Notification{Title: "标题", Text: "正文", Link: "https://x"}
	require.NoError(t, c.Send(context.Background(), n))
	assert.Equal(t, "/push", path)
	assert.Equal(t, map[string]any{"device_key": "dev-key-1", "title": "标题", "body": "正文", "url": "https://x", "group": "pt", "sound": "bell"}, got)

	// HTTP 4xx + JSON：报 code 与 message，message 里回显的 device_key 换成 ***
	status, resp = http.StatusBadRequest, `{"code":400,"message":"failed to get device token: dev-key-1"}`
	assert.EqualError(t, c.Send(context.Background(), n), "Bark 推送失败: code=400, message=failed to get device token: ***")

	// 没有 code 的响应（地址不是 Bark 服务器）算失败，不当成功
	status, resp = http.StatusOK, `<html>ok</html>`
	assert.ErrorContains(t, c.Send(context.Background(), n), "响应里没有 code")
	status, resp = http.StatusOK, `{"message":"ok"}`
	assert.ErrorContains(t, c.Send(context.Background(), n), "响应里没有 code")

	// HTTP 5xx 即使带着 code=200 也算失败，错误里只有状态码
	status, resp = http.StatusBadGateway, `{"code":200}`
	assert.EqualError(t, c.Send(context.Background(), n), "Bark 推送失败: HTTP 502")
	status, resp = http.StatusBadGateway, `dev-key-1 upstream down`
	err := c.Send(context.Background(), n)
	assert.EqualError(t, err, "Bark 推送失败: HTTP 502")

	// 没打开 allow_private：连接时拦下本机地址，并提示打开开关
	c.allowPrivate = false
	assert.ErrorContains(t, c.Send(context.Background(), n), "允许内网地址")
}

// 默认服务器：请求发到 https://api.day.app/push（测试里改连到本地 TLS 收件端）。
func TestBarkDefaultServer(t *testing.T) {
	var host string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host = r.Host + r.URL.Path
		_, _ = w.Write([]byte(`{"code":200}`))
	}))
	defer srv.Close()
	c := newChannel(t, `{"device_key":"k"}`)
	c.client = outbound.NewClient(outbound.Options{Divert: srv.Listener.Addr().String()})
	require.NoError(t, c.Send(context.Background(), notify.Notification{Title: "t"}))
	assert.Equal(t, "api.day.app/push", host)
}
