//go:build windows

package transfer

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

var errNoReplaceUnsupported = errors.New("不支持不覆盖的改名")

// renameNoReplace 交给调用方用独占新建的方式放到目标位置（移动与复制沿用这个做法）。
func renameNoReplace(string, string) error {
	return errNoReplaceUnsupported
}

// moveNoReplace 是 Place 用的不覆盖的改名：MoveFileEx 不带 MOVEFILE_REPLACE_EXISTING，目标已存在时失败、不覆盖。
func moveNoReplace(oldpath, newpath string) error {
	from, err := windows.UTF16PtrFromString(oldpath)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(newpath)
	if err != nil {
		return err
	}
	err = windows.MoveFileEx(from, to, 0)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) || errors.Is(err, windows.ERROR_FILE_EXISTS) {
		return fmt.Errorf("%s: %w", newpath, os.ErrExist)
	}
	return err
}
