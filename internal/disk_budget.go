package internal

import (
	"sync/atomic"

	"github.com/sunerpy/pt-tools/global"
)

// DiskBudget 跟踪已预留但尚未落盘的磁盘空间。
// 多个并发 RSS worker 推送前先 Reserve，防止同时看到相同的空闲空间而并发越界。
type DiskBudget struct {
	reserved atomic.Int64
}

// Reserve 预留指定字节数。
// 返回预留后的总已预留字节数。
func (b *DiskBudget) Reserve(bytes int64) int64 {
	return b.reserved.Add(bytes)
}

// Release 释放指定字节数（推送失败时调用）。
func (b *DiskBudget) Release(bytes int64) {
	if bytes <= 0 {
		return
	}
	newVal := b.reserved.Add(-bytes)
	if newVal < 0 {
		b.reserved.CompareAndSwap(newVal, 0)
	}
}

// Reserved 返回当前已预留的字节数。
func (b *DiskBudget) Reserved() int64 {
	v := b.reserved.Load()
	if v < 0 {
		return 0
	}
	return v
}

// Reset 重置预留计数（清理完成后调用以重新校准）。
func (b *DiskBudget) Reset() {
	old := b.reserved.Swap(0)
	if old > 0 {
		global.GetSlogger().Infof("[磁盘预算] 重置预留空间 (释放 %.1f GB)", float64(old)/(1024*1024*1024))
	}
}

// EffectiveFreeGB 计算有效可用空间 = 实际可用 - 已预留。
func (b *DiskBudget) EffectiveFreeGB(actualFreeBytes int64) float64 {
	effective := actualFreeBytes - b.Reserved()
	if effective < 0 {
		return 0
	}
	return float64(effective) / (1024 * 1024 * 1024)
}

// globalDiskBudget 全局单例
var globalDiskBudget = &DiskBudget{}

// GetDiskBudget 返回全局磁盘预算实例。
func GetDiskBudget() *DiskBudget {
	return globalDiskBudget
}
