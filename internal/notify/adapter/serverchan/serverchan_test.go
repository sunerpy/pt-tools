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

	var got map[string]string
	code := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": "bad key"})
	}))
	defer srv.Close()
	c.endpoint = srv.URL + "/SCTkey.send"
	long := strings.Repeat("长", 40)
	require.NoError(t, c.Send(context.Background(), notify.Notification{Title: long, Text: "正文", Link: "https://x"}))
	assert.Equal(t, 32, len([]rune(got["title"])), "标题最多 32 个字")
	assert.True(t, strings.HasPrefix(got["desp"], long), "完整标题放进正文")
	assert.Contains(t, got["desp"], "https://x")

	code = 40001
	assert.ErrorContains(t, c.Send(context.Background(), notify.Notification{Title: "t"}), "code=40001")

	// 真实服务对错误的 Key 回 HTTP 400 + JSON：报 code 与 message，不报整段响应
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"[AUTH]\u9519\u8bef\u7684Key","code":40001}`))
	}))
	defer bad.Close()
	c.endpoint = bad.URL + "/SCTkey.send"
	err := c.Send(context.Background(), notify.Notification{Title: "t"})
	assert.EqualError(t, err, "Server 酱推送失败: code=40001, message=[AUTH]错误的Key")
}
