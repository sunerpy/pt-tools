package cmd

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// emptyDB returns an unmigrated in-memory DB so queries against missing tables
// surface errors, exercising each helper's error branch.
func emptyDB(t *testing.T) *gorm.DB {
	t.Helper()
	global.InitLogger(zap.NewNop())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	return db
}

func TestDBBindingLookup_FindByChannelUser_QueryError(t *testing.T) {
	lookup := &dbBindingLookup{db: emptyDB(t)}
	_, ok, err := lookup.FindByChannelUser(context.Background(), "telegram", "u")
	require.Error(t, err)
	assert.False(t, ok)
}

func TestCommandsBindingResolver_QueryError(t *testing.T) {
	resolver := &commandsBindingResolver{lookup: &dbBindingLookup{db: emptyDB(t)}}
	_, ok, err := resolver.FindByChannelUser(context.Background(), "telegram", "u")
	require.Error(t, err)
	assert.False(t, ok)
}

func TestChatopsRSSWizardService_ListErrors(t *testing.T) {
	svc := &chatopsRSSWizardService{db: emptyDB(t)}
	ctx := context.Background()

	_, err := svc.ListDownloaders(ctx)
	require.Error(t, err)
	_, err = svc.ListFilterRules(ctx)
	require.Error(t, err)
	_, err = svc.ListNotificationChannels(ctx)
	require.Error(t, err)
}

func TestLoginReminderConfLister_QueryError(t *testing.T) {
	lister := loginReminderConfLister{db: emptyDB(t)}
	_, err := lister.ListNotificationConfs(context.Background())
	require.Error(t, err)
}

func TestWireLoginReminderMonitor_NilDB(t *testing.T) {
	global.InitLogger(zap.NewNop())
	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })

	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)
	wireLoginReminderMonitor(mgr, nil, nil, nil)
	assert.Nil(t, mgr.GetLoginReminderMonitor(), "nil DB must skip monitor wiring")
}

func TestLoginReminderResolver_Resolve_Success(t *testing.T) {
	global.InitLogger(zap.NewNop())
	t.Setenv("PT_TOOLS_SECRET_KEY", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")

	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	store := core.NewConfigStore(db)

	cipher, err := store.EncryptCookie("c_secure_uid=1")
	require.NoError(t, err)

	resolver := loginReminderResolver{
		registry:  v2.NewSiteRegistry(global.GetLogger()),
		decryptor: loginReminderDecryptor{store: store},
	}
	def, site, err := resolver.Resolve(models.SiteSetting{Name: "hdsky", CookieEncrypted: cipher})
	require.NoError(t, err)
	assert.NotNil(t, site)
	assert.NotNil(t, def)
}
