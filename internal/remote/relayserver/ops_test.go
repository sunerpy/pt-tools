package relayserver

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/remote"
	"github.com/sunerpy/pt-tools/internal/remote/relaytest"
)

func httpBase(u string) string { return "http" + strings.TrimPrefix(u, "ws") }

// getJSON 取一个 JSON 回应：状态码与内容。
func getJSON(t *testing.T, url string) (int, map[string]string) {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	return resp.StatusCode, body
}

// metrics 读 /metrics，返回「名字{标签}」到值。
func metrics(t *testing.T, u string) map[string]int64 {
	t.Helper()
	resp, err := http.Get(httpBase(u) + "/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/plain; version=0.0.4; charset=utf-8", resp.Header.Get("Content-Type"))
	out := map[string]int64{}
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.LastIndexByte(line, ' ')
		require.Positive(t, i, line)
		v, err := strconv.ParseInt(line[i+1:], 10, 64)
		require.NoError(t, err, line)
		out[line[:i]] = v
	}
	require.NoError(t, sc.Err())
	return out
}

// dialStatus 连一个 WebSocket，返回升级失败时的 HTTP 状态码（成功时 101 与连接）。
func dialStatus(t *testing.T, ctx context.Context, url string) (int, *websocket.Conn, http.Header) {
	t.Helper()
	ws, resp, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		require.NotNil(t, resp, err)
		return resp.StatusCode, nil, resp.Header
	}
	t.Cleanup(func() { _ = ws.CloseNow() })
	return http.StatusSwitchingProtocols, ws, nil
}

// readClose 读到连接关闭，返回关闭码。
func readClose(t *testing.T, ctx context.Context, ws *websocket.Conn) websocket.StatusCode {
	t.Helper()
	for {
		if _, _, err := ws.Read(ctx); err != nil {
			return websocket.CloseStatus(err)
		}
	}
}

func TestReady(t *testing.T) {
	srv, u := start(t, Config{MaxConnPerIPPerMin: -1})
	code, body := getJSON(t, httpBase(u)+"/ready")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, map[string]string{"status": "ready"}, body)
	srv.Drain()
	code, body = getJSON(t, httpBase(u)+"/ready")
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, map[string]string{"status": "unready", "reason": "draining"}, body)

	_, du := start(t, Config{Disabled: true})
	code, body = getJSON(t, httpBase(du)+"/ready")
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, "disabled", body["reason"])

	_, fu := start(t, Config{MaxConnections: 1, MaxConnPerIPPerMin: -1})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	keys := newHostKeys(t)
	authHost(t, ctx, fu, keys)
	code, body = getJSON(t, httpBase(fu)+"/ready")
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, "full", body["reason"])
}

func newHostKeys(t *testing.T) *remote.HostKeys {
	t.Helper()
	k, err := remote.GenerateHostKeys(nil)
	require.NoError(t, err)
	return k
}

