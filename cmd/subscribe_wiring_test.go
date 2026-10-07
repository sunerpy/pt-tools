package cmd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

// 订阅服务：没有识别服务时不建；建好后能读写设置，停下时断开 RSS 的入口
func TestNewSubscribeService(t *testing.T) {
	db := organizeTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&models.MediaQualityProfile{}, &models.MediaSubscription{}, &models.MediaSubscriptionTorrent{},
		&models.MediaSubscribeSetting{}, &models.MediaDoubanSource{}, &models.MediaDoubanItem{},
	))
	store := core.NewConfigStore(global.GlobalDB)
	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)
	ctx := context.Background()
	assert.Nil(t, newSubscribeService(ctx, mgr, nil, nil, nil, nil))

	svc := newSubscribeService(ctx, mgr, newMediaService(store), nil, nil, nil)
	require.NotNil(t, svc)
	set, err := svc.SaveSettings(ctx, subscribe.Settings{Enabled: true, SearchIntervalHours: 6})
	require.NoError(t, err)
	assert.True(t, set.Enabled)
	stopSubscribeService(svc)
	stopSubscribeService(nil)
	assert.Empty(t, qaDoubanURL(), "正式构建里豆瓣地址不能改")
}

func TestSubscribeNotifierDisabled(t *testing.T) {
	assert.Nil(t, subscribeNotifier(nil), "没有通知投递器时不发")
}
