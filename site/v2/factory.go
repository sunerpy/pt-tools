package v2

import (
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
)

// mergeSelectors merges src selectors into dst, only overwriting non-empty fields
func mergeSelectors(dst, src *SiteSelectors) {
	if src == nil || dst == nil {
		return
	}
	if src.TableRows != "" {
		dst.TableRows = src.TableRows
	}
	if src.Title != "" {
		dst.Title = src.Title
	}
	if src.TitleLink != "" {
		dst.TitleLink = src.TitleLink
	}
	if src.Size != "" {
		dst.Size = src.Size
	}
	if src.Seeders != "" {
		dst.Seeders = src.Seeders
	}
	if src.Leechers != "" {
		dst.Leechers = src.Leechers
	}
	if src.Snatched != "" {
		dst.Snatched = src.Snatched
	}
	if src.DiscountIcon != "" {
		dst.DiscountIcon = src.DiscountIcon
	}
	if src.DiscountEndTime != "" {
		dst.DiscountEndTime = src.DiscountEndTime
	}
	if src.DownloadLink != "" {
		dst.DownloadLink = src.DownloadLink
	}
	if src.Category != "" {
		dst.Category = src.Category
	}
	if src.UploadTime != "" {
		dst.UploadTime = src.UploadTime
	}
	if src.HRIcon != "" {
		dst.HRIcon = src.HRIcon
	}
	if src.Subtitle != "" {
		dst.Subtitle = src.Subtitle
	}
	if src.UserInfoUsername != "" {
		dst.UserInfoUsername = src.UserInfoUsername
	}
	if src.UserInfoUploaded != "" {
		dst.UserInfoUploaded = src.UserInfoUploaded
	}
	if src.UserInfoDownloaded != "" {
		dst.UserInfoDownloaded = src.UserInfoDownloaded
	}
	if src.UserInfoRatio != "" {
		dst.UserInfoRatio = src.UserInfoRatio
	}
	if src.UserInfoBonus != "" {
		dst.UserInfoBonus = src.UserInfoBonus
	}
	if src.UserInfoRank != "" {
		dst.UserInfoRank = src.UserInfoRank
	}
	if src.DetailDownloadLink != "" {
		dst.DetailDownloadLink = src.DetailDownloadLink
	}
	if src.DetailSubtitle != "" {
		dst.DetailSubtitle = src.DetailSubtitle
	}
}

// SiteConfig holds configuration for creating a site
type SiteConfig struct {
	// Type is the site type (nexusphp, unit3d, gazelle, mtorrent)
	Type string `json:"type"`
	// ID is the unique site identifier
	ID string `json:"id"`
	// Name is the human-readable site name
	Name string `json:"name"`
	// BaseURL is the site's base URL
	BaseURL string `json:"baseUrl"`
	// Options contains type-specific configuration
	Options json.RawMessage `json:"options"`
	// RateLimit is the requests per second limit (optional)
	RateLimit float64 `json:"rateLimit,omitempty"`
	// RateBurst is the maximum burst size (optional)
	RateBurst int `json:"rateBurst,omitempty"`
}

// NexusPHPOptions holds NexusPHP-specific configuration
type NexusPHPOptions struct {
	Cookie    string         `json:"cookie"`
	Selectors *SiteSelectors `json:"selectors,omitempty"`
}

// MTorrentOptions holds M-Team-specific configuration
type MTorrentOptions struct {
	APIKey string `json:"apiKey"`
}

// HDDolbyOptions holds HDDolby-specific configuration
type HDDolbyOptions struct {
	APIKey string `json:"apiKey"`
	Cookie string `json:"cookie"`
}

// Unit3DOptions holds Unit3D-specific configuration
type Unit3DOptions struct {
	APIKey string `json:"apiKey"`
}

// GazelleOptions holds Gazelle-specific configuration
type GazelleOptions struct {
	APIKey string `json:"apiKey,omitempty"`
	Cookie string `json:"cookie,omitempty"`
}

// SiteFactory creates Site instances from configuration
type SiteFactory struct {
	logger *zap.Logger
}

// NewSiteFactory creates a new SiteFactory
func NewSiteFactory(logger *zap.Logger) *SiteFactory {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SiteFactory{logger: logger}
}

