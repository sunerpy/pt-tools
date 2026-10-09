package relayserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/remote"
	"github.com/sunerpy/pt-tools/internal/remote/relaytest"
)

// start 起一个 relay（httptest），PublicURL 是它自己的 ws:// 地址。
func start(t *testing.T, cfg Config) (*Server, string) {
	t.Helper()
	var srv *Server
	ts := httptest.NewUnstartedServer(nil)
	u := "ws://" + ts.Listener.Addr().String()
	cfg.PublicURL = u
	if cfg.Version == "" {
		cfg.Version = "test"
	}
	var err error
	srv, err = New(cfg)
	require.NoError(t, err)
	ts.Config.Handler = srv.Handler()
	ts.Start()
	t.Cleanup(func() {
		srv.Close()
		ts.Close()
	})
	return srv, u
}

// 一致性测试：Go 版跑三种实例（Cloudflare 版在 relay.yml 里对 wrangler dev 跑同一套）
func TestConformanceMain(t *testing.T) {
	_, u := start(t, Config{MaxStreamsPerHost: 4, MaxConnPerIPPerMin: -1})
	relaytest.Run(t, relaytest.Target{URL: u, Profile: relaytest.ProfileMain, MaxStreamsPerHost: 4})
}

func TestConformanceLimits(t *testing.T) {
	_, u := start(t, Config{MaxStreamsPerHost: 16, MaxConnPerIPPerMin: 8, DailyBytesPerHost: 200_000})
	relaytest.Run(t, relaytest.Target{URL: u, Profile: relaytest.ProfileLimits, MaxConnPerIPPerMin: 8, DailyBytesPerHost: 200_000})
}

func TestConformanceDisabled(t *testing.T) {
	_, u := start(t, Config{Disabled: true})
	relaytest.Run(t, relaytest.Target{URL: u, Profile: relaytest.ProfileDisabled})
}

// 真正的主机端（internal/remote 的 Host 与 relay 客户端）经 Go 版 relay：上线、配对、设备会话、撤销
func TestRealHostInterop(t *testing.T) {
	_, u := start(t, Config{MaxConnPerIPPerMin: -1})
	relaytest.RunHostInterop(t, u)
}

// 主机端（internal/remote 的 relay 客户端）经这个 relay 与设备端互通：认证、开流、Noise 会话、断开
func TestHostClientInterop(t *testing.T) {
	_, u := start(t, Config{MaxConnPerIPPerMin: -1})
	keys, err := remote.GenerateHostKeys(nil)
	require.NoError(t, err)
	// 用 relaytest 的帧工具当主机：认证以后把客户端发来的 DATA 原样回给它
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	hostWS, _, err := websocket.Dial(ctx, u+"/v1/host/"+keys.HostID(), nil)
	require.NoError(t, err)
	defer hostWS.CloseNow()
	hostWS.SetReadLimit(remote.MaxOuterFrame)
	_, b, err := hostWS.Read(ctx)
	require.NoError(t, err)
	ch, err := remote.ParseOuter(b)
	require.NoError(t, err)
	auth, _ := remote.AppendOuter(nil, remote.OuterFrame{Type: remote.OuterAuth, Payload: keys.RelayAuth(ch.Payload, u)})
	require.NoError(t, hostWS.Write(ctx, websocket.MessageBinary, auth))
	_, b, err = hostWS.Read(ctx)
	require.NoError(t, err)
	ready, err := remote.ParseOuter(b)
	require.NoError(t, err)
	require.Equal(t, remote.OuterReady, ready.Type)
	go func() {
		for {
			_, msg, rerr := hostWS.Read(context.Background())
			if rerr != nil {
				return
			}
			f, perr := remote.ParseOuter(msg)
			if perr != nil || f.Type != remote.OuterData {
				continue
			}
			echo, _ := remote.AppendOuter(nil, remote.OuterFrame{Type: remote.OuterData, Stream: f.Stream, Payload: f.Payload})
			_ = hostWS.Write(context.Background(), websocket.MessageBinary, echo)
		}
	}()
	c, err := remote.DialRelay(ctx, u, keys.HostID(), nil)
	require.NoError(t, err)
	require.NoError(t, c.WriteMsg(ctx, []byte("noise message")))
	got, err := c.ReadMsg(ctx)
	require.NoError(t, err)
	assert.Equal(t, "noise message", string(got))
	c.Close(1000, "")
}

