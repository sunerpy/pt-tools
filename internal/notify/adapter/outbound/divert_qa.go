//go:build qa

package outbound

import "os"

// qaDivert 只在 qa 构建里生效：把发往各家官方地址的请求改连到本地收件端（PT_TOOLS_QA_NOTIFY_DIVERT，host:port），
// 正式构建里没有这个开关。
func qaDivert() string { return os.Getenv("PT_TOOLS_QA_NOTIFY_DIVERT") }
