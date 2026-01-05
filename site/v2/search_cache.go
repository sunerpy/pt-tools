package v2

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

// SearchCache provides caching for search results
type SearchCache struct {
	mu      sync.RWMutex
	entries map[string]*searchCacheEntry
	ttl     time.Duration
	maxSize int
}

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

	// Cache the result
	c.cache.Set(query, result)

	return result, nil
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
