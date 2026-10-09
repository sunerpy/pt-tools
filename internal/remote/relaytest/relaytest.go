// Package relaytest 是 relay 的一致性测试（黑盒）：Go 自建版与 Cloudflare 版跑同一套用例，
// 对着一个已经起好的 relay（ws:// 或 wss:// 地址）连主机与客户端，只看线上的行为。
//
// 不同的限额要不同的 relay 实例，用例按 Profile 分组：
//   - main：每主机流上限小（MaxStreamsPerHost），每 IP 不限、每天不限，跑认证、转发、替换、关闭码这些；
//   - limits：每 IP 每分钟连接上限小、每天转发量小，跑限额（会让这个 IP 一分钟内连不上，单独起一个实例）；
//   - disabled：暂停服务，一律 4503。
package relaytest

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/remote"
)

// Profile 是 relay 实例的限额组合。
type Profile string

// 三种实例。
const (
	ProfileMain     Profile = "main"
	ProfileLimits   Profile = "limits"
	ProfileDisabled Profile = "disabled"
)

// Target 是要测的 relay。
type Target struct {
	// URL 是 relay 的地址（ws://127.0.0.1:8787）
	URL string
	// Origin 是 relay 校验签名用的 origin；为空时按 URL 推导
	Origin  string
	Profile Profile
	// MaxStreamsPerHost 是这个实例的每主机流上限（main）
	MaxStreamsPerHost int
	// MaxConnPerIPPerMin 是每 IP 每分钟连接上限（limits）
	MaxConnPerIPPerMin int
	// DailyBytesPerHost 是每主机每天转发量上限（limits）
	DailyBytesPerHost int64
}

const step = 10 * time.Second

func (tg Target) origin(t *testing.T) string {
	if tg.Origin != "" {
		return tg.Origin
	}
	o, err := remote.RelayOrigin(tg.URL)
	require.NoError(t, err)
	return o
}

func (tg Target) httpURL() string {
	return "http" + strings.TrimPrefix(tg.URL, "ws")
}

// Run 跑这个 Profile 的全部用例。
func Run(t *testing.T, tg Target) {
	t.Helper()
	switch tg.Profile {
	case ProfileMain:
		t.Run("healthz", func(t *testing.T) { testHealthz(t, tg, false) })
		t.Run("not_found", func(t *testing.T) { testNotFound(t, tg) })
		t.Run("bad_host_id", func(t *testing.T) { testBadHostID(t, tg) })
		t.Run("auth_ok", func(t *testing.T) { testAuthOK(t, tg) })
		t.Run("auth_wrong_origin", func(t *testing.T) { testAuthWrong(t, tg, "origin") })
		t.Run("auth_wrong_host", func(t *testing.T) { testAuthWrong(t, tg, "host") })
		t.Run("auth_bad_signature", func(t *testing.T) { testAuthWrong(t, tg, "sig") })
		t.Run("host_offline", func(t *testing.T) { testHostOffline(t, tg) })
		t.Run("forward", func(t *testing.T) { testForward(t, tg) })
		t.Run("ping_pong", func(t *testing.T) { testPingPong(t, tg) })
		t.Run("host_disconnect", func(t *testing.T) { testHostDisconnect(t, tg) })
		t.Run("replace", func(t *testing.T) { testReplace(t, tg) })
		t.Run("stream_limit", func(t *testing.T) { testStreamLimit(t, tg) })
		t.Run("client_text", func(t *testing.T) { testClientText(t, tg) })
		t.Run("client_too_big", func(t *testing.T) { testClientTooBig(t, tg) })
		t.Run("host_bad_frame", func(t *testing.T) { testHostBadFrame(t, tg) })
		t.Run("auth_timeout", func(t *testing.T) { testAuthTimeout(t, tg) })
	case ProfileLimits:
		t.Run("daily_bytes", func(t *testing.T) { testDailyBytes(t, tg) })
		// 最后跑：之后一分钟内这个 IP 连不上
		t.Run("per_ip", func(t *testing.T) { testPerIP(t, tg) })
	case ProfileDisabled:
		t.Run("healthz", func(t *testing.T) { testHealthz(t, tg, true) })
		t.Run("disabled", func(t *testing.T) { testDisabled(t, tg) })
	default:
		t.Fatalf("不认识的 Profile %q", tg.Profile)
	}
}

// ---------------- 工具 ----------------

func newKeys(t *testing.T) *remote.HostKeys {
	t.Helper()
	k, err := remote.GenerateHostKeys(nil)
	require.NoError(t, err)
	return k
}

func dial(t *testing.T, u string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), step)
	defer cancel()
	ws, resp, err := websocket.Dial(ctx, u, nil)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err, u)
	ws.SetReadLimit(remote.MaxOuterFrame + 16)
	t.Cleanup(func() { _ = ws.CloseNow() })
	return ws
}

