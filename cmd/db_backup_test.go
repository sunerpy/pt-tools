package cmd

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"moul.io/zapgorm2"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/internal/backup"
	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

// cliKeyHome 把 HOME 指到临时目录、secret.key 写成 key（nil 表示没有），按它重新读取密钥；
// 结束时 HOME 与环境变量换回去以后再重新读一次（这个清理最先登记，所以最后执行）。
func cliKeyHome(t *testing.T, key []byte) string {
	t.Helper()
	t.Cleanup(crypto.ResetForTest)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PT_TOOLS_SECRET_KEY", "")
	if key != nil {
		dir := filepath.Join(home, ".pt-tools")
		require.NoError(t, os.MkdirAll(dir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "secret.key"), []byte(hex.EncodeToString(key)), 0o600))
	}
	crypto.ResetForTest()
	return home
}

func cliKey(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

// openServiceDB 像服务那样打开数据目录里的数据库（WAL、迁移），写一条加密的 Cookie；测试结束时关掉。
func openServiceDB(t *testing.T) *models.TorrentDB {
	t.Helper()
	db, err := models.NewDB(zapgorm2.Logger{ZapLogger: zap.NewNop()})
	require.NoError(t, err)
	t.Cleanup(func() { core.CloseDB(db) })
	enc, err := crypto.Encrypt([]byte("uid=1"))
	require.NoError(t, err)
	require.NoError(t, db.DB.Create(&models.SiteSetting{Name: "hdsky", AuthMethod: "cookie", CookieEncrypted: enc}).Error)
	return db
}

// 服务开着数据库的时候备份：放进默认的备份目录、带上密钥，现在的密钥能直接恢复它。
func TestDBBackup_WhileServiceRuns(t *testing.T) {
	home := cliKeyHome(t, cliKey(1))
	openServiceDB(t)
	var out bytes.Buffer
	require.NoError(t, runDBBackup(context.Background(), &out, "", false))
	assert.Contains(t, out.String(), "带了密钥")

	list, err := backup.List(filepath.Join(home, ".pt-tools", "backups"))
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, backup.ReasonCLI, list[0].Manifest.Reason)
	assert.True(t, list[0].Manifest.HasSecret)
	plan, err := backup.Verify(list[0].Path, t.TempDir(), core.ActiveBackupKey())
	require.NoError(t, err)
	assert.Equal(t, backup.KeyKeepFile, plan.Key)
	assert.False(t, plan.NoCipher)
}

