package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testDirect = "http://192.168.1.10:8080"

func wantGoAway(t *testing.T, c *Client, reason string) {
	t.Helper()
	waitDone(t, c)
	assert.Equal(t, reason, c.GoAway())
}

// 配对：扫码拿到的链接里有主机公钥与配对密钥；配对会话只能配对，输错计数并记审计；
// 配对成功以后主机用 GOAWAY paired 关掉会话、发通知，设备重连成正常会话，权限是配对时选的
func TestPairingFlow(t *testing.T) {
	th := newTestHost(t, Settings{DirectURL: testDirect})
	ctx := context.Background()
	ticket, err := th.StartPairing(ctx, ScopesRead, "")
	require.NoError(t, err)
	link, err := ParseLink(ticket.Link)
	require.NoError(t, err)
	assert.Equal(t, th.keys.HostID(), link.HostID)
	assert.Equal(t, th.keys.Noise.Public, link.HostKey)
	assert.Equal(t, testDirect, link.Direct)
	assert.Equal(t, PairingWaiting, th.PairingStatus().State)

	dk, err := GenerateKeypair(nil)
	require.NoError(t, err)
	c, err := th.connect(t, dk.Private, ViaDirect)
	require.NoError(t, err)
	assert.Equal(t, ModePairing, c.Mode())
	assert.Equal(t, "v1.0.0-test", c.HostVersion())

	status, _, body := get(t, c, "/api/app/v1/meta", nil)
	assert.Equal(t, http.StatusForbidden, status)
	assert.Contains(t, body, "配对会话只能配对")
	assert.Zero(t, th.disp.count())

	_, err = c.Pair(ctx, bytes.Repeat([]byte{1}, KeyLen), "我的手机")
	var pe *PairError
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, http.StatusUnauthorized, pe.Status)
	assert.Equal(t, "remote:pair denied:secret", <-th.audits)
	assert.Equal(t, 1, th.PairingStatus().Failures)

	dev, err := c.Pair(ctx, link.Secret, "我的手机")
	require.NoError(t, err)
	assert.Equal(t, "我的手机", dev.Name)
	assert.Equal(t, ScopesRead, dev.Scopes)
	assert.Equal(t, "remote:pair success", <-th.audits)
	wantGoAway(t, c, GoAwayPaired)
	select {
	case d := <-th.paired:
		assert.Equal(t, dev.ID, d.ID)
	case <-time.After(5 * time.Second):
		t.Fatal("没有发配对通知")
	}
	st := th.PairingStatus()
	assert.Equal(t, PairingPaired, st.State)
	require.NotNil(t, st.Device)
	assert.Equal(t, dev.ID, st.Device.ID)

	c, err = th.connect(t, dk.Private, ViaRelay)
	require.NoError(t, err)
	assert.Equal(t, ModeDevice, c.Mode())
	status, _, body = get(t, c, "/api/app/v1/meta", nil)
	assert.Equal(t, http.StatusOK, status)
	got := th.disp.last()
	assert.Equal(t, dev.ID, got.Peer.Device.ID)
	assert.Equal(t, ScopesRead, got.Peer.Device.Scopes)
	assert.Equal(t, ViaRelay, got.Peer.Via)
	assert.Contains(t, body, `"via":"relay"`)

	// 窗口用掉了：别的设备再来就是没配对
	other, err := GenerateKeypair(nil)
	require.NoError(t, err)
	_, err = th.connect(t, other.Private, ViaDirect)
	assert.ErrorIs(t, err, ErrNotPaired)
}

// 配对会话最多 4 条；新开窗口、取消窗口时窗口里的配对会话跟着断开
func TestPairingSessions(t *testing.T) {
	th := newTestHost(t, Settings{DirectURL: testDirect})
	ctx := context.Background()
	_, err := th.StartPairing(ctx, ScopesFull, "")
	require.NoError(t, err)
	var sessions []*Client
	for i := 0; i < maxPairingSessions; i++ {
		k, _ := GenerateKeypair(nil)
		pc, cerr := th.connect(t, k.Private, ViaDirect)
		require.NoError(t, cerr)
		require.Eventually(t, func() bool { return th.sessions.count() == i+1 }, 5*time.Second, 10*time.Millisecond)
		sessions = append(sessions, pc)
	}
	k, _ := GenerateKeypair(nil)
	_, err = th.connect(t, k.Private, ViaDirect)
	assert.ErrorContains(t, err, HelloBusy)

	_, err = th.StartPairing(ctx, ScopesFull, "")
	require.NoError(t, err)
	for _, c := range sessions {
		wantGoAway(t, c, GoAwayPairingClosed)
	}
	c, err := th.connect(t, k.Private, ViaDirect)
	require.NoError(t, err)
	th.CancelPairing()
	wantGoAway(t, c, GoAwayPairingClosed)
	assert.Equal(t, PairingClosed, th.PairingStatus().State)
	_, err = th.connect(t, k.Private, ViaDirect)
	assert.ErrorIs(t, err, ErrNotPaired)
}

