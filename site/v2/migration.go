// Package v2 provides migration utilities for backward compatibility
package v2

import (
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
)

// OldDownloaderConfig represents the old downloader configuration format
// with autoStart field that needs to be converted to AddAtPaused
type OldDownloaderConfig struct {
	Type      string `json:"type"`      // "qbittorrent", "transmission"
	Name      string `json:"name"`      // Downloader name
	URL       string `json:"url"`       // Downloader URL
	Username  string `json:"username"`  // Username for authentication
	Password  string `json:"password"`  // Password for authentication
	AutoStart bool   `json:"autoStart"` // Old field: true = start immediately
}

// NewDownloaderConfig represents the new downloader configuration format
// with AddAtPaused field for controlling torrent start behavior
type NewDownloaderConfig struct {
	Type        string `json:"type"`        // "qbittorrent", "transmission"
	Name        string `json:"name"`        // Downloader name
	URL         string `json:"url"`         // Downloader URL
	Username    string `json:"username"`    // Username for authentication
	Password    string `json:"password"`    // Password for authentication
	AddAtPaused bool   `json:"addAtPaused"` // New field: true = add paused
}

// OldSiteConfig represents the old site configuration format
type OldSiteConfig struct {
	Name       string `json:"name"`
	Type       string `json:"type"` // "nexusphp", "mteam", etc.
	URL        string `json:"url"`
	Cookie     string `json:"cookie,omitempty"`
	APIKey     string `json:"apiKey,omitempty"`
	RateLimit  int    `json:"rateLimit,omitempty"`
	Selectors  any    `json:"selectors,omitempty"`
	AuthMethod string `json:"authMethod,omitempty"` // Old field name
}

// ConfigMigrator handles migration from old configuration formats to new ones
type ConfigMigrator struct {
	logger *zap.Logger
}

// NewConfigMigrator creates a new ConfigMigrator
func NewConfigMigrator(logger *zap.Logger) *ConfigMigrator {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ConfigMigrator{logger: logger}
}

// AutoStartToAddAtPaused converts the old autoStart field to the new AddAtPaused field
// The mapping is: autoStart=true → AddAtPaused=false (start immediately)
//
//	autoStart=false → AddAtPaused=true (add paused)
func AutoStartToAddAtPaused(autoStart bool) bool {
	return !autoStart
}

// AddAtPausedToAutoStart converts the new AddAtPaused field back to the old autoStart field
// This is the inverse of AutoStartToAddAtPaused
func AddAtPausedToAutoStart(addAtPaused bool) bool {
	return !addAtPaused
}

// MigrateDownloaderConfig converts an old downloader config to the new format
func (m *ConfigMigrator) MigrateDownloaderConfig(old OldDownloaderConfig) NewDownloaderConfig {
	m.logger.Info("Migrating downloader config",
		zap.String("name", old.Name),
		zap.Bool("autoStart", old.AutoStart),
	)

	return NewDownloaderConfig{
		Type:        old.Type,
		Name:        old.Name,
		URL:         old.URL,
		Username:    old.Username,
		Password:    old.Password,
		AddAtPaused: AutoStartToAddAtPaused(old.AutoStart),
	}
}

// MigrateDownloaderConfigJSON converts old downloader config JSON to new format
func (m *ConfigMigrator) MigrateDownloaderConfigJSON(oldJSON []byte) ([]byte, error) {
	var old OldDownloaderConfig
	if err := json.Unmarshal(oldJSON, &old); err != nil {
		return nil, fmt.Errorf("parse old downloader config: %w", err)
	}

	newConfig := m.MigrateDownloaderConfig(old)
	return json.Marshal(newConfig)
}

