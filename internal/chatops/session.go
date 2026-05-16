package chatops

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	defaultSessionTTL    = 5 * time.Minute
	sessionCleanupPeriod = 10 * time.Second
)

type SessionState struct {
	Step      string
	Data      string
	Handler   CommandHandler
	ExpiresAt time.Time
}

type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]SessionState
	cancel   context.CancelFunc
	done     chan struct{}
	stopOnce sync.Once
}

func NewSessionStore() *SessionStore {
	ctx, cancel := context.WithCancel(context.Background())
	store := &SessionStore{
		sessions: make(map[string]SessionState),
		cancel:   cancel,
		done:     make(chan struct{}),
	}
	go store.cleanupLoop(ctx)
	return store
}

func (s *SessionStore) Pending(channel string, confID uint, userID string) (SessionState, bool) {
	key := sessionKey(channel, confID, userID)
	now := time.Now()

	s.mu.RLock()
	state, ok := s.sessions[key]
	s.mu.RUnlock()
	if !ok {
		return SessionState{}, false
	}
	if !state.ExpiresAt.IsZero() && !state.ExpiresAt.After(now) {
		s.mu.Lock()
		if current, exists := s.sessions[key]; exists && current.ExpiresAt.Equal(state.ExpiresAt) {
			delete(s.sessions, key)
		}
		s.mu.Unlock()
		return SessionState{}, false
	}
	return state, true
}

func (s *SessionStore) Set(channel string, confID uint, userID string, state SessionState, ttl time.Duration) {
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}
	state.ExpiresAt = time.Now().Add(ttl)

	s.mu.Lock()
	s.sessions[sessionKey(channel, confID, userID)] = state
	s.mu.Unlock()
}

func (s *SessionStore) Clear(channel string, confID uint, userID string) {
	s.mu.Lock()
	delete(s.sessions, sessionKey(channel, confID, userID))
	s.mu.Unlock()
}

func (s *SessionStore) Stop() {
	s.stopOnce.Do(func() {
		s.cancel()
		<-s.done
	})
}

func (s *SessionStore) cleanupLoop(ctx context.Context) {
	defer close(s.done)
	ticker := time.NewTicker(sessionCleanupPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.cleanupExpired(now)
		}
	}
}

func (s *SessionStore) cleanupExpired(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, state := range s.sessions {
		if !state.ExpiresAt.IsZero() && !state.ExpiresAt.After(now) {
			delete(s.sessions, key)
		}
	}
}

func sessionKey(channel string, confID uint, userID string) string {
	return fmt.Sprintf("%s:%d:%s", channel, confID, userID)
}
