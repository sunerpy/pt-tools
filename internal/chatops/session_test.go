package chatops

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
