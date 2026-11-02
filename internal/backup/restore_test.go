package backup

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/models"
)

// backupWithCookie 在用 key 的数据目录里写一条加密 Cookie 并做一份备份，返回备份文件（拷到数据目录以外）。
func backupWithCookie(t *testing.T, key []byte, withKey bool) string {
	t.Helper()
	e := newEnv(t, key)
	db := openDB(t)
	addCookie(t, db, "hdsky", "uid=1")
	info := makeBackup(t, db, e.data, withKey, ReasonManual, time.Now())
	closeDB(t, db)
	out := filepath.Join(t.TempDir(), info.Name)
	require.NoError(t, os.Rename(info.Path, out))
	return out
}

func backupWithoutCipher(t *testing.T, key []byte) string {
	t.Helper()
	e := newEnv(t, key)
	db := openDB(t)
	info := makeBackup(t, db, e.data, false, ReasonManual, time.Now())
	closeDB(t, db)
	out := filepath.Join(t.TempDir(), info.Name)
	require.NoError(t, os.Rename(info.Path, out))
	return out
}

func active(key []byte) ActiveKey { return ActiveKey{Key: key} }

// 恢复的密钥规则，每一种情况。
func TestVerify_KeyRules(t *testing.T) {
	withKey := backupWithCookie(t, keyOf(1), true)
	noKey := backupWithCookie(t, keyOf(1), false)
	noCipher := backupWithoutCipher(t, keyOf(1))
	work := t.TempDir()

	t.Run("带密钥：换成备份里的", func(t *testing.T) {
		plan, err := Verify(withKey, work, active(keyOf(2)))
		require.NoError(t, err)
		assert.Equal(t, KeyUseBackup, plan.Key)
		assert.False(t, plan.NoCipher)
	})
	t.Run("带密钥、和现在的一样：不换", func(t *testing.T) {
		plan, err := Verify(withKey, work, active(keyOf(1)))
		require.NoError(t, err)
		assert.Equal(t, KeyKeepFile, plan.Key)
	})
	t.Run("不带密钥、没有密文", func(t *testing.T) {
		plan, err := Verify(noCipher, work, active(keyOf(9)))
		require.NoError(t, err)
		assert.Equal(t, KeyKeepFile, plan.Key)
		assert.True(t, plan.NoCipher)
	})
	t.Run("不带密钥、现在的密钥解得开", func(t *testing.T) {
		plan, err := Verify(noKey, work, active(keyOf(1)))
		require.NoError(t, err)
		assert.Equal(t, KeyKeepFile, plan.Key)
	})
	t.Run("不带密钥、现在的密钥解不开：拒绝", func(t *testing.T) {
		_, err := Verify(noKey, work, active(keyOf(2)))
		require.ErrorIs(t, err, ErrKeyMismatch)
		assert.Contains(t, err.Error(), "这份备份没带密钥")
		assert.Contains(t, err.Error(), "站点 Cookie")
		_, err = Verify(noKey, work, ActiveKey{})
		assert.ErrorIs(t, err, ErrKeyMismatch, "现在的密钥读不出来时同样拒绝")
	})
	t.Run("环境变量解得开：仍用环境变量（备份里的密钥不用）", func(t *testing.T) {
		plan, err := Verify(withKey, work, ActiveKey{FromEnv: true, Key: keyOf(1)})
		require.NoError(t, err)
		assert.Equal(t, KeyKeepEnv, plan.Key)
	})
	t.Run("环境变量解不开：拒绝", func(t *testing.T) {
		_, err := Verify(withKey, work, ActiveKey{FromEnv: true, Key: keyOf(2)})
		require.ErrorIs(t, err, ErrKeyMismatch)
		assert.Contains(t, err.Error(), "PT_TOOLS_SECRET_KEY")
	})
	left, err := os.ReadDir(work)
	require.NoError(t, err)
	assert.Empty(t, left, "校验用的临时文件都删掉了")
}

