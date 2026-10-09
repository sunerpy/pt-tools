package models

import "time"

// RemoteSetting 是远程访问（路线图 M15）的设置，只有一行（ID 为 1）。默认关闭；列的默认值一律是零值。
type RemoteSetting struct {
	ID      uint `gorm:"primaryKey"`
	Enabled bool `gorm:"not null;default:false"`
	// RelaysJSON 是 relay 地址（ws:// 或 wss://）的 JSON 数组
	RelaysJSON string `gorm:"type:text;default:''"`
	// DirectURL 是 App 直连这台 pt-tools 用的地址（http:// 或 https://，可以带反向代理的子路径），空 = 配对链接里不带
	DirectURL string `gorm:"size:512;default:''"`
	// HostKeysEncrypted 是加密后的主机密钥（Ed25519 与 X25519 的私钥）。第一次打开远程访问时生成，轮换时整个换掉
	HostKeysEncrypted string `gorm:"type:text;default:''"`
	UpdatedAt         time.Time
}

// TableName 指定表名。
func (RemoteSetting) TableName() string { return "remote_settings" }

// RemoteDevice 是配对过的设备。撤销只写 RevokedAt，记录留着（审计里的设备编号还对得上），撤销以后才能删掉记录。
type RemoteDevice struct {
	ID   uint   `gorm:"primaryKey"`
	Name string `gorm:"size:64;not null;default:''"`
	// PublicKey 是设备的 X25519 公钥（base64url）；同一把公钥同时只能有一台没撤销的设备
	PublicKey string `gorm:"size:64;not null;default:'';index"`
	// Scopes 是空格分隔的权限范围（app:read，或 app:read app:write）
	Scopes      string `gorm:"size:64;not null;default:''"`
	CreatedAt   time.Time
	LastSeenAt  *time.Time
	LastSeenVia string     `gorm:"size:16;not null;default:''"`
	RevokedAt   *time.Time `gorm:"index"`
}

// TableName 指定表名。
func (RemoteDevice) TableName() string { return "remote_devices" }
