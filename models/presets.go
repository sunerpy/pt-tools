package models

import (
	"encoding/json"
	"errors"
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
		apiUrlsJSON := encodeAPIUrls(regSite.APIUrls)
		apiUrl := ""
		if len(regSite.APIUrls) > 0 {
			apiUrl = regSite.APIUrls[0]
		}

		if existing, exists := existingMap[name]; exists {
			// 站点已存在，强制对齐认证方式与默认 URL（保留用户的cookie/api_key/enabled）
			existing.AuthMethod = regSite.AuthMethod
			existing.IsBuiltin = true
			if regSite.DefaultBaseURL != "" {
				existing.BaseURL = regSite.DefaultBaseURL
			}
			existing.APIUrl = apiUrl
			existing.APIUrls = apiUrlsJSON
			if err := db.Save(&existing).Error; err != nil {
				return err
			}
		} else {
			// 站点不存在，创建新记录
			newSite := SiteSetting{
				Name:       name,
				AuthMethod: regSite.AuthMethod,
				Enabled:    false,
				BaseURL:    regSite.DefaultBaseURL,
				IsBuiltin:  true,
				APIUrl:     apiUrl,
				APIUrls:    apiUrlsJSON,
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
//
// 全部读写在一个事务里：任一步失败整体回滚，下次启动重试。原来后面几步不在事务里、错误也不检查，
// 种子记录改名撞唯一索引时 cmct 站点照样删掉，种子记录还挂在 cmct 下，用户数据只迁了一半。
func migrateCmctToSpringSunday(db *gorm.DB) error {
	// 检查是否存在 cmct 站点
	var cmctSite SiteSetting
	err := db.Where("name = ?", "cmct").First(&cmctSite).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// 没有 cmct 站点，无需迁移
		return nil
	}
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		return migrateCmctTx(tx, cmctSite)
	})
}

func migrateCmctTx(tx *gorm.DB, cmctSite SiteSetting) error {
	// 检查是否已存在 springsunday 站点
	var springSite SiteSetting
	err := tx.Where("name = ?", "springsunday").First(&springSite).Error

	switch {
	case err == nil:
		// springsunday 已存在，需要合并数据
		// 1. 将 cmct 的 RSS 关联到 springsunday
		if updateErr := tx.Model(&RSSSubscription{}).
			Where("site_id = ?", cmctSite.ID).
			Update("site_id", springSite.ID).Error; updateErr != nil {
			return updateErr
		}

		// 2. 如果 cmct 有用户配置但 springsunday 没有，则复制过来。Cookie 看密文与旧明文两列，一起复制。
		cmctHasCookie := cmctSite.CookieEncrypted != "" || cmctSite.Cookie != ""
		springHasCookie := springSite.CookieEncrypted != "" || springSite.Cookie != ""
		if cmctHasCookie && !springHasCookie {
			springSite.Cookie = cmctSite.Cookie
			springSite.CookieEncrypted = cmctSite.CookieEncrypted
		}
		if cmctSite.APIKey != "" && springSite.APIKey == "" {
			springSite.APIKey = cmctSite.APIKey
		}
		if cmctSite.Enabled && !springSite.Enabled {
			springSite.Enabled = cmctSite.Enabled
		}
		if saveErr := tx.Save(&springSite).Error; saveErr != nil {
			return saveErr
		}

		// 3. 删除 cmct 站点
		if delErr := tx.Delete(&cmctSite).Error; delErr != nil {
			return delErr
		}
	case errors.Is(err, gorm.ErrRecordNotFound):
		// springsunday 不存在，直接重命名 cmct
		cmctSite.Name = "springsunday"
		if saveErr := tx.Save(&cmctSite).Error; saveErr != nil {
			return saveErr
		}
	default:
		return err
	}

	// 更新 torrent_infos 表。(site_name, torrent_id) 唯一：springsunday 已有同一种子 ID 的记录时
	// 保留它、删掉 cmct 那条重复，其余改名，否则整批改名会撞唯一索引
	if tx.Migrator().HasTable("torrent_infos") {
		if err := tx.Exec(`DELETE FROM torrent_infos WHERE site_name = ? AND torrent_id IN
			(SELECT torrent_id FROM torrent_infos WHERE site_name = ?)`, "cmct", "springsunday").Error; err != nil {
			return err
		}
		if err := tx.Table("torrent_infos").Where("site_name = ?", "cmct").
			Update("site_name", "springsunday").Error; err != nil {
			return err
		}
	}

	// 更新 user_info 表：如果 springsunday 已存在且有数据，保留；否则从 cmct 迁移
	if tx.Migrator().HasTable("user_info") {
		// 检查 springsunday 是否有有效数据
		var springUserInfo struct {
			Username string
		}
		err := tx.Table("user_info").Where("site = ?", "springsunday").Select("username").First(&springUserInfo).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if errors.Is(err, gorm.ErrRecordNotFound) || springUserInfo.Username == "" {
			// springsunday 没有有效数据，从 cmct 迁移
			// 先删除空的 springsunday 记录（如果存在）
			if err := tx.Exec("DELETE FROM user_info WHERE site = ?", "springsunday").Error; err != nil {
				return err
			}
			// 将 cmct 重命名为 springsunday
			if err := tx.Table("user_info").Where("site = ?", "cmct").Update("site", "springsunday").Error; err != nil {
				return err
			}
		} else if err := tx.Exec("DELETE FROM user_info WHERE site = ?", "cmct").Error; err != nil {
			// springsunday 有有效数据，删除 cmct 的记录
			return err
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
	APIUrls        []string // API URL 列表，用于轮换（如 mTorrent 站点）
}

func encodeAPIUrls(urls []string) string {
	if len(urls) == 0 {
		return ""
	}
	data, err := json.Marshal(urls)
	if err != nil {
		return ""
	}
	return string(data)
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
