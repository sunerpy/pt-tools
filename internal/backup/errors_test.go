package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
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

	"github.com/sunerpy/pt-tools/models"
)

type tarEntry struct {
	name string
	body []byte
	dir  bool
}

// tarGz 按给定的顺序写一个 tar.gz（造坏掉、拼错的备份用）。
func tarGz(t *testing.T, ents ...tarEntry) string {
	t.Helper()
	return tarGzTail(t, nil, ents...)
}

// tarGzTail 同 tarGz；tail 不为空时不写 tar 的结尾，换成 tail 这些字节（压缩本身是完整的）。
func tarGzTail(t *testing.T, tail []byte, ents ...tarEntry) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range ents {
		h := &tar.Header{Name: e.name, Mode: 0o600, Size: int64(len(e.body)), Typeflag: tar.TypeReg}
		if e.dir {
			h = &tar.Header{Name: e.name, Mode: 0o700, Typeflag: tar.TypeDir}
		}
		require.NoError(t, tw.WriteHeader(h))
		if !e.dir {
			_, err := tw.Write(e.body)
			require.NoError(t, err)
		}
	}
	if tail != nil {
		require.NoError(t, tw.Flush())
		_, err := gz.Write(tail)
		require.NoError(t, err)
	} else {
		require.NoError(t, tw.Close())
	}
	require.NoError(t, gz.Close())
	p := filepath.Join(t.TempDir(), "x.tar.gz")
	require.NoError(t, os.WriteFile(p, buf.Bytes(), 0o600))
	return p
}

// 只读权限位在 Windows 与 root 下不起作用，这几处跳过。
func canLockDirs() bool { return runtime.GOOS != "windows" && os.Geteuid() != 0 }

// 参数不对、备份目录建不出来、给的密钥不对时报错。
func TestCreate_OptionErrors(t *testing.T) {
	e := newEnv(t, keyOf(1))
	db := openDB(t)
	defer closeDB(t, db)
	ctx := context.Background()
	dir := filepath.Join(e.data, DefaultDirName)

	_, err := Create(ctx, db, Options{Reason: ReasonManual})
	assert.ErrorContains(t, err, "没有指定备份目录")
	_, err = Create(ctx, db, Options{Dir: filepath.Join(writeTemp(t, "x"), "sub"), Reason: ReasonManual})
	assert.ErrorContains(t, err, "创建备份目录失败")
	_, err = Create(ctx, db, Options{Dir: dir, Reason: ReasonManual, IncludeSecret: true})
	assert.ErrorContains(t, err, "却没有给出密钥")
	_, err = Create(ctx, db, Options{
		Dir: dir, Reason: ReasonManual, IncludeSecret: true,
		Key: func() (Key, error) { return Key{Bytes: []byte("short")}, nil },
	})
	assert.ErrorContains(t, err, "密钥长度不对")
	if canLockDirs() {
		ro := filepath.Join(t.TempDir(), "ro")
		require.NoError(t, os.Mkdir(ro, 0o500))
		t.Cleanup(func() { _ = os.Chmod(ro, 0o700) })
		_, err = Create(ctx, db, Options{Dir: ro, Reason: ReasonManual})
		assert.ErrorContains(t, err, "创建临时目录失败")
	}
}

// ReadManifest 只看第一个文件：各种坏法都是 ErrInvalid，原因写清楚。
func TestReadManifest_Errors(t *testing.T) {
	_, err := ReadManifest(filepath.Join(t.TempDir(), "nope.tar.gz"))
	assert.ErrorContains(t, err, "打开备份失败")
	cases := map[string]struct{ path, want string }{
		"第一个不是 manifest":   {tarGz(t, tarEntry{name: dbName, body: []byte("x")}), "第一个文件不是 manifest.json"},
		"manifest 太大":      {tarGz(t, tarEntry{name: manifestName, body: bytes.Repeat([]byte(" "), maxManifestSize+1)}), "太大"},
		"manifest 不是 JSON": {tarGz(t, tarEntry{name: manifestName, body: []byte("{")}), "格式不对"},
		"没有格式版本":           {tarGz(t, tarEntry{name: manifestName, body: []byte(`{"reason":"manual"}`)}), "没有格式版本"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ReadManifest(c.path)
			assert.ErrorIs(t, err, ErrInvalid)
			assert.ErrorContains(t, err, c.want)
		})
	}
}

