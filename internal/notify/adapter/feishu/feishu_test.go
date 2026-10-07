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
	"github.com/sunerpy/pt-tools/models"
)

// 向量按飞书文档的说明独立计算：hmac.new(f"{ts}\n{secret}", b"", sha256) → base64。
func TestSign(t *testing.T) {
	assert.Equal(t, "OrBzY1Y01Gq+HgJsl+7OfWcMVwc7YocohQm5iiZwjhU=", Sign(1700000000, "feishu-secret"))
}

func TestFeishu(t *testing.T) {
	c := &Channel{}
	assert.Error(t, c.Init(context.Background(), nil))
	for _, bad := range []string{`{}`, `{"webhook_url":"ftp://x"}`} {
		assert.Error(t, c.Init(context.Background(), &models.NotificationConf{ConfigJSON: bad}), bad)
	}
	assert.Equal(t, Type, c.Type())
	assert.False(t, c.SupportsInbound())
	assert.True(t, c.Healthy())
	assert.NoError(t, c.Close(context.Background()))
	c.OnInbound(nil)
	assert.Contains(t, notify.DefaultRegistry().Types(), Type)

	var got map[string]any
	resp := `{"code":0,"msg":"success"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = map[string]any{}
		_ = json.Unmarshal(b, &got)
		_, _ = w.Write([]byte(resp))
	}))
	defer srv.Close()
	c = &Channel{now: func() time.Time { return time.Unix(1700000000, 0) }}
	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ConfigJSON: `{"webhook_url":"` + srv.URL + `/open-apis/bot/v2/hook/abc","secret":"feishu-secret"}`}))
	require.NoError(t, c.Send(context.Background(), notify.Notification{Title: "标题", Text: "正文"}))
	assert.Equal(t, map[string]any{
		"msg_type": "text", "content": map[string]any{"text": "标题\n正文"},
		"timestamp": "1700000000", "sign": "OrBzY1Y01Gq+HgJsl+7OfWcMVwc7YocohQm5iiZwjhU=",
	}, got)

	nosign := &Channel{}
	require.NoError(t, nosign.Init(context.Background(), &models.NotificationConf{ConfigJSON: `{"webhook_url":"` + srv.URL + `/hook"}`}))
	require.NoError(t, nosign.Send(context.Background(), notify.Notification{Text: "正文"}))
	assert.NotContains(t, got, "sign")

	resp = `{"code":19021,"msg":"sign match fail or timestamp is not within one hour from current time"}`
	assert.ErrorContains(t, c.Send(context.Background(), notify.Notification{Title: "t"}), "code=19021")
	resp = `{"StatusCode":9499}`
	assert.ErrorContains(t, c.Send(context.Background(), notify.Notification{Title: "t"}), "StatusCode=9499")
}
