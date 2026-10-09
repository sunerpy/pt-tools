package remote

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// registry 记着所有活着的会话：按设备关会话、数在线设备、关闭时一起关；会话名额也在这里占。
type registry struct {
	mu  sync.Mutex
	all map[*session]struct{}
	// epoch 每次 takeAll 加一：在那之前开始握手、之后才来登记的会话不收（握手用的是旧密钥或旧设置）
	epoch uint64
	// slots、pairingSlots 是占着的名额（握手成功、还没关掉的会话），握手回第二条消息之前占
	slots, pairingSlots int
}

// reserve 为一条会话占名额：总数不超过 MaxSessions，配对会话不超过 maxPairingSessions。
func (r *registry) reserve(mode string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.slots >= MaxSessions || (mode == ModePairing && r.pairingSlots >= maxPairingSessions) {
		return false
	}
	r.slots++
	if mode == ModePairing {
		r.pairingSlots++
	}
	return true
}

// release 还回 reserve 占的名额。
func (r *registry) release(mode string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.slots--
	if mode == ModePairing {
		r.pairingSlots--
	}
}

func newRegistry() *registry { return &registry{all: map[*session]struct{}{}} }

func (r *registry) currentEpoch() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.epoch
}

// add 登记会话；握手以后主机关过一次全部会话时返回 false。
func (r *registry) add(s *session) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.epoch != r.epoch {
		return false
	}
	r.all[s] = struct{}{}
	return true
}

func (r *registry) remove(s *session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.all, s)
}

// matching 是满足 fn 的会话（快照，调用方在锁外关它们）。
func (r *registry) matching(fn func(*session) bool) []*session {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*session
	for s := range r.all {
		if fn(s) {
			out = append(out, s)
		}
	}
	return out
}

// takeAll 让之前的握手全部作废，并返回现有的会话（调用方关掉它们）。
func (r *registry) takeAll() []*session {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.epoch++
	out := make([]*session, 0, len(r.all))
	for s := range r.all {
		out = append(out, s)
	}
	return out
}

func (r *registry) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.all)
}

// online 是现在在线的设备与连接方式（同一台设备有几条会话时取任意一条的方式）。
func (r *registry) online() map[uint]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[uint]string{}
	for s := range r.all {
		if s.mode == ModeDevice {
			out[s.device.ID] = s.via
		}
	}
	return out
}

// shutdownAll 并行关掉会话（各自最多等 2 秒发 GOAWAY）。
func shutdownAll(list []*session, reason string) {
	var wg sync.WaitGroup
	for _, s := range list {
		wg.Go(func() { s.shutdown(reason) })
	}
	wg.Wait()
}

// ipLimiter 按 IP 限制直连的握手频率：每个窗口最多 limit 次。
type ipLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string]*ipWindow
	pruned time.Time
	now    func() time.Time
}

type ipWindow struct {
	start time.Time
	n     int
}

const (
	// HandshakesPerMinute 是每个 IP 每分钟最多的直连握手次数
	HandshakesPerMinute = 30
	// limiterMaxIPs 是限流表最多记多少个 IP；满了以后新来的 IP 先拒绝，等旧的过期
	limiterMaxIPs = 16384
)

func newIPLimiter(limit int, window time.Duration, now func() time.Time) *ipLimiter {
	return &ipLimiter{limit: limit, window: window, hits: map[string]*ipWindow{}, now: now}
}

func (l *ipLimiter) allow(ip string) bool {
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
	// 表大了才清过期的，而且最多每四分之一个窗口清一次，免得每来一个新 IP 都扫一遍
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
	l.hits[ip] = &ipWindow{start: now, n: 1}
	return true
}

// remoteIP 是请求的对端 IP（没有配置 IPOf 时用）。
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
