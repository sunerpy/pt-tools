//go:build windows

package organize

import "errors"

// errRealtimeUnsupported 表示这个系统上不做实时监控（Windows 上用不到）。
var errRealtimeUnsupported = errors.New("这个系统上不支持实时监控")

// networkFS 在 Windows 上不分辨网络驱动器（ReadDirectoryChangesW 照常用）。
func networkFS(string) (string, bool) { return "", false }

func watchLimit(error) bool { return false }

func instanceLimit(error) bool { return false }

// realtimeSupported 报告这个系统上做不做实时监控。
func realtimeSupported() bool { return true }
