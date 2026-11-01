// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// DiskBudget 跟踪推送了、但下载器还没算进去的种子要占的磁盘空间。
//
// 多个并发 RSS worker 推送前先预留，防止它们同时观察到相同的 OS 空闲空间而并发越界（Issue #299）。
// 每份预留记着推给了哪个下载器、哪个种子：下次检查这个下载器时（见 PendingForGate），已经出现在下载器里的
// 种子丢掉预留——没下完的剩余体积已经算在「下载中待占用」里，下完的已经占了磁盘，再预留就算了两遍。
// 一直没出现的（例如种子的 hash 对不上）过 reservationTTL 丢掉；清理监控每轮还会全部清零（Reset）。
//
// 典型用法（持有 PushMutex）:
//
//	pending, err := internal.PendingForGate(ctx, dl, name) // 顺带丢掉已经出现在下载器里的
//	effective := free - pending - budget.Reserved()
//	release := budget.ReserveTorrent(name, hash, size)
//	if pushErr != nil {
//	    release() // 推送失败时归还
//	}
type DiskBudget struct {
	mu      sync.Mutex
	seq     uint64
	entries map[uint64]*reservation
	// now 取当前时间（测试里换成假的）；nil 时用 time.Now
	now func() time.Time
}

// reservation 是一份预留。downloader 与 hash 为空时不知道推给了哪里（旧接口 Reserve），只能等过期或清零。
type reservation struct {
	downloader string
	hash       string
	bytes      int64
	at         time.Time
}

// reservationTTL 是一份预留最长留多久：推送成功的种子几秒内就会出现在下载器里，过了这么久还没出现的
// 多半是 hash 对不上，丢掉免得一直占着。
const reservationTTL = 15 * time.Minute

func (b *DiskBudget) clock() time.Time {
	if b.now != nil {
		return b.now()
	}
	return time.Now()
}

// pruneLocked 丢掉过期的预留（调用方持有 b.mu）。
func (b *DiskBudget) pruneLocked() {
	now := b.clock()
	for id, r := range b.entries {
		if now.Sub(r.at) >= reservationTTL {
			delete(b.entries, id)
		}
	}
}

// add 记下一份预留，返回编号；bytes <= 0 时不记，返回 0。
func (b *DiskBudget) add(downloaderName, hash string, bytes int64) uint64 {
	if bytes <= 0 {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.entries == nil {
		b.entries = map[uint64]*reservation{}
	}
	b.seq++
	b.entries[b.seq] = &reservation{downloader: downloaderName, hash: strings.ToLower(hash), bytes: bytes, at: b.clock()}
	return b.seq
}

// ReserveTorrent 为推给下载器 downloaderName 的种子 hash 预留 bytes，返回归还用的函数：推送失败时调用，调用多次也只归还一次；
// 推送成功的不用调（种子出现在下载器里、过期或清零时丢掉）。bytes <= 0 时什么也不记。
func (b *DiskBudget) ReserveTorrent(downloaderName, hash string, bytes int64) (release func()) {
	id := b.add(downloaderName, hash, bytes)
	return func() {
		if id == 0 {
			return
		}
		b.mu.Lock()
		delete(b.entries, id)
		b.mu.Unlock()
	}
}

// Reserve 预留 bytes，不记下载器与种子（只能等过期或清零）。bytes <= 0 视为 no-op。返回预留后的合计。
func (b *DiskBudget) Reserve(bytes int64) int64 {
	b.add("", "", bytes)
	return b.Reserved()
}

// Release 按体积归还（配 Reserve 用）：从最近的预留扣起，扣完为止，不会扣成负数。
func (b *DiskBudget) Release(bytes int64) {
	if bytes <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for bytes > 0 && len(b.entries) > 0 {
		var last uint64
		for id := range b.entries {
			last = max(last, id)
		}
		r := b.entries[last]
		if r.bytes > bytes {
			r.bytes -= bytes
			return
		}
		bytes -= r.bytes
		delete(b.entries, last)
	}
}

// Reserved 返回还没过期的预留合计。
func (b *DiskBudget) Reserved() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pruneLocked()
	var total int64
	for _, r := range b.entries {
		total += r.bytes
	}
	return total
}

// Seen 丢掉下载器 downloaderName 上、种子已经出现在下载器里的预留（hashes 是下载器里所有种子的 info hash，小写），
// 返回丢掉的字节数与份数。
func (b *DiskBudget) Seen(downloaderName string, hashes map[string]struct{}) (freed int64, n int) {
	if len(hashes) == 0 {
		return 0, 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, r := range b.entries {
		if r.hash == "" || r.downloader != downloaderName {
			continue
		}
		if _, ok := hashes[r.hash]; ok {
			freed += r.bytes
			n++
			delete(b.entries, id)
		}
	}
	return freed, n
}

// Reset 清零（清理监控每轮开始时调用，重新校准）。
func (b *DiskBudget) Reset() {
	b.mu.Lock()
	var old int64
	for _, r := range b.entries {
		old += r.bytes
	}
	b.entries = nil
	b.mu.Unlock()
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
	return float64(b.EffectiveFreeBytes(actualFreeBytes)) / (1024 * 1024 * 1024)
}

// EffectiveFreeBytes 同 EffectiveFreeGB 但返回字节数。供下游做更精细比较。
func (b *DiskBudget) EffectiveFreeBytes(actualFreeBytes int64) int64 {
	effective := actualFreeBytes - b.Reserved()
	if effective < 0 {
		return 0
	}
	return effective
}

// PendingForGate 读下载器 dl（预留记在名字 name 下，见 ReserveTorrent）的「下载中待占用」，同时丢掉这台下载器上、
// 种子已经出现在下载器里的预留：两件事出自同一份种子列表，丢掉的预留在这份待占用里一定已经算上了（下完的已经占了磁盘），
// 不会少算。下载器给不出种子列表（没实现 downloader.PendingSnapshotter）时只读待占用，预留等过期或清理监控清零。
// 调用方持有 PushMutex。
func PendingForGate(ctx context.Context, dl downloader.Downloader, name string) (int64, error) {
	s, ok := dl.(downloader.PendingSnapshotter)
	if !ok {
		return dl.GetIncompletePendingBytes(ctx)
	}
	snap, err := s.IncompletePendingSnapshot(ctx)
	if err != nil {
		return 0, err
	}
	if freed, n := GetDiskBudget().Seen(name, snap.Hashes); n > 0 {
		if logger := global.GetSloggerSafe(); logger != nil {
			logger.Infof("[磁盘预算] %s: %d 个推送已经出现在下载器里（算进了下载中待占用），释放预留 %.1f GB",
				name, n, float64(freed)/(1024*1024*1024))
		}
	}
	return snap.Bytes, nil
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
