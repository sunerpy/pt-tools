package models

import "time"

// CookieCloud 同步的取值范围。
const (
	CookieCloudDefaultIntervalHours = 24
	CookieCloudMinIntervalHours     = 1
	CookieCloudMaxIntervalHours     = 168
)

// CookieCloudSetting 是从自建 CookieCloud 服务导入站点 Cookie 的设置（只有一行，ID 为 1）。默认不定时同步；列的默认值一律是零值。
type CookieCloudSetting struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// ServerURL 是 CookieCloud 服务地址，UUID 是浏览器扩展里的用户 KEY。
	ServerURL string `gorm:"size:512;default:''" json:"server_url"`
	UUID      string `gorm:"size:128;default:''" json:"uuid"`
	// PasswordEncrypted 是加密后的端对端加密密码；接口只返回是否已设置。密码只在本机解密用，不发给 CookieCloud 服务。
	PasswordEncrypted string `gorm:"type:text;default:''" json:"-"`
	// AutoSync 打开后按间隔把有变化的 Cookie 写进已启用的站点。
	AutoSync bool `gorm:"not null;default:false" json:"auto_sync"`
	// IntervalHours 是定时同步的间隔（0 时按 CookieCloudDefaultIntervalHours）。
	IntervalHours int        `gorm:"not null;default:0" json:"interval_hours"`
	LastSyncAt    *time.Time `json:"last_sync_at,omitempty"`
	// LastResult 是上一次同步的一句话结果（不含 Cookie 内容）。
	LastResult string    `gorm:"type:text;default:''" json:"last_result"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (CookieCloudSetting) TableName() string { return "cookiecloud_settings" }
