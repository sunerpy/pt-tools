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
		t.Run("slow_reader", func(t *testing.T) { testSlowReader(t, tg) })
		t.Run("unconfirmed", func(t *testing.T) { testUnconfirmed(t, tg) })
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

// accept 发 ACCEPT：这个流的握手通过了（之后两个方向不再限于一条消息，开始计量）。
func (h *host) accept(stream uint32) {
	h.t.Helper()
	writeOuter(h.t, h.ws, remote.OuterFrame{Type: remote.OuterAccept, Stream: stream})
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

	// 客户端 → 主机：每条二进制消息装进 DATA，内容不变（第一条是握手消息）
	big := make([]byte, remote.MaxNoiseMessage)
	_, _ = rand.Read(big)
	clientWrite(t, c1, []byte("hello"))
	f := h.expect(remote.OuterData)
	assert.Equal(t, o1.Stream, f.Stream)
	assert.Equal(t, "hello", string(f.Payload))

	// 主机 → 客户端（ACCEPT 以后两个方向都不再限于一条、最大 65535 字节）
	h.accept(o1.Stream)
	writeOuter(t, h.ws, remote.OuterFrame{Type: remote.OuterData, Stream: o1.Stream, Payload: []byte("world")})
	assert.Equal(t, "world", string(clientRead(t, c1)))
	h.accept(o2.Stream)
	writeOuter(t, h.ws, remote.OuterFrame{Type: remote.OuterData, Stream: o2.Stream, Payload: big})
	assert.True(t, bytes.Equal(big, clientRead(t, c2)))
	clientWrite(t, c2, big)
	f = h.expect(remote.OuterData)
	assert.Equal(t, o2.Stream, f.Stream)
	assert.True(t, bytes.Equal(big, f.Payload))
	// 不认识的流上的 DATA 忽略
	writeOuter(t, h.ws, remote.OuterFrame{Type: remote.OuterData, Stream: 0x7fffffff, Payload: []byte("x")})

	// 客户端关掉：主机收到 CLOSE
	require.NoError(t, c1.Close(websocket.StatusNormalClosure, ""))
	f = h.expect(remote.OuterClose)
	assert.Equal(t, o1.Stream, f.Stream)

	// 主机发完最后一条 DATA 马上关掉流（例如 GOAWAY 之后）：客户端先收到那条 DATA，再收到主机给的关闭码
	writeOuter(t, h.ws, remote.OuterFrame{Type: remote.OuterData, Stream: o2.Stream, Payload: []byte("last")})
	writeOuter(t, h.ws, remote.OuterFrame{Type: remote.OuterClose, Stream: o2.Stream, Payload: remote.ClosePayload(4321, "bye")})
	assert.Equal(t, "last", string(clientRead(t, c2)))
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

// 一个客户端一直不读：relay 不能因为写不动它而停下读主机连接，同一台主机别的流照常转发。
func testSlowReader(t *testing.T, tg Target) {
	h := connectHost(t, tg, newKeys(t))
	slow := connectClient(t, tg, h.keys.HostID())
	slowOpen := h.expect(remote.OuterOpen)
	fast := connectClient(t, tg, h.keys.HostID())
	fastOpen := h.expect(remote.OuterOpen)
	_ = slow // 一直不读
	h.accept(slowOpen.Stream)
	h.accept(fastOpen.Stream)
	// 灌 24 MiB 给不读的客户端：超过两边的 TCP 缓冲与 relay 的排队
	frame, err := remote.AppendOuter(nil, remote.OuterFrame{Type: remote.OuterData, Stream: slowOpen.Stream, Payload: make([]byte, remote.MaxNoiseMessage)})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), step)
	defer cancel()
	for i := 0; i < 24<<20/remote.MaxNoiseMessage; i++ {
		require.NoError(t, h.ws.Write(ctx, websocket.MessageBinary, frame), "relay 停下来不读主机连接了（第 %d 条）", i)
	}
	writeOuter(t, h.ws, remote.OuterFrame{Type: remote.OuterData, Stream: fastOpen.Stream, Payload: []byte("fast")})
	assert.Equal(t, "fast", string(clientRead(t, fast)))
	clientWrite(t, fast, []byte("up"))
	for {
		f := readOuter(t, h.ws)
		if f.Type == remote.OuterData && f.Stream == fastOpen.Stream {
			assert.Equal(t, "up", string(f.Payload))
			break
		}
		// 不读的那个流可能已经被 relay 关掉（CLOSE）
		require.Equal(t, remote.OuterClose, f.Type, "主机收到的帧")
		require.Equal(t, slowOpen.Stream, f.Stream)
	}
}

