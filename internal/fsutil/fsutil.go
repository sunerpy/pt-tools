// Package fsutil 读本机磁盘的剩余空间（媒体中心 M8 的首页存储与系统体检）。
package fsutil

import "errors"

// Usage 是一个路径所在磁盘的空间：Free 是当前用户能用的剩余字节，Total 是总字节；Device 认同一块盘（两个路径的 Device 一样时在
// 同一个文件系统上，首页按它去重，硬链接也只能在同一块盘里建）。
type Usage struct {
	Free   uint64 `json:"free"`
	Total  uint64 `json:"total"`
	Device string `json:"-"`
}

// ErrUnsupported 表示这个系统上读不了磁盘空间。
var ErrUnsupported = errors.New("这个系统上读不了磁盘空间")

const (
	// lowPercent、lowBytes 是剩余空间的警告线：低于总量的 5%，或者低于 20 GB
	lowPercent = 5
	lowBytes   = 20 << 30
)

// LowSpace 报告剩余空间是不是低于警告线（总量的 5% 或 20 GB，先到哪条算哪条）。总量不知道（0）时只看 20 GB。
func LowSpace(free, total uint64) bool {
	if free < lowBytes {
		return true
	}
	return total > 0 && free*100 < total*lowPercent
}