func readOuter(t *testing.T, ws *websocket.Conn) remote.OuterFrame {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), step)
	defer cancel()
	for {
		typ, b, err := ws.Read(ctx)
		require.NoError(t, err)
		if typ == websocket.MessageText {
			continue
		}
		f, err := remote.ParseOuter(b)
		require.NoError(t, err)
		return f
	}
}

func writeOuter(t *testing.T, ws *websocket.Conn, f remote.OuterFrame) {
	t.Helper()
	b, err := remote.AppendOuter(nil, f)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), step)
	defer cancel()
	require.NoError(t, ws.Write(ctx, websocket.MessageBinary, b))
}

// closeCode 等连接关掉，返回关闭码（-1 表示没有关闭帧就断了）。
func closeCode(t *testing.T, ws *websocket.Conn) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*step)
	defer cancel()
	for {
		_, _, err := ws.Read(ctx)
		if err == nil {
			continue
		}
		if errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("连接一直没有关")
		}
		return int(websocket.CloseStatus(err))
	}
}

// host 是一台认证过的主机连接。
type host struct {
	t    *testing.T
	ws   *websocket.Conn
	keys *remote.HostKeys
}

func connectHost(t *testing.T, tg Target, keys *remote.HostKeys) *host {
	t.Helper()
	ws := dial(t, tg.URL+"/v1/host/"+keys.HostID())
	f := readOuter(t, ws)
	require.Equal(t, remote.OuterChallenge, f.Type)
	writeOuter(t, ws, remote.OuterFrame{Type: remote.OuterAuth, Payload: keys.RelayAuth(f.Payload, tg.origin(t))})
	f = readOuter(t, ws)
	require.Equal(t, remote.OuterReady, f.Type)
	return &host{t: t, ws: ws, keys: keys}
}

func (h *host) expect(typ remote.OuterType) remote.OuterFrame {
	h.t.Helper()
	f := readOuter(h.t, h.ws)
	require.Equal(h.t, typ, f.Type, "主机等的帧")
	return f
}

func connectClient(t *testing.T, tg Target, hostID string) *websocket.Conn {
	t.Helper()
	return dial(t, tg.URL+"/v1/client/"+hostID)
}

func clientRead(t *testing.T, ws *websocket.Conn) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), step)
	defer cancel()
	typ, b, err := ws.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, websocket.MessageBinary, typ)
	return b
}

func clientWrite(t *testing.T, ws *websocket.Conn, b []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), step)
	defer cancel()
	require.NoError(t, ws.Write(ctx, websocket.MessageBinary, b))
}

// ---------------- 用例 ----------------

// httpGet 发一个 GET（不走代理：relay 在本机或测试网络里）。
func httpGet(t *testing.T, u string) *http.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), step)
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	require.NoError(t, err)
	resp, err := (&http.Client{Transport: &http.Transport{Proxy: nil}}).Do(req)
	require.NoError(t, err)
	return resp
}

