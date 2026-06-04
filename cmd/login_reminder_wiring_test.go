package cmd

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestWireLoginReminderMonitor_RegistersNonNil(t *testing.T) {
	global.InitLogger(zap.NewNop())

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.SiteSetting{},
		&models.SiteLoginState{},
		&models.NotificationConf{},
	))

	prevDB := global.GlobalDB
	global.GlobalDB = &models.TorrentDB{DB: db}
	t.Cleanup(func() { global.GlobalDB = prevDB })

	store := core.NewConfigStore(global.GlobalDB)
	siteRegistry := v2.NewSiteRegistry(global.GetLogger())
	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)

	require.Nil(t, mgr.GetLoginReminderMonitor(),
		"precondition: monitor must be nil before wiring (this nil is the 503 cause)")

	wireLoginReminderMonitor(mgr, store, siteRegistry, nil)

	require.NotNil(t, mgr.GetLoginReminderMonitor(),
		"after wiring, GetLoginReminderMonitor must be non-nil so the probe endpoint stops returning 503")
}

func TestLoginReminderConfLister_DecryptsConfigJSON(t *testing.T) {
	global.InitLogger(zap.NewNop())

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.NotificationConf{}))

	plaintext := `{"bot_token":"123:ABC","chat_id":"456"}`
	cipher, err := crypto.Encrypt([]byte(plaintext))
	require.NoError(t, err)
	require.NotEqual(t, plaintext, cipher, "stored ConfigJSON must be ciphertext")

	require.NoError(t, db.Create(&models.NotificationConf{
		ChannelType: "telegram",
		Name:        "tg-test",
		ConfigJSON:  cipher,
		Enabled:     true,
	}).Error)

	lister := loginReminderConfLister{db: db}
	confs, err := lister.ListNotificationConfs(context.Background())
	require.NoError(t, err)
	require.Len(t, confs, 1)
	require.Equal(t, plaintext, confs[0].ConfigJSON,
		"ConfLister must return DECRYPTED plaintext; the telegram adapter json.Unmarshals it directly, so ciphertext here reproduces the 'invalid character T' regression")
}
