package models

import (
	"time"

	"gorm.io/gorm"
)

// NasSource 是「NAS 监控」里的一个来源（首页 NAS 概览 H2）：TrueNAS、Glances、群晖、飞牛。本机来源不在这张表里，
// 它的开关在 NasSetting。读写只经 web 的 /api/nas* 与 cmd 的 nas_wiring.go。
//
// SecretEncrypted 是 API Key 或密码，ExtraEncrypted 是来源刷新出的长期凭据（群晖的设备令牌、飞牛的 token，JSON），
// 都是 AES-GCM 密文（与站点 Cookie 同一把密钥），接口只回有没有。TLSPinSHA256 是用户确认信任的证书指纹（小写十六进制，
// 空表示只按系统根证书校验）；InsecureAck 是用户确认过明文 http 连接的风险。Revision 每次保存加一：采集器按它判断
// 设置变没变，写回刷新的凭据时也只在它没变时写，免得盖掉用户刚改的设置。
//
// 布尔列故意不带 gorm 的 default：gorm 建行时会跳过零值字段、改用数据库默认值，用户关掉的开关会被存成开着。
type NasSource struct {
	ID              uint   `gorm:"primaryKey"`
	Name            string `gorm:"size:64;not null"`
	Kind            string `gorm:"size:16;not null"`
	Enabled         bool   `gorm:"not null"`
	URL             string `gorm:"column:url;type:text;not null"`
	Username        string `gorm:"size:128"`
	SecretEncrypted string `gorm:"column:secret_encrypted;type:text"`
	ExtraEncrypted  string `gorm:"column:extra_encrypted;type:text"`
	TLSPinSHA256    string `gorm:"column:tls_pin_sha256;size:64"`
	InsecureAck     bool   `gorm:"column:insecure_ack;not null"`
	Revision        int64  `gorm:"not null"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (NasSource) TableName() string { return "nas_sources" }

// NasSetting 是「NAS 监控」的设置，只有一行（id=1）；没有这一行时按默认：本机来源开着、首页自动选来源。
// HomeSource 是首页显示哪个来源：""（自动：第一个启用的来源，没有时本机）、"local" 或来源的编号。
type NasSetting struct {
	ID           uint   `gorm:"primaryKey"`
	LocalEnabled bool   `gorm:"not null"`
	HomeSource   string `gorm:"size:32"`
	UpdatedAt    time.Time
}

func (NasSetting) TableName() string { return "nas_settings" }

// GetNasSetting 读「NAS 监控」的设置；没有这一行时给默认（本机来源开着、首页自动选来源）。
func GetNasSetting(db *gorm.DB) (NasSetting, error) {
	var s NasSetting
	err := db.Where("id = ?", 1).Limit(1).Find(&s).Error
	if err != nil {
		return NasSetting{}, err
	}
	if s.ID == 0 {
		return NasSetting{ID: 1, LocalEnabled: true}, nil
	}
	return s, nil
}

// SaveNasSetting 写「NAS 监控」的设置（只有 id=1 一行）。
func SaveNasSetting(db *gorm.DB, s NasSetting) error {
	s.ID = 1
	return db.Save(&s).Error
}
