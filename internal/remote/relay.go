package remote

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// relay 客户端：主机主动连 relay（wss://<relay>/v1/host/<hostId>），认证以后 relay 把客户端的连接按流转过来，
// 每个流是一条独立的 Noise 会话。断了按指数退避加抖动重连。
const (
	// MaxRelayStreams 是一条 relay 连接上同时打开的流数上限（与 relay 的默认限额一致）
	MaxRelayStreams = 16
	// relayKeepalive 是主机给 relay 发 ping 的间隔；relay 回 pong（文本消息，Cloudflare 的自动应答不用唤醒 Durable Object）
	relayKeepalive = 30 * time.Second
	// relayDeadAfter 是多久没收到 relay 的任何消息就当连接断了
	relayDeadAfter    = 3 * relayKeepalive
	relayDialTimeout  = 15 * time.Second
	relayAuthTimeout  = 10 * time.Second
	relayWriteTimeout = 30 * time.Second
	relayBackoffMin   = time.Second
	relayBackoffMax   = time.Minute
	// relayHealthy 是连上多久以后下次断开从最短的退避重新开始
	relayHealthy = time.Minute
	// streamQueue 是每个流排队等会话读取的消息数
	streamQueue = 64
	// ctrlQueue 是排队等发出的控制帧（CLOSE）数；堆满说明 relay 在不停地开流或者写不动了，断开重连
	ctrlQueue = 64
	relayPing = "ping"
	relayPong = "pong"
)

// relay 的连接状态。
const (
	RelayConnecting = "connecting"
	RelayOnline     = "online"
	RelayOffline    = "offline"
)

var errRelayClosed = errors.New("relay 连接已经断开")

// relayClient 维持到一个 relay 的连接。
type relayClient struct {
	h      *Host
	url    string
	origin string
	keys   *HostKeys
	hostID string
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	mu     sync.Mutex
	status RelayStatus
	mux    *relayMux
}

// syncRelaysLocked 让 relay 客户端与设置一致：多的停掉，少的连上；hostId 变了的全部重连。
func (h *Host) syncRelaysLocked(st *hostState) {
	h.stopRelaysLocked(func(c *relayClient) bool { return slices.Contains(st.settings.Relays, c.url) && c.hostID == st.hostID })
	for _, u := range st.settings.Relays {
		if h.relays[u] == nil {
			h.relays[u] = h.startRelay(st, u)
		}
	}
}

// stopRelaysLocked 停掉 keep 返回 false 的 relay 客户端（keep 为 nil 时全部停掉），各自最多等 5 秒。
func (h *Host) stopRelaysLocked(keep func(*relayClient) bool) {
	for u, c := range h.relays {
		if keep != nil && keep(c) {
			continue
		}
		c.cancel()
		select {
		case <-c.done:
		case <-time.After(closeWait):
			h.logf("[远程访问] 等 relay %s 断开超时", u)
		}
		delete(h.relays, u)
	}
}

func (h *Host) startRelay(st *hostState, u string) *relayClient {
	ctx, cancel := context.WithCancel(h.ctx)
	c := &relayClient{
		h: h, url: u, keys: st.keys, hostID: st.hostID, ctx: ctx, cancel: cancel, done: make(chan struct{}),
		status: RelayStatus{URL: u, State: RelayConnecting, Since: h.cfg.Now()},
	}
	c.origin, _ = RelayOrigin(u)
	go c.run()
	return c
}

func (c *relayClient) setStatus(state string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	if c.status.State != state || c.status.Error != msg {
		c.status.State, c.status.Error, c.status.Since = state, msg, c.h.cfg.Now()
	}
}

func (c *relayClient) statusSnapshot() RelayStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.status
	if c.mux != nil {
		st.Streams = c.mux.streamCount()
	}
	return st
}

func (c *relayClient) run() {
	defer close(c.done)
	backoff := relayBackoffMin
	for {
		start := time.Now()
		err := c.connectOnce()
		if c.ctx.Err() != nil {
			return
		}
		if time.Since(start) >= relayHealthy {
			backoff = relayBackoffMin
		}
		c.setStatus(RelayOffline, err)
		c.h.cfg.Logger.Infof("[远程访问] relay %s 断开，%s 后重连: %v", c.url, backoff, err)
		// 抖动 ±20%，很多主机同时断开时不会一起重连
		wait := time.Duration(float64(backoff) * (0.8 + 0.4*rand.Float64()))
		t := time.NewTimer(wait)
		select {
		case <-t.C:
		case <-c.ctx.Done():
			t.Stop()
			return
		}
		backoff = min(backoff*2, relayBackoffMax)
	}
}

