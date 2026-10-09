package remote

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"io"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// 配对：一次只有一个配对窗口（新建会替换旧的）。配对密钥 32 字节、10 分钟内有效、只能用一次，输错 5 次作废。
// 窗口只在内存里，pt-tools 重启以后要重新生成。
const (
	PairingTTL         = 10 * time.Minute
	MaxPairingFailures = 5
)

// PairingState 是配对窗口的状态。
type PairingState string

// 配对窗口的状态。
const (
	PairingNone    PairingState = "none"
	PairingWaiting PairingState = "waiting"
	PairingPaired  PairingState = "paired"
	PairingExpired PairingState = "expired"
	// PairingClosed 是取消了，或者输错太多次作废了
	PairingClosed PairingState = "closed"
)

var (
	// ErrPairingClosed 是配对窗口已经关了（没有、过期、用过、取消或作废）
	ErrPairingClosed = errors.New("配对已经失效，请在网页上重新生成二维码")
	// ErrPairingSecret 是配对密钥不对
	ErrPairingSecret = errors.New("配对密钥不对")
)

// PairingStatus 是配对窗口给网页看的状态。
type PairingStatus struct {
	State     PairingState `json:"state"`
	ExpiresAt *time.Time   `json:"expires_at,omitempty"`
	Scopes    []string     `json:"scopes,omitempty"`
	Failures  int          `json:"failures"`
	// Device 是配对成功的设备
	Device *Device `json:"device,omitempty"`
}

type pairing struct {
	secret   [KeyLen]byte
	scopes   []string
	expires  time.Time
	failures int
	state    PairingState
	device   *Device
}

type pairings struct {
	mu  sync.Mutex
	cur *pairing
	// openUntil 是还在等的窗口的到期时间（UnixNano，0 = 没有窗口）。握手只读它，不等 mu：
	// redeem 会拿着 mu 写设备表，握手不能被一次慢的写库拖住
	openUntil atomic.Int64
	now       func() time.Time
	rng       io.Reader
}

func newPairings(now func() time.Time) *pairings {
	return &pairings{now: now, rng: rand.Reader}
}

// start 开一个新的配对窗口（替换旧的），返回配对密钥与到期时间。
func (p *pairings) start(scopes []string) ([]byte, time.Time, error) {
	var secret [KeyLen]byte
	if _, err := io.ReadFull(p.rng, secret[:]); err != nil {
		return nil, time.Time{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	expires := p.now().Add(PairingTTL)
	p.cur = &pairing{secret: secret, scopes: slices.Clone(scopes), expires: expires, state: PairingWaiting}
	p.openUntil.Store(expires.UnixNano())
	return secret[:], expires, nil
}

// setStateLocked 改当前窗口的状态；离开 waiting 时清掉 openUntil。
func (p *pairings) setStateLocked(st PairingState) {
	p.cur.state = st
	if st != PairingWaiting {
		p.openUntil.Store(0)
	}
}

// waitingLocked 报告当前窗口还能不能配对；到期的顺手标成 expired。
func (p *pairings) waitingLocked() bool {
	c := p.cur
	if c == nil || c.state != PairingWaiting {
		return false
	}
	if !p.now().Before(c.expires) {
		p.setStateLocked(PairingExpired)
		return false
	}
	return true
}

// open 报告现在是不是在配对窗口里（握手时据此决定要不要接受不认识的设备）。不拿锁，见 openUntil。
func (p *pairings) open() bool {
	until := p.openUntil.Load()
	return until != 0 && p.now().UnixNano() < until
}

// fail 记一次失败的配对请求（请求体或设备名不对）：和输错密钥一样计数，到 5 次窗口作废，这时返回 true。
// 这样一个配对窗口里失败的请求（连同审计记录）最多 5 次。
func (p *pairings) fail() (closedNow bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.waitingLocked() {
		return false
	}
	p.cur.failures++
	if p.cur.failures >= MaxPairingFailures {
		p.setStateLocked(PairingClosed)
		return true
	}
	return false
}

// redeem 核对配对密钥（常量时间比较），对了就在同一把锁里用 create 记下设备并关掉窗口。
// 整个过程线性化：取消窗口、换新窗口要么发生在它之前（这次配对失败），要么在它之后（设备已经配对）。
// 输错计数，到 5 次窗口作废，这时 closedNow 为真。create 失败时窗口照旧等着（没到期的话）。
func (p *pairings) redeem(secret []byte, create func(scopes []string) (Device, error)) (dev Device, closedNow bool, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.waitingLocked() {
		return Device{}, false, ErrPairingClosed
	}
	c := p.cur
	if len(secret) != KeyLen || subtle.ConstantTimeCompare(secret, c.secret[:]) != 1 {
		c.failures++
		if c.failures >= MaxPairingFailures {
			p.setStateLocked(PairingClosed)
			return Device{}, true, ErrPairingSecret
		}
		return Device{}, false, ErrPairingSecret
	}
	dev, err = create(slices.Clone(c.scopes))
	if err != nil {
		return Device{}, false, err
	}
	d := dev
	c.device = &d
	p.setStateLocked(PairingPaired)
	return dev, false, nil
}

// cancel 关掉配对窗口。
func (p *pairings) cancel() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c := p.cur; c != nil && c.state == PairingWaiting {
		p.setStateLocked(PairingClosed)
	}
}

// status 是当前窗口的状态。
func (p *pairings) status() PairingStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cur == nil {
		return PairingStatus{State: PairingNone}
	}
	p.waitingLocked()
	c := p.cur
	expires := c.expires
	st := PairingStatus{State: c.state, ExpiresAt: &expires, Scopes: slices.Clone(c.scopes), Failures: c.failures}
	if c.device != nil {
		d := *c.device
		st.Device = &d
	}
	return st
}
