package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/chatops"
	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

type stubNotifyChannel struct {
	recorded []notify.Notification
}

func (s *stubNotifyChannel) Type() string { return "stub" }

func (s *stubNotifyChannel) Init(_ context.Context, _ *models.NotificationConf) error { return nil }

func (s *stubNotifyChannel) SupportsInbound() bool { return true }

func (s *stubNotifyChannel) Send(_ context.Context, n notify.Notification) error {
	s.recorded = append(s.recorded, n)
	return nil
}

func (s *stubNotifyChannel) OnInbound(_ notify.InboundHandler) {}

func (s *stubNotifyChannel) Close(_ context.Context) error { return nil }

func (s *stubNotifyChannel) Healthy() bool { return true }

func TestWebCmdHasFlags(t *testing.T) {
	c := &cobra.Command{}
	// init attaches flags in package init; verify default values accessible via webCmd
	fHost := webCmd.Flags().Lookup("host")
	fPort := webCmd.Flags().Lookup("port")
	assert.NotNil(t, fHost)
	assert.NotNil(t, fPort)
	_ = c
}

// migrateChatOpsTablesForTest applies AutoMigrate for chatops tables on the
// test DB (testutil.NewTempDBDir does not include them by design).
func migrateChatOpsTablesForTest(t *testing.T, db *models.TorrentDB) {
	t.Helper()
	require.NoError(t, db.DB.AutoMigrate(
		&models.NotificationConf{},
		&models.ChannelBinding{},
		&models.ActionAudit{},
		&models.BotToken{},
		&models.NotificationOutbox{},
	))
}

// TestCmdWeb_StartsWithDisabledChatOps verifies that bootstrapChatOps runs
// cleanly when no NotificationConf rows exist (disabled chatops).
func TestCmdWeb_StartsWithDisabledChatOps(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	migrateChatOpsTablesForTest(t, db)
	global.GlobalDB = db

	store := core.NewConfigStore(db)
	mgr := scheduler.NewManager()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bs, err := bootstrapChatOps(ctx, db, mgr, store)
	require.NoError(t, err, "bootstrap with no NotificationConf must succeed")
	require.NotNil(t, bs, "bootstrap must return non-nil result")
	require.NotNil(t, bs.Deps(), "ChatOpsDeps must be populated")

	// Channels map is empty (no enabled confs).
	assert.Equal(t, 0, bs.ChannelCount(), "expected zero initialized channels")

	// Graceful shutdown does not panic and completes promptly.
	shutdownCtx, sCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer sCancel()
	require.NoError(t, bs.Shutdown(shutdownCtx))
}

// TestCmdWeb_StartsWithEnabledTelegramConf_NoCrash verifies that a Telegram
// NotificationConf with a syntactically valid encrypted blob but a fake bot
// token does NOT prevent bootstrap from completing. Channel.Init may fail —
// the failure must be logged and the channel marked disabled, never panic
// or block the boot.
func TestCmdWeb_StartsWithEnabledTelegramConf_NoCrash(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	migrateChatOpsTablesForTest(t, db)
	global.GlobalDB = db

	// Provide a deterministic secret key for crypto.Encrypt.
	t.Setenv("PT_TOOLS_SECRET_KEY", "0123456789abcdef0123456789abcdef")
	// Ensure no leftover key file interferes with this test environment.
	_ = os.Unsetenv("PT_TOOLS_SECRET_KEY_FILE")

	tgConfig := map[string]any{
		"bot_token":       "0:invalid-fake-token",
		"allowed_users":   []int64{1},
		"admin_users":     []int64{1},
		"default_chat_id": 1,
	}
	plain, err := json.Marshal(tgConfig)
	require.NoError(t, err)
	cipherStr, err := crypto.Encrypt(plain)
	require.NoError(t, err)

	conf := &models.NotificationConf{
		ChannelType: "telegram",
		Name:        "test-telegram",
		ConfigJSON:  cipherStr,
		Enabled:     true,
	}
	require.NoError(t, db.DB.Create(conf).Error)

	store := core.NewConfigStore(db)
	mgr := scheduler.NewManager()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bs, err := bootstrapChatOps(ctx, db, mgr, store)
	// Bootstrap MUST succeed even when a single channel's Init fails.
	require.NoError(t, err)
	require.NotNil(t, bs)
	require.NotNil(t, bs.Deps())

	// Graceful shutdown still works regardless of unhealthy channels.
	shutdownCtx, sCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer sCancel()
	require.NoError(t, bs.Shutdown(shutdownCtx))
}

