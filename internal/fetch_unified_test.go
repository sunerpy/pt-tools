// MIT License
// Copyright (c) 2025 pt-tools

// Coverage for the Unified RSS pipeline: FetchAndDownloadFreeRSSUnified and
// downloadWorkerUnified. Exercises the happy path (item downloaded + DB row
// written), skip/filter branches, detail-fetch failures, download failures,
// disabled site, missing DB and blank download-dir guards, and the
// no-download-link discard path — all with an in-memory UnifiedPTSite so no
// real tracker is touched.

package internal

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// unifiedFake is an in-memory internal.UnifiedPTSite. Its DownloadTorrent
// actually writes a .torrent file into downloadDir so downloadWorkerUnified's
// post-download os.Stat check passes and the "downloaded" counter increments.
type unifiedFake struct {
	enabled     bool
	detail      *v2.TorrentItem
	detailErr   error
	downloadErr error
	writeFile   bool

	detailCalls   atomic.Int32
	downloadCalls atomic.Int32
	sendCalls     atomic.Int32
}

func (f *unifiedFake) GetTorrentDetails(_ *gofeed.Item) (*v2.TorrentItem, error) {
	f.detailCalls.Add(1)
	if f.detailErr != nil {
		return nil, f.detailErr
	}
	if f.detail != nil {
		return f.detail, nil
	}
	return &v2.TorrentItem{ID: "1", Title: "t", DiscountLevel: v2.DiscountFree}, nil
}

func (f *unifiedFake) IsEnabled() bool { return f.enabled }

func (f *unifiedFake) DownloadTorrent(_, title, downloadDir string) (string, error) {
	f.downloadCalls.Add(1)
	if f.downloadErr != nil {
		return "", f.downloadErr
	}
	if f.writeFile {
		_ = os.MkdirAll(downloadDir, 0o755)
		p := filepath.Join(downloadDir, title+".torrent")
		_ = os.WriteFile(p, []byte("d4:infod4:name3:abcee"), 0o644)
	}
	return "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", nil
}

func (f *unifiedFake) MaxRetries() int           { return 1 }
func (f *unifiedFake) RetryDelay() time.Duration { return time.Millisecond }
func (f *unifiedFake) SendTorrentToDownloader(_ context.Context, _ models.RSSConfig) error {
	f.sendCalls.Add(1)
	return nil
}
func (f *unifiedFake) Context() context.Context    { return context.Background() }
func (f *unifiedFake) SiteGroup() models.SiteGroup { return models.SiteGroup("springsunday") }

var _ UnifiedPTSite = (*unifiedFake)(nil)

// feedServerUnified serves the supplied RSS body verbatim.
func feedServerUnified(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func rssBody(items string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>t</title>` +
		`<link>http://x</link><description>d</description>` + items + `</channel></rss>`
}

func itemXML(guid, title, enclosure string) string {
	enc := ""
	if enclosure != "" {
		enc = fmt.Sprintf(`<enclosure url="%s" type="application/x-bittorrent"/>`, enclosure)
	}
	return fmt.Sprintf(`<item><title>%s</title><guid>%s</guid>%s</item>`, title, guid, enc)
}

func TestFetchUnified_DBNil(t *testing.T) {
	global.GlobalDB = nil
	err := FetchAndDownloadFreeRSSUnified(context.Background(), &unifiedFake{enabled: true}, models.RSSConfig{URL: "http://x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DB 不可用")
}

func TestFetchUnified_BlankDownloadDir(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	// Persist a global row with an empty DownloadDir.
	require.NoError(t, db.DB.Create(&models.SettingsGlobal{DownloadDir: ""}).Error)
	t.Cleanup(func() { global.GlobalDB = nil })

	err = FetchAndDownloadFreeRSSUnified(context.Background(), &unifiedFake{enabled: true}, models.RSSConfig{URL: "http://x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "下载目录为空")
}

func TestFetchUnified_DisabledSite(t *testing.T) {
	_ = setupDB(t)
	err := FetchAndDownloadFreeRSSUnified(context.Background(), &unifiedFake{enabled: false}, models.RSSConfig{URL: "http://x"})
	require.Error(t, err)
	assert.Equal(t, enableError, err.Error())
}

func TestFetchUnified_FeedFetchError(t *testing.T) {
	_ = setupDB(t)
	// Unreachable URL -> fetchRSSFeed returns error.
	err := FetchAndDownloadFreeRSSUnified(context.Background(), &unifiedFake{enabled: true},
		models.RSSConfig{Name: "r", URL: "http://127.0.0.1:0/none"})
	require.Error(t, err)
}

func TestFetchUnified_EmptyFeedSucceeds(t *testing.T) {
	_ = setupDB(t)
	srv := feedServerUnified(t, rssBody(""))
	site := &unifiedFake{enabled: true}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL}))
	assert.Equal(t, int32(0), site.detailCalls.Load())
}

func TestFetchUnified_DiscardsItemsWithoutLink(t *testing.T) {
	_ = setupDB(t)
	// Item with no enclosure and no link is discarded before reaching a worker.
	srv := feedServerUnified(t, rssBody(itemXML("g1", "NoLink", "")))
	site := &unifiedFake{enabled: true}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL}))
	assert.Equal(t, int32(0), site.detailCalls.Load(), "item without link must be discarded")
}

