// MIT License
// Copyright (c) 2025 pt-tools

package maintenance

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeebo/bencode"

	"github.com/sunerpy/pt-tools/config"
	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

const testSite = models.SiteGroup("springsunday")

// setupCleanerHome 构造一个 fake home，内含 ~/.pt-tools/{logs,downloads,backups}，
// 返回 homeDir 与已迁移的 TorrentDB（DB 文件不在白名单三根内，且被红线保护）。
func setupCleanerHome(t *testing.T) (string, *models.TorrentDB) {
	t.Helper()
	home := t.TempDir()
	work := filepath.Join(home, models.WorkDir)
	for _, d := range []string{"logs", "downloads", "backups"} {
		require.NoError(t, os.MkdirAll(filepath.Join(work, d), 0o755))
	}
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	return home, db
}

// writeTorrent 在 dir 下写入一个内容唯一（基于 name）的 .torrent，返回路径与 info hash。
func writeTorrent(t *testing.T, dir, fileName, uniq string) (string, string) {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": uniq}}))
	p := filepath.Join(dir, fileName)
	require.NoError(t, os.WriteFile(p, buf.Bytes(), 0o644))
	h, err := qbit.ComputeTorrentHashWithPath(p)
	require.NoError(t, err)
	return p, h
}

func seedTorrent(t *testing.T, db *models.TorrentDB, hash string, pushed bool, retry int) {
	t.Helper()
	p := pushed
	ti := &models.TorrentInfo{
		SiteName:    string(testSite),
		TorrentID:   hash, // 唯一即可
		TorrentHash: &hash,
		IsPushed:    &p,
		RetryCount:  retry,
	}
	require.NoError(t, db.UpsertTorrent(ti))
}

func defaultZap() config.Zap {
	z := config.DefaultZapConfig
	z.MaxBackups = 3
	z.MaxAge = 30
	return z
}

func containsPath(actions []FileAction, path string) bool {
	for _, a := range actions {
		if a.Path == path {
			return true
		}
	}
	return false
}

// T-redline：红线文件在任何清理路径下都不得被删除。
func TestCleaner_RedLineNeverDeleted(t *testing.T) {
	home, db := setupCleanerHome(t)
	work := filepath.Join(home, models.WorkDir)

	redlines := []string{
		filepath.Join(work, models.DBFile),
		filepath.Join(work, models.DBFile+"-wal"),
		filepath.Join(work, models.DBFile+"-shm"),
		filepath.Join(work, "secret.key"),
		filepath.Join(work, "logs", "all.log"),
		filepath.Join(work, "logs", "debug.log"),
		filepath.Join(work, "logs", "info.log"),
		filepath.Join(work, "logs", "error.log"),
	}
	for _, p := range redlines {
		require.NoError(t, os.WriteFile(p, []byte("protected"), 0o644))
		old := time.Now().Add(-400 * 24 * time.Hour)
		require.NoError(t, os.Chtimes(p, old, old))
	}

	c := NewCleaner(home, db, defaultZap())
	res, err := c.Clean(context.Background(), CleanOptions{DryRun: false})
	require.NoError(t, err)

	for _, p := range redlines {
		_, statErr := os.Stat(p)
		assert.NoError(t, statErr, "红线文件必须存活: %s", p)
	}
	// 断言红线绝不出现在任何 Deleted 列表。
	for _, cr := range res.Categories {
		for _, p := range redlines {
			assert.False(t, containsPath(cr.Deleted, p), "红线不应出现在删除列表: %s", p)
		}
	}
}

// T-logs-base-preserved：logs 清理保留 base，删除超龄轮转备份。
func TestCleaner_LogsBasePreserved(t *testing.T) {
	home, db := setupCleanerHome(t)
	logs := filepath.Join(home, models.WorkDir, "logs")

	base := filepath.Join(logs, "all.log")
	require.NoError(t, os.WriteFile(base, []byte("base"), 0o644))
	oldBackup := filepath.Join(logs, "all-2020-01-01T00-00-00.000.log.gz")
	require.NoError(t, os.WriteFile(oldBackup, []byte("old"), 0o644))
	ancient := time.Now().Add(-400 * 24 * time.Hour)
	require.NoError(t, os.Chtimes(oldBackup, ancient, ancient))

	c := NewCleaner(home, db, defaultZap())
	res, err := c.Clean(context.Background(), CleanOptions{Categories: []CleanCategory{CategoryLogs}, DryRun: false})
	require.NoError(t, err)

	assert.FileExists(t, base, "base 日志必须保留")
	assert.NoFileExists(t, oldBackup, "超龄轮转备份应被删除")
	require.Len(t, res.Categories, 1)
	assert.True(t, containsPath(res.Categories[0].Deleted, oldBackup))
}

