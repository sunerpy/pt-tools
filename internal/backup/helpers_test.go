package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"moul.io/zapgorm2"

	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/models"
)

// env 是一个测试用的 pt-tools 数据目录：HOME 指到临时目录，密钥在 ~/.pt-tools/secret.key（或环境变量）里。
type env struct {
	home, data string
}

// newEnv 建数据目录并按 key 写 secret.key（nil 表示没有文件，由 crypto 新生成）；测试结束后换回固定的环境变量密钥。
func newEnv(t *testing.T, key []byte) env {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PT_TOOLS_SECRET_KEY", "")
	data := filepath.Join(home, models.WorkDir)
	require.NoError(t, os.MkdirAll(data, 0o700))
	if key != nil {
		require.NoError(t, os.WriteFile(filepath.Join(data, keyName), []byte(hex.EncodeToString(key)), 0o600))
	}
	crypto.ResetForTest()
	t.Cleanup(func() {
		t.Setenv("PT_TOOLS_SECRET_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
		crypto.ResetForTest()
	})
	return env{home: home, data: data}
}

func keyOf(b byte) []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = b
	}
	return k
}

// openDB 打开（必要时建）数据目录里的 torrents.db。
func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	tdb, err := models.NewDB(zapgorm2.Logger{ZapLogger: zap.NewNop()})
	require.NoError(t, err)
	return tdb.DB
}

func closeDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

// addCookie 用当前进程的密钥加密写一条站点 Cookie。
func addCookie(t *testing.T, db *gorm.DB, site, cookie string) {
	t.Helper()
	enc, err := crypto.Encrypt([]byte(cookie))
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.SiteSetting{Name: site, AuthMethod: "cookie", CookieEncrypted: enc}).Error)
}

func currentKey(t *testing.T) Key {
	t.Helper()
	k, err := crypto.ExportKey()
	require.NoError(t, err)
	return Key{Bytes: k, Source: SecretFromFile}
}

// makeBackup 在数据目录的 backups 里做一份备份。
func makeBackup(t *testing.T, db *gorm.DB, data string, withKey bool, reason Reason, at time.Time) Info {
	t.Helper()
	info, err := Create(context.Background(), db, Options{
		Dir: filepath.Join(data, DefaultDirName), Reason: reason, IncludeSecret: withKey, AppVersion: "test",
		Key: func() (Key, error) { return currentKey(t), nil }, Now: func() time.Time { return at },
	})
	require.NoError(t, err)
	return info
}

// entries 读出备份里的文件名与内容（按顺序）。
func entries(t *testing.T, path string) ([]string, map[string][]byte) {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	gz, err := gzip.NewReader(f)
	require.NoError(t, err)
	tr := tar.NewReader(gz)
	var names []string
	files := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		b, err := io.ReadAll(tr)
		require.NoError(t, err)
		names = append(names, hdr.Name)
		files[hdr.Name] = b
	}
	return names, files
}

// rewrite 按 edit 改写一份备份里的文件，另存一个新文件（模拟被改过、做坏了的备份）。
func rewrite(t *testing.T, src string, edit func(files map[string][]byte, order []string) []string) string {
	t.Helper()
	order, files := entries(t, src)
	order = edit(files, order)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, n := range order {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: n, Mode: 0o600, Size: int64(len(files[n])), Typeflag: tar.TypeReg}))
		_, err := tw.Write(files[n])
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	out := filepath.Join(t.TempDir(), "edited.tar.gz")
	require.NoError(t, os.WriteFile(out, buf.Bytes(), 0o600))
	return out
}

func setManifest(t *testing.T, files map[string][]byte, edit func(m *Manifest)) {
	t.Helper()
	var m Manifest
	require.NoError(t, json.Unmarshal(files[manifestName], &m))
	edit(&m)
	raw, err := json.Marshal(m)
	require.NoError(t, err)
	files[manifestName] = raw
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
