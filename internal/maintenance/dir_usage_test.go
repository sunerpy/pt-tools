// MIT License
// Copyright (c) 2025 pt-tools

package maintenance

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// TE1：每个类别应报告其目录当前总占用 DirUsedBytes（含不可删的 base 文件），
// 与 FreedBytes（仅已删）区分。logs / backups 均验证；dry-run 也应返回。
func TestCleaner_ReportsDirUsedBytes(t *testing.T) {
	home, db := setupCleanerHome(t)
	logs := filepath.Join(home, models.WorkDir, "logs")
	backups := filepath.Join(home, models.WorkDir, "backups")

	base := filepath.Join(logs, "all.log")
	require.NoError(t, os.WriteFile(base, make([]byte, 100), 0o644)) // 不可删 base：100B
	oldBackup := filepath.Join(logs, "all-2020-01-01T00-00-00.000.log.gz")
	require.NoError(t, os.WriteFile(oldBackup, make([]byte, 50), 0o644)) // 可删备份：50B
	ancient := time.Now().Add(-400 * 24 * time.Hour)
	require.NoError(t, os.Chtimes(oldBackup, ancient, ancient))

	// backups：3 份各 30B，keep=5 → 均保留、freed=0，但 dir 占用应为 90B。
	for _, n := range []string{"a", "b", "c"} {
		require.NoError(t, os.WriteFile(filepath.Join(backups, "backup-"+n+".json"), make([]byte, 30), 0o644))
	}

	c := NewCleaner(home, db, defaultZap())

	dry, err := c.Clean(context.Background(), CleanOptions{
		Categories: []CleanCategory{CategoryLogs, CategoryBackups}, DryRun: true,
	})
	require.NoError(t, err)
	dryLogs := findCategory(t, dry, CategoryLogs)
	assert.Equal(t, int64(150), dryLogs.DirUsedBytes, "dry-run logs 目录占用应含 base+备份=150")

	res, err := c.Clean(context.Background(), CleanOptions{
		Categories: []CleanCategory{CategoryLogs, CategoryBackups}, DryRun: false,
	})
	require.NoError(t, err)

	logsCat := findCategory(t, res, CategoryLogs)
	assert.Equal(t, int64(150), logsCat.DirUsedBytes, "logs 目录总占用含不可删 base")
	assert.Equal(t, int64(50), logsCat.FreedBytes, "freed 仅计已删备份")

	backupsCat := findCategory(t, res, CategoryBackups)
	assert.Equal(t, int64(90), backupsCat.DirUsedBytes, "backups 目录总占用 3*30")
	assert.Equal(t, int64(0), backupsCat.FreedBytes, "keep=5 未删任何备份")
}

func findCategory(t *testing.T, res *CleanResult, cat CleanCategory) CategoryResult {
	t.Helper()
	for _, cr := range res.Categories {
		if cr.Category == cat {
			return cr
		}
	}
	t.Fatalf("类别 %s 未出现在结果中", cat)
	return CategoryResult{}
}

// TF1：红线判定必须大小写不敏感（Windows FS 大小写不敏感）。
func TestIsRedLine_CaseInsensitive(t *testing.T) {
	trueCases := []string{
		"Torrents.DB", "TORRENTS.DB", "torrents.db",
		"TORRENTS.DB-WAL", "Torrents.DB-Shm",
		"SECRET.KEY", "Secret.Key",
		"All.Log", "DEBUG.LOG", "Info.Log", "ERROR.LOG",
	}
	for _, p := range trueCases {
		assert.True(t, isRedLine(p), "%s 应命中红线（大小写不敏感）", p)
		assert.True(t, isRedLine(filepath.Join("/some/dir", p)), "%s 带路径也应命中", p)
	}
	falseCases := []string{
		"all-2020-01-01T00-00-00.000.log.gz",
		"backup-a.json",
		"springsunday-x.torrent",
	}
	for _, p := range falseCases {
		assert.False(t, isRedLine(p), "%s 不应命中红线", p)
	}
}

// TF2：单个候选 os.Remove 失败时记录到 Skipped、不 panic、不中断其余候选删除。
// 构造第一个候选为非空目录（os.Remove 拒绝），第二个为可删文件。
func TestApplyDelete_RemoveFailureSkippedNotAbort(t *testing.T) {
	home, db := setupCleanerHome(t)
	logs := filepath.Join(home, models.WorkDir, "logs")

	badDir := filepath.Join(logs, "all-2020-01-01T00-00-00.000.log.gz")
	require.NoError(t, os.MkdirAll(filepath.Join(badDir, "child"), 0o755)) // 非空目录 → os.Remove 失败
	goodFile := filepath.Join(logs, "all-2021-02-02T00-00-00.000.log.gz")
	require.NoError(t, os.WriteFile(goodFile, make([]byte, 42), 0o644))

	c := NewCleaner(home, db, defaultZap())
	resolvedRoot, err := evalSymlinksExisting(logs)
	require.NoError(t, err)

	cr := &CategoryResult{Category: CategoryLogs}
	assert.NotPanics(t, func() {
		c.applyDelete(resolvedRoot, badDir, "fail-candidate", false, cr)
		c.applyDelete(resolvedRoot, goodFile, "ok-candidate", false, cr)
	})

	assert.NotEmpty(t, cr.Skipped, "删除失败候选应记入 Skipped")
	assert.True(t, containsPath(cr.Deleted, goodFile), "其余候选仍应被删除")
	assert.Equal(t, int64(42), cr.FreedBytes, "FreedBytes 仅计成功删除的候选")
	assert.NoFileExists(t, goodFile)
}
