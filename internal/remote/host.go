package remote

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"go.uber.org/zap"
)

// DefaultRelayURL 是托管 relay 的地址，构建时用 -ldflags 注入；为空时没有托管 relay，要自己填。
var DefaultRelayURL = ""

const (
	// MaxSessions 是同时在线的会话数上限（直连与 relay 合计）
	MaxSessions = 64
	// maxHandshakes 是同时在握手的连接数上限
	maxHandshakes = 32
	// maxPairingSessions 是同时在线的配对会话数上限
	maxPairingSessions = 4
	// closeWait 是关闭主机时等会话收尾的上限
	closeWait = 5 * time.Second
	// notifyTimeout 是发配对通知、写最近在线的上限
	notifyTimeout = 5 * time.Second
	// opTimeout 是网页上一次控制面操作（保存设置、轮换、改设备）的上限
	opTimeout = 15 * time.Second
)

// ErrDisabled 是远程访问没有打开。
var ErrDisabled = errors.New("远程访问没有打开")

// Config 是主机的依赖。
type Config struct {
	Store *Store
	// Dispatcher 处理设备会话里的 App API 请求
	Dispatcher Dispatcher
	// OnPaired 在新设备配对成功后调用（发「新设备已配对」通知）
	OnPaired func(ctx context.Context, d Device)
	// Audit 记下配对的结果；deviceID 为 0 表示还没有设备（密钥不对）
	Audit  func(ctx context.Context, deviceID uint, command, result string)
	Logger *zap.SugaredLogger
	// Version 是 pt-tools 的版本，握手时告诉设备
	Version string
	// HTTPClient 连 relay 用（继承代理设置）；为空时用 http.DefaultClient
	HTTPClient *http.Client
	// IdleTimeout 是会话多久没收到任何帧就断开；0 时用 IdleTimeout（90 秒）
	IdleTimeout time.Duration
	Now         func() time.Time
}

// hostState 是打开远程访问时的状态；关着时 Host.state 为 nil。
type hostState struct {
	keys     *HostKeys
	hostID   string
	settings Settings
}

// Host 是主机端：直连入口、relay 客户端、配对窗口与会话登记。
type Host struct {
	cfg      Config
	ctx      context.Context
	stop     context.CancelFunc
	pairings *pairings
	sessions *registry
	// handshakes 是握手的名额：没握手完的连接最多占 maxHandshakes 个
	handshakes chan struct{}
	limiter    *ipLimiter

	// bg 跟踪后台的写库与通知；bgClosed 之后新来的改成当场执行（Close 在等 bg，不能再 Add）
	bgMu     sync.Mutex
	bg       sync.WaitGroup
	bgClosed bool

	// mu 串行化 Reload、密钥轮换与 Close，并保护 relays、closed
	mu     sync.Mutex
	state  atomic.Pointer[hostState]
	relays map[string]*relayClient
	closed bool
}

// New 建主机；Start 之前远程访问是关着的。
func New(cfg Config) *Host {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop().Sugar()
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = IdleTimeout
	}
	ctx, stop := context.WithCancel(context.Background())
	return &Host{
		cfg: cfg, ctx: ctx, stop: stop,
		pairings: newPairings(cfg.Now), sessions: newRegistry(), handshakes: make(chan struct{}, maxHandshakes),
		limiter: newIPLimiter(HandshakesPerMinute, time.Minute, cfg.Now), relays: map[string]*relayClient{},
	}
}

func (h *Host) logf(format string, args ...any) { h.cfg.Logger.Warnf(format, args...) }

func (h *Host) audit(ctx context.Context, deviceID uint, command, result string) {
	if h.cfg.Audit != nil {
		// 请求可能已经取消：审计照样写，但有上限
		actx, cancel := context.WithTimeout(context.WithoutCancel(ctx), notifyTimeout)
		defer cancel()
		h.cfg.Audit(actx, deviceID, command, result)
	}
}

// Start 按库里的设置启动（打开着就生成或读出密钥、连上 relay）。
func (h *Host) Start(ctx context.Context) error { return h.Reload(ctx) }

// Reload 按库里的设置重新配置：关掉时断开全部会话与 relay；主机密钥换了时断开全部会话；relay 按列表增减。
func (h *Host) Reload(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.reloadLocked(ctx)
}

