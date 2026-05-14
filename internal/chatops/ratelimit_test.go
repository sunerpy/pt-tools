package chatops

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRateLimiter_PerUser_10PerMin(t *testing.T) {
	rl := NewRateLimiter()

	for i := range 10 {
		assert.True(t, rl.Allow("telegram", "user-1", "status"), "call %d should be allowed", i+1)
	}
	assert.False(t, rl.Allow("telegram", "user-1", "status"))
	assert.True(t, rl.Allow("telegram", "user-2", "status"))
	assert.True(t, rl.Allow("qq", "user-1", "status"))
}

func TestRateLimiter_PerCommand_Override(t *testing.T) {
	cmdName := "ratelimit_override_test"
	RegisterCommand(CommandSpec{
		Name:      cmdName,
		RateLimit: &RateLimitSpec{Per: 10 * time.Second, Burst: 1},
	})
	rl := NewRateLimiter()

	assert.True(t, rl.Allow("telegram", "user-1", cmdName))
	assert.False(t, rl.Allow("telegram", "user-1", cmdName))
	assert.True(t, rl.Allow("telegram", "user-1", "status"))
}

func TestRateLimiter_Reset(t *testing.T) {
	cmdName := "ratelimit_reset_test"
	RegisterCommand(CommandSpec{
		Name:      cmdName,
		RateLimit: &RateLimitSpec{Per: 50 * time.Millisecond, Burst: 1},
	})
	rl := NewRateLimiter()

	assert.True(t, rl.Allow("telegram", "user-1", cmdName))
	assert.False(t, rl.Allow("telegram", "user-1", cmdName))
	time.Sleep(70 * time.Millisecond)
	assert.True(t, rl.Allow("telegram", "user-1", cmdName))
}
