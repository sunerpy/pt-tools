package v2

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/sunerpy/pt-tools/models"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.SiteRateLimit{}))
	return db
}

func TestPersistentRateLimiter_Basic(t *testing.T) {
	db := setupTestDB(t)

	limiter := NewPersistentRateLimiter(PersistentRateLimiterConfig{
		DB:     db,
		SiteID: "test-site",
		Limit:  5,
		Window: time.Second,
	})

	for i := 0; i < 5; i++ {
		assert.True(t, limiter.Allow(), "request %d should be allowed", i+1)
	}

	assert.False(t, limiter.Allow(), "6th request should be blocked")
}

func TestPersistentRateLimiter_WindowReset(t *testing.T) {
	db := setupTestDB(t)

	limiter := NewPersistentRateLimiter(PersistentRateLimiterConfig{
		DB:     db,
		SiteID: "test-site",
		Limit:  3,
		Window: 100 * time.Millisecond,
	})

	for i := 0; i < 3; i++ {
		assert.True(t, limiter.Allow())
	}
	assert.False(t, limiter.Allow())

	time.Sleep(150 * time.Millisecond)

	assert.True(t, limiter.Allow(), "should allow after window reset")
}

func TestPersistentRateLimiter_Persistence(t *testing.T) {
	db := setupTestDB(t)

	limiter1 := NewPersistentRateLimiter(PersistentRateLimiterConfig{
		DB:       db,
		SiteID:   "persist-site",
		Limit:    10,
		Window:   time.Minute,
		SyncRate: 0,
	})

	for i := 0; i < 5; i++ {
		limiter1.Allow()
	}
	limiter1.ForceSync()

	limiter2 := NewPersistentRateLimiter(PersistentRateLimiterConfig{
		DB:     db,
		SiteID: "persist-site",
		Limit:  10,
		Window: time.Minute,
	})

	remaining, _ := limiter2.Stats()
	assert.Equal(t, 5, remaining, "new limiter should restore count from DB")
}

func TestPersistentRateLimiter_Wait(t *testing.T) {
	db := setupTestDB(t)

	limiter := NewPersistentRateLimiter(PersistentRateLimiterConfig{
		DB:     db,
		SiteID: "wait-site",
		Limit:  2,
		Window: 100 * time.Millisecond,
	})

	limiter.Allow()
	limiter.Allow()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := limiter.Wait(ctx)
	elapsed := time.Since(start)

	assert.NoError(t, err)
	assert.True(t, elapsed >= 50*time.Millisecond, "should have waited for window reset")
}

func TestPersistentRateLimiter_WaitContextCanceled(t *testing.T) {
	db := setupTestDB(t)

	limiter := NewPersistentRateLimiter(PersistentRateLimiterConfig{
		DB:     db,
		SiteID: "cancel-site",
		Limit:  1,
		Window: 10 * time.Second,
	})

	limiter.Allow()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := limiter.Wait(ctx)
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestPersistentRateLimiter_Concurrent(t *testing.T) {
	db := setupTestDB(t)

	limiter := NewPersistentRateLimiter(PersistentRateLimiterConfig{
		DB:     db,
		SiteID: "concurrent-site",
		Limit:  100,
		Window: time.Minute,
	})

	var wg sync.WaitGroup
	allowed := make(chan bool, 200)

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed <- limiter.Allow()
		}()
	}

	wg.Wait()
	close(allowed)

	allowedCount := 0
	for a := range allowed {
		if a {
			allowedCount++
		}
	}

	assert.Equal(t, 100, allowedCount, "exactly 100 requests should be allowed")
}

func TestPersistentRateLimiter_NilDB(t *testing.T) {
	limiter := NewPersistentRateLimiter(PersistentRateLimiterConfig{
		DB:     nil,
		SiteID: "nil-db-site",
		Limit:  5,
		Window: time.Second,
	})

	for i := 0; i < 5; i++ {
		assert.True(t, limiter.Allow())
	}
	assert.False(t, limiter.Allow())

	limiter.ForceSync()
	limiter.Reset()
}

func TestPersistentRateLimiter_Stats(t *testing.T) {
	db := setupTestDB(t)

	limiter := NewPersistentRateLimiter(PersistentRateLimiterConfig{
		DB:     db,
		SiteID: "stats-site",
		Limit:  10,
		Window: time.Minute,
	})

	remaining, resetAt := limiter.Stats()
	assert.Equal(t, 10, remaining)
	assert.True(t, resetAt.After(time.Now()))

	limiter.Allow()
	limiter.Allow()
	limiter.Allow()

	remaining, _ = limiter.Stats()
	assert.Equal(t, 7, remaining)
}

func TestNewPersistentRateLimiterFromRPS(t *testing.T) {
	db := setupTestDB(t)

	limiter := NewPersistentRateLimiterFromRPS(db, "rps-site", 2.0, 5)

	assert.NotNil(t, limiter)

	for i := 0; i < 120; i++ {
		limiter.Allow()
	}

	remaining, _ := limiter.Stats()
	assert.Equal(t, 0, remaining)
}

func TestPersistentRateLimiter_Reset(t *testing.T) {
	db := setupTestDB(t)

	limiter := NewPersistentRateLimiter(PersistentRateLimiterConfig{
		DB:     db,
		SiteID: "reset-site",
		Limit:  5,
		Window: time.Minute,
	})

	for i := 0; i < 5; i++ {
		limiter.Allow()
	}
	limiter.ForceSync()

	var count int64
	db.Model(&models.SiteRateLimit{}).Where("site_id = ?", "reset-site").Count(&count)
	assert.Equal(t, int64(1), count)

	limiter.Reset()

	db.Model(&models.SiteRateLimit{}).Where("site_id = ?", "reset-site").Count(&count)
	assert.Equal(t, int64(0), count)

	remaining, _ := limiter.Stats()
	assert.Equal(t, 5, remaining)
}
