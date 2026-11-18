//go:build !linux && !windows

package transfer

import "errors"

var errNoReplaceUnsupported = errors.New("不支持不覆盖的改名")

// renameNoReplace 在 Linux 以外没有原子的不覆盖改名：交给调用方用独占新建的方式放到目标位置。
func renameNoReplace(string, string) error {
	return errNoReplaceUnsupported
}

// moveNoReplace 同 renameNoReplace（这些平台都不支持）。
func moveNoReplace(string, string) error {
	return errNoReplaceUnsupported
}
