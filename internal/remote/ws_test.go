package remote

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func directServer(t *testing.T, th *testHost) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(StreamPath, th.Host)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// 直连：WebSocket 每条二进制消息是一条 Noise 消息；直连地址可以带子路径以外的写法由 DialDirect 换成 ws://
func TestDirectWebSocket(t *testing.T) {
	th := newTestHost(t, Settings{DirectURL: testDirect})
	d, priv := th.addDevice(t, "手机", ScopesFull)
	srv := directServer(t, th)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	dk, _ := KeypairFromPrivate(priv)
	raw, err := DialDirect(ctx, srv.URL+"/", srv.Client())
	require.NoError(t, err)
	c, err := Connect(ctx, raw, ClientConfig{HostID: th.keys.HostID(), HostKey: th.keys.Noise.Public, Device: dk})
	require.NoError(t, err)
	status, _, body := get(t, c, "/api/app/v1/meta", nil)
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, `"via":"direct"`)
	assert.Equal(t, d.ID, th.disp.last().Peer.Device.ID)
	status, _, body = get(t, c, "/api/app/v1/big", nil)
	assert.Equal(t, http.StatusOK, status)
	assert.Len(t, body, 3*MaxFramePayload+7)
	c.Close()

	// 文本消息与超过 65535 字节的消息：主机断开
	for _, msg := range []struct {
		typ  websocket.MessageType
		data []byte
	}{{websocket.MessageText, []byte("hello")}, {websocket.MessageBinary, make([]byte, MaxNoiseMessage+1)}} {
		ws, _, derr := websocket.Dial(ctx, "ws"+srv.URL[len("http"):]+StreamPath, nil)
		require.NoError(t, derr)
		require.NoError(t, ws.Write(ctx, msg.typ, msg.data))
		_, _, err = ws.Read(ctx)
		assert.Error(t, err)
		_ = ws.CloseNow()
	}

	// 关掉远程访问以后直连入口是 404
	_, err = th.UpdateSettings(context.Background(), Settings{Enabled: false})
	require.NoError(t, err)
	_, err = DialDirect(ctx, srv.URL, srv.Client())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

// 每个 IP 每分钟最多 30 次直连握手，多的回 429；过了一分钟恢复
func TestDirectRateLimit(t *testing.T) {
	clk := &fakeClock{t: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)}
	th := newTestHost(t, Settings{DirectURL: testDirect}, func(c *Config) { c.Now = clk.now })
	srv := directServer(t, th)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for i := 0; i < HandshakesPerMinute; i++ {
		raw, err := DialDirect(ctx, srv.URL, srv.Client())
		require.NoError(t, err, i)
		raw.Close(1000, "")
	}
	_, err := DialDirect(ctx, srv.URL, srv.Client())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "429")
	clk.add(time.Minute)
	raw, err := DialDirect(ctx, srv.URL, srv.Client())
	require.NoError(t, err)
	raw.Close(1000, "")
}

func TestIPLimiterTableBound(t *testing.T) {
	clk := &fakeClock{t: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)}
	l := newIPLimiter(2, time.Minute, clk.now)
	for i := 0; i < limiterMaxIPs; i++ {
		require.True(t, l.allow(string(rune(i+0x4e00))))
	}
	assert.False(t, l.allow("new"), "表满了先拒绝新来的 IP")
	assert.True(t, l.allow(string(rune(0x4e00))), "已经在表里的照常计数")
	assert.False(t, l.allow(string(rune(0x4e00))))
	clk.add(time.Minute)
	assert.True(t, l.allow("new"), "过期的清掉以后又能记")
}
