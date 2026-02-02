package models

import (
	"strings"

	"gorm.io/gorm"
)

// SyncSitesFromRegistry 从注册表同步站点到数据库
// 每次应用启动时调用，确保数据库中包含所有注册的站点
// 保留用户配置（cookie/api_key/enabled），不删除任何用户数据
func SyncSitesFromRegistry(db *gorm.DB, registeredSites []RegisteredSite) error {
	// 首先处理 cmct -> springsunday 的迁移（必须在同步之前）
	if err := migrateCmctToSpringSunday(db); err != nil {
		return err
	}

	// 获取数据库中现有站点
	var existingSites []SiteSetting
	if err := db.Find(&existingSites).Error; err != nil {
		return err
	}

	existingMap := make(map[string]SiteSetting)
	for _, site := range existingSites {
		existingMap[site.Name] = site
	}

	// 添加或更新注册表中的站点（不删除任何站点）
	for _, regSite := range registeredSites {
		name := strings.ToLower(regSite.ID)
		if existing, exists := existingMap[name]; exists {
			// 站点已存在，更新认证方式和BaseURL（保留用户的cookie/api_key/enabled）
			needsUpdate := existing.AuthMethod != regSite.AuthMethod ||
				(existing.BaseURL == "" && regSite.DefaultBaseURL != "") ||
				!existing.IsBuiltin
			if needsUpdate {
				existing.AuthMethod = regSite.AuthMethod
				existing.IsBuiltin = true
				if existing.BaseURL == "" {
					existing.BaseURL = regSite.DefaultBaseURL
				}
				if err := db.Save(&existing).Error; err != nil {
					return err
				}
			}
		} else {
			// 站点不存在，创建新记录
			newSite := SiteSetting{
				Name:       name,
				AuthMethod: regSite.AuthMethod,
				Enabled:    false,
				BaseURL:    regSite.DefaultBaseURL,
				IsBuiltin:  true,
			}
			if err := db.Create(&newSite).Error; err != nil {
				return err
			}
		}
	}

	return nil
}

// migrateCmctToSpringSunday 将旧的 cmct 站点迁移到 springsunday
// 包括：站点设置、种子信息、用户信息
func migrateCmctToSpringSunday(db *gorm.DB) error {
	// 检查是否存在 cmct 站点
	var cmctSite SiteSetting
	err := db.Where("name = ?", "cmct").First(&cmctSite).Error
	if err == gorm.ErrRecordNotFound {
		// 没有 cmct 站点，无需迁移
		return nil
	}
	if err != nil {
		return err
	}

	// 检查是否已存在 springsunday 站点
	var springSite SiteSetting
	err = db.Where("name = ?", "springsunday").First(&springSite).Error

	switch err {
	case nil:
		// springsunday 已存在，需要合并数据
		// 1. 将 cmct 的 RSS 关联到 springsunday
		if updateErr := db.Model(&RSSSubscription{}).
			Where("site_id = ?", cmctSite.ID).
			Update("site_id", springSite.ID).Error; updateErr != nil {
			return updateErr
		}

		// 2. 如果 cmct 有用户配置但 springsunday 没有，则复制过来
		if cmctSite.Cookie != "" && springSite.Cookie == "" {
			springSite.Cookie = cmctSite.Cookie
		}
		if cmctSite.APIKey != "" && springSite.APIKey == "" {
			springSite.APIKey = cmctSite.APIKey
		}
		if cmctSite.Enabled && !springSite.Enabled {
			springSite.Enabled = cmctSite.Enabled
		}
		if saveErr := db.Save(&springSite).Error; saveErr != nil {
			return saveErr
		}

		// 3. 删除 cmct 站点
		if delErr := db.Delete(&cmctSite).Error; delErr != nil {
			return delErr
		}
	case gorm.ErrRecordNotFound:
		// springsunday 不存在，直接重命名 cmct
		cmctSite.Name = "springsunday"
		if saveErr := db.Save(&cmctSite).Error; saveErr != nil {
			return saveErr
		}
	default:
		return err
	}

	// 更新 torrent_infos 表
	if db.Migrator().HasTable("torrent_infos") {
		db.Table("torrent_infos").Where("site_name = ?", "cmct").Update("site_name", "springsunday")
	}

	// 更新 user_info 表：如果 springsunday 已存在且有数据，保留；否则从 cmct 迁移
	if db.Migrator().HasTable("user_info") {
		// 检查 springsunday 是否有有效数据
		var springUserInfo struct {
			Username string
		}
		err := db.Table("user_info").Where("site = ?", "springsunday").Select("username").First(&springUserInfo).Error

		if err == gorm.ErrRecordNotFound || springUserInfo.Username == "" {
			// springsunday 没有有效数据，从 cmct 迁移
			// 先删除空的 springsunday 记录（如果存在）
			db.Table("user_info").Where("site = ?", "springsunday").Delete(nil)
			// 将 cmct 重命名为 springsunday
			db.Table("user_info").Where("site = ?", "cmct").Update("site", "springsunday")
		} else {
			// springsunday 有有效数据，删除 cmct 的记录
			db.Table("user_info").Where("site = ?", "cmct").Delete(nil)
		}
	}

	return nil
}

// RegisteredSite 表示从注册表获取的站点信息
type RegisteredSite struct {
	ID             string
	Name           string
	AuthMethod     string // "cookie" 或 "api_key"
	DefaultBaseURL string
}

// ExampleRSSPatterns 示例 RSS URL 的特征模式
// 包含这些模式的 URL 被认为是示例配置
var ExampleRSSPatterns = []string{
	"springxxx.xxx",  // SpringSunday 示例
	"hdsky.xxx",      // HDSKY 示例
	"rss.m-team.xxx", // MTEAM 示例
	"example.com",    // 通用示例
	"xxx.xxx",        // 通用占位符
}

// IsExampleURL 检查 URL 是否为示例 URL
func IsExampleURL(url string) bool {
	lowerURL := strings.ToLower(url)
	for _, pattern := range ExampleRSSPatterns {
		if strings.Contains(lowerURL, pattern) {
			return true
		}
	}
	return false
}

// MigrateExampleRSS 迁移旧版本的示例 RSS 配置
// 将 URL 包含示例模式的 RSS 订阅标记为 IsExample=true
func MigrateExampleRSS(db *gorm.DB) error {
	// 查找所有未标记为示例但 URL 包含示例模式的 RSS 订阅
	var rssSubscriptions []RSSSubscription
	if err := db.Where("is_example = ?", false).Find(&rssSubscriptions).Error; err != nil {
		return err
	}

	for _, rss := range rssSubscriptions {
		if IsExampleURL(rss.URL) {
			// 更新为示例配置
			if err := db.Model(&RSSSubscription{}).Where("id = ?", rss.ID).Update("is_example", true).Error; err != nil {
				return err
			}
		}
	}

	return nil
}
