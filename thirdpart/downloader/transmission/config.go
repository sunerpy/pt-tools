package transmission

import (
	"errors"
	"strings"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// TransmissionConfig Transmission 配置
type TransmissionConfig struct {
	URL       string `json:"url"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	HTTPS     bool   `json:"https"`
	AutoStart bool   `json:"auto_start"`
}

// GetType 获取下载器类型
func (c *TransmissionConfig) GetType() downloader.DownloaderType {
	return downloader.DownloaderTransmission
}

// GetURL 获取下载器 URL（自动去除尾斜杠）
func (c *TransmissionConfig) GetURL() string {
	return strings.TrimSuffix(c.URL, "/")
}

// GetUsername 获取用户名
func (c *TransmissionConfig) GetUsername() string {
	return c.Username
}

// GetPassword 获取密码
func (c *TransmissionConfig) GetPassword() string {
	return c.Password
}

// GetAutoStart 获取是否自动开始下载
func (c *TransmissionConfig) GetAutoStart() bool {
	return c.AutoStart
}

// Validate 验证配置是否有效
func (c *TransmissionConfig) Validate() error {
	if c.URL == "" {
		return errors.New("Transmission URL is required")
	}
	return nil
}

// NewTransmissionConfig 创建 Transmission 配置
func NewTransmissionConfig(url, username, password string) *TransmissionConfig {
	return &TransmissionConfig{
		URL:       url,
		Username:  username,
		Password:  password,
		HTTPS:     false,
		AutoStart: false,
	}
}

// NewTransmissionConfigWithAutoStart 创建带 auto_start 的 Transmission 配置
func NewTransmissionConfigWithAutoStart(url, username, password string, autoStart bool) *TransmissionConfig {
	return &TransmissionConfig{
		URL:       url,
		Username:  username,
		Password:  password,
		HTTPS:     false,
		AutoStart: autoStart,
	}
}
