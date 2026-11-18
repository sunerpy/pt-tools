package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"moul.io/zapgorm2"

	"github.com/sunerpy/pt-tools/internal/backup"
	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/version"
)

var backupTestKey = []byte(strings.Repeat("11", 32))

// backupStore 是备份测试用的 ConfigStore：HOME 指到临时目录，secret.key 是固定的一把，库在 t.TempDir() 里。
func backupStore(t *testing.T) (*ConfigStore, string) {
	t.Helper()
	home := keyTestHome(t, backupTestKey)
	db, err := OpenTestDB(t)
	require.NoError(t, err)
	return NewConfigStore(db), home
}

// openDataDirDB 按正式的方式（WAL、迁移前钩子）打开 ~/.pt-tools/torrents.db。
func openDataDirDB(t *testing.T, hook func(*gorm.DB) error) *models.TorrentDB {
	t.Helper()
	db, err := models.NewDBWithVersionAndHooks(zapgorm2.Logger{ZapLogger: zap.NewNop()}, version.Version, nil, nil, nil, hook)
	require.NoError(t, err)
	t.Cleanup(func() { CloseDB(db) })
	return db
}

// resetPreUpgrade 让每个测试从「这次启动没做过升级前备份」开始，结束时也清掉。
func resetPreUpgrade(t *testing.T) {
	t.Helper()
	preUpgrade.ran, preUpgrade.at, preUpgrade.name, preUpgrade.err = false, time.Time{}, "", nil
	t.Cleanup(func() { preUpgrade.ran, preUpgrade.at, preUpgrade.name, preUpgrade.err = false, time.Time{}, "", nil })
}

func enablePendingRestoreForTest(t *testing.T) {
	t.Helper()
	saved := pendingRestoreEnabled
	pendingRestoreEnabled = true
	t.Cleanup(func() { pendingRestoreEnabled = saved })
}

func addEncryptedCookie(t *testing.T, db *gorm.DB, site, cookie string) {
	t.Helper()
	enc, err := crypto.Encrypt([]byte(cookie))
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.SiteSetting{Name: site, AuthMethod: "cookie", CookieEncrypted: enc}).Error)
}

// 没有设置这一行时按默认：开着、04:30、7 份、默认目录、带上密钥；只读不建行。
func TestBackupSettings_Defaults(t *testing.T) {
	s, home := backupStore(t)
	got, err := s.BackupSettings()
	require.NoError(t, err)
	assert.True(t, got.Enabled)
	assert.Equal(t, "04:30", got.Time)
	assert.Equal(t, 7, got.Keep)
	assert.Equal(t, "", got.Dir)
	assert.Equal(t, filepath.Join(home, ".pt-tools", "backups"), got.DefaultDir)
	assert.Equal(t, got.DefaultDir, got.BackupDir())
	assert.True(t, got.IncludeSecret)
	assert.Nil(t, got.Last)
	assert.Nil(t, got.Restore)
	var n int64
	require.NoError(t, s.db.DB.Model(&models.BackupSetting{}).Count(&n).Error)
	assert.Zero(t, n)
}

func TestSaveBackupSettings_Invalid(t *testing.T) {
	s, _ := backupStore(t)
	file := filepath.Join(t.TempDir(), "a-file")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	base := BackupSettingsInput{Enabled: true, Time: "04:30", Keep: 7}
	cases := map[string]struct {
		edit func(*BackupSettingsInput)
		want string
	}{
		"时刻少一位":   {func(in *BackupSettingsInput) { in.Time = "4:30" }, "HH:MM"},
		"时刻超出":    {func(in *BackupSettingsInput) { in.Time = "24:00" }, "HH:MM"},
		"时刻不是数字":  {func(in *BackupSettingsInput) { in.Time = "04:3x" }, "HH:MM"},
		"保留 0 份":  {func(in *BackupSettingsInput) { in.Keep = 0 }, "保留份数"},
		"保留 61 份": {func(in *BackupSettingsInput) { in.Keep = 61 }, "保留份数"},
		"相对路径":    {func(in *BackupSettingsInput) { in.Dir = "backups" }, "绝对路径"},
		"建不了目录":   {func(in *BackupSettingsInput) { in.Dir = filepath.Join(file, "sub") }, "建不了备份目录"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			in := base
			c.edit(&in)
			_, err := s.SaveBackupSettings(in)
			require.ErrorIs(t, err, ErrBackupSettingsInvalid)
			assert.Contains(t, err.Error(), c.want)
		})
	}
	var n int64
	require.NoError(t, s.db.DB.Model(&models.BackupSetting{}).Count(&n).Error)
	assert.Zero(t, n, "校验不过的不写库")
}

