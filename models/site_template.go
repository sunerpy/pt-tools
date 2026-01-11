package models

import (
	"encoding/json"
	"time"
)

// SiteTemplate 站点模板
// 用于导入/导出站点配置（不包含敏感信息）
type SiteTemplate struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"uniqueIndex;size:64;not null" json:"name"`
	DisplayName  string    `gorm:"size:128" json:"display_name"`
	BaseURL      string    `gorm:"size:512;not null" json:"base_url"`
	AuthMethod   string    `gorm:"size:16;not null" json:"auth_method"` // cookie, api_key
	ParserConfig string    `gorm:"type:text" json:"parser_config"`      // JSON格式的解析器配置
	Description  string    `gorm:"size:1024" json:"description"`
	Version      string    `gorm:"size:32" json:"version"`
	Author       string    `gorm:"size:64" json:"author"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName 指定表名
func (SiteTemplate) TableName() string {
	return "site_templates"
}

// SiteTemplateExport 导出格式（不包含数据库字段）
type SiteTemplateExport struct {
	Name         string          `json:"name"`
	DisplayName  string          `json:"display_name"`
	BaseURL      string          `json:"base_url"`
	AuthMethod   string          `json:"auth_method"`
	ParserConfig json.RawMessage `json:"parser_config,omitempty"`
	Description  string          `json:"description,omitempty"`
	Version      string          `json:"version,omitempty"`
	Author       string          `json:"author,omitempty"`
}

// ToExport 转换为导出格式
func (t *SiteTemplate) ToExport() *SiteTemplateExport {
	export := &SiteTemplateExport{
		Name:        t.Name,
		DisplayName: t.DisplayName,
		BaseURL:     t.BaseURL,
		AuthMethod:  t.AuthMethod,
		Description: t.Description,
		Version:     t.Version,
		Author:      t.Author,
	}
	if t.ParserConfig != "" {
		export.ParserConfig = json.RawMessage(t.ParserConfig)
	}
	return export
}

// FromExport 从导出格式创建
func (t *SiteTemplate) FromExport(export *SiteTemplateExport) error {
	t.Name = export.Name
	t.DisplayName = export.DisplayName
	t.BaseURL = export.BaseURL
	t.AuthMethod = export.AuthMethod
	t.Description = export.Description
	t.Version = export.Version
	t.Author = export.Author
	if export.ParserConfig != nil {
		t.ParserConfig = string(export.ParserConfig)
	}
	return nil
}

// DynamicSiteSetting 动态站点设置
// 扩展SiteSetting以支持动态站点
type DynamicSiteSetting struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"uniqueIndex;size:64;not null" json:"name"`
	DisplayName  string    `gorm:"size:128" json:"display_name"`
	BaseURL      string    `gorm:"size:512" json:"base_url"`
	Enabled      bool      `json:"enabled"`
	AuthMethod   string    `gorm:"size:16;not null" json:"auth_method"` // cookie, api_key
	Cookie       string    `gorm:"size:2048" json:"cookie,omitempty"`
	APIKey       string    `gorm:"size:512" json:"api_key,omitempty"`
	APIURL       string    `gorm:"size:512" json:"api_url,omitempty"`
	DownloaderID *uint     `gorm:"index" json:"downloader_id,omitempty"` // 关联的下载器ID
	ParserConfig string    `gorm:"type:text" json:"parser_config"`       // JSON格式的解析器配置
	IsBuiltin    bool      `json:"is_builtin"`                           // 是否为内置站点
	TemplateID   *uint     `gorm:"index" json:"template_id,omitempty"`   // 关联的模板ID
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName 指定表名
func (DynamicSiteSetting) TableName() string {
	return "dynamic_site_settings"
}

// DownloaderSetting 下载器设置
type DownloaderSetting struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"uniqueIndex;size:64;not null" json:"name"`
	Type        string    `gorm:"size:32;not null" json:"type"` // qbittorrent, transmission
	URL         string    `gorm:"size:512;not null" json:"url"`
	Username    string    `gorm:"size:128" json:"username"`
	Password    string    `gorm:"size:256" json:"password"`
	IsDefault   bool      `json:"is_default"`
	Enabled     bool      `json:"enabled"`
	AutoStart   bool      `json:"auto_start"`                              // 推送种子后自动开始下载
	ExtraConfig string    `gorm:"type:text" json:"extra_config,omitempty"` // JSON格式的额外配置
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 指定表名
func (DownloaderSetting) TableName() string {
	return "downloader_settings"
}

// DownloaderDirectory 下载器目录配置
// 用于设置下载器的多个下载目录及其别名
type DownloaderDirectory struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	DownloaderID uint      `gorm:"index;not null" json:"downloader_id"` // 关联的下载器ID
	Path         string    `gorm:"size:512;not null" json:"path"`       // 目录路径
	Alias        string    `gorm:"size:128" json:"alias"`               // 目录别名（用于显示）
	IsDefault    bool      `json:"is_default"`                          // 是否为默认目录
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName 指定表名
func (DownloaderDirectory) TableName() string {
	return "downloader_directories"
}
