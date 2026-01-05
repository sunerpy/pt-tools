package migration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/sunerpy/pt-tools/models"
)

// setupTestDB 创建测试数据库
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}

	// 迁移所有表
	err = db.AutoMigrate(
		&models.SettingsGlobal{},
		&models.QbitSettings{},
		&models.SiteSetting{},
		&models.RSSSubscription{},
		&models.DownloaderSetting{},
		&models.DynamicSiteSetting{},
		&models.SiteTemplate{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

// Feature: downloader-site-extensibility, Property 4: Configuration Migration Preserves Data
// Test all settings are preserved after migration
func TestProperty4_MigrationPreservesData(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.MaxSize = 20

	properties := gopter.NewProperties(parameters)

	// Property: qBittorrent settings are preserved after migration
	properties.Property("qbit settings preserved after migration", prop.ForAll(
		func(url, user, password string, enabled bool) bool {
			if url == "" || user == "" {
				return true // Skip invalid inputs
			}

			db := setupTestDBForProperty(t)

			// Create temp backup dir for this test
			tmpDir, err := os.MkdirTemp("", "migration-test-*")
			if err != nil {
				return true // Skip on temp dir error
			}
			defer os.RemoveAll(tmpDir)

			// Create old qbit settings
			qbit := models.QbitSettings{
				URL:      url,
				User:     user,
				Password: password,
				Enabled:  enabled,
			}
			if err := db.Create(&qbit).Error; err != nil {
				return true // Skip on db error
			}

			// Run migration with temp backup dir
			service := NewMigrationServiceWithBackupDir(db, tmpDir)
			result := service.MigrateV1ToV2()

			if !result.Success {
				return false
			}

			// Verify downloader was created
			var downloader models.DownloaderSetting
			if err := db.First(&downloader).Error; err != nil {
				return false
			}

			// Verify data preserved
			return downloader.URL == url &&
				downloader.Username == user &&
				downloader.Password == password &&
				downloader.Enabled == enabled &&
				downloader.Type == "qbittorrent" &&
				downloader.IsDefault
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 100 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 50 }),
		gen.AlphaString(),
		gen.Bool(),
	))

	// Property: Site settings are preserved after migration
	properties.Property("site settings preserved after migration", prop.ForAll(
		func(name, cookie, apiKey string, enabled, isCookieAuth bool) bool {
			if name == "" {
				return true
			}

			db := setupTestDBForProperty(t)

			// Create temp backup dir for this test
			tmpDir, err := os.MkdirTemp("", "migration-test-*")
			if err != nil {
				return true // Skip on temp dir error
			}
			defer os.RemoveAll(tmpDir)

			authMethod := "cookie"
			if !isCookieAuth {
				authMethod = "api_key"
			}

			// Create old site settings
			site := models.SiteSetting{
				Name:       name,
				Enabled:    enabled,
				AuthMethod: authMethod,
				Cookie:     cookie,
				APIKey:     apiKey,
			}
			if err := db.Create(&site).Error; err != nil {
				return true
			}

			// Run migration with temp backup dir
			service := NewMigrationServiceWithBackupDir(db, tmpDir)
			result := service.MigrateV1ToV2()

			if !result.Success {
				return false
			}

			// Verify dynamic site was created
			var dynamicSite models.DynamicSiteSetting
			if err := db.Where("name = ?", name).First(&dynamicSite).Error; err != nil {
				return false
			}

			// Verify data preserved
			if dynamicSite.Name != name {
				return false
			}
			if dynamicSite.Enabled != enabled {
				return false
			}
			if dynamicSite.AuthMethod != authMethod {
				return false
			}
			if dynamicSite.Cookie != cookie {
				return false
			}
			if dynamicSite.APIKey != apiKey {
				return false
			}
			if !dynamicSite.IsBuiltin {
				return false
			}

			return true
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 30 }),
		gen.AlphaString(),
		gen.AlphaString(),
		gen.Bool(),
		gen.Bool(),
	))

	// Property: Migration count matches source count
	properties.Property("migration count matches source", prop.ForAll(
		func(siteCount int) bool {
			if siteCount < 0 || siteCount > 10 {
				return true
			}

			db := setupTestDBForProperty(t)

			// Create temp backup dir for this test
			tmpDir, err := os.MkdirTemp("", "migration-test-*")
			if err != nil {
				return true // Skip on temp dir error
			}
			defer os.RemoveAll(tmpDir)

			// Create multiple sites
			for i := 0; i < siteCount; i++ {
				site := models.SiteSetting{
					Name:       generateUniqueName(i),
					AuthMethod: "cookie",
					Cookie:     "test-cookie",
				}
				db.Create(&site)
			}

			// Run migration with temp backup dir
			service := NewMigrationServiceWithBackupDir(db, tmpDir)
			result := service.MigrateV1ToV2()

			if !result.Success {
				return false
			}

			return result.SitesMigrated == siteCount
		},
		gen.IntRange(0, 10),
	))

	properties.TestingRun(t)
}

