//go:build windows

package transfer

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

// FileID 是文件所在卷的序列号、文件索引、大小与修改时间（同一个文件的硬链接相同）。软链接取链接本身。
func FileID(path string) (string, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	h, err := windows.CreateFile(p, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x:%x%08x:%x%08x:%x%08x", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow,
		info.FileSizeHigh, info.FileSizeLow, info.LastWriteTime.HighDateTime, info.LastWriteTime.LowDateTime), nil
}

func isCrossDevice(err error) bool {
	return errors.Is(err, windows.ERROR_NOT_SAME_DEVICE)
}
