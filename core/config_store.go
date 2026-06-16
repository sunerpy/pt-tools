package core

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	internalcrypto "github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/utils"
)

var ErrKeyMissing = errors.New("AES secret key missing or corrupt")

type ConfigStore struct {
	db             *models.TorrentDB
	keyMissingWarn sync.Once
}

func NewConfigStore(db *models.TorrentDB) *ConfigStore {
	return &ConfigStore{db: db}
}

func (s *ConfigStore) EncryptCookie(plain string) (string, error) {
	if err := s.ensureCookieKeyAvailable(); err != nil {
		return "", err
	}
	cipherText, err := internalcrypto.Encrypt([]byte(plain))
	if err != nil {
		return "", fmt.Errorf("encrypt cookie failed: %w", err)
	}
	return cipherText, nil
}

func (s *ConfigStore) DecryptCookie(cipherText string) (string, error) {
	if err := s.ensureCookieKeyAvailable(); err != nil {
		return "", err
	}
	plainBytes, err := internalcrypto.Decrypt(cipherText)
	if err != nil {
		return "", fmt.Errorf("decrypt cookie failed: %w", err)
	}
	return string(plainBytes), nil
}

func (s *ConfigStore) ensureCookieKeyAvailable() error {
	if envKey := strings.TrimSpace(os.Getenv("PT_TOOLS_SECRET_KEY")); envKey != "" {
		keyBytes, err := internalcryptoKeyFromEnv(envKey)
		if err == nil && len(keyBytes) == 32 {
			return nil
		}
		return s.cookieKeyError()
	}

	keyFile, err := cookieSecretKeyFile()
	if err != nil {
		return s.cookieKeyError()
	}
	keyHex, err := os.ReadFile(keyFile)
	if err != nil {
		return s.cookieKeyError()
	}
	keyBytes, err := hex.DecodeString(string(bytes.TrimSpace(keyHex)))
	if err != nil || len(keyBytes) != 32 {
		return s.cookieKeyError()
	}
	return nil
}

func (s *ConfigStore) cookieKeyError() error {
	s.keyMissingWarn.Do(func() {
		if logger := global.GetSloggerSafe(); logger != nil {
			logger.Warn("AES secret.key missing or corrupt; 请运行 pt-tools secret import 或检查 ~/.pt-tools/secret.key")
		}
	})
	return fmt.Errorf("KEY_ERROR: %w", ErrKeyMissing)
}

func cookieSecretKeyFile() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".pt-tools", "secret.key"), nil
}

func internalcryptoKeyFromEnv(keyBase64 string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(keyBase64)
}

// SyncSites 从注册的站点列表同步到数据库
// 应在应用启动时调用，确保内存中注册的站点都存在于数据库中
func (s *ConfigStore) SyncSites(registeredSites []models.RegisteredSite) error {
	return models.SyncSitesFromRegistry(s.db.DB, registeredSites)
}

// Load 将 SQLite 中的配置组装为运行时 Config
func (s *ConfigStore) Load() (*models.Config, error) {
	db := s.db.DB
	var out models.Config
	if err := db.Transaction(func(tx *gorm.DB) error {
		var gs models.SettingsGlobal
		if e := tx.First(&gs).Error; e == nil {
			out.Global.DefaultIntervalMinutes = gs.DefaultIntervalMinutes
			out.Global.DefaultEnabled = gs.DefaultEnabled
			out.Global.DownloadDir = gs.DownloadDir
			out.Global.DownloadLimitEnabled = gs.DownloadLimitEnabled
			out.Global.DownloadSpeedLimit = gs.DownloadSpeedLimit
			out.Global.TorrentSizeGB = gs.TorrentSizeGB
			out.Global.AutoStart = gs.AutoStart
		} else {
			out.Global.DefaultIntervalMinutes = durationToMinutes(20 * time.Minute)
			out.Global.DefaultEnabled = true
			out.Global.DownloadDir = "download"
			out.Global.DownloadLimitEnabled = false
			out.Global.DownloadSpeedLimit = 20
			out.Global.TorrentSizeGB = 200
			out.Global.AutoStart = false
			def := models.SettingsGlobal{DefaultIntervalMinutes: out.Global.DefaultIntervalMinutes, DefaultEnabled: out.Global.DefaultEnabled, DownloadDir: out.Global.DownloadDir, DownloadLimitEnabled: out.Global.DownloadLimitEnabled, DownloadSpeedLimit: out.Global.DownloadSpeedLimit, TorrentSizeGB: out.Global.TorrentSizeGB, AutoStart: false}
			if ce := tx.Create(&def).Error; ce != nil {
				return ce
			}
		}
		var qs models.QbitSettings
		if e := tx.First(&qs).Error; e == nil {
			out.Qbit.Enabled = qs.Enabled
			out.Qbit.URL = qs.URL
			out.Qbit.User = qs.User
			out.Qbit.Password = qs.Password
		}
		out.Sites = map[models.SiteGroup]models.SiteConfig{}
		var sites []models.SiteSetting
		if e := tx.Find(&sites).Error; e != nil {
			return e
		}
		for _, sitem := range sites {
			sg := models.SiteGroup(strings.ToLower(sitem.Name))
			cookie, e := s.cookiePlaintextForSite(sitem)
			if e != nil {
				return e
			}
			sc := models.SiteConfig{Enabled: boolPtr(sitem.Enabled), AuthMethod: sitem.AuthMethod, Cookie: cookie, APIKey: sitem.APIKey, APIUrl: sitem.APIUrl, Passkey: sitem.Passkey, UploadLimitKBs: sitem.UploadLimitKBs, DownloadLimitKBs: sitem.DownloadLimitKBs, SeedingCapacityGB: sitem.SeedingCapacityGB, RSS: []models.RSSConfig{}}
			var rss []models.RSSSubscription
			if e := tx.Where("site_id = ?", sitem.ID).Find(&rss).Error; e != nil {
				return e
			}
			for _, r := range rss {
				sc.RSS = append(sc.RSS, models.RSSConfig{ID: r.ID, Name: r.Name, URL: r.URL, Category: r.Category, Tag: r.Tag, IntervalMinutes: r.IntervalMinutes, DownloaderID: r.DownloaderID, DownloadPath: r.DownloadPath, IsExample: r.IsExample, PauseOnFreeEnd: r.PauseOnFreeEnd, FilterMode: r.FilterMode, NotifyMode: r.NotifyMode, NotifyConfIDs: r.NotifyConfIDs, MaxNotificationsPerHour: r.MaxNotificationsPerHour})
			}
			out.Sites[sg] = sc
		}
		return nil
	}); err != nil {
		sLogger().Errorf("[配置加载失败] 错误=%v", err)
		return nil, err
	}
	sLogger().Debugf("[配置加载完成] 站点数=%d", len(out.Sites))
	return &out, nil
}

