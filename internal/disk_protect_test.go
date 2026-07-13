// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeebo/bencode"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	sm "github.com/sunerpy/pt-tools/mocks"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

// setUpDiskProtectTest 为磁盘保护测试初始化全局 DB + 写入 SettingsGlobal 行。
// 每个测试都需要：(1) 干净的 GlobalDB；(2) CleanupDiskProtect=true；
// (3) CleanupMinDiskSpaceGB=20；(4) 重置 budget。
//
// 使用 ConfigStore.SaveGlobalSettings 而非直接 db.Save —— 后者依赖 ID 主键
// 匹配，可能与 setupDB 已写入的行冲突，导致测试看到 default 行而非我们配置。
func setUpDiskProtectTest(t *testing.T) {
	t.Helper()
	db := setupDB(t)
	global.GlobalDB = db
	t.Cleanup(func() { global.GlobalDB = nil })

	store := core.NewConfigStore(db)
	require.NoError(t, store.SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir:           t.TempDir(),
		MaxRetry:              0,
		CleanupDiskProtect:    true,
		CleanupMinDiskSpaceGB: 20,
		CleanupEnabled:        false,
	}))

	// 验证 ConfigStore 真能取出
	got, err := store.GetGlobalOnly()
	require.NoError(t, err)
	require.True(t, got.CleanupDiskProtect, "CleanupDiskProtect 应为 true")
	require.Equal(t, float64(20), got.CleanupMinDiskSpaceGB)

	// 重置全局预算，避免测试间泄漏
	GetDiskBudget().Reset()
	t.Cleanup(func() { GetDiskBudget().Reset() })
}

// makeTorrentInfoWithSize 创建一个带 TorrentSize 的种子记录。
// FreeEndTime 必须置于未来，否则 processSingleTorrentWithDownloader 在 line 244
// 会判定过期并提前 return nil（导致 mock 期望落空）。
func makeTorrentInfoWithSize(t *testing.T, db *models.TorrentDB, hash string, sizeBytes int64) {
	t.Helper()
	pushed := false
	future := time.Now().Add(1 * time.Hour)
	ti := &models.TorrentInfo{
		SiteName:     string(models.SiteGroup("springsunday")),
		TorrentHash:  &hash,
		IsPushed:     &pushed,
		FreeEndTime:  &future,
		TorrentSize:  sizeBytes,
		IsDownloaded: true,
	}
	require.NoError(t, db.UpsertTorrent(ti))
}

// TestDiskProtect_RejectsWhenSizeExceedsEffectiveFree 验证：
// 即将推送种子大小被纳入计算 — 80GB 种子 + 80GB free + 20GB 阈值 → 80-80=0 < 20 拒绝。
// 旧实现（不减种子大小）会通过 (80 > 20)。
func TestDiskProtect_RejectsWhenSizeExceedsEffectiveFree(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	makeTorrentInfoWithSize(t, global.GlobalDB, hash, 80*gb)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("test-dl").AnyTimes()
	mockDl.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(80*gb, nil)
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil)
	// AddTorrentFileEx 不应被调用 —— 这是关键断言

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	// Issue #450：effective_free(80) > min(20) 但 80-80=0 < 20 属"单种子过大"，
	// 语义拆分后返回 ErrTorrentTooLarge（而非 ErrInsufficientSpace）。
	require.ErrorIs(t, err, downloader.ErrTorrentTooLarge,
		"种子 80GB > effective_free 80GB - 阈值 20GB 属过大，应跳过")
	assert.Equal(t, int64(0), GetDiskBudget().Reserved(),
		"被拒后不应预留")
}

// TestDiskProtect_AllowsWhenSizeFits 验证：30GB 种子 + 80GB free 通过 (80-30=50 > 20)。
func TestDiskProtect_AllowsWhenSizeFits(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	makeTorrentInfoWithSize(t, global.GlobalDB, hash, 30*gb)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("test-dl").AnyTimes()
	mockDl.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(80*gb, nil)
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil)
	mockDl.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).
		Return(downloader.AddTorrentResult{Success: true, Hash: hash}, nil)

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	require.NoError(t, err)
	assert.Equal(t, 30*gb, GetDiskBudget().Reserved(),
		"推送成功后预留必须保留：qBit torrents/info 在 1~2 秒内还看不到本种子，"+
			"立即 Release 会让并发 worker 重复借用同一份磁盘空间（Issue #299 race）。"+
			"预留由 cleanup_monitor 周期 Reset 归还。")
}

