package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}
	// 迁移所有表
	if err := db.AutoMigrate(
		&SchemaVersion{},
		&SettingsGlobal{},
		&SiteSetting{},
		&RSSSubscription{},
	); err != nil {
		t.Fatalf("迁移表结构失败: %v", err)
	}
	return db
}

func TestSchemaManager_NewDatabase(t *testing.T) {
	db := setupTestDB(t)
	sm := NewSchemaManager(db, "1.0.0")

	// 新数据库应该没有版本记录
	version, err := sm.GetCurrentVersion()
	if err != nil {
		t.Fatalf("获取版本失败: %v", err)
	}
	if version != 0 {
		t.Errorf("新数据库版本应为 0，实际为 %d", version)
	}

	// 运行迁移
	if runErr := sm.RunMigrations(); runErr != nil {
		t.Fatalf("运行迁移失败: %v", runErr)
	}

	// 全新数据库应该直接标记为最新版本
	version, err = sm.GetCurrentVersion()
	if err != nil {
		t.Fatalf("获取版本失败: %v", err)
	}
	if version != CurrentSchemaVersion {
		t.Errorf("全新数据库应为最新版本 %d，实际为 %d", CurrentSchemaVersion, version)
	}
}

func TestSchemaManager_LegacyDatabase(t *testing.T) {
	db := setupTestDB(t)

	// 模拟旧版本数据库：有数据但无版本记录
	site := SiteSetting{Name: "test", AuthMethod: "cookie", Enabled: false}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("创建测试站点失败: %v", err)
	}

	// 创建示例 RSS（模拟旧版本数据）
	rss := RSSSubscription{
		SiteID:          site.ID,
		Name:            "示例RSS",
		URL:             "https://springxxx.xxx/rss",
		IntervalMinutes: 5,
		IsExample:       false, // 旧版本默认为 false
	}
	if err := db.Create(&rss).Error; err != nil {
		t.Fatalf("创建测试RSS失败: %v", err)
	}

	sm := NewSchemaManager(db, "1.0.0")

	// 运行迁移
	if err := sm.RunMigrations(); err != nil {
		t.Fatalf("运行迁移失败: %v", err)
	}

	// 应该检测为旧版本并执行迁移
	version, err := sm.GetCurrentVersion()
	if err != nil {
		t.Fatalf("获取版本失败: %v", err)
	}
	if version != CurrentSchemaVersion {
		t.Errorf("迁移后应为最新版本 %d，实际为 %d", CurrentSchemaVersion, version)
	}

	// 验证示例 RSS 被标记
	var updatedRSS RSSSubscription
	if err := db.First(&updatedRSS, rss.ID).Error; err != nil {
		t.Fatalf("查询RSS失败: %v", err)
	}
	if !updatedRSS.IsExample {
		t.Error("示例RSS应该被标记为 IsExample=true")
	}
}

func TestSchemaManager_AlreadyMigrated(t *testing.T) {
	db := setupTestDB(t)
	sm := NewSchemaManager(db, "1.0.0")

	// 第一次运行迁移
	if err := sm.RunMigrations(); err != nil {
		t.Fatalf("第一次迁移失败: %v", err)
	}

	// 第二次运行迁移（应该是幂等的）
	if err := sm.RunMigrations(); err != nil {
		t.Fatalf("第二次迁移失败: %v", err)
	}

	// 版本应该保持不变
	version, err := sm.GetCurrentVersion()
	if err != nil {
		t.Fatalf("获取版本失败: %v", err)
	}
	if version != CurrentSchemaVersion {
		t.Errorf("版本应为 %d，实际为 %d", CurrentSchemaVersion, version)
	}

	// 检查版本记录数量（应该只有一条）
	var count int64
	db.Model(&SchemaVersion{}).Count(&count)
	if count != 1 {
		t.Errorf("版本记录应为 1 条，实际为 %d 条", count)
	}
}

func TestSchemaManager_IsLegacyDatabase(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(db *gorm.DB)
		expected bool
	}{
		{
			name:     "空数据库",
			setup:    func(db *gorm.DB) {},
			expected: false,
		},
		{
			name: "有站点数据",
			setup: func(db *gorm.DB) {
				db.Create(&SiteSetting{Name: "test", AuthMethod: "cookie"})
			},
			expected: true,
		},
		{
			name: "有全局设置",
			setup: func(db *gorm.DB) {
				db.Create(&SettingsGlobal{DownloadDir: "downloads"})
			},
			expected: true,
		},
		{
			name: "有RSS订阅",
			setup: func(db *gorm.DB) {
				site := SiteSetting{Name: "test", AuthMethod: "cookie"}
				db.Create(&site)
				db.Create(&RSSSubscription{SiteID: site.ID, Name: "test", URL: "http://test.com", IntervalMinutes: 5})
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			tt.setup(db)

			sm := NewSchemaManager(db, "1.0.0")
			isLegacy, err := sm.isLegacyDatabase()
			if err != nil {
				t.Fatalf("检测失败: %v", err)
			}
			if isLegacy != tt.expected {
				t.Errorf("isLegacyDatabase() = %v, want %v", isLegacy, tt.expected)
			}
		})
	}
}
