package web

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 订阅设置「等满几天改用别的版本」：网页接口默认 0，能存能读，越界的回 400、不改存着的；配置网关（AI 助手与 App API 改配置）能改，
// 改别的设置时它不变。
func TestSubscribeAPI_UnlockAfterDays(t *testing.T) {
	srv, mux := newSubscribeEnv(t)
	w := serveAuthed(mux, http.MethodGet, "/api/media/subscribe/settings", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"unlock_after_days":0`, "默认一直等")

	w = serveAuthed(mux, http.MethodPut, "/api/media/subscribe/settings", `{"enabled":true,"unlock_after_days":7}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"unlock_after_days":7`)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPut, "/api/media/subscribe/settings", `{"enabled":true,"unlock_after_days":91}`).Code)
	w = serveAuthed(mux, http.MethodGet, "/api/media/subscribe/settings", "")
	assert.Contains(t, w.Body.String(), `"unlock_after_days":7`, "越界的不存")

	srv.buildHandler()
	res := configRoundTrip(t, srv, "subscribe", ConfigChange{Op: "set", Values: map[string]any{"search_interval_hours": 24}})
	assert.Equal(t, json.Number("7"), res.Item["unlock_after_days"], "没提到的字段不变")
	res = configRoundTrip(t, srv, "subscribe", ConfigChange{Op: "set", Values: map[string]any{"unlock_after_days": 14}})
	assert.Equal(t, json.Number("14"), res.Item["unlock_after_days"])
}
