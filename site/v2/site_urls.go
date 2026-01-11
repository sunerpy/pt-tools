package v2

import (
	"sync"

	"go.uber.org/zap"
)

// SiteName represents a PT site identifier
type SiteName string

// Site name constants
const (
	SiteNameMTeam        SiteName = "mteam"
	SiteNameHDSky        SiteName = "hdsky"
	SiteNameSpringSunday SiteName = "springsunday"
	SiteNameCHDBits      SiteName = "chdbits"
	SiteNameTTG          SiteName = "ttg"
	SiteNameOurBits      SiteName = "ourbits"
	SiteNamePterClub     SiteName = "pterclub"
	SiteNameAudiences    SiteName = "audiences"
)

// String returns the string representation of SiteName
func (s SiteName) String() string {
	return string(s)
}

// DefaultSiteURLs contains the default base URLs for each site
// Sites can have multiple URLs for failover purposes
var DefaultSiteURLs = map[SiteName][]string{
	SiteNameMTeam: {
		"https://api.m-team.cc",
		"https://kp.m-team.cc",
		"https://pt.m-team.cc",
	},
	SiteNameHDSky: {
		"https://hdsky.me",
	},
	SiteNameSpringSunday: {
		"https://springsunday.net",
	},
	SiteNameCHDBits: {
		"https://chdbits.co",
	},
	SiteNameTTG: {
		"https://totheglory.im",
	},
	SiteNameOurBits: {
		"https://ourbits.club",
	},
	SiteNamePterClub: {
		"https://pterclub.com",
	},
	SiteNameAudiences: {
		"https://audiences.me",
	},
}

// SiteURLRegistry manages site URL configurations
// It provides a centralized way to register and retrieve site URLs
type SiteURLRegistry struct {
	urls   map[SiteName][]string
	mu     sync.RWMutex
	logger *zap.Logger
}

// globalRegistry is the default global registry instance
var (
	globalRegistry *SiteURLRegistry
	registryOnce   sync.Once
)

// GetGlobalRegistry returns the global site URL registry
func GetGlobalRegistry() *SiteURLRegistry {
	registryOnce.Do(func() {
		globalRegistry = NewSiteURLRegistry(nil)
		// Initialize with default URLs
		for site, urls := range DefaultSiteURLs {
			globalRegistry.RegisterURLs(site, urls)
		}
	})
	return globalRegistry
}

// NewSiteURLRegistry creates a new site URL registry
func NewSiteURLRegistry(logger *zap.Logger) *SiteURLRegistry {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SiteURLRegistry{
		urls:   make(map[SiteName][]string),
		logger: logger,
	}
}

// GetURLs returns the URL list for a site
// Returns nil if the site is not registered
func (r *SiteURLRegistry) GetURLs(siteName SiteName) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	urls, ok := r.urls[siteName]
	if !ok {
		return nil
	}
	// Return a copy to prevent modification
	result := make([]string, len(urls))
	copy(result, urls)
	return result
}

// RegisterURLs registers or updates URLs for a site
func (r *SiteURLRegistry) RegisterURLs(siteName SiteName, urls []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Store a copy to prevent external modification
	urlsCopy := make([]string, len(urls))
	copy(urlsCopy, urls)
	r.urls[siteName] = urlsCopy
	r.logger.Debug("Registered site URLs",
		zap.String("site", siteName.String()),
		zap.Strings("urls", urlsCopy),
	)
}

// GetFailoverClient creates a FailoverHTTPClient for the specified site
func (r *SiteURLRegistry) GetFailoverClient(siteName SiteName, opts ...FailoverOption) (*FailoverHTTPClient, error) {
	urls := r.GetURLs(siteName)
	if len(urls) == 0 {
		return nil, ErrNoURLsConfigured
	}
	config := DefaultFailoverConfig(urls)
	return NewFailoverHTTPClient(config, opts...), nil
}

// GetFailoverConfig returns a failover config for the specified site
func (r *SiteURLRegistry) GetFailoverConfig(siteName SiteName) (URLFailoverConfig, error) {
	urls := r.GetURLs(siteName)
	if len(urls) == 0 {
		return URLFailoverConfig{}, ErrNoURLsConfigured
	}
	return DefaultFailoverConfig(urls), nil
}

// ListSites returns all registered site names
func (r *SiteURLRegistry) ListSites() []SiteName {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sites := make([]SiteName, 0, len(r.urls))
	for site := range r.urls {
		sites = append(sites, site)
	}
	return sites
}

// HasSite checks if a site is registered
func (r *SiteURLRegistry) HasSite(siteName SiteName) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.urls[siteName]
	return ok
}

// SiteKindMap maps site names to their architecture kinds
var SiteKindMap = map[SiteName]SiteKind{
	SiteNameMTeam:        SiteMTorrent,
	SiteNameHDSky:        SiteNexusPHP,
	SiteNameSpringSunday: SiteNexusPHP,
	SiteNameCHDBits:      SiteNexusPHP,
	SiteNameTTG:          SiteNexusPHP,
	SiteNameOurBits:      SiteNexusPHP,
	SiteNamePterClub:     SiteNexusPHP,
	SiteNameAudiences:    SiteNexusPHP,
}

// GetSiteKind returns the architecture kind for a site
func GetSiteKind(siteName SiteName) SiteKind {
	if kind, ok := SiteKindMap[siteName]; ok {
		return kind
	}
	return SiteNexusPHP // Default to NexusPHP
}

// GetSiteURLsForKind returns URLs for sites of a specific kind
// This is a convenience function for getting URLs by site architecture type
func GetSiteURLsForKind(kind SiteKind) map[SiteName][]string {
	registry := GetGlobalRegistry()
	result := make(map[SiteName][]string)

	for site, siteKind := range SiteKindMap {
		if siteKind == kind {
			if urls := registry.GetURLs(site); urls != nil {
				result[site] = urls
			}
		}
	}

	return result
}
