package core

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// SiteOptions 是站点能单独改的几项（nil 不改）。Cookie、APIKey、Passkey 给空串时清除。
type SiteOptions struct {
	Enabled           *bool
	UploadLimitKBs    *int
	DownloadLimitKBs  *int
	SeedingCapacityGB *float64
	HRSeedTimeH       *int
	Cookie            *string
	APIKey            *string
	Passkey           *string
}

// UpdateSiteOptions 只改给了的几项，不动认证方式、地址与 RSS（RSS 的编号不变：UpsertSiteWithRSS 会把 RSS 整体重建）。
// 改完以后站点是启用的，就要求凭据按认证方式齐全（MissingCredential）；站点定义标为不可用的不能启用。
// 配置网关（AI 与 App API 改配置）用它。
func (s *ConfigStore) UpdateSiteOptions(site models.SiteGroup, o SiteOptions) error {
	var row models.SiteSetting
	if err := s.db.DB.Where("name = ?", string(site)).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("站点不存在: %s", site)
		}
		return err
	}
	// 键是字段名（GORM 按字段找列：UploadLimitKBs 的列名不是 upload_limit_kbs）
	cols := map[string]any{}
	hasCookie := strings.TrimSpace(row.CookieEncrypted) != "" || strings.TrimSpace(row.Cookie) != ""
	if o.Cookie != nil {
		cookie := strings.TrimSpace(*o.Cookie)
		cols["Cookie"], cols["CookieEncrypted"] = "", ""
		if cookie != "" {
			cipherText, err := s.EncryptCookie(cookie)
			if err != nil {
				return err
			}
			cols["CookieEncrypted"] = cipherText
		}
		hasCookie = cookie != ""
	}
	apiKey, passkey := row.APIKey, row.Passkey
	if o.APIKey != nil {
		apiKey = strings.TrimSpace(*o.APIKey)
		cols["APIKey"] = apiKey
	}
	if o.Passkey != nil {
		passkey = strings.TrimSpace(*o.Passkey)
		cols["Passkey"] = passkey
	}
	enabled := row.Enabled
	if o.Enabled != nil {
		enabled = *o.Enabled
		cols["Enabled"] = enabled
	}
	if enabled {
		if o.Enabled != nil && *o.Enabled {
			if def, ok := v2.GetDefinitionRegistry().Get(row.Name); ok && def.Unavailable {
				return fmt.Errorf("站点 %s 暂时不可用，不能启用：%s", row.Name, def.UnavailableReason)
			}
		}
		cookieMark := ""
		if hasCookie {
			cookieMark = "set"
		}
		if missing := MissingCredential(row.AuthMethod, cookieMark, apiKey, passkey); missing != "" {
			label := map[string]string{"cookie": "Cookie", "api_key": "API Key", "passkey": "Passkey"}[missing]
			return fmt.Errorf("站点 %s 启用时要有 %s（认证方式是 %s）", row.Name, label, row.AuthMethod)
		}
	}
	nonNegative := func(name string, v int) error {
		if v < 0 {
			return fmt.Errorf("%s 不能是负数", name)
		}
		return nil
	}
	if o.UploadLimitKBs != nil {
		if err := nonNegative("上传限速", *o.UploadLimitKBs); err != nil {
			return err
		}
		cols["UploadLimitKBs"] = *o.UploadLimitKBs
	}
	if o.DownloadLimitKBs != nil {
		if err := nonNegative("下载限速", *o.DownloadLimitKBs); err != nil {
			return err
		}
		cols["DownloadLimitKBs"] = *o.DownloadLimitKBs
	}
	if o.SeedingCapacityGB != nil {
		if *o.SeedingCapacityGB < 0 {
			return errors.New("做种容量上限不能是负数")
		}
		cols["SeedingCapacityGB"] = *o.SeedingCapacityGB
	}
	if o.HRSeedTimeH != nil {
		if err := nonNegative("H&R 做种要求", *o.HRSeedTimeH); err != nil {
			return err
		}
		cols["HRSeedTimeH"] = *o.HRSeedTimeH
	}
	if len(cols) == 0 {
		return nil
	}
	if err := s.db.DB.Model(&models.SiteSetting{}).Where("id = ?", row.ID).Updates(cols).Error; err != nil {
		return fmt.Errorf("保存站点设置失败: %w", err)
	}
	events.Publish(events.Event{Type: events.ConfigChanged, Version: time.Now().UnixNano(), Source: "sites", At: time.Now()})
	return nil
}

