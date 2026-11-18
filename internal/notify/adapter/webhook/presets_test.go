package webhook

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 编辑页的每个预设（前端常量，webhookPresets.json）都要能在所有事件的示例数据上通过保存前的校验
func TestWebhook_PresetsPassCheck(t *testing.T) {
	raw, err := os.ReadFile("../../../../web/frontend/src/utils/webhookPresets.json")
	require.NoError(t, err)
	var presets []struct {
		Key          string `json:"key"`
		URLHint      string `json:"url_hint"`
		Method       string `json:"method"`
		ContentType  string `json:"content_type"`
		BodyTemplate string `json:"body_template"`
		Success      string `json:"success_template"`
	}
	require.NoError(t, json.Unmarshal(raw, &presets))
	require.NotEmpty(t, presets)
	for _, p := range presets {
		cfg := map[string]any{
			"endpoint_url": "https://example.com/hook", "method": p.Method,
			"content_type": p.ContentType, "body_template": p.BodyTemplate,
			"success_template": p.Success,
		}
		assert.NoError(t, checkConfig(cfg), p.Key)
		// 地址提示是给人看的样子（带 <占位>），只要求是 http(s) 地址
		if p.URLHint != "" {
			assert.Regexp(t, `^https?://`, p.URLHint, p.Key)
		}
	}
}

// 预设里的成功条件按这几家服务真实的响应判断
func TestWebhook_PresetSuccessTemplates(t *testing.T) {
	raw, err := os.ReadFile("../../../../web/frontend/src/utils/webhookPresets.json")
	require.NoError(t, err)
	var presets []struct {
		Key     string `json:"key"`
		Success string `json:"success_template"`
	}
	require.NoError(t, json.Unmarshal(raw, &presets))
	cases := map[string]struct{ ok, bad string }{
		"pushplus": {`{"code":200,"msg":"请求成功","data":"x"}`, `{"code":903,"msg":"无效的用户token"}`},
		"gotify":   {`{"id":25,"appid":1,"message":"m"}`, `{"error":"Unauthorized"}`},
		"slack":    {`ok`, `no_service`},
	}
	seen := 0
	for _, p := range presets {
		c, ok := cases[p.Key]
		if !ok {
			assert.Empty(t, p.Success, p.Key)
			continue
		}
		seen++
		r, err := compile(&Config{EndpointURL: "https://example.com/hook", SuccessTemplate: p.Success})
		require.NoError(t, err, p.Key)
		assert.NoError(t, r.checkResponse(200, []byte(c.ok)), p.Key)
		assert.ErrorIs(t, r.checkResponse(200, []byte(c.bad)), errResponseRejected, p.Key)
	}
	assert.Equal(t, len(cases), seen)
}
