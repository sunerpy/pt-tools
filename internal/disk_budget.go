// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"sync"
	"sync/atomic"

	"github.com/sunerpy/pt-tools/global"
)

// DiskBudget 跟踪已预留但尚未落盘的磁盘空间。
//
// 多个并发 RSS worker 推送前先 Reserve，防止它们同时观察到相同的 OS 空闲空间
// 而并发越界（Issue #299 根因之一）。Reserve/Release 使用原子计数 + 修正循环
// 保证不会下溢到负数。
//
// 典型用法:
//
//	budget := internal.GetDiskBudget()
//	if budget.EffectiveFreeGB(rawFreeBytes)-torrentSizeGB < threshold {
//	    return ErrInsufficientSpace
//	}
//	budget.Reserve(torrentSize)
//	if pushErr != nil {
//	    budget.Release(torrentSize) // 推送失败时归还配额
//	}
//	// 推送成功的预留由 cleanup_monitor 周期 Reset，或上层在 qBit 已可见后调
//	// Release（避免与 GetIncompletePendingBytes 双重计数）。
type DiskBudget struct {
	reserved atomic.Int64
}

// Reserve 预留指定字节数。bytes <= 0 视为 no-op。返回预留后的总已预留字节数。
func (b *DiskBudget) Reserve(bytes int64) int64 {
	if bytes <= 0 {
		return b.reserved.Load()
	}
	return b.reserved.Add(bytes)
}

// Release 释放指定字节数（推送失败 / 已被下游可见时调用）。
//
// 实现细节：原始版本用 Add(-bytes) + CAS(newVal, 0)，但若并发 Reserve 在
// Add 与 CAS 之间发生，CAS 会失败保留负值，污染后续 Reserved() 读取。
// 此版本用 CompareAndSwap 循环 ——每次基于最新值计算 desired，确保最终
// 不会下溢到 < 0。
func (b *DiskBudget) Release(bytes int64) {
	if bytes <= 0 {
		return
	}
	for {
		current := b.reserved.Load()
		desired := current - bytes
		if desired < 0 {
			desired = 0
		}
		if b.reserved.CompareAndSwap(current, desired) {
			return
		}
	}
}

// Reserved 返回当前已预留的字节数。下溢的负值会被规范化为 0。
func (b *DiskBudget) Reserved() int64 {
	v := b.reserved.Load()
	if v < 0 {
		return 0
	}
	return v
}

// Reset 重置预留计数（清理监控周期开始时调用以重新校准）。
func (b *DiskBudget) Reset() {
	old := b.reserved.Swap(0)
	if old > 0 {
		// 仅当真有预留被清空时才打日志，避免空闲期日志噪音。
		// GetSloggerSafe 在 GlobalLogger 未初始化（如单测）时安全返回 nil。
		if logger := global.GetSloggerSafe(); logger != nil {
			logger.Infof("[磁盘预算] 重置预留空间 (释放 %.1f GB)", float64(old)/(1024*1024*1024))
		}
	}
}

// EffectiveFreeGB 计算"扣除已预留后"的有效可用空间（GB）。
// actualFreeBytes 通常来自 downloader.GetClientFreeSpace。下溢钳位为 0。
func (b *DiskBudget) EffectiveFreeGB(actualFreeBytes int64) float64 {
	effective := actualFreeBytes - b.Reserved()
	if effective < 0 {
		return 0
	}
	return float64(effective) / (1024 * 1024 * 1024)
}

// EffectiveFreeBytes 同 EffectiveFreeGB 但返回字节数。供下游做更精细比较。
func (b *DiskBudget) EffectiveFreeBytes(actualFreeBytes int64) int64 {
	effective := actualFreeBytes - b.Reserved()
	if effective < 0 {
		return 0
	}
	return effective
}

// 全局单例 + 推送互斥锁。pushMutex 串行化 disk-check + Reserve + AddTorrentFileEx
// critical section，关闭多 RSS worker 之间的 race（即便有 Reserve，竞态读取
// freeSpace 与 Reserve 之间的窗口仍可能让两个 worker 同时通过）。
var (
	globalDiskBudget = &DiskBudget{}
	globalPushMutex  sync.Mutex
)

// GetDiskBudget 返回全局磁盘预算实例。
func GetDiskBudget() *DiskBudget { return globalDiskBudget }

// PushMutex 返回全局推送互斥锁。调用方应在 disk-check 前 Lock，
// 推送结束后 Unlock。注意：锁的范围是单实例进程级，多实例部署需要外部协调。
func PushMutex() *sync.Mutex { return &globalPushMutex }