// UpdateRSSInSite 原地改站点下的一条 RSS 订阅（编号不变），按 AppendRSSToSite 的规则校验：名字与地址不能为空，
// 地址不能与同站点的其他订阅重复，指定的下载器与过滤规则要存在；过滤规则的关联换成 entry.FilterRuleIDs。
// 示例订阅的标记不变。
func (s *ConfigStore) UpdateRSSInSite(siteName string, entry models.RSSConfig) (models.RSSConfig, error) {
	var updated models.RSSConfig
	err := s.db.WithTransaction(func(tx *gorm.DB) error {
		var site models.SiteSetting
		if err := tx.Where("name = ?", siteName).First(&site).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("站点不存在: %s", siteName)
			}
			return err
		}
		var row models.RSSSubscription
		if err := tx.Where("site_id = ? AND id = ?", site.ID, entry.ID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("RSS 订阅不存在: %d", entry.ID)
			}
			return err
		}
		var others []models.RSSSubscription
		if err := tx.Where("site_id = ? AND id <> ?", site.ID, row.ID).Find(&others).Error; err != nil {
			return err
		}
		existing := make([]models.RSSConfig, 0, len(others))
		for _, r := range others {
			existing = append(existing, models.RSSConfig{ID: r.ID, Name: r.Name, URL: r.URL})
		}
		normalized, err := validateAndNormalizeRSS(existing, entry)
		if err != nil {
			return err
		}
		if normalized.DownloaderID != nil {
			var downloader models.DownloaderSetting
			if err := tx.First(&downloader, *normalized.DownloaderID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return fmt.Errorf("下载器不存在: %d", *normalized.DownloaderID)
				}
				return err
			}
		}
		if len(normalized.FilterRuleIDs) > 0 {
			var n int64
			if err := tx.Model(&models.FilterRule{}).Where("id IN ?", normalized.FilterRuleIDs).Count(&n).Error; err != nil {
				return err
			}
			if int(n) != len(uniqueIDs(normalized.FilterRuleIDs)) {
				return errors.New("有的过滤规则不存在")
			}
		}
		row.Name, row.URL, row.Category, row.Tag = normalized.Name, normalized.URL, normalized.Category, normalized.Tag
		row.IntervalMinutes, row.DownloaderID, row.DownloadPath = normalized.IntervalMinutes, normalized.DownloaderID, normalized.DownloadPath
		row.PauseOnFreeEnd, row.FilterMode = normalized.PauseOnFreeEnd, normalized.FilterMode
		row.NotifyMode, row.NotifyConfIDs, row.MaxNotificationsPerHour = normalized.NotifyMode, normalized.NotifyConfIDs, normalized.MaxNotificationsPerHour
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		if err := models.NewRSSFilterAssociationDB(tx).SetFilterRulesForRSS(row.ID, uniqueIDs(normalized.FilterRuleIDs)); err != nil {
			return err
		}
		updated = normalized
		updated.ID, updated.IsExample = row.ID, row.IsExample
		return nil
	})
	if err != nil {
		return models.RSSConfig{}, err
	}
	events.Publish(events.Event{Type: events.ConfigChanged, Version: time.Now().UnixNano(), Source: "sites", At: time.Now()})
	sLogger().Infof("[RSS订阅已修改] 站点=%s, RSS=%s(ID=%d)", siteName, updated.Name, updated.ID)
	return updated, nil
}

// uniqueIDs 去掉重复的编号，顺序不变。
func uniqueIDs(ids []uint) []uint {
	seen := make(map[uint]bool, len(ids))
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
