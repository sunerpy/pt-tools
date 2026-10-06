package core

import (
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/models"
)

// keyTestHome 把 HOME 指到临时目录并按其中的 secret.key 重新初始化密钥；测试结束后换回固定的环境变量密钥。
func keyTestHome(t *testing.T, keyFileContent []byte) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PT_TOOLS_SECRET_KEY", "")
	if keyFileContent != nil {
		dir := filepath.Join(home, ".pt-tools")
		require.NoError(t, os.MkdirAll(dir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "secret.key"), keyFileContent, 0o600))
	}
	crypto.ResetForTest()
	t.Cleanup(func() {
		t.Setenv("PT_TOOLS_SECRET_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
		crypto.ResetForTest()
	})
	return home
}

func keyCheckDB(t *testing.T, withCipher bool) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}, &models.NotificationConf{}))
	if withCipher {
		require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", CookieEncrypted: "b2xkLWNpcGhlcg=="}).Error)
	}
	return db
}

// secret.key 存在却无法使用时拒绝启动，并说明怎么恢复，而不是换一把新密钥继续跑。
func TestCheckSecretKeyFile_RejectsUnusableKeyFile(t *testing.T) {
	keyTestHome(t, []byte("not-a-key"))
	err := checkSecretKeyFile()
	require.ErrorIs(t, err, ErrSecretKeyUnusable)
	assert.Contains(t, err.Error(), "secret import")
}

func TestCheckSecretKeyFile_AcceptsValidKey(t *testing.T) {
	keyTestHome(t, []byte(hex.EncodeToString(make([]byte, 32))))
	assert.NoError(t, checkSecretKeyFile())
}

// 密钥文件原本不存在、库里却已有密文：原密钥丢了或没挂载进来，拒绝启动并删掉刚生成的密钥。
func TestCheckGeneratedSecretKey_RejectsWhenDataIsEncrypted(t *testing.T) {
	home := keyTestHome(t, nil)
	keyFile := filepath.Join(home, ".pt-tools", "secret.key")
	_, generated, _ := crypto.KeyStatus()
	require.True(t, generated)

	err := checkGeneratedSecretKey(keyCheckDB(t, true))
	require.ErrorIs(t, err, ErrSecretKeyRegenerated)
	assert.Contains(t, err.Error(), acceptNewSecretKeyEnv)
	_, statErr := os.Stat(keyFile)
	assert.True(t, os.IsNotExist(statErr), "新生成的密钥被删除，恢复原文件后下次启动不会误用它")
}

// 用户确认原密钥已丢失时放行，保留新密钥。
func TestCheckGeneratedSecretKey_AcceptNewKeyOptIn(t *testing.T) {
	home := keyTestHome(t, nil)
	t.Setenv(acceptNewSecretKeyEnv, "1")

	require.NoError(t, checkGeneratedSecretKey(keyCheckDB(t, true)))
	_, statErr := os.Stat(filepath.Join(home, ".pt-tools", "secret.key"))
	assert.NoError(t, statErr)
}

// 全新安装（库里没有密文）照常生成密钥启动；读取已有密钥文件时不检查。
func TestCheckGeneratedSecretKey_FreshInstallAndExistingKey(t *testing.T) {
	keyTestHome(t, nil)
	assert.NoError(t, checkGeneratedSecretKey(keyCheckDB(t, false)))

	keyTestHome(t, []byte(hex.EncodeToString(make([]byte, 32))))
	assert.NoError(t, checkGeneratedSecretKey(keyCheckDB(t, true)))
}
