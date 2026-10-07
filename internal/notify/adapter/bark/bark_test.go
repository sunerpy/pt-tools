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
}

func TestBarkSend(t *testing.T) {
	var path string
	var got map[string]any
	code := 200
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		got = map[string]any{}
		_ = json.Unmarshal(b, &got)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": "m"})
	}))
	defer srv.Close()
	c := newChannel(t, `{"device_key":"dev-1","server_url":"`+srv.URL+`/","group":"pt","sound":"bell"}`)
	require.NoError(t, c.Send(context.Background(), notify.Notification{Title: "标题", Text: "正文", Link: "https://x"}))
	assert.Equal(t, "/push", path)
	assert.Equal(t, map[string]any{"device_key": "dev-1", "title": "标题", "body": "正文", "url": "https://x", "group": "pt", "sound": "bell"}, got)

	code = 400
	assert.ErrorContains(t, c.Send(context.Background(), notify.Notification{Title: "t"}), "code=400")
}
