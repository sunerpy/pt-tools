package cmd

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/chatops"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// newChatOpsTestDB returns an in-memory DB migrated with the chatops + config
// tables that the web.go wiring helpers query.
func newChatOpsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	global.InitLogger(zap.NewNop())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.SiteSetting{},
		&models.RSSSubscription{},
		&models.DownloaderSetting{},
		&models.FilterRule{},
		&models.NotificationConf{},
		&models.ChannelBinding{},
		&models.ActionAudit{},
		&models.BotToken{},
		&models.RSSFilterAssociation{},
	))
	return db
}

func TestNopLogger_NoPanic(t *testing.T) {
	var l nopLogger
	assert.NotPanics(t, func() {
		l.Infof("x %d", 1)
		l.Warnf("y %s", "z")
	})
}

func TestChatopsLogger_FallsBackToNop(t *testing.T) {
	prev := global.GlobalLogger
	global.GlobalLogger = nil
	t.Cleanup(func() { global.GlobalLogger = prev })
	log := chatopsLogger()
	require.NotNil(t, log)
	assert.NotPanics(t, func() { log.Infof("hi"); log.Warnf("bye") })
}

func TestChatopsBootstrap_NilReceiverSafe(t *testing.T) {
	var b *chatopsBootstrap
	assert.Nil(t, b.Deps())
	assert.Nil(t, b.Chain())
	assert.Equal(t, 0, b.ChannelCount())
	require.NoError(t, b.Shutdown(context.Background()))
}

