package v2

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// BaseSite wraps a Driver and provides common functionality like rate limiting and logging.
// It implements the Site interface by delegating to the underlying Driver.
type BaseSite[Req any, Res any] struct {
	id       string
	name     string
	kind     SiteKind
	driver   Driver[Req, Res]
	limiter  *rate.Limiter
	logger   *zap.Logger
	creds    Credentials
	loggedIn bool
	mu       sync.RWMutex
}

// BaseSiteConfig holds configuration for creating a BaseSite
type BaseSiteConfig struct {
	ID        string
	Name      string
	Kind      SiteKind
	RateLimit float64 // Requests per second
	RateBurst int     // Maximum burst size
	Logger    *zap.Logger
}

// NewBaseSite creates a new BaseSite with the given driver and configuration
func NewBaseSite[Req, Res any](driver Driver[Req, Res], config BaseSiteConfig) *BaseSite[Req, Res] {
	// Default rate limit: 1 request per second with burst of 3
	rateLimit := config.RateLimit
	if rateLimit <= 0 {
		rateLimit = 1.0
	}
	rateBurst := config.RateBurst
	if rateBurst <= 0 {
		rateBurst = 3
	}

	logger := config.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	return &BaseSite[Req, Res]{
		id:      config.ID,
		name:    config.Name,
		kind:    config.Kind,
		driver:  driver,
		limiter: rate.NewLimiter(rate.Limit(rateLimit), rateBurst),
		logger:  logger,
	}
}

// ID returns the unique site identifier
func (b *BaseSite[Req, Res]) ID() string {
	return b.id
}

// Name returns the human-readable site name
func (b *BaseSite[Req, Res]) Name() string {
	return b.name
}

// Kind returns the site architecture type
func (b *BaseSite[Req, Res]) Kind() SiteKind {
	return b.kind
}

// Login authenticates with the site
func (b *BaseSite[Req, Res]) Login(ctx context.Context, creds Credentials) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.creds = creds
	b.loggedIn = true
	b.logger.Info("Logged in to site", zap.String("site", b.name))
	return nil
}

// IsLoggedIn returns whether the site is currently logged in
func (b *BaseSite[Req, Res]) IsLoggedIn() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.loggedIn
}

// Search searches for torrents on the site
func (b *BaseSite[Req, Res]) Search(ctx context.Context, query SearchQuery) ([]TorrentItem, error) {
	// Validate query
	if err := query.Validate(); err != nil {
		return nil, fmt.Errorf("invalid query: %w", err)
	}

	// Rate limiting
	if err := b.limiter.Wait(ctx); err != nil {
		b.logger.Warn("Rate limit wait failed", zap.Error(err))
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	startTime := time.Now()
	b.logger.Debug("Starting search",
		zap.String("site", b.name),
		zap.String("keyword", query.Keyword),
		zap.Bool("freeOnly", query.FreeOnly),
	)

	// Prepare request using driver
	req, err := b.driver.PrepareSearch(query)
	if err != nil {
		b.logger.Error("Failed to prepare search request", zap.Error(err))
		return nil, fmt.Errorf("prepare search: %w", err)
	}

	// Execute request
	res, err := b.driver.Execute(ctx, req)
	if err != nil {
		b.logger.Error("Failed to execute search request", zap.Error(err))
		return nil, fmt.Errorf("execute search: %w", err)
	}

	// Parse response
	items, err := b.driver.ParseSearch(res)
	if err != nil {
		b.logger.Error("Failed to parse search response", zap.Error(err))
		return nil, fmt.Errorf("parse search: %w", err)
	}

	// Set source site for all items
	for i := range items {
		items[i].SourceSite = b.id
	}

	b.logger.Info("Search completed",
		zap.String("site", b.name),
		zap.Int("results", len(items)),
		zap.Duration("duration", time.Since(startTime)),
	)

	return items, nil
}

// GetUserInfo fetches the current user's information
func (b *BaseSite[Req, Res]) GetUserInfo(ctx context.Context) (UserInfo, error) {
	// Rate limiting
	if err := b.limiter.Wait(ctx); err != nil {
		return UserInfo{}, fmt.Errorf("rate limit: %w", err)
	}

	startTime := time.Now()
	b.logger.Debug("Fetching user info", zap.String("site", b.name))

	// Delegate to driver which handles all API calls
	info, err := b.driver.GetUserInfo(ctx)
	if err != nil {
		b.logger.Error("Failed to fetch user info", zap.Error(err))
		return UserInfo{}, fmt.Errorf("get user info: %w", err)
	}

	// Set site and update time
	info.Site = b.id
	info.LastUpdate = time.Now().Unix()

	b.logger.Info("User info fetched",
		zap.String("site", b.name),
		zap.String("username", info.Username),
		zap.Duration("duration", time.Since(startTime)),
	)

	return info, nil
}

// Download downloads a torrent file by ID
func (b *BaseSite[Req, Res]) Download(ctx context.Context, torrentID string) ([]byte, error) {
	// Rate limiting
	if err := b.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	startTime := time.Now()
	b.logger.Debug("Downloading torrent",
		zap.String("site", b.name),
		zap.String("torrentID", torrentID),
	)

	// Prepare request
	req, err := b.driver.PrepareDownload(torrentID)
	if err != nil {
		b.logger.Error("Failed to prepare download request", zap.Error(err))
		return nil, fmt.Errorf("prepare download: %w", err)
	}

	// Execute request
	res, err := b.driver.Execute(ctx, req)
	if err != nil {
		b.logger.Error("Failed to execute download request", zap.Error(err))
		return nil, fmt.Errorf("execute download: %w", err)
	}

	// Parse response
	data, err := b.driver.ParseDownload(res)
	if err != nil {
		b.logger.Error("Failed to parse download response", zap.Error(err))
		return nil, fmt.Errorf("parse download: %w", err)
	}

	b.logger.Info("Torrent downloaded",
		zap.String("site", b.name),
		zap.String("torrentID", torrentID),
		zap.Int("size", len(data)),
		zap.Duration("duration", time.Since(startTime)),
	)

	return data, nil
}

// Close releases any resources held by the site
func (b *BaseSite[Req, Res]) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.loggedIn = false
	b.logger.Info("Site closed", zap.String("site", b.name))
	return nil
}

// GetDriver returns the underlying driver (for testing purposes)
func (b *BaseSite[Req, Res]) GetDriver() Driver[Req, Res] {
	return b.driver
}

// GetRateLimiter returns the rate limiter (for testing purposes)
func (b *BaseSite[Req, Res]) GetRateLimiter() *rate.Limiter {
	return b.limiter
}

// DownloadWithHash downloads a torrent using hash if driver supports it
func (b *BaseSite[Req, Res]) DownloadWithHash(ctx context.Context, torrentID, hash string) ([]byte, error) {
	if err := b.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	if hd, ok := any(b.driver).(HashDownloader); ok {
		return hd.DownloadWithHash(ctx, torrentID, hash)
	}
	return b.Download(ctx, torrentID)
}
