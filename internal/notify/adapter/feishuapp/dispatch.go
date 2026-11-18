package feishuapp

import (
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sunerpy/pt-tools/internal/notify"
)

const (
	// userQueueSize 是每个人排队（还没开始处理）的消息上限；热重载交接过来的不受它限制。
	userQueueSize = 8
	// maxHandlers 是全局同时处理的消息数（与 QQ 的上限相同）。
	maxHandlers = 16
	// defaultQueueIdle 是工作协程空闲多久退出。
	defaultQueueIdle = time.Minute
	// defaultGather 是每一条进队以后，工作协程至少等多久才取它（测试里调小）。
	defaultGather = 200 * time.Millisecond
	// defaultPreviousWait 是接手以后，一个人的下一条最多等上一个实例里他那条收尾多久（处理函数不理会取消、一直不返回时）
	defaultPreviousWait = 5 * time.Minute
)

/*
 * 按用户串行：每个 open_id 一条队列，有消息时才起工作协程，空闲一分钟退出；全局 16 个处理槽。
 * 同一个人的消息一条处理完才处理下一条，排队的按发出时间（create_time，毫秒）排，同一毫秒按进入排队的先后（取得 arrivalSeq 的先后）
 * （SDK 先开 goroutine 再调我们，帧在连接上的先后拿不到；同一个人在同一毫秒发出两条，人手发消息时不会出现），
 * 没有 create_time 的按收到的时间。这样「/delete x」和随后的「确认」不会颠倒。
 *
 * 为什么要排：SDK 每收一帧就开一个 goroutine 调 onMessage（ws/client_session.go:175、client_message.go:34），
 * 几乎同时到的几帧谁先进队要看谁先抢到锁。所以每一条进队以后，工作协程至少等 gather（200ms）才取它：
 * 比它先发、在这 200ms 里到的帧会按发出时间插到它前面。排在慢消息后面的消息早就等够了，不多等。
 * 保证的是：同一个人相隔 200ms 以内到的消息按发出时间处理；已经开始处理的那条不会被后到的消息插队，
 * 晚到 200ms 以上的更早的消息只能排进还没开始的那部分。QQ 每条消息一个 goroutine，不保证顺序。
 *
 * 热重载（cmd 关掉全部通道再重建）时，排队、还没开始处理的消息由 seal 交出来，经 apps 登记表交给同一个通道
 * 重建以后的实例（见 state.go）；正在处理的那一条随 ctx 取消，与 Telegram、QQ 一致。交出以前先等它收尾
 * （最多 handoffWait，10 秒），同一个人交接过去的下一条不会和它同时处理。
 */

// arrivalSeq 是进程级的进队顺序（取得序号的先后）：同一毫秒发出的消息按它排；交接给新实例以后仍然有序。
var arrivalSeq atomic.Uint64

// queued 是排着队、还没开始处理的一条消息。
type queued struct {
	in notify.InboundMessage
	// sent 是发出时间（create_time）；没有时是收到的时间
	sent time.Time
	seq  uint64
	// arrived 是进队的时刻：工作协程要等它过了 gather 才取这一条（交接过来的早就过了）
	arrived time.Time
	// messageID 只用于日志
	messageID string
}

// before 报告 a 是不是排在 b 前面。
func (a *queued) before(b *queued) bool {
	if !a.sent.Equal(b.sent) {
		return a.sent.Before(b.sent)
	}
	return a.seq < b.seq
}

func newQueued(in notify.InboundMessage, sent time.Time, messageID string) *queued {
	return &queued{in: in, sent: sent, seq: arrivalSeq.Add(1), arrived: time.Now(), messageID: messageID}
}

type userQueues struct {
	mu     sync.Mutex
	queues map[string]*userQueue
	slots  chan struct{}
	// sealed：排队的消息已经交出去了（通道关闭），不再收、不再取
	sealed bool
}

type userQueue struct {
	// items 是还没开始处理的消息，按 before 排好
	items []*queued
	// wake 叫醒空闲的工作协程（容量 1）
	wake    chan struct{}
	running bool
	// busy 在取出一条交给 MessageChain 时建、处理完关闭：交接时带给下一个实例，让这个人的下一条等它
	busy chan struct{}
	// waitFor 是上一个实例里这个人正在处理的那条的完成信号：工作协程取下一条以前先等它（最多 previousWait）
	waitFor <-chan struct{}
}

// handoff 是关闭的实例交给下一个实例的：排着队、还没开始处理的消息，与还有一条在处理的那几个人的完成信号。
type handoff struct {
	items map[string][]*queued
	busy  map[string]<-chan struct{}
}

func (h handoff) empty() bool { return countQueued(h.items) == 0 && len(h.busy) == 0 }

func newUserQueues() *userQueues {
	return &userQueues{queues: map[string]*userQueue{}, slots: make(chan struct{}, maxHandlers)}
}

