// MIT License
// Copyright (c) 2025 pt-tools

package chatops

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSessionStore_CleanupLoopEvictsExpired starts a real SessionStore and
// verifies an expired entry is removed by the background cleanup loop.
func TestSessionStore_CleanupLoopEvictsExpired(t *testing.T) {
	s := NewSessionStore()
	t.Cleanup(s.Stop)

	// Set with a tiny TTL, wait for it to lapse, then run cleanup directly to
	// assert eviction without waiting for the long ticker period.
	s.Set("telegram", 1, "u1", SessionState{Step: "x"}, time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	s.cleanupExpired(time.Now())

	_, ok := s.Pending("telegram", 1, "u1")
	assert.False(t, ok, "expired session must be evicted")
}

func TestCleanupExpiredRemovesOnlyStale(t *testing.T) {
	store := NewSessionStore()
	t.Cleanup(store.Stop)

	store.Set("tg", 1, "fresh", SessionState{Step: "a"}, time.Hour)
	store.mu.Lock()
	store.sessions[sessionKey("tg", 1, "stale")] = SessionState{Step: "b", ExpiresAt: time.Now().Add(-time.Hour)}
	store.mu.Unlock()

	store.cleanupExpired(time.Now())

	_, freshOK := store.Pending("tg", 1, "fresh")
	assert.True(t, freshOK)
	store.mu.RLock()
	_, staleExists := store.sessions[sessionKey("tg", 1, "stale")]
	store.mu.RUnlock()
	assert.False(t, staleExists, "expired session must be purged by cleanupExpired")
}

func TestSetDefaultTTLWhenZero(t *testing.T) {
	store := NewSessionStore()
	t.Cleanup(store.Stop)

	store.Set("tg", 1, "u", SessionState{Step: "x"}, 0)
	state, ok := store.Pending("tg", 1, "u")
	require.True(t, ok)
	assert.True(t, state.ExpiresAt.After(time.Now().Add(4*time.Minute)),
		"zero TTL must fall back to defaultSessionTTL (5m)")
}

func TestStopIdempotent(t *testing.T) {
	store := NewSessionStore()
	assert.NotPanics(t, func() {
		store.Stop()
		store.Stop()
	})
}

func TestGenerateBindCodeShape(t *testing.T) {
	code, err := GenerateBindCode()
	require.NoError(t, err)
	assert.Len(t, code, 8)
	for _, c := range code {
		assert.NotContains(t, "0O1lI", string(c), "code must avoid ambiguous chars")
	}

	code2, err := GenerateBindCode()
	require.NoError(t, err)
	assert.NotEqual(t, code, code2, "two draws must differ (probabilistically)")
}

func TestSessionStore_Set_Pending_Clear(t *testing.T) {
	store := NewSessionStore()
	t.Cleanup(store.Stop)

	store.Set("telegram", 1, "user-1", SessionState{Step: "confirm", Data: "torrent-1"}, time.Minute)

	state, ok := store.Pending("telegram", 1, "user-1")
	require.True(t, ok)
	assert.Equal(t, "confirm", state.Step)
	assert.Equal(t, "torrent-1", state.Data)
	assert.True(t, state.ExpiresAt.After(time.Now()))

	store.Clear("telegram", 1, "user-1")
	_, ok = store.Pending("telegram", 1, "user-1")
	assert.False(t, ok)
}

func TestSessionStore_TTL_Expires(t *testing.T) {
	store := NewSessionStore()
	t.Cleanup(store.Stop)

	store.Set("telegram", 1, "user-1", SessionState{Step: "confirm", Data: "torrent-1"}, 100*time.Millisecond)
	time.Sleep(200 * time.Millisecond)

	_, ok := store.Pending("telegram", 1, "user-1")
	assert.False(t, ok)
}

func TestSessionStore_ConcurrentRace(t *testing.T) {
	store := NewSessionStore()
	t.Cleanup(store.Stop)

	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			userID := fmt.Sprintf("user-%d", i)
			for j := range 100 {
				store.Set("telegram", uint(i%4), userID, SessionState{Step: "step", Data: fmt.Sprintf("%d", j)}, time.Minute)
				_, _ = store.Pending("telegram", uint(i%4), userID)
				if j%10 == 0 {
					store.Clear("telegram", uint(i%4), userID)
				}
			}
		}(i)
	}
	wg.Wait()
}
