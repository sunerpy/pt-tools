package crypto

import (
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// useTempHome 把 HOME 指到临时目录、清掉环境变量密钥，测试结束后换回固定的环境变量密钥。
func useTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PT_TOOLS_SECRET_KEY", "")
	t.Cleanup(func() {
		t.Setenv("PT_TOOLS_SECRET_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
		initKey()
	})
	return home
}

// secret.key 存在但内容不对时不能被新密钥覆盖：原文件可能只是被截断或写错格式，覆盖后原密钥就永久丢了。
func TestInitKey_InvalidFileIsNotOverwritten(t *testing.T) {
	home := useTempHome(t)
	keyFile := filepath.Join(home, ".pt-tools", "secret.key")
	require.NoError(t, os.MkdirAll(filepath.Dir(keyFile), 0o700))
	original := []byte(base64.StdEncoding.EncodeToString(make([]byte, 32))) // 误写成 base64
	require.NoError(t, os.WriteFile(keyFile, original, 0o600))

	initKey()

	path, generated, err := KeyStatus()
	assert.Equal(t, keyFile, path)
	assert.False(t, generated)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "64 位十六进制")

	got, rerr := os.ReadFile(keyFile)
	require.NoError(t, rerr)
	assert.Equal(t, original, got, "原文件保持不变")

	_, encErr := Encrypt([]byte("x"))
	assert.ErrorIs(t, encErr, errNoKey, "没有可用密钥时不加密")
}

func TestInitKey_UnreadableFileIsNotOverwritten(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root 不受文件权限限制")
	}
	home := useTempHome(t)
	keyFile := filepath.Join(home, ".pt-tools", "secret.key")
	require.NoError(t, os.MkdirAll(filepath.Dir(keyFile), 0o700))
	require.NoError(t, os.WriteFile(keyFile, []byte(hex.EncodeToString(make([]byte, 32))), 0o000))
	t.Cleanup(func() { _ = os.Chmod(keyFile, 0o600) })

	initKey()

	_, generated, err := KeyStatus()
	assert.False(t, generated)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "读取")
}

// 文件不存在时生成新密钥，并报告是新生成的。
func TestInitKey_ReportsGeneratedKey(t *testing.T) {
	home := useTempHome(t)

	initKey()

	path, generated, err := KeyStatus()
	require.NoError(t, err)
	assert.True(t, generated)
	assert.Equal(t, filepath.Join(home, ".pt-tools", "secret.key"), path)
	info, statErr := os.Stat(path)
	require.NoError(t, statErr)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	// 第二次启动读到的是已有文件，不再算新生成
	initKey()
	_, generated, err = KeyStatus()
	require.NoError(t, err)
	assert.False(t, generated)
}

func TestDiscardGeneratedKey(t *testing.T) {
	useTempHome(t)
	initKey()
	path, generated, _ := KeyStatus()
	require.True(t, generated)

	require.NoError(t, DiscardGeneratedKey())
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "新生成的密钥文件被删除")
	_, encErr := Encrypt([]byte("x"))
	assert.ErrorIs(t, encErr, errNoKey)
	require.NoError(t, DiscardGeneratedKey(), "重复调用无副作用")
}

// 读取的是已有密钥文件时，DiscardGeneratedKey 什么都不做。
func TestDiscardGeneratedKey_KeepsExistingFile(t *testing.T) {
	home := useTempHome(t)
	keyFile := filepath.Join(home, ".pt-tools", "secret.key")
	require.NoError(t, os.MkdirAll(filepath.Dir(keyFile), 0o700))
	require.NoError(t, os.WriteFile(keyFile, []byte(hex.EncodeToString(make([]byte, 32))), 0o600))
	initKey()

	require.NoError(t, DiscardGeneratedKey())
	_, statErr := os.Stat(keyFile)
	require.NoError(t, statErr)
	_, encErr := Encrypt([]byte("x"))
	assert.NoError(t, encErr)
}

// 检查时文件不存在、创建时却已存在（这里用悬空的符号链接模拟别处同时写入）：不覆盖，按已有文件处理。
func TestInitKey_CreateRaceDoesNotOverwrite(t *testing.T) {
	home := useTempHome(t)
	dir := filepath.Join(home, ".pt-tools")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	keyFile := filepath.Join(dir, "secret.key")
	require.NoError(t, os.Symlink(filepath.Join(dir, "missing-target"), keyFile))

	initKey()

	_, generated, err := KeyStatus()
	assert.False(t, generated)
	require.Error(t, err, "链接指向的文件读不出，记录错误而不是另写一个")
	target, lerr := os.Readlink(keyFile)
	require.NoError(t, lerr)
	assert.Equal(t, filepath.Join(dir, "missing-target"), target, "原链接保持不变")
}

func TestWriteNewKeyFile(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "secret.key")
	require.NoError(t, writeNewKeyFile(keyFile, "abc"))
	got, err := os.ReadFile(keyFile)
	require.NoError(t, err)
	assert.Equal(t, "abc", string(got))

	assert.ErrorIs(t, writeNewKeyFile(keyFile, "def"), os.ErrExist, "已存在时不覆盖")
	got, _ = os.ReadFile(keyFile)
	assert.Equal(t, "abc", string(got))

	assert.Error(t, writeNewKeyFile(filepath.Join(dir, "no-such-dir", "secret.key"), "x"))
}
