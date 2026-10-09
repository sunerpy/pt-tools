// Package relayserver 是 relay 的 Go 自建版（路线图 M16）：只做转发，看不到明文。协议见 docs/design/remote-access.md 的「relay」。
//
// 主机连 /v1/host/<hostId>：质询、签名（签进本 relay 的 origin）、READY，之后按流转发外层帧；
// 客户端连 /v1/client/<hostId>：每条二进制消息是一条 Noise 消息，装进 DATA 转给主机。
// 限额：每主机并发流、每主机每天转发量、每 IP 每分钟新建连接；超额以 4429 关闭，暂停服务时一律 4503。
package relayserver

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/internal/remote"
)

// 默认限额（与托管版一致，每天转发量自建版默认不限）。
const (
	DefaultMaxStreamsPerHost  = 16
	DefaultMaxConnPerIPPerMin = 30
	// authTimeout 是主机从连上到回 AUTH 的上限
	authTimeout = 10 * time.Second
	// idleTimeout 是主机或客户端多久没发任何消息就断开（主机 30 秒一次 ping，设备 30 秒一次 PING）
	idleTimeout = 90 * time.Second
	// writeTimeout 是转发一条消息的上限：对端一直不读时断开它
	writeTimeout = 30 * time.Second
	// limiterMaxIPs 是限流表最多记的 IP 数
	limiterMaxIPs = 65536
	// clientQueue 是每个客户端排队等发出的消息数；排满说明它读得太慢，关掉它（4429），不拖住同一台主机别的流
	clientQueue = 64
)

// Config 是 relay 的设置。
type Config struct {
	// PublicURL 是 relay 对外的地址（例如 wss://relay.example.com）：主机按它签名，这里按它的 origin 校验
	PublicURL string
	// MaxStreamsPerHost 是每台主机同时打开的客户端流上限；0 时用默认值 16
	MaxStreamsPerHost int
	// DailyBytesPerHost 是每台主机每天（00:00 UTC 重置）转发的字节上限，两个方向合计；0 = 不限
	DailyBytesPerHost int64
	// MaxConnPerIPPerMin 是每个 IP 每分钟新建连接的上限（主机与客户端合计）；0 时用默认值 30，负数 = 不限
	MaxConnPerIPPerMin int
	// Disabled 为真时所有连接以 4503 关闭
	Disabled bool
	// ClientIPHeader 不为空时从这个请求头取客户端 IP（放在反向代理之后时，例如 X-Real-IP 或 CF-Connecting-IP）；
	// 头里有多个地址（X-Forwarded-For）时取最后一个，即代理追加的那个。只有确定请求都经过代理时才设，否则这个头可以随意伪造
	ClientIPHeader string
	Version        string
	Logger         *zap.SugaredLogger
	Now            func() time.Time
}

// Server 是 relay。
type Server struct {
	cfg    Config
	origin string
	ctx    context.Context
	stop   context.CancelFunc

	mu    sync.Mutex
	hosts map[string]*hostConn

	// usage 是每台主机（按 hostId）当天的转发量：主机重连、换连接都不清零；usageDay 换了一天时整张表清空
	usageMu  sync.Mutex
	usage    map[string]*usage
	usageDay string

	limiter *ipLimiter
}

// usage 是一台主机当天的转发量。
type usage struct {
	bytes int64
	over  bool
}

// New 校验设置并建 relay。
func New(cfg Config) (*Server, error) {
	origin, err := remote.RelayOrigin(cfg.PublicURL)
	if err != nil {
		return nil, fmt.Errorf("PublicURL 不对: %w", err)
	}
	if cfg.MaxStreamsPerHost <= 0 {
		cfg.MaxStreamsPerHost = DefaultMaxStreamsPerHost
	}
	if cfg.MaxConnPerIPPerMin == 0 {
		cfg.MaxConnPerIPPerMin = DefaultMaxConnPerIPPerMin
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop().Sugar()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	ctx, stop := context.WithCancel(context.Background())
	return &Server{
		cfg: cfg, origin: origin, ctx: ctx, stop: stop, hosts: map[string]*hostConn{}, usage: map[string]*usage{},
		limiter: newIPLimiter(cfg.MaxConnPerIPPerMin, time.Minute, cfg.Now),
	}, nil
}

// Handler 是 relay 的路由：/v1/host/{id}、/v1/client/{id}、/healthz。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/host/{id}", s.serveHost)
	mux.HandleFunc("GET /v1/client/{id}", s.serveClient)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "version": s.cfg.Version, "disabled": s.cfg.Disabled})
	})
	return mux
}

