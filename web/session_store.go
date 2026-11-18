package web

import (
	"sync"
	"time"
)

const (
	// sessionIdleTimeout 之内没有任何请求的会话失效，需要重新登录。
	sessionIdleTimeout = 30 * 24 * time.Hour
	// sessionMaxCount 是同时保留的会话上限，超出时淘汰最久没用的那个，避免反复登录让内存无界增长。
	sessionMaxCount = 256
	// sessionTouchInterval 是刷新 lastSeen 的最小间隔，避免每个请求都写一次。
	sessionTouchInterval = time.Minute
)

// sessionStore 保存登录会话（sessionID → 用户名）。
// net/http 并发执行处理器，登录、登出、鉴权会同时读写，所有访问都要经过这里加锁。
type sessionStore struct {
	mu      sync.Mutex
	entries map[string]*sessionEntry
	now     func() time.Time
}

type sessionEntry struct {
	username string
	lastSeen time.Time
}

func newSessionStore() *sessionStore {
	return &sessionStore{entries: map[string]*sessionEntry{}, now: time.Now}
}

// create 为用户新建会话并返回 sessionID，顺带清理过期会话、在超出上限时淘汰最久没用的会话。
func (s *sessionStore) create(username string) string {
	sid := randomID()
	s.put(sid, username)
	return sid
}

// put 以指定 sessionID 写入会话。
func (s *sessionStore) put(sid, username string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.pruneLocked(now)
	for len(s.entries) >= sessionMaxCount {
		s.evictOldestLocked()
	}
	s.entries[sid] = &sessionEntry{username: username, lastSeen: now}
}

// lookup 返回会话对应的用户名；会话不存在或已过期时返回 false，过期会话顺带删除。
func (s *sessionStore) lookup(sid string) (string, bool) {
	if s == nil || sid == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[sid]
	if !ok {
		return "", false
	}
	now := s.now()
	if now.Sub(e.lastSeen) > sessionIdleTimeout {
		delete(s.entries, sid)
		return "", false
	}
	if now.Sub(e.lastSeen) >= sessionTouchInterval {
		e.lastSeen = now
	}
	return e.username, true
}

func (s *sessionStore) valid(sid string) bool {
	_, ok := s.lookup(sid)
	return ok
}

func (s *sessionStore) remove(sid string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, sid)
}

// removeAllExcept 删除除 keep 以外的全部会话（修改密码后让其他设备上的旧会话失效），返回删除数量。
func (s *sessionStore) removeAllExcept(keep string) int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for sid := range s.entries {
		if sid != keep {
			delete(s.entries, sid)
			removed++
		}
	}
	return removed
}

func (s *sessionStore) len() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

func (s *sessionStore) pruneLocked(now time.Time) {
	for sid, e := range s.entries {
		if now.Sub(e.lastSeen) > sessionIdleTimeout {
			delete(s.entries, sid)
		}
	}
}

func (s *sessionStore) evictOldestLocked() {
	var oldestSID string
	var oldest time.Time
	for sid, e := range s.entries {
		if oldestSID == "" || e.lastSeen.Before(oldest) {
			oldestSID, oldest = sid, e.lastSeen
		}
	}
	delete(s.entries, oldestSID)
}