// 备份文件本身有问题：各种情况都拒绝，原因写清楚。
func TestVerify_InvalidFiles(t *testing.T) {
	src := backupWithCookie(t, keyOf(1), true)
	work := t.TempDir()
	cases := map[string]struct {
		path string
		want string
	}{
		"不是压缩包":       {path: writeTemp(t, "junk"), want: "不是 tar.gz"},
		"没有 manifest": {path: rewrite(t, src, func(f map[string][]byte, o []string) []string { return o[1:] }), want: "没有 manifest.json"},
		"数据库被改过": {path: rewrite(t, src, func(f map[string][]byte, o []string) []string {
			f[dbName] = append([]byte(nil), f[dbName]...)
			f[dbName][len(f[dbName])-1] ^= 0xff
			return o
		}), want: "SHA-256"},
		"格式太新": {path: rewrite(t, src, func(f map[string][]byte, o []string) []string {
			setManifest(t, f, func(m *Manifest) { m.Format = FormatVersion + 1 })
			return o
		}), want: "更新版本的 pt-tools"},
		"数据库版本太新": {path: rewrite(t, src, func(f map[string][]byte, o []string) []string {
			setManifest(t, f, func(m *Manifest) { m.SchemaVersion = models.CurrentSchemaVersion + 1 })
			return o
		}), want: "先升级 pt-tools"},
		"说带了密钥却没有": {path: rewrite(t, src, func(f map[string][]byte, o []string) []string { return o[:2] }), want: "却没有 secret.key"},
		"密钥格式不对": {path: rewrite(t, src, func(f map[string][]byte, o []string) []string {
			f[keyName] = []byte("zz")
			return o
		}), want: "不是 64 位十六进制"},
		"没有数据库": {path: rewrite(t, src, func(f map[string][]byte, o []string) []string {
			return []string{o[0], o[2]}
		}), want: "没有数据库"},
		"备份里的密钥解不开自己的库": {path: rewrite(t, src, func(f map[string][]byte, o []string) []string {
			f[keyName] = []byte(strings.Repeat("ab", 32))
			return o
		}), want: "备份里的密钥解不开"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Verify(c.path, work, active(keyOf(1)))
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.want)
		})
	}
}

// 数据库损坏但 SHA-256 与 manifest 一致（做备份时就坏了）：integrity_check 拦下。
func TestVerify_CorruptDatabase(t *testing.T) {
	src := backupWithoutCipher(t, keyOf(1))
	bad := rewrite(t, src, func(f map[string][]byte, o []string) []string {
		db := append([]byte(nil), f[dbName]...)
		// 第二页以后整页写乱：文件头还认得出来，页里的 B 树坏了
		for i := 4096; i < len(db) && i < 4096*4; i++ {
			db[i] = 0x5a
		}
		f[dbName] = db
		sum, size := sha256Hex(db), int64(len(db))
		setManifest(t, f, func(m *Manifest) { m.DBSHA256, m.DBSize = sum, size })
		return o
	})
	_, err := Verify(bad, t.TempDir(), active(keyOf(1)))
	require.ErrorIs(t, err, ErrInvalid)
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "x.tar.gz")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

