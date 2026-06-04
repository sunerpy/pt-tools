package models

import (
	"time"

	"gorm.io/gorm"
)

// MigrationState records post-migration runtime metadata. One row per schema version.
type MigrationState struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	SchemaVersion int       `gorm:"uniqueIndex;not null" json:"schema_version"`
	CompletedAt   time.Time `gorm:"not null;index" json:"completed_at"`
	BroadcastSent bool      `gorm:"not null;default:false" json:"broadcast_sent"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// GetLatestMigrationCompletedAt returns the newest recorded migration
// completion timestamp; ok=false if table or rows are absent.
func GetLatestMigrationCompletedAt(db *gorm.DB) (time.Time, bool) {
	if db == nil || !db.Migrator().HasTable(&MigrationState{}) {
		return time.Time{}, false
	}

	var state MigrationState
	result := db.Order("schema_version DESC, completed_at DESC").Limit(1).Find(&state)
	if result.Error != nil || result.RowsAffected == 0 {
		return time.Time{}, false
	}
	return state.CompletedAt, true
}

// UpsertMigrationState writes (or updates) the row for schemaVersion. Idempotent.
func UpsertMigrationState(db *gorm.DB, schemaVersion int, completedAt time.Time) error {
	if db == nil {
		return nil
	}
	var existing MigrationState
	err := db.Where("schema_version = ?", schemaVersion).First(&existing).Error
	if err == nil {
		existing.CompletedAt = completedAt.UTC()
		return db.Save(&existing).Error
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}
	return db.Create(&MigrationState{
		SchemaVersion: schemaVersion,
		CompletedAt:   completedAt.UTC(),
	}).Error
}

// MarkBroadcastSent flips BroadcastSent=true for the given schema version.
func MarkBroadcastSent(db *gorm.DB, schemaVersion int) error {
	if db == nil {
		return nil
	}
	return db.Model(&MigrationState{}).
		Where("schema_version = ?", schemaVersion).
		Update("broadcast_sent", true).Error
}

// GetMigrationState returns the row for schemaVersion, or (nil, false) if missing.
func GetMigrationState(db *gorm.DB, schemaVersion int) (*MigrationState, bool) {
	if db == nil || !db.Migrator().HasTable(&MigrationState{}) {
		return nil, false
	}
	var state MigrationState
	if err := db.Where("schema_version = ?", schemaVersion).First(&state).Error; err != nil {
		return nil, false
	}
	return &state, true
}