// connectOnce 连一次 relay：认证，然后转发流，直到连接断开。
func (c *relayClient) connectOnce() error {
	c.setStatus(RelayConnecting, nil)
	dctx, cancel := context.WithTimeout(c.ctx, relayDialTimeout)
	ws, resp, err := websocket.Dial(dctx, c.url+"/v1/host/"+c.hostID, &websocket.DialOptions{HTTPClient: c.h.cfg.HTTPClient})
	cancel()
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("连接 relay 失败: %w", err)
	}
	ws.SetReadLimit(MaxOuterFrame)
	defer func() { _ = ws.CloseNow() }()
	if err = c.authenticate(ws); err != nil {
		return err
	}
	m := newRelayMux(c, ws)
	c.mu.Lock()
	c.mux = m
	c.mu.Unlock()
	c.setStatus(RelayOnline, nil)
	err = m.run()
	c.mu.Lock()
	c.mux = nil
	c.mu.Unlock()
	return err
}

// authenticate 等 relay 的质询，回签名，等 READY。
func (c *relayClient) authenticate(ws *websocket.Conn) error {
	ctx, cancel := context.WithTimeout(c.ctx, relayAuthTimeout)
	defer cancel()
	f, err := readOuterFrame(ctx, ws)
	if err != nil {
		return fmt.Errorf("等 relay 的质询失败: %w", err)
	}
	if f.Type != OuterChallenge {
		return fmt.Errorf("%w: relay 没有先发质询", ErrOuter)
	}
	auth, err := AppendOuter(nil, OuterFrame{Type: OuterAuth, Payload: c.keys.RelayAuth(f.Payload, c.origin)})
	if err != nil {
		return err
	}
	if err = ws.Write(ctx, websocket.MessageBinary, auth); err != nil {
		return fmt.Errorf("回 relay 的质询失败: %w", err)
	}
	if f, err = readOuterFrame(ctx, ws); err != nil {
		if websocket.CloseStatus(err) == CloseAuthFailed {
			return errors.New("relay 认证没有通过")
		}
		return fmt.Errorf("等 relay 确认失败: %w", err)
	}
	if f.Type != OuterReady {
		return fmt.Errorf("%w: relay 没有确认认证", ErrOuter)
	}
	return nil
}

// readOuterFrame 读一个二进制外层帧（认证阶段用；文本消息算协议错误）。
func readOuterFrame(ctx context.Context, ws *websocket.Conn) (OuterFrame, error) {
	typ, b, err := ws.Read(ctx)
	if err != nil {
		return OuterFrame{}, err
	}
	if typ != websocket.MessageBinary {
		return OuterFrame{}, fmt.Errorf("%w: 认证阶段收到文本消息", ErrOuter)
	}
	return ParseOuter(b)
}

// relayMux 是认证以后的一条 relay 连接：一个读者按流分发，写按信号量串行。
type relayMux struct {
	c      *relayClient
	ws     *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc
	sem    chan struct{}
	// ctrl 是控制帧的发送队列，只有一个写者（不为每个 CLOSE 起一个 goroutine）
	ctrl chan []byte
	last atomic.Int64 // 最近一次收到 relay 消息的时间（UnixNano）

	mu      sync.Mutex
	streams map[uint32]*relayStream
}

func newRelayMux(c *relayClient, ws *websocket.Conn) *relayMux {
	ctx, cancel := context.WithCancel(c.ctx)
	m := &relayMux{c: c, ws: ws, ctx: ctx, cancel: cancel, sem: make(chan struct{}, 1), ctrl: make(chan []byte, ctrlQueue), streams: map[uint32]*relayStream{}}
	m.last.Store(time.Now().UnixNano())
	return m
}

func (m *relayMux) streamCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.streams)
}

// run 读 relay 发来的帧直到连接断开；返回前关掉所有流。
func (m *relayMux) run() error {
	defer func() {
		m.cancel()
		m.mu.Lock()
		list := make([]*relayStream, 0, len(m.streams))
		for _, s := range m.streams {
			list = append(list, s)
		}
		m.mu.Unlock()
		for _, s := range list {
			s.remoteClosed()
		}
	}()
	go m.keepalive()
	go m.ctrlWriter()
	for {
		typ, b, err := m.ws.Read(m.ctx)
		if err != nil {
			if m.c.ctx.Err() != nil {
				return errRelayClosed
			}
			return err
		}
		m.last.Store(time.Now().UnixNano())
		if typ == websocket.MessageText {
			// 只认 pong（回 ping 的）；别的文本消息忽略，以后的版本可以加
			continue
		}
		f, err := ParseOuter(b)
		if err != nil {
			_ = m.ws.Close(CloseProtocol, "bad frame")
			return err
		}
		if err := m.handle(f); err != nil {
			_ = m.ws.Close(CloseProtocol, "protocol error")
			return err
		}
	}
}

func (m *relayMux) handle(f OuterFrame) error {
	switch f.Type {
	case OuterOpen:
		m.mu.Lock()
		_, dup := m.streams[f.Stream]
		full := len(m.streams) >= MaxRelayStreams
		var s *relayStream
		if !dup && !full {
			s = &relayStream{m: m, id: f.Stream, in: make(chan []byte, streamQueue), closed: make(chan struct{})}
			m.streams[f.Stream] = s
		}
		m.mu.Unlock()
		switch {
		case dup:
			return fmt.Errorf("%w: 流 %d 已经打开", ErrOuter, f.Stream)
		case full:
			m.sendClose(f.Stream, CloseLimited, "too many streams")
			return nil
		}
		go m.c.h.serveConn(s, ViaRelay)
	case OuterData:
		m.mu.Lock()
		s := m.streams[f.Stream]
		m.mu.Unlock()
		if s != nil && !s.push(f.Payload) {
			// 会话读得太慢，排队满了：关掉这个流，不拖住别的流
			s.Close(CloseLimited, "slow reader")
		}
	case OuterClose:
		m.mu.Lock()
		s := m.streams[f.Stream]
		m.mu.Unlock()
		if s != nil {
			s.remoteClosed()
		}
	default:
		return fmt.Errorf("%w: 认证以后 relay 不该发类型 0x%02x", ErrOuter, byte(f.Type))
	}
	return nil
}

