package v2

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// SearchOrchestrator coordinates concurrent searches across multiple sites
type SearchOrchestrator struct {
	sites      map[string]Site
	normalizer *Normalizer
	deduper    *Deduper
	ranker     *Ranker
	logger     *zap.Logger
	mu         sync.RWMutex
}

// SearchOrchestratorConfig holds configuration for SearchOrchestrator
type SearchOrchestratorConfig struct {
	Logger *zap.Logger
}

// MultiSiteSearchQuery extends SearchQuery with multi-site options
type MultiSiteSearchQuery struct {
	SearchQuery
	// Sites specifies which sites to search (empty means all)
	Sites []string `json:"sites,omitempty"`
	// Timeout is the maximum time to wait for all searches
	Timeout time.Duration `json:"timeout,omitempty"`
	// MinSeeders filters results by minimum seeders
	MinSeeders int `json:"minSeeders,omitempty"`
	// MaxSizeBytes filters results by maximum size
	MaxSizeBytes int64 `json:"maxSizeBytes,omitempty"`
	// MinSizeBytes filters results by minimum size
	MinSizeBytes int64 `json:"minSizeBytes,omitempty"`
}

// MultiSiteSearchResult contains results from a multi-site search
type MultiSiteSearchResult struct {
	// Items contains deduplicated and ranked torrent items
	Items []TorrentItem `json:"items"`
	// TotalResults is the total number of results before deduplication
	TotalResults int `json:"totalResults"`
	// SiteResults contains per-site result counts
	SiteResults map[string]int `json:"siteResults"`
	// Errors contains any errors that occurred during search
	Errors []SearchError `json:"errors,omitempty"`
	// Duration is how long the search took
	Duration time.Duration `json:"duration"`
}

// SearchError represents an error from a specific site
type SearchError struct {
	Site  string `json:"site"`
	Error string `json:"error"`
}

// NewSearchOrchestrator creates a new SearchOrchestrator
func NewSearchOrchestrator(config SearchOrchestratorConfig) *SearchOrchestrator {
	if config.Logger == nil {
		config.Logger = zap.NewNop()
	}

	return &SearchOrchestrator{
		sites:      make(map[string]Site),
		normalizer: NewNormalizer(),
		deduper:    NewDeduper(),
		ranker:     NewRanker(RankerConfig{}),
		logger:     config.Logger,
	}
}

// RegisterSite adds a site to the orchestrator
func (o *SearchOrchestrator) RegisterSite(site Site) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sites[site.ID()] = site
	o.logger.Info("Registered site for search", zap.String("site", site.ID()))
}

// UnregisterSite removes a site from the orchestrator
func (o *SearchOrchestrator) UnregisterSite(siteID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.sites, siteID)
	o.logger.Info("Unregistered site from search", zap.String("site", siteID))
}

// ListSites returns all registered site IDs
func (o *SearchOrchestrator) ListSites() []string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	ids := make([]string, 0, len(o.sites))
	for id := range o.sites {
		ids = append(ids, id)
	}
	return ids
}

// GetSite returns a registered site by ID, or nil if not found
func (o *SearchOrchestrator) GetSite(siteID string) Site {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.sites[siteID]
}

// Search performs a concurrent search across multiple sites
func (o *SearchOrchestrator) Search(ctx context.Context, query MultiSiteSearchQuery) (*MultiSiteSearchResult, error) {
	start := time.Now()

	// Apply timeout if specified
	if query.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, query.Timeout)
		defer cancel()
	}

	// Determine which sites to search
	sitesToSearch := o.getSitesToSearch(query.Sites)
	if len(sitesToSearch) == 0 {
		return &MultiSiteSearchResult{
			Items:       []TorrentItem{},
			SiteResults: make(map[string]int),
			Duration:    time.Since(start),
		}, nil
	}

	// Execute concurrent searches
	results, errors := o.searchConcurrently(ctx, sitesToSearch, query.SearchQuery)

	// Normalize titles
	for i := range results {
		results[i].Title = o.normalizer.NormalizeTitle(results[i].Title)
	}

	// Apply filters
	results = o.applyFilters(results, query)

	// Deduplicate by InfoHash
	totalBeforeDedup := len(results)
	results = o.deduper.Deduplicate(results)

	// Rank results
	results = o.ranker.Rank(results)

	// Build site results map
	siteResults := make(map[string]int)
	for _, item := range results {
		siteResults[item.SourceSite]++
	}

	return &MultiSiteSearchResult{
		Items:        results,
		TotalResults: totalBeforeDedup,
		SiteResults:  siteResults,
		Errors:       errors,
		Duration:     time.Since(start),
	}, nil
}