// MigrateSiteConfig converts an old site config to the new SiteConfig format
func (m *ConfigMigrator) MigrateSiteConfig(old OldSiteConfig) SiteConfig {
	m.logger.Info("Migrating site config",
		zap.String("name", old.Name),
		zap.String("type", old.Type),
	)

	// Determine site type
	siteType := old.Type
	if siteType == "" {
		// Try to infer from other fields
		if old.APIKey != "" && old.Cookie == "" {
			siteType = "mtorrent"
		} else {
			siteType = "nexusphp"
		}
	}

	// Normalize type names
	switch siteType {
	case "mteam", "m-team":
		siteType = "mtorrent"
	}

	// Build options based on site type
	var options json.RawMessage
	switch siteType {
	case "nexusphp":
		opts := NexusPHPOptions{
			Cookie: old.Cookie,
		}
		if old.Selectors != nil {
			if selectors, ok := old.Selectors.(*SiteSelectors); ok {
				opts.Selectors = selectors
			}
		}
		options, _ = json.Marshal(opts)
	case "mtorrent":
		opts := MTorrentOptions{
			APIKey: old.APIKey,
		}
		options, _ = json.Marshal(opts)
	case "unit3d":
		opts := Unit3DOptions{
			APIKey: old.APIKey,
		}
		options, _ = json.Marshal(opts)
	case "gazelle":
		opts := GazelleOptions{
			APIKey: old.APIKey,
			Cookie: old.Cookie,
		}
		options, _ = json.Marshal(opts)
	}

	return SiteConfig{
		Type:      siteType,
		ID:        old.Name,
		Name:      old.Name,
		BaseURL:   old.URL,
		Options:   options,
		RateLimit: float64(old.RateLimit),
	}
}

// MigrateSiteConfigJSON converts old site config JSON to new format
func (m *ConfigMigrator) MigrateSiteConfigJSON(oldJSON []byte) ([]byte, error) {
	var old OldSiteConfig
	if err := json.Unmarshal(oldJSON, &old); err != nil {
		return nil, fmt.Errorf("parse old site config: %w", err)
	}

	newConfig := m.MigrateSiteConfig(old)
	return json.Marshal(newConfig)
}

// DetectConfigVersion attempts to detect if a config is in old or new format
// Returns "old" for old format, "new" for new format, or "unknown"
func (m *ConfigMigrator) DetectDownloaderConfigVersion(configJSON []byte) string {
	var data map[string]any
	if err := json.Unmarshal(configJSON, &data); err != nil {
		return "unknown"
	}

	// Check for new format field
	if _, hasAddAtPaused := data["addAtPaused"]; hasAddAtPaused {
		return "new"
	}

	// Check for old format field
	if _, hasAutoStart := data["autoStart"]; hasAutoStart {
		return "old"
	}

	// Check for auto_start (snake_case variant)
	if _, hasAutoStart := data["auto_start"]; hasAutoStart {
		return "old"
	}

	return "unknown"
}

// MigrateDownloaderConfigIfNeeded migrates config only if it's in old format
func (m *ConfigMigrator) MigrateDownloaderConfigIfNeeded(configJSON []byte) ([]byte, bool, error) {
	version := m.DetectDownloaderConfigVersion(configJSON)

	switch version {
	case "new":
		// Already in new format, no migration needed
		return configJSON, false, nil
	case "old":
		// Migrate to new format
		newJSON, err := m.MigrateDownloaderConfigJSON(configJSON)
		if err != nil {
			return nil, false, err
		}
		m.logger.Warn("Migrated old downloader config format to new format")
		return newJSON, true, nil
	default:
		// Unknown format, return as-is
		return configJSON, false, nil
	}
}

// DeprecationWarning represents a deprecation warning message
type DeprecationWarning struct {
	Field       string `json:"field"`
	Message     string `json:"message"`
	Replacement string `json:"replacement"`
	Version     string `json:"version"` // Version when it will be removed
}

// GetDeprecationWarnings returns a list of deprecation warnings for old config fields
func GetDeprecationWarnings() []DeprecationWarning {
	return []DeprecationWarning{
		{
			Field:       "autoStart",
			Message:     "The 'autoStart' field is deprecated",
			Replacement: "Use 'addAtPaused' instead (note: logic is inverted)",
			Version:     "2.0.0",
		},
		{
			Field:       "auto_start",
			Message:     "The 'auto_start' field is deprecated",
			Replacement: "Use 'addAtPaused' instead (note: logic is inverted)",
			Version:     "2.0.0",
		},
		{
			Field:       "authMethod",
			Message:     "The 'authMethod' field in site config is deprecated",
			Replacement: "Authentication method is now inferred from the options provided",
			Version:     "2.0.0",
		},
	}
}

// CheckForDeprecatedFields checks a config JSON for deprecated fields and logs warnings
func (m *ConfigMigrator) CheckForDeprecatedFields(configJSON []byte) []DeprecationWarning {
	var data map[string]any
	if err := json.Unmarshal(configJSON, &data); err != nil {
		return nil
	}

	var warnings []DeprecationWarning
	allWarnings := GetDeprecationWarnings()

	for _, warning := range allWarnings {
		if _, exists := data[warning.Field]; exists {
			m.logger.Warn("Deprecated field detected",
				zap.String("field", warning.Field),
				zap.String("message", warning.Message),
				zap.String("replacement", warning.Replacement),
			)
			warnings = append(warnings, warning)
		}
	}

	return warnings
}