// 连接数到了上限：新连接在升级前回 503（Retry-After），已有的不受影响；断开一个以后名额还回来。
// 被拒绝的连接（hostId 不对、主机不在）也要把名额还回来。
func TestConnectionLimit(t *testing.T) {
	srv, u := start(t, Config{MaxConnections: 2, MaxConnPerIPPerMin: -1})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// 不对的 hostId 与不在线的主机：接受以后马上关掉，名额要还
	for range 3 {
		code, ws, _ := dialStatus(t, ctx, u+"/v1/client/bad")
		require.Equal(t, http.StatusSwitchingProtocols, code)
		assert.Equal(t, websocket.StatusCode(remote.CloseProtocol), readClose(t, ctx, ws))
		code, ws, _ = dialStatus(t, ctx, u+"/v1/client/"+newHostKeys(t).HostID())
		require.Equal(t, http.StatusSwitchingProtocols, code)
		assert.Equal(t, websocket.StatusCode(remote.CloseHostOffline), readClose(t, ctx, ws))
	}
	require.Eventually(t, func() bool { return srv.conns.Load() == 0 }, 5*time.Second, 10*time.Millisecond, "被拒绝的连接还了名额")

	keys := newHostKeys(t)
	authHost(t, ctx, u, keys)
	code, client, _ := dialStatus(t, ctx, u+"/v1/client/"+keys.HostID())
	require.Equal(t, http.StatusSwitchingProtocols, code)
	code, _, hdr := dialStatus(t, ctx, u+"/v1/client/"+keys.HostID())
	assert.Equal(t, http.StatusServiceUnavailable, code, "第 3 个连接")
	assert.Equal(t, "5", hdr.Get("Retry-After"))
	code, _, _ = dialStatus(t, ctx, u+"/v1/host/"+newHostKeys(t).HostID())
	assert.Equal(t, http.StatusServiceUnavailable, code, "主机也算名额")
	m := metrics(t, u)
	assert.Equal(t, int64(2), m["pt_relay_connections"])
	assert.Equal(t, int64(2), m["pt_relay_connection_limit"])
	assert.Equal(t, int64(1), m["pt_relay_hosts"])
	assert.Equal(t, int64(1), m["pt_relay_host_connections"])
	assert.Equal(t, int64(1), m["pt_relay_clients"])
	assert.Equal(t, int64(2), m[`pt_relay_rejected_total{reason="full"}`])

	require.NoError(t, client.Close(websocket.StatusNormalClosure, ""))
	require.Eventually(t, func() bool { return srv.conns.Load() == 1 }, 5*time.Second, 10*time.Millisecond)
	code, _, _ = dialStatus(t, ctx, u+"/v1/client/"+keys.HostID())
	assert.Equal(t, http.StatusSwitchingProtocols, code, "断开一个以后名额还回来")
}

// 负数 = 不限
func TestConnectionLimitUnlimited(t *testing.T) {
	srv, u := start(t, Config{MaxConnections: -1, MaxConnPerIPPerMin: -1})
	assert.Equal(t, -1, srv.cfg.MaxConnections)
	ok, _ := srv.ready()
	assert.True(t, ok)
	assert.Equal(t, int64(0), metrics(t, u)["pt_relay_connection_limit"])
	def, err := New(Config{PublicURL: "ws://127.0.0.1:1"})
	require.NoError(t, err)
	defer def.Close()
	assert.Equal(t, DefaultMaxConnections, def.cfg.MaxConnections, "0 用默认值")
}

// 排空：新连接回 503 draining，已经连着的主机照常（ping 有 pong）
func TestDrain(t *testing.T) {
	srv, u := start(t, Config{MaxConnPerIPPerMin: -1})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	keys := newHostKeys(t)
	host := authHost(t, ctx, u, keys)
	srv.Drain()
	resp, err := http.Get(httpBase(u) + "/v1/client/" + keys.HostID())
	require.NoError(t, err)
	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	resp.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Equal(t, "draining", body["error"])
	code, _, _ := dialStatus(t, ctx, u+"/v1/host/"+newHostKeys(t).HostID())
	assert.Equal(t, http.StatusServiceUnavailable, code)
	require.NoError(t, host.Write(ctx, websocket.MessageText, []byte("ping")))
	typ, b, err := host.Read(ctx)
	require.NoError(t, err)
	assert.Equal(t, websocket.MessageText, typ)
	assert.Equal(t, "pong", string(b))
	m := metrics(t, u)
	assert.Equal(t, int64(1), m["pt_relay_draining"])
	assert.Equal(t, int64(0), m["pt_relay_ready"])
	assert.Equal(t, int64(2), m[`pt_relay_rejected_total{reason="draining"}`])
}