// T-staging-rule：downloads/{tag}/ 内 .torrent 按 #450 语义删除/保留。
func TestCleaner_StagingRule(t *testing.T) {
	home, db := setupCleanerHome(t)
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), RetainHours: 24, MaxRetry: 3,
	}))
	tag := filepath.Join(home, models.WorkDir, "downloads", "mytag")
	require.NoError(t, os.MkdirAll(tag, 0o755))

	pushedPath, pushedHash := writeTorrent(t, tag, "springsunday-pushed.torrent", "pushed")
	seedTorrent(t, db, pushedHash, true, 0) // 已推送 → 删

	orphanPath, _ := writeTorrent(t, tag, "springsunday-orphan.torrent", "orphan") // 无 DB → 删

	agedPath, agedHash := writeTorrent(t, tag, "springsunday-aged.torrent", "aged")
	seedTorrent(t, db, agedHash, false, 0)
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(agedPath, old, old)) // 超保留期未推送 → 删

	freshPath, freshHash := writeTorrent(t, tag, "springsunday-fresh.torrent", "fresh")
	seedTorrent(t, db, freshHash, false, 0) // 新鲜未推送 → 保留

	c := NewCleaner(home, db, defaultZap())
	_, err := c.Clean(context.Background(), CleanOptions{Categories: []CleanCategory{CategoryStaging}, DryRun: false})
	require.NoError(t, err)

	assert.NoFileExists(t, pushedPath, "已推送应删")
	assert.NoFileExists(t, orphanPath, "孤立应删")
	assert.NoFileExists(t, agedPath, "超保留期未推送应删")
	assert.FileExists(t, freshPath, "新鲜未推送应保留")
}

// T-backups-keepN：backups 保留最近 N 份。
func TestCleaner_BackupsKeepN(t *testing.T) {
	home, db := setupCleanerHome(t)
	backups := filepath.Join(home, models.WorkDir, "backups")

	paths := make([]string, 8)
	for i := 0; i < 8; i++ {
		p := filepath.Join(backups, "backup-"+string(rune('a'+i))+".json")
		require.NoError(t, os.WriteFile(p, []byte("b"), 0o644))
		// mtime 递增：i 越大越新。
		mt := time.Now().Add(-time.Duration(8-i) * time.Hour)
		require.NoError(t, os.Chtimes(p, mt, mt))
		paths[i] = p
	}

	c := NewCleaner(home, db, defaultZap())
	_, err := c.Clean(context.Background(), CleanOptions{Categories: []CleanCategory{CategoryBackups}, KeepBackups: 5, DryRun: false})
	require.NoError(t, err)

	// 最新 5 份（索引 3..7）保留，最旧 3 份（索引 0..2）删除。
	for i := 0; i < 3; i++ {
		assert.NoFileExists(t, paths[i], "最旧 3 份应删: %s", paths[i])
	}
	for i := 3; i < 8; i++ {
		assert.FileExists(t, paths[i], "最新 5 份应保留: %s", paths[i])
	}
}

// T-dryrun：DryRun 不实际删除，但列出将删项。
func TestCleaner_DryRunDeletesNothing(t *testing.T) {
	home, db := setupCleanerHome(t)
	logs := filepath.Join(home, models.WorkDir, "logs")
	oldBackup := filepath.Join(logs, "all-2020-01-01T00-00-00.000.log.gz")
	require.NoError(t, os.WriteFile(oldBackup, []byte("old"), 0o644))
	ancient := time.Now().Add(-400 * 24 * time.Hour)
	require.NoError(t, os.Chtimes(oldBackup, ancient, ancient))

	c := NewCleaner(home, db, defaultZap())
	res, err := c.Clean(context.Background(), CleanOptions{Categories: []CleanCategory{CategoryLogs}, DryRun: true})
	require.NoError(t, err)

	assert.FileExists(t, oldBackup, "dry-run 不应实际删除")
	assert.True(t, res.DryRun)
	require.Len(t, res.Categories, 1)
	assert.True(t, containsPath(res.Categories[0].Deleted, oldBackup), "应列出将删项")
	assert.Greater(t, res.Categories[0].FreedBytes, int64(0))
}

// T-escape-file-symlink：downloads 内的软链指向外部文件 → 拒绝，外部文件存活。
func TestCleaner_EscapeFileSymlink(t *testing.T) {
	home, db := setupCleanerHome(t)
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), RetainHours: 24, MaxRetry: 3,
	}))
	downloads := filepath.Join(home, models.WorkDir, "downloads")

	external := t.TempDir()
	extFile := filepath.Join(external, "external.torrent")
	var buf bytes.Buffer
	require.NoError(t, bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": "ext"}}))
	require.NoError(t, os.WriteFile(extFile, buf.Bytes(), 0o644))
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(extFile, old, old))

	link := filepath.Join(downloads, "springsunday-link.torrent")
	require.NoError(t, os.Symlink(extFile, link))

	c := NewCleaner(home, db, defaultZap())
	res, err := c.Clean(context.Background(), CleanOptions{Categories: []CleanCategory{CategoryStaging}, DryRun: false})
	require.NoError(t, err)

	assert.FileExists(t, extFile, "软链指向的外部文件必须存活")
	require.Len(t, res.Categories, 1)
	assert.NotEmpty(t, res.Categories[0].Skipped, "越界软链应被记录到 Skipped")
}

