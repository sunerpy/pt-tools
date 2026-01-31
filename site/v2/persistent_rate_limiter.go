package v2

import (
	"context"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/models"
)

// PersistentRateLimiter 持久化速率限制器
// 使用滑动窗口算法，状态存储在数据库中，重启后可恢复
type PersistentRateLimiter struct {
	db           *gorm.DB
	siteID       string
	limit        int           // 窗口内最大请求数
	window       time.Duration // 窗口时间长度
	mu           sync.Mutex
	memoryState  *rateLimitState // 内存缓存，减少数据库访问
	lastSyncTime time.Time       // 上次同步到数据库的时间
	syncInterval time.Duration   // 同步间隔
}

type rateLimitState struct {
	windowStart  time.Time
	requestCount int
}

type PersistentRateLimiterConfig struct {
	DB       *gorm.DB
	SiteID   string
	Limit    int           // 窗口内最大请求数 (默认: 60)
	Window   time.Duration // 窗口时间 (默认: 1分钟)
	SyncRate time.Duration // 数据库同步间隔 (默认: 5秒)
}

func NewPersistentRateLimiter(cfg PersistentRateLimiterConfig) *PersistentRateLimiter {
	if cfg.Limit <= 0 {
		cfg.Limit = 60
	}
	if cfg.Window <= 0 {
		cfg.Window = time.Minute
	}
	if cfg.SyncRate <= 0 {
		cfg.SyncRate = 5 * time.Second
	}

	limiter := &PersistentRateLimiter{
		db:           cfg.DB,
		siteID:       cfg.SiteID,
		limit:        cfg.Limit,
		window:       cfg.Window,
		syncInterval: cfg.SyncRate,
	}

	limiter.loadFromDB()
	return limiter
}

// NewPersistentRateLimiterFromRPS 从每秒请求数创建限速器
func NewPersistentRateLimiterFromRPS(db *gorm.DB, siteID string, rps float64, burst int) *PersistentRateLimiter {
	if rps <= 0 {
		rps = 1.0
	}
	if burst <= 0 {
		burst = 3
	}

	window := time.Minute
	limit := int(rps * window.Seconds())
	if limit < burst {
		limit = burst
	}

	return NewPersistentRateLimiter(PersistentRateLimiterConfig{
		DB:     db,
		SiteID: siteID,
		Limit:  limit,
		Window: window,
	})
}

func (p *PersistentRateLimiter) loadFromDB() {
	if p.db == nil {
		p.memoryState = &rateLimitState{
			windowStart:  time.Now(),
			requestCount: 0,
		}
		return
	}

	var record models.SiteRateLimit
	err := p.db.Where("site_id = ?", p.siteID).First(&record).Error
	if err != nil {
		p.memoryState = &rateLimitState{
			windowStart:  time.Now(),
			requestCount: 0,
		}
		return
	}

	if time.Since(record.WindowStart) >= p.window {
		p.memoryState = &rateLimitState{
			windowStart:  time.Now(),
			requestCount: 0,
		}
	} else {
		p.memoryState = &rateLimitState{
			windowStart:  record.WindowStart,
			requestCount: record.RequestCount,
		}
	}
}

func (p *PersistentRateLimiter) syncToDB() {
	if p.db == nil || p.memoryState == nil {
		return
	}

	if time.Since(p.lastSyncTime) < p.syncInterval {
		return
	}

	record := models.SiteRateLimit{
		SiteID:       p.siteID,
		WindowStart:  p.memoryState.windowStart,
		RequestCount: p.memoryState.requestCount,
	}

	p.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "site_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"window_start", "request_count", "updated_at"}),
	}).Create(&record)

	p.lastSyncTime = time.Now()
}

// Allow 检查是否允许请求，如果允许则增加计数
func (p *PersistentRateLimiter) Allow() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()

	if p.memoryState == nil || now.Sub(p.memoryState.windowStart) >= p.window {
		p.memoryState = &rateLimitState{
			windowStart:  now,
			requestCount: 0,
		}
	}

	if p.memoryState.requestCount >= p.limit {
		p.syncToDB()
		return false
	}

	p.memoryState.requestCount++
	p.syncToDB()
	return true
}

// Wait 等待直到可以发送请求，返回等待时间
func (p *PersistentRateLimiter) Wait(ctx context.Context) error {
	for {
		if p.Allow() {
			return nil
		}

		waitTime := p.timeUntilNextWindow()
		if waitTime <= 0 {
			waitTime = 100 * time.Millisecond
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			continue
		}
	}
}

func (p *PersistentRateLimiter) timeUntilNextWindow() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.memoryState == nil {
		return 0
	}

	elapsed := time.Since(p.memoryState.windowStart)
	if elapsed >= p.window {
		return 0
	}

	return p.window - elapsed
}

// ForceSync 强制同步到数据库
func (p *PersistentRateLimiter) ForceSync() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.db == nil || p.memoryState == nil {
		return
	}

	record := models.SiteRateLimit{
		SiteID:       p.siteID,
		WindowStart:  p.memoryState.windowStart,
		RequestCount: p.memoryState.requestCount,
	}

	p.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "site_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"window_start", "request_count", "updated_at"}),
	}).Create(&record)

	p.lastSyncTime = time.Now()
}

// Reset 重置限速器状态
func (p *PersistentRateLimiter) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.memoryState = &rateLimitState{
		windowStart:  time.Now(),
		requestCount: 0,
	}

	if p.db != nil {
		p.db.Where("site_id = ?", p.siteID).Delete(&models.SiteRateLimit{})
	}
}

// Stats 返回当前限速器状态
func (p *PersistentRateLimiter) Stats() (remaining int, resetAt time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.memoryState == nil {
		return p.limit, time.Now()
	}

	remaining = p.limit - p.memoryState.requestCount
	if remaining < 0 {
		remaining = 0
	}

	resetAt = p.memoryState.windowStart.Add(p.window)
	return remaining, resetAt
}
