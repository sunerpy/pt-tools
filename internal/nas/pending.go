package nas

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

// PendingTTL 是测试连接产生的新凭据在内存里留多久（等用户点保存）。
const PendingTTL = 10 * time.Minute

// ErrPendingInvalid 是保存时带的测试结果过期了、用过了，或者不是给这组地址与账号的。
var ErrPendingInvalid = errors.New("测试结果过期了，请再测一次")

// PendingKey 是一份暂存凭据绑定的目标：类型、规范化以后的地址、账号，以及来源编号（新建时是 0）。
type PendingKey struct {
	Kind     string
	URL      string
	Username string
	ID       uint
}

// Pending 是测试连接登录产生的新长期凭据（群晖的设备令牌、飞牛的 token 等）的暂存表：回应只给一个随机的 token，
// 不回明文；保存时凭 token 取出、加密写进库里。每份 10 分钟过期、只能用一次、只给绑定的那组目标用。
type Pending struct {
	// Now 是现在的时间（测试里换掉）
	Now func() time.Time

	mu    sync.Mutex
	items map[string]*pendingItem
}

type pendingItem struct {
	key     PendingKey
	extra   map[string]string
	expires time.Time
	claimed bool
}

func (p *Pending) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// Put 暂存一份凭据，返回取它用的 token（随机 32 字节，base64url）。
func (p *Pending) Put(key PendingKey, extra map[string]string) (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b[:])
	cp := make(map[string]string, len(extra))
	for k, v := range extra {
		cp[k] = v
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.items == nil {
		p.items = map[string]*pendingItem{}
	}
	now := p.now()
	for t, it := range p.items {
		if !it.claimed && !now.Before(it.expires) {
			delete(p.items, t)
		}
	}
	p.items[token] = &pendingItem{key: key, extra: cp, expires: now.Add(PendingTTL)}
	return token, nil
}

// Claim 取出 token 暂存的凭据给保存用：过期、正被别的保存用着或绑定不符时返回 ErrPendingInvalid。
// 保存提交以后调 done(true) 删掉它；保存失败时调 done(false) 放回去（还能再用）。
func (p *Pending) Claim(token string, key PendingKey) (extra map[string]string, done func(committed bool), err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	it, ok := p.items[token]
	if !ok || it.claimed || it.key != key || !p.now().Before(it.expires) {
		return nil, nil, ErrPendingInvalid
	}
	it.claimed = true
	cp := make(map[string]string, len(it.extra))
	for k, v := range it.extra {
		cp[k] = v
	}
	var once sync.Once
	return cp, func(committed bool) {
		once.Do(func() {
			p.mu.Lock()
			defer p.mu.Unlock()
			if committed {
				delete(p.items, token)
				return
			}
			it.claimed = false
		})
	}, nil
}

// DropID 丢掉绑定 id 这个来源的暂存项（删除来源时）。
func (p *Pending) DropID(id uint) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for t, it := range p.items {
		if it.key.ID == id {
			delete(p.items, t)
		}
	}
}

// Len 是现在暂存了几份（测试用）。
func (p *Pending) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.items)
}