func (h *Host) reloadLocked(ctx context.Context) error {
	if h.closed {
		return nil
	}
	set, err := h.cfg.Store.Settings(ctx)
	if err != nil {
		return err
	}
	var keys *HostKeys
	if set.Enabled {
		if keys, err = h.cfg.Store.HostKeys(ctx, true); err != nil {
			return err
		}
	}
	h.applyLocked(set, keys)
	return nil
}

// applyLocked 让运行时与给定的设置、密钥一致，不再读库（写库已经成功时，运行时的失效不能因为读不到而跳过）。
// 关掉：先给全部会话发 GOAWAY disabled（经 relay 的会话要靠 relay 连接送到），再断开 relay。
// 主机密钥换了：全部会话发 GOAWAY key_rotated，relay 用新的 hostId 重连。
func (h *Host) applyLocked(set Settings, keys *HostKeys) {
	if h.closed {
		return
	}
	if !set.Enabled || keys == nil {
		if h.state.Swap(nil) != nil {
			// 先关会话（换状态与 takeAll 之间不夹别的等待），再关配对窗口（可能要等一次写设备表），最后断开 relay
			shutdownAll(h.sessions.takeAll(), GoAwayDisabled)
			h.pairings.cancel()
			h.stopRelaysLocked(nil)
		}
		return
	}
	st := &hostState{keys: keys, hostID: keys.HostID(), settings: set}
	old := h.state.Swap(st)
	if old != nil && old.hostID != st.hostID {
		shutdownAll(h.sessions.takeAll(), GoAwayKeyRotated)
		h.pairings.cancel()
	}
	h.syncRelaysLocked(st)
}

// boundedContext 是控制面操作用的 context：不随请求取消（写库以后的运行时失效必须做完），但有上限。
func boundedContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), opTimeout)
}

// Close 关掉主机：断开 relay 与全部会话（发 GOAWAY shutdown），最多等 5 秒。
func (h *Host) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	h.state.Store(nil)
	h.mu.Unlock()
	// 先发 GOAWAY shutdown（经 relay 的会话要靠 relay 连接送到），再断开 relay
	list := h.sessions.takeAll()
	shutdownAll(list, GoAwayShutdown)
	h.mu.Lock()
	h.stopRelaysLocked(nil)
	h.mu.Unlock()
	h.stop()
	deadline := time.NewTimer(closeWait)
	defer deadline.Stop()
	for _, s := range list {
		select {
		case <-s.done:
		case <-deadline.C:
			return
		}
	}
	h.bgMu.Lock()
	h.bgClosed = true
	h.bgMu.Unlock()
	done := make(chan struct{})
	go func() { h.bg.Wait(); close(done) }()
	select {
	case <-done:
	case <-deadline.C:
	}
}

// Enabled 报告远程访问现在是不是打开着。
func (h *Host) Enabled() bool { return h.state.Load() != nil }

// ServeHTTP 是直连入口 /remote/v1/stream：远程访问关着时 404，每个 IP 每分钟最多 30 次握手。
// IP 是连接的对端地址，不读 X-Forwarded-For（没有反向代理时可以随意伪造）；部署在反向代理之后时所有设备合用代理的额度。
func (h *Host) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.state.Load() == nil {
		http.NotFound(w, r)
		return
	}
	if !h.limiter.allow(remoteIP(r)) {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "握手太频繁，请稍后再试", http.StatusTooManyRequests)
		return
	}
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	h.serveConn(newWSConn(c), ViaDirect)
}

// serveConn 在一条连接上握手并服务会话，返回时连接已经关闭。
func (h *Host) serveConn(raw MsgConn, via string) {
	s, reason := h.handshake(raw, via)
	if s == nil {
		code := 1000
		if reason == HelloBusy {
			code = CloseLimited
		}
		raw.Close(code, reason)
		return
	}
	if !h.sessions.add(s) {
		s.shutdown(h.staleReason())
		close(s.done)
		return
	}
	// 撤销、改权限、关配对窗口、关掉远程访问可能发生在握手与登记之间（那时还关不到这条会话），登记以后再核对一次
	if reason := h.stillValid(s); reason != "" {
		s.shutdown(reason)
		close(s.done)
		return
	}
	if s.mode == ModeDevice {
		h.touch(s.device.ID, via)
	}
	s.serve()
}

