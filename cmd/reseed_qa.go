//go:build qa

package cmd

import "os"

// qaIYUUBaseURL 只在 qa 构建里生效：把 IYUU 接口指向假服务（PT_TOOLS_QA_IYUU_URL），正式构建里没有这个开关。
func qaIYUUBaseURL() string { return os.Getenv("PT_TOOLS_QA_IYUU_URL") }
