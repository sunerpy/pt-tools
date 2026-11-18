package feishuapp

import (
	"context"
	"testing"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

func TestChannel_Basics(t *testing.T) {
	c := New()
	assert.Equal(t, ChannelType, c.Type())
	assert.Equal(t, "feishu_app", c.Type())
	assert.True(t, c.SupportsInbound())
	assert.False(t, c.Healthy(), "没有 Init 时不健康")
	assert.Equal(t, notify.LinkWaiting, c.LinkState())
}

func TestChannel_RegisteredInDefaultRegistry(t *testing.T) {
	ch, err := notify.DefaultRegistry().Make(ChannelType)
	require.NoError(t, err)
	_, ok := ch.(*Channel)
	assert.True(t, ok)
	_, ok = ch.(notify.ConfigChecker)
	assert.True(t, ok, "保存前要能检查配置")
	_, ok = ch.(notify.LinkStater)
	assert.True(t, ok, "要能报「已连接」")
}

func TestInit_NilConf(t *testing.T) {
	require.Error(t, New().Init(context.Background(), nil))
}

func TestInit_BadConfig(t *testing.T) {
	resetProcessState(t)
	c := New()
	err := c.Init(context.Background(), &models.NotificationConf{ID: 1, ConfigJSON: `{"app_id":"cli_x"}`})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "App Secret")
	assert.False(t, c.Healthy())
}

// Init 只解析配置、建好发消息用的 client：不连长连接、不发请求。保存前的 checkConf 与 outbox 的回退
// 都只 Init 不 OnInbound，它们要是也连上长连接，就成了同一个应用的第二个客户端，会把事件随机抢走。
func TestInit_NoIO(t *testing.T) {
	resetProcessState(t)
	api := &fakeAPI{}
	runners := &fakeRunners{}
	apiBuilt := 0
	c := New()
	c.newAPI = func(*Config, larkcore.Logger) apiSender { apiBuilt++; return api }
	c.newRunner = runners.factory
	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ID: 3, ConfigJSON: testConfig}))
	defer func() { _ = c.Close(context.Background()) }()

	assert.Equal(t, 1, apiBuilt)
	assert.Equal(t, 0, runners.count(), "Init 不建长连接")
	assert.Equal(t, 0, api.botCallCount())
	assert.Empty(t, api.sent())
	assert.True(t, c.Healthy())
	assert.Equal(t, notify.LinkWaiting, c.LinkState())
}

func TestInit_DefaultFactoriesBuildRealClients(t *testing.T) {
	resetProcessState(t)
	c := New()
	require.NoError(t, c.Init(context.Background(), &models.NotificationConf{ID: 4, ConfigJSON: testConfig}))
	defer func() { _ = c.Close(context.Background()) }()
	_, ok := c.api.(*larkAPI)
	assert.True(t, ok)
}

func TestClose_Idempotent(t *testing.T) {
	c, _, _ := newTestChannel(t, testConfig)
	require.NoError(t, c.Close(context.Background()))
	require.NoError(t, c.Close(context.Background()))
	assert.False(t, c.Healthy())
}

func TestClose_WithoutInit(t *testing.T) {
	require.NoError(t, New().Close(context.Background()))
}

func TestLinkState_FollowsConnection(t *testing.T) {
	c, _, _ := newTestChannel(t, testConfig)
	assert.Equal(t, notify.LinkWaiting, c.LinkState())
	c.setWS(wsConnected)
	assert.Equal(t, notify.LinkConnected, c.LinkState())
	assert.True(t, c.Healthy())
	require.NoError(t, c.Close(context.Background()))
	assert.Equal(t, notify.LinkWaiting, c.LinkState(), "关闭以后不再报已连接")
	c.setWS(wsConnected)
	assert.Equal(t, notify.LinkWaiting, c.LinkState(), "关闭以后回调不再改状态")
}

func TestLogger_FallsBackToGlobal(t *testing.T) {
	c := New()
	assert.NotNil(t, c.logger())
	assert.NotNil(t, c.sdkLogger())
}
