package v2

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

// SearchCache provides caching for search results
type SearchCache struct {
	mu      sync.RWMutex
	entries map[string]*searchCacheEntry
	ttl     time.Duration
	maxSize int
	// items 按（站点, 种子编号）索引缓存的结果里的每个种子，同样的过期时间（一键追剧按搜索结果里的种子取条目）
	items map[string]itemCacheEntry
}

type itemCacheEntry struct {
	item      TorrentItem
	expiresAt time.Time
}

// maxCachedItems 是种子索引最多几条。
const maxCachedItems = 50000

func itemKey(site, id string) string { return strings.ToLower(site) + "\x00" + id }

type searchCacheEntry struct {
	result    *MultiSiteSearchResult
	expiresAt time.Time
	createdAt time.Time
}

// SearchCacheConfig holds configuration for SearchCache
type SearchCacheConfig struct {
	// TTL is the time-to-live for cache entries (default: 5 minutes)
	TTL time.Duration `json:"ttl,omitempty"`
	// MaxSize is the maximum number of entries (default: 1000)
	MaxSize int `json:"maxSize,omitempty"`
}

// NewSearchCache creates a new SearchCache
func NewSearchCache(config SearchCacheConfig) *SearchCache {
	if config.TTL <= 0 {
		config.TTL = 5 * time.Minute
	}
	if config.MaxSize <= 0 {
		config.MaxSize = 1000
	}

	return &SearchCache{
		entries: make(map[string]*searchCacheEntry),
		ttl:     config.TTL,
		maxSize: config.MaxSize,
		items:   make(map[string]itemCacheEntry),
	}
}

// Get retrieves a cached search result
func (c *SearchCache) Get(query MultiSiteSearchQuery) (*MultiSiteSearchResult, bool) {
	key := c.hashQuery(query)

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		return nil, false
	}

	if time.Now().After(entry.expiresAt) {
		// Entry expired, remove it
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, false
	}

	return entry.result, true
}

// Set stores a search result in the cache
func (c *SearchCache) Set(query MultiSiteSearchQuery, result *MultiSiteSearchResult) {
	key := c.hashQuery(query)
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict old entries if at capacity
	if len(c.entries) >= c.maxSize {
		c.evictOldest()
	}

	c.entries[key] = &searchCacheEntry{
		result:    result,
		expiresAt: now.Add(c.ttl),
		createdAt: now,
	}
	if result == nil {
		return
	}
	for _, it := range result.Items {
		if it.SourceSite == "" || it.ID == "" {
			continue
		}
		c.items[itemKey(it.SourceSite, it.ID)] = itemCacheEntry{item: it, expiresAt: now.Add(c.ttl)}
	}
	if len(c.items) > maxCachedItems {
		for k, e := range c.items {
			if now.After(e.expiresAt) {
				delete(c.items, k)
			}
		}
		for k := range c.items {
			if len(c.items) <= maxCachedItems {
				break
			}
			delete(c.items, k)
		}
	}
}

// Item 取缓存的搜索结果里的一个种子（按站点与种子编号；过期的不算）。
func (c *SearchCache) Item(site, id string) (TorrentItem, bool) {
	c.mu.RLock()
	e, ok := c.items[itemKey(site, id)]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.expiresAt) {
		return TorrentItem{}, false
	}
	return e.item, true
}

// Delete removes a specific entry from the cache
func (c *SearchCache) Delete(query MultiSiteSearchQuery) {
	key := c.hashQuery(query)

	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, key)
}

// Clear removes all entries from the cache
func (c *SearchCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*searchCacheEntry)
	c.items = make(map[string]itemCacheEntry)
}

// Size returns the number of entries in the cache
func (c *SearchCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.entries)
}

// Cleanup removes expired entries from the cache
func (c *SearchCache) Cleanup() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	removed := 0

	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
			removed++
		}
	}
	for key, e := range c.items {
		if now.After(e.expiresAt) {
			delete(c.items, key)
		}
	}

	return removed
}

// hashQuery creates a hash key for a search query
func (c *SearchCache) hashQuery(query MultiSiteSearchQuery) string {
	// Create a deterministic representation of the query
	data, _ := json.Marshal(query)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// evictOldest removes the oldest entry from the cache
func (c *SearchCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.entries {
		if oldestKey == "" || entry.createdAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.createdAt
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

// CachedSearchOrchestrator wraps SearchOrchestrator with caching
type CachedSearchOrchestrator struct {
	*SearchOrchestrator
	cache *SearchCache
}

// NewCachedSearchOrchestrator creates a new CachedSearchOrchestrator
func NewCachedSearchOrchestrator(orchestrator *SearchOrchestrator, cacheConfig SearchCacheConfig) *CachedSearchOrchestrator {
	return &CachedSearchOrchestrator{
		SearchOrchestrator: orchestrator,
		cache:              NewSearchCache(cacheConfig),
	}
}

// Search performs a cached search
func (c *CachedSearchOrchestrator) Search(ctx context.Context, query MultiSiteSearchQuery) (*MultiSiteSearchResult, error) {
	// Check cache first
	if result, ok := c.cache.Get(query); ok {
		return result, nil
	}

	// Perform actual search
	result, err := c.SearchOrchestrator.Search(ctx, query)
	if err != nil {
		return nil, err
	}

	// 有站点出错（超时、限流等）的结果不缓存：否则站点恢复后，同样的查询在 TTL 内仍拿到旧的失败
	if len(result.Errors) == 0 {
		c.cache.Set(query, result)
	}

	return result, nil
}

// CachedTorrent 取最近搜索结果里的一个种子（按站点与种子编号，与搜索结果同样的过期时间）。
func (c *CachedSearchOrchestrator) CachedTorrent(site, id string) (TorrentItem, bool) {
	return c.cache.Item(site, id)
}

// ClearCache clears the search cache
func (c *CachedSearchOrchestrator) ClearCache() {
	c.cache.Clear()
}

// CacheSize returns the number of cached entries
func (c *CachedSearchOrchestrator) CacheSize() int {
	return c.cache.Size()
}

// CleanupCache removes expired entries
func (c *CachedSearchOrchestrator) CleanupCache() int {
	return c.cache.Cleanup()
}