// keepalive 每 30 秒发一次 ping；90 秒没收到任何消息就断开重连。
func (m *relayMux) keepalive() {
	t := time.NewTicker(relayKeepalive)
	defer t.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-t.C:
		}
		if time.Since(time.Unix(0, m.last.Load())) > relayDeadAfter {
			m.c.h.cfg.Logger.Infof("[远程访问] relay %s 太久没有回应，重连", m.c.url)
			m.cancel()
			return
		}
		if err := m.writeMsg(m.ctx, websocket.MessageText, []byte(relayPing)); err != nil {
			m.cancel()
			return
		}
	}
}

// writeMsg 写一条消息。等写的名额时认调用方的 ctx；写本身用连接自己的 ctx 加超时：
// coder/websocket 在写的 ctx 结束时会关掉整条连接，不能让一个会话的结束连累同一条 relay 连接上的别的流。
func (m *relayMux) writeMsg(ctx context.Context, typ websocket.MessageType, b []byte) error {
	select {
	case m.sem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	case <-m.ctx.Done():
		return errRelayClosed
	}
	defer func() { <-m.sem }()
	wctx, cancel := context.WithTimeout(m.ctx, relayWriteTimeout)
	defer cancel()
	return m.ws.Write(wctx, typ, b)
}

func (m *relayMux) writeFrame(ctx context.Context, f OuterFrame) error {
	b, err := AppendOuter(make([]byte, 0, OuterHeaderLen+len(f.Payload)), f)
	if err != nil {
		return err
	}
	return m.writeMsg(ctx, websocket.MessageBinary, b)
}

// sendClose 把 CLOSE 排进控制帧队列（不阻塞调用方）；队列满了就断开这条 relay 连接。
func (m *relayMux) sendClose(id uint32, code int, reason string) {
	b, err := AppendOuter(make([]byte, 0, OuterHeaderLen+2+len(reason)), OuterFrame{Type: OuterClose, Stream: id, Payload: ClosePayload(uint16(code), reason)})
	if err != nil {
		return
	}
	select {
	case m.ctrl <- b:
	default:
		m.c.h.logf("[远程访问] relay %s 的控制帧堆满了（不停地开流或者写不动），断开重连", m.c.url)
		m.cancel()
	}
}

// ctrlWriter 是控制帧队列唯一的写者。
func (m *relayMux) ctrlWriter() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case b := <-m.ctrl:
			if err := m.writeMsg(m.ctx, websocket.MessageBinary, b); err != nil {
				m.cancel()
				return
			}
		}
	}
}

func (m *relayMux) removeStream(id uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.streams, id)
}

// relayStream 是 relay 连接里的一个流，实现 MsgConn。
type relayStream struct {
	m         *relayMux
	id        uint32
	in        chan []byte
	closed    chan struct{}
	closeOnce sync.Once
}

// push 把 relay 转来的一条消息排给会话；排满了返回 false。
func (s *relayStream) push(b []byte) bool {
	select {
	case <-s.closed:
		return true
	default:
	}
	select {
	case s.in <- b:
		return true
	default:
		return false
	}
}

func (s *relayStream) ReadMsg(ctx context.Context) ([]byte, error) {
	select {
	case b := <-s.in:
		return b, nil
	default:
	}
	select {
	case b := <-s.in:
		return b, nil
	case <-s.closed:
		return nil, errRelayClosed
	case <-ctx.Done():
		// 和直连的 WebSocket 一样：读的 ctx 结束（空闲超时、会话关闭）就关掉这个流
		s.Close(1000, "")
		return nil, ctx.Err()
	}
}

func (s *relayStream) WriteMsg(ctx context.Context, b []byte) error {
	select {
	case <-s.closed:
		return errRelayClosed
	default:
	}
	return s.m.writeFrame(ctx, OuterFrame{Type: OuterData, Stream: s.id, Payload: b})
}

// Close 关掉这个流并告诉 relay（relay 随后关掉客户端那条 WebSocket）。
func (s *relayStream) Close(code int, reason string) {
	s.closeOnce.Do(func() {
		close(s.closed)
		s.m.removeStream(s.id)
		s.m.sendClose(s.id, code, reason)
	})
}

// remoteClosed 是 relay 关了这个流（或者整条 relay 连接断了）：不用再告诉 relay。
func (s *relayStream) remoteClosed() {
	s.closeOnce.Do(func() {
		close(s.closed)
		s.m.removeStream(s.id)
	})
}