// Close：主机、它的客户端、还在认证的主机都收到 1012；Close 很快返回
func TestCloseSendsServiceRestart(t *testing.T) {
	srv, u := start(t, Config{MaxConnPerIPPerMin: -1})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	keys := newHostKeys(t)
	host := authHost(t, ctx, u, keys)
	code, client, _ := dialStatus(t, ctx, u+"/v1/client/"+keys.HostID())
	require.Equal(t, http.StatusSwitchingProtocols, code)
	open := readOuterFrame(t, ctx, host)
	require.Equal(t, remote.OuterOpen, open.Type)
	// 还没回 AUTH 的主机
	code, pending, _ := dialStatus(t, ctx, u+"/v1/host/"+newHostKeys(t).HostID())
	require.Equal(t, http.StatusSwitchingProtocols, code)
	require.Equal(t, remote.OuterChallenge, readOuterFrame(t, ctx, pending).Type)

	results := make(chan websocket.StatusCode, 3)
	for _, ws := range []*websocket.Conn{host, client, pending} {
		go func() { results <- readClose(t, ctx, ws) }()
	}
	t0 := time.Now()
	srv.Close()
	assert.Less(t, time.Since(t0), 3*time.Second)
	for range 3 {
		select {
		case c := <-results:
			assert.Equal(t, websocket.StatusServiceRestart, c)
		case <-ctx.Done():
			t.Fatal("没有收到关闭")
		}
	}
}

// /metrics：计数与动作对得上；NoMetrics 时 404
func TestMetrics(t *testing.T) {
	_, u := start(t, Config{MaxConnPerIPPerMin: -1})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m := metrics(t, u)
	for _, k := range []string{
		"pt_relay_ready", "pt_relay_draining", "pt_relay_connections", "pt_relay_connection_limit", "pt_relay_hosts", "pt_relay_clients",
		`pt_relay_rejected_total{reason="rate_limited"}`, `pt_relay_closed_total{code="1012"}`, `pt_relay_closed_total{code="other"}`,
		`pt_relay_bytes_total{direction="to_host"}`, "pt_relay_auth_failures_total", "go_goroutines", "go_memstats_heap_inuse_bytes",
	} {
		_, ok := m[k]
		assert.True(t, ok, k)
	}
	assert.Equal(t, int64(1), m["pt_relay_ready"])
	assert.Equal(t, int64(DefaultMaxConnections), m["pt_relay_connection_limit"])

	// 认证失败：签错的签名
	keys := newHostKeys(t)
	code, bad, _ := dialStatus(t, ctx, u+"/v1/host/"+keys.HostID())
	require.Equal(t, http.StatusSwitchingProtocols, code)
	ch := readOuterFrame(t, ctx, bad)
	auth, _ := remote.AppendOuter(nil, remote.OuterFrame{Type: remote.OuterAuth, Payload: keys.RelayAuth(ch.Payload, "ws://wrong.example")})
	require.NoError(t, bad.Write(ctx, websocket.MessageBinary, auth))
	assert.Equal(t, websocket.StatusCode(remote.CloseAuthFailed), readClose(t, ctx, bad))

	// 转发：客户端发 3 字节给主机，主机回 5 字节
	host := authHost(t, ctx, u, keys)
	_, client, _ := dialStatus(t, ctx, u+"/v1/client/"+keys.HostID())
	open := readOuterFrame(t, ctx, host)
	require.NoError(t, client.Write(ctx, websocket.MessageBinary, []byte("abc")))
	data := readOuterFrame(t, ctx, host)
	require.Equal(t, remote.OuterData, data.Type)
	reply, _ := remote.AppendOuter(nil, remote.OuterFrame{Type: remote.OuterData, Stream: open.Stream, Payload: []byte("hello")})
	require.NoError(t, host.Write(ctx, websocket.MessageBinary, reply))
	_, got, err := client.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, "hello", string(got))
	m = metrics(t, u)
	assert.Equal(t, int64(1), m["pt_relay_auth_failures_total"])
	assert.Equal(t, int64(1), m[`pt_relay_closed_total{code="4401"}`])
	assert.Equal(t, int64(3), m[`pt_relay_bytes_total{direction="to_host"}`])
	assert.Equal(t, int64(5), m[`pt_relay_bytes_total{direction="to_client"}`])
	assert.Equal(t, int64(1), m["pt_relay_hosts"])
	assert.Equal(t, int64(1), m["pt_relay_clients"])

	// 主机断开：客户端收到 4404，计数
	require.NoError(t, host.Close(websocket.StatusNormalClosure, ""))
	assert.Equal(t, websocket.StatusCode(remote.CloseHostOffline), readClose(t, ctx, client))
	require.Eventually(t, func() bool {
		return metrics(t, u)[`pt_relay_closed_total{code="4404"}`] == 1
	}, 5*time.Second, 20*time.Millisecond)

	_, nu := start(t, Config{NoMetrics: true})
	resp, err := http.Get(httpBase(nu) + "/metrics")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// 暂停服务与限流也计入拒绝数
