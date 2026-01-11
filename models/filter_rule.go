package models

import (
	"time"
)

// PatternType represents the type of pattern matching to use.
type PatternType string

const (
	// PatternKeyword matches if the title contains the keyword (case-insensitive).
	PatternKeyword PatternType = "keyword"
	// PatternWildcard uses * and ? wildcards for matching.
	PatternWildcard PatternType = "wildcard"
	// PatternRegex uses regular expressions for matching.
	PatternRegex PatternType = "regex"
)

// MatchField represents which fields to match against.
type MatchField string

const (
	// MatchFieldTitle matches only against the title field.
	MatchFieldTitle MatchField = "title"
	// MatchFieldTag matches only against the tag field.
	MatchFieldTag MatchField = "tag"
	// MatchFieldBoth matches against both title and tag fields (default).
	MatchFieldBoth MatchField = "both"
)

// FilterRule represents a user-defined filter rule for RSS items.
type FilterRule struct {
	ID          uint        `gorm:"primaryKey" json:"id"`
	Name        string      `gorm:"size:128;not null" json:"name"`
	Pattern     string      `gorm:"size:512;not null" json:"pattern"`
	PatternType PatternType `gorm:"size:16;not null;default:'keyword'" json:"pattern_type"`
	MatchField  MatchField  `gorm:"size:16;not null;default:'both'" json:"match_field"`
	RequireFree bool        `gorm:"default:true" json:"require_free"`
	Enabled     bool        `gorm:"default:true" json:"enabled"`
	SiteID      *uint       `gorm:"index" json:"site_id"`
	RSSID       *uint       `gorm:"index" json:"rss_id"`
	Priority    int         `gorm:"default:100" json:"priority"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// TableName returns the table name for FilterRule.
func (FilterRule) TableName() string {
	return "filter_rules"
}

// FilterRuleDB provides database operations for FilterRule.
type FilterRuleDB struct {
	db *TorrentDB
}

// NewFilterRuleDB creates a new FilterRuleDB.
func NewFilterRuleDB(db *TorrentDB) *FilterRuleDB {
	return &FilterRuleDB{db: db}
}

// Create creates a new filter rule.
func (f *FilterRuleDB) Create(rule *FilterRule) error {
	return f.db.DB.Create(rule).Error
}

// GetByID retrieves a filter rule by ID.
func (f *FilterRuleDB) GetByID(id uint) (*FilterRule, error) {
	var rule FilterRule
	err := f.db.DB.First(&rule, id).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// GetAll retrieves all filter rules.
func (f *FilterRuleDB) GetAll() ([]FilterRule, error) {
	var rules []FilterRule
	err := f.db.DB.Order("priority ASC, id ASC").Find(&rules).Error
	return rules, err
}

// GetEnabled retrieves all enabled filter rules ordered by priority.
func (f *FilterRuleDB) GetEnabled() ([]FilterRule, error) {
	var rules []FilterRule
	err := f.db.DB.Where("enabled = ?", true).Order("priority ASC, id ASC").Find(&rules).Error
	return rules, err
}

// GetBySiteID retrieves filter rules for a specific site.
func (f *FilterRuleDB) GetBySiteID(siteID uint) ([]FilterRule, error) {
	var rules []FilterRule
	err := f.db.DB.Where("site_id = ? OR site_id IS NULL", siteID).
		Where("enabled = ?", true).
		Order("priority ASC, id ASC").
		Find(&rules).Error
	return rules, err
}

// GetByRSSID retrieves filter rules for a specific RSS subscription.
func (f *FilterRuleDB) GetByRSSID(rssID uint) ([]FilterRule, error) {
	var rules []FilterRule
	err := f.db.DB.Where("rss_id = ? OR rss_id IS NULL", rssID).
		Where("enabled = ?", true).
		Order("priority ASC, id ASC").
		Find(&rules).Error
	return rules, err
}

// Update updates an existing filter rule.
func (f *FilterRuleDB) Update(rule *FilterRule) error {
	return f.db.DB.Save(rule).Error
}

// Delete deletes a filter rule by ID.
func (f *FilterRuleDB) Delete(id uint) error {
	return f.db.DB.Delete(&FilterRule{}, id).Error
}

// Exists checks if a filter rule with the given name exists.
func (f *FilterRuleDB) Exists(name string) (bool, error) {
	var count int64
	err := f.db.DB.Model(&FilterRule{}).Where("name = ?", name).Count(&count).Error
	return count > 0, err
}

// ExistsExcluding checks if a filter rule with the given name exists, excluding a specific ID.
func (f *FilterRuleDB) ExistsExcluding(name string, excludeID uint) (bool, error) {
	var count int64
	err := f.db.DB.Model(&FilterRule{}).Where("name = ? AND id != ?", name, excludeID).Count(&count).Error
	return count > 0, err
}
