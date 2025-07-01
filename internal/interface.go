package internal

import (
	"context"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/sunerpy/pt-tools/config"
	"github.com/sunerpy/pt-tools/models"
)

// todo SendTorrentToQbit会从库里面查找记录  历史记录为免费但原来没有下载的话 会继续发送
type PTSiteInter[T models.ResType] interface {
	GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[T], error)
	IsEnabled() bool
	DownloadTorrent(url, title, downloadDir string) (string, error)
	MaxRetries() int
	RetryDelay() time.Duration
	SendTorrentToQbit(ctx context.Context, rssCfg config.RSSConfig) error
	Context() context.Context
}