// 放进 restore-pending：要换密钥时带上密钥；再放一份会替换前一份；取消就删掉。
func TestStageReadCancel(t *testing.T) {
	src := backupWithCookie(t, keyOf(1), true)
	e := newEnv(t, keyOf(2))
	now := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)

	p, err := Stage(src, "a.tar.gz", e.data, active(keyOf(2)), now)
	require.NoError(t, err)
	assert.Equal(t, KeyUseBackup, p.Key)
	assert.True(t, p.HasKey)
	pend := filepath.Join(e.data, PendingDir)
	for _, n := range []string{dbName, keyName, readyName, pendingInfoName} {
		_, err = os.Stat(filepath.Join(pend, n))
		assert.NoError(t, err, n)
	}
	got, err := ReadPending(e.data)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "a.tar.gz", got.Name)
	assert.True(t, got.StagedAt.Equal(now))

	// 第二次放：现在的密钥就是备份里的，不用换；密钥照样放进去，启动时原来的 secret.key 丢了也能用它
	require.NoError(t, os.WriteFile(filepath.Join(e.data, keyName), []byte(strings.Repeat("01", 32)), 0o600))
	p, err = Stage(src, "b.tar.gz", e.data, active(keyOf(1)), now)
	require.NoError(t, err)
	assert.Equal(t, KeyKeepFile, p.Key)
	assert.True(t, p.HasKey)
	raw, err := os.ReadFile(filepath.Join(pend, keyName))
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("01", 32), string(raw))
	got, err = ReadPending(e.data)
	require.NoError(t, err)
	assert.Equal(t, "b.tar.gz", got.Name)
	assert.True(t, got.HasKey)

	// 不带密钥的备份不放密钥
	noKey := backupWithCookie(t, keyOf(1), false)
	require.NoError(t, os.WriteFile(filepath.Join(e.data, keyName), []byte(strings.Repeat("01", 32)), 0o600))
	p, err = Stage(noKey, "d.tar.gz", e.data, active(keyOf(1)), now)
	require.NoError(t, err)
	assert.False(t, p.HasKey)
	_, err = os.Stat(filepath.Join(pend, keyName))
	assert.True(t, os.IsNotExist(err), "不带密钥的备份不放密钥")

	require.NoError(t, CancelPending(e.data))
	got, err = ReadPending(e.data)
	require.NoError(t, err)
	assert.Nil(t, got)
	entries, err := os.ReadDir(e.data)
	require.NoError(t, err)
	for _, x := range entries {
		assert.False(t, strings.HasPrefix(x.Name(), ".restore-pending") || strings.HasPrefix(x.Name(), ".pt-tools-restore"), "没有留下临时目录：%s", x.Name())
	}

	// 校验不过的不放
	_, err = Stage(src, "c.tar.gz", e.data, ActiveKey{FromEnv: true, Key: keyOf(3)}, now)
	assert.ErrorIs(t, err, ErrKeyMismatch)
	got, err = ReadPending(e.data)
	require.NoError(t, err)
	assert.Nil(t, got)
}

// stagedEnv 建一个数据目录：现有数据库里有一条用 current 加密的 Cookie（Cookie 的值是 marker），
// 再把 src 这份备份放进 restore-pending。
func stagedEnv(t *testing.T, src string, current []byte, marker string) env {
	t.Helper()
	e := newEnv(t, current)
	db := openDB(t)
	addCookie(t, db, "current", marker)
	closeDB(t, db)
	_, err := Stage(src, filepath.Base(src), e.data, active(current), time.Now())
	require.NoError(t, err)
	return e
}

// cookieOf 打开数据目录的数据库，用当前进程的密钥解出某个站点的 Cookie。
func cookieOf(t *testing.T, site string) (string, error) {
	t.Helper()
	db := openDB(t)
	defer closeDB(t, db)
	var row models.SiteSetting
	if err := db.Where("name = ?", site).First(&row).Error; err != nil {
		return "", err
	}
	plain, err := crypto.Decrypt(row.CookieEncrypted)
	return string(plain), err
}

func TestApplyPending_NothingPending(t *testing.T) {
	e := newEnv(t, keyOf(1))
	res, err := ApplyPending(e.data, false, time.Now())
	require.NoError(t, err)
	assert.Equal(t, ApplyResult{}, res)

	// 半成品（没写 ready）删掉
	require.NoError(t, os.MkdirAll(filepath.Join(e.data, PendingDir), 0o700))
	res, err = ApplyPending(e.data, false, time.Now())
	require.NoError(t, err)
	assert.False(t, res.Applied)
	_, statErr := os.Stat(filepath.Join(e.data, PendingDir))
	assert.True(t, os.IsNotExist(statErr))
}

