// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func TestDiskBudget_ReservedNormalizesNegative(t *testing.T) {
	b := &DiskBudget{}
	b.Release(50 * gb) // release with nothing reserved → clamps to 0
	assert.Equal(t, int64(0), b.Reserved())
}

func TestDiskBudget_ReserveAndReleaseBasic(t *testing.T) {
	resetGlobalBudget()
	b := &DiskBudget{}
	assert.Equal(t, int64(0), b.Reserved())

	b.Reserve(100 * gb)
	assert.Equal(t, 100*gb, b.Reserved())

	b.Reserve(50 * gb)
	assert.Equal(t, 150*gb, b.Reserved())

	b.Release(40 * gb)
	assert.Equal(t, 110*gb, b.Reserved())

	b.Release(110 * gb)
	assert.Equal(t, int64(0), b.Reserved())
}

// TestDiskBudget_ReserveZeroNoop 验证 Reserve(0) 和 Release(0) 不影响计数。
func TestDiskBudget_ReserveZeroNoop(t *testing.T) {
	b := &DiskBudget{}
	b.Reserve(0)
	b.Reserve(-100)
	b.Release(0)
	b.Release(-100)
	assert.Equal(t, int64(0), b.Reserved())
}

// TestDiskBudget_ReleaseClampsAtZero 验证 Release 超过当前持有量不会下溢负数。
func TestDiskBudget_ReleaseClampsAtZero(t *testing.T) {
	b := &DiskBudget{}
	b.Reserve(50 * gb)
	b.Release(200 * gb)
	assert.Equal(t, int64(0), b.Reserved())
}

func TestDiskBudget_Reset(t *testing.T) {
	b := &DiskBudget{}
	b.Reserve(123 * gb)
	assert.Equal(t, 123*gb, b.Reserved())
	b.Reset()
	assert.Equal(t, int64(0), b.Reserved())
}

func TestDiskBudget_EffectiveFreeGB(t *testing.T) {
	b := &DiskBudget{}
	b.Reserve(30 * gb)
	got := b.EffectiveFreeGB(100 * gb)
	assert.InDelta(t, 70.0, got, 0.01, "100GB - 30GB reserved = 70GB effective")

	got = b.EffectiveFreeGB(20 * gb)
	assert.Equal(t, 0.0, got, "actualFree < reserved 应钳位为 0")
}

func TestDiskBudget_EffectiveFreeBytes(t *testing.T) {
	b := &DiskBudget{}
	b.Reserve(30 * gb)
	assert.Equal(t, 70*gb, b.EffectiveFreeBytes(100*gb))
	assert.Equal(t, int64(0), b.EffectiveFreeBytes(10*gb), "下溢钳位 0")
}

// TestDiskBudget_ConcurrentReserveRelease 高并发下证明计数最终一致。
// 1000 个 goroutine 各 Reserve 1MB 并随后 Release 1MB —— 最终应回 0 且无下溢。
func TestDiskBudget_ConcurrentReserveRelease(t *testing.T) {
	b := &DiskBudget{}
	const n = 1000
	const each = int64(1024 * 1024)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			b.Reserve(each)
			b.Release(each)
		}()
	}
	wg.Wait()
	assert.Equal(t, int64(0), b.Reserved(), "并发 Reserve+Release 后应归零")
}

// TestDiskBudget_ConcurrentReleaseDoesNotUnderflow 证明并发 Release 在
// 同时 Reserve 干扰下也不会下溢出去。这是修复"Add(-bytes)+CAS"竞态的回归测试。
func TestDiskBudget_ConcurrentReleaseDoesNotUnderflow(t *testing.T) {
	b := &DiskBudget{}
	b.Reserve(1000 * gb)

	var wg sync.WaitGroup
	const releaseN = 500
	const reserveN = 500
	wg.Add(releaseN + reserveN)

	for i := 0; i < releaseN; i++ {
		go func() {
			defer wg.Done()
			b.Release(2 * gb) // 共释放 1000 GB
		}()
	}
	for i := 0; i < reserveN; i++ {
		go func() {
			defer wg.Done()
			b.Reserve(1 * gb) // 共预留 500 GB
		}()
	}
	wg.Wait()

	// 不变量：Reserved() 始终非负
	got := b.Reserved()
	assert.GreaterOrEqual(t, got, int64(0), "Reserved 必须 >= 0，旧版本会下溢")
	// 期望终值：1000 - 1000 + 500 = 500 GB（由于钳位行为在并发下严格相等不可
	// 保证，但应在 [0, 500GB] 范围内 —— 关键不变量是非负）
	assert.LessOrEqual(t, got, 500*gb, "终值不可能超过最大预留量")
}

