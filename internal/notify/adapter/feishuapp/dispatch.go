package feishuapp

import (
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/internal/notify"
)

const (
	// userQueueSize 是每个人排队的消息上限。
	userQueueSize = 8
	// maxHandlers 是全局同时处理的消息数（与 QQ 的上限相同）。
	maxHandlers = 16
	// defaultQueueIdle 是工作协程空闲多久退出。
	defaultQueueIdle = time.Minute
)

/*
 * 按用户串行：每个 open_id 一条先进先出的队列，有消息时才起工作协程，空闲一分钟退出；全局 16 个处理槽。
 * 这样同一个人的「/delete x」和随后的「YES」不会乱序。SDK 每帧开一个 goroutine 调处理函数，
 * QQ 每条消息一个 goroutine，都不保证顺序。
 */

type userQueues struct {
	mu     sync.Mutex
	queues map[string]*userQueue
	slots  chan struct{}
}

type userQueue struct {
	ch      chan notify.InboundMessage
	running bool
}

func newUserQueues() *userQueues {
	return &userQueues{queues: map[string]*userQueue{}, slots: make(chan struct{}, maxHandlers)}
}

// enqueue 把消息放进这个人的队列；队列满了或通道正在关闭时返回 false。
func (c *Channel) enqueue(userID string, in notify.InboundMessage) bool {
	q := c.queues
	q.mu.Lock()
	defer q.mu.Unlock()
	uq := q.queues[userID]
	if uq == nil {
		uq = &userQueue{ch: make(chan notify.InboundMessage, userQueueSize)}
		q.queues[userID] = uq
	}
	select {
	case uq.ch <- in:
	default:
		return false
	}
	if uq.running {
		return true
	}
	if !c.spawn(func() { c.drain(userID, uq) }) {
		// 通道正在关闭：没有工作协程时队列是空的，取回的就是刚放进去的这条
		<-uq.ch
		delete(q.queues, userID)
		return false
	}
	uq.running = true
	return true
}

// drain 是一个人的工作协程：按顺序处理，空闲 queueIdle 以后退出；通道关闭时马上退出。
func (c *Channel) drain(userID string, uq *userQueue) {
	q := c.queues
	idle := c.timing(c.queueIdle, defaultQueueIdle)
	timer := time.NewTimer(idle)
	defer timer.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case in := <-uq.ch:
			select {
			case q.slots <- struct{}{}:
			case <-c.ctx.Done():
				return
			}
			c.deliver(in)
			<-q.slots
			timer.Reset(idle)
		case <-timer.C:
			q.mu.Lock()
			if len(uq.ch) == 0 {
				uq.running = false
				if q.queues[userID] == uq {
					delete(q.queues, userID)
				}
				q.mu.Unlock()
				return
			}
			q.mu.Unlock()
			timer.Reset(idle)
		}
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