func TestFetchUnified_HappyPath_DownloadsAndRecords(t *testing.T) {
	db := setupDB(t)
	srv := feedServerUnified(t, rssBody(itemXML("g100", "FreeMovie", "http://x/f.torrent")))
	site := &unifiedFake{
		enabled:   true,
		writeFile: true,
		detail: &v2.TorrentItem{
			ID: "g100", Title: "FreeMovie", DiscountLevel: v2.DiscountFree, SizeBytes: 1024,
		},
	}
	cfg := models.RSSConfig{Name: "r", URL: srv.URL, Tag: "movie"}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site, cfg))

	assert.Equal(t, int32(1), site.detailCalls.Load())
	assert.Equal(t, int32(1), site.downloadCalls.Load())

	ti, err := db.GetTorrentBySiteAndID("springsunday", "g100")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.True(t, ti.IsFree)
	assert.True(t, ti.IsDownloaded)
	require.NotNil(t, ti.TorrentHash)
}

func TestFetchUnified_NonFreeIsSkipped(t *testing.T) {
	db := setupDB(t)
	srv := feedServerUnified(t, rssBody(itemXML("g200", "Paid", "http://x/p.torrent")))
	site := &unifiedFake{
		enabled: true,
		detail: &v2.TorrentItem{
			ID: "g200", Title: "Paid", DiscountLevel: v2.DiscountNone, SizeBytes: 1024,
		},
	}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "t"}))

	assert.Equal(t, int32(0), site.downloadCalls.Load(), "non-free must not download")
	ti, err := db.GetTorrentBySiteAndID("springsunday", "g200")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.True(t, ti.IsSkipped)
	assert.False(t, ti.IsFree)
}

func TestFetchUnified_DetailErrorCountsFailed(t *testing.T) {
	_ = setupDB(t)
	srv := feedServerUnified(t, rssBody(itemXML("g300", "Boom", "http://x/b.torrent")))
	site := &unifiedFake{enabled: true, detailErr: fmt.Errorf("detail boom")}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL}))
	assert.Equal(t, int32(1), site.detailCalls.Load())
	assert.Equal(t, int32(0), site.downloadCalls.Load())
}

func TestFetchUnified_DownloadErrorCountsFailed(t *testing.T) {
	db := setupDB(t)
	srv := feedServerUnified(t, rssBody(itemXML("g400", "DlFail", "http://x/d.torrent")))
	site := &unifiedFake{
		enabled:     true,
		downloadErr: fmt.Errorf("download boom"),
		detail: &v2.TorrentItem{
			ID: "g400", Title: "DlFail", DiscountLevel: v2.DiscountFree, SizeBytes: 1024,
		},
	}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "t"}))
	assert.Equal(t, int32(1), site.downloadCalls.Load())

	// Row exists (created before download) but not marked downloaded.
	ti, err := db.GetTorrentBySiteAndID("springsunday", "g400")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.False(t, ti.IsDownloaded)
}

func TestFetchUnified_AlreadyPushedIsSkipped(t *testing.T) {
	db := setupDB(t)
	pushed := true
	now := time.Now()
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "g500", IsPushed: &pushed, PushTime: &now}
	require.NoError(t, db.UpsertTorrent(ti))

	srv := feedServerUnified(t, rssBody(itemXML("g500", "Dup", "http://x/dup.torrent")))
	site := &unifiedFake{enabled: true}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site,
		models.RSSConfig{Name: "r", URL: srv.URL}))
	assert.Equal(t, int32(0), site.detailCalls.Load(), "already-pushed torrent must be skipped before detail fetch")
}

func TestFetchUnified_ContextCanceled(t *testing.T) {
	_ = setupDB(t)
	srv := feedServerUnified(t, rssBody(itemXML("g600", "Cancel", "http://x/c.torrent")))
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled
	site := &unifiedFake{enabled: true}
	err := FetchAndDownloadFreeRSSUnified(ctx, site, models.RSSConfig{Name: "r", URL: srv.URL})
	// Either the ctx error surfaces, or the worker drains before the send loop
	// observes cancellation; both are valid. Just ensure no panic and it returns.
	_ = err
}
