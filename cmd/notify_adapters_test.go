package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

// cmd 的副作用导入把每个通知适配器都注册进默认注册表：漏了导入，配置了的通道启动时会被跳过。
func TestNotifyAdaptersRegistered(t *testing.T) {
	types := notify.DefaultRegistry().Types()
	for _, want := range []string{"telegram", "qq_onebot", "webhook", "wecom_webhook", "bark", "serverchan", "ntfy", "dingtalk", "feishu"} {
		assert.Contains(t, types, want)
	}
}

// 只出站的通道都支持保存前检查配置（app 层保存时调用），规则与 Init 相同。
func TestOutboundAdaptersCheckConfig(t *testing.T) {
	cases := map[string]struct{ good, bad string }{
		"bark":          {`{"device_key":"k"}`, `{"device_key":"k","server_url":"http://192.168.1.2"}`},
		"serverchan":    {`{"send_key":"SCTk"}`, `{"send_key":""}`},
		"ntfy":          {`{"topic":"pt"}`, `{"topic":"a.b"}`},
		"dingtalk":      {`{"webhook_url":"https://oapi.dingtalk.com/robot/send?access_token=t"}`, `{"webhook_url":"https://example.com/robot/send?access_token=t"}`},
		"feishu":        {`{"webhook_url":"https://open.feishu.cn/open-apis/bot/v2/hook/x"}`, `{"webhook_url":"https://example.com/hook/x"}`},
		"wecom_webhook": {`{"webhook_key":"k"}`, `{"webhook_key":"k","msg_type":"card"}`},
	}
	for typ, tc := range cases {
		ch, err := notify.DefaultRegistry().Make(typ)
		require.NoError(t, err, typ)
		checker, ok := ch.(notify.ConfigChecker)
		require.True(t, ok, "%s 要支持保存前检查", typ)
		assert.NoError(t, checker.CheckConfig(&models.NotificationConf{ChannelType: typ, ConfigJSON: tc.good}), typ)
		assert.Error(t, checker.CheckConfig(&models.NotificationConf{ChannelType: typ, ConfigJSON: tc.bad}), typ)
	}
}