func (s *ConfigStore) SaveGlobal(gl models.SettingsGlobal) error {
	db := s.db.DB
	var gs models.SettingsGlobal
	if err := db.First(&gs).Error; err != nil {
		gs = models.SettingsGlobal{}
	}
	if gl.DefaultIntervalMinutes < models.MinIntervalMinutes {
		gl.DefaultIntervalMinutes = models.MinIntervalMinutes
	}
	gs.DefaultIntervalMinutes = gl.DefaultIntervalMinutes
	gs.DefaultEnabled = gl.DefaultEnabled
	gs.DownloadDir = gl.DownloadDir
	gs.DownloadLimitEnabled = gl.DownloadLimitEnabled
	gs.DownloadSpeedLimit = gl.DownloadSpeedLimit
	gs.TorrentSizeGB = gl.TorrentSizeGB
	gs.AutoStart = gl.AutoStart
	if strings.TrimSpace(gs.DownloadDir) == "" {
		return errors.New("下载目录不能为空")
	}
	if home, herr := os.UserHomeDir(); herr == nil {
		if _, rerr := utils.ResolveDownloadBase(home, models.WorkDir, gs.DownloadDir); rerr != nil {
			return rerr
		}
	}
	if err := db.Save(&gs).Error; err != nil {
		return err
	}
	sLogger().Infof("[全局配置已更新] 下载目录=%s, 自动启动=%v", gs.DownloadDir, gs.AutoStart)
	events.Publish(events.Event{Type: events.ConfigChanged, Version: time.Now().UnixNano(), Source: "global", At: time.Now()})
	return nil
}

func (s *ConfigStore) GetGlobalOnly() (models.SettingsGlobal, error) {
	db := s.db.DB
	var gs models.SettingsGlobal
	var out models.SettingsGlobal
	if err := db.First(&gs).Error; err == nil {
		return gs, nil
	}
	out.DefaultIntervalMinutes = durationToMinutes(20 * time.Minute)
	out.DefaultEnabled = true
	out.DownloadDir = "download"
	out.DownloadLimitEnabled = false
	out.DownloadSpeedLimit = 20
	out.TorrentSizeGB = 200
	out.AutoStart = false
	out.CleanupDiskProtect = true
	out.CleanupMinDiskSpaceGB = 50
	return out, nil
}

// Unified API structures: use models.SettingsGlobal for external I/O
func (s *ConfigStore) GetGlobalSettings() (models.SettingsGlobal, error) {
	var gs models.SettingsGlobal
	if err := s.db.DB.First(&gs).Error; err != nil {
		// provide defaults
		gs.DefaultIntervalMinutes = durationToMinutes(20 * time.Minute)
		gs.DefaultEnabled = true
		gs.DownloadDir = "download"
		gs.DownloadLimitEnabled = false
		gs.DownloadSpeedLimit = 20
		gs.TorrentSizeGB = 200
		gs.AutoStart = false
		gs.CleanupDiskProtect = true
		gs.CleanupMinDiskSpaceGB = 50
		return gs, nil
	}
	return gs, nil
}