// SiteMigrationGuide provides migration guidance for each site type
type SiteMigrationGuide struct {
	SiteType        string   `json:"siteType"`
	OldFormat       string   `json:"oldFormat"`
	NewFormat       string   `json:"newFormat"`
	Steps           []string `json:"steps"`
	Notes           []string `json:"notes,omitempty"`
	BreakingChanges []string `json:"breakingChanges,omitempty"`
}

// GetSiteMigrationGuides returns migration guides for all site types
func GetSiteMigrationGuides() []SiteMigrationGuide {
	return []SiteMigrationGuide{
		{
			SiteType: "nexusphp",
			OldFormat: `{
  "name": "hdsky",
  "type": "nexusphp",
  "url": "https://hdsky.me",
  "cookie": "your_cookie_here"
}`,
			NewFormat: `{
  "type": "nexusphp",
  "id": "hdsky",
  "name": "HDSky",
  "baseUrl": "https://hdsky.me",
  "options": {
    "cookie": "your_cookie_here",
    "selectors": {
      "tableRows": "table.torrents > tbody > tr",
      "title": "td.embedded > a"
    }
  }
}`,
			Steps: []string{
				"1. Change 'url' to 'baseUrl'",
				"2. Move 'cookie' into 'options' object",
				"3. Add 'id' field (use site name in lowercase)",
				"4. Optionally add custom 'selectors' for site-specific parsing",
			},
			Notes: []string{
				"The 'selectors' field is optional - default selectors work for most NexusPHP sites",
				"Custom selectors can be used for sites with non-standard HTML structure",
			},
		},
		{
			SiteType: "mtorrent",
			OldFormat: `{
  "name": "mteam",
  "type": "mteam",
  "url": "https://api.m-team.cc",
  "apiKey": "your_api_key_here"
}`,
			NewFormat: `{
  "type": "mtorrent",
  "id": "mteam",
  "name": "M-Team",
  "baseUrl": "https://api.m-team.cc",
  "options": {
    "apiKey": "your_api_key_here"
  }
}`,
			Steps: []string{
				"1. Change 'type' from 'mteam' to 'mtorrent'",
				"2. Change 'url' to 'baseUrl'",
				"3. Move 'apiKey' into 'options' object",
				"4. Add 'id' field",
			},
			BreakingChanges: []string{
				"The type name changed from 'mteam' to 'mtorrent'",
			},
		},
		{
			SiteType: "unit3d",
			OldFormat: `{
  "name": "blutopia",
  "type": "unit3d",
  "url": "https://blutopia.cc",
  "apiKey": "your_api_key_here"
}`,
			NewFormat: `{
  "type": "unit3d",
  "id": "blutopia",
  "name": "Blutopia",
  "baseUrl": "https://blutopia.cc",
  "options": {
    "apiKey": "your_api_key_here"
  }
}`,
			Steps: []string{
				"1. Change 'url' to 'baseUrl'",
				"2. Move 'apiKey' into 'options' object",
				"3. Add 'id' field",
			},
		},
		{
			SiteType: "gazelle",
			OldFormat: `{
  "name": "redacted",
  "type": "gazelle",
  "url": "https://redacted.ch",
  "apiKey": "your_api_key_here"
}`,
			NewFormat: `{
  "type": "gazelle",
  "id": "redacted",
  "name": "Redacted",
  "baseUrl": "https://redacted.ch",
  "options": {
    "apiKey": "your_api_key_here"
  }
}`,
			Steps: []string{
				"1. Change 'url' to 'baseUrl'",
				"2. Move 'apiKey' (or 'cookie') into 'options' object",
				"3. Add 'id' field",
			},
			Notes: []string{
				"Gazelle sites can use either apiKey or cookie for authentication",
				"If using cookie, add it to options instead of apiKey",
			},
		},
	}
}

// SiteAdapter provides an adapter layer to use old site implementations with new interface
// This allows gradual migration of site implementations
type SiteAdapter struct {
	// OldSite is the old site implementation (if any)
	OldSite any
	// NewSite is the new v2 site implementation
	NewSite Site
	// UseNew indicates whether to use the new implementation
	UseNew bool
}