func TestMetricsRejected(t *testing.T) {
	_, du := start(t, Config{Disabled: true})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, ws, _ := dialStatus(t, ctx, du+"/v1/client/"+newHostKeys(t).HostID())
	assert.Equal(t, websocket.StatusCode(remote.CloseDisabled), readClose(t, ctx, ws))
	assert.Equal(t, int64(1), metrics(t, du)[`pt_relay_rejected_total{reason="disabled"}`])

	_, lu := start(t, Config{MaxConnPerIPPerMin: 1})
	_, ws, _ = dialStatus(t, ctx, lu+"/v1/client/"+newHostKeys(t).HostID())
	readClose(t, ctx, ws)
	_, ws, _ = dialStatus(t, ctx, lu+"/v1/client/"+newHostKeys(t).HostID())
	assert.Equal(t, websocket.StatusCode(remote.CloseLimited), readClose(t, ctx, ws))
	m := metrics(t, lu)
	assert.Equal(t, int64(1), m[`pt_relay_rejected_total{reason="rate_limited"}`])
	assert.Equal(t, int64(1), m[`pt_relay_closed_total{code="4429"}`])
}

func TestClosedWithOther(t *testing.T) {
	var st stats
	st.closedWith(1000)
	st.closedWith(3999)
	assert.Equal(t, int64(1), st.closed[0].Load())
	assert.Equal(t, int64(1), st.closed[len(closeCodes)].Load())
}

// relay 重启：旧实例 Close（1012）以后，真正的主机几秒内连上同一个地址上的新实例
func TestHostReconnectsAfterRestart(t *testing.T) {
	var h atomic.Pointer[http.Handler]
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { (*h.Load()).ServeHTTP(w, r) }))
	u := "ws://" + ts.Listener.Addr().String()
	cfg := Config{PublicURL: u, MaxConnPerIPPerMin: -1, Version: "test"}
	a, err := New(cfg)
	require.NoError(t, err)
	ha := a.Handler()
	h.Store(&ha)
	ts.Start()
	t.Cleanup(ts.Close)
	host := relaytest.NewHost(t, u)

	b, err := New(cfg)
	require.NoError(t, err)
	t.Cleanup(b.Close)
	hb := b.Handler()
	h.Store(&hb)
	t0 := time.Now()
	a.Close()
	require.Eventually(t, func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		return len(b.hosts) == 1
	}, 10*time.Second, 50*time.Millisecond, "主机没有连上新的 relay")
	assert.Less(t, time.Since(t0), 6*time.Second, "1012 以后 1–5 秒内重连")
	ov, err := host.Overview(context.Background())
	require.NoError(t, err)
	require.Len(t, ov.RelayStatus, 1)
	assert.Equal(t, remote.RelayOnline, ov.RelayStatus[0].State)
}

// Close 开始以后才来开流的客户端（和 Close 交错）收到 1012，不是 4404
func TestClientDuringCloseGetsServiceRestart(t *testing.T) {
	s, err := New(Config{PublicURL: "ws://127.0.0.1:1", MaxConnPerIPPerMin: -1})
	require.NoError(t, err)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	u := "ws://" + strings.TrimPrefix(ts.URL, "http://")
	keys := newHostKeys(t)
	// 模拟交错：Close 已经开始（closingAll），但这个客户端在置排空以前就占到了名额
	s.mu.Lock()
	s.closingAll = true
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	code, ws, _ := dialStatus(t, ctx, u+"/v1/client/"+keys.HostID())
	require.Equal(t, http.StatusSwitchingProtocols, code)
	assert.Equal(t, websocket.StatusServiceRestart, readClose(t, ctx, ws))
	s.Close()
}