// 压缩包里的内容不对：读到一半断了、manifest 坏了、说没带密钥却有、secret.key 太长；目录项与不认识的文件跳过。
func TestVerify_BadEntries(t *testing.T) {
	src := backupWithCookie(t, keyOf(1), true)
	raw, err := os.ReadFile(src)
	require.NoError(t, err)
	_, files := entries(t, src)
	manifest := files[manifestName]
	var m Manifest
	require.NoError(t, json.Unmarshal(manifest, &m))
	m.HasSecret = false
	noSecret, err := json.Marshal(m)
	require.NoError(t, err)

	cases := map[string]struct{ path, want string }{
		"数据库读到一半断了": {writeTemp(t, string(raw[:len(raw)/2])), "解出数据库失败"},
		"后面跟着认不出的内容": {tarGzTail(t, bytes.Repeat([]byte("x"), 512),
			tarEntry{name: manifestName, body: manifest}, tarEntry{name: dbName, body: files[dbName]},
		), "读不完这个压缩包"},
		"manifest 坏了": {tarGz(t, tarEntry{name: manifestName, body: []byte("{")}), "格式不对"},
		"说没带密钥却有": {tarGz(t,
			tarEntry{name: manifestName, body: noSecret}, tarEntry{name: dbName, body: files[dbName]}, tarEntry{name: keyName, body: files[keyName]},
		), "却有 secret.key"},
		"secret.key 太长": {tarGz(t,
			tarEntry{name: manifestName, body: manifest}, tarEntry{name: dbName, body: files[dbName]},
			tarEntry{name: keyName, body: bytes.Repeat([]byte("a"), maxKeySize+1)},
		), "读不出 secret.key"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, verr := Verify(c.path, t.TempDir(), active(keyOf(1)))
			assert.ErrorIs(t, verr, ErrInvalid)
			assert.ErrorContains(t, verr, c.want)
		})
	}

	withDir := tarGz(t,
		tarEntry{name: manifestName, body: manifest}, tarEntry{name: "extra/", dir: true}, tarEntry{name: "notes.txt", body: []byte("x")},
		tarEntry{name: dbName, body: files[dbName]}, tarEntry{name: keyName, body: files[keyName]},
	)
	_, err = Verify(withDir, t.TempDir(), active(keyOf(1)))
	assert.NoError(t, err, "目录项与不认识的文件跳过")

	_, err = Verify(filepath.Join(t.TempDir(), "nope.tar.gz"), t.TempDir(), active(keyOf(1)))
	assert.ErrorContains(t, err, "打开备份失败")
	_, err = Verify(src, filepath.Join(writeTemp(t, "x"), "sub"), active(keyOf(1)))
	assert.ErrorContains(t, err, "创建临时目录失败")
	_, err = Stage(filepath.Join(t.TempDir(), "nope.tar.gz"), "x", t.TempDir(), active(keyOf(1)), time.Now())
	assert.ErrorContains(t, err, "打开备份失败")
}

// 待恢复的说明文件坏了：读不出来；启动时整个删掉，按原来的数据启动。
func TestPending_Broken(t *testing.T) {
	e := newEnv(t, keyOf(1))
	pend := filepath.Join(e.data, PendingDir)
	require.NoError(t, os.MkdirAll(pend, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(pend, readyName), []byte("x"), 0o600))
	_, err := ReadPending(e.data)
	assert.ErrorContains(t, err, "读取待恢复失败", "没有 pending.json")
	require.NoError(t, os.WriteFile(filepath.Join(pend, pendingInfoName), []byte("{"), 0o600))
	_, err = ReadPending(e.data)
	assert.ErrorContains(t, err, "读取待恢复失败", "pending.json 不是 JSON")

	res, err := ApplyPending(e.data, false, time.Now())
	require.NoError(t, err)
	assert.False(t, res.Applied)
	assert.ErrorContains(t, res.Err, "读取待恢复失败")
	_, statErr := os.Stat(pend)
	assert.True(t, os.IsNotExist(statErr), "坏的待恢复删掉了")

	if runtime.GOOS != "windows" {
		// restore-pending 是个文件：读 ready 时不是「不存在」（Windows 上是，那里就当没有待恢复）
		require.NoError(t, os.WriteFile(pend, []byte("x"), 0o600))
		_, err = ReadPending(e.data)
		assert.ErrorContains(t, err, "读取待恢复失败")
	}
}