func (s *ConfigStore) SaveGlobalSettings(gs models.SettingsGlobal) error {
	if strings.TrimSpace(gs.DownloadDir) == "" {
		return errors.New("下载目录不能为空")
	}
	if gs.DefaultIntervalMinutes < models.MinIntervalMinutes {
		gs.DefaultIntervalMinutes = models.MinIntervalMinutes
	}
	if home, herr := os.UserHomeDir(); herr == nil {
		if _, rerr := utils.ResolveDownloadBase(home, models.WorkDir, gs.DownloadDir); rerr != nil {
			return rerr
		}
	}
	// upsert single row
	var cur models.SettingsGlobal
	db := s.db.DB
	if err := db.First(&cur).Error; err == nil {
		cur.DefaultIntervalMinutes = gs.DefaultIntervalMinutes
		cur.DefaultEnabled = gs.DefaultEnabled
		cur.DownloadDir = gs.DownloadDir
		cur.DownloadLimitEnabled = gs.DownloadLimitEnabled
		cur.DownloadSpeedLimit = gs.DownloadSpeedLimit
		cur.TorrentSizeGB = gs.TorrentSizeGB
		cur.MinFreeMinutes = gs.MinFreeMinutes
		cur.AutoStart = gs.AutoStart
		cur.RetainHours = gs.RetainHours
		cur.MaxRetry = gs.MaxRetry
		cur.DefaultConcurrency = gs.DefaultConcurrency
		cur.CleanupEnabled = gs.CleanupEnabled
		cur.CleanupIntervalMin = gs.CleanupIntervalMin
		cur.CleanupScope = gs.CleanupScope
		cur.CleanupScopeTags = gs.CleanupScopeTags
		cur.CleanupRemoveData = gs.CleanupRemoveData
		cur.CleanupConditionMode = gs.CleanupConditionMode
		cur.CleanupMaxSeedTimeH = gs.CleanupMaxSeedTimeH
		cur.CleanupMinRatio = gs.CleanupMinRatio
		cur.CleanupMaxInactiveH = gs.CleanupMaxInactiveH
		cur.CleanupSlowSeedTimeH = gs.CleanupSlowSeedTimeH
		cur.CleanupSlowMaxRatio = gs.CleanupSlowMaxRatio
		cur.CleanupDelFreeExpired = gs.CleanupDelFreeExpired
		cur.CleanupDiskProtect = gs.CleanupDiskProtect
		cur.CleanupMinDiskSpaceGB = gs.CleanupMinDiskSpaceGB
		cur.CleanupProtectDL = gs.CleanupProtectDL
		cur.CleanupProtectHR = gs.CleanupProtectHR
		cur.CleanupMinRetainH = gs.CleanupMinRetainH
		cur.CleanupProtectTags = gs.CleanupProtectTags
		cur.AutoDeleteOnFreeEnd = gs.AutoDeleteOnFreeEnd
		cur.PeerRatioEnabled = gs.PeerRatioEnabled
		cur.PeerRatioMaxSL = gs.PeerRatioMaxSL
		cur.PeerRatioIntervalMin = gs.PeerRatioIntervalMin
		cur.PeerRatioRemoveData = gs.PeerRatioRemoveData
		cur.DefaultFilterMode = models.NormalizeFilterMode(gs.DefaultFilterMode)
		if err := db.Save(&cur).Error; err != nil {
			return err
		}
	} else {
		if err := db.Create(&gs).Error; err != nil {
			return err
		}
	}
	events.Publish(events.Event{Type: events.ConfigChanged, Version: time.Now().UnixNano(), Source: "global", At: time.Now()})
	return nil
}

func (s *ConfigStore) SaveQbit(qb models.QbitSettings) error {
	db := s.db.DB
	var q models.QbitSettings
	if err := db.First(&q).Error; err != nil {
		q = models.QbitSettings{}
	}
	q.Enabled = qb.Enabled
	q.URL = qb.URL
	q.User = qb.User
	q.Password = qb.Password
	if strings.TrimSpace(q.URL) == "" || strings.TrimSpace(q.User) == "" || strings.TrimSpace(q.Password) == "" {
		return errors.New("qBittorrent URL、用户名、密码均为必填")
	}
	if err := db.Save(&q).Error; err != nil {
		return err
	}
	events.Publish(events.Event{Type: events.ConfigChanged, Version: time.Now().UnixNano(), Source: "qbit", At: time.Now()})
	return nil
}

func (s *ConfigStore) GetQbitOnly() (models.QbitSettings, error) {
	db := s.db.DB
	var q models.QbitSettings
	if err := db.First(&q).Error; err == nil {
		return q, nil
	}
	return q, nil
}

func (s *ConfigStore) GetQbitSettings() (models.QbitSettings, error) {
	var q models.QbitSettings
	if err := s.db.DB.First(&q).Error; err != nil {
		return q, nil
	}
	return q, nil
}

