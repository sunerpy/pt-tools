package v2

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

// UserInfoService provides user information management across multiple sites
type UserInfoService struct {
	repo   UserInfoRepo
	sites  map[string]Site
	cache  *userInfoCache
	logger *zap.Logger
	mu     sync.RWMutex
}

// UserInfoServiceConfig holds configuration for UserInfoService
type UserInfoServiceConfig struct {
	Repo     UserInfoRepo
	CacheTTL time.Duration
	Logger   *zap.Logger
}

// userInfoCache provides in-memory caching for user info
type userInfoCache struct {
	mu   sync.RWMutex
	data map[string]cachedUserInfo
	ttl  time.Duration
}

type cachedUserInfo struct {
	info      UserInfo
	expiresAt time.Time
}

// NewUserInfoService creates a new UserInfoService
func NewUserInfoService(config UserInfoServiceConfig) *UserInfoService {
	if config.Repo == nil {
		config.Repo = NewInMemoryUserInfoRepo()
	}
	if config.CacheTTL <= 0 {
		config.CacheTTL = 5 * time.Minute
	}
	if config.Logger == nil {
		config.Logger = zap.NewNop()
	}

	return &UserInfoService{
		repo:  config.Repo,
		sites: make(map[string]Site),
		cache: &userInfoCache{
			data: make(map[string]cachedUserInfo),
			ttl:  config.CacheTTL,
		},
		logger: config.Logger,
	}
}

// RegisterSite registers a site for user info fetching
func (s *UserInfoService) RegisterSite(site Site) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sites[site.ID()] = site
	s.logger.Info("Registered site", zap.String("site", site.ID()))
}

// UnregisterSite removes a site from the service
func (s *UserInfoService) UnregisterSite(siteID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sites, siteID)
	s.logger.Info("Unregistered site", zap.String("site", siteID))
}

// GetSite returns a registered site by ID
func (s *UserInfoService) GetSite(siteID string) (Site, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	site, ok := s.sites[siteID]
	return site, ok
}

// ListSites returns all registered site IDs
func (s *UserInfoService) ListSites() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.sites))
	for id := range s.sites {
		ids = append(ids, id)
	}
	return ids
}

// FetchAndSave fetches user info from a site and saves it
func (s *UserInfoService) FetchAndSave(ctx context.Context, siteID string) (UserInfo, error) {
	s.mu.RLock()
	site, ok := s.sites[siteID]
	s.mu.RUnlock()

	if !ok {
		return UserInfo{}, fmt.Errorf("site %s not registered: %w", siteID, ErrSiteNotFound)
	}

	s.logger.Debug("Fetching user info", zap.String("site", siteID))

	info, err := site.GetUserInfo(ctx)
	if err != nil {
		s.logger.Error("Failed to fetch user info",
			zap.String("site", siteID),
			zap.Error(err),
		)
		return UserInfo{}, fmt.Errorf("fetch user info from %s: %w", siteID, err)
	}

	// Save to repository
	if err := s.repo.Save(ctx, info); err != nil {
		s.logger.Error("Failed to save user info",
			zap.String("site", siteID),
			zap.Error(err),
		)
		return UserInfo{}, fmt.Errorf("save user info for %s: %w", siteID, err)
	}

	// Update cache
	s.cache.set(siteID, info)

	s.logger.Info("User info fetched and saved",
		zap.String("site", siteID),
		zap.String("username", info.Username),
		zap.Int64("uploaded", info.Uploaded),
	)

	return info, nil
}

// FetchAndSaveAll fetches user info from all registered sites
func (s *UserInfoService) FetchAndSaveAll(ctx context.Context) ([]UserInfo, []error) {
	s.mu.RLock()
	siteIDs := make([]string, 0, len(s.sites))
	for id := range s.sites {
		siteIDs = append(siteIDs, id)
	}
	s.mu.RUnlock()

	var (
		results []UserInfo
		errors  []error
		mu      sync.Mutex
		wg      sync.WaitGroup
	)

	for _, siteID := range siteIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()

			info, err := s.FetchAndSave(ctx, id)
			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				errors = append(errors, err)
			} else {
				results = append(results, info)
			}
		}(siteID)
	}

	wg.Wait()
	return results, errors
}

