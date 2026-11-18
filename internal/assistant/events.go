package assistant

import (
	"context"
	"sync"
	"time"
)

// 事件的种类。
const (
	// EventRunStarted 是一次运行开始
	EventRunStarted = "run_started"
	// EventDelta 是模型回答的一段文字（只在内存里，最终的回答另有 EventMessage）
	EventDelta = "delta"
	// EventReasoning 是模型推理内容（思考过程）的一段，用法同 delta；只在网页的事件流里，App API 不给（契约里没有这一种）
	EventReasoning = "reasoning"
	// EventMessage 是落库的一条消息：用户的、模型的（含工具调用）、工具结果、提示
	EventMessage = "message"
	// EventToolStarted 是开始执行一个工具
	EventToolStarted = "tool_started"
	// EventConfirmation 是待确认操作建好了或者有了结论
	EventConfirmation = "confirmation"
	// EventRunFinished 是运行结束：Status 是 done、canceled 或 failed
	EventRunFinished = "run_finished"
	// EventUsage 是会话的上下文用量变了（每次模型回答以后、压缩以后）：Usage 是新的用量。只在网页的事件流里
	EventUsage = "usage"
	// EventCompacting 是开始压缩上下文（写摘要要一会儿）；压缩完有一条提示消息。只在网页的事件流里
	EventCompacting = "compacting"
)

// 运行结束的状态。
const (
	RunDone     = "done"
	RunCanceled = "canceled"
	RunFailed   = "failed"
)

