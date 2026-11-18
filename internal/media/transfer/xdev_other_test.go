//go:build !unix

package transfer

import "os"

// sameDevice 在不是 Unix 的系统上总报告同一个文件系统：跨文件系统的测试只在 Linux 上跑。
func sameDevice(_, _ os.FileInfo) bool { return true }
