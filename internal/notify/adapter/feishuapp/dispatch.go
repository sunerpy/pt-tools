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
	// defaultGather 是队列从空变成有消息以后，工作协程先等多久再取（测试里调小）。
	defaultGather = 200 * time.Millisecond
)

/*
 * 按用户串行：每个 open_id 一条队列，有消息时才起工作协程，空闲一分钟退出；全局 16 个处理槽。
 * 同一个人的消息一条处理完才处理下一条，排队的按发出时间（create_time，毫秒）排，同一毫秒按进入 onMessage 的先后
 * （SDK 先开 goroutine 再调我们，帧在连接上的先后拿不到；同一个人在同一毫秒发出两条，人手发消息时不会出现），
 * 没有 create_time 的按收到的时间。这样「/delete x」和随后的「确认」不会颠倒。
 *
 * 为什么要排：SDK 每收一帧就开一个 goroutine 调 onMessage（ws/client_session.go:175、client_message.go:34），
 * 几乎同时到的几帧谁先进队要看谁先抢到锁。所以队列从空变成有消息时，工作协程先等 gather（200ms）再取，
 * 让这几帧按发出时间排好；处理过程中新到的消息直接按时间插进还没开始的那部分。
 * 保证的是：同一个窗口里到的、排着队还没开始的消息按发出时间处理；已经开始处理的那条不会被后到的消息插队，
 * 比窗口晚到的更早的消息也只能排进还没开始的那部分。QQ 每条消息一个 goroutine，不保证顺序。
 *
 * 热重载（cmd 关掉全部通道再重建）时，排队、还没开始处理的消息由 seal 交出来，经 apps 登记表交给同一个通道
 * 重建以后的实例（见 state.go）；正在处理的那一条随 ctx 取消，与 Telegram、QQ 一致。
 */

// arrivalSeq 是进程级的进队顺序（进入 onMessage 的先后）：同一毫秒发出的消息按它排；交接给新实例以后仍然有序。
var arrivalSeq atomic.Uint64

// queued 是排着队、还没开始处理的一条消息。
type queued struct {
	in notify.InboundMessage
	// sent 是发出时间（create_time）；没有时是收到的时间
	sent time.Time
	seq  uint64
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
	return &queued{in: in, sent: sent, seq: arrivalSeq.Add(1), messageID: messageID}
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
	// readyAt：队列从空变成有消息时记下，工作协程等到这一刻再取
	readyAt time.Time
	// wake 叫醒空闲的工作协程（容量 1）
	wake    chan struct{}
	running bool
}

func newUserQueues() *userQueues {
	return &userQueues{queues: map[string]*userQueue{}, slots: make(chan struct{}, maxHandlers)}
}

// insert 按发出时间把 it 插进还没开始的那部分；队列原来是空的就从现在起等 gather。
func (uq *userQueue) insert(it *queued, gather time.Duration) {
	if len(uq.items) == 0 {
		uq.readyAt = time.Now().Add(gather)
	}
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
	if !uq.running {
		if c.spawn(func() { c.drain(userID, uq) }) {
			uq.running = true
		} else if !adopting {
			// 通道正在关闭：不起工作协程，这条也不收
			if len(uq.items) == 0 {
				delete(q.queues, userID)
			}
			return false
		}
	}
	uq.insert(it, c.timing(c.gather, defaultGather))
	return true
}

// drain 是一个人的工作协程：等到 readyAt 以后按顺序处理，空闲 queueIdle 以后退出；通道关闭时马上退出。
// ctx 取消以后不再取出消息：先拿处理槽，再在锁里看一次 ctx 与 sealed 才取出，取消了就不取，
// 消息留在队首，由 seal 交给下一个实例。Close 也在这把锁里取消 ctx，所以「取出」与「取消」有确定的先后：
// 取消以前取出的那一条就是正在处理的，随 ctx 取消（与 Telegram、QQ 一致）。取出以后到 deliver 之间没有会阻塞的步骤。
// 交接时旧实例正在处理的那条如果不理会取消、继续跑，新实例接手的同一个人的后续消息会和它同时处理。
func (c *Channel) drain(userID string, uq *userQueue) {
	q := c.queues
	idle := c.timing(c.queueIdle, defaultQueueIdle)
	timer := time.NewTimer(idle)
	defer timer.Stop()
	for {
		q.mu.Lock()
		if q.sealed {
			q.mu.Unlock()
			return
		}
		n := len(uq.items)
		wait := time.Until(uq.readyAt)
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
		q.mu.Unlock()
		c.deliver(it.in)
		<-q.slots
	}
}

// seal 交出所有排着队、还没开始处理的消息（按人分好，各自按发出时间排好），之后不再收、不再取。
// 监督循环退出时调用（这时 ctx 已经取消，工作协程不会再取出消息）。
func (q *userQueues) seal() map[string][]*queued {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.sealed = true
	out := map[string][]*queued{}
	for user, uq := range q.queues {
		if len(uq.items) > 0 {
			out[user] = uq.items
			uq.items = nil
		}
	}
	return out
}

// adopt 接手上一个实例留下的消息：照原来的顺序放进队列，不受 8 条的上限限制。
// 名单在这次重建里改过、发消息的人已经不在名单里的，不再执行。
func (c *Channel) adopt(items map[string][]*queued) {
	if len(items) == 0 {
		return
	}
	c.mu.RLock()
	cfg := c.cfg
	c.mu.RUnlock()
	took, denied := 0, 0
	for user, list := range items {
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
