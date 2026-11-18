package models

import "time"

// APIToken 是给 App、MCP 与 qB 兼容入口用的 API 令牌（路线图 M12）。令牌是 `ptt_<id>_<secret>`：明文只在新建时给一次，
// 库里只存 secret 的 SHA-256。撤销就是删掉这一行，立即生效。
type APIToken struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Name string `gorm:"size:64;not null;default:''" json:"name"`
	// SecretHash 是 secret 的 SHA-256（十六进制）
	SecretHash string `gorm:"size:64;not null;default:''" json:"-"`
	// Scopes 是空格分隔的权限范围（app:read app:write 等）
	Scopes     string     `gorm:"size:255;not null;default:''" json:"-"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedBy  string     `gorm:"size:64;not null;default:''" json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (APIToken) TableName() string { return "api_tokens" }
