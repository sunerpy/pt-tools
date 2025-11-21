package core

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/utils"
	"gorm.io/gorm"
)

type ConfigStore struct {
	db *models.TorrentDB
}

func NewConfigStore(db *models.TorrentDB) *ConfigStore {
	return &ConfigStore{db: db}
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
			// default auto start to false if column missing; migrate via SaveGlobal when user sets
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
			sc := models.SiteConfig{Enabled: boolPtr(sitem.Enabled), AuthMethod: sitem.AuthMethod, Cookie: sitem.Cookie, APIKey: sitem.APIKey, APIUrl: sitem.APIUrl}
			var rss []models.RSSSubscription
			if e := tx.Where("site_id = ?", sitem.ID).Find(&rss).Error; e != nil {
				return e
			}
			for _, r := range rss {
				sc.RSS = append(sc.RSS, models.RSSConfig{ID: r.ID, Name: r.Name, URL: r.URL, Category: r.Category, Tag: r.Tag, IntervalMinutes: r.IntervalMinutes})
			}
			// 应用默认站点认证方式与 API URL（只读快照层，不写库）
			switch sg {
			case models.MTEAM:
				sc.AuthMethod = "api_key"
				sc.APIUrl = models.DefaultAPIUrlMTeam
			case models.CMCT, models.HDSKY:
				sc.AuthMethod = "cookie"
				sc.APIUrl = ""
			}
			out.Sites[sg] = sc
		}
		return nil
	}); err != nil {
		return nil, err
	}
	// optional: validate here if needed
	return &out, nil
}

// Save* 方法：供 Web 写入配置
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
	return out, nil
}

