package store

import (
	"fmt"

	"gorm.io/gorm"
)

// ScraperSchemaVersion 维护 scraper 独立的 schema 版本
// 不混入 pt-tools 根 models 的 SchemaVersion 表
type ScraperSchemaVersion struct {
	ID        uint `gorm:"primaryKey"`
	Version   int  `gorm:"not null"`
	AppliedAt int64
}

func (ScraperSchemaVersion) TableName() string { return "scraper_schema_versions" }

const CurrentScraperSchemaVersion = 1

// Migrate 在给定 DB 上建表并记录 scraper 的 schema 版本
// 幂等：重复调用安全（AutoMigrate 不会重建已有表）
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&ScraperSchemaVersion{}); err != nil {
		return fmt.Errorf("migrate scraper_schema_versions: %w", err)
	}
	if err := db.AutoMigrate(AllModels()...); err != nil {
		return fmt.Errorf("migrate scraper models: %w", err)
	}
	// 记录/更新版本（upsert）
	var current ScraperSchemaVersion
	res := db.First(&current)
	if res.Error != nil && res.Error == gorm.ErrRecordNotFound {
		return db.Create(&ScraperSchemaVersion{Version: CurrentScraperSchemaVersion}).Error
	}
	if res.Error != nil {
		return fmt.Errorf("read schema version: %w", res.Error)
	}
	if current.Version < CurrentScraperSchemaVersion {
		return db.Model(&current).Update("version", CurrentScraperSchemaVersion).Error
	}
	return nil
}
