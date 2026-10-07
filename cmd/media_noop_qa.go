//go:build !qa

package cmd

// qaTMDBBaseURL 在正式构建里总是空串：TMDB 接口地址固定，API Key 不会被发到别的地方。
func qaTMDBBaseURL() string { return "" }