// TestDiskProtect_PendingDownloadsSubtracted 验证 in-flight pending 被扣除：
// 100GB free + 60GB pending（已下载中的种子）+ 30GB 新种子 + 20GB 阈值
// → effective=100-60=40，40-30=10 < 20 拒绝。
// 这是 Issue #299 的核心修复 —— qBit 报告 100GB 但其实 60GB 已被在下载中的种子占用。
func TestDiskProtect_PendingDownloadsSubtracted(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	makeTorrentInfoWithSize(t, global.GlobalDB, hash, 30*gb)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("test-dl").AnyTimes()
	mockDl.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(100*gb, nil)
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(60*gb, nil)
	// AddTorrentFileEx 不应被调用

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	// Issue #450：effective(40) > min(20) 但 40-30=10 < 20 属"单种子过大"，
	// 语义拆分后返回 ErrTorrentTooLarge。pending 扣除逻辑本身不变。
	require.ErrorIs(t, err, downloader.ErrTorrentTooLarge,
		"100GB free - 60GB pending - 30GB new = 10 < 20 阈值 应被跳过")
}

// TestDiskProtect_ReleaseOnPushFailure 验证推送失败时 Reserve 被归还。
func TestDiskProtect_ReleaseOnPushFailure(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	makeTorrentInfoWithSize(t, global.GlobalDB, hash, 30*gb)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("test-dl").AnyTimes()
	mockDl.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(100*gb, nil)
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil)
	mockDl.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).
		Return(downloader.AddTorrentResult{Success: false, Message: "boom"}, errors.New("boom"))

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	require.Error(t, err)
	assert.Equal(t, int64(0), GetDiskBudget().Reserved(),
		"推送失败必须 Release 归还预算，否则后续永远无法通过检查")
}

// TestDiskProtect_FailClosedOnFreeSpaceError 验证 fail-closed：
// GetClientFreeSpace 出错时，磁盘保护启用应拒绝推送（旧版"继续推送"是 bug）。
func TestDiskProtect_FailClosedOnFreeSpaceError(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	makeTorrentInfoWithSize(t, global.GlobalDB, hash, 30*gb)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("test-dl").AnyTimes()
	mockDl.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(int64(0), errors.New("qbit unreachable"))
	// AddTorrentFileEx 不应被调用 —— fail-closed 验证

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	require.ErrorIs(t, err, downloader.ErrInsufficientSpace,
		"GetClientFreeSpace 失败时应 fail-closed（拒绝），而非 fail-open（旧实现）")
}

// TestDiskProtect_PendingErrorTreatedAsZero 验证 GetIncompletePendingBytes 出错时
// 退化为 0（保守但仍生效）—— 不应整体 fail-closed，因为 pending 查询是辅助信号。
func TestDiskProtect_PendingErrorTreatedAsZero(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	makeTorrentInfoWithSize(t, global.GlobalDB, hash, 30*gb)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("test-dl").AnyTimes()
	mockDl.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(100*gb, nil)
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), errors.New("api fail"))
	mockDl.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).
		Return(downloader.AddTorrentResult{Success: true, Hash: hash}, nil)

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	require.NoError(t, err, "GetIncompletePendingBytes 失败应退化为 0，不阻止推送")
}

// TestDiskProtect_ConcurrentPushesSerializeAndReject 是 Issue #299 race 的端到端回归。
//
// 模拟 5 个并发 worker 共享 DiskBudget + PushMutex 同时执行 disk-check + Reserve 流程，
// downloader 每次都报告稳定的 "100GB free, 0 pending"（qBit 还没开始下载）。
// 期望：互斥锁 + Reserve 让前 2 个通过 (Reserve 30GB → 60GB → 第 3 个看到
// effective=40<min=20+30=50 拒)；3 个被拒。
//
// 这是直接调用核心算法（绕过 DB 层）的并发回归 —— 完整 e2e 在 setup 上太复杂，
// 此测试足以证明 race 修复生效，而 e2e 测试见前面 6 个单 worker 用例。
func TestDiskProtect_ConcurrentPushesSerializeAndReject(t *testing.T) {
	GetDiskBudget().Reset()
	t.Cleanup(func() { GetDiskBudget().Reset() })
	const torrentSize = 30 * gb
	const free = 100 * gb
	const pending = 0
	const minBytes = 20 * gb
	const workers = 5

	var success, fail atomic.Int32
	var wg sync.WaitGroup
	mu := PushMutex()
	budget := GetDiskBudget()

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.Lock()
			defer mu.Unlock()

			effective := free - pending - budget.Reserved()
			if effective < 0 {
				effective = 0
			}
			if effective-torrentSize < minBytes {
				fail.Add(1)
				return
			}
			budget.Reserve(torrentSize)
			success.Add(1)
		}()
	}
	wg.Wait()

	t.Logf("成功 %d, 失败 %d, 最终预留 %d GB",
		success.Load(), fail.Load(), budget.Reserved()/gb)

	// 关键不变量 (Issue #299 race 修复)：
	// - 不会全部成功（5/5），那是旧 race bug
	// - 期望 successes ∈ [2,2]，fail ∈ [3,3]：100-(2*30)=40，再扣 30 = 10 < 20 阈值
	assert.Equal(t, int32(2), success.Load(),
		"互斥锁 + Reserve 应让恰好 2 个通过：100GB - 60GB reserved - 30GB new = 10 < 20")
	assert.Equal(t, int32(3), fail.Load(),
		"剩余 3 个 worker 应被磁盘保护拦截")
	assert.Equal(t, 2*torrentSize, budget.Reserved(),
		"通过的 2 个 worker 共预留 60GB")
}

