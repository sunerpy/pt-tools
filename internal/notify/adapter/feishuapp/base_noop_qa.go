//go:build !qa

package feishuapp

import "time"

// qaBaseURL 在正式构建里恒为空：只连飞书与 Lark 的官方域名。
func qaBaseURL() string { return "" }

// qaAccountsURL 在正式构建里恒为空：扫码一键创建只连 SDK 默认的 accounts.feishu.cn（Lark 租户由 SDK 切到 accounts.larksuite.com）。
func qaAccountsURL() string { return "" }

// qaSlowAck 在正式构建里恒为 0：应答不额外等待。
func qaSlowAck() time.Duration { return 0 }
