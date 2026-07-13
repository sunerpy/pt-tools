package models

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func closedDB(t *testing.T, tables ...any) *gorm.DB {
	t.Helper()
	db := newMemDB(t, tables...)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	return db
}

func TestErrorPaths_ClosedDB(t *testing.T) {
	db := closedDB(t, &SiteSetting{}, &RSSSubscription{}, &FilterRule{},
		&RSSFilterAssociation{}, &SiteLoginState{})

	siteRepo := NewSiteRepository(db)
	_, err := siteRepo.ListSites()
	assert.Error(t, err)
	_, err = siteRepo.ListEnabledSites()
	assert.Error(t, err)
	_, err = siteRepo.SiteExistsByName("x")
	assert.Error(t, err)
	_, err = siteRepo.CreateSite(SiteData{Name: "n", AuthMethod: "cookie"})
	assert.Error(t, err)

	assocDB := NewRSSFilterAssociationDB(db)
	_, err = assocDB.GetByRSSID(1)
	assert.Error(t, err)
	_, err = assocDB.GetByFilterRuleID(1)
	assert.Error(t, err)
	assert.Error(t, assocDB.SetFilterRulesForRSS(1, []uint{2}))

	loginRepo := NewSiteLoginStateRepository(db)
	assert.Error(t, loginRepo.UpsertLoginState("s", map[string]any{"LastProbeStatus": "OK"}))
	_, err = loginRepo.GetLoginState("s")
	assert.Error(t, err)
	_, err = loginRepo.ListLoginStates(false)
	assert.Error(t, err)

	assert.Error(t, MigrateExampleRSS(db))
	assert.Error(t, SyncSitesFromRegistry(db, []RegisteredSite{{ID: "x", AuthMethod: "cookie"}}))
	assert.Error(t, setDefaultsForSiteSetting(db))
	assert.Error(t, migrateV1ToV2(db))

	assert.Error(t, loginRepo.UpdateProbeResult("s", "OK", nil, nil, nil))
	assert.Error(t, loginRepo.ClampLastVisit("s", time.Now(), time.Now()))
	assert.Error(t, loginRepo.IncrProbeFailures("s"))
	assert.Error(t, loginRepo.ResetProbeFailures("s"))
	assert.Error(t, assocDB.DeleteByRSSID(1))
	assert.Error(t, assocDB.DeleteByFilterRuleID(1))
	_, err = assocDB.HasAssociations(1)
	assert.Error(t, err)
	_, err = assocDB.Exists(1, 2)
	assert.Error(t, err)

	sm := NewSchemaManager(db, "1.0.0")
	assert.Error(t, sm.RunMigrations())
	_, err = sm.isLegacyDatabase()
	assert.Error(t, err)
}

type dynamicSiteSettingLegacyRow struct {
	ID           uint   `gorm:"primaryKey"`
	Name         string `gorm:"uniqueIndex;size:64;not null"`
	DisplayName  string `gorm:"size:128"`
	BaseURL      string `gorm:"size:512"`
	Enabled      bool
	AuthMethod   string `gorm:"size:16;not null"`
	Cookie       string `gorm:"size:2048"`
	APIKey       string `gorm:"size:512"`
	APIURL       string `gorm:"size:512"`
	DownloaderID *uint  `gorm:"index"`
	ParserConfig string `gorm:"type:text"`
	IsBuiltin    bool
	TemplateID   *uint `gorm:"index"`
}

func (dynamicSiteSettingLegacyRow) TableName() string { return "dynamic_site_settings" }

