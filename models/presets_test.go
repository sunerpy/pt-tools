package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{ID: "springsunday", Name: "SpringSunday", AuthMethod: "cookie", DefaultBaseURL: ""},
		{ID: "hdsky", Name: "HDSky", AuthMethod: "cookie", DefaultBaseURL: ""},
		{ID: "mteam", Name: "M-Team", AuthMethod: "api_key", DefaultBaseURL: "https://api.m-team.cc", APIUrls: []string{"https://api.m-team.cc", "https://kp.m-team.cc"}},
		{ID: "hddolby", Name: "HDDolby", AuthMethod: "cookie", DefaultBaseURL: ""},
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
		if site.APIUrl != "https://api.m-team.cc" {
			t.Errorf("mteam APIUrl = %q, want %q", site.APIUrl, "https://api.m-team.cc")
		}
		if site.APIUrls != `["https://api.m-team.cc","https://kp.m-team.cc"]` {
			t.Errorf("mteam APIUrls = %q, want JSON array", site.APIUrls)
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

func TestMigrateCmctToSpringSunday_RenameBranch(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &RSSSubscription{}, &TorrentInfo{})

	cmct := SiteSetting{Name: "cmct", AuthMethod: "cookie", Cookie: "ck", Enabled: true}
	require.NoError(t, db.Create(&cmct).Error)
	require.NoError(t, db.Create(&TorrentInfo{SiteName: "cmct", TorrentID: "t1"}).Error)

	require.NoError(t, SyncSitesFromRegistry(db, nil))

	var spring SiteSetting
	require.NoError(t, db.Where("name = ?", "springsunday").First(&spring).Error)
	assert.Equal(t, "ck", spring.Cookie)
	assert.True(t, spring.Enabled)

	var cnt int64
	db.Model(&SiteSetting{}).Where("name = ?", "cmct").Count(&cnt)
	assert.Equal(t, int64(0), cnt)

	var ti TorrentInfo
	require.NoError(t, db.Where("torrent_id = ?", "t1").First(&ti).Error)
	assert.Equal(t, "springsunday", ti.SiteName)
}

func TestSyncSitesFromRegistry_CreateAndUpdate(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &RSSSubscription{})

	require.NoError(t, db.Create(&SiteSetting{Name: "mteam", AuthMethod: "cookie", Cookie: "userck", Enabled: true}).Error)

	require.NoError(t, SyncSitesFromRegistry(db, []RegisteredSite{
		{ID: "MTEAM", Name: "M-Team", AuthMethod: "api_key", DefaultBaseURL: "https://kp.m-team.cc", APIUrls: []string{"https://api.m-team.cc", "https://api2.m-team.cc"}},
		{ID: "hdsky", Name: "HDSky", AuthMethod: "cookie", DefaultBaseURL: "https://hdsky.me"},
	}))

	var mteam SiteSetting
	require.NoError(t, db.Where("name = ?", "mteam").First(&mteam).Error)
	assert.Equal(t, "api_key", mteam.AuthMethod)
	assert.Equal(t, "userck", mteam.Cookie)
	assert.True(t, mteam.Enabled)
	assert.Equal(t, "https://api.m-team.cc", mteam.APIUrl)
	assert.Contains(t, mteam.APIUrls, "api2.m-team.cc")

	var hdsky SiteSetting
	require.NoError(t, db.Where("name = ?", "hdsky").First(&hdsky).Error)
	assert.False(t, hdsky.Enabled)
	assert.True(t, hdsky.IsBuiltin)
}

func TestMigrateCmctToSpringSunday_UserInfoReparent(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &RSSSubscription{})
	require.NoError(t, db.Exec("CREATE TABLE user_info (id INTEGER PRIMARY KEY, site TEXT, username TEXT)").Error)
	require.NoError(t, db.Exec("INSERT INTO user_info (site, username) VALUES ('cmct','alice')").Error)

	require.NoError(t, db.Create(&SiteSetting{Name: "cmct", AuthMethod: "cookie", Cookie: "ck"}).Error)

	require.NoError(t, SyncSitesFromRegistry(db, nil))

	var uname string
	require.NoError(t, db.Raw("SELECT username FROM user_info WHERE site = ?", "springsunday").Scan(&uname).Error)
	assert.Equal(t, "alice", uname)
}

