package models

import (
	"time"

	"go.uber.org/zap"
)

// DiscountType 定义优惠类型
type DiscountType string

const (
	DISCOUNT_NONE        DiscountType = "none"
	DISCOUNT_FREE        DiscountType = "free"
	DISCOUNT_TWO_X       DiscountType = "2x"
	DISCOUNT_TWO_X_FREE  DiscountType = "2xfree"
	DISCOUNT_THIRTY      DiscountType = "30%"
	DISCOUNT_FIFTY       DiscountType = "50%"
	DISCOUNT_TWO_X_FIFTY DiscountType = "2x50%"
	DISCOUNT_CUSTOM      DiscountType = "custom"
)

// PHPTorrentInfo 定义种子信息结构
type PHPTorrentInfo struct {
	Title     string       // 种子标题
	SubTitle  string       // 副标题/标签
	TorrentID string       // 种子 ID
	Discount  DiscountType // 优惠类型
	EndTime   time.Time    // 优惠结束时间
	SizeMB    float64      // 种子大小，单位为 MB
	Seeders   int          // 做种人数
	Leechers  int          // 下载人数
	Completed float64      // 最大完成百分比
	HR        bool         // 是否为 HR（Hit & Run）
}

func (p PHPTorrentInfo) IsFree() bool {
	if p.Discount == DISCOUNT_FREE || p.Discount == DISCOUNT_TWO_X_FREE {
		return true
	}
	return false
}

func (p PHPTorrentInfo) CanbeFinished(logger *zap.SugaredLogger, enabled bool, speedLimit, sizeLimitGB int) bool {
	// 种子大小检查独立于限速开关，只要设置了大小限制就生效
	if sizeLimitGB > 0 && p.SizeMB >= float64(sizeLimitGB*1024) {
		logger.Warn("种子大小超过设定值,跳过...")
		return false
	}
	// 限速检查仅在启用时生效
	if enabled && speedLimit > 0 {
		duration := time.Until(p.EndTime)
		secondsDiff := int(duration.Seconds())
		if float64(secondsDiff)*float64(speedLimit) < (p.SizeMB / 1024 / 1024) {
			logger.Warn("种子免费时间不足以完成下载,跳过...")
			return false
		}
	}
	return true
}

func (p PHPTorrentInfo) GetFreeEndTime() *time.Time {
	time := p.EndTime
	return &time
}

func (p PHPTorrentInfo) GetFreeLevel() string {
	if p.Discount != "" {
		return string(p.Discount)
	}
	return "failed"
}

// GetName 获取种子名称
func (p PHPTorrentInfo) GetName() string {
	return p.Title
}

// GetSubTitle 获取副标题
func (p PHPTorrentInfo) GetSubTitle() string {
	return p.SubTitle
}