// NewSiteAdapter creates a new SiteAdapter
// If newSite is provided and useNew is true, it will be used
// Otherwise, oldSite will be used (for backward compatibility)
func NewSiteAdapter(oldSite any, newSite Site, useNew bool) *SiteAdapter {
	return &SiteAdapter{
		OldSite: oldSite,
		NewSite: newSite,
		UseNew:  useNew,
	}
}

// GetSite returns the appropriate site implementation based on configuration
func (a *SiteAdapter) GetSite() any {
	if a.UseNew && a.NewSite != nil {
		return a.NewSite
	}
	return a.OldSite
}

// IsUsingNewImplementation returns true if using the new v2 implementation
func (a *SiteAdapter) IsUsingNewImplementation() bool {
	return a.UseNew && a.NewSite != nil
}

// MigrationStatus represents the migration status of a site
type MigrationStatus struct {
	SiteID            string `json:"siteId"`
	SiteName          string `json:"siteName"`
	OldImplementation bool   `json:"oldImplementation"`
	NewImplementation bool   `json:"newImplementation"`
	MigrationReady    bool   `json:"migrationReady"`
	Notes             string `json:"notes,omitempty"`
}

// SiteMigrationManager manages the migration of sites from old to new implementation
type SiteMigrationManager struct {
	adapters map[string]*SiteAdapter
	logger   *zap.Logger
}

// NewSiteMigrationManager creates a new SiteMigrationManager
func NewSiteMigrationManager(logger *zap.Logger) *SiteMigrationManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SiteMigrationManager{
		adapters: make(map[string]*SiteAdapter),
		logger:   logger,
	}
}

// RegisterSite registers a site with both old and new implementations
func (m *SiteMigrationManager) RegisterSite(siteID string, oldSite any, newSite Site, useNew bool) {
	m.adapters[siteID] = NewSiteAdapter(oldSite, newSite, useNew)
	m.logger.Info("Registered site for migration",
		zap.String("siteId", siteID),
		zap.Bool("useNew", useNew),
	)
}

// GetSite returns the site implementation for the given ID
func (m *SiteMigrationManager) GetSite(siteID string) any {
	if adapter, ok := m.adapters[siteID]; ok {
		return adapter.GetSite()
	}
	return nil
}

// GetNewSite returns the new v2 site implementation if available
func (m *SiteMigrationManager) GetNewSite(siteID string) Site {
	if adapter, ok := m.adapters[siteID]; ok {
		return adapter.NewSite
	}
	return nil
}

// MigrateToNew switches a site to use the new implementation
func (m *SiteMigrationManager) MigrateToNew(siteID string) error {
	adapter, ok := m.adapters[siteID]
	if !ok {
		return fmt.Errorf("site not found: %s", siteID)
	}
	if adapter.NewSite == nil {
		return fmt.Errorf("new implementation not available for site: %s", siteID)
	}
	adapter.UseNew = true
	m.logger.Info("Migrated site to new implementation",
		zap.String("siteId", siteID),
	)
	return nil
}

// RollbackToOld switches a site back to the old implementation
func (m *SiteMigrationManager) RollbackToOld(siteID string) error {
	adapter, ok := m.adapters[siteID]
	if !ok {
		return fmt.Errorf("site not found: %s", siteID)
	}
	if adapter.OldSite == nil {
		return fmt.Errorf("old implementation not available for site: %s", siteID)
	}
	adapter.UseNew = false
	m.logger.Info("Rolled back site to old implementation",
		zap.String("siteId", siteID),
	)
	return nil
}

// GetMigrationStatus returns the migration status of all registered sites
func (m *SiteMigrationManager) GetMigrationStatus() []MigrationStatus {
	var statuses []MigrationStatus
	for siteID, adapter := range m.adapters {
		status := MigrationStatus{
			SiteID:            siteID,
			OldImplementation: adapter.OldSite != nil,
			NewImplementation: adapter.NewSite != nil,
			MigrationReady:    adapter.NewSite != nil,
		}
		if adapter.NewSite != nil {
			status.SiteName = adapter.NewSite.Name()
		}
		if adapter.UseNew {
			status.Notes = "Using new v2 implementation"
		} else {
			status.Notes = "Using old implementation"
		}
		statuses = append(statuses, status)
	}
	return statuses
}
