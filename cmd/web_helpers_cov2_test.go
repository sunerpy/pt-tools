package cmd

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/chatops"
	"github.com/sunerpy/pt-tools/internal/notify"
)

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
