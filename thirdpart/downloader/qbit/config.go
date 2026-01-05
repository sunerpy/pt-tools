package qbit

import (
	"errors"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// QBitConfig qBittorrent 配置
type QBitConfig struct {
	URL       string `json:"url"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	AutoStart bool   `json:"auto_start"`
}

// GetType 获取下载器类型
func (c *QBitConfig) GetType() downloader.DownloaderType {
	return downloader.DownloaderQBittorrent
}

// GetURL 获取下载器 URL
func (c *QBitConfig) GetURL() string {
	return c.URL
}

// GetUsername 获取用户名
func (c *QBitConfig) GetUsername() string {
	return c.Username
}

// GetPassword 获取密码
func (c *QBitConfig) GetPassword() string {
	return c.Password
}

// GetAutoStart 获取是否自动开始下载
func (c *QBitConfig) GetAutoStart() bool {
	return c.AutoStart
}

// Validate 验证配置是否有效
func (c *QBitConfig) Validate() error {
	if c.URL == "" {
		return errors.New("qBittorrent URL is required")
	}
	return nil
}

// NewQBitConfig 创建 qBittorrent 配置
func NewQBitConfig(url, username, password string) *QBitConfig {
	return &QBitConfig{
		URL:       url,
		Username:  username,
		Password:  password,
		AutoStart: false,
	}
}

// NewQBitConfigWithAutoStart 创建带 auto_start 的 qBittorrent 配置
func NewQBitConfigWithAutoStart(url, username, password string, autoStart bool) *QBitConfig {
	return &QBitConfig{
		URL:       url,
		Username:  username,
		Password:  password,
		AutoStart: autoStart,
	}
}
