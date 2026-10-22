package models

import "time"

// MCPSetting 是 MCP 接入的设置（只有 ID 1 一行）：开关与地址的路径。关着时外部的 AI 客户端连不上（内置 AI 助手在进程内
// 调工具，不受它影响）。升级上来第一次启动时，已经有带 MCP 权限的令牌就记成开着，免得已经接好的客户端断掉。
type MCPSetting struct {
	ID      uint `gorm:"primaryKey" json:"-"`
	Enabled bool `gorm:"not null;default:false" json:"enabled"`
	// Path 是 MCP 的地址路径（/mcp 这样，以 / 开头、末尾不带 /）
	Path      string    `gorm:"size:128;not null;default:'/mcp'" json:"path"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (MCPSetting) TableName() string { return "mcp_settings" }
