package v2

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 同一个 InfoHash 在两个站点上：KeepDuplicates 为真时两条都在（按条目搜索要列出每个来源），为假时和以前一样合并成一条。
func TestSearchOrchestrator_KeepDuplicates(t *testing.T) {
	o := NewSearchOrchestrator(SearchOrchestratorConfig{})
	o.RegisterSite(&mockSearchSite{id: "site1", items: []TorrentItem{{ID: "1", Title: "Test", InfoHash: "abc123", Seeders: 10, SourceSite: "site1"}}})
	o.RegisterSite(&mockSearchSite{id: "site2", items: []TorrentItem{{ID: "2", Title: "Test", InfoHash: "abc123", Seeders: 20, SourceSite: "site2"}}})

	kept, err := o.Search(context.Background(), MultiSiteSearchQuery{SearchQuery: SearchQuery{Keyword: "test"}, KeepDuplicates: true})
	require.NoError(t, err)
	require.Len(t, kept.Items, 2)
	sites := []string{kept.Items[0].SourceSite, kept.Items[1].SourceSite}
	assert.ElementsMatch(t, []string{"site1", "site2"}, sites)
	assert.Equal(t, 2, kept.TotalResults)
	assert.Equal(t, map[string]int{"site1": 1, "site2": 1}, kept.SiteResults)

	merged, err := o.Search(context.Background(), MultiSiteSearchQuery{SearchQuery: SearchQuery{Keyword: "test"}})
	require.NoError(t, err)
	assert.Len(t, merged.Items, 1)
	assert.Equal(t, 20, merged.Items[0].Seeders)
}

// 两种查询的缓存互不影响：先按合并的搜一次，再要保留重复的，拿到的不是缓存里合并过的那份。
func TestCachedSearchOrchestrator_KeepDuplicatesCachedSeparately(t *testing.T) {
	o := NewSearchOrchestrator(SearchOrchestratorConfig{})
	o.RegisterSite(&mockSearchSite{id: "site1", items: []TorrentItem{{ID: "1", Title: "Test", InfoHash: "abc123", SourceSite: "site1"}}})
	o.RegisterSite(&mockSearchSite{id: "site2", items: []TorrentItem{{ID: "2", Title: "Test", InfoHash: "abc123", SourceSite: "site2"}}})
	cached := NewCachedSearchOrchestrator(o, SearchCacheConfig{})

	q := MultiSiteSearchQuery{SearchQuery: SearchQuery{Keyword: "test"}}
	merged, err := cached.Search(context.Background(), q)
	require.NoError(t, err)
	assert.Len(t, merged.Items, 1)

	q.KeepDuplicates = true
	kept, err := cached.Search(context.Background(), q)
	require.NoError(t, err)
	assert.Len(t, kept.Items, 2)
	assert.Equal(t, 2, cached.CacheSize())

	again, err := cached.Search(context.Background(), MultiSiteSearchQuery{SearchQuery: SearchQuery{Keyword: "test"}})
	require.NoError(t, err)
	assert.Len(t, again.Items, 1, "合并的那份缓存没被覆盖")
}