func TestNewValidation(t *testing.T) {
	_, err := New(Config{PublicURL: "https://relay.example.com"})
	assert.Error(t, err)
	_, err = New(Config{})
	assert.Error(t, err)
	s, err := New(Config{PublicURL: "wss://Relay.Example.com:443/"})
	require.NoError(t, err)
	assert.Equal(t, "wss://relay.example.com", s.origin)
	assert.Equal(t, DefaultMaxStreamsPerHost, s.cfg.MaxStreamsPerHost)
	assert.Equal(t, DefaultMaxConnPerIPPerMin, s.cfg.MaxConnPerIPPerMin)
	s.Close()
}

func TestClientIP(t *testing.T) {
	s, err := New(Config{PublicURL: "wss://r.example.com"})
	require.NoError(t, err)
	defer s.Close()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.2:5555"
	r.Header.Set("X-Real-IP", "1.2.3.4")
	assert.Equal(t, "10.0.0.2", s.clientIP(r), "没配置请求头时不认它")
	s.cfg.ClientIPHeader = "X-Forwarded-For"
	// 代理把它看到的地址追加在最后；前面的部分是客户端自己写的，不能认
	r.Header.Set("X-Forwarded-For", " 6.6.6.6 , 5.6.7.8 ")
	assert.Equal(t, "5.6.7.8", s.clientIP(r))
	r.Header.Add("X-Forwarded-For", "9.9.9.9")
	assert.Equal(t, "9.9.9.9", s.clientIP(r), "有多行时取最后一行")
	r.Header.Del("X-Forwarded-For")
	assert.Equal(t, "10.0.0.2", s.clientIP(r), "头为空时退回对端地址")
}

// 限流的键：IPv4 按地址，IPv6 按 /64，映射的 IPv4 当 IPv4
func TestLimitKey(t *testing.T) {
	for in, want := range map[string]string{
		"1.2.3.4":                   "1.2.3.4",
		"::ffff:1.2.3.4":            "1.2.3.4",
		"2001:db8:1:2:aaaa::1":      "2001:db8:1:2::/64",
		"2001:db8:1:2:bbbb:cccc::9": "2001:db8:1:2::/64",
		"fe80::1%eth0":              "fe80::/64",
		"not-an-ip":                 "not-an-ip",
	} {
		assert.Equal(t, want, limitKey(in), in)
	}
}

// 流编号用到头以后从 1 重来，跳过还开着的
func TestStreamIDWrap(t *testing.T) {
	s, err := New(Config{PublicURL: "ws://127.0.0.1:1"})
	require.NoError(t, err)
	defer s.Close()
	h := &hostConn{s: s, streams: map[uint32]*clientConn{}, done: make(chan struct{})}
	h.streams[1] = &clientConn{h: h, id: 1}
	h.next = ^uint32(0) - 1
	assert.Equal(t, ^uint32(0), h.allocStream())
	assert.Equal(t, uint32(2), h.allocStream(), "跳过 0 与还开着的 1")
}

func TestIPLimiter(t *testing.T) {
	now := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	l := newIPLimiter(2, time.Minute, func() time.Time { return now })
	assert.True(t, l.allow("a"))
	assert.True(t, l.allow("a"))
	assert.False(t, l.allow("a"))
	assert.True(t, l.allow("b"))
	now = now.Add(time.Minute)
	assert.True(t, l.allow("a"))
	unlimited := newIPLimiter(-1, time.Minute, time.Now)
	for i := 0; i < 100; i++ {
		assert.True(t, unlimited.allow("a"))
	}
}

func TestClientCloseCode(t *testing.T) {
	for in, want := range map[uint16]int{1000: 1000, 1001: 1001, 4404: 4404, 3000: 3000, 1006: 1000, 1011: 1000, 0: 1000, 5000: 1000} {
		assert.Equal(t, want, clientCloseCode(in), in)
	}
}

// 每天的用量到了 00:00 UTC 重置
func TestDailyQuotaResets(t *testing.T) {
	now := time.Date(2026, 10, 9, 23, 59, 0, 0, time.UTC)
	s, err := New(Config{PublicURL: "wss://r.example.com", DailyBytesPerHost: 10, Now: func() time.Time { return now }})
	require.NoError(t, err)
	defer s.Close()
	h := &hostConn{s: s, streams: map[uint32]*clientConn{}, done: make(chan struct{})}
	assert.True(t, h.count(10))
	assert.False(t, h.count(1))
	assert.True(t, h.overQuota())
	now = now.Add(2 * time.Minute)
	assert.False(t, h.overQuota())
	assert.True(t, h.count(5))
}

func TestHealthz(t *testing.T) {
	_, u := start(t, Config{Version: "v1.2.3"})
	resp, err := http.Get("http" + strings.TrimPrefix(u, "ws") + "/healthz")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
}