// 不换密钥的恢复：原文件挪进 pre-restore，换上备份的数据库。
func TestApplyPending_Success(t *testing.T) {
	src := backupWithCookie(t, keyOf(1), false)
	e := stagedEnv(t, src, keyOf(1), "before")
	res, err := ApplyPending(e.data, false, time.Date(2026, 11, 2, 5, 0, 0, 0, time.Local))
	require.NoError(t, err)
	require.NoError(t, res.Err)
	assert.True(t, res.Applied)
	assert.Equal(t, filepath.Join(e.data, DefaultDirName, "pre-restore-20261102-050000"), res.PreRestoreDir)
	_, err = os.Stat(filepath.Join(res.PreRestoreDir, dbName))
	assert.NoError(t, err, "原来的数据库挪进了 pre-restore")
	_, err = os.Stat(filepath.Join(e.data, PendingDir))
	assert.True(t, os.IsNotExist(err))

	got, err := cookieOf(t, "hdsky")
	require.NoError(t, err)
	assert.Equal(t, "uid=1", got)
	_, err = cookieOf(t, "current")
	assert.Error(t, err, "换上的是备份的数据，恢复以前写的那一行没有了")
}

// 换密钥的恢复：secret.key 换成备份里的并重新读取，「本进程新生成了密钥」的标记清掉。
func TestApplyPending_SwapsKey(t *testing.T) {
	src := backupWithCookie(t, keyOf(1), true)
	e := stagedEnv(t, src, keyOf(2), "before")
	res, err := ApplyPending(e.data, false, time.Now())
	require.NoError(t, err)
	require.NoError(t, res.Err)
	assert.True(t, res.Applied)
	raw, err := os.ReadFile(filepath.Join(e.data, keyName))
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("01", 32), string(raw))
	k, err := crypto.ExportKey()
	require.NoError(t, err)
	assert.Equal(t, keyOf(1), k)
	_, generated, loadErr := crypto.KeyStatus()
	assert.False(t, generated)
	assert.NoError(t, loadErr)
	old, err := os.ReadFile(filepath.Join(res.PreRestoreDir, keyName))
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("02", 32), string(old), "原来的密钥挪进了 pre-restore")
	got, err := cookieOf(t, "hdsky")
	require.NoError(t, err)
	assert.Equal(t, "uid=1", got)
}

// 备份带的密钥和现在的 secret.key 一样：不换密钥文件、不碰内存里的密钥状态。
func TestApplyPending_SameKeyNoSwap(t *testing.T) {
	src := backupWithCookie(t, keyOf(1), true)
	e := stagedEnv(t, src, keyOf(1), "before")
	before := crypto.SnapshotKeyState()
	res, err := ApplyPending(e.data, false, time.Now())
	require.NoError(t, err)
	require.NoError(t, res.Err)
	assert.True(t, res.Applied)
	assert.Equal(t, before, crypto.SnapshotKeyState())
	_, err = os.Stat(filepath.Join(res.PreRestoreDir, keyName))
	assert.True(t, os.IsNotExist(err), "密钥文件没有挪走")
	got, err := cookieOf(t, "hdsky")
	require.NoError(t, err)
	assert.Equal(t, "uid=1", got)
}

// 暂存以后原来的 secret.key 丢了、启动时新生成了一把：备份带了密钥，就换上备份里的密钥，正常启动。
func TestApplyPending_KeyLostAfterStage(t *testing.T) {
	src := backupWithCookie(t, keyOf(1), true)
	e := stagedEnv(t, src, keyOf(1), "before")
	require.NoError(t, os.Remove(filepath.Join(e.data, keyName)))
	crypto.ResetForTest()
	_, generated, _ := crypto.KeyStatus()
	require.True(t, generated, "模拟启动时新生成了密钥")

	res, err := ApplyPending(e.data, false, time.Now())
	require.NoError(t, err)
	require.NoError(t, res.Err)
	assert.True(t, res.Applied)
	k, err := crypto.ExportKey()
	require.NoError(t, err)
	assert.Equal(t, keyOf(1), k)
	_, generated, loadErr := crypto.KeyStatus()
	assert.False(t, generated)
	assert.NoError(t, loadErr)
	_, err = os.Stat(filepath.Join(res.PreRestoreDir, keyName))
	assert.NoError(t, err, "新生成的那把挪进了 pre-restore")
}