func (s *ConfigStore) SaveQbitSettings(q models.QbitSettings) error {
	if strings.TrimSpace(q.URL) == "" || strings.TrimSpace(q.User) == "" || strings.TrimSpace(q.Password) == "" {
		return errors.New("qBittorrent URL、用户名、密码均为必填")
	}
	var cur models.QbitSettings
	db := s.db.DB
	if err := db.First(&cur).Error; err == nil {
		cur.Enabled = q.Enabled
		cur.URL = q.URL
		cur.User = q.User
		cur.Password = q.Password
		if err := db.Save(&cur).Error; err != nil {
			return err
		}
	} else {
		if err := db.Create(&q).Error; err != nil {
			return err
		}
	}
	events.Publish(events.Event{Type: events.ConfigChanged, Version: time.Now().UnixNano(), Source: "qbit", At: time.Now()})
	return nil
}

func (s *ConfigStore) UpsertSite(site models.SiteGroup, sc models.SiteConfig) (uint, error) {
	db := s.db.DB
	var row models.SiteSetting
	if err := db.Where("name = ?", string(site)).First(&row).Error; err != nil {
		row = models.SiteSetting{Name: string(site)}
	}
	if sc.Enabled != nil {
		row.Enabled = *sc.Enabled
	}
	row.AuthMethod = sc.AuthMethod
	row.Cookie = sc.Cookie
	if strings.TrimSpace(sc.Cookie) == "" {
		row.CookieEncrypted = ""
	} else {
		cookieCipherText, err := s.EncryptCookie(sc.Cookie)
		if err != nil {
			return 0, err
		}
		row.CookieEncrypted = cookieCipherText
	}
	row.APIKey = sc.APIKey
	row.APIUrl = sc.APIUrl
	row.Passkey = sc.Passkey
	if err := db.Save(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (s *ConfigStore) ReplaceSiteRSS(siteID uint, rss []models.RSSConfig) error {
	db := s.db.DB
	if err := db.Where("site_id = ?", siteID).Delete(&models.RSSSubscription{}).Error; err != nil {
		return err
	}
	for _, r := range rss {
		row := models.RSSSubscription{
			SiteID:                  siteID,
			Name:                    r.Name,
			URL:                     r.URL,
			Category:                r.Category,
			Tag:                     r.Tag,
			IntervalMinutes:         r.IntervalMinutes,
			DownloaderID:            r.DownloaderID,
			FilterMode:              r.FilterMode,
			NotifyMode:              r.NotifyMode,
			NotifyConfIDs:           r.NotifyConfIDs,
			MaxNotificationsPerHour: r.MaxNotificationsPerHour,
		}
		if err := db.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *ConfigStore) EnsureAdmin(username, passwordHash string) error {
	db := s.db.DB
	var u models.AdminUser
	uname := strings.TrimSpace(username)
	if err := db.Where("username = ?", uname).First(&u).Error; err == nil {
		return nil
	}
	u.Username = uname
	u.PasswordHash = passwordHash
	return db.Create(&u).Error
}

func (s *ConfigStore) GetAdmin(username string) (*models.AdminUser, error) {
	var u models.AdminUser
	uname := strings.TrimSpace(username)
	if err := s.db.DB.Where("username = ?", uname).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *ConfigStore) UpdateAdmin(u *models.AdminUser) error {
	return s.db.DB.Save(u).Error
}

func (s *ConfigStore) UpdateAdminPassword(username, newHash string) error {
	var u models.AdminUser
	if err := s.db.DB.Where("username = ?", strings.TrimSpace(username)).First(&u).Error; err != nil {
		return err
	}
	u.PasswordHash = newHash
	return s.db.DB.Save(&u).Error
}

func (s *ConfigStore) AdminCount() (int64, error) {
	var users []models.AdminUser
	_ = s.db.DB.Select("username").Find(&users).Error
	names := make([]string, 0, len(users))
	for _, u := range users {
		names = append(names, u.Username)
	}
	sLogger().Infof("admin_users users=%v count=%d", names, len(users))
	var cnt int64
	err := s.db.DB.Model(&models.AdminUser{}).Count(&cnt).Error
	return cnt, err
}

// UpsertSiteWithRSS 原子性保存站点与 RSS，并进行严格校验
func (s *ConfigStore) UpsertSiteWithRSS(site models.SiteGroup, sc models.SiteConfig) error {
	// 严格校验：
	// 1) 认证方式必填且合法；
	// 2) 根据认证方式二选一且对应字段不为空（api_key 或 cookie），另一项必须为空；
	//    特殊：cookie_and_api_key 同时需要两者
	// 3) 对于非预置站点，API URL 必填；预置站点使用常量
	// 4) RSS 列表可以为空，但如果有则各项字段需合法。
	am := strings.ToLower(strings.TrimSpace(sc.AuthMethod))
	if am != "cookie" && am != "api_key" && am != "cookie_and_api_key" && am != "passkey" {
		return errors.New("认证方式必须为 'cookie'、'api_key'、'cookie_and_api_key' 或 'passkey'")
	}

	// 检查站点是否在注册表中（有默认 URL），注册表中的站点不需要用户提供 API URL
	registry := v2.GetGlobalSiteRegistry()
	meta, isRegistered := registry.Get(string(site))
	hasDefaultURL := isRegistered && meta.DefaultBaseURL != ""
	if !hasDefaultURL && strings.TrimSpace(sc.APIUrl) == "" {
		return errors.New("API 地址不能为空")
	}
	var existingSite models.SiteSetting
	hasStoredCookie := false
	if err := s.db.DB.Where("name = ?", string(site)).First(&existingSite).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	} else {
		hasStoredCookie = strings.TrimSpace(existingSite.CookieEncrypted) != "" || strings.TrimSpace(existingSite.Cookie) != ""
	}

	apiKeyEmpty := strings.TrimSpace(sc.APIKey) == ""
	cookieEmpty := strings.TrimSpace(sc.Cookie) == "" && !hasStoredCookie
	passkeyEmpty := strings.TrimSpace(sc.Passkey) == ""
	switch am {
	case "api_key":
		if apiKeyEmpty {
			return errors.New("API Key 不能为空")
		}
	case "cookie_and_api_key":
		if apiKeyEmpty {
			return errors.New("API Key 不能为空")
		}
		if cookieEmpty {
			return errors.New("Cookie 不能为空")
		}
	case "cookie":
		if cookieEmpty {
			return errors.New("Cookie 不能为空")
		}
		if !apiKeyEmpty {
			return errors.New("认证方式为 cookie 时 API Key 必须留空")
		}
	case "passkey":
		if passkeyEmpty {
			return errors.New("Passkey 不能为空")
		}
	}
	// RSS 列表允许为空，只在有内容时进行校验
	if len(sc.RSS) > 0 {
		// 检查重复 RSS URL
		urlSet := make(map[string]bool)
		for i, r := range sc.RSS {
			normalizedURL := strings.TrimSpace(strings.ToLower(r.URL))
			if urlSet[normalizedURL] {
				return fmt.Errorf("第 %d 条 RSS 的 URL 与之前的重复: %s", i+1, r.URL)
			}
			urlSet[normalizedURL] = true
		}
		for i, r := range sc.RSS {
			if strings.TrimSpace(r.Name) == "" {
				return errors.New("第 " + fmt.Sprint(i+1) + " 条 RSS 的 name 不能为空")
			}
			if strings.TrimSpace(r.URL) == "" {
				return errors.New("第 " + fmt.Sprint(i+1) + " 条 RSS 的 url 不能为空")
			}
			// category 允许为空
			// Tag 允许为空，后端将使用父目录作为下载子路径
			if r.IntervalMinutes < models.MinIntervalMinutes {
				r.IntervalMinutes = models.MinIntervalMinutes
			}
			// DownloadSubPath 前端已移除，后端使用 Tag 作为子目录；允许为空
		}
	}
	// 事务保存站点与 RSS
	return s.db.WithTransaction(func(tx *gorm.DB) error {
		var row models.SiteSetting
		if err := tx.Where("name = ?", string(site)).First(&row).Error; err != nil {
			row = models.SiteSetting{Name: string(site), DisplayName: string(site), IsBuiltin: true}
		}
		if sc.Enabled != nil {
			row.Enabled = *sc.Enabled
		}
		row.AuthMethod = sc.AuthMethod
		if strings.TrimSpace(sc.Cookie) != "" {
			row.Cookie = sc.Cookie
			cookieCipherText, err := s.EncryptCookie(sc.Cookie)
			if err != nil {
				return err
			}
			row.CookieEncrypted = cookieCipherText
		} else {
			// 本次保存未携带 cookie：保留已存储的登录态 cookie（含 api_key/passkey 站点），不因 auth_method 清除。
			row.Cookie = ""
		}
		row.APIKey = sc.APIKey
		row.APIUrl = sc.APIUrl
		row.Passkey = sc.Passkey
		row.UploadLimitKBs = sc.UploadLimitKBs
		row.DownloadLimitKBs = sc.DownloadLimitKBs
		row.SeedingCapacityGB = sc.SeedingCapacityGB
		if err := tx.Save(&row).Error; err != nil {
			return err
		}

		// 替换 RSS
		if err := tx.Where("site_id = ?", row.ID).Delete(&models.RSSSubscription{}).Error; err != nil {
			return err
		}

		assocDB := models.NewRSSFilterAssociationDB(tx)

		for _, r := range sc.RSS {
			if r.IntervalMinutes < models.MinIntervalMinutes {
				r.IntervalMinutes = models.MinIntervalMinutes
			}
			rr := models.RSSSubscription{
				SiteID:                  row.ID,
				Name:                    r.Name,
				URL:                     r.URL,
				Category:                r.Category,
				Tag:                     r.Tag,
				IntervalMinutes:         r.IntervalMinutes,
				DownloaderID:            r.DownloaderID,
				DownloadPath:            r.DownloadPath,
				PauseOnFreeEnd:          r.PauseOnFreeEnd,
				FilterMode:              r.FilterMode,
				NotifyMode:              r.NotifyMode,
				NotifyConfIDs:           r.NotifyConfIDs,
				MaxNotificationsPerHour: r.MaxNotificationsPerHour,
			}
			if err := tx.Create(&rr).Error; err != nil {
				return err
			}

			// 保存 RSS-Filter 关联
			if len(r.FilterRuleIDs) > 0 {
				if err := assocDB.SetFilterRulesForRSS(rr.ID, r.FilterRuleIDs); err != nil {
					return err
				}
			}
		}
		events.Publish(events.Event{Type: events.ConfigChanged, Version: time.Now().UnixNano(), Source: "sites", At: time.Now()})
		sLogger().Infof("[站点配置已更新] 站点=%s, 启用=%v, RSS数量=%d", site, row.Enabled, len(sc.RSS))
		return nil
	})
}

func (s *ConfigStore) AppendRSSToSite(siteName string, entry models.RSSConfig) (models.RSSConfig, error) {
	var created models.RSSConfig
	err := s.db.WithTransaction(func(tx *gorm.DB) error {
		var site models.SiteSetting
		if err := tx.Where("name = ?", siteName).First(&site).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("站点不存在: %s", siteName)
			}
			return err
		}

		var rows []models.RSSSubscription
		if err := tx.Where("site_id = ?", site.ID).Find(&rows).Error; err != nil {
			return err
		}
		existing := make([]models.RSSConfig, 0, len(rows))
		for _, r := range rows {
			existing = append(existing, models.RSSConfig{ID: r.ID, Name: r.Name, URL: r.URL, Category: r.Category, Tag: r.Tag, IntervalMinutes: r.IntervalMinutes, DownloaderID: r.DownloaderID, DownloadPath: r.DownloadPath, IsExample: r.IsExample, PauseOnFreeEnd: r.PauseOnFreeEnd, FilterMode: r.FilterMode, NotifyMode: r.NotifyMode, NotifyConfIDs: r.NotifyConfIDs, MaxNotificationsPerHour: r.MaxNotificationsPerHour})
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

		rss := models.RSSSubscription{
			SiteID:                  site.ID,
			Name:                    normalized.Name,
			URL:                     normalized.URL,
			Category:                normalized.Category,
			Tag:                     normalized.Tag,
			IntervalMinutes:         normalized.IntervalMinutes,
			DownloaderID:            normalized.DownloaderID,
			DownloadPath:            normalized.DownloadPath,
			PauseOnFreeEnd:          normalized.PauseOnFreeEnd,
			FilterMode:              normalized.FilterMode,
			NotifyMode:              normalized.NotifyMode,
			NotifyConfIDs:           normalized.NotifyConfIDs,
			MaxNotificationsPerHour: normalized.MaxNotificationsPerHour,
		}
		if err := tx.Create(&rss).Error; err != nil {
			return err
		}

		if len(normalized.FilterRuleIDs) > 0 {
			assocDB := models.NewRSSFilterAssociationDB(tx)
			if err := assocDB.SetFilterRulesForRSS(rss.ID, normalized.FilterRuleIDs); err != nil {
				return err
			}
		}

		created = normalized
		created.ID = rss.ID
		return nil
	})
	if err != nil {
		return models.RSSConfig{}, err
	}
	events.Publish(events.Event{Type: events.ConfigChanged, Version: time.Now().UnixNano(), Source: "sites", At: time.Now()})
	sLogger().Infof("[RSS订阅已追加] 站点=%s, RSS=%s", siteName, created.Name)
	return created, nil
}

// DeleteRSSFromSite 原子删除指定站点下的单条 RSS 订阅，并清理其过滤规则关联。
func (s *ConfigStore) DeleteRSSFromSite(siteName string, rssID uint) (models.RSSConfig, error) {
	var deleted models.RSSConfig
	err := s.db.WithTransaction(func(tx *gorm.DB) error {
		var site models.SiteSetting
		if err := tx.Where("name = ?", siteName).First(&site).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("站点不存在: %s", siteName)
			}
			return err
		}

		var row models.RSSSubscription
		if err := tx.Where("site_id = ? AND id = ?", site.ID, rssID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("RSS 订阅不存在: %d", rssID)
			}
			return err
		}

		assocDB := models.NewRSSFilterAssociationDB(tx)
		if err := assocDB.DeleteByRSSID(row.ID); err != nil {
			return err
		}
		if err := tx.Where("site_id = ? AND id = ?", site.ID, rssID).Delete(&models.RSSSubscription{}).Error; err != nil {
			return err
		}

		deleted = models.RSSConfig{ID: row.ID, Name: row.Name, URL: row.URL}
		return nil
	})
	if err != nil {
		return models.RSSConfig{}, err
	}
	events.Publish(events.Event{Type: events.ConfigChanged, Version: time.Now().UnixNano(), Source: "sites", At: time.Now()})
	sLogger().Infof("[RSS订阅已删除] 站点=%s, RSS=%s(ID=%d)", siteName, deleted.Name, deleted.ID)
	return deleted, nil
}

