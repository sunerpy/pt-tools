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
	r.Header.Set("X-Forwarded-For", " 5.6.7.8 , 10.0.0.1")
	assert.Equal(t, "5.6.7.8", s.clientIP(r))
	r.Header.Del("X-Forwarded-For")
	assert.Equal(t, "10.0.0.2", s.clientIP(r), "头为空时退回对端地址")
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
