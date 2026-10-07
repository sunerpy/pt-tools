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
	"github.com/sunerpy/pt-tools/models"
)

// 向量按钉钉文档的 Python 示例独立计算：hmac.new(secret, f"{ts}\n{secret}", sha256) → base64。
func TestSign(t *testing.T) {
	assert.Equal(t, "TSZbRFUuvaSQaRKUpF970OPCb2/LcQAP3wOvwZIzBZk=", Sign(1700000000000, "SEC0123456789abcdef"))
}

func TestDingTalkInit(t *testing.T) {
	c := &Channel{}
	assert.Error(t, c.Init(context.Background(), nil))
	for _, bad := range []string{`{}`, `{"webhook_url":"ftp://x"}`, `{"webhook_url":"https://oapi.dingtalk.com/robot/send","msg_type":"card"}`} {
		assert.Error(t, c.Init(context.Background(), &models.NotificationConf{ConfigJSON: bad}), bad)
	}
	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ConfigJSON: `{"webhook_url":"https://oapi.dingtalk.com/robot/send?access_token=t"}`}))
	assert.Equal(t, "markdown", c.msgType)
	assert.Equal(t, Type, c.Type())
	assert.False(t, c.SupportsInbound())
	assert.True(t, c.Healthy())
	assert.NoError(t, c.Close(context.Background()))
	c.OnInbound(nil)
	assert.Contains(t, notify.DefaultRegistry().Types(), Type)
}

func TestDingTalkSend(t *testing.T) {
	var query map[string]string
	var got map[string]any
	errcode := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		query = map[string]string{"access_token": q.Get("access_token"), "timestamp": q.Get("timestamp"), "sign": q.Get("sign")}
		b, _ := io.ReadAll(r.Body)
		got = map[string]any{}
		_ = json.Unmarshal(b, &got)
		_ = json.NewEncoder(w).Encode(map[string]any{"errcode": errcode, "errmsg": "sign not match"})
	}))
	defer srv.Close()
	c := &Channel{now: func() time.Time { return time.UnixMilli(1700000000000) }}
	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ConfigJSON: `{"webhook_url":"` + srv.URL + `/robot/send?access_token=tok","secret":"SEC0123456789abcdef"}`}))
	require.NoError(t, c.Send(context.Background(), notify.Notification{Title: "标题", Text: "正文", Link: "https://x"}))
	assert.Equal(t, map[string]string{"access_token": "tok", "timestamp": "1700000000000", "sign": "TSZbRFUuvaSQaRKUpF970OPCb2/LcQAP3wOvwZIzBZk="}, query, "加签拼在地址后面")
	assert.Equal(t, "markdown", got["msgtype"])
	md := got["markdown"].(map[string]any)
	assert.Equal(t, "标题", md["title"])
	assert.Equal(t, "### 标题\n\n正文\n\n[查看](https://x)", md["text"])

	text := &Channel{}
	require.NoError(t, text.Init(context.Background(), &models.NotificationConf{ConfigJSON: `{"webhook_url":"` + srv.URL + `/robot/send?access_token=tok","msg_type":"text"}`}))
	require.NoError(t, text.Send(context.Background(), notify.Notification{Title: "标题", Text: "正文"}))
	assert.Equal(t, map[string]any{"msgtype": "text", "text": map[string]any{"content": "标题\n正文"}}, got)
	assert.Empty(t, query["sign"], "没配 secret 不加签")

	errcode = 310000
	err := c.Send(context.Background(), notify.Notification{Title: "t"})
	assert.ErrorContains(t, err, "errcode=310000")
	assert.NotContains(t, err.Error(), "tok")
}
