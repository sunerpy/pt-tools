package notify

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	DigestWindow    = 30 * time.Second
	DigestThreshold = 5
	DigestMaxItems  = 50
)

// DigestItem 是 DigestBuffer 内部的一条待合并通知。
// LogID 指向触发该条目的 rss_notification_log 行 ID，flush 回调据此更新结果。
type DigestItem struct {
	LogID uint
	Title string
	Text  string
}

// DigestFlushFunc 在窗口结束（条数达阈值或定时器到期）时被调用，由实现方完成
// 实际推送以及 rss_notification_log 行的结果更新。
type DigestFlushFunc func(ctx context.Context, confID uint, items []DigestItem)

// DigestBuffer 按 NotificationConf ID 缓冲一段时间内的通知，达到 threshold 立即合并、
// 否则在 window 到期时合并刷写。
type DigestBuffer struct {
	mu        sync.Mutex
	pending   map[uint][]DigestItem
	timers    map[uint]*time.Timer
	flush     DigestFlushFunc
	ctx       context.Context
	window    time.Duration
	threshold int
}

func NewDigestBuffer(ctx context.Context, flush DigestFlushFunc) *DigestBuffer {
	return NewDigestBufferWithWindow(ctx, flush, DigestWindow, DigestThreshold)
}

// NewDigestBufferWithWindow 暴露 window/threshold 用于单测。
func NewDigestBufferWithWindow(ctx context.Context, flush DigestFlushFunc, window time.Duration, threshold int) *DigestBuffer {
	if window <= 0 {
		window = DigestWindow
	}
	if threshold <= 0 {
		threshold = DigestThreshold
	}
	return &DigestBuffer{
		pending:   map[uint][]DigestItem{},
		timers:    map[uint]*time.Timer{},
		flush:     flush,
		ctx:       ctx,
		window:    window,
		threshold: threshold,
	}
}

// Add 入列一条 DigestItem。命中阈值立即异步刷写，否则在窗口到期时刷写。
func (b *DigestBuffer) Add(confID uint, item DigestItem) {
	b.mu.Lock()
	b.pending[confID] = append(b.pending[confID], item)
	if len(b.pending[confID]) >= b.threshold {
		items := b.pending[confID]
		delete(b.pending, confID)
		if t, ok := b.timers[confID]; ok {
			t.Stop()
			delete(b.timers, confID)
		}
		b.mu.Unlock()
		go b.flush(b.ctx, confID, items)
		return
	}
	if _, ok := b.timers[confID]; !ok {
		cid := confID
		b.timers[confID] = time.AfterFunc(b.window, func() {
			b.mu.Lock()
			items := b.pending[cid]
			delete(b.pending, cid)
			delete(b.timers, cid)
			b.mu.Unlock()
			if len(items) > 0 {
				b.flush(b.ctx, cid, items)
			}
		})
	}
	b.mu.Unlock()
}

// FlushAll 强制刷写所有 pending 条目，用于优雅退出。
func (b *DigestBuffer) FlushAll() {
	b.mu.Lock()
	snapshot := b.pending
	b.pending = map[uint][]DigestItem{}
	for _, t := range b.timers {
		t.Stop()
	}
	b.timers = map[uint]*time.Timer{}
	b.mu.Unlock()
	for cid, items := range snapshot {
		if len(items) > 0 {
			b.flush(b.ctx, cid, items)
		}
	}
}

// CombineDigest 将多个条目合并为单条消息。单条直接透传；多条生成编号摘要。
// 超过 DigestMaxItems 的部分会被截断并提示剩余数量。
func CombineDigest(items []DigestItem) (title, text string) {
	if len(items) == 0 {
		return "", ""
	}
	if len(items) == 1 {
		return items[0].Title, items[0].Text
	}
	capped := items
	truncated := false
	if len(capped) > DigestMaxItems {
		capped = capped[:DigestMaxItems]
		truncated = true
	}
	var b strings.Builder
	fmt.Fprintf(&b, "📦 %d 条新通知\n\n", len(items))
	for i, it := range capped {
		fmt.Fprintf(&b, "%d. %s\n", i+1, it.Title)
	}
	if truncated {
		fmt.Fprintf(&b, "\n…还有 %d 条已省略", len(items)-DigestMaxItems)
	}
	title = fmt.Sprintf("RSS 摘要 · %d 条", len(items))
	text = b.String()
	return title, text
}
