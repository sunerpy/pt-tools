package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/config"
	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/models"
)

// setupCleanHome 构造一个 fake home，内含 ~/.pt-tools/{logs,downloads,backups}，
// 播种可删除文件（超龄轮转日志 + 多份旧备份）与红线文件（torrents.db/secret.key/base 日志）。
// 返回 homeDir、迁移后的 TorrentDB 与用于断言的关键路径。
func setupCleanHome(t *testing.T) (home string, db *models.TorrentDB, oldLog, oldBackup, baseLog, redlineDB, redlineKey string) {
	t.Helper()
	home = t.TempDir()
	work := filepath.Join(home, models.WorkDir)
	for _, d := range []string{"logs", "downloads", "backups"} {
		require.NoError(t, os.MkdirAll(filepath.Join(work, d), 0o755))
	}

	logs := filepath.Join(work, "logs")
	baseLog = filepath.Join(logs, "all.log")
	require.NoError(t, os.WriteFile(baseLog, []byte("base"), 0o644))
	oldLog = filepath.Join(logs, "all-2020-01-01T00-00-00.000.log.gz")
	require.NoError(t, os.WriteFile(oldLog, []byte("old-rotated"), 0o644))
	ancient := time.Now().Add(-400 * 24 * time.Hour)
	require.NoError(t, os.Chtimes(oldLog, ancient, ancient))

	backups := filepath.Join(work, "backups")
	for i := 0; i < 8; i++ {
		p := filepath.Join(backups, "backup-"+string(rune('a'+i))+".json")
		require.NoError(t, os.WriteFile(p, []byte("b"), 0o644))
		mt := time.Now().Add(-time.Duration(8-i) * time.Hour)
		require.NoError(t, os.Chtimes(p, mt, mt))
	}
	// 最旧一份将被删除（KeepBackups=5，共 8 份 → 删 3 份）。
	oldBackup = filepath.Join(backups, "backup-a.json")

	redlineDB = filepath.Join(work, models.DBFile)
	require.NoError(t, os.WriteFile(redlineDB, []byte("db"), 0o644))
	redlineKey = filepath.Join(work, "secret.key")
	require.NoError(t, os.WriteFile(redlineKey, []byte("key"), 0o644))

	var err error
	db, err = core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	return home, db, oldLog, oldBackup, baseLog, redlineDB, redlineKey
}

// TestClean_DryRunByDefault：默认（dryRun=true, confirm=false）只预览，不删除任何文件。
func TestClean_DryRunByDefault(t *testing.T) {
	home, db, oldLog, oldBackup, baseLog, redlineDB, redlineKey := setupCleanHome(t)

	var buf bytes.Buffer
	err := executeClean(&buf, home, db, config.DefaultZapConfig, nil, true, false, 5)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "预览")

	// 默认预览不得删除任何文件。
	assert.FileExists(t, oldLog, "预览模式下不得删除轮转日志")
	assert.FileExists(t, oldBackup, "预览模式下不得删除旧备份")
	assert.FileExists(t, baseLog)
	assert.FileExists(t, redlineDB)
	assert.FileExists(t, redlineKey)
}

// TestClean_RequiresConfirmForRealDelete：--dry-run=false 且未 --confirm → 拒绝并返回错误，且不删除。
func TestClean_RequiresConfirmForRealDelete(t *testing.T) {
	home, db, oldLog, oldBackup, _, redlineDB, redlineKey := setupCleanHome(t)

	var buf bytes.Buffer
	err := executeClean(&buf, home, db, config.DefaultZapConfig, nil, false, false, 5)
	require.Error(t, err, "破坏性删除缺少 --confirm 必须报错")
	assert.Contains(t, buf.String(), "--confirm")

	// 拒绝后不得删除任何文件。
	assert.FileExists(t, oldLog, "拒绝时不得删除")
	assert.FileExists(t, oldBackup, "拒绝时不得删除")
	assert.FileExists(t, redlineDB)
	assert.FileExists(t, redlineKey)
}

// TestClean_ConfirmDeletes：--confirm 触发真实删除，可删文件被移除，红线文件仍在。
func TestClean_ConfirmDeletes(t *testing.T) {
	home, db, oldLog, oldBackup, baseLog, redlineDB, redlineKey := setupCleanHome(t)

	var buf bytes.Buffer
	// dryRunFlag 保持默认 true，confirm=true → 有效 dryRun = !confirm = false（真实删除）。
	err := executeClean(&buf, home, db, config.DefaultZapConfig, nil, true, true, 5)
	require.NoError(t, err)

	assert.NoFileExists(t, oldLog, "确认后超龄轮转日志应被删除")
	assert.NoFileExists(t, oldBackup, "确认后最旧备份应被删除")

	// 红线与 base 日志必须存活。
	assert.FileExists(t, baseLog, "base 日志必须保留")
	assert.FileExists(t, redlineDB, "torrents.db 红线必须保留")
	assert.FileExists(t, redlineKey, "secret.key 红线必须保留")
}

// TestClean_CategoryFilter：--category logs 只清理 logs，不触碰 backups。
func TestClean_CategoryFilter(t *testing.T) {
	home, db, oldLog, oldBackup, _, _, _ := setupCleanHome(t)

	var buf bytes.Buffer
	err := executeClean(&buf, home, db, config.DefaultZapConfig, []string{"logs"}, true, true, 5)
	require.NoError(t, err)

	assert.NoFileExists(t, oldLog, "logs 类别应被清理")
	assert.FileExists(t, oldBackup, "未指定 backups，旧备份必须保留")
}

// TestClean_InvalidCategory：非法类别应返回错误。
func TestClean_InvalidCategory(t *testing.T) {
	home, db, _, _, _, _, _ := setupCleanHome(t)

	var buf bytes.Buffer
	err := executeClean(&buf, home, db, config.DefaultZapConfig, []string{"nope"}, true, true, 5)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nope")
}

// TestClean_Help：帮助/用法输出列出所有旗标。
func TestClean_Help(t *testing.T) {
	out := cleanCmd.UsageString()
	assert.Contains(t, out, "--dry-run")
	assert.Contains(t, out, "--confirm")
	assert.Contains(t, out, "--category")
	assert.Contains(t, out, "--keep-backups")
}

// TestClean_RegisteredAsTopLevel：clean 注册为顶级命令且默认 dry-run=true。
func TestClean_RegisteredAsTopLevel(t *testing.T) {
	var found *cobra.Command
	for _, c := range rootCmd.Commands() {
		if c.Name() == "clean" {
			found = c
			break
		}
	}
	require.NotNil(t, found, "clean 必须注册为顶级命令")

	dr, err := found.Flags().GetBool("dry-run")
	require.NoError(t, err)
	assert.True(t, dr, "--dry-run 默认必须为 true（安全默认）")
}
