package cmd

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestLoginReminderDecryptor_Decrypt(t *testing.T) {
	global.InitLogger(zap.NewNop())
	t.Setenv("PT_TOOLS_SECRET_KEY", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	global.GlobalDB = &models.TorrentDB{DB: db}
	store := core.NewConfigStore(global.GlobalDB)

	d := loginReminderDecryptor{store: store}

	// Empty cookie -> empty plaintext, no error.
	plain, err := d.Decrypt(models.SiteSetting{})
	require.NoError(t, err)
	assert.Empty(t, plain)

	// Round-trip a real cookie through the store's encrypt/decrypt.
	cipher, err := store.EncryptCookie("session=abc123")
	require.NoError(t, err)
	got, err := d.Decrypt(models.SiteSetting{CookieEncrypted: cipher})
	require.NoError(t, err)
	assert.Equal(t, "session=abc123", got)
}

func TestLoginReminderResolver_Resolve_DecryptError(t *testing.T) {
	global.InitLogger(zap.NewNop())
	t.Setenv("PT_TOOLS_SECRET_KEY", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	global.GlobalDB = &models.TorrentDB{DB: db}
	store := core.NewConfigStore(global.GlobalDB)

	resolver := loginReminderResolver{
		registry:  v2.NewSiteRegistry(global.GetLogger()),
		decryptor: loginReminderDecryptor{store: store},
	}

	// A non-base64 / undecryptable cookie must surface as an error from Resolve.
	_, _, err = resolver.Resolve(models.SiteSetting{Name: "hdsky", CookieEncrypted: "not-valid-cipher"})
	require.Error(t, err)
}

func TestLoginReminderResolver_Resolve_UnknownSite(t *testing.T) {
	global.InitLogger(zap.NewNop())
	t.Setenv("PT_TOOLS_SECRET_KEY", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	global.GlobalDB = &models.TorrentDB{DB: db}
	store := core.NewConfigStore(global.GlobalDB)

	resolver := loginReminderResolver{
		registry:  v2.NewSiteRegistry(global.GetLogger()),
		decryptor: loginReminderDecryptor{store: store},
	}

	// Empty cookie decrypts fine (empty); an unregistered site name makes
	// CreateSite fail, so Resolve returns an error.
	_, _, err = resolver.Resolve(models.SiteSetting{Name: "does-not-exist"})
	require.Error(t, err)
}

func TestLoginReminderConfLister_SkipsUndecryptable(t *testing.T) {
	global.InitLogger(zap.NewNop())
	t.Setenv("PT_TOOLS_SECRET_KEY", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.NotificationConf{}))

	// Enabled conf with garbage ConfigJSON is skipped (decrypt fails).
	require.NoError(t, db.Create(&models.NotificationConf{
		ChannelType: "telegram", Name: "bad", ConfigJSON: "garbage", Enabled: true,
	}).Error)
	// Enabled conf with empty ConfigJSON passes through untouched.
	require.NoError(t, db.Create(&models.NotificationConf{
		ChannelType: "qq_onebot", Name: "empty", ConfigJSON: "", Enabled: true,
	}).Error)

	lister := loginReminderConfLister{db: db}
	confs, err := lister.ListNotificationConfs(t.Context())
	require.NoError(t, err)
	require.Len(t, confs, 1, "undecryptable conf should be skipped")
	assert.Equal(t, "empty", confs[0].Name)
}