// T-escape-root-symlink：downloads 整个是指向外部盘的软链 → 整类拒绝，外部文件全存活。
func TestCleaner_EscapeRootSymlink(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, models.WorkDir)
	require.NoError(t, os.MkdirAll(filepath.Join(work, "logs"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(work, "backups"), 0o755))
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), RetainHours: 24, MaxRetry: 3,
	}))

	// downloads 本身是指向外部目录的软链。
	external := t.TempDir()
	extFile := filepath.Join(external, "springsunday-ext.torrent")
	var buf bytes.Buffer
	require.NoError(t, bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": "extroot"}}))
	require.NoError(t, os.WriteFile(extFile, buf.Bytes(), 0o644))
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(extFile, old, old))
	require.NoError(t, os.Symlink(external, filepath.Join(work, "downloads")))

	c := NewCleaner(home, db, defaultZap())
	res, err := c.Clean(context.Background(), CleanOptions{Categories: []CleanCategory{CategoryStaging}, DryRun: false})
	require.NoError(t, err)

	assert.FileExists(t, extFile, "整根软链越界时外部文件必须全部存活")
	require.Len(t, res.Categories, 1)
	assert.NotEmpty(t, res.Categories[0].Note, "整类应被拒绝并记录 Note")
	assert.Empty(t, res.Categories[0].Deleted, "整类拒绝时不应有任何删除")
}

// T-dotdot：resolvedRoot 之外的绝对/`..` 候选被 withinRoot 拒绝。
func TestCleaner_DotDotRejected(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	assert.False(t, withinRoot(root, filepath.Join(outside, "x")), "root 外路径应被拒绝")
	assert.True(t, withinRoot(root, filepath.Join(root, "sub", "y")), "root 内路径应通过")
	assert.True(t, withinRoot(root, root), "root 自身应通过")

	// candidateInRoot 对越界候选返回 false。
	ok, why := candidateInRoot(root, filepath.Join(outside, "z.torrent"))
	assert.False(t, ok)
	assert.NotEmpty(t, why)
}

// T-categories-filter：只清理指定类别，其余类别文件不受影响。
func TestCleaner_CategoriesFilter(t *testing.T) {
	home, db := setupCleanerHome(t)
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), RetainHours: 24, MaxRetry: 3,
	}))
	work := filepath.Join(home, models.WorkDir)

	// logs 有可删备份。
	oldBackup := filepath.Join(work, "logs", "all-2020-01-01T00-00-00.000.log.gz")
	require.NoError(t, os.WriteFile(oldBackup, []byte("old"), 0o644))
	ancient := time.Now().Add(-400 * 24 * time.Hour)
	require.NoError(t, os.Chtimes(oldBackup, ancient, ancient))

	// downloads 有可删孤立种子。
	orphanPath, _ := writeTorrent(t, filepath.Join(work, "downloads"), "springsunday-orphan.torrent", "orphanfilter")

	// backups 有超额备份。
	extraBackup := filepath.Join(work, "backups", "old.json")
	require.NoError(t, os.WriteFile(extraBackup, []byte("b"), 0o644))
	require.NoError(t, os.Chtimes(extraBackup, ancient, ancient))

	c := NewCleaner(home, db, defaultZap())
	res, err := c.Clean(context.Background(), CleanOptions{Categories: []CleanCategory{CategoryLogs}, KeepBackups: 0, DryRun: false})
	require.NoError(t, err)

	assert.NoFileExists(t, oldBackup, "logs 类别应被清理")
	assert.FileExists(t, orphanPath, "未选中的 staging 不应被触碰")
	assert.FileExists(t, extraBackup, "未选中的 backups 不应被触碰")
	require.Len(t, res.Categories, 1)
	assert.Equal(t, CategoryLogs, res.Categories[0].Category)
}

// TestShouldSweepStaging_MirrorsIssue450 验证提取的判定与 #450 语义一致。
func TestShouldSweepStaging_MirrorsIssue450(t *testing.T) {
	_, db := setupCleanerHome(t)
	dir := t.TempDir()

	pushedPath, pushedHash := writeTorrent(t, dir, "springsunday-p.torrent", "swp")
	seedTorrent(t, db, pushedHash, true, 0)
	assert.True(t, ShouldSweepStaging(db, pushedPath, testSite, 24, 3), "已推送应扫")

	orphanPath, _ := writeTorrent(t, dir, "springsunday-o.torrent", "swo")
	assert.True(t, ShouldSweepStaging(db, orphanPath, testSite, 24, 3), "孤立应扫")

	freshPath, freshHash := writeTorrent(t, dir, "springsunday-f.torrent", "swf")
	seedTorrent(t, db, freshHash, false, 0)
	assert.False(t, ShouldSweepStaging(db, freshPath, testSite, 24, 3), "新鲜未推送应保留")
}