func testHealthz(t *testing.T, tg Target, wantDisabled bool) {
	resp := httpGet(t, tg.httpURL()+"/healthz")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body struct {
		OK       bool   `json:"ok"`
		Version  string `json:"version"`
		Disabled bool   `json:"disabled"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body.OK)
	assert.NotEmpty(t, body.Version)
	assert.Equal(t, wantDisabled, body.Disabled)
}

func testNotFound(t *testing.T, tg Target) {
	resp := httpGet(t, tg.httpURL()+"/v1/other/x")
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func testBadHostID(t *testing.T, tg Target) {
	ws := dial(t, tg.URL+"/v1/client/NOT-A-HOST")
	assert.Equal(t, remote.CloseProtocol, closeCode(t, ws))
}

func testAuthOK(t *testing.T, tg Target) {
	h := connectHost(t, tg, newKeys(t))
	_ = h
}

func testAuthWrong(t *testing.T, tg Target, what string) {
	keys := newKeys(t)
	id := keys.HostID()
	if what == "host" {
		id = newKeys(t).HostID()
	}
	ws := dial(t, tg.URL+"/v1/host/"+id)
	f := readOuter(t, ws)
	require.Equal(t, remote.OuterChallenge, f.Type)
	origin := tg.origin(t)
	if what == "origin" {
		origin = "wss://another-relay.example.com"
	}
	auth := keys.RelayAuth(f.Payload, origin)
	if what == "sig" {
		auth[len(auth)-1] ^= 1
	}
	writeOuter(t, ws, remote.OuterFrame{Type: remote.OuterAuth, Payload: auth})
	assert.Equal(t, remote.CloseAuthFailed, closeCode(t, ws))
}

func testHostOffline(t *testing.T, tg Target) {
	ws := connectClient(t, tg, newKeys(t).HostID())
	assert.Equal(t, remote.CloseHostOffline, closeCode(t, ws))
}

func testForward(t *testing.T, tg Target) {
	h := connectHost(t, tg, newKeys(t))
	c1 := connectClient(t, tg, h.keys.HostID())
	o1 := h.expect(remote.OuterOpen)
	c2 := connectClient(t, tg, h.keys.HostID())
	o2 := h.expect(remote.OuterOpen)
	assert.NotZero(t, o1.Stream)
	assert.Greater(t, o2.Stream, o1.Stream, "流编号递增")

	// 客户端 → 主机：每条二进制消息装进 DATA，内容不变（最大 65535 字节）
	big := make([]byte, remote.MaxNoiseMessage)
	_, _ = rand.Read(big)
	clientWrite(t, c1, []byte("hello"))
	f := h.expect(remote.OuterData)
	assert.Equal(t, o1.Stream, f.Stream)
	assert.Equal(t, "hello", string(f.Payload))
	clientWrite(t, c2, big)
	f = h.expect(remote.OuterData)
	assert.Equal(t, o2.Stream, f.Stream)
	assert.True(t, bytes.Equal(big, f.Payload))

	// 主机 → 客户端
	writeOuter(t, h.ws, remote.OuterFrame{Type: remote.OuterData, Stream: o1.Stream, Payload: []byte("world")})
	assert.Equal(t, "world", string(clientRead(t, c1)))
	writeOuter(t, h.ws, remote.OuterFrame{Type: remote.OuterData, Stream: o2.Stream, Payload: big})
	assert.True(t, bytes.Equal(big, clientRead(t, c2)))
	// 不认识的流上的 DATA 忽略
	writeOuter(t, h.ws, remote.OuterFrame{Type: remote.OuterData, Stream: 0x7fffffff, Payload: []byte("x")})

	// 客户端关掉：主机收到 CLOSE
	require.NoError(t, c1.Close(websocket.StatusNormalClosure, ""))
	f = h.expect(remote.OuterClose)
	assert.Equal(t, o1.Stream, f.Stream)

	// 主机关掉一个流：客户端收到主机给的关闭码
	writeOuter(t, h.ws, remote.OuterFrame{Type: remote.OuterClose, Stream: o2.Stream, Payload: remote.ClosePayload(4321, "bye")})
	assert.Equal(t, 4321, closeCode(t, c2))
}

func testPingPong(t *testing.T, tg Target) {
	h := connectHost(t, tg, newKeys(t))
	ctx, cancel := context.WithTimeout(context.Background(), step)
	defer cancel()
	require.NoError(t, h.ws.Write(ctx, websocket.MessageText, []byte("ping")))
	typ, b, err := h.ws.Read(ctx)
	require.NoError(t, err)
	assert.Equal(t, websocket.MessageText, typ)
	assert.Equal(t, "pong", string(b))
}

func testHostDisconnect(t *testing.T, tg Target) {
	h := connectHost(t, tg, newKeys(t))
	c := connectClient(t, tg, h.keys.HostID())
	h.expect(remote.OuterOpen)
	require.NoError(t, h.ws.Close(websocket.StatusNormalClosure, ""))
	assert.Equal(t, remote.CloseHostOffline, closeCode(t, c))
	// 主机断开以后新的客户端也是 4404
	c2 := connectClient(t, tg, h.keys.HostID())
	assert.Equal(t, remote.CloseHostOffline, closeCode(t, c2))
}

func testReplace(t *testing.T, tg Target) {
	keys := newKeys(t)
	old := connectHost(t, tg, keys)
	c := connectClient(t, tg, keys.HostID())
	old.expect(remote.OuterOpen)
	fresh := connectHost(t, tg, keys)
	assert.Equal(t, remote.CloseReplaced, closeCode(t, old.ws))
	assert.Equal(t, remote.CloseHostOffline, closeCode(t, c), "旧连接的客户端 4404")
	// 新连接照常工作
	c2 := connectClient(t, tg, keys.HostID())
	o := fresh.expect(remote.OuterOpen)
	clientWrite(t, c2, []byte("x"))
	f := fresh.expect(remote.OuterData)
	assert.Equal(t, o.Stream, f.Stream)
}

func testStreamLimit(t *testing.T, tg Target) {
	require.Positive(t, tg.MaxStreamsPerHost)
	h := connectHost(t, tg, newKeys(t))
	var clients []*websocket.Conn
	for i := 0; i < tg.MaxStreamsPerHost; i++ {
		clients = append(clients, connectClient(t, tg, h.keys.HostID()))
		h.expect(remote.OuterOpen)
	}
	extra := connectClient(t, tg, h.keys.HostID())
	assert.Equal(t, remote.CloseLimited, closeCode(t, extra))
	// 关掉一个以后又能开
	require.NoError(t, clients[0].Close(websocket.StatusNormalClosure, ""))
	h.expect(remote.OuterClose)
	again := connectClient(t, tg, h.keys.HostID())
	h.expect(remote.OuterOpen)
	clientWrite(t, again, []byte("ok"))
	h.expect(remote.OuterData)
}

func testClientText(t *testing.T, tg Target) {
	h := connectHost(t, tg, newKeys(t))
	c := connectClient(t, tg, h.keys.HostID())
	h.expect(remote.OuterOpen)
	ctx, cancel := context.WithTimeout(context.Background(), step)
	defer cancel()
	require.NoError(t, c.Write(ctx, websocket.MessageText, []byte("hello")))
	assert.Equal(t, remote.CloseProtocol, closeCode(t, c))
}

func testClientTooBig(t *testing.T, tg Target) {
	h := connectHost(t, tg, newKeys(t))
	c := connectClient(t, tg, h.keys.HostID())
	h.expect(remote.OuterOpen)
	ctx, cancel := context.WithTimeout(context.Background(), step)
	defer cancel()
	_ = c.Write(ctx, websocket.MessageBinary, make([]byte, remote.MaxNoiseMessage+1))
	assert.Equal(t, int(websocket.StatusMessageTooBig), closeCode(t, c))
}

func testHostBadFrame(t *testing.T, tg Target) {
	h := connectHost(t, tg, newKeys(t))
	c := connectClient(t, tg, h.keys.HostID())
	h.expect(remote.OuterOpen)
	ctx, cancel := context.WithTimeout(context.Background(), step)
	defer cancel()
	require.NoError(t, h.ws.Write(ctx, websocket.MessageBinary, []byte{0x55, 0, 0, 0, 1}))
	assert.Equal(t, remote.CloseProtocol, closeCode(t, h.ws))
	assert.Equal(t, remote.CloseHostOffline, closeCode(t, c))
}

func testAuthTimeout(t *testing.T, tg Target) {
	keys := newKeys(t)
	ws := dial(t, tg.URL+"/v1/host/"+keys.HostID())
	readOuter(t, ws)
	start := time.Now()
	code := closeCode(t, ws)
	assert.NotEqual(t, int(websocket.StatusNormalClosure), code)
	assert.GreaterOrEqual(t, time.Since(start), 5*time.Second, "认证超时不该太短")
}

func testDailyBytes(t *testing.T, tg Target) {
	require.Positive(t, tg.DailyBytesPerHost)
	h := connectHost(t, tg, newKeys(t))
	c := connectClient(t, tg, h.keys.HostID())
	o := h.expect(remote.OuterOpen)
	chunk := make([]byte, 32<<10)
	sent := int64(0)
	var code int
	for sent <= tg.DailyBytesPerHost+int64(len(chunk)) {
		ctx, cancel := context.WithTimeout(context.Background(), step)
		err := c.Write(ctx, websocket.MessageBinary, chunk)
		cancel()
		if err != nil {
			break
		}
		sent += int64(len(chunk))
		if sent <= tg.DailyBytesPerHost {
			f := h.expect(remote.OuterData)
			require.Equal(t, o.Stream, f.Stream)
		}
	}
	code = closeCode(t, c)
	assert.Equal(t, remote.CloseLimited, code, "超额以后客户端流 4429")
	c2 := connectClient(t, tg, h.keys.HostID())
	assert.Equal(t, remote.CloseLimited, closeCode(t, c2), "超额以后新的客户端也 4429")
	// 主机连接保持
	ctx, cancel := context.WithTimeout(context.Background(), step)
	defer cancel()
	require.NoError(t, h.ws.Write(ctx, websocket.MessageText, []byte("ping")))
	for {
		typ, b, err := h.ws.Read(ctx)
		require.NoError(t, err)
		if typ == websocket.MessageText {
			assert.Equal(t, "pong", string(b))
			break
		}
	}
}

func testPerIP(t *testing.T, tg Target) {
	require.Positive(t, tg.MaxConnPerIPPerMin)
	keys := newKeys(t)
	var mu sync.Mutex
	limited := 0
	for i := 0; i < tg.MaxConnPerIPPerMin+2; i++ {
		ws := connectClient(t, tg, keys.HostID())
		code := closeCode(t, ws)
		mu.Lock()
		if code == remote.CloseLimited {
			limited++
		}
		mu.Unlock()
	}
	// 这一分钟里前面的用例也连过：至少最后两次被拒
	assert.GreaterOrEqual(t, limited, 2, fmt.Sprintf("每 IP 每分钟 %d 次", tg.MaxConnPerIPPerMin))
}

func testDisabled(t *testing.T, tg Target) {
	keys := newKeys(t)
	for _, p := range []string{"/v1/host/", "/v1/client/"} {
		ws := dial(t, tg.URL+p+keys.HostID())
		assert.Equal(t, remote.CloseDisabled, closeCode(t, ws), p)
	}
}
