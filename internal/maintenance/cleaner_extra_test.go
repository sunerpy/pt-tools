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

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/models"
)

// TestCleaner_MissingWorkDirAborts：工作目录不存在时整体放弃清理并返回错误。
func TestCleaner_MissingWorkDirAborts(t *testing.T) {
	home := filepath.Join(t.TempDir(), "no-such-home")
	_, db := setupCleanerHome(t)
	c := NewCleaner(home, db, defaultZap())
	res, err := c.Clean(context.Background(), CleanOptions{})
	require.Error(t, err)
	assert.Nil(t, res)
}

// TestCleaner_AllCategoriesDefault：Categories 为空时清理全部三类。
func TestCleaner_AllCategoriesDefault(t *testing.T) {
	home, db := setupCleanerHome(t)
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), RetainHours: 24, MaxRetry: 3,
	}))
	work := filepath.Join(home, models.WorkDir)

	oldBackup := filepath.Join(work, "logs", "all-2020-01-01T00-00-00.000.log.gz")
	require.NoError(t, os.WriteFile(oldBackup, []byte("old"), 0o644))
	ancient := time.Now().Add(-400 * 24 * time.Hour)
	require.NoError(t, os.Chtimes(oldBackup, ancient, ancient))
	orphanPath, _ := writeTorrent(t, filepath.Join(work, "downloads"), "springsunday-o.torrent", "alldefault")
	extraBackup := filepath.Join(work, "backups", "old.json")
	require.NoError(t, os.WriteFile(extraBackup, []byte("b"), 0o644))
	require.NoError(t, os.Chtimes(extraBackup, ancient, ancient))
	for i := 0; i < 5; i++ {
		p := filepath.Join(work, "backups", "keep-"+string(rune('a'+i))+".json")
		require.NoError(t, os.WriteFile(p, []byte("k"), 0o644))
	}

	c := NewCleaner(home, db, defaultZap())
	res, err := c.Clean(context.Background(), CleanOptions{})
	require.NoError(t, err)
	assert.Len(t, res.Categories, 3)
	assert.NoFileExists(t, oldBackup)
	assert.NoFileExists(t, orphanPath)
	assert.NoFileExists(t, extraBackup)
}

// TestCleaner_MissingCategoryRootSkipped：类别根不存在时被跳过（无 CategoryResult）。
func TestCleaner_MissingCategoryRootSkipped(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, models.WorkDir), 0o755))
	_, db := setupCleanerHome(t)
	c := NewCleaner(home, db, defaultZap())
	res, err := c.Clean(context.Background(), CleanOptions{Categories: []CleanCategory{CategoryLogs}})
	require.NoError(t, err)
	assert.Empty(t, res.Categories, "根不存在应跳过，不产生结果")
}

// TestCleaner_StagingDisabledWhenRetainZero：RetainHours<=0 时 staging 清理禁用。
func TestCleaner_StagingDisabledWhenRetainZero(t *testing.T) {
	home, db := setupCleanerHome(t)
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), RetainHours: 24, MaxRetry: 3,
	}))
	require.NoError(t, db.DB.Model(&models.SettingsGlobal{}).Where("1 = 1").Update("retain_hours", 0).Error)
	orphanPath, _ := writeTorrent(t, filepath.Join(home, models.WorkDir, "downloads"), "springsunday-o.torrent", "retainzero")

	c := NewCleaner(home, db, defaultZap())
	res, err := c.Clean(context.Background(), CleanOptions{Categories: []CleanCategory{CategoryStaging}})
	require.NoError(t, err)
	require.Len(t, res.Categories, 1)
	assert.NotEmpty(t, res.Categories[0].Note)
	assert.FileExists(t, orphanPath, "禁用时不删")
}

// TestCleaner_DeleteFailureRecorded：删除失败（候选是目录）记录到 Skipped。
func TestCleaner_ApplyDeleteFailureRecorded(t *testing.T) {
	home, db := setupCleanerHome(t)
	logs := filepath.Join(home, models.WorkDir, "logs")
	// 制造一个"备份名"的目录：os.Remove 对非空目录会失败。
	badDir := filepath.Join(logs, "all-2020-01-01T00-00-00.000.log.gz")
	require.NoError(t, os.MkdirAll(filepath.Join(badDir, "child"), 0o755))
	ancient := time.Now().Add(-400 * 24 * time.Hour)
	require.NoError(t, os.Chtimes(badDir, ancient, ancient))

	cr := &CategoryResult{Category: CategoryLogs}
	c := NewCleaner(home, db, defaultZap())
	resolvedRoot, err := evalSymlinksExisting(logs)
	require.NoError(t, err)
	c.applyDelete(resolvedRoot, badDir, "test", false, cr)
	assert.NotEmpty(t, cr.Skipped, "删除失败应记录 Skipped")
	assert.Empty(t, cr.Deleted)
}

// TestShouldSweepStaging_NilDBAndBadHash：nil db 与非法文件返回 false。
func TestShouldSweepStaging_NilDBAndBadHash(t *testing.T) {
	assert.False(t, ShouldSweepStaging(nil, "/tmp/x.torrent", testSite, 24, 3))

	_, db := setupCleanerHome(t)
	bad := filepath.Join(t.TempDir(), "not-a-torrent.txt")
	require.NoError(t, os.WriteFile(bad, []byte("garbage"), 0o644))
	assert.False(t, ShouldSweepStaging(db, bad, testSite, 24, 3), "非法种子哈希失败应保守不删")
	assert.False(t, shouldSweepAnySite(db, bad, 24, 3))
	assert.False(t, shouldSweepAnySite(nil, bad, 24, 3))
}

// TestShouldSweepDecision_MaxRetry：达最大重试即扫。
func TestShouldSweepDecision_MaxRetry(t *testing.T) {
	ti := &models.TorrentInfo{RetryCount: 3}
	assert.True(t, shouldSweepDecision(ti, "/no/such/file", 24, 3), "达最大重试应扫")
	assert.False(t, shouldSweepDecision(ti, "/no/such/file", 24, 0), "maxRetry=0 且文件不存在应保守不删")
}

// TestIsRedLine：各红线 basename 命中，普通轮转备份不命中。
func TestIsRedLine(t *testing.T) {
	for _, n := range []string{models.DBFile, models.DBFile + "-wal", models.DBFile + "-shm", "secret.key", "all.log", "debug.log", "info.log", "error.log"} {
		assert.True(t, isRedLine("/x/"+n), n)
	}
	assert.False(t, isRedLine("/x/all-2020-01-01T00-00-00.000.log.gz"))
	assert.False(t, isRedLine("/x/backup.json"))
}

// TestFileSize：目录返回 0，普通文件返回真实大小。
func TestFileSize(t *testing.T) {
	dir := t.TempDir()
	assert.Equal(t, int64(0), fileSize(dir))
	f := filepath.Join(dir, "f")
	require.NoError(t, os.WriteFile(f, []byte("hello"), 0o644))
	assert.Equal(t, int64(5), fileSize(f))
	assert.Equal(t, int64(0), fileSize(filepath.Join(dir, "nope")))
}
