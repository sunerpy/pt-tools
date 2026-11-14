//go:build !qa

package cmd

// qaTMDBBaseURL 在正式构建里总是空串：TMDB 接口地址固定，API Key 不会被发到别的地方。
func qaTMDBBaseURL() string { return "" }

// qaTMDBImageURL 在正式构建里总是空串：图片取自 TMDB 的图片服务器。
func qaTMDBImageURL() string { return "" }

// qaDoubanURL 在正式构建里总是空串：豆瓣想看从豆瓣拉取。
func qaDoubanURL() string { return "" }

// qaBangumiURL 在正式构建里总是空串：番剧日历从 Bangumi 取。
func qaBangumiURL() string { return "" }

// qaTVDBBaseURL 在正式构建里总是空串：TheTVDB 接口地址固定，API Key 与订阅 PIN 不会被发到别的地方。
func qaTVDBBaseURL() string { return "" }

// qaTVDBImageHosts 在正式构建里总是空的：图片只从 TheTVDB 自己的图片服务器下载。
func qaTVDBImageHosts() []string { return nil }