// CreateSite creates a Site from configuration
func (f *SiteFactory) CreateSite(config SiteConfig) (Site, error) {
	if config.ID == "" {
		return nil, fmt.Errorf("site ID is required")
	}
	if config.Name == "" {
		config.Name = config.ID
	}
	if config.BaseURL == "" {
		return nil, fmt.Errorf("site baseUrl is required")
	}

	kind := SiteKind(config.Type)

	switch kind {
	case SiteNexusPHP:
		return f.createNexusPHPSite(config)
	case SiteMTorrent:
		return f.createMTorrentSite(config)
	case SiteUnit3D:
		return f.createUnit3DSite(config)
	case SiteGazelle:
		return f.createGazelleSite(config)
	case SiteHDDolby:
		return f.createHDDolbySite(config)
	default:
		return nil, fmt.Errorf("unsupported site type: %s", config.Type)
	}
}

// createNexusPHPSite creates a NexusPHP site
func (f *SiteFactory) createNexusPHPSite(config SiteConfig) (Site, error) {
	var opts NexusPHPOptions
	if len(config.Options) > 0 {
		if err := json.Unmarshal(config.Options, &opts); err != nil {
			return nil, fmt.Errorf("parse NexusPHP options: %w", err)
		}
	}

	if opts.Cookie == "" {
		return nil, fmt.Errorf("NexusPHP site requires cookie")
	}

	// Debug: list all registered definitions
	registry := GetDefinitionRegistry()
	allDefs := registry.List()
	f.logger.Info("Registered site definitions",
		zap.Strings("definitions", allDefs),
		zap.String("lookingFor", config.ID),
	)

	// Try to load site definition from registry
	siteDef := registry.GetOrDefault(config.ID)
	f.logger.Info("Loading site definition",
		zap.String("siteID", config.ID),
		zap.Bool("found", siteDef != nil),
		zap.Bool("hasUserInfo", siteDef != nil && siteDef.UserInfo != nil),
	)

	// Start with default selectors, then merge any custom selectors
	selectors := DefaultNexusPHPSelectors()
	if opts.Selectors != nil {
		mergeSelectors(&selectors, opts.Selectors)
	}
	if siteDef != nil && siteDef.Selectors != nil {
		mergeSelectors(&selectors, siteDef.Selectors)
	}

	driver := NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL:   config.BaseURL,
		Cookie:    opts.Cookie,
		Selectors: &selectors,
	})

	// Set site definition on driver if available
	if siteDef != nil {
		driver.SetSiteDefinition(siteDef)
		f.logger.Info("Set site definition on driver",
			zap.String("siteID", config.ID),
			zap.String("defID", siteDef.ID),
		)
	}

	site := NewBaseSite(driver, BaseSiteConfig{
		ID:        config.ID,
		Name:      config.Name,
		Kind:      SiteNexusPHP,
		RateLimit: config.RateLimit,
		RateBurst: config.RateBurst,
		Logger:    f.logger.With(zap.String("site", config.ID)),
	})

	return site, nil
}

// createMTorrentSite creates an M-Team site
func (f *SiteFactory) createMTorrentSite(config SiteConfig) (Site, error) {
	var opts MTorrentOptions
	if len(config.Options) > 0 {
		if err := json.Unmarshal(config.Options, &opts); err != nil {
			return nil, fmt.Errorf("parse MTorrent options: %w", err)
		}
	}

	if opts.APIKey == "" {
		return nil, fmt.Errorf("MTorrent site requires apiKey")
	}

	// Try to load site definition from registry
	siteDef := GetDefinitionRegistry().GetOrDefault(config.ID)

	driver := NewMTorrentDriver(MTorrentDriverConfig{
		BaseURL: config.BaseURL,
		APIKey:  opts.APIKey,
	})

	// Set site definition on driver if available
	if siteDef != nil {
		driver.SetSiteDefinition(siteDef)
	}

	site := NewBaseSite(driver, BaseSiteConfig{
		ID:        config.ID,
		Name:      config.Name,
		Kind:      SiteMTorrent,
		RateLimit: config.RateLimit,
		RateBurst: config.RateBurst,
		Logger:    f.logger.With(zap.String("site", config.ID)),
	})

	return site, nil
}