// GetUserInfo retrieves user info for a site (from cache or repository)
func (s *UserInfoService) GetUserInfo(ctx context.Context, siteID string) (UserInfo, error) {
	// Check cache first
	if info, ok := s.cache.get(siteID); ok {
		return info, nil
	}

	// Fall back to repository
	info, err := s.repo.Get(ctx, siteID)
	if err != nil {
		return UserInfo{}, err
	}

	// Update cache
	s.cache.set(siteID, info)
	return info, nil
}

// GetAllUserInfo retrieves all stored user info
func (s *UserInfoService) GetAllUserInfo(ctx context.Context) ([]UserInfo, error) {
	return s.repo.ListAll(ctx)
}

// GetAggregated returns aggregated statistics across all sites
func (s *UserInfoService) GetAggregated(ctx context.Context) (AggregatedStats, error) {
	return s.repo.GetAggregated(ctx)
}

// DeleteUserInfo removes user info for a site
func (s *UserInfoService) DeleteUserInfo(ctx context.Context, siteID string) error {
	s.cache.delete(siteID)
	return s.repo.Delete(ctx, siteID)
}

// Cache methods

func (c *userInfoCache) get(siteID string) (UserInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached, ok := c.data[siteID]
	if !ok {
		return UserInfo{}, false
	}

	if time.Now().After(cached.expiresAt) {
		return UserInfo{}, false
	}

	return cached.info, true
}

func (c *userInfoCache) set(siteID string, info UserInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[siteID] = cachedUserInfo{
		info:      info,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *userInfoCache) delete(siteID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, siteID)
}

func (c *userInfoCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = make(map[string]cachedUserInfo)
}

// ClearCache clears the user info cache
func (s *UserInfoService) ClearCache() {
	s.cache.clear()
}

// SyncError represents a sync error for a specific site
type SyncError struct {
	Site  string
	Error error
}

// FetchAndSaveAllWithConcurrency fetches user info from all registered sites with concurrency control
// maxConcurrent: maximum number of concurrent requests
// timeout: timeout for each site request
func (s *UserInfoService) FetchAndSaveAllWithConcurrency(
	ctx context.Context,
	maxConcurrent int,
	timeout time.Duration,
) ([]UserInfo, []SyncError) {
	startTime := time.Now()

	s.mu.RLock()
	siteIDs := make([]string, 0, len(s.sites))
	for id := range s.sites {
		siteIDs = append(siteIDs, id)
	}
	s.mu.RUnlock()

	if len(siteIDs) == 0 {
		return nil, nil
	}

	s.logger.Info("Starting concurrent user info fetch",
		zap.Int("sites", len(siteIDs)),
		zap.Int("maxConcurrent", maxConcurrent),
		zap.Duration("timeout", timeout),
	)

	// Use semaphore for concurrency control with errgroup
	sem := semaphore.NewWeighted(int64(maxConcurrent))
	g, gctx := errgroup.WithContext(ctx)

	var mu sync.Mutex
	var results []UserInfo
	var errors []SyncError

	for _, siteID := range siteIDs {
		siteID := siteID // capture loop variable
		g.Go(func() error {
			// Acquire semaphore
			if err := sem.Acquire(gctx, 1); err != nil {
				mu.Lock()
				errors = append(errors, SyncError{Site: siteID, Error: err})
				mu.Unlock()
				return nil // Don't fail the whole operation
			}
			defer sem.Release(1)

			// Create context with timeout for this site
			siteCtx, cancel := context.WithTimeout(gctx, timeout)
			defer cancel()

			siteStartTime := time.Now()
			info, err := s.FetchAndSave(siteCtx, siteID)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				s.logger.Warn("Failed to fetch user info",
					zap.String("site", siteID),
					zap.Duration("duration", time.Since(siteStartTime)),
					zap.Error(err),
				)
				errors = append(errors, SyncError{Site: siteID, Error: err})
			} else {
				s.logger.Debug("Fetched user info successfully",
					zap.String("site", siteID),
					zap.Duration("duration", time.Since(siteStartTime)),
				)
				results = append(results, info)
			}
			return nil
		})
	}

	// Wait for all to complete (errors are collected, not returned)
	_ = g.Wait()

	s.logger.Info("Completed concurrent user info fetch",
		zap.Int("successful", len(results)),
		zap.Int("failed", len(errors)),
		zap.Duration("totalDuration", time.Since(startTime)),
	)

	return results, errors
}