// 配对窗口过期以后，配对会话里提交正确的密钥也回 410，会话随后关掉
func TestPairingExpiredInSession(t *testing.T) {
	clk := &fakeClock{t: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)}
	th := newTestHost(t, Settings{DirectURL: testDirect}, func(c *Config) { c.Now = clk.now })
	ctx := context.Background()
	ticket, err := th.StartPairing(ctx, ScopesFull, "")
	require.NoError(t, err)
	link, _ := ParseLink(ticket.Link)
	k, _ := GenerateKeypair(nil)
	c, err := th.connect(t, k.Private, ViaDirect)
	require.NoError(t, err)
	clk.add(PairingTTL + time.Second)
	_, err = c.Pair(ctx, link.Secret, "迟到的手机")
	var pe *PairError
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, http.StatusGone, pe.Status)
	wantGoAway(t, c, GoAwayPairingClosed)
	assert.Equal(t, PairingExpired, th.PairingStatus().State)
}

func TestStartPairingErrors(t *testing.T) {
	th := newTestHost(t, Settings{})
	ctx := context.Background()
	_, err := th.StartPairing(ctx, ScopesFull, "")
	assert.ErrorIs(t, err, ErrInvalid, "没有直连地址也没有 relay")
	ticket, err := th.StartPairing(ctx, ScopesFull, "https://pt.example.com/")
	require.NoError(t, err)
	link, _ := ParseLink(ticket.Link)
	assert.Equal(t, "https://pt.example.com", link.Direct)
	_, err = th.StartPairing(ctx, ScopesFull, "ftp://x")
	assert.ErrorIs(t, err, ErrInvalid)
	_, err = th.StartPairing(ctx, []string{"mcp:read"}, testDirect)
	assert.ErrorIs(t, err, ErrInvalid)
	_, err = th.UpdateSettings(ctx, Settings{Enabled: false})
	require.NoError(t, err)
	_, err = th.StartPairing(ctx, ScopesFull, testDirect)
	assert.ErrorIs(t, err, ErrDisabled)
}