func TestMigrateCmctToSpringSunday_UserInfoKeepExisting(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &RSSSubscription{})
	require.NoError(t, db.Exec("CREATE TABLE user_info (id INTEGER PRIMARY KEY, site TEXT, username TEXT)").Error)
	require.NoError(t, db.Exec("INSERT INTO user_info (site, username) VALUES ('cmct','old')").Error)
	require.NoError(t, db.Exec("INSERT INTO user_info (site, username) VALUES ('springsunday','current')").Error)

	require.NoError(t, db.Create(&SiteSetting{Name: "cmct", AuthMethod: "cookie"}).Error)
	require.NoError(t, db.Create(&SiteSetting{Name: "springsunday", AuthMethod: "cookie"}).Error)

	require.NoError(t, SyncSitesFromRegistry(db, nil))

	var cmctCount int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM user_info WHERE site = ?", "cmct").Scan(&cmctCount).Error)
	assert.Equal(t, int64(0), cmctCount)

	var uname string
	require.NoError(t, db.Raw("SELECT username FROM user_info WHERE site = ?", "springsunday").Scan(&uname).Error)
	assert.Equal(t, "current", uname)
}

func TestMigrateCmctToSpringSunday_MergeExisting(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &RSSSubscription{})

	// both cmct and springsunday exist; springsunday lacks cookie/apikey/enabled
	cmct := SiteSetting{Name: "cmct", AuthMethod: "cookie", Cookie: "ck", APIKey: "ak", Enabled: true}
	require.NoError(t, db.Create(&cmct).Error)
	spring := SiteSetting{Name: "springsunday", AuthMethod: "cookie"}
	require.NoError(t, db.Create(&spring).Error)

	// an RSS attached to cmct should be reparented to springsunday
	require.NoError(t, db.Create(&RSSSubscription{SiteID: cmct.ID, Name: "r", URL: "http://x", IntervalMinutes: 5}).Error)

	require.NoError(t, SyncSitesFromRegistry(db, []RegisteredSite{
		{ID: "springsunday", Name: "SpringSunday", AuthMethod: "cookie"},
	}))

	// cmct removed, springsunday inherited user config
	var cnt int64
	db.Model(&SiteSetting{}).Where("name = ?", "cmct").Count(&cnt)
	assert.Equal(t, int64(0), cnt)

	var merged SiteSetting
	require.NoError(t, db.Where("name = ?", "springsunday").First(&merged).Error)
	assert.Equal(t, "ck", merged.Cookie)
	assert.Equal(t, "ak", merged.APIKey)
	assert.True(t, merged.Enabled)

	// RSS reparented
	var rss RSSSubscription
	require.NoError(t, db.Where("name = ?", "r").First(&rss).Error)
	assert.Equal(t, merged.ID, rss.SiteID)
}

func TestMigrateExampleRSS(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &RSSSubscription{})

	require.NoError(t, db.Create(&RSSSubscription{Name: "example", URL: "https://example.com/rss", IntervalMinutes: 5}).Error)
	require.NoError(t, db.Create(&RSSSubscription{Name: "real", URL: "https://hdsky.me/rss", IntervalMinutes: 5}).Error)

	require.NoError(t, MigrateExampleRSS(db))

	var ex RSSSubscription
	require.NoError(t, db.Where("name = ?", "example").First(&ex).Error)
	assert.True(t, ex.IsExample)

	var real RSSSubscription
	require.NoError(t, db.Where("name = ?", "real").First(&real).Error)
	assert.False(t, real.IsExample)
}
