// Package v2 provides site registry for managing site metadata and creation
package v2

import (
	"encoding/json"
	"fmt"
	"sync"

	"go.uber.org/zap"
)

// SiteMeta contains metadata for a site type
type SiteMeta struct {
	// ID is the unique site identifier (e.g., "mteam", "hdsky")
	ID string
	// Name is the human-readable site name
	Name string
	// Kind is the site architecture type
	Kind SiteKind
	// DefaultBaseURL is the default base URL for the site
	DefaultBaseURL string
	// AuthMethod is the authentication method (cookie, api_key)
	AuthMethod string
	// RateLimit is the default rate limit (requests per second)
	RateLimit float64
	// RateBurst is the default rate burst
	RateBurst int
}

// SiteRegistry manages site metadata and creation
type SiteRegistry struct {
	mu      sync.RWMutex
	sites   map[string]SiteMeta
	factory *SiteFactory
	logger  *zap.Logger
}

// NewSiteRegistry creates a new SiteRegistry with default site metadata
func NewSiteRegistry(logger *zap.Logger) *SiteRegistry {
	if logger == nil {
		logger = zap.NewNop()
	}

	registry := &SiteRegistry{
		sites:   make(map[string]SiteMeta),
		factory: NewSiteFactory(logger),
		logger:  logger,
	}

	// Register default sites
	registry.registerDefaults()

	return registry
}

// registerDefaults registers the default site metadata
func (r *SiteRegistry) registerDefaults() {
	// M-Team (uses MTorrent API)
	r.Register(SiteMeta{
		ID:             "mteam",
		Name:           "M-Team",
		Kind:           SiteMTorrent,
		DefaultBaseURL: "https://api.m-team.cc",
		AuthMethod:     "api_key",
		RateLimit:      1.0,
		RateBurst:      3,
	})

	// HDSky (NexusPHP)
	r.Register(SiteMeta{
		ID:             "hdsky",
		Name:           "HDSky",
		Kind:           SiteNexusPHP,
		DefaultBaseURL: "https://hdsky.me",
		AuthMethod:     "cookie",
		RateLimit:      2.0, // Increased from 0.5 to allow faster searches
		RateBurst:      5,   // Increased from 2 for initial burst
	})

	// SpringSunday (NexusPHP)
	r.Register(SiteMeta{
		ID:             "springsunday",
		Name:           "SpringSunday",
		Kind:           SiteNexusPHP,
		DefaultBaseURL: "https://springsunday.net",
		AuthMethod:     "cookie",
		RateLimit:      2.0, // Increased from 0.5
		RateBurst:      5,   // Increased from 2
	})

	// HDDolby (NexusPHP)
	r.Register(SiteMeta{
		ID:             "hddolby",
		Name:           "HDDolby",
		Kind:           SiteNexusPHP,
		DefaultBaseURL: "https://www.hddolby.com",
		AuthMethod:     "cookie",
		RateLimit:      2.0, // Increased from 0.5
		RateBurst:      5,   // Increased from 2
	})
}

// Register adds or updates site metadata
func (r *SiteRegistry) Register(meta SiteMeta) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sites[meta.ID] = meta
	r.logger.Debug("Registered site metadata", zap.String("id", meta.ID), zap.String("kind", string(meta.Kind)))
}

// Get returns site metadata by ID
func (r *SiteRegistry) Get(id string) (SiteMeta, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	meta, ok := r.sites[id]
	return meta, ok
}

// List returns all registered site IDs
func (r *SiteRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.sites))
	for id := range r.sites {
		ids = append(ids, id)
	}
	return ids
}

// SiteCredentials holds authentication credentials for a site
type SiteCredentials struct {
	Cookie string
	APIKey string
}

// CreateSite creates a Site instance from registry metadata and credentials
func (r *SiteRegistry) CreateSite(siteID string, creds SiteCredentials, customBaseURL string) (Site, error) {
	meta, ok := r.Get(siteID)
	if !ok {
		return nil, fmt.Errorf("site %s not found in registry", siteID)
	}

	// Determine base URL
	baseURL := customBaseURL
	if baseURL == "" {
		baseURL = meta.DefaultBaseURL
	}
	if baseURL == "" {
		return nil, fmt.Errorf("no base URL available for site %s", siteID)
	}

	// Build options based on site kind
	var options []byte
	var err error

	switch meta.Kind {
	case SiteMTorrent:
		if creds.APIKey == "" {
			return nil, fmt.Errorf("site %s requires API key", siteID)
		}
		options, err = json.Marshal(MTorrentOptions{APIKey: creds.APIKey})
	case SiteNexusPHP:
		if creds.Cookie == "" {
			return nil, fmt.Errorf("site %s requires cookie", siteID)
		}
		options, err = json.Marshal(NexusPHPOptions{Cookie: creds.Cookie})
	case SiteUnit3D:
		if creds.APIKey == "" {
			return nil, fmt.Errorf("site %s requires API key", siteID)
		}
		options, err = json.Marshal(Unit3DOptions{APIKey: creds.APIKey})
	case SiteGazelle:
		if creds.APIKey == "" && creds.Cookie == "" {
			return nil, fmt.Errorf("site %s requires API key or cookie", siteID)
		}
		options, err = json.Marshal(GazelleOptions{APIKey: creds.APIKey, Cookie: creds.Cookie})
	default:
		return nil, fmt.Errorf("unsupported site kind: %s", meta.Kind)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to marshal options for site %s: %w", siteID, err)
	}

	// Create site using factory
	return r.factory.CreateSite(SiteConfig{
		Type:      string(meta.Kind),
		ID:        meta.ID,
		Name:      meta.Name,
		BaseURL:   baseURL,
		Options:   options,
		RateLimit: meta.RateLimit,
		RateBurst: meta.RateBurst,
	})
}

// GetSiteKind returns the site kind for a given site ID
func (r *SiteRegistry) GetSiteKind(siteID string) (SiteKind, bool) {
	meta, ok := r.Get(siteID)
	if !ok {
		return "", false
	}
	return meta.Kind, true
}

// GetDefaultBaseURL returns the default base URL for a given site ID
func (r *SiteRegistry) GetDefaultBaseURL(siteID string) (string, bool) {
	meta, ok := r.Get(siteID)
	if !ok {
		return "", false
	}
	return meta.DefaultBaseURL, true
}
