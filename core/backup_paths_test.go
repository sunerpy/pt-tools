package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/sunerpy/pt-tools/internal/backup"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/version"
)

// noHome 让 os.UserHomeDir 拿不到主目录（Unix 看 HOME，Windows 看 USERPROFILE）。
func noHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
}

// 拿不到主目录时，用到数据目录的都报错，不往别处写。
func TestBackup_NoHomeDir(t *testing.T) {
	s, _ := backupStore(t)
	noHome(t)
	ctx := context.Background()
	_, err := DataDir()
	require.Error(t, err)
	_, err = s.BackupSettings()
	assert.Error(t, err)
	_, err = s.RunBackup(ctx, backup.ReasonManual)
	assert.Error(t, err)
	_, _, err = s.ListBackups()
	assert.Error(t, err)
	_, err = s.BackupFile("pt-tools-backup-20261102-043000-manual.tar.gz")
	assert.Error(t, err)
	assert.Error(t, s.DeleteBackup("pt-tools-backup-20261102-043000-manual.tar.gz"))
	_, err = s.StageRestoreBackup("pt-tools-backup-20261102-043000-manual.tar.gz")
	assert.Error(t, err)
	_, err = StageRestore(filepath.Join(t.TempDir(), "x.tar.gz"), "x")
	assert.Error(t, err)
	_, err = PendingRestore()
	assert.Error(t, err)
	assert.Error(t, CancelRestore())
	_, err = BackupDataDir(ctx, "", nil)
	assert.Error(t, err)
	_, err = rawBackupTarget(s.db.DB)
	assert.Error(t, err)
	enablePendingRestoreForTest(t)
	_, err = applyPendingRestore()
	assert.Error(t, err)
}

// 读写设置时数据库出错都报出来（这里把连接关掉）。
func TestBackupSettings_DBErrors(t *testing.T) {
	s, _ := backupStore(t)
	sqlDB, err := s.db.DB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	_, err = s.BackupSettings()
	assert.ErrorContains(t, err, "读取备份设置失败")
	_, err = s.SaveBackupSettings(BackupSettingsInput{Enabled: true, Time: "04:30", Keep: 7, IncludeSecret: true})
	assert.ErrorContains(t, err, "保存备份设置失败")
	assert.ErrorContains(t, s.RecordBackupResult(backup.ReasonManual, "x", nil, time.Now()), "保存备份设置失败")
	assert.ErrorContains(t, s.RecordRestoreResult("x", "", nil, time.Now()), "保存备份设置失败")
	_, err = s.RunBackup(context.Background(), backup.ReasonManual)
	assert.Error(t, err)
	_, err = rawBackupTarget(s.db.DB)
	assert.Error(t, err)
}

// 表里的时刻为空、份数超出范围时按默认值读。
func TestBackupSettings_RowFallbacks(t *testing.T) {
	s, _ := backupStore(t)
	require.NoError(t, s.db.DB.Create(&models.BackupSetting{ID: 1, Enabled: true, Time: "", Keep: 0}).Error)
	got, err := s.BackupSettings()
	require.NoError(t, err)
	assert.Equal(t, backup.DefaultTime, got.Time)
	assert.Equal(t, backup.DefaultKeep, got.Keep)
	assert.False(t, got.IncludeSecret, "关着的开关照样读成关")
}

// 备份目录建不出来或不能写时不保存，原因写清楚。
func TestSaveBackupSettings_DirNotUsable(t *testing.T) {
	s, _ := backupStore(t)
	file := filepath.Join(t.TempDir(), "f")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	_, err := s.SaveBackupSettings(BackupSettingsInput{Enabled: true, Time: "04:30", Keep: 7, Dir: filepath.Join(file, "sub")})
	require.ErrorIs(t, err, ErrBackupSettingsInvalid)
	assert.ErrorContains(t, err, "建不了备份目录")
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		return // 只读的权限位在 Windows 与 root 下不起作用
	}
	ro := filepath.Join(t.TempDir(), "ro")
	require.NoError(t, os.Mkdir(ro, 0o500))
	t.Cleanup(func() { _ = os.Chmod(ro, 0o700) })
	_, err = s.SaveBackupSettings(BackupSettingsInput{Enabled: true, Time: "04:30", Keep: 7, Dir: ro})
	require.ErrorIs(t, err, ErrBackupSettingsInvalid)
	assert.ErrorContains(t, err, "不能写")
}

// secret.key 读不出来或是这次新生成的：带不进备份，恢复校验时也没有「当前生效的密钥」。
func TestBackupKey_Unusable(t *testing.T) {
	keyTestHome(t, []byte("not-a-key"))
	assert.ErrorContains(t, keyUsableForBackup(), "读不出来")
	assert.Nil(t, ActiveBackupKey().Key)
	_, err := CurrentBackupKey()
	assert.Error(t, err)

	keyTestHome(t, nil)
	assert.ErrorContains(t, keyUsableForBackup(), "原本不存在")
}

// 命令行备份：只读打开数据目录里的库，按设置放进备份目录；也能写到指定的位置、不带密钥；程序版本取库里记的上次启动的版本。
func TestBackupDataDir(t *testing.T) {
	home := keyTestHome(t, backupTestKey)
	ctx := context.Background()
	_, err := BackupDataDir(ctx, "", nil)
	assert.ErrorContains(t, err, "没有找到数据库")

	db := openDataDirDB(t, nil)
	addEncryptedCookie(t, db.DB, "hdsky", "uid=1")
	info, err := BackupDataDir(ctx, "", nil)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".pt-tools", "backups"), filepath.Dir(info.Path))
	assert.Equal(t, backup.ReasonCLI, info.Manifest.Reason)
	assert.True(t, info.Manifest.HasSecret)
	assert.Equal(t, version.Version, info.Manifest.AppVersion, "库里没记上次启动的版本时写这个程序的版本")

	require.NoError(t, models.WriteAppMeta(db.DB, models.AppMetaLastStartedVersion, "v0.48.0"))
	out, no := t.TempDir(), false
	info, err = BackupDataDir(ctx, out, &no)
	require.NoError(t, err)
	assert.Equal(t, out, filepath.Dir(info.Path))
	assert.False(t, info.Manifest.HasSecret)
	assert.Equal(t, "v0.48.0", info.Manifest.AppVersion)
}

// 这个进程的密钥是新生成的：命令行备份带不上密钥，提示加 --no-secret。
func TestBackupDataDir_KeyGenerated(t *testing.T) {
	keyTestHome(t, nil)
	openDataDirDB(t, nil)
	_, err := BackupDataDir(context.Background(), "", nil)
	assert.ErrorContains(t, err, "带不上密钥")
	assert.ErrorContains(t, err, "--no-secret")
}

// 升级前读备份设置：backup_settings 的列对不上时报错。
func TestRawBackupTarget_BadTable(t *testing.T) {
	keyTestHome(t, backupTestKey)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "x.db")), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	t.Cleanup(func() { CloseDB(&models.TorrentDB{DB: db}) })
	require.NoError(t, db.Exec("CREATE TABLE backup_settings (id INTEGER)").Error)
	_, err = rawBackupTarget(db)
	assert.ErrorContains(t, err, "读取备份设置失败")
}
