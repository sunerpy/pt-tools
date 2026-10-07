//go:build qa

package cmd

import "os"

// qaTMDBBaseURL 只在 qa 构建里生效：把 TMDB 接口指向假服务（PT_TOOLS_QA_TMDB_URL），正式构建里没有这个开关。
func qaTMDBBaseURL() string { return os.Getenv("PT_TOOLS_QA_TMDB_URL") }

// qaTMDBImageURL 只在 qa 构建里生效：把 TMDB 图片地址指向假服务（PT_TOOLS_QA_TMDB_IMAGE_URL）。
func qaTMDBImageURL() string { return os.Getenv("PT_TOOLS_QA_TMDB_IMAGE_URL") }

// qaDoubanURL 只在 qa 构建里生效：把豆瓣想看的 RSS 地址指向假服务（PT_TOOLS_QA_DOUBAN_URL）。
func qaDoubanURL() string { return os.Getenv("PT_TOOLS_QA_DOUBAN_URL") }
