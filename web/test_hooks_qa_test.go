//go:build qa

package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify"
)

type qaRecorder struct{ msgs []notify.InboundMessage }

func (r *qaRecorder) Process(_ context.Context, msg notify.InboundMessage) error {
	r.msgs = append(r.msgs, msg)
	return nil
}

// /test/feishu/inject 按飞书的形状造消息：发送人是字符串的 open_id，可以带 message_type。
func TestQAFeishuInject(t *testing.T) {
	rec := &qaRecorder{}
	SetQAInboundProcessor(rec)
	t.Cleanup(func() { SetQAInboundProcessor(nil) })
	mux := http.NewServeMux()
	RegisterQATestHooks(mux)

	body := `{"text":"/status","conf_id":3,"chat_id":"oc_group","message_type":"group","from":{"user_id":"ou_admin"}}`
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/test/feishu/inject", strings.NewReader(body)))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Len(t, rec.msgs, 1)
	assert.Equal(t, notify.InboundMessage{
		ChannelType: "feishu_app", SourceConfID: 3, ChannelUserID: "ou_admin",
		ChatID: "oc_group", MessageType: "group", Text: "/status",
	}, rec.msgs[0])

	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/test/telegram/inject", strings.NewReader(`{"text":"/help","from":{"id":42}}`)))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "42", rec.msgs[1].ChannelUserID, "Telegram 照旧用数字 ID")
}
