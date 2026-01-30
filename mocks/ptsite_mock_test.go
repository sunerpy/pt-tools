package mocks

import (
	"context"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/sunerpy/pt-tools/models"
)

type PTSiteMock[T models.ResType] struct {
	Enabled bool
	Detail  *models.APIResponse[T]
	DlHash  string
	DlErr   error
}

func (m *PTSiteMock[T]) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[T], error) {
	return m.Detail, nil
}
func (m *PTSiteMock[T]) IsEnabled() bool { return m.Enabled }
func (m *PTSiteMock[T]) DownloadTorrent(url, title, dir string) (string, error) {
	return m.DlHash, m.DlErr
}
func (m *PTSiteMock[T]) MaxRetries() int           { return 1 }
func (m *PTSiteMock[T]) RetryDelay() time.Duration { return 0 }
func (m *PTSiteMock[T]) SendTorrentToDownloader(ctx context.Context, rssCfg models.RSSConfig) error {
	return nil
}
func (m *PTSiteMock[T]) Context() context.Context { return context.Background() }
