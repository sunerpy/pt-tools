package outbound

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

func TestPostJSON(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Content-Type")
		if r.URL.Path == "/bad" {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(strings.Repeat("错", 300)))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	body, err := PostJSON(context.Background(), srv.URL+"/ok", map[string]string{"a": "b"})
	require.NoError(t, err)
	assert.Equal(t, `{"ok":true}`, string(body))
	assert.Equal(t, "application/json", got)

	_, err = PostJSON(context.Background(), srv.URL+"/bad", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 502")
	assert.True(t, strings.HasSuffix(err.Error(), "..."), "响应体截短")

	srv.Close()
	_, err = PostJSON(context.Background(), srv.URL+"/hook?access_token=SECRET-TOKEN", nil)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "SECRET-TOKEN", "错误里不带地址")
}

func TestHelpers(t *testing.T) {
	assert.Equal(t, "标题\n正文\nhttps://x", Text(notify.Notification{Title: " 标题 ", Text: "正文", Link: "https://x"}))
	assert.Equal(t, "正文", Text(notify.Notification{Text: "正文"}))
	u, err := HTTPURL(" https://ntfy.example/ ", "地址")
	require.NoError(t, err)
	assert.Equal(t, "https://ntfy.example", u)
	for _, bad := range []string{"ftp://x", "https://u:p@x", "https://x/#a", "x"} {
		_, err := HTTPURL(bad, "地址")
		assert.Error(t, err, bad)
	}
	var dst struct{ A string }
	require.NoError(t, Config("", &dst, "t"))
	assert.Error(t, Config("{", &dst, "t"))
	assert.Equal(t, "abc", Truncate([]byte(" abc ")))
}
