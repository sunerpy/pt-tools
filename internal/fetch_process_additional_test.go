package internal

import (
	"context"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

type ptStub struct{}

func (pt *ptStub) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	d := models.PHPTorrentInfo{Title: item.Title, TorrentID: item.GUID}
	return &models.APIResponse[models.PHPTorrentInfo]{Code: "success", Data: d}, nil
}
func (pt *ptStub) IsEnabled() bool                                        { return true }
func (pt *ptStub) DownloadTorrent(url, title, dir string) (string, error) { return "h", nil }
func (pt *ptStub) MaxRetries() int                                        { return 1 }
func (pt *ptStub) RetryDelay() time.Duration                              { return 0 }
func (pt *ptStub) SendTorrentToDownloader(ctx context.Context, rssCfg models.RSSConfig) error {
	return nil
}
func (pt *ptStub) Context() context.Context { return context.Background() }
func TestProcessRSS_WithStub_NoPanic(t *testing.T) {
	cfg := models.RSSConfig{Name: "r", URL: "http://example/rss", IntervalMinutes: 1}
	// 调用 cmd 包中的 processRSS（与生产逻辑一致）
	require.NotPanics(t, func() { _ = cmdProcessRSS(context.Background(), models.SiteGroup("springsunday"), cfg, &ptStub{}) })
}

// 复制签名以使用 cmd.processRSS（避免 import cycle）
func cmdProcessRSS[T models.ResType](ctx context.Context, siteName models.SiteGroup, cfg models.RSSConfig, ptSite PTSiteInter[T]) error {
	if err := FetchAndDownloadFreeRSS(ctx, siteName, ptSite, cfg); err != nil {
		return err
	}
	if err := ptSite.SendTorrentToDownloader(ctx, cfg); err != nil {
		return err
	}
	return nil
}
