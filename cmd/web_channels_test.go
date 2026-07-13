package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/chatops"
	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

// inboundStubChannel extends stubNotifyChannel to record inbound-handler wiring
// so initEnabledChannels' OnInbound branch is observable.
type inboundStubChannel struct {
	stubNotifyChannel
	initErr    error
	inboundSet bool
	initCalls  int
	lastConfig string
}

func (s *inboundStubChannel) Init(_ context.Context, conf *models.NotificationConf) error {
	s.initCalls++
	if conf != nil {
		s.lastConfig = conf.ConfigJSON
	}
	return s.initErr
}

func (s *inboundStubChannel) OnInbound(_ notify.InboundHandler) { s.inboundSet = true }

func newInitTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	global.InitLogger(zap.NewNop())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.NotificationConf{}))
	return db
}

func TestInitEnabledChannels_Success_DecryptAndInbound(t *testing.T) {
	t.Setenv("PT_TOOLS_SECRET_KEY", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")
	db := newInitTestDB(t)

	plain, err := json.Marshal(map[string]any{"k": "v"})
	require.NoError(t, err)
	cipher, err := crypto.Encrypt(plain)
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.NotificationConf{
		ChannelType: "stubok", Name: "ok", ConfigJSON: cipher, Enabled: true,
	}).Error)

	stub := &inboundStubChannel{}
	reg := notify.NewRegistry()
	reg.Register("stubok", func() notify.Channel { return stub })

	inboundCalled := false
	inbound := func(context.Context, notify.InboundMessage) error { inboundCalled = true; return nil }

	out, err := initEnabledChannels(context.Background(), db, reg, inbound, nopLogger{})
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.True(t, stub.inboundSet, "SupportsInbound channel should get OnInbound wired")
	assert.Equal(t, string(plain), stub.lastConfig, "config should be decrypted before Init")
	_ = inboundCalled
}

func TestInitEnabledChannels_UnknownFactory_Skipped(t *testing.T) {
	db := newInitTestDB(t)
	require.NoError(t, db.Create(&models.NotificationConf{
		ChannelType: "nope", Name: "n", Enabled: true,
	}).Error)
	reg := notify.NewRegistry()
	out, err := initEnabledChannels(context.Background(), db, reg, nil, nopLogger{})
	require.NoError(t, err)
	assert.Empty(t, out, "unknown channel type should be skipped, not fatal")
}

func TestInitEnabledChannels_InitError_Skipped(t *testing.T) {
	db := newInitTestDB(t)
	require.NoError(t, db.Create(&models.NotificationConf{
		ChannelType: "stubbad", Name: "bad", Enabled: true,
	}).Error)
	reg := notify.NewRegistry()
	reg.Register("stubbad", func() notify.Channel {
		return &inboundStubChannel{initErr: errors.New("boom")}
	})
	out, err := initEnabledChannels(context.Background(), db, reg, nil, nopLogger{})
	require.NoError(t, err)
	assert.Empty(t, out, "channel whose Init fails must be skipped")
}

func TestInitEnabledChannels_DecryptError_Skipped(t *testing.T) {
	t.Setenv("PT_TOOLS_SECRET_KEY", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")
	db := newInitTestDB(t)
	require.NoError(t, db.Create(&models.NotificationConf{
		ChannelType: "stubok", Name: "bad-cipher", ConfigJSON: "not-base64-cipher", Enabled: true,
	}).Error)
	reg := notify.NewRegistry()
	reg.Register("stubok", func() notify.Channel { return &inboundStubChannel{} })
	out, err := initEnabledChannels(context.Background(), db, reg, nil, nopLogger{})
	require.NoError(t, err)
	assert.Empty(t, out, "undecryptable config should skip the channel")
}

// erroringChannel.Send always fails, exercising the error branches of
// liveNotifyManager.Send and Reply.
type erroringChannel struct{ stubNotifyChannel }

func (*erroringChannel) Send(context.Context, notify.Notification) error {
	return errors.New("send failed")
}

func TestLiveNotifyManager_Send_ChannelError(t *testing.T) {
	global.InitLogger(zap.NewNop())
	m := newLiveNotifyManager(map[uint]notify.Channel{3: &erroringChannel{}})
	err := m.Send(context.Background(), 3, app.Notification{Text: "x", SourceConfID: 3})
	require.Error(t, err)
}

func TestLiveNotifyManager_Reply_ChannelError(t *testing.T) {
	global.InitLogger(zap.NewNop())
	m := newLiveNotifyManager(map[uint]notify.Channel{4: &erroringChannel{}})
	msg := notify.InboundMessage{ChannelType: "stub", SourceConfID: 4, ChatID: "c", ChannelUserID: "u"}
	err := m.Reply(context.Background(), msg, chatops.Reply{Text: "hello"})
	require.Error(t, err)
}

func TestLiveNotifyManager_Reply_NilManager(t *testing.T) {
	var m *liveNotifyManager
	err := m.Reply(context.Background(), notify.InboundMessage{}, chatops.Reply{Text: "x"})
	require.Error(t, err)
}