// --output 是目录时在里面起名，是文件时写到这个文件（已存在时不覆盖）；--no-secret 不带密钥。
func TestDBBackup_OutputAndNoSecret(t *testing.T) {
	cliKeyHome(t, cliKey(1))
	openServiceDB(t)
	dir := t.TempDir()
	var out bytes.Buffer
	require.NoError(t, runDBBackup(context.Background(), &out, dir, true))
	assert.Contains(t, out.String(), "没带密钥")
	list, err := backup.List(dir)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.False(t, list[0].Manifest.HasSecret)

	file := filepath.Join(t.TempDir(), "mine.tar.gz")
	require.NoError(t, runDBBackup(context.Background(), &out, file, false))
	m, err := backup.ReadManifest(file)
	require.NoError(t, err)
	assert.True(t, m.HasSecret)
	err = runDBBackup(context.Background(), &out, file, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不覆盖")
}

func TestDBBackup_NoDatabase(t *testing.T) {
	cliKeyHome(t, cliKey(1))
	err := runDBBackup(context.Background(), &bytes.Buffer{}, "", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "没有找到数据库")
}

// secret.key 丢了（命令启动时新生成了一把）：带不上密钥，报错；--no-secret 照样能备份数据库。新生成的那把用完删掉。
func TestDBBackup_KeyLost(t *testing.T) {
	home := cliKeyHome(t, cliKey(1))
	db := openServiceDB(t)
	core.CloseDB(db)
	keyFile := filepath.Join(home, ".pt-tools", "secret.key")
	require.NoError(t, os.Remove(keyFile))
	crypto.ResetForTest()

	err := runDBBackup(context.Background(), &bytes.Buffer{}, "", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "带不上密钥")
	assert.Contains(t, err.Error(), "--no-secret")
	_, statErr := os.Stat(keyFile)
	assert.True(t, os.IsNotExist(statErr), "命令新生成的密钥删掉了")

	crypto.ResetForTest()
	require.NoError(t, runDBBackup(context.Background(), &bytes.Buffer{}, "", true))
	_, statErr = os.Stat(keyFile)
	assert.True(t, os.IsNotExist(statErr))
}

// 校验通过以后放进待恢复（不动现在的数据库），--cancel 取消；文件不对时报错。
func TestDBRestore_StageAndCancel(t *testing.T) {
	home := cliKeyHome(t, cliKey(1))
	openServiceDB(t)
	file := filepath.Join(t.TempDir(), "b.tar.gz")
	require.NoError(t, runDBBackup(context.Background(), &bytes.Buffer{}, file, false))

	var out bytes.Buffer
	require.NoError(t, runDBRestore(&out, file, false))
	assert.Contains(t, out.String(), "备份校验通过：b.tar.gz")
	assert.Contains(t, out.String(), "命令行备份")
	assert.Contains(t, out.String(), "secret.key 不变")
	assert.Contains(t, out.String(), filepath.Join(home, ".pt-tools", "backups", "pre-restore-<时间>")+"/", "写出换下来的文件放在哪")
	p, err := core.PendingRestore()
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.True(t, p.HasKey)
	_, err = os.Stat(filepath.Join(home, ".pt-tools", "torrents.db"))
	assert.NoError(t, err, "现在的数据库还在原处")

	out.Reset()
	require.NoError(t, runDBRestore(&out, "", true))
	assert.Contains(t, out.String(), "已取消恢复 b.tar.gz")
	out.Reset()
	require.NoError(t, runDBRestore(&out, "", true))
	assert.Contains(t, out.String(), "没有待恢复的备份")

	assert.ErrorContains(t, runDBRestore(&out, "", false), "--input")
	assert.ErrorContains(t, runDBRestore(&out, filepath.Join(t.TempDir(), "nope.tar.gz"), false), "找不到备份文件")
	junk := filepath.Join(t.TempDir(), "junk.tar.gz")
	require.NoError(t, os.WriteFile(junk, []byte("junk"), 0o600))
	assert.ErrorContains(t, runDBRestore(&out, junk, false), "不能恢复")
}

// 原来的 secret.key 丢了、备份带着密钥：照样能放进待恢复，恢复时换上备份里的密钥；这次新生成的密钥删掉。
func TestDBRestore_KeyLostWithKeyedBackup(t *testing.T) {
	home := cliKeyHome(t, cliKey(1))
	db := openServiceDB(t)
	file := filepath.Join(t.TempDir(), "b.tar.gz")
	require.NoError(t, runDBBackup(context.Background(), &bytes.Buffer{}, file, false))
	core.CloseDB(db)
	keyFile := filepath.Join(home, ".pt-tools", "secret.key")
	require.NoError(t, os.Remove(keyFile))
	crypto.ResetForTest()

	var out bytes.Buffer
	require.NoError(t, runDBRestore(&out, file, false))
	assert.Contains(t, out.String(), "换成备份里的密钥")
	_, err := os.Stat(keyFile)
	assert.True(t, os.IsNotExist(err))
	p, err := core.PendingRestore()
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.True(t, p.HasKey)
}

func TestKeyActionText(t *testing.T) {
	assert.Contains(t, keyActionText(backup.Pending{Plan: backup.Plan{Key: backup.KeyKeepEnv}}), "PT_TOOLS_SECRET_KEY")
	assert.Contains(t, keyActionText(backup.Pending{Plan: backup.Plan{Key: backup.KeyUseBackup}}), "换成备份里的密钥")
	assert.Contains(t, keyActionText(backup.Pending{Plan: backup.Plan{Key: backup.KeyKeepFile, NoCipher: true}}), "不需要密钥")
	assert.Contains(t, keyActionText(backup.Pending{Plan: backup.Plan{Key: backup.KeyKeepFile}}), "解得开")
}

// 定时任务读到的设置：开关、时刻、做过的日期、生效的目录与上次成功的时间。
func TestBackupSchedule(t *testing.T) {
	cliKeyHome(t, cliKey(1))
	db, err := core.OpenTestDB(t)
	require.NoError(t, err)
	store := core.NewConfigStore(db)
	got, err := backupSchedule(store)()
	require.NoError(t, err)
	assert.True(t, got.Enabled)
	assert.Equal(t, "04:30", got.Time)
	assert.Empty(t, got.Day)
	assert.True(t, strings.HasSuffix(got.Dir, filepath.Join(".pt-tools", "backups")))
	assert.Nil(t, got.LastOK)

	at := time.Date(2026, 11, 2, 4, 30, 0, 0, time.Local)
	require.NoError(t, store.RecordBackupResult(backup.ReasonScheduled, "a.tar.gz", nil, at))
	got, err = backupSchedule(store)()
	require.NoError(t, err)
	assert.Equal(t, "2026-11-02", got.Day)
	require.NotNil(t, got.LastOK)
	assert.True(t, got.LastOK.Equal(at))

	assert.Nil(t, wireBackupJob(nil, store, nil), "没有调度器时不启动")
	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)
	assert.NotNil(t, wireBackupJob(mgr, store, nil))
}

func TestAppVersionText(t *testing.T) {
	assert.Equal(t, "v1.0.0-rc.53", appVersionText("v1.0.0-rc.53"))
	assert.Equal(t, "未知", appVersionText(backup.UnknownAppVersion))
	assert.Equal(t, "未知", appVersionText(""))
}
