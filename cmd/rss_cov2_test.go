package cmd

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// TestProcessRSSUnified_SendError drives processRSSUnified's send-error return
// branch: the RSS fetch succeeds (empty feed) but SendTorrentToDownloader fails.
func TestProcessRSSUnified_SendError(t *testing.T) {
	seedGlobalWithDownloadDir(t)
	srv := emptyFeedServer(t)
	site := &fakeUnifiedSite{enabled: true, sendErr: errors.New("push failed")}
	cfg := models.RSSConfig{Name: "job", URL: srv.URL}
	err := processRSSUnified(context.Background(), cfg, site)
	require.Error(t, err)
	assert.Equal(t, 1, site.sendCalls())
}

// sendErrSite is a deprecated-generic PTSiteInter stub whose downloader push
// always fails, used to cover processRSS's send-error branch.
type sendErrSite struct{}

func (s *sendErrSite) GetTorrentDetails(_ *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	return &models.APIResponse[models.PHPTorrentInfo]{Code: "success", Data: models.PHPTorrentInfo{Title: "x", TorrentID: "id", Discount: models.DISCOUNT_FREE, EndTime: time.Now().Add(time.Hour), SizeMB: 1}}, nil
}
func (s *sendErrSite) IsEnabled() bool                                { return true }
func (s *sendErrSite) DownloadTorrent(_, _, _ string) (string, error) { return "h", nil }
func (s *sendErrSite) MaxRetries() int                                { return 1 }
func (s *sendErrSite) RetryDelay() time.Duration                      { return 0 }
func (s *sendErrSite) SendTorrentToDownloader(_ context.Context, _ models.RSSConfig) error {
	return errors.New("push boom")
}
func (s *sendErrSite) Context() context.Context { return context.Background() }

// TestProcessRSS_SendErrorBranch drives the deprecated processRSS send-error
// return path using an empty feed so fetch succeeds and only the push fails.
func TestProcessRSS_SendErrorBranch(t *testing.T) {
	seedGlobalWithDownloadDir(t)
	srv := emptyFeedServer(t)
	err := processRSS(context.Background(), models.SiteGroup("springsunday"),
		models.RSSConfig{Name: "r", URL: srv.URL}, &sendErrSite{})
	require.Error(t, err)
}

// okSite is a deprecated-generic PTSiteInter stub whose push succeeds, used to
// cover executeTask's success-log branch.
type okSite struct{}

func (s *okSite) GetTorrentDetails(_ *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	return &models.APIResponse[models.PHPTorrentInfo]{Code: "success", Data: models.PHPTorrentInfo{Title: "x", TorrentID: "id", Discount: models.DISCOUNT_FREE, EndTime: time.Now().Add(time.Hour), SizeMB: 1}}, nil
}
func (s *okSite) IsEnabled() bool                                { return true }
func (s *okSite) DownloadTorrent(_, _, _ string) (string, error) { return "h", nil }
func (s *okSite) MaxRetries() int                                { return 1 }
func (s *okSite) RetryDelay() time.Duration                      { return 0 }
func (s *okSite) SendTorrentToDownloader(_ context.Context, _ models.RSSConfig) error {
	return nil
}
func (s *okSite) Context() context.Context { return context.Background() }

// TestExecuteTask_SuccessBranch drives the deprecated executeTask success-log
// branch via an empty feed + succeeding push.
func TestExecuteTask_SuccessBranch(t *testing.T) {
	global.InitLogger(zap.NewNop())
	seedGlobalWithDownloadDir(t)
	srv := emptyFeedServer(t)
	executeTask(context.Background(), models.SiteGroup("springsunday"),
		models.RSSConfig{Name: "r", URL: srv.URL}, &okSite{})
}
