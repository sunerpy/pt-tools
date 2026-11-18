//go:build !qa

package feishuapp

// qaBaseURL 在正式构建里恒为空：只连飞书与 Lark 的官方域名。
func qaBaseURL() string { return "" }