// 启动时用的是环境变量的密钥：待恢复里的密钥不用，secret.key 文件不动。
func TestApplyPending_EnvKeyIgnoresStagedKey(t *testing.T) {
	src := backupWithCookie(t, keyOf(1), true)
	e := stagedEnv(t, src, keyOf(2), "before")
	t.Setenv("PT_TOOLS_SECRET_KEY", base64.StdEncoding.EncodeToString(keyOf(1)))
	crypto.ResetForTest()
	res, err := ApplyPending(e.data, true, time.Now())
	require.NoError(t, err)
	require.NoError(t, res.Err)
	assert.True(t, res.Applied)
	raw, err := os.ReadFile(filepath.Join(e.data, keyName))
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("02", 32), string(raw), "secret.key 没有换")
}

// 待恢复里的密钥坏了：不换，删掉待恢复，按原来的数据启动。
func TestApplyPending_BadStagedKey(t *testing.T) {
	src := backupWithCookie(t, keyOf(1), true)
	e := stagedEnv(t, src, keyOf(2), "before")
	require.NoError(t, os.WriteFile(filepath.Join(e.data, PendingDir, keyName), []byte("zz"), 0o600))
	res, err := ApplyPending(e.data, false, time.Now())
	require.NoError(t, err)
	require.Error(t, res.Err)
	assert.Contains(t, res.Err.Error(), "密钥读不出来")
	assert.False(t, res.Applied)
	_, err = os.Stat(filepath.Join(e.data, PendingDir))
	assert.True(t, os.IsNotExist(err))
	got, err := cookieOf(t, "current")
	require.NoError(t, err)
	assert.Equal(t, "before", got)
}

// 第 1、2、3 步分别失败：文件回到原位、密钥状态与恢复以前逐项相同、原来的数据库能打开、密文能解开。
func TestApplyPending_RollbackEachStep(t *testing.T) {
	failAt := map[string]func(){
		"第 1 步：建 pre-restore 失败": func() { ops.mkdir = func(string) error { return errors.New("read-only") } },
		"第 1 步：挪走原数据库失败": func() {
			ops.rename = func(o, n string) error {
				if strings.Contains(n, "pre-restore") {
					return errors.New("busy")
				}
				return os.Rename(o, n)
			}
		},
		"第 2 步：放入备份的数据库失败": func() {
			ops.rename = func(o, n string) error {
				if strings.Contains(o, PendingDir) && strings.HasSuffix(o, dbName) {
					return errors.New("no space")
				}
				return os.Rename(o, n)
			}
		},
		"第 2 步：放入备份的密钥失败": func() {
			ops.rename = func(o, n string) error {
				if strings.Contains(o, PendingDir) && strings.HasSuffix(o, keyName) {
					return errors.New("no space")
				}
				return os.Rename(o, n)
			}
		},
		"第 3 步：复验失败": func() { ops.verify = func(string) error { return errors.New("复验失败") } },
	}
	for name, inject := range failAt {
		t.Run(name, func(t *testing.T) {
			src := backupWithCookie(t, keyOf(1), true)
			e := stagedEnv(t, src, keyOf(2), "before")
			beforeKey := crypto.SnapshotKeyState()
			p1, g1, err1 := crypto.KeyStatus()
			saved := ops
			inject()
			res, err := ApplyPending(e.data, false, time.Now())
			ops = saved
			require.NoError(t, err, "回滚成功时继续启动")
			require.Error(t, res.Err)
			assert.False(t, res.Applied)

			p2, g2, err2 := crypto.KeyStatus()
			assert.Equal(t, p1, p2)
			assert.Equal(t, g1, g2)
			assert.Equal(t, err1, err2)
			assert.Equal(t, beforeKey, crypto.SnapshotKeyState(), "密钥状态原样还原")
			raw, err := os.ReadFile(filepath.Join(e.data, keyName))
			require.NoError(t, err)
			assert.Equal(t, strings.Repeat("02", 32), string(raw), "原来的密钥回到原位")
			got, err := cookieOf(t, "current")
			require.NoError(t, err, "原来的数据库能打开、密文能解开")
			assert.Equal(t, "before", got)
			_, err = os.Stat(filepath.Join(e.data, PendingDir))
			assert.True(t, os.IsNotExist(err), "待恢复删掉了，下次启动不会再试")
			_, err = os.Stat(res.PreRestoreDir)
			assert.True(t, os.IsNotExist(err), "空的 pre-restore 目录删掉了")
		})
	}
}