func (f *SiteFactory) createHDDolbySite(config SiteConfig) (Site, error) {
	var opts HDDolbyOptions
	if len(config.Options) > 0 {
		if err := json.Unmarshal(config.Options, &opts); err != nil {
			return nil, fmt.Errorf("parse HDDolby options: %w", err)
		}
	}

	if opts.APIKey == "" {
		return nil, fmt.Errorf("HDDolby 站点需要配置 RSS Key（从站点 RSS 订阅页面获取）")
	}

	if opts.Cookie == "" {
		return nil, fmt.Errorf("HDDolby 站点需要配置 Cookie（用于获取时魔等信息）")
	}

	siteDef := GetDefinitionRegistry().GetOrDefault(config.ID)

	driver := NewHDDolbyDriver(HDDolbyDriverConfig{
		BaseURL: config.BaseURL,
		APIKey:  opts.APIKey,
		Cookie:  opts.Cookie,
	})

	if siteDef != nil {
		driver.SetSiteDefinition(siteDef)
	}

	site := NewBaseSite(driver, BaseSiteConfig{
		ID:        config.ID,
		Name:      config.Name,
		Kind:      SiteHDDolby,
		RateLimit: config.RateLimit,
		RateBurst: config.RateBurst,
		Logger:    f.logger.With(zap.String("site", config.ID)),
	})

	return site, nil
}

// createUnit3DSite creates a Unit3D site
func (f *SiteFactory) createUnit3DSite(config SiteConfig) (Site, error) {
	var opts Unit3DOptions
	if len(config.Options) > 0 {
		if err := json.Unmarshal(config.Options, &opts); err != nil {
			return nil, fmt.Errorf("parse Unit3D options: %w", err)
		}
	}

	if opts.APIKey == "" {
		return nil, fmt.Errorf("Unit3D site requires apiKey")
	}

	driver := NewUnit3DDriver(Unit3DDriverConfig{
		BaseURL: config.BaseURL,
		APIKey:  opts.APIKey,
	})

	site := NewBaseSite(driver, BaseSiteConfig{
		ID:        config.ID,
		Name:      config.Name,
		Kind:      SiteUnit3D,
		RateLimit: config.RateLimit,
		RateBurst: config.RateBurst,
		Logger:    f.logger.With(zap.String("site", config.ID)),
	})

	return site, nil
}

// createGazelleSite creates a Gazelle site
func (f *SiteFactory) createGazelleSite(config SiteConfig) (Site, error) {
	var opts GazelleOptions
	if len(config.Options) > 0 {
		if err := json.Unmarshal(config.Options, &opts); err != nil {
			return nil, fmt.Errorf("parse Gazelle options: %w", err)
		}
	}

	if opts.APIKey == "" && opts.Cookie == "" {
		return nil, fmt.Errorf("Gazelle site requires apiKey or cookie")
	}

	driver := NewGazelleDriver(GazelleDriverConfig{
		BaseURL: config.BaseURL,
		APIKey:  opts.APIKey,
		Cookie:  opts.Cookie,
	})

	site := NewBaseSite(driver, BaseSiteConfig{
		ID:        config.ID,
		Name:      config.Name,
		Kind:      SiteGazelle,
		RateLimit: config.RateLimit,
		RateBurst: config.RateBurst,
		Logger:    f.logger.With(zap.String("site", config.ID)),
	})

	return site, nil
}

// CreateSiteFromJSON creates a Site from JSON configuration
func (f *SiteFactory) CreateSiteFromJSON(jsonData []byte) (Site, error) {
	var config SiteConfig
	if err := json.Unmarshal(jsonData, &config); err != nil {
		return nil, fmt.Errorf("parse site config: %w", err)
	}
	return f.CreateSite(config)
}

// CreateSitesFromJSON creates multiple Sites from JSON array configuration
func (f *SiteFactory) CreateSitesFromJSON(jsonData []byte) ([]Site, error) {
	var configs []SiteConfig
	if err := json.Unmarshal(jsonData, &configs); err != nil {
		return nil, fmt.Errorf("parse site configs: %w", err)
	}

	sites := make([]Site, 0, len(configs))
	for _, config := range configs {
		site, err := f.CreateSite(config)
		if err != nil {
			f.logger.Warn("Failed to create site",
				zap.String("id", config.ID),
				zap.Error(err),
			)
			continue
		}
		sites = append(sites, site)
	}

	return sites, nil
}
