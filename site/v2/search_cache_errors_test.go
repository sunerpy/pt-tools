package v2

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 有站点出错的搜索结果不缓存：站点恢复后，同样的查询要重新去站点搜。
func TestCachedSearchOrchestrator_DoesNotCacheResultsWithErrors(t *testing.T) {
	o := NewSearchOrchestrator(SearchOrchestratorConfig{})
	flaky := &mockSearchSite{id: "flaky", name: "Flaky", err: errors.New("timeout")}
	o.RegisterSite(flaky)
	cached := NewCachedSearchOrchestrator(o, SearchCacheConfig{TTL: time.Hour, MaxSize: 10})
	q := MultiSiteSearchQuery{SearchQuery: SearchQuery{Keyword: "movie"}}

	res, err := cached.Search(context.Background(), q)
	require.NoError(t, err)
	require.NotEmpty(t, res.Errors)
	assert.Zero(t, cached.CacheSize(), "带错误的结果不进缓存")

	flaky.err = nil
	flaky.items = []TorrentItem{{ID: "1", Title: "Movie", SourceSite: "flaky"}}
	res, err = cached.Search(context.Background(), q)
	require.NoError(t, err)
	assert.Len(t, res.Items, 1, "站点恢复后拿到新结果")
	assert.Equal(t, 1, cached.CacheSize(), "成功的结果照常缓存")
}
