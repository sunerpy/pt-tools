package models

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 升级前备份挂在 beforeMigrate 上：它要在 WAL 设好之后、AutoMigrate 之前被调用，看到的是迁移以前的库。
func TestNewDB_BeforeMigrateRunsBeforeAutoMigrate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	calls := 0
	var sawTorrents, sawNewTable bool
	var mode string
	hook := func(db *gorm.DB) error {
		calls++
		var err error
		sawTorrents, err = TableExists(db, "torrent_infos")
		require.NoError(t, err)
		sawNewTable, err = TableExists(db, "backup_settings")
		require.NoError(t, err)
		require.NoError(t, db.Raw("PRAGMA journal_mode").Scan(&mode).Error)
		return nil
	}
	tdb, err := NewDBWithVersionAndHooks(zapNopGormLogger(), "1.0.0", nil, nil, nil, hook)
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.False(t, sawTorrents, "全新的库：钩子在 AutoMigrate 之前，还没有业务表")
	assert.False(t, sawNewTable)
	assert.Equal(t, "wal", mode)
	ok, err := TableExists(tdb.DB, "backup_settings")
	require.NoError(t, err)
	assert.True(t, ok, "AutoMigrate 建了备份设置表")
	closeTestGorm(tdb.DB)

	// 第二次打开：钩子看到上一次建好的业务表
	tdb, err = NewDBWithVersionAndHooks(zapNopGormLogger(), "1.0.1", nil, nil, nil, hook)
	require.NoError(t, err)
	t.Cleanup(func() { closeTestGorm(tdb.DB) })
	assert.Equal(t, 2, calls)
	assert.True(t, sawTorrents)
}

// 钩子出错（升级前备份没做成）不阻止打开数据库。
func TestNewDB_BeforeMigrateErrorDoesNotBlock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	tdb, err := NewDBWithVersionAndHooks(zapNopGormLogger(), "1.0.0", nil, nil, nil, func(*gorm.DB) error {
		return errors.New("disk full")
	})
	require.NoError(t, err)
	t.Cleanup(func() { closeTestGorm(tdb.DB) })
	assert.True(t, tdb.DB.Migrator().HasTable(&TorrentInfo{}))
}

func TestAppMeta_ReadWrite(t *testing.T) {
	db := newMemDB(t)
	got, err := ReadAppMeta(db, AppMetaLastStartedVersion)
	require.NoError(t, err)
	assert.Empty(t, got, "表还不存在时当作没有")

	require.NoError(t, db.AutoMigrate(&AppMeta{}))
	got, err = ReadAppMeta(db, AppMetaLastStartedVersion)
	require.NoError(t, err)
	assert.Empty(t, got)

	require.NoError(t, WriteAppMeta(db, AppMetaLastStartedVersion, "v1.0.0-rc.52"))
	require.NoError(t, WriteAppMeta(db, AppMetaLastStartedVersion, "v1.0.0-rc.53"))
	got, err = ReadAppMeta(db, AppMetaLastStartedVersion)
	require.NoError(t, err)
	assert.Equal(t, "v1.0.0-rc.53", got)
	var n int64
	require.NoError(t, db.Model(&AppMeta{}).Count(&n).Error)
	assert.Equal(t, int64(1), n, "同一项只有一行")
	assert.Error(t, WriteAppMeta(nil, "k", "v"))
}

// 加密列清单里的每一列都真的在库里：表或列改了名，启动检查与备份校验会悄悄跳过它们。
func TestEncryptedColumnsExistInSchema(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	tdb, err := NewDB(zapNopGormLogger())
	require.NoError(t, err)
	t.Cleanup(func() { closeTestGorm(tdb.DB) })
	require.NotEmpty(t, EncryptedColumns)
	for _, c := range EncryptedColumns {
		assert.True(t, tdb.DB.Migrator().HasColumn(c.Table, c.Column), "%s.%s", c.Table, c.Column)
	}
}
