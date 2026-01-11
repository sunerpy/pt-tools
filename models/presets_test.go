package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestIsExampleURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{"SpringSunday示例URL", "https://springxxx.xxx/rss", true},
		{"HDSKY示例URL", "https://hdsky.xxx/torrentrss.php?xxx", true},
		{"MTEAM示例URL", "https://rss.m-team.xxx/api/rss/xxx", true},
		{"通用示例URL", "https://example.com/rss", true},
		{"真实SpringSunday URL", "https://springsunday.net/rss", false},
		{"真实HDSKY URL", "https://hdsky.me/torrentrss.php?passkey=abc", false},
		{"真实MTEAM URL", "https://api.m-team.cc/api/rss/abc", false},
		{"空URL", "", false},
		{"大小写混合", "https://HDSKY.XXX/rss", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsExampleURL(tt.url)
			if result != tt.expected {
				t.Errorf("IsExampleURL(%q) = %v, want %v", tt.url, result, tt.expected)
			}
		})
	}
}

func TestSyncSitesFromRegistry(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}

	if err := db.AutoMigrate(&SiteSetting{}, &RSSSubscription{}); err != nil {
		t.Fatalf("迁移表结构失败: %v", err)
	}

	// 模拟注册的站点
	registeredSites := []RegisteredSite{
		{ID: "springsunday", Name: "SpringSunday", AuthMethod: "cookie", DefaultAPIUrl: ""},
		{ID: "hdsky", Name: "HDSky", AuthMethod: "cookie", DefaultAPIUrl: ""},
		{ID: "mteam", Name: "M-Team", AuthMethod: "api_key", DefaultAPIUrl: "https://api.m-team.cc"},
		{ID: "hddolby", Name: "HDDolby", AuthMethod: "cookie", DefaultAPIUrl: ""},
	}

	// 执行同步
	if err := SyncSitesFromRegistry(db, registeredSites); err != nil {
		t.Fatalf("SyncSitesFromRegistry 失败: %v", err)
	}

	// 验证站点已创建
	var sites []SiteSetting
	if err := db.Find(&sites).Error; err != nil {
		t.Fatalf("查询站点失败: %v", err)
	}

	if len(sites) != 4 {
		t.Errorf("期望 4 个站点，实际 %d 个", len(sites))
	}

	// 验证站点属性
	siteMap := make(map[string]SiteSetting)
	for _, site := range sites {
		siteMap[site.Name] = site
	}

	if site, ok := siteMap["mteam"]; ok {
		if site.AuthMethod != "api_key" {
			t.Errorf("mteam AuthMethod = %q, want %q", site.AuthMethod, "api_key")
		}
	} else {
		t.Error("未找到 mteam 站点")
	}

	// 再次执行同步，不应创建重复记录
	if err := SyncSitesFromRegistry(db, registeredSites); err != nil {
		t.Fatalf("第二次 SyncSitesFromRegistry 失败: %v", err)
	}

	var sitesAfter []SiteSetting
	if err := db.Find(&sitesAfter).Error; err != nil {
		t.Fatalf("查询站点失败: %v", err)
	}

	if len(sitesAfter) != 4 {
		t.Errorf("重复同步后期望 4 个站点，实际 %d 个", len(sitesAfter))
	}
}

func TestSyncSitesFromRegistry_PreserveUserData(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}

	if err := db.AutoMigrate(&SiteSetting{}, &RSSSubscription{}); err != nil {
		t.Fatalf("迁移表结构失败: %v", err)
	}

	// 先创建用户已配置的站点
	userSite := SiteSetting{
		Name:       "customsite",
		AuthMethod: "cookie",
		Cookie:     "user_cookie",
		Enabled:    true,
	}
	if err := db.Create(&userSite).Error; err != nil {
		t.Fatalf("创建用户站点失败: %v", err)
	}

	// 同步注册的站点（不包含 customsite）
	registeredSites := []RegisteredSite{
		{ID: "springsunday", Name: "SpringSunday", AuthMethod: "cookie"},
	}
	if err := SyncSitesFromRegistry(db, registeredSites); err != nil {
		t.Fatalf("同步失败: %v", err)
	}

	// 验证用户站点未被删除
	var sites []SiteSetting
	if err := db.Find(&sites).Error; err != nil {
		t.Fatalf("查询站点失败: %v", err)
	}

	if len(sites) != 2 {
		t.Errorf("期望 2 个站点（用户站点 + 注册站点），实际 %d 个", len(sites))
	}

	// 验证用户数据被保留
	var customSite SiteSetting
	if err := db.Where("name = ?", "customsite").First(&customSite).Error; err != nil {
		t.Fatal("用户站点 customsite 应该被保留")
	}

	if customSite.Cookie != "user_cookie" {
		t.Errorf("用户 Cookie 应被保留，期望 user_cookie，实际 %s", customSite.Cookie)
	}
}

func TestSyncSitesFromRegistry_MigrateCmct(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}

	if err := db.AutoMigrate(&SiteSetting{}, &RSSSubscription{}); err != nil {
		t.Fatalf("迁移表结构失败: %v", err)
	}

	// 创建旧的 cmct 站点（模拟旧数据库）
	oldSite := SiteSetting{
		Name:       "cmct",
		AuthMethod: "cookie",
		Cookie:     "test_cookie",
		Enabled:    true,
	}
	if err := db.Create(&oldSite).Error; err != nil {
		t.Fatalf("创建旧站点失败: %v", err)
	}

	// 同步新的注册站点（包含 springsunday）
	registeredSites := []RegisteredSite{
		{ID: "springsunday", Name: "SpringSunday", AuthMethod: "cookie"},
	}
	if err := SyncSitesFromRegistry(db, registeredSites); err != nil {
		t.Fatalf("同步失败: %v", err)
	}

	// 验证 cmct 被重命名为 springsunday
	var sites []SiteSetting
	if err := db.Find(&sites).Error; err != nil {
		t.Fatalf("查询站点失败: %v", err)
	}

	if len(sites) != 1 {
		t.Errorf("期望 1 个站点，实际 %d 个", len(sites))
	}

	if sites[0].Name != "springsunday" {
		t.Errorf("站点名应为 springsunday，实际为 %s", sites[0].Name)
	}

	// 验证用户配置被保留
	if sites[0].Cookie != "test_cookie" {
		t.Errorf("Cookie 应被保留，期望 test_cookie，实际 %s", sites[0].Cookie)
	}

	if !sites[0].Enabled {
		t.Error("Enabled 应被保留为 true")
	}
}