// 保存只改设置这几列：关掉、换时刻、换目录以后，最近一次备份的结果还在；目录清空回到默认目录。
func TestSaveBackupSettings_RoundTrip(t *testing.T) {
	s, _ := backupStore(t)
	at := time.Date(2026, 11, 2, 4, 30, 0, 0, time.Local)
	require.NoError(t, s.RecordBackupResult(backup.ReasonScheduled, "a.tar.gz", nil, at))

	root := t.TempDir()
	got, err := s.SaveBackupSettings(BackupSettingsInput{
		Enabled: false, Time: " 23:05 ", Keep: 3, Dir: filepath.Join(root, "x", "..", "bk"), IncludeSecret: false,
	})
	require.NoError(t, err)
	assert.False(t, got.Enabled)
	assert.Equal(t, "23:05", got.Time)
	assert.Equal(t, 3, got.Keep)
	assert.Equal(t, filepath.Join(root, "bk"), got.Dir)
	assert.Equal(t, filepath.Join(root, "bk"), got.BackupDir())
	assert.False(t, got.IncludeSecret)
	require.NotNil(t, got.Last, "保存设置不清掉最近一次备份的结果")
	assert.Equal(t, "a.tar.gz", got.Last.Name)
	assert.Equal(t, "2026-11-02", got.ScheduledDay)
	entries, err := os.ReadDir(filepath.Join(root, "bk"))
	require.NoError(t, err)
	assert.Empty(t, entries, "试写的文件删掉了")

	got, err = s.SaveBackupSettings(BackupSettingsInput{Enabled: true, Time: "04:30", Keep: 7, Dir: "  ", IncludeSecret: true})
	require.NoError(t, err)
	assert.Equal(t, "", got.Dir)
	assert.Equal(t, got.DefaultDir, got.BackupDir())
	assert.True(t, got.Enabled)
	assert.True(t, got.IncludeSecret)
}

// 成功时记下上次成功的时间；失败时只记原因，上次成功的时间不变。定时备份记下做过的日期，手动的不记。
func TestRecordBackupResult(t *testing.T) {
	s, _ := backupStore(t)
	at1 := time.Date(2026, 11, 2, 4, 30, 0, 0, time.Local)
	require.NoError(t, s.RecordBackupResult(backup.ReasonScheduled, "a.tar.gz", nil, at1))
	got, err := s.BackupSettings()
	require.NoError(t, err)
	require.NotNil(t, got.Last)
	assert.Equal(t, backup.ReasonScheduled, got.Last.Reason)
	assert.Equal(t, "a.tar.gz", got.Last.Name)
	assert.Empty(t, got.Last.Error)
	assert.True(t, got.Last.At.Equal(at1))
	require.NotNil(t, got.Last.LastOKAt)
	assert.True(t, got.Last.LastOKAt.Equal(at1))
	assert.Equal(t, "2026-11-02", got.ScheduledDay)
	assert.True(t, got.Enabled, "建出来的这一行是默认设置")
	assert.Equal(t, 7, got.Keep)

	at2 := at1.Add(26 * time.Hour)
	require.NoError(t, s.RecordBackupResult(backup.ReasonManual, "", errors.New("磁盘满了"), at2))
	got, err = s.BackupSettings()
	require.NoError(t, err)
	assert.Equal(t, backup.ReasonManual, got.Last.Reason)
	assert.Equal(t, "磁盘满了", got.Last.Error)
	assert.True(t, got.Last.At.Equal(at2))
	assert.True(t, got.Last.LastOKAt.Equal(at1), "上次成功的时间不变")
	assert.Equal(t, "2026-11-02", got.ScheduledDay, "手动备份不改定时备份做过的日期")

	require.NoError(t, s.RecordBackupResult(backup.ReasonScheduled, "", errors.New("x"), at2))
	got, err = s.BackupSettings()
	require.NoError(t, err)
	assert.Equal(t, "2026-11-03", got.ScheduledDay, "定时备份失败了也算这天做过（失败另发通知）")
}

