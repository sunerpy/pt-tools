//go:build !linux

package transfer

import (
	"errors"
	"os"
)

// cloneFile 在 Linux 以外不试写时复制。
func cloneFile(*os.File, *os.File) error { return errCloneUnsupported }

// cloneUnsupported 报告克隆失败是因为做不了，这时逐字节复制。
func cloneUnsupported(err error) bool { return errors.Is(err, errCloneUnsupported) }
