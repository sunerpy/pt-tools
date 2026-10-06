package cmd

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

// 关闭之后的热重载不能把通道重新建起来。
func TestReloadChatOpsChannels_AfterShutdownDoesNotRecreate(t *testing.T) {
	db := newReloadDB(t)
	require.NoError(t, db.Create(&models.NotificationConf{
		ChannelType: "reloadstub", Name: "n", Enabled: true,
	}).Error)
	reg := notify.NewRegistry()
	made := 0
	reg.Register("reloadstub", func() notify.Channel {
		made++
		return &inboundStubChannel{}
	})
	bs := &chatopsBootstrap{
		registry: reg,
		manager:  newLiveNotifyManager(nil),
		channels: map[uint]notify.Channel{},
	}
	require.NoError(t, bs.Shutdown(context.Background()))

	err := reloadChatOpsChannels(context.Background(), db, bs, nil)

	require.Error(t, err)
	assert.Zero(t, made, "关闭之后不再新建通道实例")
	assert.Zero(t, bs.ChannelCount())
}

// 热重载与关闭并发时不能有数据竞争（-race 下原来会报 bs.channels 的读写竞争）。
func TestReloadChatOpsChannels_ConcurrentWithShutdown(t *testing.T) {
	db := newReloadDB(t)
	require.NoError(t, db.Create(&models.NotificationConf{
		ChannelType: "reloadstub", Name: "n", Enabled: true,
	}).Error)
	reg := notify.NewRegistry()
	reg.Register("reloadstub", func() notify.Channel { return &inboundStubChannel{} })
	bs := &chatopsBootstrap{
		registry: reg,
		manager:  newLiveNotifyManager(nil),
		channels: map[uint]notify.Channel{},
	}

	var wg sync.WaitGroup
	wg.Go(func() {
		for range 20 {
			_ = reloadChatOpsChannels(context.Background(), db, bs, nil)
		}
	})
	require.NoError(t, bs.Shutdown(context.Background()))
	wg.Wait()

	assert.Zero(t, bs.ChannelCount(), "关闭之后不留运行中的通道")
}
