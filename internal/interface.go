package internal

import (
	"context"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// PTSiteInter 旧的泛型接口（已废弃，请使用 UnifiedPTSite）
// Deprecated: Use UnifiedPTSite instead for new implementations
type PTSiteInter[T models.ResType] interface {
	GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[T], error)
	IsEnabled() bool
	DownloadTorrent(url, title, downloadDir string) (string, error)
	MaxRetries() int
	RetryDelay() time.Duration
	SendTorrentToDownloader(ctx context.Context, rssCfg models.RSSConfig) error
	Context() context.Context
}

// UnifiedPTSite 统一的 PT 站点接口（非泛型）
// 新增站点应使用此接口，无需创建新的实现类
type UnifiedPTSite interface {
	// GetTorrentDetails 获取种子详情，返回统一的 TorrentItem
	GetTorrentDetails(item *gofeed.Item) (*v2.TorrentItem, error)
	// IsEnabled 检查站点是否启用
	IsEnabled() bool
	// DownloadTorrent 下载种子文件，返回 torrent hash
	DownloadTorrent(url, title, downloadDir string) (string, error)
	// MaxRetries 返回最大重试次数
	MaxRetries() int
	// RetryDelay 返回重试间隔
	RetryDelay() time.Duration
	// SendTorrentToDownloader 发送种子到下载器
	SendTorrentToDownloader(ctx context.Context, rssCfg models.RSSConfig) error
	// Context 返回上下文
	Context() context.Context
	// SiteGroup 返回站点分组标识
	SiteGroup() models.SiteGroup
}