// handshake 占一个握手名额完成握手，返回建好（还没登记）的会话；握手没完成时返回 nil 与关闭原因。
func (h *Host) handshake(raw MsgConn, via string) (*session, string) {
	select {
	case h.handshakes <- struct{}{}:
	default:
		return nil, HelloBusy
	}
	defer func() { <-h.handshakes }()
	// 先读 epoch 再读状态：关掉与轮换都是先换状态、再让 epoch 加一，这样读到旧状态的握手一定带着旧 epoch，登记时被拒
	epoch := h.sessions.currentEpoch()
	st := h.state.Load()
	if st == nil {
		return nil, GoAwayDisabled
	}
	var dev *Device
	reserved := ""
	conn, peer, hello, err := acceptHandshake(h.ctx, raw, st.keys, st.hostID, nil, func(peer []byte, _ ClientHello) HostHello {
		lctx, cancel := context.WithTimeout(h.ctx, HandshakeTimeout)
		d, err := h.cfg.Store.ActiveDeviceByKey(lctx, peer)
		cancel()
		mode := ModeDevice
		switch {
		case err != nil:
			h.logf("[远程访问] 查设备失败: %v", err)
			return HostHello{Error: HelloBusy}
		case d != nil:
			dev = d
		case !h.pairings.open():
			return HostHello{Error: HelloNotPaired}
		default:
			mode = ModePairing
		}
		// 名额在回第二条消息之前占好：总会话数与配对会话数都不会被并发的握手超过
		if !h.sessions.reserve(mode) {
			return HostHello{Error: HelloBusy}
		}
		reserved = mode
		return HostHello{Mode: mode, Host: h.cfg.Version}
	})
	if err != nil {
		if reserved != "" {
			h.sessions.release(reserved)
		}
		h.cfg.Logger.Debugf("[远程访问] 握手没有完成（%s）: %v", via, err)
		return nil, ""
	}
	s := newSession(h, conn, raw, via, hello.Mode, peer, epoch)
	if dev != nil {
		s.device = *dev
	}
	return s, ""
}

// staleReason 是握手以后主机关过全部会话时给设备的原因。
func (h *Host) staleReason() string {
	if h.state.Load() == nil {
		return GoAwayDisabled
	}
	return GoAwayKeyRotated
}

// stillValid 核对登记好的会话还能不能用，不能用时返回 GOAWAY 的原因。
func (h *Host) stillValid(s *session) string {
	if h.state.Load() == nil {
		return GoAwayDisabled
	}
	if s.mode == ModePairing {
		if !h.pairings.open() {
			return GoAwayPairingClosed
		}
		return ""
	}
	lctx, cancel := context.WithTimeout(h.ctx, HandshakeTimeout)
	defer cancel()
	d, err := h.cfg.Store.ActiveDeviceByKey(lctx, s.peerKey)
	switch {
	case err != nil:
		return GoAwayShutdown
	case d == nil || d.ID != s.device.ID:
		return GoAwayRevoked
	case !sameScopes(d.Scopes, s.device.Scopes):
		return GoAwayScopeChanged
	}
	return ""
}

