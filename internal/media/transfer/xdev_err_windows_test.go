package transfer

import "golang.org/x/sys/windows"

// errXDev 是这个系统上「不在同一个文件系统」的错误，测试用它模拟跨文件系统（isCrossDevice 认的就是它）。
var errXDev error = windows.ERROR_NOT_SAME_DEVICE
