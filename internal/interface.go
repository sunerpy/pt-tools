package internal

import (
	"context"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/sunerpy/pt-tools/config"
	"github.com/sunerpy/pt-tools/models"
)

type PTSiteInter[T models.ResType] interface {
	GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[T], error)
	IsEnabled() bool
	DownloadTorrent(url, title, downloadDir string) (string, error)
	MaxRetries() int
	RetryDelay() time.Duration
	SendTorrentToQbit(ctx context.Context, rssCfg config.RSSConfig) error
	Context() context.Context
}
