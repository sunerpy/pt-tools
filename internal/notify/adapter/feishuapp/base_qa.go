//go:build qa

package feishuapp

import "os"

// qaBaseURL 只在 qa 构建里生效：把开放平台的地址改连到本地的假飞书（PT_TOOLS_QA_FEISHU_BASE_URL，
// 例如 http://127.0.0.1:18431），长连接的地址由假服务的 /callback/ws/endpoint 给出。正式构建里没有这个开关。
func qaBaseURL() string { return os.Getenv("PT_TOOLS_QA_FEISHU_BASE_URL") }