// 保留份数小于 1 时按 1 份；备份目录是个文件或读不了时报错。
func TestPrune_Edges(t *testing.T) {
	e := newEnv(t, keyOf(1))
	db := openDB(t)
	defer closeDB(t, db)
	base := time.Date(2026, 11, 1, 4, 30, 0, 0, time.Local)
	for i := range 3 {
		makeBackup(t, db, e.data, false, ReasonScheduled, base.Add(time.Duration(i)*time.Hour))
	}
	removed, err := Prune(filepath.Join(e.data, DefaultDirName), 0)
	require.NoError(t, err)
	assert.Len(t, removed, 2, "至少留一份")
	_, err = Prune(writeTemp(t, "x"), 7)
	assert.ErrorContains(t, err, "不是目录")
	if canLockDirs() {
		locked := filepath.Join(t.TempDir(), "locked")
		require.NoError(t, os.Mkdir(locked, 0o000))
		t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
		_, err = List(locked)
		assert.ErrorContains(t, err, "读取备份目录失败")
	}
}

// 没有 schema_versions 表或表是空的时版本是 0；没有存加密凭证的表时算没有密文；列名的叫法。
func TestSchemaVersionAndCipherCheck_EmptyDB(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "empty.db")), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	defer closeDB(t, db)
	v, err := schemaVersion(db)
	require.NoError(t, err)
	assert.Zero(t, v)
	require.NoError(t, db.Exec("CREATE TABLE schema_versions (version INTEGER)").Error)
	v, err = schemaVersion(db)
	require.NoError(t, err)
	assert.Zero(t, v)
	none, err := cipherCheck(db, func(string) ([]byte, error) { return nil, errCannotDecrypt })
	require.NoError(t, err)
	assert.True(t, none)

	assert.Equal(t, "站点 Cookie", columnLabel("site_settings"))
	assert.Equal(t, "通知通道的凭证", columnLabel("notification_conf"))
	assert.Equal(t, "CloakBrowser 令牌", columnLabel("cloak_settings"))
	assert.Equal(t, "x", columnLabel("x"))
}

// 加密列清单里的每张表都有中文叫法：校验失败时告诉用户是哪类凭证解不开，而不是表名。
func TestColumnLabel_CoversEncryptedColumns(t *testing.T) {
	for _, c := range models.EncryptedColumns {
		assert.NotEqual(t, c.Table, columnLabel(c.Table), "%s 没有中文叫法", c.Table)
	}
}

// 只有某个模块的密钥是密文时也要校验：解不开时报出是哪类凭证，解得开时不算「没有密文」。
func TestCipherCheck_ModuleSecrets(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "media.db")), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	defer closeDB(t, db)
	require.NoError(t, db.AutoMigrate(&models.MediaSetting{}))
	require.NoError(t, db.Create(&models.MediaSetting{ID: 1, TMDBKeyEncrypted: "b2xkLWNpcGhlcg=="}).Error)

	_, err = cipherCheck(db, func(string) ([]byte, error) { return nil, errCannotDecrypt })
	require.ErrorIs(t, err, errCannotDecrypt)
	assert.Contains(t, err.Error(), "TMDB")

	none, err := cipherCheck(db, func(string) ([]byte, error) { return []byte("k"), nil })
	require.NoError(t, err)
	assert.False(t, none, "库里有 TMDB Key 的密文，不能当成没有密文")
}
