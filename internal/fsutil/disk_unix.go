//go:build unix

package fsutil

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// DiskUsage 读 path 所在磁盘的空间（path 要存在）。
func DiskUsage(path string) (Usage, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return Usage{}, fmt.Errorf("读不到 %s 的磁盘空间: %w", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return Usage{}, fmt.Errorf("读不到 %s 的磁盘空间: %w", path, err)
	}
	u := Usage{Free: st.Bavail * uint64(st.Bsize), Total: st.Blocks * uint64(st.Bsize)} //nolint:gosec // 块大小不会是负数
	if sys, ok := info.Sys().(*syscall.Stat_t); ok {
		u.Device = fmt.Sprint(sys.Dev)
	}
	return u, nil
}
