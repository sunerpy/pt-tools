package remote

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRelay 是测试用的最小 relay：主机认证、按流转发、主机离线时 4404。真正的 relay 在路线图 M16。
type fakeRelay struct {
	srv *httptest.Server
	// origin 是它校验签名用的 origin（默认是自己的地址）
	origin string

	mu      sync.Mutex
	host    *websocket.Conn
	hostID  string
	next    uint32
	clients map[uint32]*websocket.Conn
	authN   atomic.Int32
	pings   atomic.Int32
	closes  chan uint16
}

func newFakeRelay(t *testing.T) *fakeRelay {
	t.Helper()
	r := &fakeRelay{clients: map[uint32]*websocket.Conn{}, closes: make(chan uint16, 64)}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/host/{id}", r.serveHost)
	mux.HandleFunc("/v1/client/{id}", r.serveClient)
	r.srv = httptest.NewServer(mux)
	r.origin = "ws://" + r.srv.Listener.Addr().String()
	t.Cleanup(r.srv.Close)
	return r
}

func (r *fakeRelay) url() string { return "ws://" + r.srv.Listener.Addr().String() }

func (r *fakeRelay) writeOuter(ctx context.Context, ws *websocket.Conn, f OuterFrame) error {
	b, err := AppendOuter(nil, f)
	if err != nil {
		return err
	}
	return ws.Write(ctx, websocket.MessageBinary, b)
}

func (r *fakeRelay) serveHost(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	ws, err := websocket.Accept(w, req, nil)
	if err != nil {
		return
	}
	ws.SetReadLimit(MaxOuterFrame)
	ctx := context.Background()
	nonce := make([]byte, NonceLen)
	_, _ = rand.Read(nonce)
	if r.writeOuter(ctx, ws, OuterFrame{Type: OuterChallenge, Payload: nonce}) != nil {
		return
	}
	_, b, err := ws.Read(ctx)
	if err != nil {
		return
	}
	r.authN.Add(1)
	f, err := ParseOuter(b)
	if err != nil || f.Type != OuterAuth || VerifyRelayAuth(id, nonce, r.origin, f.Payload) != nil {
		_ = ws.Close(CloseAuthFailed, "auth failed")
		return
	}
	if r.writeOuter(ctx, ws, OuterFrame{Type: OuterReady}) != nil {
		return
	}
	r.mu.Lock()
	if r.host != nil {
		_ = r.host.Close(CloseReplaced, "replaced")
	}
	r.host, r.hostID = ws, id
	r.mu.Unlock()
	for {
		typ, b, err := ws.Read(ctx)
		if err != nil {
			// 主机断开：这台主机的客户端连接全部关掉（4404），和真正的 relay 一样
			r.mu.Lock()
			if r.host == ws {
				r.host = nil
				for id, c := range r.clients {
					_ = c.Close(CloseHostOffline, "host offline")
					delete(r.clients, id)
				}
			}
			r.mu.Unlock()
			return
		}
		if typ == websocket.MessageText {
			if string(b) == relayPing {
				r.pings.Add(1)
				_ = ws.Write(ctx, websocket.MessageText, []byte(relayPong))
			}
			continue
		}
		f, err := ParseOuter(b)
		if err != nil {
			_ = ws.Close(CloseProtocol, "bad frame")
			return
		}
		r.mu.Lock()
		c := r.clients[f.Stream]
		r.mu.Unlock()
		switch f.Type {
		case OuterData:
			if c != nil {
				_ = c.Write(ctx, websocket.MessageBinary, f.Payload)
			}
		case OuterClose:
			code, _ := ParseClosePayload(f.Payload)
			r.closes <- code
			if c != nil {
				_ = c.Close(websocket.StatusNormalClosure, "")
			}
		}
	}
}

