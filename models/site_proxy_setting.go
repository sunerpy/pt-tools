package models

import "time"

// SiteProxySetting 是全局设置里的「站点代理」：哪些站点的请求经哪个代理发出。只有一行（id=1），没有这一行表示关闭。
// 读写只经 core.ConfigStore（代理地址在那里加解密）。
type SiteProxySetting struct {
	ID      uint `gorm:"primaryKey" json:"-"`
	Enabled bool `gorm:"not null;default:false" json:"enabled"`
	// ProxyEncrypted 是加密后的代理地址（可能带用户名密码）
	ProxyEncrypted string `gorm:"type:text" json:"-"`
	// Mode 是 all（全部站点）、include（只有选中的）或 exclude（除了选中的）
	Mode string `gorm:"size:16;not null;default:'all'" json:"mode"`
	// SitesJSON 是选中站点 ID 的 JSON 数组（规范化后的小写 ID）
	SitesJSON string    `gorm:"type:text" json:"-"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (SiteProxySetting) TableName() string { return "site_proxy_settings" }