// 设备会话只放行 /api/app/v1/*；只转发白名单里的请求头（Cookie、Authorization 一律去掉），回应头也只写回白名单里的
func TestDeviceSessionRouting(t *testing.T) {
	th := newTestHost(t, Settings{DirectURL: testDirect})
	d, priv := th.addDevice(t, "手机", ScopesFull)
	c, err := th.connect(t, priv, ViaDirect)
	require.NoError(t, err)
	require.Equal(t, ModeDevice, c.Mode())

	// 客户端那边只发白名单里的头；这里直接写帧，模拟一个不守规矩的设备
	head, _ := json.Marshal(RequestHead{Method: "GET", Path: "/api/app/v1/torrents?page=2", Headers: map[string]string{
		"Cookie": "session=x", "authorization": "Bearer ptt_1_x", "X-Forwarded-For": "1.2.3.4", "Accept-Language": "zh-CN", "Host": "evil",
	}})
	status, hdr, body := rawRequest(t, c, 101, head, nil)
	assert.Equal(t, http.StatusOK, status)
	got := th.disp.last()
	assert.Equal(t, "/api/app/v1/torrents?page=2", got.Path)
	assert.Empty(t, got.Header.Get("Cookie"))
	assert.Empty(t, got.Header.Get("Authorization"))
	assert.Empty(t, got.Header.Get("X-Forwarded-For"))
	assert.Equal(t, "zh-CN", got.Header.Get("Accept-Language"))
	assert.Equal(t, d.ID, got.Peer.Device.ID)
	assert.Equal(t, "no-store", hdr.Get("Cache-Control"))
	assert.Equal(t, "application/json", hdr.Get("Content-Type"))
	assert.Empty(t, hdr.Get("Set-Cookie"))
	assert.Empty(t, hdr.Get("X-Internal"))
	assert.Contains(t, body, `"path":"/api/app/v1/torrents"`)

	before := th.disp.count()
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/api/sites", http.StatusForbidden},
		{"GET", "/api/app/v1", http.StatusForbidden},
		{"POST", "/remote/v1/pair", http.StatusForbidden},
		{"GET", "/mcp", http.StatusForbidden},
		{"GET", "/api/app/v1/../sites", http.StatusBadRequest},
		{"GET", "/api/app/v1/./meta", http.StatusBadRequest},
		{"GET", "/api/app/v1//meta", http.StatusBadRequest},
		{"GET", "/api/app/v1/%2e%2e/sites", http.StatusBadRequest},
		{"GET", "/api/app/v1/meta/", http.StatusBadRequest},
		{"GET", "http://evil/api/app/v1/meta", http.StatusBadRequest},
		{"GET", "/api/app/v1/a b", http.StatusBadRequest},
		{"GET", "/api/app/v1/a#b", http.StatusBadRequest},
		{"GET", "/api/app/v1/a?x=%zz", http.StatusBadRequest},
		{"CONNECT", "/api/app/v1/meta", http.StatusMethodNotAllowed},
		{"TRACE", "/api/app/v1/meta", http.StatusMethodNotAllowed},
	} {
		h, _ := json.Marshal(RequestHead{Method: tc.method, Path: tc.path})
		got, _, _ := rawRequest(t, c, 102, h, nil)
		assert.Equal(t, tc.status, got, "%s %s", tc.method, tc.path)
	}
	assert.Equal(t, before, th.disp.count(), "被拒的请求一条也没有交给 App API")

	// HEAD 不写回应体；大回应拆成多个帧，收齐以后一字不差；什么也没写的处理函数回 200 空体
	head, _ = json.Marshal(RequestHead{Method: "HEAD", Path: "/api/app/v1/meta"})
	status, _, body = rawRequest(t, c, 103, head, nil)
	assert.Equal(t, http.StatusOK, status)
	assert.Empty(t, body)
	status, hdr, body = get(t, c, "/api/app/v1/big", nil)
	assert.Equal(t, http.StatusOK, status)
	assert.Len(t, body, 3*MaxFramePayload+7)
	assert.Equal(t, "application/octet-stream", hdr.Get("Content-Type"))
	status, _, body = get(t, c, "/api/app/v1/nothing", nil)
	assert.Equal(t, http.StatusOK, status)
	assert.Empty(t, body)

	// 处理函数 panic：回 500，会话照样能用
	status, _, _ = get(t, c, "/api/app/v1/panic", nil)
	assert.Equal(t, http.StatusInternalServerError, status)
	status, _, _ = get(t, c, "/api/app/v1/meta", nil)
	assert.Equal(t, http.StatusOK, status)

	// 带请求体的 POST
	req, _ := http.NewRequest(http.MethodPost, "/api/app/v1/push", strings.NewReader(`{"site":"a","id":"1"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.RoundTrip(req)
	require.NoError(t, err)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(b), `{\"site\":\"a\",\"id\":\"1\"}`)
	assert.Equal(t, "application/json", th.disp.last().Header.Get("Content-Type"))
}

// rawRequest 直接写 REQ_HEAD、REQ_BODY、REQ_END（绕过 Client 的白名单），等回应。
func rawRequest(t *testing.T, c *Client, id uint32, head, body []byte) (int, http.Header, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	call := &clientCall{head: make(chan *http.Response, 1)}
	call.body = &callBody{notify: make(chan struct{}, 1)}
	c.mu.Lock()
	c.pending[id] = call
	c.mu.Unlock()
	require.NoError(t, c.conn.WriteFrame(ctx, Frame{Type: FrameReqHead, ID: id, Payload: head}))
	if len(body) > 0 {
		require.NoError(t, c.conn.WriteFrame(ctx, Frame{Type: FrameReqBody, ID: id, Payload: body}))
	}
	require.NoError(t, c.conn.WriteFrame(ctx, Frame{Type: FrameReqEnd, ID: id}))
	select {
	case resp := <-call.head:
		require.NotNil(t, resp)
		b, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, resp.Header, string(b)
	case <-ctx.Done():
		t.Fatal("没有等到回应")
	}
	return 0, nil, ""
}

// 同时最多 16 个请求，第 17 个直接回 429；设备取消请求以后处理函数的 ctx 结束，名额空出来
func TestInflightLimitAndCancel(t *testing.T) {
	th := newTestHost(t, Settings{DirectURL: testDirect})
	_, priv := th.addDevice(t, "手机", ScopesFull)
	c, err := th.connect(t, priv, ViaDirect)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	errs := make(chan error, MaxInflight)
	for i := 0; i < MaxInflight; i++ {
		go func() {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/api/app/v1/slow", nil)
			_, err := c.RoundTrip(req)
			errs <- err
		}()
	}
	for i := 0; i < MaxInflight; i++ {
		select {
		case <-th.disp.started:
		case <-time.After(5 * time.Second):
			t.Fatal("慢请求没有全部开始")
		}
	}
	status, _, body := get(t, c, "/api/app/v1/meta", nil)
	assert.Equal(t, http.StatusTooManyRequests, status)
	assert.Contains(t, body, "too_many_requests")

	cancel()
	for i := 0; i < MaxInflight; i++ {
		select {
		case <-th.disp.canceled:
		case <-time.After(5 * time.Second):
			t.Fatal("取消没有传到处理函数")
		}
		assert.ErrorIs(t, <-errs, context.Canceled)
	}
	require.Eventually(t, func() bool {
		s, _, _ := get(t, c, "/api/app/v1/meta", nil)
		return s == http.StatusOK
	}, 5*time.Second, 20*time.Millisecond)
}

// 请求体：单个超过 16 MiB 回 413；一条会话里没处理完的请求体合计也不超过 16 MiB
func TestRequestBodyLimits(t *testing.T) {
	th := newTestHost(t, Settings{DirectURL: testDirect})
	_, priv := th.addDevice(t, "手机", ScopesFull)
	c, err := th.connect(t, priv, ViaDirect)
	require.NoError(t, err)
	ctx := context.Background()
	chunk := bytes.Repeat([]byte{'a'}, MaxFramePayload)
	sendBody := func(id uint32, n int) {
		for sent := 0; sent < n; {
			k := min(n-sent, len(chunk))
			require.NoError(t, c.conn.WriteFrame(ctx, Frame{Type: FrameReqBody, ID: id, Payload: chunk[:k]}))
			sent += k
		}
	}
	register := func(id uint32) *clientCall {
		call := &clientCall{head: make(chan *http.Response, 1), body: &callBody{notify: make(chan struct{}, 1)}}
		c.mu.Lock()
		c.pending[id] = call
		c.mu.Unlock()
		head, _ := json.Marshal(RequestHead{Method: "POST", Path: "/api/app/v1/push"})
		require.NoError(t, c.conn.WriteFrame(ctx, Frame{Type: FrameReqHead, ID: id, Payload: head}))
		return call
	}
	statusOf := func(call *clientCall) int {
		select {
		case resp := <-call.head:
			return resp.StatusCode
		case <-time.After(10 * time.Second):
			t.Fatal("没有等到回应")
		}
		return 0
	}

	big := register(201)
	sendBody(201, MaxRequestBody+1)
	assert.Equal(t, http.StatusRequestEntityTooLarge, statusOf(big))
	require.NoError(t, c.conn.WriteFrame(ctx, Frame{Type: FrameReqEnd, ID: 201}))

	a := register(202)
	sendBody(202, 9<<20)
	b := register(203)
	sendBody(203, 9<<20)
	assert.Equal(t, http.StatusRequestEntityTooLarge, statusOf(b))
	require.NoError(t, c.conn.WriteFrame(ctx, Frame{Type: FrameReqEnd, ID: 203}))
	require.NoError(t, c.conn.WriteFrame(ctx, Frame{Type: FrameReqEnd, ID: 202}))
	assert.Equal(t, http.StatusOK, statusOf(a))
	assert.Len(t, th.disp.last().Body, 9<<20)

	// 处理完以后额度还回来了
	ok := register(204)
	sendBody(204, 9<<20)
	require.NoError(t, c.conn.WriteFrame(ctx, Frame{Type: FrameReqEnd, ID: 204}))
	assert.Equal(t, http.StatusOK, statusOf(ok))
}

// 违反协议（请求编号还在用又发 REQ_HEAD、设备发了回应帧）：主机发 GOAWAY protocol_error 并断开
func TestProtocolViolations(t *testing.T) {
	th := newTestHost(t, Settings{DirectURL: testDirect})
	_, priv := th.addDevice(t, "手机", ScopesFull)
	ctx := context.Background()
	head, _ := json.Marshal(RequestHead{Method: "GET", Path: "/api/app/v1/meta"})

	c, err := th.connect(t, priv, ViaDirect)
	require.NoError(t, err)
	require.NoError(t, c.conn.WriteFrame(ctx, Frame{Type: FrameReqHead, ID: 5, Payload: head}))
	require.NoError(t, c.conn.WriteFrame(ctx, Frame{Type: FrameReqHead, ID: 5, Payload: head}))
	wantGoAway(t, c, GoAwayProtocol)

	c, err = th.connect(t, priv, ViaDirect)
	require.NoError(t, err)
	require.NoError(t, c.conn.WriteFrame(ctx, Frame{Type: FrameRespEnd, ID: 5}))
	wantGoAway(t, c, GoAwayProtocol)

	// REQ_HEAD 不是 JSON：只回 400，会话不断
	c, err = th.connect(t, priv, ViaDirect)
	require.NoError(t, err)
	status, _, _ := rawRequest(t, c, 9, []byte("not json"), nil)
	assert.Equal(t, http.StatusBadRequest, status)
	status, _, _ = get(t, c, "/api/app/v1/meta", nil)
	assert.Equal(t, http.StatusOK, status)

	// 不认识的编号上的请求体、结束、取消一律忽略
	require.NoError(t, c.conn.WriteFrame(ctx, Frame{Type: FrameReqBody, ID: 77, Payload: []byte("x")}))
	require.NoError(t, c.conn.WriteFrame(ctx, Frame{Type: FrameReqEnd, ID: 77}))
	require.NoError(t, c.conn.WriteFrame(ctx, Frame{Type: FrameCancel, ID: 77}))
	status, _, _ = get(t, c, "/api/app/v1/meta", nil)
	assert.Equal(t, http.StatusOK, status)
}

// PING 原样回 PONG；空闲超过 IdleTimeout 断开
func TestPingAndIdle(t *testing.T) {
	th := newTestHost(t, Settings{DirectURL: testDirect}, func(c *Config) { c.IdleTimeout = 300 * time.Millisecond })
	_, priv := th.addDevice(t, "手机", ScopesFull)
	dk, _ := KeypairFromPrivate(priv)
	a, b := memPipe()
	go th.serveConn(b, ViaDirect)
	ctx := context.Background()
	conn, hello, err := dialHandshake(ctx, a, th.keys.HostID(), th.keys.Noise.Public, dk, nil, ClientHello{})
	require.NoError(t, err)
	require.Equal(t, ModeDevice, hello.Mode)
	require.NoError(t, conn.WriteFrame(ctx, Frame{Type: FramePing, Payload: []byte("abc")}))
	f, err := conn.ReadFrame(ctx)
	require.NoError(t, err)
	assert.Equal(t, FramePong, f.Type)
	assert.Equal(t, "abc", string(f.Payload))
	start := time.Now()
	_, err = conn.ReadFrame(ctx)
	assert.Error(t, err, "空闲以后主机断开")
	assert.GreaterOrEqual(t, time.Since(start), 250*time.Millisecond)
}

// 撤销立即断开这台设备的全部会话，之后握手被拒；改权限也立即断开，重连以后按新权限；改名不断开
func TestRevokeAndScopeChange(t *testing.T) {
	th := newTestHost(t, Settings{DirectURL: testDirect})
	ctx := context.Background()
	d, priv := th.addDevice(t, "手机", ScopesFull)
	other, otherPriv := th.addDevice(t, "平板", ScopesFull)
	c1, err := th.connect(t, priv, ViaDirect)
	require.NoError(t, err)
	c2, err := th.connect(t, priv, ViaRelay)
	require.NoError(t, err)
	c3, err := th.connect(t, otherPriv, ViaDirect)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return th.sessions.count() == 3 }, 5*time.Second, 10*time.Millisecond)
	devices, err := th.Devices(ctx)
	require.NoError(t, err)
	require.Len(t, devices, 2)
	assert.True(t, devices[0].Online)

	name := "新名字"
	_, err = th.UpdateDevice(ctx, d.ID, &name, nil)
	require.NoError(t, err)
	status, _, _ := get(t, c1, "/api/app/v1/meta", nil)
	assert.Equal(t, http.StatusOK, status, "改名不断开")

	_, err = th.UpdateDevice(ctx, d.ID, nil, ScopesRead)
	require.NoError(t, err)
	wantGoAway(t, c1, GoAwayScopeChanged)
	wantGoAway(t, c2, GoAwayScopeChanged)
	c1, err = th.connect(t, priv, ViaDirect)
	require.NoError(t, err)
	get(t, c1, "/api/app/v1/meta", nil)
	assert.Equal(t, ScopesRead, th.disp.last().Peer.Device.Scopes)

	_, err = th.RevokeDevice(ctx, d.ID)
	require.NoError(t, err)
	wantGoAway(t, c1, GoAwayRevoked)
	_, err = th.connect(t, priv, ViaDirect)
	assert.ErrorIs(t, err, ErrNotPaired)

	// 别的设备不受影响
	status, _, _ = get(t, c3, "/api/app/v1/meta", nil)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, other.ID, th.disp.last().Peer.Device.ID)
	require.NoError(t, th.DeleteDevice(ctx, d.ID))
}

