package models

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type spyHooks struct {
	backupCalls  atomic.Int32
	encryptCalls atomic.Int32
	decryptCalls atomic.Int32
	failEncrypt  int32
}

func (s *spyHooks) BackupTable(db *gorm.DB, table string) (string, error) {
	s.backupCalls.Add(1)
	return filepath.Join("backup", table+".json"), nil
}

func (s *spyHooks) EncryptCookie(plain string) (string, error) {
	call := s.encryptCalls.Add(1)
	if s.failEncrypt > 0 && call >= s.failEncrypt {
		return "", errors.New("KEY_ERROR: injected encrypt failure")
	}
	return "cipher:" + plain, nil
}

func (s *spyHooks) DecryptCookie(cipher string) (string, error) {
	s.decryptCalls.Add(1)
	plain, ok := strings.CutPrefix(cipher, "cipher:")
	if !ok {
		return "", fmt.Errorf("invalid test ciphertext: %s", cipher)
	}
	return plain, nil
}

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

func setupV8MigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "v8.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("创建 v8 测试数据库失败: %v", err)
	}
	if err := db.AutoMigrate(&SchemaVersion{}, &SiteSetting{}); err != nil {
		t.Fatalf("迁移 v8 基础表失败: %v", err)
	}
	if err := db.Create(&SchemaVersion{Version: 8, Description: "test v8", AppVersion: "test"}).Error; err != nil {
		t.Fatalf("写入 v8 版本失败: %v", err)
	}
	for idx := 0; idx < 5; idx++ {
		name := fmt.Sprintf("site-%d", idx)
		if idx == 0 {
			name = "mteam"
		}
		setting := SiteSetting{
			Name:       name,
			AuthMethod: "cookie",
			Cookie:     fmt.Sprintf("uid=%d; token=abc", idx),
			Enabled:    true,
		}
		if err := db.Create(&setting).Error; err != nil {
			t.Fatalf("写入站点 %s 失败: %v", name, err)
		}
	}
	return db
}

func TestMigrationV8ToV9Happy(t *testing.T) {
	db := setupV8MigrationDB(t)
	hooks := &spyHooks{}
	sm := NewSchemaManagerWithHooks(db, "test", hooks.BackupTable, hooks.EncryptCookie, hooks.DecryptCookie)

	if err := sm.RunMigrations(); err != nil {
		t.Fatalf("v8→v9 迁移失败: %v", err)
	}

	var sites []SiteSetting
	if err := db.Order("id ASC").Find(&sites).Error; err != nil {
		t.Fatalf("查询站点失败: %v", err)
	}
	if len(sites) != 5 {
		t.Fatalf("站点数 = %d, want 5", len(sites))
	}
	for _, site := range sites {
		if site.CookieEncrypted == "" {
			t.Fatalf("站点 %s CookieEncrypted 为空", site.Name)
		}
		plain, err := hooks.DecryptCookie(site.CookieEncrypted)
		if err != nil {
			t.Fatalf("解密站点 %s 失败: %v", site.Name, err)
		}
		if plain != site.Cookie {
			t.Fatalf("站点 %s 解密结果 = %q, want %q", site.Name, plain, site.Cookie)
		}
	}

	var loginStateCount int64
	if err := db.Model(&SiteLoginState{}).Count(&loginStateCount).Error; err != nil {
		t.Fatalf("统计登录状态失败: %v", err)
	}
	if loginStateCount != 5 {
		t.Fatalf("site_login_state 行数 = %d, want 5", loginStateCount)
	}
	var mteamState SiteLoginState
	if err := db.Where("site_name = ?", "mteam").First(&mteamState).Error; err != nil {
		t.Fatalf("查询 mteam 登录状态失败: %v", err)
	}
	if mteamState.BanThresholdDays != DefaultBanThresholdDays || mteamState.RemindBeforeDays != DefaultRemindBeforeDays {
		t.Fatalf("mteam preset = (%d,%d), want (%d,%d)", mteamState.BanThresholdDays, mteamState.RemindBeforeDays, DefaultBanThresholdDays, DefaultRemindBeforeDays)
	}
	version, err := sm.GetCurrentVersion()
	if err != nil {
		t.Fatalf("获取版本失败: %v", err)
	}
	if version != CurrentSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, CurrentSchemaVersion)
	}
	if hooks.backupCalls.Load() < 1 {
		t.Fatal("backup hook 未调用")
	}
}

func TestMigrationV8ToV9RollbackOnEncryptError(t *testing.T) {
	db := setupV8MigrationDB(t)
	hooks := &spyHooks{failEncrypt: 3}
	sm := NewSchemaManagerWithHooks(db, "test", hooks.BackupTable, hooks.EncryptCookie, hooks.DecryptCookie)

	if err := sm.RunMigrations(); err == nil {
		t.Fatal("期望加密失败导致迁移返回错误")
	}

	var encryptedCount int64
	if err := db.Model(&SiteSetting{}).Where("cookie_encrypted IS NOT NULL AND cookie_encrypted != ''").Count(&encryptedCount).Error; err != nil {
		t.Fatalf("统计已加密 cookie 失败: %v", err)
	}
	if encryptedCount != 0 {
		t.Fatalf("已加密 cookie 行数 = %d, want 0", encryptedCount)
	}
	version, err := sm.GetCurrentVersion()
	if err != nil {
		t.Fatalf("获取版本失败: %v", err)
	}
	if version != 8 {
		t.Fatalf("schema version = %d, want 8", version)
	}
	if hooks.backupCalls.Load() < 1 {
		t.Fatal("backup hook 未调用")
	}
}

func TestMigrationV8ToV9Idempotent(t *testing.T) {
	db := setupV8MigrationDB(t)
	hooks := &spyHooks{}
	sm := NewSchemaManagerWithHooks(db, "test", hooks.BackupTable, hooks.EncryptCookie, hooks.DecryptCookie)

	if err := sm.RunMigrations(); err != nil {
		t.Fatalf("首次迁移失败: %v", err)
	}
	hooks.encryptCalls.Store(0)
	if err := sm.migrateV8ToV9(db); err != nil {
		t.Fatalf("重复执行 v9 迁移失败: %v", err)
	}
	if calls := hooks.encryptCalls.Load(); calls != 0 {
		t.Fatalf("重复迁移加密调用数 = %d, want 0", calls)
	}
}

func TestMigrationV8ToV9NilHook(t *testing.T) {
	db := setupV8MigrationDB(t)
	sm := NewSchemaManager(db, "test")

	err := sm.RunMigrations()
	if err == nil {
		t.Fatal("期望 nil hook 导致迁移失败")
	}
	if !strings.Contains(err.Error(), "backup hook") && !strings.Contains(err.Error(), "crypto hooks") {
		t.Fatalf("错误 = %v, want mention hook", err)
	}
	version, versionErr := sm.GetCurrentVersion()
	if versionErr != nil {
		t.Fatalf("获取版本失败: %v", versionErr)
	}
	if version != 8 {
		t.Fatalf("schema version = %d, want 8", version)
	}
}

func TestSchemaManager_NewDatabase(t *testing.T) {
	db := setupTestDB(t)
	hooks := &spyHooks{}
	sm := NewSchemaManagerWithHooks(db, "1.0.0", hooks.BackupTable, hooks.EncryptCookie, hooks.DecryptCookie)

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

	hooks := &spyHooks{}
	sm := NewSchemaManagerWithHooks(db, "1.0.0", hooks.BackupTable, hooks.EncryptCookie, hooks.DecryptCookie)

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
