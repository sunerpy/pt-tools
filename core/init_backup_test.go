package core

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 迁移前的备份写到 ~/.pt-tools/backups，文件名带这一步迁移的版本（原来都写成 v8_to_v9）。
func TestBackupMigrationTable_NamesFileWithVersions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	db, err := OpenTestDB(t)
	require.NoError(t, err)

	path, err := backupMigrationTable(db.DB, "settings_globals", 15, 16)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".pt-tools", "backups"), filepath.Dir(path))
	assert.True(t, strings.HasPrefix(filepath.Base(path), "settings_globals_v15_to_v16_"), path)
}
