package v2

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSearchCache(t *testing.T) {
	c := NewSearchCache(SearchCacheConfig{})
	assert.NotNil(t, c)
	assert.Equal(t, 5*time.Minute, c.ttl)
	assert.Equal(t, 1000, c.maxSize)
}

func TestNewSearchCache_CustomConfig(t *testing.T) {
	c := NewSearchCache(SearchCacheConfig{
		TTL:     10 * time.Minute,
		MaxSize: 500,
	})
	assert.Equal(t, 10*time.Minute, c.ttl)
	assert.Equal(t, 500, c.maxSize)
}

func TestSearchCache_GetSet(t *testing.T) {
	c := NewSearchCache(SearchCacheConfig{TTL: 1 * time.Minute})

	query := MultiSiteSearchQuery{
		SearchQuery: SearchQuery{Keyword: "test"},
	}
	result := &MultiSiteSearchResult{
		Items: []TorrentItem{{ID: "1", Title: "Test"}},
	}

	// Initially not in cache
	_, ok := c.Get(query)
	assert.False(t, ok)

	// Set and get
	c.Set(query, result)
	cached, ok := c.Get(query)
	assert.True(t, ok)
	assert.Equal(t, result.Items[0].ID, cached.Items[0].ID)
}

func TestSearchCache_Expiration(t *testing.T) {
	c := NewSearchCache(SearchCacheConfig{TTL: 50 * time.Millisecond})

	query := MultiSiteSearchQuery{
		SearchQuery: SearchQuery{Keyword: "test"},
	}
	result := &MultiSiteSearchResult{
		Items: []TorrentItem{{ID: "1"}},
	}

	c.Set(query, result)

	// Should be in cache
	_, ok := c.Get(query)
	assert.True(t, ok)

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Should be expired
	_, ok = c.Get(query)
	assert.False(t, ok)
}

func TestSearchCache_Delete(t *testing.T) {
	c := NewSearchCache(SearchCacheConfig{})

	query := MultiSiteSearchQuery{
		SearchQuery: SearchQuery{Keyword: "test"},
	}
	result := &MultiSiteSearchResult{
		Items: []TorrentItem{{ID: "1"}},
	}

	c.Set(query, result)
	assert.Equal(t, 1, c.Size())

	c.Delete(query)
	assert.Equal(t, 0, c.Size())
}

func TestSearchCache_Clear(t *testing.T) {
	c := NewSearchCache(SearchCacheConfig{})

	for i := 0; i < 10; i++ {
		query := MultiSiteSearchQuery{
			SearchQuery: SearchQuery{Keyword: string(rune('a' + i))},
		}
		c.Set(query, &MultiSiteSearchResult{})
	}

	assert.Equal(t, 10, c.Size())

	c.Clear()
	assert.Equal(t, 0, c.Size())
}

func TestSearchCache_MaxSize(t *testing.T) {
	c := NewSearchCache(SearchCacheConfig{MaxSize: 5})

	for i := 0; i < 10; i++ {
		query := MultiSiteSearchQuery{
			SearchQuery: SearchQuery{Keyword: string(rune('a' + i))},
		}
		c.Set(query, &MultiSiteSearchResult{})
	}

	// Should not exceed max size
	assert.LessOrEqual(t, c.Size(), 5)
}

func TestSearchCache_Cleanup(t *testing.T) {
	c := NewSearchCache(SearchCacheConfig{TTL: 50 * time.Millisecond})

	for i := 0; i < 5; i++ {
		query := MultiSiteSearchQuery{
			SearchQuery: SearchQuery{Keyword: string(rune('a' + i))},
		}
		c.Set(query, &MultiSiteSearchResult{})
	}

	assert.Equal(t, 5, c.Size())

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	removed := c.Cleanup()
	assert.Equal(t, 5, removed)
	assert.Equal(t, 0, c.Size())
}