func TestRecordRestoreResult(t *testing.T) {
	s, _ := backupStore(t)
	at := time.Date(2026, 11, 2, 9, 0, 0, 0, time.Local)
	require.NoError(t, s.RecordRestoreResult("a.tar.gz", "/data/backups/pre-restore-20261102-090000", nil, at))
	got, err := s.BackupSettings()
	require.NoError(t, err)
	require.NotNil(t, got.Restore)
	assert.Equal(t, "a.tar.gz", got.Restore.Name)
	assert.Equal(t, "/data/backups/pre-restore-20261102-090000", got.Restore.Dir)
	assert.Empty(t, got.Restore.Error)
	assert.True(t, got.Restore.At.Equal(at))

	require.NoError(t, s.RecordRestoreResult("b.tar.gz", "/data/backups/pre-restore-x", errors.New("密钥解不开"), at.Add(time.Hour)))
	got, err = s.BackupSettings()
	require.NoError(t, err)
	assert.Equal(t, "b.tar.gz", got.Restore.Name)
	assert.Equal(t, "密钥解不开", got.Restore.Error)
	assert.Empty(t, got.Restore.Dir, "没换上时文件已经放回原位，不记目录")
	assert.Nil(t, got.Last)
}

// 按设置做一份备份：带上密钥、记下结果、能列出来、能下载与删除，现在的密钥能直接恢复它。
func TestRunBackup(t *testing.T) {
	s, _ := backupStore(t)
	addEncryptedCookie(t, s.db.DB, "hdsky", "uid=1")
	info, err := s.RunBackup(context.Background(), backup.ReasonManual)
	require.NoError(t, err)
	assert.True(t, backup.IsBackupName(info.Name))
	assert.Equal(t, backup.ReasonManual, info.Manifest.Reason)
	assert.Equal(t, version.Version, info.Manifest.AppVersion)
	assert.True(t, info.Manifest.HasSecret)
	assert.Equal(t, backup.SecretFromFile, info.Manifest.SecretSource)

	got, err := s.BackupSettings()
	require.NoError(t, err)
	require.NotNil(t, got.Last)
	assert.Equal(t, info.Name, got.Last.Name)
	assert.Empty(t, got.Last.Error)

	list, dir, err := s.ListBackups()
	require.NoError(t, err)
	assert.Equal(t, got.DefaultDir, dir)
	require.Len(t, list, 1)
	assert.Equal(t, info.Name, list[0].Name)

	p, err := s.BackupFile(info.Name)
	require.NoError(t, err)
	plan, err := backup.Verify(p, t.TempDir(), ActiveBackupKey())
	require.NoError(t, err)
	assert.Equal(t, backup.KeyKeepFile, plan.Key)
	assert.False(t, plan.NoCipher)

	_, err = s.BackupFile("../torrents.db")
	assert.ErrorIs(t, err, backup.ErrNotFound)
	require.NoError(t, s.DeleteBackup(info.Name))
	assert.ErrorIs(t, s.DeleteBackup(info.Name), backup.ErrNotFound)
}

// 不带密钥的设置做出来的备份里没有密钥；定时备份按保留份数删旧的，手动的不删。
func TestRunBackup_NoSecretAndPrune(t *testing.T) {
	s, _ := backupStore(t)
	_, err := s.SaveBackupSettings(BackupSettingsInput{Enabled: true, Time: "04:30", Keep: 2, IncludeSecret: false})
	require.NoError(t, err)
	manual, err := s.RunBackup(context.Background(), backup.ReasonManual)
	require.NoError(t, err)
	assert.False(t, manual.Manifest.HasSecret)
	for range 3 {
		_, err = s.RunBackup(context.Background(), backup.ReasonScheduled)
		require.NoError(t, err)
	}
	list, _, err := s.ListBackups()
	require.NoError(t, err)
	count := map[backup.Reason]int{}
	for _, b := range list {
		count[b.Manifest.Reason]++
	}
	assert.Equal(t, 2, count[backup.ReasonScheduled])
	assert.Equal(t, 1, count[backup.ReasonManual])
}

