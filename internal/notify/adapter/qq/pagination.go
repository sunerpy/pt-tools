package qq

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const defaultPageHint = "/next 查看更多"

type sessionKey struct {
	confID string
	userID string
}

type session struct {
	rows      []string
	nextPage  int
	expiresAt time.Time
}

type paginator struct {
	mu       sync.Mutex
	pageSize int
	ttl      time.Duration
	sessions map[sessionKey]*session
	stop     chan struct{}
	stopOnce sync.Once
}

func newPaginator(pageSize int, ttl time.Duration) *paginator {
	if pageSize <= 0 {
		pageSize = 20
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	p := &paginator{
		pageSize: pageSize,
		ttl:      ttl,
		sessions: make(map[sessionKey]*session),
		stop:     make(chan struct{}),
	}
	go p.gcLoop()
	return p
}

func (p *paginator) Stop() {
	p.stopOnce.Do(func() { close(p.stop) })
}

func (p *paginator) gcLoop() {
	t := time.NewTicker(p.ttl)
	defer t.Stop()
	for {
		select {
		case <-p.stop:
			return
		case now := <-t.C:
			p.gc(now)
		}
	}
}

func (p *paginator) gc(now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, s := range p.sessions {
		if now.After(s.expiresAt) {
			delete(p.sessions, k)
		}
	}
}

func (p *paginator) StartOrAdvance(confID, userID string, rows []string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	k := sessionKey{confID: confID, userID: userID}
	s := &session{rows: rows, nextPage: 0, expiresAt: time.Now().Add(p.ttl)}
	p.sessions[k] = s
	return p.renderLocked(k, s)
}

func (p *paginator) AdvanceOnly(confID, userID string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	k := sessionKey{confID: confID, userID: userID}
	s, ok := p.sessions[k]
	if !ok {
		return ""
	}
	return p.renderLocked(k, s)
}

func (p *paginator) renderLocked(k sessionKey, s *session) string {
	start := s.nextPage * p.pageSize
	if start >= len(s.rows) {
		delete(p.sessions, k)
		return ""
	}
	end := start + p.pageSize
	if end > len(s.rows) {
		end = len(s.rows)
	}
	page := s.rows[start:end]
	s.nextPage++
	s.expiresAt = time.Now().Add(p.ttl)

	var b strings.Builder
	for i, row := range page {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(row)
	}
	if end < len(s.rows) {
		fmt.Fprintf(&b, "\n— 第 %d/%d 页，回复 %s",
			s.nextPage, (len(s.rows)+p.pageSize-1)/p.pageSize, defaultPageHint)
	} else {
		delete(p.sessions, k)
	}
	return b.String()
}

func (p *paginator) HasSession(confID, userID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.sessions[sessionKey{confID: confID, userID: userID}]
	return ok
}

func (p *paginator) OnReconnect(confID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k := range p.sessions {
		if k.confID == confID {
			delete(p.sessions, k)
		}
	}
}
