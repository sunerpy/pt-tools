package chatops

import (
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultRateLimitPer   = time.Minute
	defaultRateLimitBurst = 10
)

type RateLimitSpec struct {
	Per   time.Duration
	Burst int
}

type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
}

type tokenBucket struct {
	limiter            *rate.Limiter
	consecutiveDenials int
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{buckets: make(map[string]*tokenBucket)}
}

func (rl *RateLimiter) Allow(channel, userID, command string) bool {
	key, spec := rateLimitKey(channel, userID, command)

	rl.mu.Lock()
	bucket, ok := rl.buckets[key]
	if !ok {
		bucket = &tokenBucket{limiter: newTokenLimiter(spec)}
		rl.buckets[key] = bucket
	}
	rl.mu.Unlock()

	allowed := bucket.limiter.Allow()

	rl.mu.Lock()
	if allowed {
		bucket.consecutiveDenials = 0
	} else {
		bucket.consecutiveDenials++
	}
	rl.mu.Unlock()
	return allowed
}

func rateLimitKey(channel, userID, command string) (string, RateLimitSpec) {
	commandName := normalizeCommandName(command)
	if spec, ok := DefaultRegistry().Get(commandName); ok && spec.RateLimit != nil {
		return fmt.Sprintf("cmd:%s:%s:%s", channel, userID, spec.Name), *spec.RateLimit
	}
	return fmt.Sprintf("user:%s:%s", channel, userID), RateLimitSpec{Per: defaultRateLimitPer, Burst: defaultRateLimitBurst}
}

func newTokenLimiter(spec RateLimitSpec) *rate.Limiter {
	if spec.Per <= 0 {
		spec.Per = defaultRateLimitPer
	}
	if spec.Burst <= 0 {
		spec.Burst = defaultRateLimitBurst
	}
	return rate.NewLimiter(rate.Limit(float64(spec.Burst)/spec.Per.Seconds()), spec.Burst)
}