// 登记以后再核对：握手与登记之间被撤销、改了权限的会话会被关掉
func TestStillValid(t *testing.T) {
	th := newTestHost(t, Settings{DirectURL: testDirect})
	ctx := context.Background()
	d, priv := th.addDevice(t, "手机", ScopesFull)
	dk, _ := KeypairFromPrivate(priv)
	s := &session{mode: ModeDevice, device: d, peerKey: dk.Public}
	assert.Empty(t, th.stillValid(s))
	_, _, err := th.store.UpdateDevice(ctx, d.ID, nil, ScopesRead)
	require.NoError(t, err)
	assert.Equal(t, GoAwayScopeChanged, th.stillValid(s))
	_, err = th.store.RevokeDevice(ctx, d.ID)
	require.NoError(t, err)
	assert.Equal(t, GoAwayRevoked, th.stillValid(s))
	assert.Equal(t, GoAwayPairingClosed, th.stillValid(&session{mode: ModePairing}))

	// 配对会话只对握手时的那个窗口有效：换了新窗口，旧窗口的会话登记以后核对不过
	_, err = th.StartPairing(ctx, ScopesFull, "")
	require.NoError(t, err)
	oldGen := th.pairings.openWindow()
	assert.Empty(t, th.stillValid(&session{mode: ModePairing, pairGen: oldGen}))
	_, err = th.StartPairing(ctx, ScopesFull, "")
	require.NoError(t, err)
	assert.Equal(t, GoAwayPairingClosed, th.stillValid(&session{mode: ModePairing, pairGen: oldGen}))
	assert.Empty(t, th.stillValid(&session{mode: ModePairing, pairGen: th.pairings.openWindow()}))
}