func (r *fakeRelay) serveClient(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	ws, err := websocket.Accept(w, req, nil)
	if err != nil {
		return
	}
	ws.SetReadLimit(MaxNoiseMessage)
	ctx := context.Background()
	r.mu.Lock()
	host := r.host
	if host == nil || r.hostID != id {
		r.mu.Unlock()
		_ = ws.Close(CloseHostOffline, "host offline")
		return
	}
	r.next++
	sid := r.next
	r.clients[sid] = ws
	r.mu.Unlock()
	if r.writeOuter(ctx, host, OuterFrame{Type: OuterOpen, Stream: sid}) != nil {
		return
	}
	for {
		_, b, err := ws.Read(ctx)
		if err != nil {
			_ = r.writeOuter(ctx, host, OuterFrame{Type: OuterClose, Stream: sid})
			return
		}
		if r.writeOuter(ctx, host, OuterFrame{Type: OuterData, Stream: sid, Payload: b}) != nil {
			return
		}
	}
}

func relayStatusOf(t *testing.T, th *testHost) []RelayStatus {
	t.Helper()
	ov, err := th.Overview(context.Background())
	require.NoError(t, err)
	return ov.RelayStatus
}

func waitRelay(t *testing.T, th *testHost, state string) RelayStatus {
	t.Helper()
	var last RelayStatus
	require.Eventually(t, func() bool {
		st := relayStatusOf(t, th)
		if len(st) != 1 {
			return false
		}
		last = st[0]
		return last.State == state
	}, 10*time.Second, 20*time.Millisecond, "relay 状态没有变成 %s（%+v）", state, last)
	return last
}

// 经 relay：主机认证（签名签进 relay 的 origin）以后，设备连 relay 的客户端入口，流里的 Noise 会话和直连一样
func TestRelayEndToEnd(t *testing.T) {
	relay := newFakeRelay(t)
	th := newTestHost(t, Settings{Relays: []string{relay.url()}})
	d, priv := th.addDevice(t, "手机", ScopesFull)
	waitRelay(t, th, RelayOnline)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	dk, _ := KeypairFromPrivate(priv)
	raw, err := DialRelay(ctx, relay.url(), th.keys.HostID(), nil)
	require.NoError(t, err)
	c, err := Connect(ctx, raw, ClientConfig{HostID: th.keys.HostID(), HostKey: th.keys.Noise.Public, Device: dk})
	require.NoError(t, err)
	status, _, body := get(t, c, "/api/app/v1/meta", nil)
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, `"via":"relay"`)
	assert.Equal(t, d.ID, th.disp.last().Peer.Device.ID)
	status, _, body = get(t, c, "/api/app/v1/big", nil)
	assert.Equal(t, http.StatusOK, status)
	assert.Len(t, body, 3*MaxFramePayload+7)
	assert.Equal(t, 1, waitRelay(t, th, RelayOnline).Streams)

	// 撤销：主机关掉这个流，relay 收到 CLOSE
	_, err = th.RevokeDevice(context.Background(), d.ID)
	require.NoError(t, err)
	wantGoAway(t, c, GoAwayRevoked)
	select {
	case <-relay.closes:
	case <-time.After(5 * time.Second):
		t.Fatal("relay 没有收到 CLOSE")
	}
	require.Eventually(t, func() bool { return relayStatusOf(t, th)[0].Streams == 0 }, 5*time.Second, 20*time.Millisecond)

	// 主机不在线的 hostId：relay 用 4404 关掉
	other := seedKeys(t, 11)
	raw, err = DialRelay(ctx, relay.url(), other.HostID(), nil)
	require.NoError(t, err)
	_, err = raw.ReadMsg(ctx)
	assert.Equal(t, websocket.StatusCode(CloseHostOffline), websocket.CloseStatus(err))
}

// relay 校验签名用的 origin 和主机连的地址不一样（比如被转到了别的 relay）：认证不通过，主机显示断开并按退避重连
func TestRelayAuthFailure(t *testing.T) {
	relay := newFakeRelay(t)
	relay.origin = "wss://another-relay.example.com"
	th := newTestHost(t, Settings{Relays: []string{relay.url()}})
	st := waitRelay(t, th, RelayOffline)
	assert.Contains(t, st.Error, "relay 认证没有通过")
	require.Eventually(t, func() bool { return relay.authN.Load() >= 2 }, 5*time.Second, 50*time.Millisecond, "断开以后会重连")
}

