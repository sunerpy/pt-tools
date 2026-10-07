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

// /sub、/subs 经订阅服务：订阅的名字带年份与季，进度写成一句话
func TestChatOpsSubscribeAdapter(t *testing.T) {
	assert.Equal(t, "沙丘2 (2024)", subscriptionName(&models.MediaSubscription{MediaType: models.MediaKindMovie, Title: "沙丘2", Year: 2024}))
	assert.Equal(t, "最后生还者 (2025) 第 2 季", subscriptionName(&models.MediaSubscription{MediaType: models.MediaKindTV, Title: "最后生还者", Year: 2025, Season: 2}))
	assert.Equal(t, "待确认", subscriptionStatus(models.MediaSubPending))
	tv := &subscribe.SubscriptionView{MediaSubscription: models.MediaSubscription{MediaType: models.MediaKindTV}, Progress: &subscribe.Progress{Total: 3, InLibrary: 1, Downloading: 1, Missing: []int{2}}}
	assert.Equal(t, "已入库 1/3，下载中 1，缺 1", progressText(tv))
	movie := &subscribe.SubscriptionView{MediaSubscription: models.MediaSubscription{MediaType: models.MediaKindMovie}, Progress: &subscribe.Progress{Total: 1, Downloading: 1}}
	assert.Equal(t, "下载中", progressText(movie))
	assert.Empty(t, progressText(&subscribe.SubscriptionView{}))
	_, err := chatopsSubscribe{}.Subscribe(context.Background(), "book", 1, 0)
	require.Error(t, err)
}