// insert 按发出时间把 it 插进还没开始的那部分。
func (uq *userQueue) insert(it *queued) {
	i := sort.Search(len(uq.items), func(i int) bool { return it.before(uq.items[i]) })
	uq.items = slices.Insert(uq.items, i, it)
	select {
	case uq.wake <- struct{}{}:
	default:
	}
}

// enqueue 把消息放进这个人的队列。adopting 为 false 时（新收到的消息）排队的满了、通道正在关闭都不收，返回 false；
// 为 true 时（接手上一个实例留下的）不看 8 条的上限，通道正在关闭也照样放进队列，等 seal 再交出去。
func (c *Channel) enqueue(userID string, it *queued, adopting bool) bool {
	q := c.queues
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.sealed || (!adopting && c.ctx.Err() != nil) {
		return false
	}
	uq := q.queues[userID]
	if uq == nil {
		uq = &userQueue{wake: make(chan struct{}, 1)}
		q.queues[userID] = uq
	}
	if !adopting && len(uq.items) >= userQueueSize {
		return false
	}
	if !c.startDrainLocked(userID, uq) && !adopting {
		// 通道正在关闭：不起工作协程，这条也不收
		if len(uq.items) == 0 && uq.waitFor == nil {
			delete(q.queues, userID)
		}
		return false
	}
	uq.insert(it)
	return true
}

// startDrainLocked 在这个人还没有工作协程时起一个（调用方持有 q.mu）；已经在跑或者起来了返回 true，
// 通道正在关闭、起不了返回 false。drains 计数在 q.mu 里加：Close 也在 q.mu 里取消 ctx，取消以后
// enqueue 就直接返回了，所以 supervise 收尾时等 drains 一定在所有 Add 之后。
func (c *Channel) startDrainLocked(userID string, uq *userQueue) bool {
	if uq.running {
		return true
	}
	c.drains.Add(1)
	if !c.spawn(func() {
		defer c.drains.Done()
		c.drain(userID, uq)
	}) {
		c.drains.Done()
		return false
	}
	uq.running = true
	return true
}

// drain 是一个人的工作协程：按顺序处理（每条进队以后至少等 gather），空闲 queueIdle 以后退出；通道关闭时马上退出。
// ctx 取消以后不再取出消息：先拿处理槽，再在锁里看一次 ctx 与 sealed 才取出，取消了就不取，
// 消息留在队首，由 seal 交给下一个实例。Close 也在这把锁里取消 ctx，所以「取出」与「取消」有确定的先后：
// 取消以前取出的那一条就是正在处理的，随 ctx 取消（与 Telegram、QQ 一致）。取出以后到 deliver 之间没有会阻塞的步骤。
// 交接时旧实例正在处理的那条如果不理会取消、继续跑，新实例接手的同一个人的后续消息会和它同时处理。
func (c *Channel) drain(userID string, uq *userQueue) {
	q := c.queues
	idle := c.timing(c.queueIdle, defaultQueueIdle)
	gather := c.timing(c.gather, defaultGather)
	timer := time.NewTimer(idle)
	defer timer.Stop()
	for {
		q.mu.Lock()
		if q.sealed {
			q.mu.Unlock()
			return
		}
		if prev := uq.waitFor; prev != nil {
			q.mu.Unlock()
			if !c.waitPrevious(userID, prev) {
				return
			}
			q.mu.Lock()
			if uq.waitFor == prev {
				uq.waitFor = nil
			}
			q.mu.Unlock()
			continue
		}
		n := len(uq.items)
		var wait time.Duration
		if n > 0 {
			// 每一条进队以后都要等过 gather 才取：比它先发、晚到 gather 以内的帧会插到它前面。
			// 排在慢消息后面的早就等够了，不多等
			wait = time.Until(uq.items[0].arrived.Add(gather))
		}
		q.mu.Unlock()

		if n == 0 {
			timer.Reset(idle)
			select {
			case <-c.ctx.Done():
				return
			case <-uq.wake:
			case <-timer.C:
				q.mu.Lock()
				if len(uq.items) == 0 {
					uq.running = false
					if q.queues[userID] == uq {
						delete(q.queues, userID)
					}
					q.mu.Unlock()
					return
				}
				q.mu.Unlock()
			}
			continue
		}
		if wait > 0 {
			timer.Reset(wait)
			select {
			case <-c.ctx.Done():
				return
			case <-timer.C:
			}
			continue
		}

		select {
		case q.slots <- struct{}{}:
		case <-c.ctx.Done():
			return
		}
		q.mu.Lock()
		if q.sealed || c.ctx.Err() != nil || len(uq.items) == 0 {
			q.mu.Unlock()
			<-q.slots
			if c.ctx.Err() != nil {
				return
			}
			continue
		}
		it := uq.items[0]
		uq.items[0] = nil
		uq.items = uq.items[1:]
		busy := make(chan struct{})
		uq.busy = busy
		q.mu.Unlock()
		if c.afterTake != nil {
			c.afterTake()
		}
		if c.ctx.Err() != nil {
			// 取出以后、交给 MessageChain 以前被取消：放回队首，留给交接。supervise 收尾时先等工作协程退出
			// 再 seal，所以这里在 seal 之前（只有等过了 handoffWait 才可能晚于 seal）
			q.mu.Lock()
			sealed := q.sealed
			if !sealed {
				uq.items = slices.Insert(uq.items, 0, it)
			}
			uq.busy = nil
			close(busy)
			q.mu.Unlock()
			<-q.slots
			if sealed {
				c.logger().Warnf("飞书应用机器人(%d): 关闭时一条消息刚取出、没赶上交接，没有处理 open_id=%s message_id=%s",
					c.confID, it.in.ChannelUserID, it.messageID)
			}
			return
		}
		c.deliver(it.in)
		q.mu.Lock()
		uq.busy = nil
		close(busy)
		q.mu.Unlock()
		<-q.slots
	}
}

