//go:build unix

package organize

import (
	"os"
	"syscall"
)

// sameDevice 报告两个目录是不是在同一个文件系统上。
func sameDevice(a, b os.FileInfo) bool {
	return a.Sys().(*syscall.Stat_t).Dev == b.Sys().(*syscall.Stat_t).Dev
}
