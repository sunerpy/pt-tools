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

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/notify"
	telegramadapter "github.com/sunerpy/pt-tools/internal/notify/adapter/telegram"
	"github.com/sunerpy/pt-tools/models"
)

func newBindingDB(t *testing.T) *gorm.DB {
	t.Helper()
	global.InitLogger(zap.NewNop())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.BotToken{}, &models.ChannelBinding{}))
	return db
}

func TestBindingConsumerAdapter_ConsumeCode(t *testing.T) {
	db := newBindingDB(t)
	svc := app.NewBindingService(db, "system")
	ctx := context.Background()

	// Issue a bind code, then consume it through the adapter (error-only shape).
	dto, err := svc.IssueCode(ctx, 1, "label", time.Hour)
	require.NoError(t, err)

	adapter := &bindingConsumerAdapter{svc: svc}
	require.NoError(t, adapter.ConsumeCode(ctx, dto.Code, "telegram", "user-1"))

	// Second consume of the same code fails (already used).
	err = adapter.ConsumeCode(ctx, dto.Code, "telegram", "user-2")
	require.Error(t, err)
}

type stubCallbackActions struct{}

func (stubCallbackActions) OnRSSDownload(context.Context, uint, int64) error { return nil }
func (stubCallbackActions) OnRSSIgnore(context.Context, uint, int64) error   { return nil }

func TestReloadChatOpsChannels_WiresCallbackHandler(t *testing.T) {
	db := newReloadDB(t)
	require.NoError(t, db.Create(&models.NotificationConf{
		ChannelType: "reloadcb", Name: "cb", Enabled: true,
	}).Error)

	setHandler := false
	reg := notify.NewRegistry()
	reg.Register("reloadcb", func() notify.Channel {
		return &callbackAwareChannel{onSet: func() { setHandler = true }}
	})

	bs := &chatopsBootstrap{
		registry: reg,
		manager:  newLiveNotifyManager(nil),
		channels: map[uint]notify.Channel{},
	}
	require.NoError(t, reloadChatOpsChannels(context.Background(), db, bs, stubCallbackActions{}))
	assert.True(t, setHandler, "callback handler should be wired onto the new channel")
	assert.Len(t, bs.channels, 1)
}

// callbackAwareChannel implements notify.Channel plus the telegram
// SetCallbackActionHandler seam that reloadChatOpsChannels type-asserts on.
type callbackAwareChannel struct {
	stubNotifyChannel
	onSet func()
}

func (c *callbackAwareChannel) Close(context.Context) error { return nil }
func (c *callbackAwareChannel) SetCallbackActionHandler(telegramadapter.CallbackActionHandler) {
	if c.onSet != nil {
		c.onSet()
	}
}

var _ = app.NewRSSCallbackActions