// 关掉远程访问：全部会话收到 GOAWAY disabled，新连接握手不了；轮换主机密钥：全部会话收到 key_rotated，设备全部撤销
func TestDisableAndRotate(t *testing.T) {
	th := newTestHost(t, Settings{DirectURL: testDirect})
	ctx := context.Background()
	_, priv := th.addDevice(t, "手机", ScopesFull)
	c, err := th.connect(t, priv, ViaDirect)
	require.NoError(t, err)
	_, err = th.UpdateSettings(ctx, Settings{Enabled: false, DirectURL: testDirect})
	require.NoError(t, err)
	wantGoAway(t, c, GoAwayDisabled)
	assert.False(t, th.Enabled())
	_, err = th.connect(t, priv, ViaDirect)
	assert.Error(t, err)

	ov, err := th.UpdateSettings(ctx, Settings{Enabled: true, DirectURL: testDirect})
	require.NoError(t, err)
	assert.Equal(t, th.keys.HostID(), ov.HostID, "重新打开用的还是原来的密钥")
	c, err = th.connect(t, priv, ViaDirect)
	require.NoError(t, err)

	ov, err = th.RotateKeys(ctx)
	require.NoError(t, err)
	assert.NotEqual(t, th.keys.HostID(), ov.HostID)
	wantGoAway(t, c, GoAwayKeyRotated)
	devices, err := th.Devices(ctx)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	assert.NotNil(t, devices[0].RevokedAt)
	// 设备记着旧的主机公钥：握手解不开
	_, err = th.connect(t, priv, ViaDirect)
	assert.Error(t, err)
}