// Close 断开所有连接。
func (s *Server) Close() {
	s.stop()
	s.mu.Lock()
	hosts := make([]*hostConn, 0, len(s.hosts))
	for _, h := range s.hosts {
		hosts = append(hosts, h)
	}
	s.mu.Unlock()
	for _, h := range hosts {
		h.close(websocket.StatusGoingAway, "relay shutting down")
	}
}

// clientIP 是客户端的地址：设了 ClientIPHeader 时取这个头最后一行的最后一个地址（代理追加在最后，前面的部分客户端可以随便写）。
func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.ClientIPHeader != "" {
		if vals := r.Header.Values(s.cfg.ClientIPHeader); len(vals) > 0 {
			parts := strings.Split(vals[len(vals)-1], ",")
			if v := strings.TrimSpace(parts[len(parts)-1]); v != "" {
				return v
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// limitKey 是限流按什么计：IPv4 按地址，IPv6 按 /64（一台机器通常拿得到整个 /64，按地址计等于不限）。
func limitKey(ip string) string {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	a = a.WithZone("").Unmap()
	if a.Is4() {
		return a.String()
	}
	p, err := a.Prefix(64)
	if err != nil {
		return ip
	}
	return p.String()
}

// accept 升级成 WebSocket；暂停服务、限流与 hostId 不对时接受以后马上用对应的关闭码关掉（客户端能看到原因）。
func (s *Server) accept(w http.ResponseWriter, r *http.Request, readLimit int64) (*websocket.Conn, string, bool) {
	hostID := r.PathValue("id")
	// 客户端可以是浏览器里的 App（内容端到端加密，没有 Cookie 之类的环境凭证），不查 Origin
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return nil, "", false
	}
	ws.SetReadLimit(readLimit)
	switch {
	case s.cfg.Disabled:
		_ = ws.Close(remote.CloseDisabled, "relay disabled")
		return nil, "", false
	case !remote.ValidHostID(hostID):
		_ = ws.Close(remote.CloseProtocol, "bad host id")
		return nil, "", false
	case !s.limiter.allow(limitKey(s.clientIP(r))):
		_ = ws.Close(remote.CloseLimited, "too many connections")
		return nil, "", false
	}
	return ws, hostID, true
}

func (s *Server) serveHost(w http.ResponseWriter, r *http.Request) {
	ws, hostID, ok := s.accept(w, r, remote.MaxOuterFrame)
	if !ok {
		return
	}
	h := &hostConn{s: s, ws: ws, id: hostID, streams: map[uint32]*clientConn{}, done: make(chan struct{})}
	if err := h.authenticate(); err != nil {
		s.cfg.Logger.Infof("[relay] 主机 %s 认证没有通过: %v", hostID, err)
		_ = ws.Close(remote.CloseAuthFailed, "auth failed")
		return
	}
	// 拿住写锁再登记、写 READY：登记以后来的客户端的 OPEN 排在 READY 后面，主机一收到 READY 客户端也就找得到它
	h.writeMu.Lock()
	s.mu.Lock()
	old := s.hosts[hostID]
	s.hosts[hostID] = h
	s.mu.Unlock()
	ready, _ := remote.AppendOuter(nil, remote.OuterFrame{Type: remote.OuterReady})
	err := h.writeLocked(websocket.MessageBinary, ready)
	if testHookReady != nil {
		testHookReady()
	}
	h.writeMu.Unlock()
	if err != nil {
		// READY 没写出去：换回旧连接（它还没关），这条连接上已经登记的流随它一起以 4404 关掉
		s.mu.Lock()
		if s.hosts[hostID] == h {
			if old != nil {
				s.hosts[hostID] = old
			} else {
				delete(s.hosts, hostID)
			}
		}
		s.mu.Unlock()
		h.close(websocket.StatusInternalError, "")
		return
	}
	if old != nil {
		old.close(remote.CloseReplaced, "replaced by a newer connection")
	}
	s.cfg.Logger.Infof("[relay] 主机 %s 已连上（%s）", hostID, s.clientIP(r))
	h.readLoop()
	s.mu.Lock()
	if s.hosts[hostID] == h {
		delete(s.hosts, hostID)
	}
	s.mu.Unlock()
	h.close(websocket.StatusNormalClosure, "")
}

func (s *Server) serveClient(w http.ResponseWriter, r *http.Request) {
	ws, hostID, ok := s.accept(w, r, remote.MaxNoiseMessage)
	if !ok {
		return
	}
	s.mu.Lock()
	h := s.hosts[hostID]
	s.mu.Unlock()
	if h == nil {
		_ = ws.Close(remote.CloseHostOffline, "host offline")
		return
	}
	c, code, reason := h.openStream(ws)
	if c == nil {
		_ = ws.Close(websocket.StatusCode(code), reason)
		return
	}
	go c.writeLoop()
	c.readLoop()
}

// testHookReady 在主机的 READY 写出以后、放开写锁以前调用（测试用：拉长 READY 前后的时序）。
var testHookReady func()

// errClosed 是连接已经关了。
var errClosed = errors.New("连接已经关闭")

// hostConn 是一台主机的连接与它的客户端流。
type hostConn struct {
	s  *Server
	ws *websocket.Conn
	id string

	writeMu sync.Mutex
	closed  atomic.Bool
	done    chan struct{}
	once    sync.Once

	mu      sync.Mutex
	streams map[uint32]*clientConn
	next    uint32
}

func (h *hostConn) authenticate() error {
	nonce := make([]byte, remote.NonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	if err := h.writeOuter(remote.OuterFrame{Type: remote.OuterChallenge, Payload: nonce}); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(h.s.ctx, authTimeout)
	defer cancel()
	typ, b, err := h.ws.Read(ctx)
	if err != nil {
		return err
	}
	if typ != websocket.MessageBinary {
		return errors.New("认证阶段收到文本消息")
	}
	f, err := remote.ParseOuter(b)
	if err != nil {
		return err
	}
	if f.Type != remote.OuterAuth {
		return errors.New("没有回 AUTH")
	}
	return remote.VerifyRelayAuth(h.id, nonce, h.s.origin, f.Payload)
}

// write 写一条消息给主机（串行；写不动时断开主机）。
func (h *hostConn) write(typ websocket.MessageType, b []byte) error {
	if h.closed.Load() {
		return errClosed
	}
	h.writeMu.Lock()
	defer h.writeMu.Unlock()
	if err := h.writeLocked(typ, b); err != nil {
		go h.close(websocket.StatusPolicyViolation, "write timeout")
		return err
	}
	return nil
}

// writeLocked 写一条消息（调用方持有 writeMu）。
func (h *hostConn) writeLocked(typ websocket.MessageType, b []byte) error {
	if h.closed.Load() {
		return errClosed
	}
	ctx, cancel := context.WithTimeout(h.s.ctx, writeTimeout)
	defer cancel()
	return h.ws.Write(ctx, typ, b)
}

func (h *hostConn) writeOuter(f remote.OuterFrame) error {
	b, err := remote.AppendOuter(make([]byte, 0, remote.OuterHeaderLen+len(f.Payload)), f)
	if err != nil {
		return err
	}
	return h.write(websocket.MessageBinary, b)
}

// close 关掉主机连接与它的全部客户端流（4404）。可以重复调用。
func (h *hostConn) close(code websocket.StatusCode, reason string) {
	h.once.Do(func() {
		h.closed.Store(true)
		close(h.done)
		h.mu.Lock()
		list := make([]*clientConn, 0, len(h.streams))
		for _, c := range h.streams {
			list = append(list, c)
		}
		h.streams = map[uint32]*clientConn{}
		h.mu.Unlock()
		for _, c := range list {
			c.closeBy(remote.CloseHostOffline, "host offline", false)
		}
		go func() { _ = h.ws.Close(code, reason) }()
	})
}

// usageOf 是 hostId 当天的用量（调用方持有 usageMu）；到了新的一天（UTC）整张表清空。
func (s *Server) usageOf(hostID string) *usage {
	if day := s.cfg.Now().UTC().Format("2006-01-02"); day != s.usageDay {
		s.usage, s.usageDay = map[string]*usage{}, day
	}
	u := s.usage[hostID]
	if u == nil {
		u = &usage{}
		s.usage[hostID] = u
	}
	return u
}

// count 记下转发的字节；超过每天的上限时关掉这台主机的全部客户端流，并拒绝新的流，直到 00:00 UTC。
func (h *hostConn) count(n int) bool {
	limit := h.s.cfg.DailyBytesPerHost
	if limit <= 0 {
		return true
	}
	h.s.usageMu.Lock()
	u := h.s.usageOf(h.id)
	u.bytes += int64(n)
	tripped := !u.over && u.bytes > limit
	if tripped {
		u.over = true
	}
	over := u.over
	var list []*clientConn
	if tripped {
		// 用量按 hostId 记：这台主机现在登记的连接（可能刚替换了 h）上的流也一起关（锁的顺序 usageMu → s.mu → h.mu）
		list = h.streamList()
		h.s.mu.Lock()
		cur := h.s.hosts[h.id]
		h.s.mu.Unlock()
		if cur != nil && cur != h {
			list = append(list, cur.streamList()...)
		}
	}
	h.s.usageMu.Unlock()
	for _, c := range list {
		c.close(remote.CloseLimited, "daily quota exceeded")
	}
	return !over
}

func (h *hostConn) streamList() []*clientConn {
	h.mu.Lock()
	defer h.mu.Unlock()
	list := make([]*clientConn, 0, len(h.streams))
	for _, c := range h.streams {
		list = append(list, c)
	}
	return list
}

func (h *hostConn) overQuota() bool {
	if h.s.cfg.DailyBytesPerHost <= 0 {
		return false
	}
	h.s.usageMu.Lock()
	defer h.s.usageMu.Unlock()
	return h.s.usageOf(h.id).over
}

// openStream 为新的客户端分配流编号并通知主机；超出上限时返回关闭码。
// 额度检查与登记在同一段锁里（usageMu → h.mu，和 count 超额时取流列表的顺序一样），超额那一刻之后登记的流一定会被拒绝。
func (h *hostConn) openStream(ws *websocket.Conn) (*clientConn, int, string) {
	c, code, reason := h.register(ws)
	if c == nil {
		return nil, code, reason
	}
	if err := h.writeOuter(remote.OuterFrame{Type: remote.OuterOpen, Stream: c.id}); err != nil {
		h.removeStream(c.id)
		return nil, remote.CloseHostOffline, "host offline"
	}
	return c, 0, ""
}

// register 检查额度与流上限并登记新的流（锁只在这里面持有，通知主机在锁外）。
func (h *hostConn) register(ws *websocket.Conn) (*clientConn, int, string) {
	if h.s.cfg.DailyBytesPerHost > 0 {
		h.s.usageMu.Lock()
		defer h.s.usageMu.Unlock()
		if h.s.usageOf(h.id).over {
			return nil, remote.CloseLimited, "daily quota exceeded"
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed.Load() {
		return nil, remote.CloseHostOffline, "host offline"
	}
	if len(h.streams) >= h.s.cfg.MaxStreamsPerHost {
		return nil, remote.CloseLimited, "too many streams"
	}
	c := newClientConn(h, ws, h.allocStream())
	h.streams[c.id] = c
	return c, 0, ""
}

// allocStream 分配下一个流编号（调用方持有 h.mu）：用到头以后从 1 重来，跳过还开着的。
func (h *hostConn) allocStream() uint32 {
	for {
		h.next++
		if h.next != 0 && h.streams[h.next] == nil {
			return h.next
		}
	}
}

func (h *hostConn) stream(id uint32) *clientConn {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.streams[id]
}

func (h *hostConn) removeStream(id uint32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.streams, id)
}

// readLoop 读主机发来的消息：ping 回 pong，DATA 转给客户端，CLOSE 关掉客户端；别的是违反协议。
func (h *hostConn) readLoop() {
	for {
		ctx, cancel := context.WithTimeout(h.s.ctx, idleTimeout)
		typ, b, err := h.ws.Read(ctx)
		cancel()
		if err != nil {
			return
		}
		if typ == websocket.MessageText {
			if string(b) == "ping" {
				_ = h.write(websocket.MessageText, []byte("pong"))
			}
			continue
		}
		f, err := remote.ParseOuter(b)
		if err != nil {
			h.close(remote.CloseProtocol, "bad frame")
			return
		}
		switch f.Type {
		case remote.OuterData:
			c := h.stream(f.Stream)
			if c == nil {
				continue
			}
			forward, code := c.fromHost(len(f.Payload))
			if code != 0 {
				c.close(code, "wait for ACCEPT")
				continue
			}
			if !forward {
				continue
			}
			if !c.pushData(f.Payload) {
				// 客户端读得太慢，排队满了：关掉它，不拖住别的流
				c.close(remote.CloseLimited, "slow reader")
			}
		case remote.OuterClose:
			if c := h.stream(f.Stream); c != nil {
				code, reason := remote.ParseClosePayload(f.Payload)
				// 排在已经收到的 DATA 后面：先发完再关（例如 GOAWAY 之后马上关）
				c.pushClose(clientCloseCode(code), reason)
			}
		case remote.OuterAccept:
			if c := h.stream(f.Stream); c != nil {
				c.accept()
			}
		default:
			h.close(remote.CloseProtocol, "unexpected frame")
			return
		}
	}
}

// closeStatus 是读出错时对端的关闭码；没有关闭帧时是 1000。
func closeStatus(err error) uint16 {
	if code := websocket.CloseStatus(err); code > 0 {
		return uint16(code)
	}
	return uint16(websocket.StatusNormalClosure)
}

// clientCloseCode 把主机给的关闭码换成能发给客户端的（1000、1001、3000–4999 原样，别的换成 1000）。
func clientCloseCode(code uint16) int {
	switch {
	case code == 1000 || code == 1001:
		return int(code)
	case code >= 3000 && code <= 4999:
		return int(code)
	}
	return 1000
}

// clientConn 是一个客户端流。
type clientConn struct {
	h  *hostConn
	ws *websocket.Conn
	id uint32

	// out 是排队等发给客户端的消息，由 writeLoop 按顺序发出：最多 clientQueue 条 DATA，多一格留给主机的 CLOSE。
	// 只有主机的读循环往里放（pushData、pushClose），所以 queued 与 closing 不用加锁
	out     chan outMsg
	queued  atomic.Int32
	closing bool
	done    chan struct{}
	once    sync.Once
	closed  atomic.Bool

	// 主机发 ACCEPT 以前（accepted 为假），每个方向只能发一条不超过 MaxUnconfirmed 的消息（客户端的握手第一条、
	// 主机拒绝握手的第二条），字节数先记在 pending 里；ACCEPT 时一起计入每天的转发量，没有 ACCEPT 就关掉的流不计
	confMu        sync.Mutex
	accepted      bool
	clientSent    bool
	hostSent      bool
	clientPending int
	hostPending   int
}

func newClientConn(h *hostConn, ws *websocket.Conn, id uint32) *clientConn {
	return &clientConn{h: h, ws: ws, id: id, out: make(chan outMsg, clientQueue+1), done: make(chan struct{})}
}

// outMsg 是要发给客户端的一条消息，或者主机要求的关闭（发完排在前面的消息再关）。
type outMsg struct {
	data   []byte
	close  bool
	code   int
	reason string
}

// fromClient 在转发客户端的一条消息之前调用：ACCEPT 以前只放行一条（不计量），之后计入每天的转发量。
// 不放行时返回关闭码。
func (c *clientConn) fromClient(n int) (bool, int, string) {
	c.confMu.Lock()
	if !c.accepted {
		defer c.confMu.Unlock()
		if c.clientSent || n > remote.MaxUnconfirmed {
			return false, remote.CloseProtocol, "wait for the host"
		}
		c.clientSent, c.clientPending = true, n
		return true, 0, ""
	}
	c.confMu.Unlock()
	if !c.h.count(n) {
		return false, remote.CloseLimited, "daily quota exceeded"
	}
	return true, 0, ""
}

// fromHost 在转发主机的一条 DATA 之前调用：ACCEPT 以前只放行一条（拒绝握手的回话，不计量），之后计量。
// 返回要不要转发，以及违反规则时的关闭码（不为 0 时关掉这个流）。
func (c *clientConn) fromHost(n int) (bool, int) {
	c.confMu.Lock()
	if !c.accepted {
		defer c.confMu.Unlock()
		if c.hostSent || n > remote.MaxUnconfirmed {
			return false, remote.CloseProtocol
		}
		c.hostSent, c.hostPending = true, n
		return true, 0
	}
	c.confMu.Unlock()
	return c.h.count(n), 0
}

// accept 是主机发了 ACCEPT：这个流的握手通过了，之前两个方向的那一条一起计入转发量。重复的忽略。
func (c *clientConn) accept() {
	c.confMu.Lock()
	if c.accepted {
		c.confMu.Unlock()
		return
	}
	c.accepted = true
	n := c.clientPending + c.hostPending
	c.clientPending, c.hostPending = 0, 0
	c.confMu.Unlock()
	if n > 0 {
		c.h.count(n)
	}
}

// pushData 把主机的一条 DATA 排进客户端的队列，不等；排满时返回 false。流已经关了、主机已经要求关时直接丢掉。
func (c *clientConn) pushData(b []byte) bool {
	if c.closed.Load() || c.closing {
		return true
	}
	if c.queued.Load() >= clientQueue {
		return false
	}
	c.queued.Add(1)
	c.out <- outMsg{data: b}
	return true
}

// pushClose 把主机的 CLOSE 排在已经收到的 DATA 后面（用预留的那一格）；只排一次。
func (c *clientConn) pushClose(code int, reason string) {
	if c.closed.Load() || c.closing {
		return
	}
	c.closing = true
	c.out <- outMsg{close: true, code: code, reason: reason}
}

// writeLoop 按顺序把排队的消息发给客户端；写不动（对端一直不读）时断开它。
func (c *clientConn) writeLoop() {
	for {
		select {
		case <-c.done:
			return
		case m := <-c.out:
			if m.close {
				c.closeBy(m.code, m.reason, false)
				return
			}
			c.queued.Add(-1)
			ctx, cancel := context.WithTimeout(c.h.s.ctx, writeTimeout)
			err := c.ws.Write(ctx, websocket.MessageBinary, m.data)
			cancel()
			if err != nil {
				c.close(int(websocket.StatusPolicyViolation), "write timeout")
				return
			}
		}
	}
}

// close 关掉客户端连接，并告诉主机这个流关了。
func (c *clientConn) close(code int, reason string) { c.closeBy(code, reason, true) }

// closeBy 关掉客户端连接；notifyHost 为真且主机还连着时告诉主机（主机自己要求关的、主机已经断开时不用）。可以重复调用。
func (c *clientConn) closeBy(code int, reason string, notifyHost bool) {
	c.once.Do(func() {
		c.closed.Store(true)
		close(c.done)
		c.h.removeStream(c.id)
		if notifyHost && !c.h.closed.Load() {
			_ = c.h.writeOuter(remote.OuterFrame{Type: remote.OuterClose, Stream: c.id, Payload: remote.ClosePayload(uint16(code), reason)})
		}
		go func() { _ = c.ws.Close(websocket.StatusCode(code), reason) }()
	})
}

// readLoop 读客户端的消息：每条二进制消息装进 DATA 转给主机。
func (c *clientConn) readLoop() {
	for {
		ctx, cancel := context.WithTimeout(c.h.s.ctx, idleTimeout)
		typ, b, err := c.ws.Read(ctx)
		cancel()
		if err != nil {
			// 客户端走了：把它的关闭码（没有时 1000）告诉主机
			c.close(clientCloseCode(closeStatus(err)), "")
			return
		}
		if typ != websocket.MessageBinary || len(b) == 0 {
			c.close(remote.CloseProtocol, "binary only")
			return
		}
		if ok, code, reason := c.fromClient(len(b)); !ok {
			c.close(code, reason)
			return
		}
		if err := c.h.writeOuter(remote.OuterFrame{Type: remote.OuterData, Stream: c.id, Payload: b}); err != nil {
			c.close(remote.CloseHostOffline, "host offline")
			return
		}
	}
}

// ipLimiter 按 IP 限制新建连接：每个窗口最多 limit 次；limit 小于 0 时不限。
type ipLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string]*window
	pruned time.Time
	now    func() time.Time
}

type window struct {
	start time.Time
	n     int
}

func newIPLimiter(limit int, w time.Duration, now func() time.Time) *ipLimiter {
	return &ipLimiter{limit: limit, window: w, hits: map[string]*window{}, now: now}
}

func (l *ipLimiter) allow(ip string) bool {
	if l.limit < 0 {
		return true
	}
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	w := l.hits[ip]
	if w != nil && now.Sub(w.start) < l.window {
		if w.n >= l.limit {
			return false
		}
		w.n++
		return true
	}
	if w == nil && len(l.hits) >= limiterMaxIPs/2 && now.Sub(l.pruned) >= l.window/4 {
		for k, v := range l.hits {
			if now.Sub(v.start) >= l.window {
				delete(l.hits, k)
			}
		}
		l.pruned = now
	}
	if w == nil && len(l.hits) >= limiterMaxIPs {
		return false
	}
	l.hits[ip] = &window{start: now, n: 1}
	return true
}