// Event 是会话事件流里的一个事件。Seq 在这个进程里递增，重启以后的序号一定比重启前的大。
type Event struct {
	Seq   int64  `json:"seq"`
	Type  string `json:"type"`
	RunID string `json:"run_id,omitempty"`
	// Text 是 delta 与 reasoning 的文字；Replace 为真时它是这一步到现在的全部文字（脱敏改了已经发出去的部分），整段替换
	Text         string            `json:"text,omitempty"`
	Replace      bool              `json:"replace,omitempty"`
	Message      *MessageView      `json:"message,omitempty"`
	Tool         *ToolCallView     `json:"tool,omitempty"`
	Confirmation *ConfirmationView `json:"confirmation,omitempty"`
	// Status 是 run_finished 的结束状态
	Status string `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
	// Usage 是 usage 事件的用量
	Usage *UsageView `json:"usage,omitempty"`
	At    time.Time  `json:"at"`
}

// EventPage 是一次读事件的结果。
type EventPage struct {
	Events []Event `json:"events"`
	// Reset 为真表示中间的事件已经丢了（或者 pt-tools 重启过）：重新读一遍会话，从它的 last_seq 接着读
	Reset bool `json:"reset"`
	// Last 是下次读的 after
	Last    int64 `json:"last"`
	Running bool  `json:"running"`
}

// 事件缓冲的上限。
const (
	streamCap  = 1024
	streamIdle = 30 * time.Minute
)

// hub 是各会话的事件流：每个会话在内存里留最近的事件，读的一方按序号续读，没有新事件时等通知。
type hub struct {
	mu      sync.Mutex
	base    int64
	seq     int64
	streams map[uint]*stream
	// gone 是丢掉的缓冲最后的序号：再建缓冲时接着它判断有没有漏
	gone map[uint]int64
}

type stream struct {
	events []Event
	// dropped 是丢掉的事件里最大的序号：after 比它小说明漏了事件
	dropped int64
	wake    chan struct{}
	// partial、reasoning 是进行中的运行这一步到现在的回答文字与推理内容（读会话时给，之后的 delta、reasoning 接着拼）
	partial   string
	reasoning string
	last      time.Time
}

func newHub(now time.Time) *hub {
	base := now.UnixMicro()
	return &hub{base: base, seq: base, streams: map[uint]*stream{}, gone: map[uint]int64{}}
}

func (h *hub) get(conv uint) *stream {
	st := h.streams[conv]
	if st == nil {
		st = &stream{wake: make(chan struct{}), dropped: h.gone[conv]}
		delete(h.gone, conv)
		h.streams[conv] = st
	}
	return st
}

// publish 给事件编号、放进会话的缓冲并叫醒等着的读者。
func (h *hub) publish(conv uint, ev Event) Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	ev.Seq = h.seq
	st := h.get(conv)
	st.events = append(st.events, ev)
	if n := len(st.events) - streamCap; n > 0 {
		st.dropped = st.events[n-1].Seq
		st.events = append(st.events[:0:0], st.events[n:]...)
	}
	switch {
	case ev.Type == EventDelta && ev.Replace:
		st.partial = ev.Text
	case ev.Type == EventDelta:
		st.partial += ev.Text
	case ev.Type == EventReasoning && ev.Replace:
		st.reasoning = ev.Text
	case ev.Type == EventReasoning:
		st.reasoning += ev.Text
	case ev.Type == EventRunFinished, ev.Type == EventMessage && ev.Message != nil && ev.Message.Role != "user":
		st.partial, st.reasoning = "", ""
	}
	st.last = ev.At
	close(st.wake)
	st.wake = make(chan struct{})
	return ev
}

// snapshot 是读会话时的位置：当前的序号与进行中的回答文字、推理内容。先取它再读库，之后从这个序号续读不会漏。
func (h *hub) snapshot(conv uint) (seq int64, partial, reasoning string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if st := h.streams[conv]; st != nil {
		partial, reasoning = st.partial, st.reasoning
	}
	return h.seq, partial, reasoning
}

// read 取 after 以后的事件，同时给一个有新事件时会关上的通道。
func (h *hub) read(conv uint, after int64) (events []Event, reset bool, last int64, wake <-chan struct{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	st := h.get(conv)
	if after != 0 && (after < h.base || after < st.dropped || after > h.seq) {
		return nil, true, h.seq, st.wake
	}
	for _, ev := range st.events {
		if ev.Seq > after {
			events = append(events, ev)
		}
	}
	last = after
	if n := len(events); n > 0 {
		last = events[n-1].Seq
	}
	return events, false, last, st.wake
}

// forget 丢掉会话的缓冲（会话删掉以后）。
func (h *hub) forget(conv uint) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if st := h.streams[conv]; st != nil {
		close(st.wake)
	}
	delete(h.streams, conv)
	delete(h.gone, conv)
}

// sweep 丢掉闲了 streamIdle 以上、没有进行中运行的缓冲。
func (h *hub) sweep(now time.Time, running func(uint) bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for conv, st := range h.streams {
		if now.Sub(st.last) < streamIdle || running(conv) {
			continue
		}
		last := st.dropped
		if n := len(st.events); n > 0 {
			last = st.events[n-1].Seq
		}
		h.gone[conv] = last
		close(st.wake)
		delete(h.streams, conv)
	}
}

// wait 读 after 以后的事件；没有时最多等 wait。keep 不为空时只给它挑中的：挑剩的也算读过了（Last 越过它们），
// 一个也没挑中时接着等，不让读的一方空转。
func (h *hub) wait(ctx context.Context, conv uint, after int64, wait time.Duration, keep func(Event) bool) EventPage {
	var timer <-chan time.Time
	if wait > 0 {
		t := time.NewTimer(wait)
		defer t.Stop()
		timer = t.C
	}
	for {
		events, reset, last, wake := h.read(conv, after)
		if keep != nil && !reset {
			kept := events[:0]
			for _, ev := range events {
				if keep(ev) {
					kept = append(kept, ev)
				}
			}
			events, after = kept, last
		}
		if len(events) > 0 || reset || timer == nil {
			if events == nil {
				events = []Event{}
			}
			return EventPage{Events: events, Reset: reset, Last: last}
		}
		select {
		case <-wake:
		case <-timer:
			return EventPage{Events: []Event{}, Last: last}
		case <-ctx.Done():
			return EventPage{Events: []Event{}, Last: last}
		}
	}
}