func TestChatopsRSSWizardService_CRUD(t *testing.T) {
	db := newChatOpsTestDB(t)
	global.GlobalDB = &models.TorrentDB{DB: db}
	store := core.NewConfigStore(global.GlobalDB)

	// Seed a site to append RSS onto.
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", AuthMethod: "cookie", Enabled: true}).Error)
	// Seed reference rows for the list helpers.
	require.NoError(t, db.Create(&models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://x", IsDefault: true}).Error)
	require.NoError(t, db.Create(&models.FilterRule{Name: "rule", Pattern: "x", Enabled: true}).Error)
	require.NoError(t, db.Create(&models.NotificationConf{ChannelType: "telegram", Name: "tg", Enabled: true}).Error)

	svc := &chatopsRSSWizardService{store: store, db: db}
	ctx := context.Background()

	created, err := svc.AppendRSSToSite("hdsky", models.RSSConfig{Name: "sub", URL: "http://feed"})
	require.NoError(t, err)
	assert.NotZero(t, created.ID)

	list, err := svc.ListRSSForSite("hdsky")
	require.NoError(t, err)
	require.Len(t, list, 1)

	dls, err := svc.ListDownloaders(ctx)
	require.NoError(t, err)
	require.Len(t, dls, 1)
	assert.Equal(t, "qb", dls[0].Name)

	rules, err := svc.ListFilterRules(ctx)
	require.NoError(t, err)
	require.Len(t, rules, 1)

	chans, err := svc.ListNotificationChannels(ctx)
	require.NoError(t, err)
	require.Len(t, chans, 1)

	deleted, err := svc.DeleteRSSFromSite("hdsky", created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, deleted.ID)
}

func TestDBBindingLookup_FindByChannelUser(t *testing.T) {
	db := newChatOpsTestDB(t)
	require.NoError(t, db.Create(&models.ChannelBinding{
		NotificationConfID: 7, ChannelType: "telegram", ChannelUserID: "u1",
		PtAdmin: true, Allowed: true, ReplyLang: "zh",
	}).Error)

	lookup := &dbBindingLookup{db: db}
	ctx := context.Background()

	info, ok, err := lookup.FindByChannelUser(ctx, "telegram", "u1")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, uint(7), info.ConfID)
	assert.True(t, info.PtAdmin)

	_, ok, err = lookup.FindByChannelUser(ctx, "telegram", "missing")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestCommandsBindingResolver_FindByChannelUser(t *testing.T) {
	db := newChatOpsTestDB(t)
	require.NoError(t, db.Create(&models.ChannelBinding{
		ID: 42, NotificationConfID: 1, ChannelType: "qq_onebot", ChannelUserID: "abc", Allowed: true,
	}).Error)
	resolver := &commandsBindingResolver{lookup: &dbBindingLookup{db: db}}

	id, ok, err := resolver.FindByChannelUser(context.Background(), "qq_onebot", "abc")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, uint(42), id)

	_, ok, err = resolver.FindByChannelUser(context.Background(), "qq_onebot", "none")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestAuditRecorderAdapter_Record(t *testing.T) {
	db := newChatOpsTestDB(t)
	rec := &auditRecorderAdapter{svc: app.NewAuditService(db)}
	err := rec.Record(context.Background(), chatops.AuditEntry{
		NotificationConfID: 1, ChannelType: "telegram", ChannelUserID: "u",
		Command: "/status", Result: "ok", LatencyMs: 3,
	})
	require.NoError(t, err)
	var count int64
	require.NoError(t, db.Model(&models.ActionAudit{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestLiveNotifyManager_Send(t *testing.T) {
	t.Run("nil manager errors", func(t *testing.T) {
		var m *liveNotifyManager
		err := m.Send(context.Background(), 1, app.Notification{Text: "x"})
		require.Error(t, err)
	})
	t.Run("missing channel errors", func(t *testing.T) {
		m := newLiveNotifyManager(nil)
		err := m.Send(context.Background(), 9, app.Notification{Text: "x"})
		require.Error(t, err)
	})
	t.Run("delivers to registered channel", func(t *testing.T) {
		ch := &stubNotifyChannel{}
		m := newLiveNotifyManager(map[uint]notify.Channel{5: ch})
		require.NoError(t, m.Send(context.Background(), 5, app.Notification{
			Title: "t", Text: "body", SourceConfID: 5,
		}))
		require.Len(t, ch.recorded, 1)
		assert.Equal(t, "body", ch.recorded[0].Text)
	})
}

func TestLiveNotifyManager_SetChannels_NilResets(t *testing.T) {
	m := newLiveNotifyManager(map[uint]notify.Channel{1: &stubNotifyChannel{}})
	m.SetChannels(nil)
	err := m.Send(context.Background(), 1, app.Notification{Text: "x"})
	require.Error(t, err, "after reset channel 1 should be gone")
}

func TestRSSNotifierAdapter_Delegates(t *testing.T) {
	rec := &recordingRSSNotifier{}
	adapter := &rssNotifierAdapter{inner: rec}
	require.NoError(t, adapter.NotifyNewItem(context.Background(), internal.RSSItemNotice{
		SiteName: "hdsky", TorrentID: "1",
	}))
	require.NoError(t, adapter.NotifyFilteredItem(context.Background(), internal.RSSFilteredNotice{
		SiteName: "hdsky", TorrentID: "2",
	}))
	assert.Equal(t, 1, rec.newCalls)
	assert.Equal(t, 1, rec.filteredCalls)
}

type recordingRSSNotifier struct {
	newCalls      int
	filteredCalls int
}

func (r *recordingRSSNotifier) NotifyNewItem(_ context.Context, _ app.RSSItemEvent) error {
	r.newCalls++
	return nil
}

func (r *recordingRSSNotifier) NotifyFilteredItem(_ context.Context, _ app.RSSFilteredEvent) error {
	r.filteredCalls++
	return nil
}

func TestGetRegisteredSitesFromRegistry(t *testing.T) {
	global.InitLogger(zap.NewNop())
	reg := v2.NewSiteRegistry(global.GetLogger())
	sites := getRegisteredSitesFromRegistry(reg)
	// Site definitions are registered via root.go's side-effect import.
	assert.NotEmpty(t, sites, "expected registered sites from definition registry")
	for _, s := range sites {
		assert.NotEmpty(t, s.ID)
		assert.NotEmpty(t, s.AuthMethod)
	}
}

func TestBootstrapChatOps_GuardClauses(t *testing.T) {
	global.InitLogger(zap.NewNop())
	store := core.NewConfigStore(&models.TorrentDB{})
	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)

	_, err := bootstrapChatOps(context.Background(), nil, mgr, store)
	require.Error(t, err)

	db := newChatOpsTestDB(t)
	tdb := &models.TorrentDB{DB: db}
	_, err = bootstrapChatOps(context.Background(), tdb, nil, store)
	require.Error(t, err)

	_, err = bootstrapChatOps(context.Background(), tdb, mgr, nil)
	require.Error(t, err)
}

func TestReloadChatOpsChannels_RebuildsFromDB(t *testing.T) {
	db := newChatOpsTestDB(t)
	global.GlobalDB = &models.TorrentDB{DB: db}
	store := core.NewConfigStore(global.GlobalDB)
	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bs, err := bootstrapChatOps(ctx, global.GlobalDB, mgr, store)
	require.NoError(t, err)
	require.NotNil(t, bs)
	assert.Equal(t, 0, bs.ChannelCount())

	// No enabled channels -> reload keeps channel map empty and returns nil.
	require.NoError(t, reloadChatOpsChannels(ctx, db, bs, nil))
	assert.Equal(t, 0, bs.ChannelCount())

	shutdownCtx, sc := context.WithTimeout(context.Background(), 2*time.Second)
	defer sc()
	require.NoError(t, bs.Shutdown(shutdownCtx))
}

func TestRunChatOpsChannelReloader_ExitsOnCancel(t *testing.T) {
	db := newChatOpsTestDB(t)
	global.GlobalDB = &models.TorrentDB{DB: db}
	store := core.NewConfigStore(global.GlobalDB)
	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)

	ctx, cancel := context.WithCancel(context.Background())
	bs, err := bootstrapChatOps(ctx, global.GlobalDB, mgr, store)
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		runChatOpsChannelReloader(ctx, db, bs, nil)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("reloader did not exit on context cancel")
	}
}

func TestRunChatOpsChannelReloader_NilArgsReturn(t *testing.T) {
	// nil bs / nil db should return immediately without blocking.
	done := make(chan struct{})
	go func() {
		runChatOpsChannelReloader(context.Background(), nil, nil, nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("expected immediate return on nil args")
	}
}

func TestTruncateLog(t *testing.T) {
	assert.Equal(t, "abc", truncateLog("abc", 5))
	assert.Equal(t, "abcde...", truncateLog("abcdefgh", 5))
}
