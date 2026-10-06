package models

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupV10MigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "v10.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&SchemaVersion{}, &SiteSetting{}))
	require.NoError(t, db.Create(&SchemaVersion{Version: 10, Description: "test v10", AppVersion: "test"}).Error)
	rows := []SiteSetting{
		{Name: "plain-only", AuthMethod: "cookie", Cookie: "uid=1; pass=a"},
		{Name: "both", AuthMethod: "cookie", Cookie: "uid=2; pass=b", CookieEncrypted: "cipher:uid=2; pass=b"},
		{Name: "bad-cipher", AuthMethod: "cookie", Cookie: "uid=3; pass=c", CookieEncrypted: "garbage"},
		{Name: "cipher-only", AuthMethod: "cookie", CookieEncrypted: "cipher:uid=4; pass=d"},
		{Name: "no-cookie", AuthMethod: "api_key", APIKey: "k"},
	}
	for i := range rows {
		require.NoError(t, db.Create(&rows[i]).Error)
	}
	return db
}

func loadSite(t *testing.T, db *gorm.DB, name string) SiteSetting {
	t.Helper()
	var s SiteSetting
	require.NoError(t, db.Where("name = ?", name).First(&s).Error)
	return s
}

// v11：明文 Cookie 只留密文。只有明文的行先加密；密文解不开时用明文重新加密，免得丢掉唯一能用的副本；
// 已有可用密文的行只清明文。不写表备份，免得把要清除的明文再写一份到磁盘。
func TestMigrationV10ToV11ClearsPlaintextCookies(t *testing.T) {
	db := setupV10MigrationDB(t)
	hooks := &spyHooks{}
	sm := NewSchemaManagerWithHooks(db, "test", hooks.BackupTable, hooks.EncryptCookie, hooks.DecryptCookie)
	require.NoError(t, sm.RunMigrations())

	for name, want := range map[string]string{
		"plain-only":  "uid=1; pass=a",
		"both":        "uid=2; pass=b",
		"bad-cipher":  "uid=3; pass=c",
		"cipher-only": "uid=4; pass=d",
	} {
		s := loadSite(t, db, name)
		assert.Empty(t, s.Cookie, name)
		plain, err := hooks.DecryptCookie(s.CookieEncrypted)
		require.NoError(t, err, name)
		assert.Equal(t, want, plain, name)
	}
	assert.Empty(t, loadSite(t, db, "no-cookie").CookieEncrypted)
	assert.Equal(t, "k", loadSite(t, db, "no-cookie").APIKey)
	assert.Zero(t, hooks.backupCalls.Load(), "no plaintext backup is written")

	version, err := sm.GetCurrentVersion()
	require.NoError(t, err)
	assert.Equal(t, 11, version)
	assert.Equal(t, 11, CurrentSchemaVersion)

	// 再跑一次没有可迁移的行，什么也不做。
	encrypts := hooks.encryptCalls.Load()
	require.NoError(t, sm.migrateV10ToV11(db))
	assert.Equal(t, encrypts, hooks.encryptCalls.Load())
}

// 加密失败时整批回滚，明文不会被清掉。
func TestMigrationV10ToV11RollsBackOnEncryptFailure(t *testing.T) {
	db := setupV10MigrationDB(t)
	hooks := &spyHooks{failEncrypt: 2}
	sm := NewSchemaManagerWithHooks(db, "test", hooks.BackupTable, hooks.EncryptCookie, hooks.DecryptCookie)
	require.Error(t, sm.RunMigrations())

	assert.Equal(t, "uid=1; pass=a", loadSite(t, db, "plain-only").Cookie)
	assert.Equal(t, "uid=2; pass=b", loadSite(t, db, "both").Cookie)
	assert.Equal(t, "uid=3; pass=c", loadSite(t, db, "bad-cipher").Cookie)
	version, err := sm.GetCurrentVersion()
	require.NoError(t, err)
	assert.Equal(t, 10, version)
}

// 没有明文行（或者没有 site_settings 表）时不需要加密钩子。
func TestMigrationV10ToV11NoPlaintextNeedsNoHooks(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "empty.db")), &gorm.Config{})
	require.NoError(t, err)
	sm := NewSchemaManager(db, "test")
	require.NoError(t, sm.migrateV10ToV11(db), "no site_settings table")

	require.NoError(t, db.AutoMigrate(&SiteSetting{}))
	require.NoError(t, db.Create(&SiteSetting{Name: "cipher-only", CookieEncrypted: "cipher:x"}).Error)
	require.NoError(t, sm.migrateV10ToV11(db), "no plaintext rows")

	require.NoError(t, db.Create(&SiteSetting{Name: "plain", Cookie: "x=1"}).Error)
	assert.Error(t, sm.migrateV10ToV11(db), "plaintext rows need the crypto hooks")
}
