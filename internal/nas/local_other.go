//go:build !linux

package nas

import "context"

// LocalSupported 是这个平台能不能用本机来源（只有 Linux 有 /proc 与 /sys）。
const LocalSupported = false

// NewLocalSource 在 Linux 以外的平台上是一个用不了的来源：Collect 总是返回 ErrUnsupported。
func NewLocalSource() Source { return unsupportedLocal{} }

type unsupportedLocal struct{}

func (unsupportedLocal) Collect(context.Context, bool) (*Snapshot, error) { return nil, ErrUnsupported }
func (unsupportedLocal) Close() error                                     { return nil }
