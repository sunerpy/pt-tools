//go:build linux

package nas

// LocalSupported 是这个平台能不能用本机来源（只有 Linux 有 /proc 与 /sys）。
const LocalSupported = true

// NewLocalSource 是读本机 /proc 与 /sys 的本机来源。
func NewLocalSource() Source { return NewLocal("/") }