// ListRSSForSite 返回指定站点的所有 RSS 订阅。
func (s *ConfigStore) ListRSSForSite(siteName string) ([]models.RSSConfig, error) {
	sc, err := s.GetSiteConf(models.SiteGroup(siteName))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("站点不存在: %s", siteName)
		}
		return nil, err
	}
	return sc.RSS, nil
}

// DeleteSite 删除站点（预置站点禁止删除）
func (s *ConfigStore) DeleteSite(name string) error {
	lower := strings.ToLower(name)
	if lower == "springsunday" || lower == "hdsky" || lower == "mteam" {
		return errors.New("预置站点不可删除")
	}
	err := s.db.WithTransaction(func(tx *gorm.DB) error {
		var site models.SiteSetting
		if err := tx.Where("name = ?", lower).First(&site).Error; err != nil {
			return err
		}
		if err := tx.Where("site_id = ?", site.ID).Delete(&models.RSSSubscription{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&site).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	events.Publish(events.Event{Type: events.ConfigChanged, Version: time.Now().UnixNano(), Source: "sites", At: time.Now()})
	return nil
}

// 工具函数
// func minutesToDuration(m int32) (d time.Duration) { return time.Duration(m) * time.Minute }
func durationToMinutes(d time.Duration) int32 { return int32(d / time.Minute) }
func boolPtr(b bool) *bool                    { return &b }

// ReloadGlobal 从 DB 加载并刷新全局配置与目录缓存
// 已弃用：由各业务按需读取 DB 并更新目录缓存
// ListSites 从 DB 读取站点与 RSS 配置（不依赖 global.GlobalCfg）
func (s *ConfigStore) ListSites() (map[models.SiteGroup]models.SiteConfig, error) {
	out := map[models.SiteGroup]models.SiteConfig{}
	var sites []models.SiteSetting
	if err := s.db.DB.Find(&sites).Error; err != nil {
		return nil, err
	}
	for _, ss := range sites {
		sg := models.SiteGroup(strings.ToLower(ss.Name))
		cookie, err := s.cookiePlaintextForSite(ss)
		if err != nil {
			return nil, err
		}
		sc := models.SiteConfig{Enabled: boolPtr(ss.Enabled), AuthMethod: ss.AuthMethod, Cookie: cookie, APIKey: ss.APIKey, APIUrl: ss.APIUrl, Passkey: ss.Passkey, UploadLimitKBs: ss.UploadLimitKBs, DownloadLimitKBs: ss.DownloadLimitKBs, SeedingCapacityGB: ss.SeedingCapacityGB, RSS: []models.RSSConfig{}}
		var rss []models.RSSSubscription
		if err := s.db.DB.Where("site_id = ?", ss.ID).Find(&rss).Error; err != nil {
			return nil, err
		}
		for _, r := range rss {
			sc.RSS = append(sc.RSS, models.RSSConfig{ID: r.ID, Name: r.Name, URL: r.URL, Category: r.Category, Tag: r.Tag, IntervalMinutes: r.IntervalMinutes, DownloaderID: r.DownloaderID, DownloadPath: r.DownloadPath, IsExample: r.IsExample, PauseOnFreeEnd: r.PauseOnFreeEnd, FilterMode: r.FilterMode, NotifyMode: r.NotifyMode, NotifyConfIDs: r.NotifyConfIDs, MaxNotificationsPerHour: r.MaxNotificationsPerHour})
		}
		// 注意：AuthMethod 和 APIUrl 已从数据库读取（由 SyncSites 初始化）
		out[sg] = sc
	}
	return out, nil
}

// GetSiteConf 获取指定站点配置
func (s *ConfigStore) GetSiteConf(name models.SiteGroup) (models.SiteConfig, error) {
	var ss models.SiteSetting
	if err := s.db.DB.Where("name = ?", string(name)).First(&ss).Error; err != nil {
		return models.SiteConfig{}, err
	}
	// 初始化 RSS 为空数组，确保 JSON 序列化时返回 [] 而不是 null
	cookie, err := s.cookiePlaintextForSite(ss)
	if err != nil {
		return models.SiteConfig{}, err
	}
	sc := models.SiteConfig{Enabled: boolPtr(ss.Enabled), AuthMethod: ss.AuthMethod, Cookie: cookie, APIKey: ss.APIKey, APIUrl: ss.APIUrl, Passkey: ss.Passkey, UploadLimitKBs: ss.UploadLimitKBs, DownloadLimitKBs: ss.DownloadLimitKBs, SeedingCapacityGB: ss.SeedingCapacityGB, RSS: []models.RSSConfig{}}
	var rss []models.RSSSubscription
	if err := s.db.DB.Where("site_id = ?", ss.ID).Find(&rss).Error; err != nil {
		return models.SiteConfig{}, err
	}

	// 获取 RSS-Filter 关联
	assocDB := models.NewRSSFilterAssociationDB(s.db.DB)

	for _, r := range rss {
		rssCfg := models.RSSConfig{
			ID:                      r.ID,
			Name:                    r.Name,
			URL:                     r.URL,
			Category:                r.Category,
			Tag:                     r.Tag,
			IntervalMinutes:         r.IntervalMinutes,
			DownloaderID:            r.DownloaderID,
			DownloadPath:            r.DownloadPath,
			IsExample:               r.IsExample,
			PauseOnFreeEnd:          r.PauseOnFreeEnd,
			FilterMode:              r.FilterMode,
			NotifyMode:              r.NotifyMode,
			NotifyConfIDs:           r.NotifyConfIDs,
			MaxNotificationsPerHour: r.MaxNotificationsPerHour,
		}

		// 获取关联的过滤规则 ID
		ruleIDs, err := assocDB.GetFilterRuleIDsForRSS(r.ID)
		if err == nil {
			rssCfg.FilterRuleIDs = ruleIDs
		}

		sc.RSS = append(sc.RSS, rssCfg)
	}
	// 注意：AuthMethod 和 APIUrl 已从数据库读取（由 SyncSites 初始化）
	return sc, nil
}

func (s *ConfigStore) cookiePlaintextForSite(site models.SiteSetting) (string, error) {
	if strings.TrimSpace(site.CookieEncrypted) == "" {
		return site.Cookie, nil
	}
	plainCookie, err := s.DecryptCookie(site.CookieEncrypted)
	if err != nil {
		return "", err
	}
	return plainCookie, nil
}

// CloakConfigSnapshot is the in-memory view of CloakBrowser-Manager接入配置。
// 仅在 ConfigStore 内部及调用方使用；HTTP 层通过 web.api_cloak.go 暴露的响应类型
// 控制对外字段，token 永远不会以明文出现在 API 响应里。
type CloakConfigSnapshot struct {
	Endpoint string
	HasToken bool
}

// GetCloakConfig 返回 endpoint + 是否已设置 token。永远不返回 token 明文。
func (s *ConfigStore) GetCloakConfig() (CloakConfigSnapshot, error) {
	var row models.CloakSettings
	err := s.db.DB.First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return CloakConfigSnapshot{}, err
	}
	return CloakConfigSnapshot{
		Endpoint: row.Endpoint,
		HasToken: strings.TrimSpace(row.TokenEncrypted) != "",
	}, nil
}

// GetCloakEndpoint 返回当前持久化的 endpoint，记录不存在时返回空字符串。
func (s *ConfigStore) GetCloakEndpoint() (string, error) {
	cfg, err := s.GetCloakConfig()
	if err != nil {
		return "", err
	}
	return cfg.Endpoint, nil
}

// GetCloakToken 解密并返回 token 明文。
// 仅 test-connection / driver 内部调用，绝不能写到任何 HTTP 响应或日志。
func (s *ConfigStore) GetCloakToken() (string, error) {
	var row models.CloakSettings
	if err := s.db.DB.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	if strings.TrimSpace(row.TokenEncrypted) == "" {
		return "", nil
	}
	plain, err := s.DecryptCookie(row.TokenEncrypted)
	if err != nil {
		return "", err
	}
	return plain, nil
}

// SetCloakEndpoint 仅更新 endpoint 字段，不影响已存 token。
func (s *ConfigStore) SetCloakEndpoint(endpoint string) error {
	endpoint = strings.TrimSpace(endpoint)
	db := s.db.DB
	var row models.CloakSettings
	if err := db.First(&row).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		row = models.CloakSettings{Endpoint: endpoint}
		return db.Create(&row).Error
	}
	row.Endpoint = endpoint
	return db.Save(&row).Error
}

// SetCloakToken 加密后落库；空字符串表示清除 token。
func (s *ConfigStore) SetCloakToken(plaintext string) error {
	db := s.db.DB
	var row models.CloakSettings
	if err := db.First(&row).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		row = models.CloakSettings{}
		if strings.TrimSpace(plaintext) != "" {
			cipherText, encErr := s.EncryptCookie(plaintext)
			if encErr != nil {
				return encErr
			}
			row.TokenEncrypted = cipherText
		}
		return db.Create(&row).Error
	}
	if strings.TrimSpace(plaintext) == "" {
		row.TokenEncrypted = ""
	} else {
		cipherText, encErr := s.EncryptCookie(plaintext)
		if encErr != nil {
			return encErr
		}
		row.TokenEncrypted = cipherText
	}
	return db.Save(&row).Error
}

// SaveCloakConfig 在单次事务里同时更新 endpoint + token；
// emptyToken=true 时显式清空 token，否则 plaintextToken=="" 表示保持不变（部分更新）。
func (s *ConfigStore) SaveCloakConfig(endpoint, plaintextToken string, clearToken bool) error {
	endpoint = strings.TrimSpace(endpoint)
	return s.db.WithTransaction(func(tx *gorm.DB) error {
		var row models.CloakSettings
		err := tx.First(&row).Error
		isNew := false
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			isNew = true
			row = models.CloakSettings{}
		}
		row.Endpoint = endpoint
		switch {
		case clearToken:
			row.TokenEncrypted = ""
		case strings.TrimSpace(plaintextToken) != "":
			cipherText, encErr := s.EncryptCookie(plaintextToken)
			if encErr != nil {
				return encErr
			}
			row.TokenEncrypted = cipherText
		}
		if isNew {
			return tx.Create(&row).Error
		}
		return tx.Save(&row).Error
	})
}