// makeUniqueTorrentFile 创建一个内容唯一（基于 nameSuffix）的 .torrent 文件，
// 用于让并发测试里多个 worker 拿到不同的 hash + 不同的 DB 行。
func makeUniqueTorrentFile(t *testing.T, dir, nameSuffix string) (string, string) {
	t.Helper()
	var buf bytes.Buffer
	torrent := map[string]any{"info": map[string]any{"name": "abc-" + nameSuffix}}
	require.NoError(t, bencode.NewEncoder(&buf).Encode(torrent))
	path := filepath.Join(dir, "x-"+nameSuffix+".torrent")
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o644))
	h, err := qbit.ComputeTorrentHashWithPath(path)
	require.NoError(t, err)
	return path, h
}

// TestDiskProtect_RealRSSPath_SuccessKeepsReservation 是 P0 race 的真实路径回归。
//
// 旧版（推送成功立即 Release）会让此测试失败：
//
//	worker A 推送成功 → 立即 Release(30GB) → Reserved=0
//	worker B 此时进入临界区 → 看到 effective_free = 100-0-0 = 100，30GB 通过
//	worker C 同样通过 → 三个 worker 同时占用 100GB 中的 90GB
//	最终 Reserved = 30GB（只有最后一个未 Release），但实际已超借
//
// 新版预期：成功后保留预留，cleanup_monitor 周期 Reset 归还。
//
// 100GB free, 0 pending, 30GB 种子, 阈值 20GB。
// 第 1 个：effective=100, 100-30=70 ≥ 20，通过，Reserved=30
// 第 2 个：effective=70, 70-30=40 ≥ 20，通过，Reserved=60
// 第 3 个：effective=40, 40-30=10 < 20，拒绝
// 第 4/5 个：同样拒绝
// 期望 success=2 / fail=3 / final reserved = 60GB。
func TestDiskProtect_RealRSSPath_SuccessKeepsReservation(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()

	const workers = 5
	const torrentSize int64 = 30 * gb
	const free int64 = 100 * gb

	type job struct {
		path string
		hash string
	}
	jobs := make([]job, workers)
	for i := 0; i < workers; i++ {
		path, hash := makeUniqueTorrentFile(t, dir, fmt.Sprintf("w%d", i))
		pushed := false
		future := time.Now().Add(1 * time.Hour)
		ti := &models.TorrentInfo{
			SiteName:     string(models.SiteGroup("springsunday")),
			TorrentID:    fmt.Sprintf("race-w%d", i),
			TorrentHash:  &hash,
			IsPushed:     &pushed,
			FreeEndTime:  &future,
			TorrentSize:  torrentSize,
			IsDownloaded: true,
		}
		require.NoError(t, global.GlobalDB.UpsertTorrent(ti))
		jobs[i] = job{path: path, hash: hash}
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("test-dl").AnyTimes()
	mockDl.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()
	for _, j := range jobs {
		mockDl.EXPECT().CheckTorrentExists(j.hash).Return(false, nil).AnyTimes()
	}
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(free, nil).AnyTimes()
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil).AnyTimes()
	mockDl.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).
		DoAndReturn(func(data []byte, _ downloader.AddTorrentOptions) (downloader.AddTorrentResult, error) {
			h, err := qbit.ComputeTorrentHash(data)
			require.NoError(t, err)
			return downloader.AddTorrentResult{Success: true, Hash: h}, nil
		}).AnyTimes()

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}

	var success, fail atomic.Int32
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
				j.path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
			if err != nil {
				// Issue #450：拒绝可能是 insufficient（盘满）或 too-large（单种子过大）。
				// 本场景 effective 一直 > min，被拒的都是 too-large。
				if errors.Is(err, downloader.ErrInsufficientSpace) || errors.Is(err, downloader.ErrTorrentTooLarge) {
					fail.Add(1)
				} else {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			success.Add(1)
		}(j)
	}
	wg.Wait()

	t.Logf("成功 %d, 失败 %d, 最终预留 %d GB",
		success.Load(), fail.Load(), GetDiskBudget().Reserved()/gb)

	assert.Equal(t, int32(2), success.Load(),
		"100GB free / 30GB 种子 / 20GB 阈值 → 仅前 2 个 worker 应该成功推送")
	assert.Equal(t, int32(3), fail.Load(),
		"后 3 个 worker 应被磁盘保护拦截")
	assert.Equal(t, 2*torrentSize, GetDiskBudget().Reserved(),
		"两次成功推送的预留必须保留，等待 cleanup_monitor 周期 Reset；"+
			"如果此处显示 0 表示 P0 race 被复活：成功路径仍在立即 Release。")
}
