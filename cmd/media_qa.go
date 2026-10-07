//go:build qa

package cmd

import "os"

// qaTMDBBaseURL 只在 qa 构建里生效：把 TMDB 接口指向假服务（PT_TOOLS_QA_TMDB_URL），正式构建里没有这个开关。
func qaTMDBBaseURL() string { return os.Getenv("PT_TOOLS_QA_TMDB_URL") }
