package models

import (
	"time"

	"gorm.io/gorm"
)

// SchemaVersion 数据库架构版本表
// 用于跟踪数据库迁移状态
type SchemaVersion struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Version     int       `gorm:"uniqueIndex;not null" json:"version"` // 架构版本号
	Description string    `gorm:"size:255" json:"description"`         // 版本描述
	AppliedAt   time.Time `gorm:"not null" json:"applied_at"`          // 应用时间
	AppVersion  string    `gorm:"size:64" json:"app_version"`          // 应用版本
}

// 当前数据库架构版本
// 每次添加新的迁移时递增此值
const CurrentSchemaVersion = 3

// 架构版本历史：
// v1: 初始版本（无版本表的旧应用）
// v2: 添加 IsExample 字段到 RSS 订阅，添加 DefaultConcurrency 到全局设置
// v3: 添加 user_info 表用于存储用户统计信息

// MigrationFunc 迁移函数类型
type MigrationFunc func(db *gorm.DB) error

// Migration 迁移定义
type Migration struct {
	Version     int
	Description string
	Up          MigrationFunc
}

// SchemaManager 架构版本管理器
type SchemaManager struct {
	db         *gorm.DB
	migrations []Migration
	appVersion string
}

// NewSchemaManager 创建架构版本管理器
func NewSchemaManager(db *gorm.DB, appVersion string) *SchemaManager {
	sm := &SchemaManager{
		db:         db,
		appVersion: appVersion,
		migrations: make([]Migration, 0),
	}
	sm.registerMigrations()
	return sm
}

// registerMigrations 注册所有迁移
func (sm *SchemaManager) registerMigrations() {
	// v1 -> v2: 添加 IsExample 字段迁移
	sm.migrations = append(sm.migrations, Migration{
		Version:     2,
		Description: "添加 RSS IsExample 字段和全局 DefaultConcurrency 字段",
		Up:          migrateV1ToV2,
	})

	// v2 -> v3: 添加 user_info 表
	sm.migrations = append(sm.migrations, Migration{
		Version:     3,
		Description: "添加 user_info 表用于存储用户统计信息",
		Up:          migrateV2ToV3,
	})
}

// GetCurrentVersion 获取当前数据库架构版本
func (sm *SchemaManager) GetCurrentVersion() (int, error) {
	// 检查版本表是否存在
	if !sm.db.Migrator().HasTable(&SchemaVersion{}) {
		return 0, nil // 版本表不存在，返回 0
	}

	var sv SchemaVersion
	err := sm.db.Order("version DESC").First(&sv).Error
	if err == gorm.ErrRecordNotFound {
		return 0, nil // 表存在但无记录
	}
	if err != nil {
		return 0, err
	}
	return sv.Version, nil
}

// EnsureSchemaVersionTable 确保版本表存在
func (sm *SchemaManager) EnsureSchemaVersionTable() error {
	return sm.db.AutoMigrate(&SchemaVersion{})
}

// RecordVersion 记录版本
func (sm *SchemaManager) RecordVersion(version int, description string) error {
	sv := SchemaVersion{
		Version:     version,
		Description: description,
		AppliedAt:   time.Now(),
		AppVersion:  sm.appVersion,
	}
	return sm.db.Create(&sv).Error
}

// RunMigrations 运行所有待执行的迁移
func (sm *SchemaManager) RunMigrations() error {
	// 确保版本表存在
	if err := sm.EnsureSchemaVersionTable(); err != nil {
		return err
	}

	// 获取当前版本
	currentVersion, err := sm.GetCurrentVersion()
	if err != nil {
		return err
	}

	// 检测是否为旧应用（无版本记录但有数据）
	if currentVersion == 0 {
		isLegacy, err := sm.isLegacyDatabase()
		if err != nil {
			return err
		}
		if isLegacy {
			// 旧应用，标记为 v1
			if err := sm.RecordVersion(1, "旧版本应用初始化"); err != nil {
				return err
			}
			currentVersion = 1
		} else {
			// 全新环境，直接标记为最新版本
			if err := sm.RecordVersion(CurrentSchemaVersion, "全新安装"); err != nil {
				return err
			}
			return nil // 无需迁移
		}
	}

	// 执行待执行的迁移
	for _, m := range sm.migrations {
		if m.Version > currentVersion {
			if err := m.Up(sm.db); err != nil {
				return err
			}
			if err := sm.RecordVersion(m.Version, m.Description); err != nil {
				return err
			}
		}
	}

	return nil
}

// isLegacyDatabase 检测是否为旧版本数据库（有数据但无版本表记录）
func (sm *SchemaManager) isLegacyDatabase() (bool, error) {
	// 检查是否有站点设置数据
	var siteCount int64
	if err := sm.db.Model(&SiteSetting{}).Count(&siteCount).Error; err != nil {
		return false, err
	}
	if siteCount > 0 {
		return true, nil
	}

	// 检查是否有全局设置数据
	var globalCount int64
	if err := sm.db.Model(&SettingsGlobal{}).Count(&globalCount).Error; err != nil {
		return false, err
	}
	if globalCount > 0 {
		return true, nil
	}

	// 检查是否有 RSS 订阅数据
	var rssCount int64
	if err := sm.db.Model(&RSSSubscription{}).Count(&rssCount).Error; err != nil {
		return false, err
	}
	if rssCount > 0 {
		return true, nil
	}

	return false, nil
}

// migrateV1ToV2 v1 到 v2 的迁移
func migrateV1ToV2(db *gorm.DB) error {
	// 1. 将示例 RSS URL 标记为 IsExample=true
	if err := MigrateExampleRSS(db); err != nil {
		return err
	}

	// 2. 确保全局设置有默认并发数（GORM AutoMigrate 会自动添加新字段）
	// 这里只需要确保已有记录有合理的默认值
	if err := db.Model(&SettingsGlobal{}).
		Where("default_concurrency = ? OR default_concurrency IS NULL", 0).
		Update("default_concurrency", DefaultConcurrency).Error; err != nil {
		return err
	}

	return nil
}

// migrateV2ToV3 v2 到 v3 的迁移 - 添加 user_info 表
func migrateV2ToV3(db *gorm.DB) error {
	// user_info 表会通过 GORM AutoMigrate 自动创建
	// 这里只需要确保表存在即可
	// 表结构定义在 site/v2/userinfo_db_repo.go 中的 UserInfoRecord

	// 检查表是否已存在
	if db.Migrator().HasTable("user_info") {
		return nil
	}

	// 创建 user_info 表
	type UserInfoRecord struct {
		ID         uint   `gorm:"primaryKey"`
		Site       string `gorm:"uniqueIndex;size:64;not null"`
		Username   string `gorm:"size:128"`
		UserID     string `gorm:"size:64"`
		Rank       string `gorm:"size:64"`
		Uploaded   int64
		Downloaded int64
		Ratio      float64
		Seeding    int
		Leeching   int
		Bonus      float64
		JoinDate   int64
		LastAccess int64
		LastUpdate int64
		CreatedAt  time.Time
		UpdatedAt  time.Time
	}

	return db.Migrator().CreateTable(&UserInfoRecord{})
}