func TestMigrateV4ToV5_CreateAndMerge(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &dynamicSiteSettingLegacyRow{})

	dlID := uint(7)
	require.NoError(t, db.Create(&dynamicSiteSettingLegacyRow{
		Name: "newsite", DisplayName: "New Site", BaseURL: "https://new.example",
		Enabled: true, AuthMethod: "cookie", Cookie: "ck", APIKey: "ak",
		APIURL: "https://api.new", DownloaderID: &dlID, ParserConfig: "{}", IsBuiltin: true,
	}).Error)
	tmpl := uint(3)
	require.NoError(t, db.Create(&dynamicSiteSettingLegacyRow{
		Name: "existing", DisplayName: "Legacy Name", BaseURL: "https://legacy",
		AuthMethod: "cookie", Cookie: "legacyck", APIKey: "legacyak", APIURL: "https://legacy-api",
		DownloaderID: &tmpl, ParserConfig: "{\"a\":1}", TemplateID: &tmpl, IsBuiltin: true,
	}).Error)

	require.NoError(t, db.Create(&SiteSetting{Name: "existing", AuthMethod: "cookie"}).Error)

	require.NoError(t, migrateV4ToV5(db))

	assert.False(t, db.Migrator().HasTable("dynamic_site_settings"))

	var created SiteSetting
	require.NoError(t, db.Where("name = ?", "newsite").First(&created).Error)
	assert.Equal(t, "New Site", created.DisplayName)
	assert.Equal(t, "ck", created.Cookie)
	require.NotNil(t, created.DownloaderID)
	assert.Equal(t, uint(7), *created.DownloaderID)

	var merged SiteSetting
	require.NoError(t, db.Where("name = ?", "existing").First(&merged).Error)
	assert.Equal(t, "Legacy Name", merged.DisplayName)
	assert.Equal(t, "legacyck", merged.Cookie)
	assert.Equal(t, "https://legacy-api", merged.APIUrl)
	assert.Equal(t, "{\"a\":1}", merged.ParserConfig)
	require.NotNil(t, merged.DownloaderID)
	require.NotNil(t, merged.TemplateID)
	assert.True(t, merged.IsBuiltin)
}

func TestMigrateV4ToV5_EmptyLegacyTable(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &dynamicSiteSettingLegacyRow{})
	require.NoError(t, db.Create(&SiteSetting{Name: "s1", AuthMethod: "cookie"}).Error)

	require.NoError(t, migrateV4ToV5(db))

	assert.False(t, db.Migrator().HasTable("dynamic_site_settings"))
	var s SiteSetting
	require.NoError(t, db.Where("name = ?", "s1").First(&s).Error)
	assert.Equal(t, "s1", s.DisplayName)
	assert.True(t, s.IsBuiltin)
}

func TestMigrateV5ToV6_AddsColumns(t *testing.T) {
	db := newMemDB(t)
	require.NoError(t, db.Exec("CREATE TABLE rss_subscriptions (id INTEGER PRIMARY KEY)").Error)

	require.NoError(t, migrateV5ToV6(db))

	assert.True(t, db.Migrator().HasColumn(&RSSSubscription{}, "NotifyMode"))
	assert.True(t, db.Migrator().HasColumn(&RSSSubscription{}, "NotifyConfIDs"))
	assert.True(t, db.Migrator().HasColumn(&RSSSubscription{}, "MaxNotificationsPerHour"))
	assert.True(t, db.Migrator().HasTable(&RSSNotificationLog{}))
}

func TestMigrateV6ToV7_AddsPurposeColumn(t *testing.T) {
	db := newMemDB(t)
	require.NoError(t, db.Exec("CREATE TABLE filter_rules (id INTEGER PRIMARY KEY)").Error)

	require.NoError(t, migrateV6ToV7(db))

	assert.True(t, db.Migrator().HasColumn(&FilterRule{}, "Purpose"))
}

func TestMigrateV7ToV8_AddsQuietHours(t *testing.T) {
	db := newMemDB(t)
	require.NoError(t, db.Exec("CREATE TABLE notification_conf (id INTEGER PRIMARY KEY)").Error)

	require.NoError(t, migrateV7ToV8(db))

	assert.True(t, db.Migrator().HasColumn(&NotificationConf{}, "QuietHoursStart"))
	assert.True(t, db.Migrator().HasColumn(&NotificationConf{}, "QuietHoursEnd"))
}

