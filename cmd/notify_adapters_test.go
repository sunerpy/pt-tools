package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sunerpy/pt-tools/internal/notify"
)

// cmd 的副作用导入把每个通知适配器都注册进默认注册表：漏了导入，配置了的通道启动时会被跳过。
func TestNotifyAdaptersRegistered(t *testing.T) {
	types := notify.DefaultRegistry().Types()
	for _, want := range []string{"telegram", "qq_onebot", "webhook", "wecom_webhook", "bark", "serverchan", "ntfy", "dingtalk", "feishu"} {
		assert.Contains(t, types, want)
	}
}