// waitPrevious 等上一个实例里这个人正在处理的那条收尾，最多 previousWait；ctx 取消时返回 false。
func (c *Channel) waitPrevious(userID string, prev <-chan struct{}) bool {
	limit := c.timing(c.previousWait, defaultPreviousWait)
	timer := time.NewTimer(limit)
	defer timer.Stop()
	select {
	case <-prev:
		return true
	case <-c.ctx.Done():
		return false
	case <-timer.C:
		c.logger().Warnf("飞书应用机器人(%d): 上一个实例里 open_id=%s 的那条消息 %s内没收尾，接着处理他后面的消息",
			c.confID, userID, durationText(limit))
		return true
	}
}

// seal 交出所有排着队、还没开始处理的消息（按人分好，各自按发出时间排好），以及还有一条在处理（或者自己也在等
// 上一个实例）的那几个人的完成信号，之后不再收、不再取。监督循环退出时调用（这时 ctx 已经取消，工作协程不会再取出消息）。
func (q *userQueues) seal() handoff {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.sealed = true
	out := handoff{items: map[string][]*queued{}, busy: map[string]<-chan struct{}{}}
	for user, uq := range q.queues {
		if len(uq.items) > 0 {
			out.items[user] = uq.items
			uq.items = nil
		}
		switch {
		case uq.busy != nil:
			out.busy[user] = uq.busy
		case uq.waitFor != nil:
			out.busy[user] = uq.waitFor
		}
	}
	return out
}

// adopt 接手上一个实例留下的：还有一条在处理的人先挂上完成信号（他的下一条等那条收尾），
// 再照原来的顺序放进排队的消息，不受 8 条的上限限制。名单在这次重建里改过、发消息的人已经不在名单里的，不再执行。
func (c *Channel) adopt(h handoff) {
	if h.empty() {
		return
	}
	q := c.queues
	q.mu.Lock()
	for user, prev := range h.busy {
		uq := q.queues[user]
		if uq == nil {
			uq = &userQueue{wake: make(chan struct{}, 1)}
			q.queues[user] = uq
		}
		uq.waitFor = prev
		c.startDrainLocked(user, uq)
	}
	q.mu.Unlock()
	if countQueued(h.items) == 0 {
		return
	}
	c.mu.RLock()
	cfg := c.cfg
	c.mu.RUnlock()
	took, denied := 0, 0
	for user, list := range h.items {
		for _, it := range list {
			if !cfg.permitted(it.in.ChannelUserID) {
				denied++
				continue
			}
			if c.enqueue(user, it, true) {
				took++
			}
		}
	}
	log := c.logger()
	log.Infof("飞书应用机器人(%d): 接手上一个实例留下的 %d 条消息，按原来的顺序处理", c.confID, took)
	if denied > 0 {
		log.Infof("飞书应用机器人(%d): 上一个实例留下的 %d 条消息的发送人已经不在名单里，不处理", c.confID, denied)
	}
}

// deliver 把消息交给 MessageChain，用通道自己的生命周期 ctx（Close 时取消，与 Telegram、QQ 一致）。
func (c *Channel) deliver(in notify.InboundMessage) {
	c.mu.RLock()
	handler := c.handler
	c.mu.RUnlock()
	if handler == nil {
		return
	}
	if err := handler(c.ctx, in); err != nil {
		c.logger().Warnf("飞书应用机器人(%d): ChatOps 处理出错 open_id=%s: %v", c.confID, in.ChannelUserID, c.red.wrap(err))
	}
}

// queuedUsers 是此刻有队列的人数（测试用）。
func (c *Channel) queuedUsers() int {
	c.queues.mu.Lock()
	defer c.queues.mu.Unlock()
	return len(c.queues.queues)
}

// countQueued 是交出来的消息一共几条。
func countQueued(items map[string][]*queued) int {
	n := 0
	for _, list := range items {
		n += len(list)
	}
	return n
}
