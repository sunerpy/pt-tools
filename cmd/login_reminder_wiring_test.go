package cmd

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/web"
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

// fakeNotificationSvc 只实现 PushSync / Push；其余方法沿用嵌入的 nil 接口，被调用就会 panic，
// 用来证明登录监控只走 PushSync。
type fakeNotificationSvc struct {
	app.NotificationService
	pushSync atomic.Int32
	push     atomic.Int32
	lastConf atomic.Uint32
}

func (f *fakeNotificationSvc) PushSync(_ context.Context, n app.Notification) error {
	f.pushSync.Add(1)
	f.lastConf.Store(uint32(n.SourceConfID))
	return nil
}

func (f *fakeNotificationSvc) Push(context.Context, app.Notification) error {
	f.push.Add(1)
	return nil
}

type countingChannel struct{}

func (countingChannel) Type() string                                         { return "counting" }
func (countingChannel) Init(context.Context, *models.NotificationConf) error { return nil }
func (countingChannel) SupportsInbound() bool                                { return false }
func (countingChannel) Send(context.Context, notify.Notification) error      { return nil }
func (countingChannel) OnInbound(notify.InboundHandler)                      {}
func (countingChannel) Close(context.Context) error                          { return nil }
func (countingChannel) Healthy() bool                                        { return true }

// M1 问题 12：登录监控经 ChatOps 的 live 通道发送，不再用 registry.Make 另起通道实例。
func TestWireLoginReminderMonitor_UsesLiveChannelsNotNewInstances(t *testing.T) {
	global.InitLogger(zap.NewNop())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}, &models.SiteLoginState{}, &models.NotificationConf{},
		&models.MigrationState{}, &models.MonitorNotificationLog{}))
	prevDB := global.GlobalDB
	global.GlobalDB = &models.TorrentDB{DB: db}
	t.Cleanup(func() { global.GlobalDB = prevDB })
	require.NoError(t, db.Create(&models.NotificationConf{ChannelType: "counting", Name: "c", Enabled: true}).Error)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", Enabled: true, AuthMethod: "cookie"}).Error)

	var makes atomic.Int32
	registry := notify.NewRegistry()
	registry.Register("counting", func() notify.Channel { makes.Add(1); return countingChannel{} })
	svc := &fakeNotificationSvc{}
	bs := &chatopsBootstrap{registry: registry, deps: &web.ChatOpsDeps{NotificationSvc: svc}}

	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)
	wireLoginReminderMonitor(mgr, core.NewConfigStore(global.GlobalDB), v2.NewSiteRegistry(global.GetLogger()), bs)
	mon := mgr.GetLoginReminderMonitor()
	require.NotNil(t, mon)
	require.True(t, mon.Notifier().Enabled())

	require.NoError(t, mon.SendTestReminder(context.Background(), "hdsky"))
	assert.Equal(t, int32(1), svc.pushSync.Load(), "delivered through the live manager")
	assert.Equal(t, int32(0), svc.push.Load(), "Push would fall back to the outbox, which cannot drive QQ/Telegram")
	assert.Equal(t, int32(0), makes.Load(), "no channel instance is created for login notifications")
	assert.Equal(t, uint32(1), svc.lastConf.Load())
}

func TestWireLoginReminderMonitor_WithoutChatOpsRecordsOnly(t *testing.T) {
	global.InitLogger(zap.NewNop())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}, &models.SiteLoginState{}, &models.NotificationConf{},
		&models.MigrationState{}, &models.MonitorNotificationLog{}))
	prevDB := global.GlobalDB
	global.GlobalDB = &models.TorrentDB{DB: db}
	t.Cleanup(func() { global.GlobalDB = prevDB })
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)
	wireLoginReminderMonitor(mgr, core.NewConfigStore(global.GlobalDB), v2.NewSiteRegistry(global.GetLogger()), nil)
	mon := mgr.GetLoginReminderMonitor()
	require.NotNil(t, mon)
	assert.False(t, mon.Notifier().Enabled())
	assert.Nil(t, loginReminderSender(nil))
	assert.Nil(t, loginReminderSender(&chatopsBootstrap{}))
	assert.ErrorContains(t, mon.SendTestReminder(context.Background(), "hdsky"), "通知")
}