// 备份目录写不了：返回错误并记下失败原因。
func TestRunBackup_Failure(t *testing.T) {
	s, _ := backupStore(t)
	dir := filepath.Join(t.TempDir(), "bk")
	_, err := s.SaveBackupSettings(BackupSettingsInput{Enabled: true, Time: "04:30", Keep: 7, Dir: dir, IncludeSecret: true})
	require.NoError(t, err)
	// 保存以后目录被换成了一个文件
	require.NoError(t, os.RemoveAll(dir))
	require.NoError(t, os.WriteFile(dir, []byte("x"), 0o600))

	_, err = s.RunBackup(context.Background(), backup.ReasonScheduled)
	require.Error(t, err)
	got, err := s.BackupSettings()
	require.NoError(t, err)
	require.NotNil(t, got.Last)
	assert.NotEmpty(t, got.Last.Error)
	assert.Nil(t, got.Last.LastOKAt)
	assert.NotEmpty(t, got.ScheduledDay, "定时备份失败也记下这一天，不在同一天里反复重试")
}

// 升级前备份：全新的库不做；上次启动的版本和这次一样时不做；不一样（或没有记录）时在迁移以前做一份。
func TestPreUpgradeBackup(t *testing.T) {
	home := keyTestHome(t, backupTestKey)
	resetPreUpgrade(t)
	backups := filepath.Join(home, ".pt-tools", "backups")

	// 全新安装：打开时还没有 torrent_infos
	db := openDataDirDB(t, preUpgradeBackup)
	assert.False(t, preUpgrade.ran)
	_, err := os.Stat(backups)
	assert.True(t, os.IsNotExist(err), "全新的库不备份")
	addEncryptedCookie(t, db.DB, "hdsky", "uid=1")
	CloseDB(db)

	// 没有记录上次启动的版本（从不记这一项的旧版本升上来）
	db = openDataDirDB(t, preUpgradeBackup)
	require.True(t, preUpgrade.ran)
	require.NoError(t, preUpgrade.err)
	list, err := backup.List(backups)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, preUpgrade.name, list[0].Name)
	assert.Equal(t, backup.ReasonPreUpgrade, list[0].Manifest.Reason)
	assert.Equal(t, backup.UnknownAppVersion, list[0].Manifest.AppVersion)
	assert.True(t, list[0].Manifest.HasSecret)

	// 记下的结果在数据库打开以后写进备份设置
	store := NewConfigStore(db)
	recordStartupBackups(store, backup.ApplyResult{})
	got, err := store.BackupSettings()
	require.NoError(t, err)
	require.NotNil(t, got.Last)
	assert.Equal(t, backup.ReasonPreUpgrade, got.Last.Reason)
	assert.Equal(t, preUpgrade.name, got.Last.Name)
	assert.Empty(t, got.Last.Error)
	assert.Nil(t, got.Restore)

	// 这次启动正常结束，记下版本；同一个版本再启动不备份
	require.NoError(t, models.WriteAppMeta(db.DB, models.AppMetaLastStartedVersion, version.Version))
	CloseDB(db)
	resetPreUpgrade(t)
	db = openDataDirDB(t, preUpgradeBackup)
	assert.False(t, preUpgrade.ran)
	list, err = backup.List(backups)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	// 上次是别的版本：备份里写上一个版本
	require.NoError(t, models.WriteAppMeta(db.DB, models.AppMetaLastStartedVersion, "v0.48.0"))
	CloseDB(db)
	db = openDataDirDB(t, preUpgradeBackup)
	require.True(t, preUpgrade.ran)
	require.NoError(t, preUpgrade.err)
	list, err = backup.List(backups)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "v0.48.0", list[0].Manifest.AppVersion)
	CloseDB(db)
}