// 主机发 ACCEPT 以前，每个方向只能发一条不超过 4096 字节的消息（客户端的 Noise 握手第一条、主机拒绝握手的回话），多的或者大的 4400。
func testUnconfirmed(t *testing.T, tg Target) {
	h := connectHost(t, tg, newKeys(t))
	c := connectClient(t, tg, h.keys.HostID())
	o := h.expect(remote.OuterOpen)
	clientWrite(t, c, make([]byte, remote.MaxUnconfirmed))
	f := h.expect(remote.OuterData)
	assert.Equal(t, o.Stream, f.Stream)
	clientWrite(t, c, []byte("second"))
	assert.Equal(t, remote.CloseProtocol, closeCode(t, c), "主机回话以前的第二条消息")
	f = h.expect(remote.OuterClose)
	assert.Equal(t, o.Stream, f.Stream)

	big := connectClient(t, tg, h.keys.HostID())
	ob := h.expect(remote.OuterOpen)
	clientWrite(t, big, make([]byte, remote.MaxUnconfirmed+1))
	assert.Equal(t, remote.CloseProtocol, closeCode(t, big), "主机回话以前的大消息")
	f = h.expect(remote.OuterClose)
	assert.Equal(t, ob.Stream, f.Stream)

	// 主机那边：ACCEPT 以前回一条（拒绝握手）可以，第二条 4400
	r := connectClient(t, tg, h.keys.HostID())
	or := h.expect(remote.OuterOpen)
	writeOuter(t, h.ws, remote.OuterFrame{Type: remote.OuterData, Stream: or.Stream, Payload: []byte("not_paired")})
	assert.Equal(t, "not_paired", string(clientRead(t, r)))
	writeOuter(t, h.ws, remote.OuterFrame{Type: remote.OuterData, Stream: or.Stream, Payload: []byte("more")})
	assert.Equal(t, remote.CloseProtocol, closeCode(t, r), "ACCEPT 以前主机的第二条")
	f = h.expect(remote.OuterClose)
	assert.Equal(t, or.Stream, f.Stream)
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
	// 被拒绝的握手（没有 ACCEPT 就关掉）不计量：两个方向各 4096 字节
	rej := connectClient(t, tg, h.keys.HostID())
	orj := h.expect(remote.OuterOpen)
	clientWrite(t, rej, make([]byte, remote.MaxUnconfirmed))
	h.expect(remote.OuterData)
	writeOuter(t, h.ws, remote.OuterFrame{Type: remote.OuterData, Stream: orj.Stream, Payload: make([]byte, remote.MaxUnconfirmed)})
	clientRead(t, rej)
	writeOuter(t, h.ws, remote.OuterFrame{Type: remote.OuterClose, Stream: orj.Stream, Payload: remote.ClosePayload(1000, "")})
	assert.Equal(t, 1000, closeCode(t, rej))
	// 握手：客户端一条、主机 ACCEPT 以后回一条，流上的转发量从这里开始计
	clientWrite(t, c, []byte("hi"))
	h.expect(remote.OuterData)
	h.accept(o.Stream)
	writeOuter(t, h.ws, remote.OuterFrame{Type: remote.OuterData, Stream: o.Stream, Payload: []byte("hi")})
	clientRead(t, c)
	chunk := make([]byte, 32<<10)
	sent := int64(4)
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
	// 用量按 hostId 计：主机重连（替换旧连接）不清零
	fresh := connectHost(t, tg, h.keys)
	assert.Equal(t, remote.CloseReplaced, closeCode(t, h.ws))
	h = fresh
	c3 := connectClient(t, tg, h.keys.HostID())
	assert.Equal(t, remote.CloseLimited, closeCode(t, c3), "主机重连以后照样 4429")
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
