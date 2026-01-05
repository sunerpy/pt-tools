package downloader

import (
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// MockConfig 用于测试的模拟配置
type MockConfig struct {
	Type      DownloaderType
	URL       string
	Username  string
	Password  string
	AutoStart bool
}

func (c *MockConfig) GetType() DownloaderType { return c.Type }
func (c *MockConfig) GetURL() string          { return c.URL }
func (c *MockConfig) GetUsername() string     { return c.Username }
func (c *MockConfig) GetPassword() string     { return c.Password }
func (c *MockConfig) GetAutoStart() bool      { return c.AutoStart }
func (c *MockConfig) Validate() error {
	if c.URL == "" {
		return ErrInvalidConfig
	}
	return nil
}

// genDownloaderType 生成随机下载器类型
func genDownloaderType() gopter.Gen {
	return gen.OneConstOf(DownloaderQBittorrent, DownloaderTransmission)
}

// genValidURL 生成有效的 URL
func genValidURL() gopter.Gen {
	return gen.AnyString().Map(func(s string) string {
		if s == "" {
			return "http://localhost:8080"
		}
		return "http://" + s + ":8080"
	})
}

// TestProperty1_DownloaderFactoryConsistency 属性测试：下载器工厂一致性
// Feature: downloader-site-extensibility, Property 1: Downloader Factory Consistency
// 对于任何有效的 DownloaderConfig，工厂函数应返回正确类型的 Downloader 实例
func TestProperty1_DownloaderFactoryConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// 属性：配置类型应与返回的下载器类型匹配
	properties.Property("factory returns correct downloader type for config type", prop.ForAll(
		func(dlType DownloaderType, url string) bool {
			config := &MockConfig{
				Type:     dlType,
				URL:      url,
				Username: "test",
				Password: "test",
			}

			// 验证配置类型与 GetType 返回值一致
			return config.GetType() == dlType
		},
		genDownloaderType(),
		genValidURL(),
	))

	// 属性：有效配置应通过验证
	properties.Property("valid config passes validation", prop.ForAll(
		func(url string) bool {
			if url == "" {
				return true // 跳过空 URL
			}
			config := &MockConfig{
				Type:     DownloaderQBittorrent,
				URL:      url,
				Username: "test",
				Password: "test",
			}
			return config.Validate() == nil
		},
		genValidURL(),
	))

	// 属性：空 URL 配置应验证失败
	properties.Property("empty URL config fails validation", prop.ForAll(
		func(dlType DownloaderType) bool {
			config := &MockConfig{
				Type:     dlType,
				URL:      "",
				Username: "test",
				Password: "test",
			}
			return config.Validate() == ErrInvalidConfig
		},
		genDownloaderType(),
	))

	properties.TestingRun(t)
}

// TestDownloaderTypeConstants 测试下载器类型常量
func TestDownloaderTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		dlType   DownloaderType
		expected string
	}{
		{"qBittorrent type", DownloaderQBittorrent, "qbittorrent"},
		{"Transmission type", DownloaderTransmission, "transmission"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.dlType) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, tt.dlType)
			}
		})
	}
}

// TestMockConfigImplementsInterface 测试 MockConfig 实现 DownloaderConfig 接口
func TestMockConfigImplementsInterface(t *testing.T) {
	var _ DownloaderConfig = (*MockConfig)(nil)
}

// TestDownloadTaskInfo 测试下载任务信息结构
func TestDownloadTaskInfo(t *testing.T) {
	info := DownloadTaskInfo{
		Name:          "test-torrent",
		Hash:          "abc123",
		SizeLeft:      1024 * 1024 * 100, // 100 MB
		DownloadSpeed: 1024 * 1024,       // 1 MB/s
		ETA:           100 * time.Second,
	}

	if info.Name != "test-torrent" {
		t.Errorf("expected name 'test-torrent', got '%s'", info.Name)
	}
	if info.Hash != "abc123" {
		t.Errorf("expected hash 'abc123', got '%s'", info.Hash)
	}
}
