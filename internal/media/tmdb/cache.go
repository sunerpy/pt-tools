package tmdb

import (
	"sync/atomic"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/models"
)

// Cache 是响应缓存。
type Cache interface {
	Get(key string) ([]byte, bool)
	Set(key string, value []byte, ttl time.Duration)
}

// DBCache 把缓存放在 SQLite 的 media_cache 表里。读写失败时当作没有缓存，不影响请求。
type DBCache struct {
	db   *gorm.DB
	now  func() time.Time
	sets atomic.Int64
}

// NewDBCache 建一个数据库缓存。
func NewDBCache(db *gorm.DB) *DBCache { return &DBCache{db: db, now: time.Now} }

// purgeEvery 是每写多少次清一次过期的缓存。
const purgeEvery = 200

// Get 读一条没过期的缓存。
func (c *DBCache) Get(key string) ([]byte, bool) {
	var row models.MediaCache
	if err := c.db.Where("key = ? AND expires_at > ?", key, c.now()).Limit(1).Find(&row).Error; err != nil || row.Key == "" {
		return nil, false
	}
	return []byte(row.Value), true
}

// Set 写一条缓存；隔一段时间顺便清掉过期的。
func (c *DBCache) Set(key string, value []byte, ttl time.Duration) {
	row := models.MediaCache{Key: key, Value: string(value), ExpiresAt: c.now().Add(ttl)}
	_ = c.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "expires_at"}),
	}).Create(&row).Error
	if c.sets.Add(1)%purgeEvery == 0 {
		_ = c.Purge()
	}
}

// Purge 删除过期的缓存。
func (c *DBCache) Purge() error {
	return c.db.Where("expires_at <= ?", c.now()).Delete(&models.MediaCache{}).Error
}
