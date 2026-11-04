//go:build windows

package fsutil

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
)

// DiskUsage 读 path 所在磁盘的空间（path 要存在）；同一个卷（例如 D:\）的路径 Device 相同。
func DiskUsage(path string) (Usage, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return Usage{}, fmt.Errorf("读不到 %s 的磁盘空间: %w", path, err)
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree); err != nil {
		return Usage{}, fmt.Errorf("读不到 %s 的磁盘空间: %w", path, err)
	}
	u := Usage{Free: free, Total: total}
	vol := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumePathName(p, &vol[0], uint32(len(vol))); err == nil {
		u.Device = strings.ToUpper(windows.UTF16ToString(vol))
	}
	return u, nil
}
