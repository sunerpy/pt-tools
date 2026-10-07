//go:build !windows

package transfer

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// FileID 是文件所在设备与 inode 编号（同一个文件的硬链接相同）。软链接取链接本身。
func FileID(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("读不到 %s 的 inode", path)
	}
	return fmt.Sprintf("%x:%x", uint64(st.Dev), uint64(st.Ino)), nil //nolint:unconvert // Dev 在部分平台不是 uint64
}

func isCrossDevice(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}