func TestRunMigrations_LegacyChainV1ToV10(t *testing.T) {
	db := newMemDB(
		t,
		&SchemaVersion{}, &SettingsGlobal{}, &SiteSetting{}, &RSSSubscription{},
		&FilterRule{}, &NotificationConf{},
	)

	require.NoError(t, db.Create(&SiteSetting{Name: "hdsky", AuthMethod: "cookie", Cookie: "uid=1;pass=2", Enabled: true}).Error)
	require.NoError(t, db.Create(&RSSSubscription{Name: "ex", URL: "https://example.com/rss", IntervalMinutes: 5}).Error)

	backup := func(*gorm.DB, string) (string, error) { return "b", nil }
	enc := func(p string) (string, error) { return "enc:" + p, nil }
	dec := func(c string) (string, error) { return c[4:], nil }

	sm := NewSchemaManagerWithHooks(db, "1.0.0", backup, enc, dec)
	require.NoError(t, sm.RunMigrations())

	got, err := sm.GetCurrentVersion()
	require.NoError(t, err)
	assert.Equal(t, CurrentSchemaVersion, got)

	assert.True(t, db.Migrator().HasColumn(&SiteSetting{}, "CookieEncrypted"))
	assert.True(t, db.Migrator().HasTable(&SiteLoginState{}))
	assert.True(t, db.Migrator().HasColumn(&SiteLoginState{}, "ApiLastLoginAt"))

	var cipher string
	require.NoError(t, db.Model(&SiteSetting{}).Where("name = ?", "hdsky").Pluck("cookie_encrypted", &cipher).Error)
	assert.Equal(t, "enc:uid=1;pass=2", cipher)

	var rss RSSSubscription
	require.NoError(t, db.Where("name = ?", "ex").First(&rss).Error)
	assert.True(t, rss.IsExample)

	_, ok := GetMigrationState(db, 10)
	assert.True(t, ok)
}

func TestRunMigrations_FreshInstall(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &SettingsGlobal{}, &RSSSubscription{})
	sm := NewSchemaManager(db, "1.2.3")

	require.NoError(t, sm.RunMigrations())

	got, err := sm.GetCurrentVersion()
	require.NoError(t, err)
	assert.Equal(t, CurrentSchemaVersion, got)
}

func TestGetCurrentVersion_NoTable(t *testing.T) {
	db := newMemDB(t)
	sm := NewSchemaManager(db, "1.0.0")

	got, err := sm.GetCurrentVersion()
	require.NoError(t, err)
	assert.Equal(t, 0, got)
}

func TestSchemaManager_RecordAndReadVersion(t *testing.T) {
	db := newMemDB(t)
	sm := NewSchemaManager(db, "9.9.9")
	require.NoError(t, sm.EnsureSchemaVersionTable())
	require.NoError(t, sm.RecordVersion(3, "test v3"))

	got, err := sm.GetCurrentVersion()
	require.NoError(t, err)
	assert.Equal(t, 3, got)
}

func TestSchemaManager_IsLegacyByRSSOnly(t *testing.T) {
	db := newMemDB(t, &SchemaVersion{}, &SettingsGlobal{}, &SiteSetting{}, &RSSSubscription{},
		&FilterRule{}, &NotificationConf{})
	require.NoError(t, db.Create(&RSSSubscription{Name: "r", URL: "http://x", IntervalMinutes: 5}).Error)

	backup := func(*gorm.DB, string) (string, error) { return "b", nil }
	enc := func(p string) (string, error) { return "e:" + p, nil }
	dec := func(c string) (string, error) { return c[2:], nil }
	sm := NewSchemaManagerWithHooks(db, "1.0.0", backup, enc, dec)

	require.NoError(t, sm.RunMigrations())
	got, err := sm.GetCurrentVersion()
	require.NoError(t, err)
	assert.Equal(t, CurrentSchemaVersion, got)
}

func TestMigrateV8ToV9_NilHooks(t *testing.T) {
	db := newMemDB(t, &SiteSetting{})
	sm := NewSchemaManager(db, "1.0.0")
	assert.Error(t, sm.migrateV8ToV9(db))

	sm.BackupTable = func(*gorm.DB, string) (string, error) { return "b", nil }
	assert.Error(t, sm.migrateV8ToV9(db))
}

func TestMigrateV8ToV9_DecryptSanityMismatch(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &SiteLoginState{})
	require.NoError(t, db.Create(&SiteSetting{Name: "s", AuthMethod: "cookie", Cookie: "raw"}).Error)

	sm := NewSchemaManagerWithHooks(
		db, "1.0.0",
		func(*gorm.DB, string) (string, error) { return "b", nil },
		func(p string) (string, error) { return "e:" + p, nil },
		func(string) (string, error) { return "TAMPERED", nil },
	)
	err := sm.migrateV8ToV9(db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sanity")
}