// TestDiskBudget_RaceConditionGate 模拟 Issue #299 race：两个 worker 同时读
// "120 GB free"，各想推送 80 GB —— 没有 Reserve 的话两个都通过 (120-80=40>20
// 阈值)；有 Reserve 后第二个看到的 effective_free=120-80=40，再减自己的 80=
// -40 < 阈值，被拒。
func TestDiskBudget_RaceConditionGate(t *testing.T) {
	b := &DiskBudget{}
	const rawFree = 120 * gb
	const torrentSize = 80 * gb
	const minThreshold = 20 * gb

	// worker 1
	eff1 := b.EffectiveFreeBytes(rawFree)
	require.GreaterOrEqual(t, eff1-torrentSize, minThreshold, "worker 1 应通过")
	b.Reserve(torrentSize)

	// worker 2 同时跑（rawFree 还没变，因为 qBit 还没开始下载）
	eff2 := b.EffectiveFreeBytes(rawFree)
	assert.Equal(t, 40*gb, eff2, "worker 2 看到的有效空间应已扣除 worker 1 的预留")
	assert.Less(t, eff2-torrentSize, minThreshold,
		"worker 2 应被拦截（40-80 = -40 < 20）—— 这正是 Issue #299 的修复点")
}

// TestDiskBudget_GlobalSingleton 验证 GetDiskBudget 返回同一实例。
func TestDiskBudget_GlobalSingleton(t *testing.T) {
	a := GetDiskBudget()
	c := GetDiskBudget()
	assert.Same(t, a, c)
}

// TestPushMutex_Singleton 验证 PushMutex 返回同一实例并能正常上锁。
func TestPushMutex_Singleton(t *testing.T) {
	m1 := PushMutex()
	m2 := PushMutex()
	assert.Same(t, m1, m2, "PushMutex 必须是同一全局实例")

	// 能锁并解锁，不死锁。Lock + 其它操作 + Unlock 形式让 staticcheck SA2001
	// 满意（empty critical section 误判）。
	m1.Lock()
	_ = m1
	m1.Unlock()
}

// TestPushMutex_SerializesPushers 用 mutex 串行化 100 个 worker 验证
// "并发推送被序列化" 不变量：任意时刻最多 1 个临界区在执行。
func TestPushMutex_SerializesPushers(t *testing.T) {
	mu := PushMutex()
	var inCritical atomic.Int32
	var maxConcurrent atomic.Int32

	var wg sync.WaitGroup
	const n = 100
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			mu.Lock()
			c := inCritical.Add(1)
			// 更新峰值
			for {
				old := maxConcurrent.Load()
				if c <= old || maxConcurrent.CompareAndSwap(old, c) {
					break
				}
			}
			inCritical.Add(-1)
			mu.Unlock()
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(1), maxConcurrent.Load(), "互斥锁应保证临界区最多 1 个 goroutine")
}

// 推给下载器的种子出现在下载器里以后丢掉预留：只丢这台下载器上 hash 对得上的（大小写不论）；
// 别的下载器上同一个 hash 的、还没看到的、旧接口不记种子的都留着
func TestDiskBudget_SeenDropsVisibleReservations(t *testing.T) {
	b := &DiskBudget{}
	b.ReserveTorrent("qb", "AAAA", 10*gb)
	b.ReserveTorrent("qb", "bbbb", 20*gb)
	b.ReserveTorrent("tr", "aaaa", 40*gb)
	b.Reserve(80 * gb)
	require.Equal(t, 150*gb, b.Reserved())

	freed, n := b.Seen("qb", map[string]struct{}{"aaaa": {}, "zzzz": {}})
	assert.Equal(t, 10*gb, freed)
	assert.Equal(t, 1, n)
	assert.Equal(t, 140*gb, b.Reserved())

	freed, n = b.Seen("qb", nil)
	assert.Zero(t, freed)
	assert.Zero(t, n)
	assert.Equal(t, 140*gb, b.Reserved())
}