// 队列满时主机的 CLOSE 用预留的那一格：排在 64 条 DATA 后面，之后这个流上的消息都丢掉
func TestClientQueueCloseSlot(t *testing.T) {
	s, err := New(Config{PublicURL: "ws://127.0.0.1:1"})
	require.NoError(t, err)
	defer s.Close()
	h := &hostConn{s: s, streams: map[uint32]*clientConn{}, done: make(chan struct{})}
	c := newClientConn(h, nil, 1)
	for i := 0; i < clientQueue; i++ {
		require.True(t, c.pushData([]byte{byte(i)}), i)
	}
	assert.False(t, c.pushData([]byte("x")), "第 65 条 DATA 排不进")
	c.pushClose(4321, "bye")
	require.Len(t, c.out, clientQueue+1)
	c.pushClose(4000, "again")
	assert.True(t, c.pushData([]byte("late")), "关闭排上以后的 DATA 丢掉")
	assert.Len(t, c.out, clientQueue+1)
	for i := 0; i < clientQueue; i++ {
		m := <-c.out
		assert.Equal(t, []byte{byte(i)}, m.data)
	}
	last := <-c.out
	assert.True(t, last.close)
	assert.Equal(t, 4321, last.code)
}

// 每天的用量按 hostId 记在 relay 上：换一条连接不清零，过了 00:00 UTC 重置
func TestUsageByHostID(t *testing.T) {
	now := time.Date(2026, 10, 9, 23, 0, 0, 0, time.UTC)
	s, err := New(Config{PublicURL: "ws://127.0.0.1:1", DailyBytesPerHost: 10, Now: func() time.Time { return now }})
	require.NoError(t, err)
	defer s.Close()
	a := &hostConn{s: s, id: "h1", streams: map[uint32]*clientConn{}, done: make(chan struct{})}
	assert.True(t, a.count(8))
	b := &hostConn{s: s, id: "h1", streams: map[uint32]*clientConn{}, done: make(chan struct{})}
	assert.False(t, b.count(5), "同一个 hostId 的新连接接着算")
	assert.True(t, b.overQuota())
	other := &hostConn{s: s, id: "h2", streams: map[uint32]*clientConn{}, done: make(chan struct{})}
	assert.False(t, other.overQuota())
	now = now.Add(2 * time.Hour)
	assert.False(t, b.overQuota(), "过了 00:00 UTC 重置")
	assert.True(t, b.count(5))
}

// authHost 以 keys 连上 relay（u）并认证。
func authHost(t *testing.T, ctx context.Context, u string, keys *remote.HostKeys) *websocket.Conn {
	t.Helper()
	ws, _, err := websocket.Dial(ctx, u+"/v1/host/"+keys.HostID(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.CloseNow() })
	ws.SetReadLimit(remote.MaxOuterFrame)
	ch := readOuterFrame(t, ctx, ws)
	auth, _ := remote.AppendOuter(nil, remote.OuterFrame{Type: remote.OuterAuth, Payload: keys.RelayAuth(ch.Payload, u)})
	require.NoError(t, ws.Write(ctx, websocket.MessageBinary, auth))
	require.Equal(t, remote.OuterReady, readOuterFrame(t, ctx, ws).Type)
	return ws
}

func readOuterFrame(t *testing.T, ctx context.Context, ws *websocket.Conn) remote.OuterFrame {
	t.Helper()
	_, b, err := ws.Read(ctx)
	require.NoError(t, err)
	f, err := remote.ParseOuter(b)
	require.NoError(t, err)
	return f
}

// ACCEPT 以前的消息不计入每天的转发量：未认证的客户端发垃圾、被拒绝的握手都耗不掉主机的额度
func TestUnconfirmedNotCounted(t *testing.T) {
	_, u := start(t, Config{MaxConnPerIPPerMin: -1, DailyBytesPerHost: 10_000})
	keys, err := remote.GenerateHostKeys(nil)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	host := authHost(t, ctx, u, keys)
	writeHost := func(f remote.OuterFrame) {
		b, _ := remote.AppendOuter(nil, f)
		require.NoError(t, host.Write(ctx, websocket.MessageBinary, b))
	}
	for i := 0; i < 4; i++ {
		g, gerr := remote.DialRelay(ctx, u, keys.HostID(), nil)
		require.NoError(t, gerr)
		open := readOuterFrame(t, ctx, host)
		require.Equal(t, remote.OuterOpen, open.Type)
		require.NoError(t, g.WriteMsg(ctx, make([]byte, 4000)))
		require.Equal(t, remote.OuterData, readOuterFrame(t, ctx, host).Type)
		// 主机解不开，关掉这个流
		writeHost(remote.OuterFrame{Type: remote.OuterClose, Stream: open.Stream, Payload: remote.ClosePayload(1000, "")})
		g.Close(1000, "")
	}
	c, err := remote.DialRelay(ctx, u, keys.HostID(), nil)
	require.NoError(t, err)
	defer c.Close(1000, "")
	var open remote.OuterFrame
	for open.Type != remote.OuterOpen {
		open = readOuterFrame(t, ctx, host)
	}
	require.NoError(t, c.WriteMsg(ctx, []byte("hi")))
	for f := readOuterFrame(t, ctx, host); f.Type != remote.OuterData || f.Stream != open.Stream; f = readOuterFrame(t, ctx, host) {
	}
	writeHost(remote.OuterFrame{Type: remote.OuterAccept, Stream: open.Stream})
	writeHost(remote.OuterFrame{Type: remote.OuterData, Stream: open.Stream, Payload: []byte("hi")})
	_, err = c.ReadMsg(ctx)
	require.NoError(t, err)
	require.NoError(t, c.WriteMsg(ctx, make([]byte, 9000)))
	f := readOuterFrame(t, ctx, host)
	assert.Equal(t, remote.OuterData, f.Type, "前面 16000 字节没有 ACCEPT 的消息不算，这条还在额度里")
	assert.Len(t, f.Payload, 9000)
}