func TestSearchCache_DifferentQueries(t *testing.T) {
	c := NewSearchCache(SearchCacheConfig{})

	query1 := MultiSiteSearchQuery{
		SearchQuery: SearchQuery{Keyword: "test1"},
	}
	query2 := MultiSiteSearchQuery{
		SearchQuery: SearchQuery{Keyword: "test2"},
	}

	result1 := &MultiSiteSearchResult{Items: []TorrentItem{{ID: "1"}}}
	result2 := &MultiSiteSearchResult{Items: []TorrentItem{{ID: "2"}}}

	c.Set(query1, result1)
	c.Set(query2, result2)

	cached1, ok := c.Get(query1)
	assert.True(t, ok)
	assert.Equal(t, "1", cached1.Items[0].ID)

	cached2, ok := c.Get(query2)
	assert.True(t, ok)
	assert.Equal(t, "2", cached2.Items[0].ID)
}

func TestSearchCache_SameQueryDifferentSites(t *testing.T) {
	c := NewSearchCache(SearchCacheConfig{})

	query1 := MultiSiteSearchQuery{
		SearchQuery: SearchQuery{Keyword: "test"},
		Sites:       []string{"site1"},
	}
	query2 := MultiSiteSearchQuery{
		SearchQuery: SearchQuery{Keyword: "test"},
		Sites:       []string{"site2"},
	}

	result1 := &MultiSiteSearchResult{Items: []TorrentItem{{ID: "1"}}}
	result2 := &MultiSiteSearchResult{Items: []TorrentItem{{ID: "2"}}}

	c.Set(query1, result1)
	c.Set(query2, result2)

	// Should be different cache entries
	assert.Equal(t, 2, c.Size())

	cached1, _ := c.Get(query1)
	cached2, _ := c.Get(query2)
	assert.Equal(t, "1", cached1.Items[0].ID)
	assert.Equal(t, "2", cached2.Items[0].ID)
}

func TestCachedSearchOrchestrator(t *testing.T) {
	orchestrator := NewSearchOrchestrator(SearchOrchestratorConfig{})

	site := &mockSearchSite{
		id:    "site1",
		items: []TorrentItem{{ID: "1", Title: "Test", SourceSite: "site1"}},
	}
	orchestrator.RegisterSite(site)

	cached := NewCachedSearchOrchestrator(orchestrator, SearchCacheConfig{})

	query := MultiSiteSearchQuery{
		SearchQuery: SearchQuery{Keyword: "test"},
	}

	// First search - should hit the site
	result1, err := cached.Search(context.Background(), query)
	require.NoError(t, err)
	assert.Len(t, result1.Items, 1)
	assert.Equal(t, 1, cached.CacheSize())

	// Second search - should hit the cache
	result2, err := cached.Search(context.Background(), query)
	require.NoError(t, err)
	assert.Len(t, result2.Items, 1)
	assert.Equal(t, 1, cached.CacheSize())
}

func TestCachedSearchOrchestrator_ClearCache(t *testing.T) {
	orchestrator := NewSearchOrchestrator(SearchOrchestratorConfig{})
	cached := NewCachedSearchOrchestrator(orchestrator, SearchCacheConfig{})

	query := MultiSiteSearchQuery{
		SearchQuery: SearchQuery{Keyword: "test"},
	}
	cached.cache.Set(query, &MultiSiteSearchResult{})

	assert.Equal(t, 1, cached.CacheSize())

	cached.ClearCache()
	assert.Equal(t, 0, cached.CacheSize())
}

func TestCachedSearchOrchestrator_CleanupCache(t *testing.T) {
	orchestrator := NewSearchOrchestrator(SearchOrchestratorConfig{})
	cached := NewCachedSearchOrchestrator(orchestrator, SearchCacheConfig{TTL: 50 * time.Millisecond})

	query := MultiSiteSearchQuery{
		SearchQuery: SearchQuery{Keyword: "test"},
	}
	cached.cache.Set(query, &MultiSiteSearchResult{})

	time.Sleep(100 * time.Millisecond)

	removed := cached.CleanupCache()
	assert.Equal(t, 1, removed)
}
