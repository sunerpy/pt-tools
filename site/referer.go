package site

import (
	"strings"

	"go.uber.org/zap"
)

type RefererProvider interface {
	GetReferer() string
}
type DefaultReferer struct {
	siteName   string
	refererMap map[string]string
}

// NewDefaultReferer 构造函数，用于初始化 refererMap
func NewDefaultReferer(siteName string) *DefaultReferer {
	return &DefaultReferer{
		siteName: siteName,
		refererMap: map[string]string{
			"hdsky":   "https://hdsky.me/",
			"example": "https://example.com/",
		},
	}
}

// GetReferer 实现 RefererProvider 接口
func (d *DefaultReferer) GetReferer() string {
	if referer, ok := d.refererMap[strings.ToLower(d.siteName)]; ok {
		return referer
	}
	logger.Fatal("无法找到默认 Referer", zap.String("siteName", d.siteName))
	return ""
}