// 升级前备份没做成：照常打开数据库，失败原因写进备份设置。用的是设置里的目录与带不带密钥。
func TestPreUpgradeBackup_FailureDoesNotBlock(t *testing.T) {
	keyTestHome(t, backupTestKey)
	resetPreUpgrade(t)
	db := openDataDirDB(t, nil)
	store := NewConfigStore(db)
	dir := filepath.Join(t.TempDir(), "bk")
	_, err := store.SaveBackupSettings(BackupSettingsInput{Enabled: true, Time: "04:30", Keep: 7, Dir: dir, IncludeSecret: false})
	require.NoError(t, err)
	require.NoError(t, models.WriteAppMeta(db.DB, models.AppMetaLastStartedVersion, "v0.48.0"))
	CloseDB(db)
	require.NoError(t, os.RemoveAll(dir))
	require.NoError(t, os.WriteFile(dir, []byte("x"), 0o600))

	db = openDataDirDB(t, preUpgradeBackup)
	require.True(t, preUpgrade.ran)
	require.Error(t, preUpgrade.err)
	var n int64
	require.NoError(t, db.DB.Model(&models.SiteSetting{}).Count(&n).Error, "数据库照常打开、迁移")
	store = NewConfigStore(db)
	recordStartupBackups(store, backup.ApplyResult{})
	got, err := store.BackupSettings()
	require.NoError(t, err)
	require.NotNil(t, got.Last)
	assert.Equal(t, backup.ReasonPreUpgrade, got.Last.Reason)
	assert.NotEmpty(t, got.Last.Error)
	assert.Empty(t, got.ScheduledDay)

	// 目录恢复正常以后再试：用的是设置里的目录、不带密钥
	require.NoError(t, os.Remove(dir))
	CloseDB(db)
	resetPreUpgrade(t)
	db = openDataDirDB(t, preUpgradeBackup)
	require.NoError(t, preUpgrade.err)
	list, err := backup.List(dir)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.False(t, list[0].Manifest.HasSecret)
	CloseDB(db)
}

// 只有 Web 服务进程换上待恢复：别的子命令（例如服务还在跑时执行的 pt-tools db backup）不碰。
func TestApplyPendingRestore_OnlyWhenEnabled(t *testing.T) {
	home := keyTestHome(t, backupTestKey)
	db := openDataDirDB(t, nil)
	store := NewConfigStore(db)
	info, err := store.RunBackup(context.Background(), backup.ReasonManual)
	require.NoError(t, err)
	_, err = store.StageRestoreBackup(info.Name)
	require.NoError(t, err)
	p, err := PendingRestore()
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, info.Name, p.Name)
	CloseDB(db)

	saved := pendingRestoreEnabled
	pendingRestoreEnabled = false
	t.Cleanup(func() { pendingRestoreEnabled = saved })
	res, err := applyPendingRestore()
	require.NoError(t, err)
	assert.Equal(t, backup.ApplyResult{}, res)
	_, err = os.Stat(filepath.Join(home, ".pt-tools", backup.PendingDir, "ready"))
	assert.NoError(t, err, "没打开时待恢复原样留着")

	require.NoError(t, CancelRestore())
	p, err = PendingRestore()
	require.NoError(t, err)
	assert.Nil(t, p)
}

// 恢复成功：换上备份的数据，结果写进备份设置。
func TestApplyPendingRestore_RecordsResult(t *testing.T) {
	keyTestHome(t, backupTestKey)
	db := openDataDirDB(t, nil)
	store := NewConfigStore(db)
	addEncryptedCookie(t, db.DB, "hdsky", "uid=1")
	info, err := store.RunBackup(context.Background(), backup.ReasonManual)
	require.NoError(t, err)
	_, err = store.StageRestoreBackup(info.Name)
	require.NoError(t, err)
	// 暂存以后又改了数据：恢复以后应当回到备份时的样子
	require.NoError(t, db.DB.Where("name = ?", "hdsky").Delete(&models.SiteSetting{}).Error)
	CloseDB(db)

	enablePendingRestoreForTest(t)
	res, err := applyPendingRestore()
	require.NoError(t, err)
	require.NoError(t, res.Err)
	require.True(t, res.Applied)

	db = openDataDirDB(t, nil)
	store = NewConfigStore(db)
	recordStartupBackups(store, res)
	got, err := store.BackupSettings()
	require.NoError(t, err)
	require.NotNil(t, got.Restore)
	assert.Equal(t, info.Name, got.Restore.Name)
	assert.Empty(t, got.Restore.Error)
	assert.Equal(t, res.PreRestoreDir, got.Restore.Dir)
	var row models.SiteSetting
	require.NoError(t, db.DB.Where("name = ?", "hdsky").First(&row).Error)
	plain, err := crypto.Decrypt(row.CookieEncrypted)
	require.NoError(t, err)
	assert.Equal(t, "uid=1", string(plain))
}

