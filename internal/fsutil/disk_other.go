//go:build !unix && !windows

package fsutil

// DiskUsage 在这个系统上读不了磁盘空间。
func DiskUsage(string) (Usage, error) { return Usage{}, ErrUnsupported }
