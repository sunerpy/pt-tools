package ntfy

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

func TestNtfyInit(t *testing.T) {
	c := &Channel{}
	assert.Error(t, c.Init(context.Background(), nil))
	for _, bad := range []string{
		`{}`, `{"topic":"a b"}`, `{"topic":"a.b"}`, `{"topic":"t","priority":6}`, `{"topic":"t","server_url":"ftp://x"}`,
		`{"topic":"t","server_url":"http://localhost:8080"}`, `{"topic":"t","server_url":"http://169.254.169.254","allow_private":true}`,
	} {
		assert.Error(t, c.CheckConfig(&models.NotificationConf{ConfigJSON: bad}), bad)
	}
	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ConfigJSON: `{"topic":"pt-tools_1"}`}))
	assert.Equal(t, DefaultServer, c.server)
	assert.NoError(t, c.CheckConfig(&models.NotificationConf{ConfigJSON: `{"topic":"t","server_url":"http://localhost:8080","allow_private":true}`}))
	assert.Equal(t, Type, c.Type())
	assert.False(t, c.SupportsInbound())
	assert.True(t, c.Healthy())
	assert.NoError(t, c.Close(context.Background()))
	c.OnInbound(nil)
	assert.Contains(t, notify.DefaultRegistry().Types(), Type)
	var _ notify.ConfigChecker = c
}

func TestNtfySend(t *testing.T) {
	var path, auth string
	var got map[string]any
	status, resp := http.StatusOK, `{"id":"x","event":"message"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		got = map[string]any{}
		_ = json.Unmarshal(b, &got)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(resp))
	}))
	defer srv.Close()
	newC := func(cfg string) *Channel {
		c := &Channel{}
		require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ConfigJSON: cfg}))
		return c
	}
	c := newC(`{"server_url":"` + srv.URL + `","topic":"pt","token":"tk_abc","priority":4,"allow_private":true}`)
	require.NoError(t, c.Send(context.Background(), notify.Notification{Title: "标题", Text: "正文", Link: "https://x"}))
	assert.Equal(t, "/", path, "按 JSON 发布到根地址")
	assert.Equal(t, "Bearer tk_abc", auth)
	assert.Equal(t, map[string]any{"topic": "pt", "title": "标题", "message": "正文", "priority": float64(4), "click": "https://x"}, got)

	c = newC(`{"server_url":"` + srv.URL + `","topic":"pt","username":"u","password":"secret-pw","allow_private":true}`)
	require.NoError(t, c.Send(context.Background(), notify.Notification{Title: "只有标题"}))
	assert.Equal(t, "Basic dTpzZWNyZXQtcHc=", auth)
	assert.Equal(t, "只有标题", got["message"], "没有正文时用标题")
	assert.NotContains(t, got, "priority")

	// 失败时报 ntfy 的 code 与 error；回显的鉴权信息换成 ***
	status, resp = http.StatusUnauthorized, `{"code":40101,"http":401,"error":"unauthorized: Basic dTpzZWNyZXQtcHc= secret-pw"}`
	assert.EqualError(t, c.Send(context.Background(), notify.Notification{Title: "t"}), "ntfy 推送失败: HTTP 401, code=40101, error=unauthorized: Basic *** ***")

	// 没有消息 id 的 200（地址不是 ntfy 服务器）算失败；没有 JSON 的错误只报状态码
	status, resp = http.StatusOK, `<html></html>`
	assert.ErrorContains(t, c.Send(context.Background(), notify.Notification{Title: "t"}), "响应里没有消息 id")
	status, resp = http.StatusForbidden, `forbidden`
	assert.EqualError(t, c.Send(context.Background(), notify.Notification{Title: "t"}), "ntfy 推送失败: HTTP 403")

	c.allowPrivate = false
	assert.ErrorContains(t, c.Send(context.Background(), notify.Notification{Title: "t"}), "允许内网地址")
}

// 默认服务器：请求发到 https://ntfy.sh/（测试里改连到本地 TLS 收件端）。
func TestNtfyDefaultServer(t *testing.T) {
	var host string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host = r.Host + r.URL.Path
		_, _ = w.Write([]byte(`{"id":"x"}`))
	}))
	defer srv.Close()
	c := &Channel{client: outbound.NewClient(outbound.Options{Divert: srv.Listener.Addr().String()})}
	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ConfigJSON: `{"topic":"pt"}`}))
	require.NoError(t, c.Send(context.Background(), notify.Notification{Title: "t"}))
	assert.Equal(t, "ntfy.sh/", host)
}
