package cmd

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
