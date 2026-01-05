package v2

import (
	"context"
	"sync"
	"time"
)

// UserInfoRepo defines the interface for storing and retrieving user information
type UserInfoRepo interface {
	// Save stores user info for a site
	Save(ctx context.Context, info UserInfo) error
	// Get retrieves user info for a specific site
	Get(ctx context.Context, site string) (UserInfo, error)
	// ListAll retrieves all stored user info
	ListAll(ctx context.Context) ([]UserInfo, error)
	// ListBySites retrieves user info for specific sites
	ListBySites(ctx context.Context, sites []string) ([]UserInfo, error)
	// Delete removes user info for a site
	Delete(ctx context.Context, site string) error
	// GetAggregated calculates aggregated statistics
	GetAggregated(ctx context.Context) (AggregatedStats, error)
}

// InMemoryUserInfoRepo is an in-memory implementation of UserInfoRepo
type InMemoryUserInfoRepo struct {
	mu    sync.RWMutex
	store map[string]UserInfo
}

// NewInMemoryUserInfoRepo creates a new in-memory user info repository
func NewInMemoryUserInfoRepo() *InMemoryUserInfoRepo {
	return &InMemoryUserInfoRepo{
		store: make(map[string]UserInfo),
	}
}

// Save stores user info for a site
func (r *InMemoryUserInfoRepo) Save(ctx context.Context, info UserInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if info.Site == "" {
		return ErrSiteNotFound
	}

	info.LastUpdate = time.Now().Unix()
	r.store[info.Site] = info
	return nil
}

// Get retrieves user info for a specific site
func (r *InMemoryUserInfoRepo) Get(ctx context.Context, site string) (UserInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, ok := r.store[site]
	if !ok {
		return UserInfo{}, ErrSiteNotFound
	}
	return info, nil
}

// ListAll retrieves all stored user info
func (r *InMemoryUserInfoRepo) ListAll(ctx context.Context) ([]UserInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]UserInfo, 0, len(r.store))
	for _, info := range r.store {
		result = append(result, info)
	}
	return result, nil
}

// ListBySites retrieves user info for specific sites
func (r *InMemoryUserInfoRepo) ListBySites(ctx context.Context, sites []string) ([]UserInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]UserInfo, 0, len(sites))
	for _, site := range sites {
		if info, ok := r.store[site]; ok {
			result = append(result, info)
		}
	}
	return result, nil
}

// Delete removes user info for a site
func (r *InMemoryUserInfoRepo) Delete(ctx context.Context, site string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.store[site]; !ok {
		return ErrSiteNotFound
	}
	delete(r.store, site)
	return nil
}

// GetAggregated calculates aggregated statistics
func (r *InMemoryUserInfoRepo) GetAggregated(ctx context.Context) (AggregatedStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := AggregatedStats{
		LastUpdate:   time.Now().Unix(),
		PerSiteStats: make([]UserInfo, 0, len(r.store)),
	}

	var totalRatio float64
	var ratioCount int

	for _, info := range r.store {
		stats.TotalUploaded += info.Uploaded
		stats.TotalDownloaded += info.Downloaded
		stats.TotalSeeding += info.Seeding
		stats.TotalLeeching += info.Leeching
		stats.TotalBonus += info.Bonus
		stats.SiteCount++
		stats.PerSiteStats = append(stats.PerSiteStats, info)

		// Aggregate extended fields
		stats.TotalBonusPerHour += info.BonusPerHour
		stats.TotalSeedingBonus += info.SeedingBonus
		stats.TotalUnreadMessages += info.UnreadMessageCount
		stats.TotalSeederSize += info.SeederSize
		stats.TotalLeecherSize += info.LeecherSize

		// Only count valid ratios for average
		if info.Ratio > 0 && info.Ratio < 1000 { // Exclude infinite ratios
			totalRatio += info.Ratio
			ratioCount++
		}
	}

	if ratioCount > 0 {
		stats.AverageRatio = totalRatio / float64(ratioCount)
	}

	return stats, nil
}

// Count returns the number of stored user info entries
func (r *InMemoryUserInfoRepo) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.store)
}

// Clear removes all stored user info
func (r *InMemoryUserInfoRepo) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store = make(map[string]UserInfo)
}