// Unified API structures: use models.SettingsGlobal for external I/O
func (s *ConfigStore) GetGlobalSettings() (models.SettingsGlobal, error) {
	var gs models.SettingsGlobal
	if err := s.db.DB.First(&gs).Error; err != nil {
		// provide defaults
		gs.DefaultIntervalMinutes = int32(durationToMinutes(20 * time.Minute))
		gs.DefaultEnabled = true
		gs.DownloadDir = "download"
		gs.DownloadLimitEnabled = false
		gs.DownloadSpeedLimit = 20
		gs.TorrentSizeGB = 200
		gs.AutoStart = false
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
		cur.AutoStart = gs.AutoStart
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
	row.APIKey = sc.APIKey
	row.APIUrl = sc.APIUrl
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
			SiteID:          siteID,
			Name:            r.Name,
			URL:             r.URL,
			Category:        r.Category,
			Tag:             r.Tag,
			IntervalMinutes: r.IntervalMinutes,
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
	global.GetSlogger().Infof("admin_users users=%v count=%d", names, len(users))
	var cnt int64
	err := s.db.DB.Model(&models.AdminUser{}).Count(&cnt).Error
	return cnt, err
}

// UpsertSiteWithRSS 原子性保存站点与 RSS，并进行严格校验
func (s *ConfigStore) UpsertSiteWithRSS(site models.SiteGroup, sc models.SiteConfig) error {
	// 严格校验：
	// 1) 认证方式必填且合法；
	// 2) 根据认证方式二选一且对应字段不为空（api_key 或 cookie），另一项必须为空；
	// 3) API URL 必填；
	// 4) RSS 列表至少一条，且各项字段合法。
	am := strings.ToLower(strings.TrimSpace(sc.AuthMethod))
	if am != "cookie" && am != "api_key" {
		return errors.New("认证方式必须为 'cookie' 或 'api_key'")
	}
	if strings.TrimSpace(sc.APIUrl) == "" {
		return errors.New("API 地址不能为空")
	}
	apiKeyEmpty := strings.TrimSpace(sc.APIKey) == ""
	cookieEmpty := strings.TrimSpace(sc.Cookie) == ""
	if am == "api_key" {
		if apiKeyEmpty {
			return errors.New("API Key 不能为空")
		}
		if !cookieEmpty {
			return errors.New("认证方式为 api_key 时 Cookie 必须留空")
		}
	} else { // cookie
		if cookieEmpty {
			return errors.New("cookie 不能为空")
		}
		if !apiKeyEmpty {
			return errors.New("认证方式为 cookie 时 API Key 必须留空")
		}
	}
	if len(sc.RSS) == 0 {
		return errors.New("RSS 列表不能为空")
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
	// 事务保存站点与 RSS
	return s.db.DB.Transaction(func(tx *gorm.DB) error {
		var row models.SiteSetting
		if err := tx.Where("name = ?", string(site)).First(&row).Error; err != nil {
			row = models.SiteSetting{Name: string(site)}
		}
		if sc.Enabled != nil {
			row.Enabled = *sc.Enabled
		}
		row.AuthMethod = sc.AuthMethod
		row.Cookie = sc.Cookie
		row.APIKey = sc.APIKey
		row.APIUrl = sc.APIUrl
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		// 替换 RSS
		if err := tx.Where("site_id = ?", row.ID).Delete(&models.RSSSubscription{}).Error; err != nil {
			return err
		}
		for _, r := range sc.RSS {
			if r.IntervalMinutes < models.MinIntervalMinutes {
				r.IntervalMinutes = models.MinIntervalMinutes
			}
			rr := models.RSSSubscription{SiteID: row.ID, Name: r.Name, URL: r.URL, Category: r.Category, Tag: r.Tag, IntervalMinutes: r.IntervalMinutes}
			if err := tx.Create(&rr).Error; err != nil {
				return err
			}
		}
		events.Publish(events.Event{Type: events.ConfigChanged, Version: time.Now().UnixNano(), Source: "sites", At: time.Now()})
		return nil
	})
}

// DeleteSite 删除站点（预置站点禁止删除）
func (s *ConfigStore) DeleteSite(name string) error {
	lower := strings.ToLower(name)
	if lower == "cmct" || lower == "hdsky" || lower == "mteam" {
		return errors.New("预置站点不可删除")
	}
	tx := s.db.DB.Begin()
	var site models.SiteSetting
	if err := tx.Where("name = ?", lower).First(&site).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Where("site_id = ?", site.ID).Delete(&models.RSSSubscription{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Delete(&site).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit().Error; err != nil {
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
		sc := models.SiteConfig{Enabled: boolPtr(ss.Enabled), AuthMethod: ss.AuthMethod, Cookie: ss.Cookie, APIKey: ss.APIKey, APIUrl: ss.APIUrl}
		var rss []models.RSSSubscription
		if err := s.db.DB.Where("site_id = ?", ss.ID).Find(&rss).Error; err != nil {
			return nil, err
		}
		for _, r := range rss {
			sc.RSS = append(sc.RSS, models.RSSConfig{ID: r.ID, Name: r.Name, URL: r.URL, Category: r.Category, Tag: r.Tag, IntervalMinutes: r.IntervalMinutes})
		}
		switch sg {
		case models.MTEAM:
			sc.AuthMethod = "api_key"
			sc.APIUrl = models.DefaultAPIUrlMTeam
		case models.CMCT, models.HDSKY:
			sc.AuthMethod = "cookie"
			sc.APIUrl = ""
		}
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
	sc := models.SiteConfig{Enabled: boolPtr(ss.Enabled), AuthMethod: ss.AuthMethod, Cookie: ss.Cookie, APIKey: ss.APIKey, APIUrl: ss.APIUrl}
	var rss []models.RSSSubscription
	if err := s.db.DB.Where("site_id = ?", ss.ID).Find(&rss).Error; err != nil {
		return models.SiteConfig{}, err
	}
	for _, r := range rss {
		sc.RSS = append(sc.RSS, models.RSSConfig{ID: r.ID, Name: r.Name, URL: r.URL, Category: r.Category, Tag: r.Tag, IntervalMinutes: r.IntervalMinutes})
	}
	// 读取单站点时也应用默认认证方式与 API URL
	switch name {
	case models.MTEAM:
		sc.AuthMethod = "api_key"
		sc.APIUrl = models.DefaultAPIUrlMTeam
	case models.CMCT, models.HDSKY:
		sc.AuthMethod = "cookie"
		sc.APIUrl = ""
	}
	return sc, nil
}
