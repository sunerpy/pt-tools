package v2

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSearchSite implements Site for testing
type mockSearchSite struct {
	id       string
	name     string
	kind     SiteKind
	items    []TorrentItem
	err      error
	delay    time.Duration
	userInfo UserInfo
}

func (m *mockSearchSite) ID() string                                         { return m.id }
func (m *mockSearchSite) Name() string                                       { return m.name }
func (m *mockSearchSite) Kind() SiteKind                                     { return m.kind }
func (m *mockSearchSite) Login(ctx context.Context, creds Credentials) error { return nil }
func (m *mockSearchSite) GetUserInfo(ctx context.Context) (UserInfo, error)  { return m.userInfo, nil }
func (m *mockSearchSite) Download(ctx context.Context, torrentID string) ([]byte, error) {
	return nil, nil
}
func (m *mockSearchSite) Close() error { return nil }

func (m *mockSearchSite) Search(ctx context.Context, query SearchQuery) ([]TorrentItem, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if m.err != nil {
		return nil, m.err
	}
	return m.items, nil
}

func TestNewSearchOrchestrator(t *testing.T) {
	o := NewSearchOrchestrator(SearchOrchestratorConfig{})
	assert.NotNil(t, o)
	assert.NotNil(t, o.sites)
	assert.NotNil(t, o.normalizer)
	assert.NotNil(t, o.deduper)
	assert.NotNil(t, o.ranker)
}

func TestSearchOrchestrator_RegisterUnregisterSite(t *testing.T) {
	o := NewSearchOrchestrator(SearchOrchestratorConfig{})

	site := &mockSearchSite{id: "site1", name: "Site 1"}
	o.RegisterSite(site)

	sites := o.ListSites()
	assert.Contains(t, sites, "site1")

	o.UnregisterSite("site1")
	sites = o.ListSites()
	assert.NotContains(t, sites, "site1")
}

func TestSearchOrchestrator_Search_SingleSite(t *testing.T) {
	o := NewSearchOrchestrator(SearchOrchestratorConfig{})

	site := &mockSearchSite{
		id:   "site1",
		name: "Site 1",
		items: []TorrentItem{
			{ID: "1", Title: "Test Torrent 1", Seeders: 10, SourceSite: "site1"},
			{ID: "2", Title: "Test Torrent 2", Seeders: 20, SourceSite: "site1"},
		},
	}
	o.RegisterSite(site)

	result, err := o.Search(context.Background(), MultiSiteSearchQuery{
		SearchQuery: SearchQuery{Keyword: "test"},
	})

	require.NoError(t, err)
	assert.Len(t, result.Items, 2)
	assert.Equal(t, 2, result.TotalResults)
	assert.Equal(t, 2, result.SiteResults["site1"])
}

func TestSearchOrchestrator_Search_MultipleSites(t *testing.T) {
	o := NewSearchOrchestrator(SearchOrchestratorConfig{})

	site1 := &mockSearchSite{
		id:   "site1",
		name: "Site 1",
		items: []TorrentItem{
			{ID: "1", Title: "Test Torrent 1", Seeders: 10, SourceSite: "site1"},
		},
	}
	site2 := &mockSearchSite{
		id:   "site2",
		name: "Site 2",
		items: []TorrentItem{
			{ID: "2", Title: "Test Torrent 2", Seeders: 20, SourceSite: "site2"},
		},
	}
	o.RegisterSite(site1)
	o.RegisterSite(site2)

	result, err := o.Search(context.Background(), MultiSiteSearchQuery{
		SearchQuery: SearchQuery{Keyword: "test"},
	})

	require.NoError(t, err)
	assert.Len(t, result.Items, 2)
	assert.Equal(t, 1, result.SiteResults["site1"])
	assert.Equal(t, 1, result.SiteResults["site2"])
}

func TestSearchOrchestrator_Search_SpecificSites(t *testing.T) {
	o := NewSearchOrchestrator(SearchOrchestratorConfig{})

	site1 := &mockSearchSite{
		id:    "site1",
		items: []TorrentItem{{ID: "1", SourceSite: "site1"}},
	}
	site2 := &mockSearchSite{
		id:    "site2",
		items: []TorrentItem{{ID: "2", SourceSite: "site2"}},
	}
	o.RegisterSite(site1)
	o.RegisterSite(site2)

	result, err := o.Search(context.Background(), MultiSiteSearchQuery{
		SearchQuery: SearchQuery{Keyword: "test"},
		Sites:       []string{"site1"},
	})

	require.NoError(t, err)
	assert.Len(t, result.Items, 1)
	assert.Equal(t, 1, result.SiteResults["site1"])
	assert.Equal(t, 0, result.SiteResults["site2"])
}

func TestSearchOrchestrator_Search_WithTimeout(t *testing.T) {
	o := NewSearchOrchestrator(SearchOrchestratorConfig{})

	site := &mockSearchSite{
		id:    "site1",
		delay: 500 * time.Millisecond,
		items: []TorrentItem{{ID: "1", SourceSite: "site1"}},
	}
	o.RegisterSite(site)

	result, err := o.Search(context.Background(), MultiSiteSearchQuery{
		SearchQuery: SearchQuery{Keyword: "test"},
		Timeout:     100 * time.Millisecond,
	})

	require.NoError(t, err)
	assert.Len(t, result.Items, 0)
	assert.Len(t, result.Errors, 1)
	assert.Equal(t, "site1", result.Errors[0].Site)
}