// 一条 relay 连接上最多 16 个流，第 17 个直接用 4429 关掉
func TestRelayStreamLimit(t *testing.T) {
	relay := newFakeRelay(t)
	th := newTestHost(t, Settings{Relays: []string{relay.url()}})
	waitRelay(t, th, RelayOnline)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var conns []MsgConn
	for i := 0; i < MaxRelayStreams; i++ {
		raw, err := DialRelay(ctx, relay.url(), th.keys.HostID(), nil)
		require.NoError(t, err)
		conns = append(conns, raw)
	}
	require.Eventually(t, func() bool { return relayStatusOf(t, th)[0].Streams == MaxRelayStreams }, 5*time.Second, 20*time.Millisecond)
	raw, err := DialRelay(ctx, relay.url(), th.keys.HostID(), nil)
	require.NoError(t, err)
	select {
	case code := <-relay.closes:
		assert.Equal(t, uint16(CloseLimited), code)
	case <-time.After(5 * time.Second):
		t.Fatal("第 17 个流没有被关掉")
	}
	raw.Close(1000, "")
	for _, c := range conns {
		c.Close(1000, "")
	}
}

// 设置里去掉 relay：断开它；换主机密钥：用新的 hostId 重连
func TestRelaySettingsChanges(t *testing.T) {
	relay := newFakeRelay(t)
	th := newTestHost(t, Settings{Relays: []string{relay.url()}})
	waitRelay(t, th, RelayOnline)
	ctx := context.Background()
	ov, err := th.RotateKeys(ctx)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		relay.mu.Lock()
		defer relay.mu.Unlock()
		return relay.host != nil && relay.hostID == ov.HostID
	}, 10*time.Second, 20*time.Millisecond, "换了密钥以后用新的 hostId 重连")

	_, err = th.UpdateSettings(ctx, Settings{Enabled: true, DirectURL: testDirect})
	require.NoError(t, err)
	assert.Empty(t, relayStatusOf(t, th))
	require.Eventually(t, func() bool {
		relay.mu.Lock()
		defer relay.mu.Unlock()
		return relay.host == nil
	}, 5*time.Second, 20*time.Millisecond, "去掉以后断开")
}

// 流的读写：排队满了的流被关掉，不拖住同一条 relay 连接上的别的流
func TestRelayStreamQueue(t *testing.T) {
	m := &relayMux{streams: map[uint32]*relayStream{}}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	defer m.cancel()
	s := &relayStream{m: m, id: 1, in: make(chan []byte, 2), closed: make(chan struct{})}
	assert.True(t, s.push([]byte("a")))
	assert.True(t, s.push([]byte("b")))
	assert.False(t, s.push([]byte("c")))
	b, err := s.ReadMsg(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "a", string(b))
	s.remoteClosed()
	assert.True(t, s.push([]byte("d")), "关掉以后丢弃，不算排满")
	b, err = s.ReadMsg(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "b", string(b), "关掉之前排进来的照样读到")
	_, err = s.ReadMsg(context.Background())
	assert.ErrorIs(t, err, errRelayClosed)
	assert.ErrorIs(t, s.WriteMsg(context.Background(), []byte("x")), errRelayClosed)
}

func TestRelayClientsDialErrors(t *testing.T) {
	_, err := DialRelay(context.Background(), "https://relay", seedKeys(t, 1).HostID(), nil)
	assert.Error(t, err)
	_, err = DialRelay(context.Background(), "wss://relay", "bad", nil)
	assert.Error(t, err)
	_, err = DialDirect(context.Background(), "wss://x", nil)
	assert.True(t, err != nil && strings.Contains(err.Error(), "直连地址"))
}

