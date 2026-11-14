//go:build qa

package cmd

import (
	"os"
	"strings"
)

// qaTMDBBaseURL 只在 qa 构建里生效：把 TMDB 接口指向假服务（PT_TOOLS_QA_TMDB_URL），正式构建里没有这个开关。
func qaTMDBBaseURL() string { return os.Getenv("PT_TOOLS_QA_TMDB_URL") }

// qaTMDBImageURL 只在 qa 构建里生效：把 TMDB 图片地址指向假服务（PT_TOOLS_QA_TMDB_IMAGE_URL）。
func qaTMDBImageURL() string { return os.Getenv("PT_TOOLS_QA_TMDB_IMAGE_URL") }

// qaDoubanURL 只在 qa 构建里生效：把豆瓣想看的 RSS 地址指向假服务（PT_TOOLS_QA_DOUBAN_URL）。
func qaDoubanURL() string { return os.Getenv("PT_TOOLS_QA_DOUBAN_URL") }

// qaBangumiURL 只在 qa 构建里生效：把番剧日历的 Bangumi 接口指向假服务（PT_TOOLS_QA_BANGUMI_URL）。
func qaBangumiURL() string { return os.Getenv("PT_TOOLS_QA_BANGUMI_URL") }

// qaTVDBBaseURL 只在 qa 构建里生效：把 TheTVDB 接口指向假服务（PT_TOOLS_QA_TVDB_URL，到 /v4 为止）。
func qaTVDBBaseURL() string { return os.Getenv("PT_TOOLS_QA_TVDB_URL") }

// qaTVDBImageHosts 只在 qa 构建里生效：允许从假服务下载 TheTVDB 图片（PT_TOOLS_QA_TVDB_IMAGE_HOSTS，逗号分隔的主机）。
func qaTVDBImageHosts() []string {
	var out []string
	for _, h := range strings.Split(os.Getenv("PT_TOOLS_QA_TVDB_IMAGE_HOSTS"), ",") {
		if h = strings.TrimSpace(h); h != "" {
			out = append(out, h)
		}
	}
	return out
}