func TestSearchOrchestrator_Search_PartialFailure(t *testing.T) {
	o := NewSearchOrchestrator(SearchOrchestratorConfig{})

	site1 := &mockSearchSite{
		id:    "site1",
		items: []TorrentItem{{ID: "1", SourceSite: "site1"}},
	}
	site2 := &mockSearchSite{
		id:  "site2",
		err: errors.New("connection failed"),
	}
	o.RegisterSite(site1)
	o.RegisterSite(site2)

	result, err := o.Search(context.Background(), MultiSiteSearchQuery{
		SearchQuery: SearchQuery{Keyword: "test"},
	})

	require.NoError(t, err)
	assert.Len(t, result.Items, 1)
	assert.Len(t, result.Errors, 1)
	assert.Equal(t, "site2", result.Errors[0].Site)
}

func TestSearchOrchestrator_Search_Deduplication(t *testing.T) {
	o := NewSearchOrchestrator(SearchOrchestratorConfig{})

	// Same torrent on two sites (same InfoHash)
	site1 := &mockSearchSite{
		id: "site1",
		items: []TorrentItem{
			{ID: "1", Title: "Test", InfoHash: "abc123", Seeders: 10, SourceSite: "site1"},
		},
	}
	site2 := &mockSearchSite{
		id: "site2",
		items: []TorrentItem{
			{ID: "2", Title: "Test", InfoHash: "abc123", Seeders: 20, SourceSite: "site2"},
		},
	}
	o.RegisterSite(site1)
	o.RegisterSite(site2)

	result, err := o.Search(context.Background(), MultiSiteSearchQuery{
		SearchQuery: SearchQuery{Keyword: "test"},
	})

	require.NoError(t, err)
	assert.Len(t, result.Items, 1)
	assert.Equal(t, 2, result.TotalResults)
	// Should keep the best seeders count
	assert.Equal(t, 20, result.Items[0].Seeders)
}

func TestSearchOrchestrator_Search_Filters(t *testing.T) {
	o := NewSearchOrchestrator(SearchOrchestratorConfig{})

	site := &mockSearchSite{
		id: "site1",
		items: []TorrentItem{
			{ID: "1", Title: "Small", SizeBytes: 100, Seeders: 5, SourceSite: "site1"},
			{ID: "2", Title: "Medium", SizeBytes: 500, Seeders: 10, SourceSite: "site1"},
			{ID: "3", Title: "Large", SizeBytes: 1000, Seeders: 20, SourceSite: "site1"},
		},
	}
	o.RegisterSite(site)

	t.Run("MinSeeders", func(t *testing.T) {
		result, err := o.Search(context.Background(), MultiSiteSearchQuery{
			SearchQuery: SearchQuery{Keyword: "test"},
			MinSeeders:  10,
		})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
	})

	t.Run("MaxSizeBytes", func(t *testing.T) {
		result, err := o.Search(context.Background(), MultiSiteSearchQuery{
			SearchQuery:  SearchQuery{Keyword: "test"},
			MaxSizeBytes: 500,
		})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
	})

	t.Run("MinSizeBytes", func(t *testing.T) {
		result, err := o.Search(context.Background(), MultiSiteSearchQuery{
			SearchQuery:  SearchQuery{Keyword: "test"},
			MinSizeBytes: 500,
		})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
	})
}

func TestSearchOrchestrator_Search_FreeOnly(t *testing.T) {
	o := NewSearchOrchestrator(SearchOrchestratorConfig{})

	site := &mockSearchSite{
		id: "site1",
		items: []TorrentItem{
			{ID: "1", Title: "Free", DiscountLevel: DiscountFree, SourceSite: "site1"},
			{ID: "2", Title: "Normal", DiscountLevel: DiscountNone, SourceSite: "site1"},
			{ID: "3", Title: "2xFree", DiscountLevel: Discount2xFree, SourceSite: "site1"},
		},
	}
	o.RegisterSite(site)

	result, err := o.Search(context.Background(), MultiSiteSearchQuery{
		SearchQuery: SearchQuery{Keyword: "test", FreeOnly: true},
	})

	require.NoError(t, err)
	assert.Len(t, result.Items, 2)
	for _, item := range result.Items {
		assert.True(t, item.IsFree())
	}
}

func TestSearchOrchestrator_Search_NoSites(t *testing.T) {
	o := NewSearchOrchestrator(SearchOrchestratorConfig{})

	result, err := o.Search(context.Background(), MultiSiteSearchQuery{
		SearchQuery: SearchQuery{Keyword: "test"},
	})

	require.NoError(t, err)
	assert.Len(t, result.Items, 0)
}

func TestSearchOrchestrator_Search_Ranking(t *testing.T) {
	o := NewSearchOrchestrator(SearchOrchestratorConfig{})

	site := &mockSearchSite{
		id: "site1",
		items: []TorrentItem{
			{ID: "1", Title: "Low Seeders", Seeders: 5, SourceSite: "site1"},
			{ID: "2", Title: "High Seeders", Seeders: 100, SourceSite: "site1"},
			{ID: "3", Title: "Medium Seeders", Seeders: 50, SourceSite: "site1"},
		},
	}
	o.RegisterSite(site)

	result, err := o.Search(context.Background(), MultiSiteSearchQuery{
		SearchQuery: SearchQuery{Keyword: "test"},
	})

	require.NoError(t, err)
	assert.Len(t, result.Items, 3)
	// Should be sorted by score (seeders)
	assert.Equal(t, 100, result.Items[0].Seeders)
	assert.Equal(t, 50, result.Items[1].Seeders)
	assert.Equal(t, 5, result.Items[2].Seeders)
}
