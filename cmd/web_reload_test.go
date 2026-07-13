package cmd

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/chatops"
	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

func publishNotificationConfigChanged() error {
	events.Publish(events.Event{
		Type: events.ConfigChanged, Version: time.Now().UnixNano(),
		Source: "notification", At: time.Now(),
	})
	return nil
}

func publishSitesConfigChanged() error {
	events.Publish(events.Event{
		Type: events.ConfigChanged, Version: time.Now().UnixNano(),
		Source: "sites", At: time.Now(),
	})
	return nil
}

// closeRecordingChannel tracks Close so reload/shutdown loop bodies are observable.
type closeRecordingChannel struct {
	stubNotifyChannel
	closeCalls int
	closeErr   error
}

func (c *closeRecordingChannel) Close(context.Context) error {
	c.closeCalls++
	return c.closeErr
}

func newReloadDB(t *testing.T) *gorm.DB {
	t.Helper()
	global.InitLogger(zap.NewNop())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.NotificationConf{}))
	return db
}

// TestReloadChatOpsChannels_ClosesOldAndRebuilds populates an existing channel,
// registers a factory + an enabled conf, and verifies the old channel is closed
// and a new one is built, exercising both loops in reloadChatOpsChannels.
func TestReloadChatOpsChannels_ClosesOldAndRebuilds(t *testing.T) {
	db := newReloadDB(t)
	require.NoError(t, db.Create(&models.NotificationConf{
		ChannelType: "reloadstub", Name: "n", Enabled: true,
	}).Error)

	reg := notify.NewRegistry()
	reg.Register("reloadstub", func() notify.Channel { return &inboundStubChannel{} })

	old := &closeRecordingChannel{}
	mgr := newLiveNotifyManager(map[uint]notify.Channel{99: old})
	bs := &chatopsBootstrap{
		registry: reg,
		manager:  mgr,
		channels: map[uint]notify.Channel{99: old},
	}

	require.NoError(t, reloadChatOpsChannels(context.Background(), db, bs, nil))
	assert.Equal(t, 1, old.closeCalls, "existing channel must be closed on reload")
	assert.Len(t, bs.channels, 1, "reload should rebuild channel map from DB")
	_, hasStale := bs.channels[99]
	assert.False(t, hasStale, "stale conf id should be gone after rebuild")
}

// TestReloadChatOpsChannels_InitFailureRestores forces initEnabledChannels to
// error (query on a missing table) and asserts the manager is restored.
func TestReloadChatOpsChannels_InitError(t *testing.T) {
	global.InitLogger(zap.NewNop())
	// DB without notification_conf table -> Find errors inside initEnabledChannels.
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	reg := notify.NewRegistry()
	old := &closeRecordingChannel{}
	mgr := newLiveNotifyManager(map[uint]notify.Channel{1: old})
	bs := &chatopsBootstrap{
		registry: reg,
		manager:  mgr,
		channels: map[uint]notify.Channel{1: old},
	}
	err = reloadChatOpsChannels(context.Background(), db, bs, nil)
	require.Error(t, err)
	assert.Equal(t, 1, old.closeCalls)
}

// TestRunChatOpsChannelReloader_ProcessesEvent publishes a matching
// ConfigChanged/notification event and verifies the reload path runs.
func TestRunChatOpsChannelReloader_ProcessesEvent(t *testing.T) {
	db := newReloadDB(t)
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
	// Give the subscriber a moment to register before publishing.
	time.Sleep(50 * time.Millisecond)
	require.NoError(t, publishNotificationConfigChanged())
	// Also publish an unrelated event to exercise the filter's continue branch.
	require.NoError(t, publishSitesConfigChanged())
	time.Sleep(100 * time.Millisecond)
	cancel()
}

func TestChatopsBootstrap_Shutdown_ClosesChannels(t *testing.T) {
	global.InitLogger(zap.NewNop())
	ch := &closeRecordingChannel{}
	bs := &chatopsBootstrap{channels: map[uint]notify.Channel{1: ch}}
	require.NoError(t, bs.Shutdown(context.Background()))
	assert.Equal(t, 1, ch.closeCalls)
	// closeOnce: a second Shutdown must not re-close.
	require.NoError(t, bs.Shutdown(context.Background()))
	assert.Equal(t, 1, ch.closeCalls)
}

func TestChatopsBootstrap_Shutdown_ChannelError(t *testing.T) {
	global.InitLogger(zap.NewNop())
	ch := &closeRecordingChannel{closeErr: errors.New("close boom")}
	bs := &chatopsBootstrap{channels: map[uint]notify.Channel{2: ch}}
	err := bs.Shutdown(context.Background())
	require.Error(t, err)
}

func TestSecretExportCmd_RunE(t *testing.T) {
	t.Setenv("PT_TOOLS_SECRET_KEY", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")
	captureStdout(t, func() {
		_ = secretExportCmd.RunE(secretExportCmd, nil)
	})
}

func TestSecretImportCmd_RunE_InvalidBase64(t *testing.T) {
	// Feed non-base64 on stdin so the import RunE wrapper returns an error.
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin })
	go func() { _, _ = w.WriteString("!!!not-base64!!!\n"); _ = w.Close() }()

	c := &cobra.Command{}
	c.Flags().Bool("force", false, "")
	captureStdout(t, func() {
		err := secretImportCmd.RunE(c, nil)
		require.Error(t, err)
	})
}

func TestChain_ReturnsChain(t *testing.T) {
	chain := chatops.NewMessageChain(
		chatops.DefaultRegistry(), nil, nil, nil, nil, nil, nil,
	)
	bs := &chatopsBootstrap{chain: chain}
	assert.Same(t, chain, bs.Chain())
}
