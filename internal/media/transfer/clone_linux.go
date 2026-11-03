//go:build linux

package transfer

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// cloneFile 让 dst 和 src 共用磁盘上的数据（FICLONE 写时复制，Btrfs、XFS 等支持）：不多占空间，之后改了哪边才各自写新的。
func cloneFile(dst, src *os.File) error {
	return unix.IoctlFileClone(int(dst.Fd()), int(src.Fd()))
}

// cloneUnsupported 报告克隆失败是因为做不了（文件系统不支持、跨文件系统、内核没有这个调用），这时逐字节复制。
func cloneUnsupported(err error) bool {
	return errors.Is(err, errCloneUnsupported) || errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.EXDEV) ||
		errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOTTY) || errors.Is(err, unix.ENOSYS)
}