// 打开时生成不了主机密钥（没有加密密钥）：设置不写成打开
func TestEnableWithoutCipher(t *testing.T) {
	store := NewStore(newTestDB(t), nil)
	h := New(Config{Store: store, Dispatcher: newFakeDispatcher()})
	t.Cleanup(h.Close)
	_, err := h.UpdateSettings(context.Background(), Settings{Enabled: true})
	assert.ErrorIs(t, err, ErrNoCipher)
	set, err := store.Settings(context.Background())
	require.NoError(t, err)
	assert.False(t, set.Enabled)
	assert.False(t, h.Enabled())
}

// 关闭主机：全部会话收到 GOAWAY shutdown
func TestHostClose(t *testing.T) {
	th := newTestHost(t, Settings{DirectURL: testDirect})
	_, priv := th.addDevice(t, "手机", ScopesFull)
	c, err := th.connect(t, priv, ViaDirect)
	require.NoError(t, err)
	th.Close()
	wantGoAway(t, c, GoAwayShutdown)
	assert.False(t, th.Enabled())
	ov, err := th.Overview(context.Background())
	require.NoError(t, err)
	assert.Zero(t, ov.Sessions)
}

func TestOverview(t *testing.T) {
	old := DefaultRelayURL
	DefaultRelayURL = "wss://relay.example.com/"
	t.Cleanup(func() { DefaultRelayURL = old })
	th := newTestHost(t, Settings{DirectURL: testDirect})
	ov, err := th.Overview(context.Background())
	require.NoError(t, err)
	assert.True(t, ov.Enabled)
	assert.Equal(t, th.keys.HostID(), ov.HostID)
	assert.Equal(t, EncodeKey(th.keys.Noise.Public), ov.HostKey)
	assert.Equal(t, "wss://relay.example.com", ov.DefaultRelay)
	assert.Equal(t, StreamPath, ov.StreamPath)
	assert.Empty(t, ov.RelayStatus)
}

