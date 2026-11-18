//go:build !linux && !windows

package organize

import "errors"

// errRealtimeUnsupported 表示这个系统上不做实时监控：macOS、BSD 的 kqueue 每个文件都要占一个文件描述符，
// 监控目录里文件多时会把描述符用光。只按间隔扫描。
var errRealtimeUnsupported = errors.New("这个系统上不支持实时监控（只在 Linux 与 Windows 上）")

func networkFS(string) (string, bool) { return "", false }

func watchLimit(error) bool { return false }

func instanceLimit(error) bool { return false }

// realtimeSupported 报告这个系统上做不做实时监控。
func realtimeSupported() bool { return false }
