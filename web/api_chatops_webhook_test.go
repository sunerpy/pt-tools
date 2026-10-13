package web

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify/adapter/webhook"
)

func TestChatOpsWebhookPreviewAPI(t *testing.T) {
	srv, _, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")
	const path = "/api/chatops/webhook/preview"

	// 和其他 ChatOps 管理接口一样要登录
	resp := chatopsReq(t, srv, http.MethodPost, path, "", map[string]any{"type": "test"})
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	var out webhook.PreviewResult
	resp = chatopsReq(t, srv, http.MethodPost, path, tok, map[string]any{
		"type": "rss.matched", "endpoint_url": "https://example.com/hook?s={{urlquery .site}}",
		"content_type": "text", "body_template": "{{.site}}：{{.size}}", "headers": map[string]string{"X-A": "1"},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	decodeBody(t, resp, &out)
	assert.Equal(t, webhook.PreviewResult{
		Method: http.MethodPost, URL: "https://example.com/hook?s=hdtime",
		ContentType: "text/plain; charset=utf-8", Body: "hdtime：15.00 GB",
	}, out)

	var e errResp
	resp = chatopsReq(t, srv, http.MethodPost, path, tok, map[string]any{
		"type": "rss.matched", "endpoint_url": "https://example.com/hook", "body_template": `{"a": {{.title}}}`,
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	decodeBody(t, resp, &e)
	assert.Equal(t, "invalid_template", e.Error)
	assert.Contains(t, e.Detail, "不是合法的 JSON")

	resp = chatopsReq(t, srv, http.MethodPost, path, tok, map[string]any{"type": "nope", "endpoint_url": "https://example.com/hook"})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
	resp = chatopsReq(t, srv, http.MethodPost, path, tok, map[string]any{"type": "test", "endpoint_url": "https://example.com/hook", "headers": 1})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "请求头不是对象")
	resp.Body.Close()
}
