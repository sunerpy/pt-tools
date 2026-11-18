//go:build qa

package feishuapp

import (
	"os"
	"sync/atomic"
	"time"
)

// qaBaseURL 只在 qa 构建里生效：把开放平台的地址改连到本地的假飞书（PT_TOOLS_QA_FEISHU_BASE_URL，
// 例如 http://127.0.0.1:18431），长连接的地址由假服务的 /callback/ws/endpoint 给出。正式构建里没有这个开关。
func qaBaseURL() string { return os.Getenv("PT_TOOLS_QA_FEISHU_BASE_URL") }

var qaSlowUsed atomic.Bool

// qaSlowAck 只在 qa 构建里生效：PT_TOOLS_QA_FEISHU_SLOW_ONCE=1 时，第一条消息的应答晚 4 秒，
// 让假服务按同一个 message_id 重推一次，验证去重。
func qaSlowAck() time.Duration {
	if os.Getenv("PT_TOOLS_QA_FEISHU_SLOW_ONCE") != "1" || !qaSlowUsed.CompareAndSwap(false, true) {
		return 0
	}
	return 4 * time.Second
}
