//go:build !windows

package transfer

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// FileID 是文件所在设备、inode 编号、大小与修改时间（同一个文件的硬链接相同）。软链接取链接本身。
// 带上大小与修改时间：文件删掉后 inode 编号很快会被新文件重用，只比 inode 会把换掉的文件当成原来的。
func FileID(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("读不到 %s 的 inode", path)
	}
	return fmt.Sprintf("%x:%x:%x:%x", uint64(st.Dev), uint64(st.Ino), info.Size(), info.ModTime().UnixNano()), nil //nolint:unconvert // Dev 在部分平台不是 uint64
}

func isCrossDevice(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}
