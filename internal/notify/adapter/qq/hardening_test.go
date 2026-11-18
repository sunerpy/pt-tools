package qq

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

// 监听在非本机地址时必须设置 Access Token：反向 WS 的 user_id 由连接方自报，没有 token 谁都能冒充管理员。
func TestQQ_Init_RequiresTokenOffLoopback(t *testing.T) {
	cases := []struct {
		addr, token string
		wantErr     bool
	}{
		{"0.0.0.0:0", "", true},
		{":0", "", true},
		{"[::]:0", "", true},
		{"0.0.0.0:0", "sekret", false},
		{"127.0.0.1:0", "", false},
		{"localhost:0", "", false},
		{"[::1]:0", "", false},
	}
	for _, tc := range cases {
		q := New()
		err := q.Init(context.Background(), &models.NotificationConf{
			ID:         21,
			ConfigJSON: `{"listen_addr":"` + tc.addr + `","access_token":"` + tc.token + `","admin_qq_users":[1]}`,
		})
		if tc.wantErr {
			require.Error(t, err, "addr=%s", tc.addr)
			assert.Contains(t, err.Error(), "Access Token")
			assert.Nil(t, q.listener, "拒绝时不能开始监听")
		} else {
			require.NoError(t, err, "addr=%s", tc.addr)
		}
		_ = q.Close(context.Background())
	}
}

// 新连接握手后旧连接才断开时，旧读循环不能把新连接的 caller 清空。
func TestQQ_Reconnect_OldConnDoesNotClearNewCaller(t *testing.T) {
	q := New()
	require.NoError(t, q.Init(context.Background(), &models.NotificationConf{
		ID: 22, ConfigJSON: `{"listen_addr":"127.0.0.1:0","admin_qq_users":[1]}`,
	}))
	defer func() { _ = q.Close(context.Background()) }()

	first := dialAdapter(t, q, 7, nil)
	defer func() { _ = first.Close() }()
	waitCaller(t, q)
	firstCaller := q.caller.Load()

	second := dialAdapter(t, q, 7, nil)
	defer func() { _ = second.Close() }()
	require.Eventually(t, func() bool {
		c := q.caller.Load()
		return c != nil && c != firstCaller
	}, 2*time.Second, 10*time.Millisecond)

	// 服务端替换时主动关掉了旧连接：旧客户端读到错误
	require.NoError(t, first.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, _, err := first.ReadMessage()
	require.Error(t, err)

	// 等旧读循环跑完它的退出逻辑，新连接的 caller 仍在
	time.Sleep(50 * time.Millisecond)
	assert.NotNil(t, q.activeCaller())
	assert.True(t, q.Healthy())
}

// Close 要断开已 upgrade 的 WebSocket，否则热重载后旧连接还会继续收发。
func TestQQ_Close_ClosesActiveWebSocket(t *testing.T) {
	q := New()
	require.NoError(t, q.Init(context.Background(), &models.NotificationConf{
		ID: 23, ConfigJSON: `{"listen_addr":"127.0.0.1:0","admin_qq_users":[1]}`,
	}))
	conn := dialAdapter(t, q, 7, nil)
	defer func() { _ = conn.Close() }()
	waitCaller(t, q)

	require.NoError(t, q.Close(context.Background()))
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, _, err := conn.ReadMessage()
	require.Error(t, err)
	assert.Nil(t, q.activeCaller())
}

// 同时处理的入站消息有上限，超出的消息丢弃，不再为每条消息无界开协程。
func TestQQ_HandleRawEvent_BoundsConcurrentHandlers(t *testing.T) {
	q := New()
	require.NoError(t, q.Init(context.Background(), &models.NotificationConf{
		ID: 24, ConfigJSON: `{"admin_qq_users":[10001]}`,
	}))
	defer func() { _ = q.Close(context.Background()) }()

	release := make(chan struct{})
	var started atomic.Int32
	q.OnInbound(func(context.Context, notify.InboundMessage) error {
		started.Add(1)
		<-release
		return nil
	})
	for range qqMaxConcurrentHandlers + 5 {
		require.NoError(t, q.HandleRawEvent(buildEvent(10001, "/status")))
	}
	require.Eventually(t, func() bool { return started.Load() == qqMaxConcurrentHandlers }, 2*time.Second, 10*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(qqMaxConcurrentHandlers), started.Load(), "超出上限的消息被丢弃")

	close(release)
	// 名额释放后恢复处理
	require.Eventually(t, func() bool { return len(q.handlerSlots) == 0 }, 2*time.Second, 10*time.Millisecond)
	require.NoError(t, q.HandleRawEvent(buildEvent(10001, "/status")))
	require.Eventually(t, func() bool { return started.Load() == qqMaxConcurrentHandlers+1 }, 2*time.Second, 10*time.Millisecond)
}
