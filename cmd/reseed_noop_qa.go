//go:build !qa

package cmd

// qaIYUUBaseURL 在正式构建里总是空串：IYUU 接口地址固定，token 不会被发到别的地方。
func qaIYUUBaseURL() string { return "" }
