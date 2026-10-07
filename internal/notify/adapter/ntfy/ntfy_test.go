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
	"github.com/sunerpy/pt-tools/models"
)

func TestNtfyInit(t *testing.T) {
	c := &Channel{}
	assert.Error(t, c.Init(context.Background(), nil))
	for _, bad := range []string{`{}`, `{"topic":"a b"}`, `{"topic":"t","priority":6}`, `{"topic":"t","server_url":"ftp://x"}`} {
		assert.Error(t, c.Init(context.Background(), &models.NotificationConf{ConfigJSON: bad}), bad)
	}
	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ConfigJSON: `{"topic":"pt-tools_1"}`}))
	assert.Equal(t, DefaultServer, c.server)
	assert.Equal(t, Type, c.Type())
	assert.False(t, c.SupportsInbound())
	assert.True(t, c.Healthy())
	assert.NoError(t, c.Close(context.Background()))
	c.OnInbound(nil)
	assert.Contains(t, notify.DefaultRegistry().Types(), Type)
}

func TestNtfySend(t *testing.T) {
	var path, auth string
	var got map[string]any
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		got = map[string]any{}
		_ = json.Unmarshal(b, &got)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"id":"x"}`))
	}))
	defer srv.Close()
	newC := func(cfg string) *Channel {
		c := &Channel{}
		require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ConfigJSON: cfg}))
		return c
	}
	c := newC(`{"server_url":"` + srv.URL + `","topic":"pt","token":"tk_abc","priority":4}`)
	require.NoError(t, c.Send(context.Background(), notify.Notification{Title: "标题", Text: "正文", Link: "https://x"}))
	assert.Equal(t, "/", path, "按 JSON 发布到根地址")
	assert.Equal(t, "Bearer tk_abc", auth)
	assert.Equal(t, map[string]any{"topic": "pt", "title": "标题", "message": "正文", "priority": float64(4), "click": "https://x"}, got)

	c = newC(`{"server_url":"` + srv.URL + `","topic":"pt","username":"u","password":"p"}`)
	require.NoError(t, c.Send(context.Background(), notify.Notification{Title: "只有标题"}))
	assert.Equal(t, "Basic dTpw", auth)
	assert.Equal(t, "只有标题", got["message"], "没有正文时用标题")
	assert.NotContains(t, got, "priority")

	status = http.StatusForbidden
	assert.ErrorContains(t, c.Send(context.Background(), notify.Notification{Title: "t"}), "HTTP 403")
}
