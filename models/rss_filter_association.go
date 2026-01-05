package models

import (
	"time"

	"gorm.io/gorm"
)

// RSSFilterAssociation represents a many-to-many relationship between RSS subscriptions and filter rules.
type RSSFilterAssociation struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	RSSID        uint      `gorm:"uniqueIndex:idx_rss_filter;not null" json:"rss_id"`
	FilterRuleID uint      `gorm:"uniqueIndex:idx_rss_filter;not null" json:"filter_rule_id"`
	CreatedAt    time.Time `json:"created_at"`
}

// TableName returns the table name for RSSFilterAssociation.
func (RSSFilterAssociation) TableName() string {
	return "rss_filter_associations"
}

// RSSFilterAssociationDB provides database operations for RSSFilterAssociation.
type RSSFilterAssociationDB struct {
	db *gorm.DB
}

// NewRSSFilterAssociationDB creates a new RSSFilterAssociationDB.
func NewRSSFilterAssociationDB(db *gorm.DB) *RSSFilterAssociationDB {
	return &RSSFilterAssociationDB{db: db}
}

// Create creates a new RSS-filter association.
func (r *RSSFilterAssociationDB) Create(assoc *RSSFilterAssociation) error {
	return r.db.Create(assoc).Error
}

// GetByRSSID retrieves all filter rule IDs associated with an RSS subscription.
func (r *RSSFilterAssociationDB) GetByRSSID(rssID uint) ([]uint, error) {
	var associations []RSSFilterAssociation
	err := r.db.Where("rss_id = ?", rssID).Find(&associations).Error
	if err != nil {
		return nil, err
	}
	ids := make([]uint, len(associations))
	for i, assoc := range associations {
		ids[i] = assoc.FilterRuleID
	}
	return ids, nil
}

// GetByFilterRuleID retrieves all RSS IDs associated with a filter rule.
func (r *RSSFilterAssociationDB) GetByFilterRuleID(filterRuleID uint) ([]uint, error) {
	var associations []RSSFilterAssociation
	err := r.db.Where("filter_rule_id = ?", filterRuleID).Find(&associations).Error
	if err != nil {
		return nil, err
	}
	ids := make([]uint, len(associations))
	for i, assoc := range associations {
		ids[i] = assoc.RSSID
	}
	return ids, nil
}

// GetFilterRulesForRSS retrieves all filter rules associated with an RSS subscription.
func (r *RSSFilterAssociationDB) GetFilterRulesForRSS(rssID uint) ([]FilterRule, error) {
	var rules []FilterRule
	err := r.db.Table("filter_rules").
		Joins("INNER JOIN rss_filter_associations ON filter_rules.id = rss_filter_associations.filter_rule_id").
		Where("rss_filter_associations.rss_id = ?", rssID).
		Where("filter_rules.enabled = ?", true).
		Order("filter_rules.priority ASC, filter_rules.id ASC").
		Find(&rules).Error
	return rules, err
}

// SetFilterRulesForRSS sets the filter rules for an RSS subscription (replaces all existing associations).
func (r *RSSFilterAssociationDB) SetFilterRulesForRSS(rssID uint, filterRuleIDs []uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Delete existing associations
		if err := tx.Where("rss_id = ?", rssID).Delete(&RSSFilterAssociation{}).Error; err != nil {
			return err
		}
		// Create new associations
		for _, filterRuleID := range filterRuleIDs {
			assoc := RSSFilterAssociation{
				RSSID:        rssID,
				FilterRuleID: filterRuleID,
			}
			if err := tx.Create(&assoc).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteByRSSID deletes all associations for an RSS subscription.
func (r *RSSFilterAssociationDB) DeleteByRSSID(rssID uint) error {
	return r.db.Where("rss_id = ?", rssID).Delete(&RSSFilterAssociation{}).Error
}

// DeleteByFilterRuleID deletes all associations for a filter rule.
func (r *RSSFilterAssociationDB) DeleteByFilterRuleID(filterRuleID uint) error {
	return r.db.Where("filter_rule_id = ?", filterRuleID).Delete(&RSSFilterAssociation{}).Error
}

// Exists checks if an association exists.
func (r *RSSFilterAssociationDB) Exists(rssID, filterRuleID uint) (bool, error) {
	var count int64
	err := r.db.Model(&RSSFilterAssociation{}).
		Where("rss_id = ? AND filter_rule_id = ?", rssID, filterRuleID).
		Count(&count).Error
	return count > 0, err
}

// HasAssociations checks if an RSS subscription has any filter rule associations.
func (r *RSSFilterAssociationDB) HasAssociations(rssID uint) (bool, error) {
	var count int64
	err := r.db.Model(&RSSFilterAssociation{}).
		Where("rss_id = ?", rssID).
		Count(&count).Error
	return count > 0, err
}

// GetFilterRuleIDsForRSS is an alias for GetByRSSID for better readability.
func (r *RSSFilterAssociationDB) GetFilterRuleIDsForRSS(rssID uint) ([]uint, error) {
	return r.GetByRSSID(rssID)
}
