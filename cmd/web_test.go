package cmd

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

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