// 归还函数只归还自己那一份，调用多次也只归还一次；体积 <= 0 时什么也不记
func TestDiskBudget_ReserveTorrentRelease(t *testing.T) {
	b := &DiskBudget{}
	release := b.ReserveTorrent("qb", "aaaa", 10*gb)
	b.ReserveTorrent("qb", "bbbb", 20*gb)
	release()
	release()
	assert.Equal(t, 20*gb, b.Reserved())

	b.ReserveTorrent("qb", "cccc", 0)()
	b.ReserveTorrent("qb", "dddd", -1)()
	assert.Equal(t, 20*gb, b.Reserved())
}

// 一直没出现在下载器里的预留过 reservationTTL 丢掉（例如 hash 对不上），旧接口的也一样
func TestDiskBudget_ReservationsExpire(t *testing.T) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	b := &DiskBudget{now: func() time.Time { return now }}
	b.ReserveTorrent("qb", "aaaa", 10*gb)
	now = now.Add(5 * time.Minute)
	b.Reserve(20 * gb)
	now = now.Add(reservationTTL - 5*time.Minute - time.Second)
	assert.Equal(t, 30*gb, b.Reserved(), "都还没到期")
	now = now.Add(time.Second)
	assert.Equal(t, 20*gb, b.Reserved(), "第一份到期")
	now = now.Add(5 * time.Minute)
	assert.Zero(t, b.Reserved())
}

// snapshotDL 是能给出种子列表的下载器（只用到 IncompletePendingSnapshot）。
type snapshotDL struct {
	downloader.Downloader
	snap downloader.PendingSnapshot
	err  error
}

func (d snapshotDL) IncompletePendingSnapshot(context.Context) (downloader.PendingSnapshot, error) {
	return d.snap, d.err
}

// plainDL 是给不出种子列表的下载器（只用到 GetIncompletePendingBytes）。
type plainDL struct {
	downloader.Downloader
	pending int64
}

func (d plainDL) GetIncompletePendingBytes(context.Context) (int64, error) { return d.pending, nil }

// PendingForGate：下载器给得出种子列表时，待占用与丢预留出自同一份列表（丢掉这台下载器上已经看到的）；
// 读不到时报错、预留不动；给不出种子列表的下载器只读待占用，预留等过期或清零
func TestPendingForGate(t *testing.T) {
	b := GetDiskBudget()
	b.Reset()
	t.Cleanup(b.Reset)
	b.ReserveTorrent("qb", "aaaa", 10*gb)
	b.ReserveTorrent("qb", "bbbb", 20*gb)
	b.ReserveTorrent("tr", "aaaa", 40*gb)

	_, err := PendingForGate(context.Background(), snapshotDL{err: errors.New("连接被拒绝")}, "qb")
	require.Error(t, err)
	assert.Equal(t, 70*gb, b.Reserved(), "读不到种子列表：预留不动")

	pending, err := PendingForGate(context.Background(), snapshotDL{snap: downloader.PendingSnapshot{
		Bytes: 9 * gb, Hashes: map[string]struct{}{"aaaa": {}},
	}}, "qb")
	require.NoError(t, err)
	assert.Equal(t, 9*gb, pending)
	assert.Equal(t, 60*gb, b.Reserved(), "qb 上的 aaaa 已经在下载器里：丢掉；tr 上的不动")

	pending, err = PendingForGate(context.Background(), plainDL{pending: 3 * gb}, "qb")
	require.NoError(t, err)
	assert.Equal(t, 3*gb, pending)
	assert.Equal(t, 60*gb, b.Reserved(), "给不出种子列表：只读待占用")
}