func TestLiveNotifyManagerReply(t *testing.T) {
	msg := notify.InboundMessage{
		ChannelType:   "qq_onebot",
		SourceConfID:  1,
		ChannelUserID: "429471838",
		ChatID:        "274984594",
	}

	t.Run("empty reply is ignored", func(t *testing.T) {
		ch := &stubNotifyChannel{}
		mgr := newLiveNotifyManager(map[uint]notify.Channel{msg.SourceConfID: ch})

		err := mgr.Reply(context.Background(), msg, chatops.Reply{})

		require.NoError(t, err)
		assert.Empty(t, ch.recorded)
	})

	t.Run("silent drop is ignored", func(t *testing.T) {
		ch := &stubNotifyChannel{}
		mgr := newLiveNotifyManager(map[uint]notify.Channel{msg.SourceConfID: ch})

		err := mgr.Reply(context.Background(), msg, chatops.Reply{SilentDrop: true, Text: "ignored"})

		require.NoError(t, err)
		assert.Empty(t, ch.recorded)
	})

	t.Run("missing channel returns error", func(t *testing.T) {
		mgr := newLiveNotifyManager(nil)

		err := mgr.Reply(context.Background(), msg, chatops.Reply{Text: "hello"})

		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "通道未运行"), "error=%v", err)
	})

	t.Run("text reply sends to inbound chat", func(t *testing.T) {
		ch := &stubNotifyChannel{}
		mgr := newLiveNotifyManager(map[uint]notify.Channel{msg.SourceConfID: ch})

		err := mgr.Reply(context.Background(), msg, chatops.Reply{Text: "帮助信息"})

		require.NoError(t, err)
		require.Len(t, ch.recorded, 1)
		assert.Equal(t, notify.Notification{
			Text:         "帮助信息",
			ChannelType:  msg.ChannelType,
			SourceConfID: msg.SourceConfID,
			UserID:       msg.ChannelUserID,
			Targets:      map[string]string{"chat_id": msg.ChatID},
		}, ch.recorded[0])
	})
}

func newReloadDBNoTable(t *testing.T) *gorm.DB {
	t.Helper()
	global.InitLogger(zap.NewNop())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	return db
}

func waitBriefly() { time.Sleep(60 * time.Millisecond) }

// TestLiveNotifyManager_Reply_WithMessageType covers the branch of Reply that
// copies msg.MessageType into the notification targets map, and the successful
// delivery + info-log tail.
func TestLiveNotifyManager_Reply_WithMessageType(t *testing.T) {
	global.InitLogger(zap.NewNop())
	ch := &stubNotifyChannel{}
	m := newLiveNotifyManager(map[uint]notify.Channel{6: ch})

	msg := notify.InboundMessage{
		ChannelType:   "qq",
		SourceConfID:  6,
		ChatID:        "group-1",
		ChannelUserID: "u-6",
		MessageType:   "group",
	}
	require.NoError(t, m.Reply(context.Background(), msg, chatops.Reply{Text: "pong"}))
	require.Len(t, ch.recorded, 1)
	assert.Equal(t, "group", ch.recorded[0].Targets["message_type"])
	assert.Equal(t, "group-1", ch.recorded[0].Targets["chat_id"])
}

// TestLiveNotifyManager_Reply_SilentDropAndEmpty covers the two early-return
// guards in Reply: SilentDrop and empty text+buttons.
func TestLiveNotifyManager_Reply_SilentDropAndEmpty(t *testing.T) {
	global.InitLogger(zap.NewNop())
	ch := &stubNotifyChannel{}
	m := newLiveNotifyManager(map[uint]notify.Channel{7: ch})
	msg := notify.InboundMessage{ChannelType: "tg", SourceConfID: 7, ChatID: "c", ChannelUserID: "u"}

	require.NoError(t, m.Reply(context.Background(), msg, chatops.Reply{SilentDrop: true, Text: "x"}))
	require.NoError(t, m.Reply(context.Background(), msg, chatops.Reply{}))
	assert.Empty(t, ch.recorded, "silent-drop and empty replies must not send")
}

// TestLiveNotifyManager_Reply_ChannelNotRunning covers the "channel not
// running" branch (SourceConfID has no live channel).
func TestLiveNotifyManager_Reply_ChannelNotRunning(t *testing.T) {
	global.InitLogger(zap.NewNop())
	m := newLiveNotifyManager(nil)
	msg := notify.InboundMessage{ChannelType: "tg", SourceConfID: 99, ChatID: "c", ChannelUserID: "u"}
	err := m.Reply(context.Background(), msg, chatops.Reply{Text: "hello"})
	require.Error(t, err)
}

// TestReloadChatOpsChannels_CloseErrorContinues covers the error branch of the
// old-channel Close loop: a channel that fails to Close is logged and the
// reload still proceeds to rebuild from the (empty) DB.
func TestReloadChatOpsChannels_CloseErrorContinues(t *testing.T) {
	global.InitLogger(zap.NewNop())
	db := newReloadDB(t)

	failing := &closeRecordingChannel{closeErr: errors.New("close boom")}
	reg := notify.NewRegistry()
	bs := &chatopsBootstrap{
		registry: reg,
		manager:  newLiveNotifyManager(map[uint]notify.Channel{5: failing}),
		channels: map[uint]notify.Channel{5: failing},
	}

	require.NoError(t, reloadChatOpsChannels(context.Background(), db, bs, nil))
	assert.Equal(t, 1, failing.closeCalls, "failing channel Close must still be attempted")
	assert.Empty(t, bs.channels, "no enabled DB rows -> rebuilt map is empty")
}

// TestRunChatOpsChannelReloader_ReloadError publishes a matching event while the
// reloader is running against a DB whose notification_conf table is missing, so
// reloadChatOpsChannels returns an error and the warn-log branch runs.
func TestRunChatOpsChannelReloader_ReloadError(t *testing.T) {
	global.InitLogger(zap.NewNop())
	db := newReloadDBNoTable(t)
	reg := notify.NewRegistry()
	bs := &chatopsBootstrap{
		registry: reg,
		manager:  newLiveNotifyManager(nil),
		channels: map[uint]notify.Channel{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	go func() {
		close(started)
		runChatOpsChannelReloader(ctx, db, bs, nil)
	}()
	<-started
	waitBriefly()
	require.NoError(t, publishNotificationConfigChanged())
	waitBriefly()
	cancel()
}
