package internal

import (
	"context"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

type noopPT struct{}

func (n *noopPT) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	return &models.APIResponse[models.PHPTorrentInfo]{Code: "success", Data: models.PHPTorrentInfo{}}, nil
}
func (n *noopPT) IsEnabled() bool                                        { return true }
func (n *noopPT) DownloadTorrent(url, title, dir string) (string, error) { return "", nil }
func (n *noopPT) MaxRetries() int                                        { return 1 }
func (n *noopPT) RetryDelay() time.Duration                              { return 0 }
func (n *noopPT) SendTorrentToDownloader(ctx context.Context, rssCfg models.RSSConfig) error {
	return nil
}
func (n *noopPT) Context() context.Context { return context.Background() }
func TestProcessTorrentsWithDBUpdate_NoFail(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.InitLogger(zap.NewNop())
	global.GlobalDB = db
	// minimal call ensures not panic (details of processing covered elsewhere)
	require.NotPanics(t, func() {
		_ = ProcessTorrentsWithDBUpdate(context.Background(), nil, t.TempDir(), "cat", "tag", models.SpringSunday)
	})
}