// wsPair 是一对连着的 WebSocket（测试里当客户端连接用）。
func wsPair(t *testing.T) (server, client *websocket.Conn) {
	t.Helper()
	got := make(chan *websocket.Conn, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		got <- ws
		<-r.Context().Done()
	}))
	t.Cleanup(ts.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http"), &websocket.DialOptions{HTTPClient: &http.Client{Transport: &http.Transport{Proxy: nil}}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.CloseNow() })
	return <-got, c
}

// 超额时，这台主机现在登记的连接（刚替换了旧连接）上的流也要关：旧连接最后几条 DATA 让用量过线的情况
func TestQuotaTripClosesCurrentHostStreams(t *testing.T) {
	s, err := New(Config{PublicURL: "ws://127.0.0.1:1", DailyBytesPerHost: 10})
	require.NoError(t, err)
	defer s.Close()
	old := &hostConn{s: s, id: "h1", streams: map[uint32]*clientConn{}, done: make(chan struct{})}
	cur := &hostConn{s: s, id: "h1", streams: map[uint32]*clientConn{}, done: make(chan struct{})}
	cur.closed.Store(true) // 不往主机写 CLOSE
	s.hosts["h1"] = cur
	srv, _ := wsPair(t)
	c := newClientConn(cur, srv, 1)
	cur.streams[1] = c
	assert.False(t, old.count(11), "旧连接让用量过线")
	assert.True(t, c.closed.Load(), "新连接上登记的流也关掉了")
	assert.Empty(t, cur.streamList())
	delete(s.hosts, "h1") // 没有真的主机连接，s.Close 不用关它
}

// 真正的主机拒绝握手（没有配对窗口、随便一把设备密钥，not_paired）时不发 ACCEPT，relay 不计量：
// 反复用没配对的密钥连也耗不掉额度，之后照样能配对
func TestRejectedHandshakesNotCounted(t *testing.T) {
	_, u := start(t, Config{MaxConnPerIPPerMin: -1, DailyBytesPerHost: 3000})
	h := relaytest.NewHost(t, u)
	ctx := context.Background()
	ov, err := h.Overview(ctx)
	require.NoError(t, err)
	hostKey, err := remote.DecodeKey(ov.HostKey)
	require.NoError(t, err)
	for i := 0; i < 20; i++ {
		dev, gerr := remote.GenerateKeypair(nil)
		require.NoError(t, gerr)
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		raw, derr := remote.DialRelay(cctx, u, ov.HostID, nil)
		require.NoError(t, derr)
		_, cerr := remote.Connect(cctx, raw, remote.ClientConfig{HostID: ov.HostID, HostKey: hostKey, Device: dev, Client: "intruder"})
		cancel()
		require.ErrorIs(t, cerr, remote.ErrNotPaired, "第 %d 次", i)
	}
	// 20 次握手一来一回合计超过 3000 字节，但都没有 ACCEPT：额度还在，配对照样成功
	ticket, err := h.StartPairing(ctx, remote.ScopesRead, "")
	require.NoError(t, err)
	link, err := remote.ParseLink(ticket.Link)
	require.NoError(t, err)
	dev, err := remote.GenerateKeypair(nil)
	require.NoError(t, err)
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	raw, err := remote.DialRelay(cctx, u, link.HostID, nil)
	require.NoError(t, err)
	pc, err := remote.Connect(cctx, raw, remote.ClientConfig{HostID: link.HostID, HostKey: link.HostKey, Device: dev, Client: "interop"})
	require.NoError(t, err)
	_, err = pc.Pair(cctx, link.Secret, "额度测试")
	require.NoError(t, err)
}
