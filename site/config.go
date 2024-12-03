package site

import (
	"github.com/sunerpy/pt-tools/config"
)

type SiteMapConfig struct {
	Name          string            // 站点名称
	SharedConfig  *SharedSiteConfig // 共享配置
	Config        config.SiteConfig
	Parser        SiteParser        // 站点特定解析器
	CustomHeaders map[string]string // 特定站点覆盖的请求头
}
type SharedSiteConfig struct {
	Cookie  string            // 登录 Cookie
	Headers map[string]string // 通用 HTTP 请求头
	SiteCfg SiteConfig        // 动态配置，包括 Referer
}
type SiteConfig struct {
	RefererConf RefererProvider
}

// 工厂方法创建 SharedSiteConfig
func newSharedSiteConfig(cookie string, refererProvider RefererProvider) *SharedSiteConfig {
	return &SharedSiteConfig{
		Cookie: cookie,
		Headers: map[string]string{
			"User-Agent": "Mozilla/5.0",
		},
		SiteCfg: SiteConfig{
			RefererConf: refererProvider,
		},
	}
}

// 单个 URL 创建单个 SiteMapConfig
func NewSiteMapConfig(name, cookie string, conf config.SiteConfig, parser SiteParser) *SiteMapConfig {
	refererProvider := NewDefaultReferer(name)
	sharedCfg := newSharedSiteConfig(cookie, refererProvider)
	return &SiteMapConfig{
		Name:         name,
		Config:       conf,
		SharedConfig: sharedCfg,
		Parser:       parser,
	}
}