func TestMigrateV9ToV10_NilBackupHook(t *testing.T) {
	db := newMemDB(t, &SiteLoginState{})
	sm := NewSchemaManager(db, "1.0.0")
	assert.Error(t, sm.migrateV9ToV10(db))
}

func TestMigrateV2ToV3_UserInfoTableExists(t *testing.T) {
	db := newMemDB(t, &SiteSetting{})
	require.NoError(t, db.Exec("CREATE TABLE user_info (id INTEGER PRIMARY KEY)").Error)
	require.NoError(t, migrateV2ToV3(db))
	assert.True(t, db.Migrator().HasTable("user_info"))
}

func TestMigrateV8ToV9_IdempotentColumnPresent(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &SiteLoginState{})
	require.NoError(t, db.Create(&SiteSetting{Name: "s", AuthMethod: "cookie", Cookie: "raw"}).Error)

	backup := func(*gorm.DB, string) (string, error) { return "b", nil }
	enc := func(p string) (string, error) { return "e:" + p, nil }
	dec := func(c string) (string, error) { return c[2:], nil }
	sm := NewSchemaManagerWithHooks(db, "1.0.0", backup, enc, dec)

	require.NoError(t, sm.migrateV8ToV9(db))

	var cipher string
	require.NoError(t, db.Model(&SiteSetting{}).Where("name = ?", "s").Pluck("cookie_encrypted", &cipher).Error)
	assert.Equal(t, "e:raw", cipher)
}

func TestMigrateV1ToV2_Full(t *testing.T) {
	db := newMemDB(t, &SettingsGlobal{}, &RSSSubscription{})

	require.NoError(t, db.Create(&SettingsGlobal{DefaultConcurrency: 0}).Error)
	require.NoError(t, db.Create(&RSSSubscription{Name: "ex", URL: "https://example.com/rss", IntervalMinutes: 5}).Error)

	require.NoError(t, migrateV1ToV2(db))

	var gl SettingsGlobal
	require.NoError(t, db.First(&gl).Error)
	assert.Equal(t, DefaultConcurrency, gl.DefaultConcurrency)

	var rss RSSSubscription
	require.NoError(t, db.Where("name = ?", "ex").First(&rss).Error)
	assert.True(t, rss.IsExample)
}

func TestMigrators_V5V6V7V8(t *testing.T) {
	db := newMemDB(t, &RSSSubscription{}, &FilterRule{}, &NotificationConf{}, &RSSNotificationLog{})

	// v5→v6 is idempotent when columns already exist (AutoMigrate created them)
	require.NoError(t, migrateV5ToV6(db))
	assert.True(t, db.Migrator().HasTable(&RSSNotificationLog{}))

	// v6→v7 backfills purpose; column already present via AutoMigrate
	require.NoError(t, migrateV6ToV7(db))

	// v7→v8 adds quiet-hours columns; idempotent when present
	require.NoError(t, migrateV7ToV8(db))
}

func TestMigrators_V6V7_NoTables(t *testing.T) {
	// FilterRule / NotificationConf tables absent → migrators no-op cleanly
	db := newMemDB(t, &RSSSubscription{})
	require.NoError(t, migrateV6ToV7(db))
	require.NoError(t, migrateV7ToV8(db))
}

func TestMigrateV4ToV5_NoLegacyTable(t *testing.T) {
	db := newMemDB(t, &SiteSetting{})
	// no dynamic_site_settings table → sets defaults only
	require.NoError(t, db.Create(&SiteSetting{Name: "abc", AuthMethod: "cookie"}).Error)
	require.NoError(t, migrateV4ToV5(db))

	var site SiteSetting
	require.NoError(t, db.Where("name = ?", "abc").First(&site).Error)
	assert.Equal(t, "abc", site.DisplayName) // defaulted from Name
	assert.True(t, site.IsBuiltin)
}

func TestMigrateV2ToV3_CreatesUserInfo(t *testing.T) {
	db := newMemDB(t, &SiteSetting{})
	assert.False(t, db.Migrator().HasTable("user_info_records"))
	require.NoError(t, migrateV2ToV3(db))
	assert.True(t, db.Migrator().HasTable("user_info_records"))
}

func TestMigrateV3ToV4_Noop(t *testing.T) {
	db := newMemDB(t, &SiteSetting{})
	require.NoError(t, migrateV3ToV4(db))
}
