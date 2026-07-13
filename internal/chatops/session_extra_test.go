package chatops

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