func sameScopes(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// background 在后台执行 fn；主机在关闭时当场执行。
func (h *Host) background(fn func()) {
	h.bgMu.Lock()
	if h.bgClosed {
		h.bgMu.Unlock()
		fn()
		return
	}
	h.bg.Go(fn)
	h.bgMu.Unlock()
}

// touch 在后台记下设备最近一次在线（会话开始与结束时）。
func (h *Host) touch(id uint, via string) {
	h.background(func() {
		ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
		defer cancel()
		if err := h.cfg.Store.TouchDevice(ctx, id, via, h.cfg.Now()); err != nil {
			h.logf("[远程访问] 记最近在线失败: %v", err)
		}
	})
}

// paired 是配对完成：关掉别的配对会话（窗口已经用掉），发通知。
func (h *Host) paired(d Device, keep *session) {
	h.closePairingSessions(keep)
	if h.cfg.OnPaired != nil {
		h.background(func() {
			ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
			defer cancel()
			h.cfg.OnPaired(ctx, d)
		})
	}
}

// closePairingSessions 关掉 keep 以外的配对会话（窗口用掉、作废、取消或换了新的）。
func (h *Host) closePairingSessions(keep *session) {
	shutdownAll(h.sessions.matching(func(s *session) bool { return s != keep && s.mode == ModePairing }), GoAwayPairingClosed)
}

// closeDevice 关掉这台设备的全部会话。
func (h *Host) closeDevice(id uint, reason string) {
	shutdownAll(h.sessions.matching(func(s *session) bool { return s.mode == ModeDevice && s.device.ID == id }), reason)
}

// RelayStatus 是一个 relay 的连接状态。
type RelayStatus struct {
	URL string `json:"url"`
	// State 是 connecting（正在连）、online（已连上）或 offline（断开，等着重连）
	State   string    `json:"state"`
	Error   string    `json:"error,omitempty"`
	Since   time.Time `json:"since"`
	Streams int       `json:"streams"`
}

// Overview 是给网页看的设置与状态。
type Overview struct {
	Settings
	// HostID 与 HostKey（X25519 公钥）在第一次打开远程访问以后才有
	HostID       string        `json:"host_id,omitempty"`
	HostKey      string        `json:"host_key,omitempty"`
	DefaultRelay string        `json:"default_relay,omitempty"`
	RelayStatus  []RelayStatus `json:"relay_status"`
	Sessions     int           `json:"sessions"`
	StreamPath   string        `json:"stream_path"`
}

// Overview 读设置与状态。
func (h *Host) Overview(ctx context.Context) (Overview, error) {
	set, err := h.cfg.Store.Settings(ctx)
	if err != nil {
		return Overview{}, err
	}
	out := Overview{Settings: set, RelayStatus: []RelayStatus{}, Sessions: h.sessions.count(), StreamPath: StreamPath}
	if u, nerr := NormalizeRelayURL(DefaultRelayURL); nerr == nil {
		out.DefaultRelay = u
	}
	keys, err := h.cfg.Store.HostKeys(ctx, false)
	if err != nil {
		return Overview{}, err
	}
	if keys != nil {
		out.HostID, out.HostKey = keys.HostID(), EncodeKey(keys.Noise.Public)
	}
	h.mu.Lock()
	for _, u := range set.Relays {
		if c := h.relays[u]; c != nil {
			out.RelayStatus = append(out.RelayStatus, c.statusSnapshot())
		}
	}
	h.mu.Unlock()
	return out, nil
}

// UpdateSettings 保存设置并重新配置。打开时先确认主机密钥生成得了，免得设置写成打开、主机却起不来。
func (h *Host) UpdateSettings(ctx context.Context, in Settings) (Overview, error) {
	in, err := NormalizeSettings(in)
	if err != nil {
		return Overview{}, err
	}
	ctx, cancel := boundedContext(ctx)
	defer cancel()
	h.mu.Lock()
	var keys *HostKeys
	if in.Enabled {
		if keys, err = h.cfg.Store.HostKeys(ctx, true); err != nil {
			h.mu.Unlock()
			return Overview{}, err
		}
	}
	saved, err := h.cfg.Store.SaveSettings(ctx, in)
	if err != nil {
		// 写没写进去不确定：关掉时宁可先让运行时也关掉
		if !in.Enabled {
			h.applyLocked(in, nil)
		}
		h.mu.Unlock()
		return Overview{}, err
	}
	h.applyLocked(saved, keys)
	h.mu.Unlock()
	return h.Overview(ctx)
}

// RotateKeys 换一套主机密钥：撤销所有设备，断开全部会话，relay 用新的 hostId 重连。
func (h *Host) RotateKeys(ctx context.Context) (Overview, error) {
	ctx, cancel := boundedContext(ctx)
	defer cancel()
	h.mu.Lock()
	// 先关配对窗口：正在写设备表的配对做完（它的设备随后被这次轮换撤销），之后不会再有设备按旧的主机密钥配对进来
	h.pairings.cancel()
	keys, err := h.cfg.Store.RotateHostKeys(ctx)
	if err != nil {
		h.mu.Unlock()
		return Overview{}, err
	}
	// 新密钥已经提交：不再读库，直接用内存里的设置换掉运行时的密钥（关着远程访问时没有会话，什么也不用做）
	if st := h.state.Load(); st != nil {
		h.applyLocked(st.settings, keys)
	}
	h.mu.Unlock()
	return h.Overview(ctx)
}

// PairingTicket 是新开的配对窗口：链接只在这里给一次。
type PairingTicket struct {
	Link      string    `json:"link"`
	ExpiresAt time.Time `json:"expires_at"`
	Scopes    []string  `json:"scopes"`
	HostID    string    `json:"host_id"`
}

// StartPairing 开一个配对窗口（替换旧的）。direct 不为空时用它做链接里的直连地址，否则用设置里的。
func (h *Host) StartPairing(_ context.Context, scopes []string, direct string) (PairingTicket, error) {
	st := h.state.Load()
	if st == nil {
		return PairingTicket{}, ErrDisabled
	}
	scopes, err := NormalizeScopes(scopes)
	if err != nil {
		return PairingTicket{}, err
	}
	d := st.settings.DirectURL
	if direct != "" {
		if d, err = NormalizeDirectURL(direct); err != nil {
			return PairingTicket{}, fmt.Errorf("%w：%v", ErrInvalid, err)
		}
	}
	if d == "" && len(st.settings.Relays) == 0 {
		return PairingTicket{}, fmt.Errorf("%w：先填直连地址或者 relay，App 才知道怎么连过来", ErrInvalid)
	}
	secret, expires, err := h.pairings.start(scopes)
	if err != nil {
		return PairingTicket{}, err
	}
	// 旧窗口里连上来的配对会话跟着作废
	h.closePairingSessions(nil)
	link := PairingLink{HostID: st.hostID, HostKey: st.keys.Noise.Public, Secret: secret, Relays: st.settings.Relays, Direct: d}
	return PairingTicket{Link: link.String(), ExpiresAt: expires, Scopes: scopes, HostID: st.hostID}, nil
}

// PairingStatus 是配对窗口的状态。
func (h *Host) PairingStatus() PairingStatus { return h.pairings.status() }

// CancelPairing 关掉配对窗口与窗口里的配对会话。
func (h *Host) CancelPairing() {
	h.pairings.cancel()
	h.closePairingSessions(nil)
}

// DeviceStatus 是设备列表里的一项：设备加上现在在不在线。
type DeviceStatus struct {
	Device
	Online    bool   `json:"online"`
	OnlineVia string `json:"online_via,omitempty"`
}

// Devices 列出设备与在线状态。
func (h *Host) Devices(ctx context.Context) ([]DeviceStatus, error) {
	list, err := h.cfg.Store.Devices(ctx)
	if err != nil {
		return nil, err
	}
	online := h.sessions.online()
	out := make([]DeviceStatus, 0, len(list))
	for _, d := range list {
		via, ok := online[d.ID]
		out = append(out, DeviceStatus{Device: d, Online: ok, OnlineVia: via})
	}
	return out, nil
}

// UpdateDevice 改设备的名字或权限；权限变了时立即断开这台设备的会话，重连以后按新权限。
func (h *Host) UpdateDevice(ctx context.Context, id uint, name *string, scopes []string) (Device, error) {
	ctx, cancel := boundedContext(ctx)
	defer cancel()
	d, changed, err := h.cfg.Store.UpdateDevice(ctx, id, name, scopes)
	if err != nil {
		// 参数不对、没有这台、已经撤销都发生在写库之前；别的错误时权限可能已经改了，宁可断开让它重连
		if scopes != nil && !errors.Is(err, ErrInvalid) && !errors.Is(err, ErrDeviceNotFound) && !errors.Is(err, ErrDeviceRevoked) {
			h.closeDevice(id, GoAwayScopeChanged)
		}
		return Device{}, err
	}
	if changed {
		h.closeDevice(id, GoAwayScopeChanged)
	}
	return d, nil
}

// RevokeDevice 撤销设备并立即断开它的会话；之后它的握手会被拒绝。
func (h *Host) RevokeDevice(ctx context.Context, id uint) (Device, error) {
	ctx, cancel := boundedContext(ctx)
	defer cancel()
	d, err := h.cfg.Store.RevokeDevice(ctx, id)
	if !errors.Is(err, ErrDeviceNotFound) {
		// 写库以后读回失败时撤销可能已经生效：照样断开
		h.closeDevice(id, GoAwayRevoked)
	}
	if err != nil {
		return Device{}, err
	}
	return d, nil
}

// DeleteDevice 删掉撤销了的设备的记录。
func (h *Host) DeleteDevice(ctx context.Context, id uint) error {
	ctx, cancel := boundedContext(ctx)
	defer cancel()
	return h.cfg.Store.DeleteDevice(ctx, id)
}
