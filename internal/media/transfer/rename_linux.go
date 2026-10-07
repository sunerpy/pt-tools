//go:build linux

package transfer

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

var errNoReplaceUnsupported = errors.New("不支持不覆盖的改名")

// renameNoReplace 把 oldpath 改名为 newpath，newpath 已存在时不覆盖、返回 os.ErrExist。
// 文件系统不支持 RENAME_NOREPLACE（部分网络文件系统）时返回 errNoReplaceUnsupported。
func renameNoReplace(oldpath, newpath string) error {
	err := unix.Renameat2(unix.AT_FDCWD, oldpath, unix.AT_FDCWD, newpath, unix.RENAME_NOREPLACE)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, unix.EEXIST):
		return fmt.Errorf("%s: %w", newpath, os.ErrExist)
	case errors.Is(err, unix.EINVAL), errors.Is(err, unix.ENOSYS), errors.Is(err, unix.EOPNOTSUPP), errors.Is(err, unix.ENOTSUP):
		return errNoReplaceUnsupported
	}
	return err
}
