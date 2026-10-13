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
	}
	require.NoError(t, json.Unmarshal(raw, &presets))
	require.NotEmpty(t, presets)
	for _, p := range presets {
		cfg := map[string]any{
			"endpoint_url": "https://example.com/hook", "method": p.Method,
			"content_type": p.ContentType, "body_template": p.BodyTemplate,
		}
		assert.NoError(t, checkConfig(cfg), p.Key)
		// 地址提示是给人看的样子（带 <占位>），只要求是 http(s) 地址
		if p.URLHint != "" {
			assert.Regexp(t, `^https?://`, p.URLHint, p.Key)
		}
	}
}