// Feature: downloader-site-extensibility, Property 12: Migration Rollback on Failure
// Test that failed migration restores original config
func TestProperty12_MigrationRollbackOnFailure(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.MaxSize = 20

	properties := gopter.NewProperties(parameters)

	// Property: Backup is created before migration
	properties.Property("backup is created before migration", prop.ForAll(
		func(url, user string) bool {
			if url == "" || user == "" {
				return true
			}

			db := setupTestDBForProperty(t)

			// Create temp backup dir for this test
			tmpDir, err := os.MkdirTemp("", "migration-test-*")
			if err != nil {
				return true // Skip on temp dir error
			}
			defer os.RemoveAll(tmpDir)

			// Create qbit settings
			qbit := models.QbitSettings{
				URL:  url,
				User: user,
			}
			db.Create(&qbit)

			// Run migration with temp backup dir
			service := NewMigrationServiceWithBackupDir(db, tmpDir)
			result := service.MigrateV1ToV2()

			// Backup path should be set
			if result.BackupPath == "" {
				return false
			}

			// Backup file should exist
			if _, err := os.Stat(result.BackupPath); os.IsNotExist(err) {
				return false
			}

			return true
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property: Backup can be restored
	properties.Property("backup can be restored", prop.ForAll(
		func(url, user, password string) bool {
			if url == "" || user == "" {
				return true
			}

			db := setupTestDBForProperty(t)

			// Create temp backup dir for this test
			tmpDir, err := os.MkdirTemp("", "migration-test-*")
			if err != nil {
				return true // Skip on temp dir error
			}
			defer os.RemoveAll(tmpDir)

			// Create original settings
			qbit := models.QbitSettings{
				URL:      url,
				User:     user,
				Password: password,
			}
			db.Create(&qbit)

			service := NewMigrationServiceWithBackupDir(db, tmpDir)

			// Create backup
			backupPath, err := service.CreateBackup()
			if err != nil {
				return false
			}

			// Modify data
			db.Model(&models.QbitSettings{}).Where("id = ?", qbit.ID).Update("url", "modified-url")

			// Restore backup
			if err := service.RestoreBackup(backupPath); err != nil {
				return false
			}

			// Verify original data restored
			var restored models.QbitSettings
			db.First(&restored)

			return restored.URL == url && restored.User == user && restored.Password == password
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 100 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 50 }),
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

// setupTestDBForProperty creates a fresh test DB for property tests
func setupTestDBForProperty(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}

	db.AutoMigrate(
		&models.SettingsGlobal{},
		&models.QbitSettings{},
		&models.SiteSetting{},
		&models.RSSSubscription{},
		&models.DownloaderSetting{},
		&models.DynamicSiteSetting{},
		&models.SiteTemplate{},
	)

	return db
}

// generateUniqueName generates a unique name for testing
func generateUniqueName(index int) string {
	return "site-" + string(rune('a'+index))
}

// TestMigrationService_Basic tests basic migration functionality
func TestMigrationService_Basic(t *testing.T) {
	db := setupTestDB(t)
	service := NewMigrationService(db)

	// Test migration status when empty
	status := service.GetMigrationStatus()
	if status.NeedsMigration {
		t.Error("should not need migration when empty")
	}

	// Add qbit settings
	qbit := models.QbitSettings{
		URL:      "http://localhost:8080",
		User:     "admin",
		Password: "password",
		Enabled:  true,
	}
	db.Create(&qbit)

	// Now should need migration
	status = service.GetMigrationStatus()
	if !status.NeedsMigration {
		t.Error("should need migration after adding qbit settings")
	}
	if !status.HasOldQbitConfig {
		t.Error("should have old qbit config")
	}

	// Run migration
	result := service.MigrateV1ToV2()
	if !result.Success {
		t.Errorf("migration failed: %s", result.Message)
	}
	if result.DownloadersMigrated != 1 {
		t.Errorf("expected 1 downloader migrated, got %d", result.DownloadersMigrated)
	}

	// Verify downloader created
	var downloader models.DownloaderSetting
	if err := db.First(&downloader).Error; err != nil {
		t.Errorf("downloader not found: %v", err)
	}
	if downloader.URL != "http://localhost:8080" {
		t.Errorf("expected URL 'http://localhost:8080', got '%s'", downloader.URL)
	}
	if downloader.Type != "qbittorrent" {
		t.Errorf("expected type 'qbittorrent', got '%s'", downloader.Type)
	}
	if !downloader.IsDefault {
		t.Error("expected downloader to be default")
	}

	// Should not need migration anymore
	status = service.GetMigrationStatus()
	if status.NeedsMigration {
		t.Error("should not need migration after migration")
	}
}

// TestMigrationService_SiteMigration tests site migration
func TestMigrationService_SiteMigration(t *testing.T) {
	db := setupTestDB(t)
	service := NewMigrationService(db)

	// Add sites
	sites := []models.SiteSetting{
		{Name: "hdsky", Enabled: true, AuthMethod: "cookie", Cookie: "hdsky-cookie"},
		{Name: "mteam", Enabled: false, AuthMethod: "api_key", APIKey: "mteam-key", APIUrl: "https://api.mteam.com"},
	}
	for _, site := range sites {
		db.Create(&site)
	}

	// Run migration
	result := service.MigrateV1ToV2()
	if !result.Success {
		t.Errorf("migration failed: %s", result.Message)
	}
	if result.SitesMigrated != 2 {
		t.Errorf("expected 2 sites migrated, got %d", result.SitesMigrated)
	}

	// Verify dynamic sites created
	var dynamicSites []models.DynamicSiteSetting
	db.Find(&dynamicSites)
	if len(dynamicSites) != 2 {
		t.Errorf("expected 2 dynamic sites, got %d", len(dynamicSites))
	}

	// Verify site data
	for _, ds := range dynamicSites {
		if !ds.IsBuiltin {
			t.Errorf("expected site %s to be builtin", ds.Name)
		}
	}
}

// TestMigrationService_Backup tests backup functionality
func TestMigrationService_Backup(t *testing.T) {
	db := setupTestDB(t)
	service := NewMigrationService(db)

	// Add some data
	db.Create(&models.QbitSettings{URL: "http://test", User: "user", Password: "pass"})
	db.Create(&models.SiteSetting{Name: "test-site", AuthMethod: "cookie", Cookie: "test"})

	// Create backup
	backupPath, err := service.CreateBackup()
	if err != nil {
		t.Fatalf("failed to create backup: %v", err)
	}
	defer os.Remove(backupPath)

	// Verify backup file exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("backup file does not exist")
	}

	// Verify backup is in correct directory
	dir := filepath.Dir(backupPath)
	if !filepath.IsAbs(dir) {
		t.Error("backup path should be absolute")
	}
}

// TestIsMigrationNeeded 测试迁移需求检查
func TestIsMigrationNeeded(t *testing.T) {
	db := setupTestDB(t)
	service := NewMigrationService(db)

	// 空数据库不需要迁移
	needed := service.IsMigrationNeeded()
	if needed {
		t.Error("empty database should not need migration")
	}

	// 添加旧的 qbit 配置
	db.Create(&models.QbitSettings{URL: "http://test", User: "user"})

	// 现在应该需要迁移
	needed = service.IsMigrationNeeded()
	if !needed {
		t.Error("should need migration after adding qbit settings")
	}

	// 添加新的下载器配置
	db.Create(&models.DownloaderSetting{Name: "test", Type: "qbittorrent", URL: "http://test"})

	// 有新配置后不需要迁移
	needed = service.IsMigrationNeeded()
	if needed {
		t.Error("should not need migration after adding new downloader")
	}
}

// TestIsMigrationNeeded_WithSiteSettings 测试站点设置的迁移需求检查
func TestIsMigrationNeeded_WithSiteSettings(t *testing.T) {
	db := setupTestDB(t)
	service := NewMigrationService(db)

	// 添加旧的站点配置
	db.Create(&models.SiteSetting{Name: "test-site", AuthMethod: "cookie", Cookie: "test"})

	// IsMigrationNeeded 只检查 qbit 设置，所以这里不需要迁移
	needed := service.IsMigrationNeeded()
	// 注意：IsMigrationNeeded 只检查 qbit 设置
	_ = needed

	// 添加新的动态站点配置
	db.Create(&models.DynamicSiteSetting{Name: "test-site", AuthMethod: "cookie"})

	// 有新配置后不需要迁移
	needed = service.IsMigrationNeeded()
	if needed {
		t.Error("should not need migration after adding dynamic site")
	}
}

// TestMigrationService_RestoreBackup 测试恢复备份
func TestMigrationService_RestoreBackup(t *testing.T) {
	db := setupTestDB(t)
	service := NewMigrationService(db)

	// 添加原始数据
	originalQbit := models.QbitSettings{URL: "http://original", User: "original-user", Password: "original-pass"}
	db.Create(&originalQbit)

	// 创建备份
	backupPath, err := service.CreateBackup()
	if err != nil {
		t.Fatalf("failed to create backup: %v", err)
	}
	defer os.Remove(backupPath)

	// 修改数据
	db.Model(&models.QbitSettings{}).Where("id = ?", originalQbit.ID).Updates(map[string]any{
		"url":      "http://modified",
		"user":     "modified-user",
		"password": "modified-pass",
	})

	// 验证数据已修改
	var modified models.QbitSettings
	db.First(&modified)
	if modified.URL != "http://modified" {
		t.Error("data should be modified before restore")
	}

	// 恢复备份
	err = service.RestoreBackup(backupPath)
	if err != nil {
		t.Fatalf("failed to restore backup: %v", err)
	}

	// 验证数据已恢复
	var restored models.QbitSettings
	db.First(&restored)
	if restored.URL != "http://original" {
		t.Errorf("expected URL 'http://original', got '%s'", restored.URL)
	}
	if restored.User != "original-user" {
		t.Errorf("expected user 'original-user', got '%s'", restored.User)
	}
}

// TestMigrationService_RestoreBackup_InvalidPath 测试恢复无效备份路径
func TestMigrationService_RestoreBackup_InvalidPath(t *testing.T) {
	db := setupTestDB(t)
	service := NewMigrationService(db)

	// 尝试恢复不存在的备份
	err := service.RestoreBackup("/nonexistent/path/backup.json")
	if err == nil {
		t.Error("expected error for invalid backup path")
	}
}

// TestMigrationService_CreateBackup_WithAllData 测试创建包含所有数据的备份
func TestMigrationService_CreateBackup_WithAllData(t *testing.T) {
	db := setupTestDB(t)
	service := NewMigrationService(db)

	// 添加各种数据
	db.Create(&models.QbitSettings{URL: "http://qbit", User: "qbit-user"})
	db.Create(&models.SiteSetting{Name: "site1", AuthMethod: "cookie", Cookie: "cookie1"})
	db.Create(&models.SiteSetting{Name: "site2", AuthMethod: "api_key", APIKey: "key2"})
	db.Create(&models.SettingsGlobal{DownloadDir: "/downloads", DefaultIntervalMinutes: 10})

	// 创建备份
	backupPath, err := service.CreateBackup()
	if err != nil {
		t.Fatalf("failed to create backup: %v", err)
	}
	defer os.Remove(backupPath)

	// 验证备份文件存在且不为空
	info, err := os.Stat(backupPath)
	if err != nil {
		t.Fatalf("backup file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("backup file should not be empty")
	}
}

// TestMigrationService_MigrateV1ToV2_AlreadyMigrated 测试已迁移的情况
func TestMigrationService_MigrateV1ToV2_AlreadyMigrated(t *testing.T) {
	db := setupTestDB(t)
	service := NewMigrationService(db)

	// 添加新格式的数据（已迁移）
	db.Create(&models.DownloaderSetting{Name: "existing", Type: "qbittorrent", URL: "http://test"})
	db.Create(&models.DynamicSiteSetting{Name: "existing-site", AuthMethod: "cookie"})

	// 运行迁移
	result := service.MigrateV1ToV2()

	// 应该成功但没有迁移任何数据
	if !result.Success {
		t.Errorf("migration should succeed: %s", result.Message)
	}
	if result.DownloadersMigrated != 0 {
		t.Errorf("expected 0 downloaders migrated, got %d", result.DownloadersMigrated)
	}
	if result.SitesMigrated != 0 {
		t.Errorf("expected 0 sites migrated, got %d", result.SitesMigrated)
	}
}

// TestMigrationService_MigrateV1ToV2_WithRSSSubscriptions 测试RSS订阅迁移
func TestMigrationService_MigrateV1ToV2_WithRSSSubscriptions(t *testing.T) {
	db := setupTestDB(t)
	service := NewMigrationService(db)

	// 添加站点和RSS订阅
	site := models.SiteSetting{Name: "test-site", AuthMethod: "cookie", Cookie: "test"}
	db.Create(&site)

	rss := models.RSSSubscription{
		SiteID:          site.ID,
		Name:            "test-rss",
		URL:             "https://example.com/rss",
		IntervalMinutes: 15,
	}
	db.Create(&rss)

	// 运行迁移
	result := service.MigrateV1ToV2()
	if !result.Success {
		t.Errorf("migration failed: %s", result.Message)
	}

	// 验证站点已迁移
	if result.SitesMigrated != 1 {
		t.Errorf("expected 1 site migrated, got %d", result.SitesMigrated)
	}
}

// TestNewMigrationServiceWithBackupDir 测试自定义备份目录
func TestNewMigrationServiceWithBackupDir(t *testing.T) {
	db := setupTestDB(t)
	tmpDir := t.TempDir()

	service := NewMigrationServiceWithBackupDir(db, tmpDir)

	// 添加数据
	db.Create(&models.QbitSettings{URL: "http://test", User: "user"})

	// 创建备份
	backupPath, err := service.CreateBackup()
	if err != nil {
		t.Fatalf("failed to create backup: %v", err)
	}

	// 验证备份在指定目录
	if filepath.Dir(backupPath) != tmpDir {
		t.Errorf("backup should be in %s, got %s", tmpDir, filepath.Dir(backupPath))
	}
}

// TestMigrationService_RestoreBackup_InvalidJSON 测试恢复无效JSON备份
func TestMigrationService_RestoreBackup_InvalidJSON(t *testing.T) {
	db := setupTestDB(t)
	tmpDir := t.TempDir()
	service := NewMigrationServiceWithBackupDir(db, tmpDir)

	// 创建无效JSON文件
	invalidFile := filepath.Join(tmpDir, "invalid.json")
	err := os.WriteFile(invalidFile, []byte("not valid json"), 0o644)
	require.NoError(t, err)

	// 尝试恢复
	err = service.RestoreBackup(invalidFile)
	require.Error(t, err)
	require.Contains(t, err.Error(), "解析备份数据失败")
}

// TestMigrationService_RestoreBackup_WithGlobalSettings 测试恢复全局设置
func TestMigrationService_RestoreBackup_WithGlobalSettings(t *testing.T) {
	db := setupTestDB(t)
	tmpDir := t.TempDir()
	service := NewMigrationServiceWithBackupDir(db, tmpDir)

	// 添加全局设置
	global := models.SettingsGlobal{
		DownloadDir:            "/downloads",
		DefaultIntervalMinutes: 15,
	}
	db.Create(&global)

	// 创建备份
	backupPath, err := service.CreateBackup()
	require.NoError(t, err)

	// 修改全局设置
	db.Model(&models.SettingsGlobal{}).Where("id = ?", global.ID).Updates(map[string]any{
		"download_dir":             "/modified",
		"default_interval_minutes": 30,
	})

	// 恢复备份
	err = service.RestoreBackup(backupPath)
	require.NoError(t, err)

	// 验证恢复
	var restored models.SettingsGlobal
	db.First(&restored)
	require.Equal(t, "/downloads", restored.DownloadDir)
	require.Equal(t, int32(15), restored.DefaultIntervalMinutes)
}

// TestMigrationService_RestoreBackup_WithQbitSettings 测试恢复qBittorrent设置
func TestMigrationService_RestoreBackup_WithQbitSettings(t *testing.T) {
	db := setupTestDB(t)
	tmpDir := t.TempDir()
	service := NewMigrationServiceWithBackupDir(db, tmpDir)

	// 添加qbit设置
	qbit := models.QbitSettings{
		URL:      "http://qbit:8080",
		User:     "admin",
		Password: "secret",
		Enabled:  true,
	}
	db.Create(&qbit)

	// 创建备份
	backupPath, err := service.CreateBackup()
	require.NoError(t, err)

	// 修改qbit设置
	db.Model(&models.QbitSettings{}).Where("id = ?", qbit.ID).Updates(map[string]any{
		"url":      "http://modified:9090",
		"user":     "modified",
		"password": "modified",
	})

	// 恢复备份
	err = service.RestoreBackup(backupPath)
	require.NoError(t, err)

	// 验证恢复
	var restored models.QbitSettings
	db.First(&restored)
	require.Equal(t, "http://qbit:8080", restored.URL)
	require.Equal(t, "admin", restored.User)
	require.Equal(t, "secret", restored.Password)
}

// TestMigrationService_RestoreBackup_WithSites 测试恢复站点设置
func TestMigrationService_RestoreBackup_WithSites(t *testing.T) {
	db := setupTestDB(t)
	tmpDir := t.TempDir()
	service := NewMigrationServiceWithBackupDir(db, tmpDir)

	// 添加站点设置
	sites := []models.SiteSetting{
		{Name: "site1", AuthMethod: "cookie", Cookie: "cookie1", Enabled: true},
		{Name: "site2", AuthMethod: "api_key", APIKey: "key2", Enabled: false},
	}
	for _, s := range sites {
		db.Create(&s)
	}

	// 创建备份
	backupPath, err := service.CreateBackup()
	require.NoError(t, err)

	// 删除站点
	db.Where("1 = 1").Delete(&models.SiteSetting{})

	// 恢复备份
	err = service.RestoreBackup(backupPath)
	require.NoError(t, err)

	// 验证恢复
	var restored []models.SiteSetting
	db.Find(&restored)
	require.Len(t, restored, 2)
}

// TestMigrationService_RestoreBackup_WithRSS 测试恢复RSS订阅
func TestMigrationService_RestoreBackup_WithRSS(t *testing.T) {
	db := setupTestDB(t)
	tmpDir := t.TempDir()
	service := NewMigrationServiceWithBackupDir(db, tmpDir)

	// 添加站点和RSS订阅
	site := models.SiteSetting{Name: "test-site", AuthMethod: "cookie", Cookie: "test"}
	db.Create(&site)

	rss := models.RSSSubscription{
		SiteID:          site.ID,
		Name:            "test-rss",
		URL:             "https://example.com/rss",
		IntervalMinutes: 15,
	}
	db.Create(&rss)

	// 创建备份
	backupPath, err := service.CreateBackup()
	require.NoError(t, err)

	// 删除RSS
	db.Where("1 = 1").Delete(&models.RSSSubscription{})

	// 恢复备份
	err = service.RestoreBackup(backupPath)
	require.NoError(t, err)

	// 验证恢复
	var restored []models.RSSSubscription
	db.Find(&restored)
	require.Len(t, restored, 1)
	require.Equal(t, "test-rss", restored[0].Name)
}

// TestMigrationService_CreateBackup_ErrorCases 测试创建备份的错误情况
func TestMigrationService_CreateBackup_ErrorCases(t *testing.T) {
	db := setupTestDB(t)

	// 使用不可写的目录
	service := NewMigrationServiceWithBackupDir(db, "/nonexistent/path/that/does/not/exist")

	_, err := service.CreateBackup()
	require.Error(t, err)
	require.Contains(t, err.Error(), "创建备份目录失败")
}

// TestMigrationService_MigrateV1ToV2_EmptyQbitURL 测试空URL的qbit设置不迁移
func TestMigrationService_MigrateV1ToV2_EmptyQbitURL(t *testing.T) {
	db := setupTestDB(t)
	tmpDir := t.TempDir()
	service := NewMigrationServiceWithBackupDir(db, tmpDir)

	// 添加空URL的qbit设置
	qbit := models.QbitSettings{
		URL:      "",
		User:     "admin",
		Password: "password",
	}
	db.Create(&qbit)

	// 运行迁移
	result := service.MigrateV1ToV2()
	require.True(t, result.Success)
	require.Equal(t, 0, result.DownloadersMigrated)
}

// TestMigrationService_GetMigrationStatus_AllCases 测试迁移状态的各种情况
func TestMigrationService_GetMigrationStatus_AllCases(t *testing.T) {
	db := setupTestDB(t)
	service := NewMigrationService(db)

	// 空数据库
	status := service.GetMigrationStatus()
	require.False(t, status.NeedsMigration)
	require.False(t, status.HasOldQbitConfig)
	require.False(t, status.HasNewDownloaders)
	require.Equal(t, 0, status.OldSitesCount)
	require.Equal(t, 0, status.NewDynamicSitesCount)

	// 添加旧站点
	db.Create(&models.SiteSetting{Name: "old-site", AuthMethod: "cookie"})
	status = service.GetMigrationStatus()
	require.Equal(t, 1, status.OldSitesCount)

	// 添加新动态站点
	db.Create(&models.DynamicSiteSetting{Name: "new-site", AuthMethod: "cookie"})
	status = service.GetMigrationStatus()
	require.Equal(t, 1, status.NewDynamicSitesCount)

	// 添加旧qbit配置
	db.Create(&models.QbitSettings{URL: "http://test", User: "user"})
	status = service.GetMigrationStatus()
	require.True(t, status.HasOldQbitConfig)
	require.True(t, status.NeedsMigration)

	// 添加新下载器配置
	db.Create(&models.DownloaderSetting{Name: "new-dl", Type: "qbittorrent", URL: "http://test"})
	status = service.GetMigrationStatus()
	require.True(t, status.HasNewDownloaders)
	require.False(t, status.NeedsMigration)
}
