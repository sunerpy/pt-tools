// Package v2 provides caching utilities for performance optimization
package v2

import (
	"container/list"
	"sync"
	"time"
)

// CacheEntry represents a cached item with TTL
type CacheEntry struct {
	Key       string
	Value     any
	ExpiresAt time.Time
}

// IsExpired checks if the cache entry has expired
func (e *CacheEntry) IsExpired() bool {
	return time.Now().After(e.ExpiresAt)
}

// LRUCache is a thread-safe LRU cache with TTL support
type LRUCache struct {
	mu       sync.RWMutex
	capacity int
	ttl      time.Duration
	items    map[string]*list.Element
	order    *list.List
}

// NewLRUCache creates a new LRU cache with the specified capacity and TTL
func NewLRUCache(capacity int, ttl time.Duration) *LRUCache {
	return &LRUCache{
		capacity: capacity,
		ttl:      ttl,
		items:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

// Get retrieves a value from the cache
func (c *LRUCache) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		return nil, false
	}

	entry := elem.Value.(*CacheEntry)
	if entry.IsExpired() {
		c.removeElement(elem)
		return nil, false
	}

	// Move to front (most recently used)
	c.order.MoveToFront(elem)
	return entry.Value, true
}

// Set adds or updates a value in the cache
func (c *LRUCache) Set(key string, value any) {
	c.SetWithTTL(key, value, c.ttl)
}

// SetWithTTL adds or updates a value with a custom TTL
func (c *LRUCache) SetWithTTL(key string, value any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update existing entry
	if elem, ok := c.items[key]; ok {
		entry := elem.Value.(*CacheEntry)
		entry.Value = value
		entry.ExpiresAt = time.Now().Add(ttl)
		c.order.MoveToFront(elem)
		return
	}

	// Evict if at capacity
	if c.order.Len() >= c.capacity {
		c.evictOldest()
	}

	// Add new entry
	entry := &CacheEntry{
		Key:       key,
		Value:     value,
		ExpiresAt: time.Now().Add(ttl),
	}
	elem := c.order.PushFront(entry)
	c.items[key] = elem
}

// Delete removes a value from the cache
func (c *LRUCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.removeElement(elem)
	}
}

// Clear removes all entries from the cache
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.order.Init()
}

// Len returns the number of items in the cache
func (c *LRUCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.order.Len()
}

// evictOldest removes the least recently used item
func (c *LRUCache) evictOldest() {
	elem := c.order.Back()
	if elem != nil {
		c.removeElement(elem)
	}
}

// removeElement removes an element from the cache
func (c *LRUCache) removeElement(elem *list.Element) {
	entry := elem.Value.(*CacheEntry)
	delete(c.items, entry.Key)
	c.order.Remove(elem)
}

// Cleanup removes all expired entries
func (c *LRUCache) Cleanup() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	var removed int
	for elem := c.order.Back(); elem != nil; {
		prev := elem.Prev()
		entry := elem.Value.(*CacheEntry)
		if entry.IsExpired() {
			c.removeElement(elem)
			removed++
		}
		elem = prev
	}
	return removed
}

// TwoLevelCache provides L1 (in-memory) and optional L2 (external) caching
type TwoLevelCache struct {
	l1    *LRUCache
	l2    L2Cache
	l1TTL time.Duration
	l2TTL time.Duration
	useL2 bool
}

// L2Cache defines the interface for L2 cache (e.g., Redis)
type L2Cache interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte, ttl time.Duration) error
	Delete(key string) error
}

// TwoLevelCacheConfig configures the two-level cache
type TwoLevelCacheConfig struct {
	L1Capacity int           // L1 cache capacity (default: 1000)
	L1TTL      time.Duration // L1 TTL (default: 5 minutes)
	L2TTL      time.Duration // L2 TTL (default: 1 hour)
	L2Cache    L2Cache       // Optional L2 cache implementation
}

// NewTwoLevelCache creates a new two-level cache
func NewTwoLevelCache(config TwoLevelCacheConfig) *TwoLevelCache {
	if config.L1Capacity <= 0 {
		config.L1Capacity = 1000
	}
	if config.L1TTL <= 0 {
		config.L1TTL = 5 * time.Minute
	}
	if config.L2TTL <= 0 {
		config.L2TTL = 1 * time.Hour
	}

	return &TwoLevelCache{
		l1:    NewLRUCache(config.L1Capacity, config.L1TTL),
		l2:    config.L2Cache,
		l1TTL: config.L1TTL,
		l2TTL: config.L2TTL,
		useL2: config.L2Cache != nil,
	}
}

// Get retrieves a value, checking L1 first, then L2
func (c *TwoLevelCache) Get(key string, unmarshal func([]byte) (any, error)) (any, bool) {
	// Check L1
	if value, ok := c.l1.Get(key); ok {
		return value, true
	}

	// Check L2 if available
	if c.useL2 && unmarshal != nil {
		data, err := c.l2.Get(key)
		if err == nil && data != nil {
			value, err := unmarshal(data)
			if err == nil {
				// Populate L1
				c.l1.Set(key, value)
				return value, true
			}
		}
	}

	return nil, false
}

// Set stores a value in both L1 and L2
func (c *TwoLevelCache) Set(key string, value any, marshal func(any) ([]byte, error)) error {
	// Set in L1
	c.l1.Set(key, value)

	// Set in L2 if available
	if c.useL2 && marshal != nil {
		data, err := marshal(value)
		if err != nil {
			return err
		}
		return c.l2.Set(key, data, c.l2TTL)
	}

	return nil
}

// Delete removes a value from both L1 and L2
func (c *TwoLevelCache) Delete(key string) error {
	c.l1.Delete(key)

	if c.useL2 {
		return c.l2.Delete(key)
	}

	return nil
}

// Clear removes all entries from L1 (L2 is not cleared)
func (c *TwoLevelCache) Clear() {
	c.l1.Clear()
}