// 不换密钥时复验失败（暂存以后密钥变了）：不碰密钥状态，文件放回原位。
func TestApplyPending_VerifyFailsWithoutKeySwap(t *testing.T) {
	src := backupWithCookie(t, keyOf(1), false)
	e := stagedEnv(t, src, keyOf(1), "before")
	// 暂存以后换了一把密钥（例如用户 secret import 了别的密钥）
	require.NoError(t, os.WriteFile(filepath.Join(e.data, keyName), []byte(strings.Repeat("03", 32)), 0o600))
	crypto.ResetForTest()
	before := crypto.SnapshotKeyState()
	res, err := ApplyPending(e.data, false, time.Now())
	require.NoError(t, err)
	require.Error(t, res.Err)
	assert.Contains(t, res.Err.Error(), "解不开")
	assert.Equal(t, before, crypto.SnapshotKeyState())
	_, err = os.Stat(filepath.Join(e.data, dbName))
	assert.NoError(t, err, "原来的数据库回到原位")
}

// 回滚也失败：返回错误让调用方停止启动，错误里写明两份文件各在哪里。
func TestApplyPending_RollbackFailureStopsStartup(t *testing.T) {
	src := backupWithCookie(t, keyOf(1), false)
	e := stagedEnv(t, src, keyOf(1), "before")
	saved := ops
	defer func() { ops = saved }()
	ops.verify = func(string) error { return errors.New("复验失败") }
	ops.rename = func(o, n string) error {
		if strings.Contains(o, "pre-restore") {
			return errors.New("permission denied")
		}
		return os.Rename(o, n)
	}
	res, err := ApplyPending(e.data, false, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "停止启动")
	assert.Contains(t, err.Error(), res.PreRestoreDir)
	assert.Contains(t, err.Error(), filepath.Join(e.data, PendingDir))
}

func TestSqliteURI(t *testing.T) {
	u := sqliteURI("/tmp/a b/torrents.db", "mode=ro")
	assert.Equal(t, "file:///tmp/a%20b/torrents.db?mode=ro", u)
}

func TestEnvKeySetup(t *testing.T) {
	// 环境变量密钥时 Stage 不放 secret.key
	src := backupWithCookie(t, keyOf(1), true)
	e := newEnv(t, nil)
	t.Setenv("PT_TOOLS_SECRET_KEY", base64.StdEncoding.EncodeToString(keyOf(1)))
	crypto.ResetForTest()
	p, err := Stage(src, "x", e.data, ActiveKey{FromEnv: true, Key: keyOf(1)}, time.Now())
	require.NoError(t, err)
	assert.Equal(t, KeyKeepEnv, p.Key)
	_, err = os.Stat(filepath.Join(e.data, PendingDir, keyName))
	assert.True(t, os.IsNotExist(err))
	res, err := ApplyPending(e.data, true, time.Now())
	require.NoError(t, err)
	require.NoError(t, res.Err)
	assert.True(t, res.Applied)
}