// getSitesToSearch returns the sites to search based on the query
func (o *SearchOrchestrator) getSitesToSearch(requestedSites []string) []Site {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if len(requestedSites) == 0 {
		// Search all sites
		sites := make([]Site, 0, len(o.sites))
		for _, site := range o.sites {
			sites = append(sites, site)
		}
		return sites
	}

	// Search only requested sites
	sites := make([]Site, 0, len(requestedSites))
	for _, siteID := range requestedSites {
		if site, ok := o.sites[siteID]; ok {
			sites = append(sites, site)
		}
	}
	return sites
}

// searchConcurrently executes searches on multiple sites concurrently
func (o *SearchOrchestrator) searchConcurrently(ctx context.Context, sites []Site, query SearchQuery) ([]TorrentItem, []SearchError) {
	var (
		results []TorrentItem
		errors  []SearchError
		mu      sync.Mutex
		wg      sync.WaitGroup
	)

	startTime := time.Now()
	o.logger.Info("Starting concurrent search",
		zap.Int("siteCount", len(sites)),
		zap.String("keyword", query.Keyword),
	)

	for _, site := range sites {
		wg.Add(1)
		go func(s Site) {
			defer wg.Done()
			siteStart := time.Now()

			// Check context before starting
			select {
			case <-ctx.Done():
				mu.Lock()
				errors = append(errors, SearchError{
					Site:  s.ID(),
					Error: ctx.Err().Error(),
				})
				mu.Unlock()
				return
			default:
			}

			o.logger.Debug("Searching site", zap.String("site", s.ID()), zap.String("keyword", query.Keyword))

			items, err := s.Search(ctx, query)
			siteDuration := time.Since(siteStart)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				o.logger.Warn("Search failed",
					zap.String("site", s.ID()),
					zap.Duration("duration", siteDuration),
					zap.Error(err),
				)
				errors = append(errors, SearchError{
					Site:  s.ID(),
					Error: err.Error(),
				})
				return
			}

			o.logger.Info("Site search completed",
				zap.String("site", s.ID()),
				zap.Int("results", len(items)),
				zap.Duration("duration", siteDuration),
			)
			results = append(results, items...)
		}(site)
	}

	wg.Wait()
	o.logger.Info("All site searches completed",
		zap.Int("totalResults", len(results)),
		zap.Int("errorCount", len(errors)),
		zap.Duration("totalDuration", time.Since(startTime)),
	)
	return results, errors
}

// applyFilters applies query filters to results
func (o *SearchOrchestrator) applyFilters(items []TorrentItem, query MultiSiteSearchQuery) []TorrentItem {
	if query.MinSeeders == 0 && query.MaxSizeBytes == 0 && query.MinSizeBytes == 0 && !query.FreeOnly {
		return items
	}

	filtered := make([]TorrentItem, 0, len(items))
	for _, item := range items {
		// Filter by minimum seeders
		if query.MinSeeders > 0 && item.Seeders < query.MinSeeders {
			continue
		}

		// Filter by maximum size
		if query.MaxSizeBytes > 0 && item.SizeBytes > query.MaxSizeBytes {
			continue
		}

		// Filter by minimum size
		if query.MinSizeBytes > 0 && item.SizeBytes < query.MinSizeBytes {
			continue
		}

		// Filter by free only
		if query.FreeOnly && !item.IsFree() {
			continue
		}

		filtered = append(filtered, item)
	}

	return filtered
}