// 配对会话的请求体在收包时就按 4 KiB 截住；JSON 之后多出内容回 400；每个结果都记审计
func TestPairingRequestLimits(t *testing.T) {
	th := newTestHost(t, Settings{DirectURL: testDirect})
	ctx := context.Background()
	_, err := th.StartPairing(ctx, ScopesFull, "")
	require.NoError(t, err)
	k, _ := GenerateKeypair(nil)
	c, err := th.connect(t, k.Private, ViaDirect)
	require.NoError(t, err)
	require.Equal(t, ModePairing, c.Mode())

	head, _ := json.Marshal(RequestHead{Method: "POST", Path: PairPath})
	status, _, _ := rawRequest(t, c, 11, head, bytes.Repeat([]byte{'a'}, maxPairBody+1))
	assert.Equal(t, http.StatusRequestEntityTooLarge, status)

	status, _, body := rawRequest(t, c, 12, head, []byte(`{"secret":"x","name":"a"}{"more":1}`))
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Contains(t, body, "invalid_body")
	assert.Equal(t, "remote:pair denied:invalid_body", <-th.audits)

	status, _, _ = rawRequest(t, c, 13, head, []byte(`{"secret":"x","name":"a\nb"}`))
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, "remote:pair denied:invalid_name", <-th.audits)
	assert.Equal(t, 2, th.PairingStatus().Failures, "请求体与设备名不对也计入失败次数")

	// 再失败 3 次（任何一种），窗口作废，会话随后关掉；之后的请求也不再记审计
	for i := 0; i < 3; i++ {
		rawRequest(t, c, uint32(20+i), head, []byte(`not json`))
		<-th.audits
	}
	wantGoAway(t, c, GoAwayPairingClosed)
	assert.Equal(t, PairingClosed, th.PairingStatus().State)
	select {
	case a := <-th.audits:
		t.Fatalf("窗口作废以后又记了审计: %s", a)
	default:
	}
}

// 窗口关了（这里是过期）以后配对会话里的请求直接 410，会话关掉
func TestPairingRequestAfterWindowExpired(t *testing.T) {
	clk := &fakeClock{t: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)}
	th := newTestHost(t, Settings{DirectURL: testDirect}, func(c *Config) { c.Now = clk.now })
	_, err := th.StartPairing(context.Background(), ScopesFull, "")
	require.NoError(t, err)
	k, _ := GenerateKeypair(nil)
	c, err := th.connect(t, k.Private, ViaDirect)
	require.NoError(t, err)
	clk.add(PairingTTL)
	head, _ := json.Marshal(RequestHead{Method: "POST", Path: PairPath})
	status, _, _ := rawRequest(t, c, 31, head, []byte(`not json`))
	assert.Equal(t, http.StatusGone, status)
	assert.Equal(t, "remote:pair denied:pairing_closed", <-th.audits)
	wantGoAway(t, c, GoAwayPairingClosed)
}

