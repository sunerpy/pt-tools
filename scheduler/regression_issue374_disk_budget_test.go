// MIT License
// Copyright (c) 2025 pt-tools

// Issue #374 regression tests: 「本进程预留」必须在 CleanupDiskProtect=true 时
// 周期归零，独立于 CleanupEnabled 开关。
//
// 修复后契约：
//   maybeResetDiskBudget(cfg):
//     cfg.CleanupDiskProtect=true → Reset() 一次
//     cfg.CleanupDiskProtect=false → no-op
//   与 cfg.CleanupEnabled 解耦。

package scheduler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

const oneGB = int64(1024 * 1024 * 1024)

// TestIssue374_DiskProtectOn_CleanupOff_ResetStillRuns 修复后核心契约：
// 用户配置「磁盘保护开但自动删种关」时，maybeResetDiskBudget 仍会触发 Reset，
// 预留不再单调累积。这是 Bug 报告中 984.6 GB 现象的直接验证。
func TestIssue374_DiskProtectOn_CleanupOff_ResetStillRuns(t *testing.T) {
	db := setupTestDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, db.DB.Create(&models.SettingsGlobal{
		DownloadDir:           t.TempDir(),
		CleanupDiskProtect:    true,
		CleanupMinDiskSpaceGB: 200,
		CleanupEnabled:        false, // 关键：自动删种关
	}).Error)

	budget := ptinternal.GetDiskBudget()
	budget.Reset()
	t.Cleanup(func() { budget.Reset() })

	const n = 30
	const sizePerTorrent = 30 * oneGB
	for i := 0; i < n; i++ {
		budget.Reserve(sizePerTorrent)
	}
	require.Equal(t, int64(n)*sizePerTorrent, budget.Reserved(), "sanity: 累计 900 GB")

	cm := NewCleanupMonitor(db.DB, downloader.NewDownloaderManager())
	cfg := cm.loadConfig()
	require.NotNil(t, cfg)
	require.True(t, cfg.CleanupDiskProtect)
	require.False(t, cfg.CleanupEnabled)

	cm.maybeResetDiskBudget(cfg)

	assert.Equal(t, int64(0), budget.Reserved(),
		"修复 #374：CleanupDiskProtect=true + CleanupEnabled=false 时 Reset 必须运行")
}

// TestIssue374_DiskProtectOff_ResetSkipped 反向：
// 当 CleanupDiskProtect=false 时（用户根本没开磁盘保护），无需 Reset；
// 否则我们就是在偷偷修改一个用户没启用的子系统的状态。
func TestIssue374_DiskProtectOff_ResetSkipped(t *testing.T) {
	db := setupTestDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	row := models.SettingsGlobal{
		DownloadDir:           t.TempDir(),
		CleanupDiskProtect:    false,
		CleanupMinDiskSpaceGB: 0,
		CleanupEnabled:        false,
	}
	require.NoError(t, db.DB.Create(&row).Error)
	// GORM `default:true` 标签会把零值 false 覆盖回 true，必须显式 Update
	require.NoError(t, db.DB.Model(&row).Update("cleanup_disk_protect", false).Error)

	budget := ptinternal.GetDiskBudget()
	budget.Reset()
	t.Cleanup(func() { budget.Reset() })

	budget.Reserve(50 * oneGB)
	require.Equal(t, 50*oneGB, budget.Reserved())

	cm := NewCleanupMonitor(db.DB, downloader.NewDownloaderManager())
	cfg := cm.loadConfig()
	require.NotNil(t, cfg)
	require.False(t, cfg.CleanupDiskProtect)

	cm.maybeResetDiskBudget(cfg)

	assert.Equal(t, 50*oneGB, budget.Reserved(),
		"CleanupDiskProtect=false → Reset 不该跑（DiskBudget 子系统未启用，prep 状态保留）")
}

// TestIssue374_NilConfig_NoOp 防御性：
// loadConfig 失败返回 nil 时 maybeResetDiskBudget 必须是 no-op，
// 不能 panic 或访问 nil 字段。
func TestIssue374_NilConfig_NoOp(t *testing.T) {
	db := setupTestDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	budget := ptinternal.GetDiskBudget()
	budget.Reset()
	t.Cleanup(func() { budget.Reset() })

	budget.Reserve(10 * oneGB)
	cm := NewCleanupMonitor(db.DB, downloader.NewDownloaderManager())

	require.NotPanics(t, func() {
		cm.maybeResetDiskBudget(nil)
	})
	assert.Equal(t, 10*oneGB, budget.Reserved(), "nil cfg → no-op，不动现状")
}

// TestIssue374_BothFlagsOn_ResetRunsViaRunOnce 兼容性：
// 用户同时开 CleanupEnabled+CleanupDiskProtect 时，runOnce 路径仍会 Reset
// （runOnce 内的 resetDiskBudget 调用未被删除）。证明 fix 没破坏旧的 happy path。
func TestIssue374_BothFlagsOn_ResetRunsViaRunOnce(t *testing.T) {
	db := setupTestDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, db.DB.Create(&models.SettingsGlobal{
		DownloadDir:           t.TempDir(),
		CleanupDiskProtect:    true,
		CleanupMinDiskSpaceGB: 200,
		CleanupEnabled:        true,
	}).Error)

	budget := ptinternal.GetDiskBudget()
	budget.Reset()
	t.Cleanup(func() { budget.Reset() })

	budget.Reserve(500 * oneGB)
	cm := NewCleanupMonitor(db.DB, downloader.NewDownloaderManager())
	cfg := cm.loadConfig()
	require.NotNil(t, cfg)

	cm.runOnce(cfg)

	assert.Equal(t, int64(0), budget.Reserved(),
		"CleanupEnabled=true 时 runOnce 路径仍正确清零")
}

// TestIssue374_ResetIsIdempotent 性质测试：
// 多次连续 Reset 等价于一次，不会下溢或异常。runLoop 在 5 分钟节奏下会反复
// 调用，必须保证幂等。
func TestIssue374_ResetIsIdempotent(t *testing.T) {
	db := setupTestDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, db.DB.Create(&models.SettingsGlobal{
		DownloadDir:        t.TempDir(),
		CleanupDiskProtect: true,
	}).Error)

	budget := ptinternal.GetDiskBudget()
	budget.Reset()
	t.Cleanup(func() { budget.Reset() })

	cm := NewCleanupMonitor(db.DB, downloader.NewDownloaderManager())
	cfg := cm.loadConfig()
	require.NotNil(t, cfg)

	for i := 0; i < 10; i++ {
		cm.maybeResetDiskBudget(cfg)
	}
	assert.Equal(t, int64(0), budget.Reserved(), "连续 10 次 Reset 仍为 0")
}