// 经 relay 的会话在主机密钥轮换、撤销时同样先收到 GOAWAY 再断开
func TestRelaySessionGoAway(t *testing.T) {
	relay := newFakeRelay(t)
	th := newTestHost(t, Settings{Relays: []string{relay.url()}})
	d, priv := th.addDevice(t, "手机", ScopesFull)
	waitRelay(t, th, RelayOnline)
	dial := func() *Client {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		dk, _ := KeypairFromPrivate(priv)
		raw, err := DialRelay(ctx, relay.url(), th.keys.HostID(), nil)
		require.NoError(t, err)
		c, err := Connect(ctx, raw, ClientConfig{HostID: th.keys.HostID(), HostKey: th.keys.Noise.Public, Device: dk})
		require.NoError(t, err)
		return c
	}
	c := dial()
	_, err := th.UpdateDevice(context.Background(), d.ID, nil, ScopesRead)
	require.NoError(t, err)
	wantGoAway(t, c, GoAwayScopeChanged)

	c = dial()
	_, err = th.RotateKeys(context.Background())
	require.NoError(t, err)
	wantGoAway(t, c, GoAwayKeyRotated)
}

// 关掉远程访问与关闭主机：经 relay 的会话先收到 GOAWAY（disabled、shutdown），再断开 relay
func TestRelayGoAwayBeforeDisconnect(t *testing.T) {
	relay := newFakeRelay(t)
	th := newTestHost(t, Settings{Relays: []string{relay.url()}})
	_, priv := th.addDevice(t, "手机", ScopesFull)
	waitRelay(t, th, RelayOnline)
	dial := func() *Client {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		dk, _ := KeypairFromPrivate(priv)
		raw, err := DialRelay(ctx, relay.url(), th.keys.HostID(), nil)
		require.NoError(t, err)
		c, err := Connect(ctx, raw, ClientConfig{HostID: th.keys.HostID(), HostKey: th.keys.Noise.Public, Device: dk})
		require.NoError(t, err)
		return c
	}
	c := dial()
	_, err := th.UpdateSettings(context.Background(), Settings{Enabled: false, Relays: []string{relay.url()}})
	require.NoError(t, err)
	wantGoAway(t, c, GoAwayDisabled)

	_, err = th.UpdateSettings(context.Background(), Settings{Enabled: true, Relays: []string{relay.url()}})
	require.NoError(t, err)
	waitRelay(t, th, RelayOnline)
	c = dial()
	th.Close()
	wantGoAway(t, c, GoAwayShutdown)
}

// relay 不停地发超限的 OPEN：CLOSE 排队由一个写者发出，goroutine 不会跟着涨
func TestRelayOpenFloodBounded(t *testing.T) {
	relay := newFakeRelay(t)
	th := newTestHost(t, Settings{Relays: []string{relay.url()}})
	waitRelay(t, th, RelayOnline)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// 先占满 16 个流
	for i := 0; i < MaxRelayStreams; i++ {
		raw, err := DialRelay(ctx, relay.url(), th.keys.HostID(), nil)
		require.NoError(t, err)
		t.Cleanup(func() { raw.Close(1000, "") })
	}
	require.Eventually(t, func() bool { return relayStatusOf(t, th)[0].Streams == MaxRelayStreams }, 5*time.Second, 20*time.Millisecond)
	before := runtime.NumGoroutine()
	relay.mu.Lock()
	host := relay.host
	relay.mu.Unlock()
	require.NotNil(t, host)
	// CLOSE 堆满时主机断开这条 relay 连接（之后的写会失败），这也是想要的保护
	sent := 0
	for i := 0; i < 500; i++ {
		if relay.writeOuter(ctx, host, OuterFrame{Type: OuterOpen, Stream: uint32(10000 + i)}) != nil {
			break
		}
		sent++
	}
	time.Sleep(300 * time.Millisecond) // 让主机把 OPEN 读完（数 goroutine 用，不是等某个结果）
	assert.Less(t, runtime.NumGoroutine()-before, 40, "超限的 OPEN 不该各起一个 goroutine（发了 %d 个）", sent)
}