// 第 5 次输错：当前与别的配对会话都收到 GOAWAY pairing_closed
func TestPairingFifthFailureClosesSessions(t *testing.T) {
	th := newTestHost(t, Settings{DirectURL: testDirect})
	ctx := context.Background()
	_, err := th.StartPairing(ctx, ScopesFull, "")
	require.NoError(t, err)
	k1, _ := GenerateKeypair(nil)
	k2, _ := GenerateKeypair(nil)
	c1, err := th.connect(t, k1.Private, ViaDirect)
	require.NoError(t, err)
	c2, err := th.connect(t, k2.Private, ViaDirect)
	require.NoError(t, err)
	for i := 0; i < MaxPairingFailures; i++ {
		_, perr := c1.Pair(ctx, bytes.Repeat([]byte{byte(i + 1)}, KeyLen), "x")
		var pe *PairError
		require.ErrorAs(t, perr, &pe)
		assert.Equal(t, http.StatusUnauthorized, pe.Status)
	}
	wantGoAway(t, c1, GoAwayPairingClosed)
	wantGoAway(t, c2, GoAwayPairingClosed)
	assert.Equal(t, PairingClosed, th.PairingStatus().State)
}

// 配对会话的名额在握手时原子地占：并发的未知设备最多拿到 4 条配对会话
func TestPairingSessionReservation(t *testing.T) {
	th := newTestHost(t, Settings{DirectURL: testDirect})
	_, err := th.StartPairing(context.Background(), ScopesFull, "")
	require.NoError(t, err)
	const n = 12
	var wg sync.WaitGroup
	var mu sync.Mutex
	var pairing, busy int
	var clients []*Client
	for i := 0; i < n; i++ {
		wg.Go(func() {
			k, _ := GenerateKeypair(nil)
			c, cerr := th.connect(t, k.Private, ViaDirect)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case cerr == nil && c.Mode() == ModePairing:
				pairing++
				clients = append(clients, c)
			case cerr != nil && strings.Contains(cerr.Error(), HelloBusy):
				busy++
			}
		})
	}
	wg.Wait()
	assert.Equal(t, maxPairingSessions, pairing)
	assert.Equal(t, n-maxPairingSessions, busy)
	// 关掉以后名额还回来
	th.CancelPairing()
	for _, c := range clients {
		waitDone(t, c)
	}
	_, err = th.StartPairing(context.Background(), ScopesFull, "")
	require.NoError(t, err)
	k, _ := GenerateKeypair(nil)
	require.Eventually(t, func() bool {
		c, cerr := th.connect(t, k.Private, ViaDirect)
		if cerr != nil {
			return false
		}
		c.Close()
		return true
	}, 5*time.Second, 20*time.Millisecond)
}

// 请求已经取消（浏览器断开）时关掉远程访问：设置写进去了，运行时也照样关掉
func TestDisableWithCanceledRequest(t *testing.T) {
	th := newTestHost(t, Settings{DirectURL: testDirect})
	_, priv := th.addDevice(t, "手机", ScopesFull)
	c, err := th.connect(t, priv, ViaDirect)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = th.UpdateSettings(ctx, Settings{Enabled: false, DirectURL: testDirect})
	require.NoError(t, err)
	wantGoAway(t, c, GoAwayDisabled)
	assert.False(t, th.Enabled())

	// 轮换与撤销同理
	_, err = th.UpdateSettings(context.Background(), Settings{Enabled: true, DirectURL: testDirect})
	require.NoError(t, err)
	c, err = th.connect(t, priv, ViaDirect)
	require.NoError(t, err)
	_, err = th.RotateKeys(ctx)
	require.NoError(t, err)
	wantGoAway(t, c, GoAwayKeyRotated)
}

// 关掉以后才来登记的会话：stillValid 先看远程访问是不是还开着
func TestStillValidWhenDisabled(t *testing.T) {
	th := newTestHost(t, Settings{DirectURL: testDirect})
	d, priv := th.addDevice(t, "手机", ScopesFull)
	dk, _ := KeypairFromPrivate(priv)
	s := &session{mode: ModeDevice, device: d, peerKey: dk.Public}
	assert.Empty(t, th.stillValid(s))
	th.state.Store(nil)
	assert.Equal(t, GoAwayDisabled, th.stillValid(s))
}