// 暂存以后原来的 secret.key 丢了，启动时新生成了一把密钥（方案 M0 验收）：
// （a）备份不带密钥：复验失败、回滚，之后 checkGeneratedSecretKey 拒绝启动，不带着错的密钥打开旧数据；
// （b）备份带密钥：换上备份里的密钥，复验通过，之后正常启动。
func TestPendingRestore_KeyLostAfterStage(t *testing.T) {
	for name, withKey := range map[string]bool{"（a）备份不带密钥": false, "（b）备份带密钥": true} {
		t.Run(name, func(t *testing.T) {
			home := keyTestHome(t, backupTestKey)
			db := openDataDirDB(t, nil)
			addEncryptedCookie(t, db.DB, "hdsky", "uid=1")
			store := NewConfigStore(db)
			_, err := store.SaveBackupSettings(BackupSettingsInput{Enabled: true, Time: "04:30", Keep: 7, IncludeSecret: withKey})
			require.NoError(t, err)
			info, err := store.RunBackup(context.Background(), backup.ReasonManual)
			require.NoError(t, err)
			_, err = store.StageRestoreBackup(info.Name)
			require.NoError(t, err)
			CloseDB(db)

			keyFile := filepath.Join(home, ".pt-tools", "secret.key")
			require.NoError(t, os.Remove(keyFile))
			crypto.ResetForTest()
			_, generated, _ := crypto.KeyStatus()
			require.True(t, generated, "模拟重启时新生成了一把密钥")

			enablePendingRestoreForTest(t)
			res, err := applyPendingRestore()
			require.NoError(t, err)
			require.NoError(t, checkSecretKeyFile())
			db = openDataDirDB(t, nil)
			keyErr := checkGeneratedSecretKey(db.DB)
			if !withKey {
				require.Error(t, res.Err)
				assert.False(t, res.Applied)
				assert.ErrorIs(t, keyErr, ErrSecretKeyRegenerated)
				_, err = os.Stat(keyFile)
				assert.True(t, os.IsNotExist(err), "新生成的密钥删掉了，放回原来的 secret.key 以后再启动")
				return
			}
			require.NoError(t, res.Err)
			assert.True(t, res.Applied)
			require.NoError(t, keyErr)
			raw, err := os.ReadFile(keyFile)
			require.NoError(t, err)
			assert.Equal(t, string(backupTestKey), string(raw), "secret.key 换成了备份里的")
			var row models.SiteSetting
			require.NoError(t, db.DB.Where("name = ?", "hdsky").First(&row).Error)
			plain, err := crypto.Decrypt(row.CookieEncrypted)
			require.NoError(t, err)
			assert.Equal(t, "uid=1", string(plain))
		})
	}
}

// 环境变量的密钥：带进备份的标成 env，校验时按环境变量判断。
func TestBackupKeyFromEnv(t *testing.T) {
	keyTestHome(t, nil)
	t.Setenv("PT_TOOLS_SECRET_KEY", "MDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDA=")
	crypto.ResetForTest()
	k, err := CurrentBackupKey()
	require.NoError(t, err)
	assert.Equal(t, backup.SecretFromEnv, k.Source)
	assert.Len(t, k.Bytes, 32)
	a := ActiveBackupKey()
	assert.True(t, a.FromEnv)
	assert.Equal(t, k.Bytes, a.Key)
}
