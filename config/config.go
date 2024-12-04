package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/models"
)

type DirConf struct {
	HomeDir     string
	WorkDir     string
	DownloadDir string
}

// 全局配置
type GlobalConfig struct {
	DefaultInterval      time.Duration `mapstructure:"default_interval"`
	DefaultEnabled       bool          `mapstructure:"default_enabled"`
	DownloadDir          string        `mapstructure:"download_dir"`
	DownloadLimitEnabled bool          `mapstructure:"download_limit_enabled"`
	DownloadSpeedLimit   int           `mapstructure:"download_speed_limit"`
}

// qBittorrent 配置
type QbitConfig struct {
	Enabled  bool   `mapstructure:"enabled"`  // 是否启用
	URL      string `mapstructure:"url"`      // qBittorrent API URL
	User     string `mapstructure:"user"`     // qBittorrent 用户名
	Password string `mapstructure:"password"` // qBittorrent 密码
}

// 单个 RSS 配置
type RSSConfig struct {
	Name            string `mapstructure:"name"`
	URL             string `mapstructure:"url"`               // RSS 订阅链接
	Category        string `mapstructure:"category"`          // qBittorrent 分类
	Tag             string `mapstructure:"tag"`               // qBittorrent 标签
	IntervalMinutes int32  `mapstructure:"interval_minutes"`  // 抓取间隔（覆盖全局）
	DownloadSubPath string `mapstructure:"download_sub_path"` // 下载目录
}

// 单个站点的配置
type SiteConfig struct {
	Enabled    *bool       `mapstructure:"enabled"`     // 是否启用，优先级高于全局默认值
	Name       string      `mapstructure:"name"`        // 站点名称
	AuthMethod string      `mapstructure:"auth_method"` // 认证方式: "cookie" 或 "api_key"
	Cookie     string      `mapstructure:"cookie"`      // 登录 Cookie
	APIKey     string      `mapstructure:"api_key"`     // API Token（如果需要）
	APIUrl     string      `mapstructure:"api_url"`
	RSS        []RSSConfig `mapstructure:"rss"` // 多个 RSS 配置
}

// 完整配置
type Config struct {
	Global GlobalConfig `mapstructure:"global"` // 全局默认配置
	// Zap    Zap                             `mapstructure:"zap"`
	Qbit  QbitConfig                      `mapstructure:"qbit"`  // 全局 qBittorrent 配置
	Sites map[models.SiteGroup]SiteConfig `mapstructure:"sites"` // 多个站点配置
}

// Mteam  SiteConfig   `mapstructure:"mteam"`  // 多站点配置
// 允许的站点名称范围
var allowedSites = []string{"cmct", "hdsky", "mteam"}

// 验证 Sites 中的键是否在允许的范围内
func (c *Config) ValidateSites() error {
	for siteName, cfg := range c.Sites {
		_, err := models.ValidateSiteName(string(siteName))
		if err != nil {
			return err
		}
		validAuthMethods := []string{"cookie", "api_key"}
		if !contains(validAuthMethods, cfg.AuthMethod) {
			return fmt.Errorf("无效的认证方式: %s", cfg.AuthMethod)
		}
		if cfg.AuthMethod == "api_key" && cfg.APIKey == "" {
			return fmt.Errorf("API Token 不能为空")
		}
		if cfg.AuthMethod == "cookie" && cfg.Cookie == "" {
			return fmt.Errorf("Cookie 不能为空")
		}
	}
	return nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) { // 忽略大小写
			return true
		}
	}
	return false
}
