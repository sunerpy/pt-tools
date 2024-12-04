package site

import (
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"go.uber.org/zap"
)

type RefererProvider interface {
	GetReferer() string
}
type DefaultReferer struct {
	siteName   models.SiteGroup
	refererMap map[models.SiteGroup]string
}

// NewDefaultReferer 构造函数，用于初始化 refererMap
func NewDefaultReferer(siteName models.SiteGroup) *DefaultReferer {
	return &DefaultReferer{
		siteName: siteName,
		refererMap: map[models.SiteGroup]string{
			models.HDSKY: "https://hdsky.me/",
			models.CMCT:  "https://springsunday.net/",
		},
	}
}

// GetReferer 实现 RefererProvider 接口
func (d *DefaultReferer) GetReferer() string {
	if referer, ok := d.refererMap[d.siteName]; ok {
		return referer
	}
	global.GlobalLogger.Fatal("无法找到默认 Referer", zap.String("siteName", string(d.siteName)))
	return ""
}
