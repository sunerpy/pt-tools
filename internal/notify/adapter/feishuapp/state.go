package feishuapp

import (
	"container/list"
	"sync"
	"time"
)

/*
 * 进程级的状态：不随热重载丢失。改任何一条通知设置都会关掉全部通道实例再全量重建（cmd 的热重载），
 * 这几张表要跨实例有效。
 */

const (
	// dedupeTTL 覆盖平台的四次重推（15 秒、5 分钟、1 小时、6 小时，合计约 7 小时 5 分）。
	dedupeTTL = 8 * time.Hour
	dedupeMax = 10000
	// denyNoticeEvery 是给名单外的同一个人回提示的最短间隔。
	denyNoticeEvery = 10 * time.Minute
	throttleMax     = 1024
)

var (
	// seenMessages 按 app_id + message_id 去重：接收消息文档要求用 message_id 而不是 event_id 去重。
	seenMessages = newDedupe(dedupeTTL, dedupeMax)
	// denyNotices 是名单外提示的节流：同一个 open_id 10 分钟最多一次。
	denyNotices = newThrottle(denyNoticeEvery)
	// apps 是 app_id → 正在用它的通道实例：同一进程里两条配置用同一个应用时，第二条不连长连接。
	apps = newAppRegistry()
)

// resetForTest 清空进程级的状态，测试用。
func resetForTest() {
	seenMessages.reset()
	denyNotices.reset()
	apps.reset()
}

// dedupe 是带过期的去重表，最多 max 条，满了丢最旧的。
type dedupe struct {
	mu    sync.Mutex
	ttl   time.Duration
	max   int
	now   func() time.Time
	at    map[string]time.Time
	order *list.List // dedupeEntry，按记下的先后
}

type dedupeEntry struct {
	key string
	at  time.Time
}

func newDedupe(ttl time.Duration, maxEntries int) *dedupe {
	return &dedupe{ttl: ttl, max: maxEntries, now: time.Now, at: map[string]time.Time{}, order: list.New()}
}

// check 记下 key，返回它之前是否已经在表里（且没过期）。
func (d *dedupe) check(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := d.now()
	d.prune(now)
	if t, ok := d.at[key]; ok && now.Sub(t) < d.ttl {
		return true
	}
	d.at[key] = now
	d.order.PushBack(dedupeEntry{key: key, at: now})
	for len(d.at) > d.max && d.order.Len() > 0 {
		d.drop(d.order.Front())
	}
	return false
}

// forget 把 key 移出表：这条消息没处理（队列满了），要让平台重推时能再处理。
func (d *dedupe) forget(key string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.at, key)
}

func (d *dedupe) prune(now time.Time) {
	for e := d.order.Front(); e != nil; e = d.order.Front() {
		if now.Sub(e.Value.(dedupeEntry).at) < d.ttl {
			return
		}
		d.drop(e)
	}
}

// drop 移掉一个顺序项；表里的同一个 key 只有记下的时间一致时才删（forget 以后又记下的不受影响）。
func (d *dedupe) drop(e *list.Element) {
	ent := e.Value.(dedupeEntry)
	d.order.Remove(e)
	if t, ok := d.at[ent.key]; ok && t.Equal(ent.at) {
		delete(d.at, ent.key)
	}
}

func (d *dedupe) reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.at = map[string]time.Time{}
	d.order.Init()
	d.now = time.Now
}

// throttle 限制同一个 key 在 every 之内只放行一次。
type throttle struct {
	mu    sync.Mutex
	every time.Duration
	now   func() time.Time
	last  map[string]time.Time
}

func newThrottle(every time.Duration) *throttle {
	return &throttle{every: every, now: time.Now, last: map[string]time.Time{}}
}

func (t *throttle) allow(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	if last, ok := t.last[key]; ok && now.Sub(last) < t.every {
		return false
	}
	if len(t.last) >= throttleMax {
		for k, v := range t.last {
			if now.Sub(v) >= t.every {
				delete(t.last, k)
			}
		}
	}
	t.last[key] = now
	return true
}

func (t *throttle) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.last = map[string]time.Time{}
	t.now = time.Now
}

// appRegistry 记下每个 app_id 正被哪个通道实例使用。
type appRegistry struct {
	mu     sync.Mutex
	owners map[string]*Channel
}

func newAppRegistry() *appRegistry { return &appRegistry{owners: map[string]*Channel{}} }

// claim 登记 c 使用 appID；已被另一个实例占着时返回那个实例的通道编号与 false。
func (r *appRegistry) claim(appID string, c *Channel) (uint, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cur := r.owners[appID]; cur != nil && cur != c {
		return cur.confID, false
	}
	r.owners[appID] = c
	return 0, true
}

// release 只释放 c 自己的登记。
func (r *appRegistry) release(appID string, c *Channel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.owners[appID] == c {
		delete(r.owners, appID)
	}
}

func (r *appRegistry) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.owners = map[string]*Channel{}
}
