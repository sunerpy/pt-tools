// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// legacyPTStub is a PTSiteInter[PHPTorrentInfo] that returns a configurable
// detail and optionally writes a .torrent file so the legacy downloadWorker's
// happy path (download + DB update) runs to completion.
type legacyPTStub struct {
	enabled   bool
	discount  models.DiscountType
	sizeMB    float64
	writeFile bool
	detailErr error
	dlErr     error
	dlCalls   int
}

func (p *legacyPTStub) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	if p.detailErr != nil {
		return nil, p.detailErr
	}
	return &models.APIResponse[models.PHPTorrentInfo]{
		Code: "success",
		Data: models.PHPTorrentInfo{
			Title: item.Title, TorrentID: item.GUID,
			Discount: p.discount, SizeMB: p.sizeMB, EndTime: time.Now().Add(2 * time.Hour),
		},
	}, nil
}
func (p *legacyPTStub) IsEnabled() bool { return p.enabled }
func (p *legacyPTStub) DownloadTorrent(_, title, dir string) (string, error) {
	p.dlCalls++
	if p.dlErr != nil {
		return "", p.dlErr
	}
	if p.writeFile {
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, title+".torrent"), []byte("d4:infod4:name1:aee"), 0o644)
	}
	return "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", nil
}
func (p *legacyPTStub) MaxRetries() int           { return 1 }
func (p *legacyPTStub) RetryDelay() time.Duration { return 0 }
func (p *legacyPTStub) SendTorrentToDownloader(_ context.Context, _ models.RSSConfig) error {
	return nil
}
func (p *legacyPTStub) Context() context.Context { return context.Background() }

func TestFetchAndDownloadFreeRSS_HappyPath(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	srv := feedServerUnified(t, rssBody(itemXML("lg1", "LegacyFree", "http://x/l.torrent")))
	site := &legacyPTStub{enabled: true, discount: models.DISCOUNT_FREE, sizeMB: 1, writeFile: true}
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "movie"}))
	assert.Equal(t, 1, site.dlCalls)

	ti, err := db.GetTorrentBySiteAndID("springsunday", "lg1")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.True(t, ti.IsFree)
	assert.True(t, ti.IsDownloaded)
}

func TestFetchAndDownloadFreeRSS_NonFreeSkipped(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	srv := feedServerUnified(t, rssBody(itemXML("lg2", "LegacyPaid", "http://x/p.torrent")))
	site := &legacyPTStub{enabled: true, discount: models.DISCOUNT_NONE, sizeMB: 1}
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "t"}))
	assert.Equal(t, 0, site.dlCalls)
	ti, err := db.GetTorrentBySiteAndID("springsunday", "lg2")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.True(t, ti.IsSkipped)
}

func TestFetchAndDownloadFreeRSS_DetailError(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	srv := feedServerUnified(t, rssBody(itemXML("lg3", "Boom", "http://x/b.torrent")))
	site := &legacyPTStub{enabled: true, detailErr: assertDLErr}
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL}))
	assert.Equal(t, 0, site.dlCalls)
}

func TestFetchAndDownloadFreeRSS_DownloadError(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	srv := feedServerUnified(t, rssBody(itemXML("lg4", "DlFail", "http://x/d.torrent")))
	site := &legacyPTStub{enabled: true, discount: models.DISCOUNT_FREE, sizeMB: 1, dlErr: assertDLErr}
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "t"}))
	assert.Equal(t, 1, site.dlCalls)
	ti, err := db.GetTorrentBySiteAndID("springsunday", "lg4")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.False(t, ti.IsDownloaded)
}

func TestFetchAndDownloadFreeRSS_DisabledSite(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	err := FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"),
		&legacyPTStub{enabled: false}, models.RSSConfig{URL: "http://x"})
	require.Error(t, err)
}

func TestFetchAndDownloadFreeRSS_DBNil(t *testing.T) {
	global.GlobalDB = nil
	err := FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"),
		&legacyPTStub{enabled: true}, models.RSSConfig{URL: "http://x"})
	require.Error(t, err)
}
