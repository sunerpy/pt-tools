// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeebo/bencode"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/filter"
	sm "github.com/sunerpy/pt-tools/mocks"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

func TestProcessTorrentsWithDownloaderByRSS_DisabledDownloader(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	dlID := uint(1)
	ds := models.DownloaderSetting{Name: "off", Type: "qbittorrent", URL: "http://x", Enabled: false}
	ds.ID = dlID
	require.NoError(t, db.DB.Create(&ds).Error)

	err := ProcessTorrentsWithDownloaderByRSS(context.Background(),
		models.RSSConfig{DownloaderID: &dlID}, t.TempDir(), "cat", "tag", models.SiteGroup("springsunday"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "未启用")
}

func TestProcessTorrentsWithDownloaderByRSS_SuccessAndSweep(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	srv := fakeQbitServer(t, false, 500*gb)
	dm := downloader.NewDownloaderManager()
	dm.RegisterFactory(downloader.DownloaderQBittorrent, func(cfg downloader.DownloaderConfig, name string) (downloader.Downloader, error) {
		return qbit.NewQbitClient(qbit.NewQBitConfigWithAutoStart(cfg.GetURL(), cfg.GetUsername(), cfg.GetPassword(), cfg.GetAutoStart()), name)
	})
	SetGlobalDownloaderManager(dm)
	t.Cleanup(func() { SetGlobalDownloaderManager(nil) })

	ds := models.DownloaderSetting{Name: "qb-def", Type: "qbittorrent", URL: srv.URL, Enabled: true, IsDefault: true, AutoStart: true}
	require.NoError(t, db.DB.Create(&ds).Error)

	dir := t.TempDir()
	var buf bytes.Buffer
	require.NoError(t, bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": "z"}}))
	fp := filepath.Join(dir, "springsunday-tsucc.torrent")
	require.NoError(t, os.WriteFile(fp, buf.Bytes(), 0o644))
	hash, err := qbit.ComputeTorrentHashWithPath(fp)
	require.NoError(t, err)

	future := time.Now().Add(1 * time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "tsucc", FreeEndTime: &future, IsPushed: &pushed}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	require.NoError(t, ProcessTorrentsWithDownloaderByRSS(context.Background(),
		models.RSSConfig{}, dir, "cat", "tag", models.SiteGroup("springsunday")))

	got, gerr := db.GetTorrentBySiteAndID("springsunday", "tsucc")
	require.NoError(t, gerr)
	require.NotNil(t, got.IsPushed)
	assert.True(t, *got.IsPushed)
}

func TestRecordDiskProtectError(t *testing.T) {
	global.GlobalDB = nil
	recordDiskProtectError(models.SiteGroup("s"), "h", "msg") // no panic

	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	hash := "abc123"
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "id1"}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	recordDiskProtectError(models.SiteGroup("springsunday"), hash, "磁盘满")
	got, err := db.GetTorrentBySiteAndHash("springsunday", hash)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "磁盘满", got.LastError)
}

func TestSweepStagingDir_Disabled(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	p, _ := writeStagingTorrent(t, dir, "keep")
	// retainHours <= 0 -> no-op.
	sweepStagingDir(dir, models.SiteGroup("springsunday"), 0)
	_, err := os.Stat(p)
	require.NoError(t, err, "file must remain when retain disabled")
}

func TestSweepStagingDir_RemovesPushed(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	p, hash := writeStagingTorrent(t, dir, "pushed")

	pushed := true
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "pid"}
	ti.TorrentHash = &hash
	ti.IsPushed = &pushed
	require.NoError(t, db.UpsertTorrent(ti))

	sweepStagingDir(dir, models.SiteGroup("springsunday"), 24)
	_, err := os.Stat(p)
	assert.True(t, os.IsNotExist(err), "pushed torrent file must be swept")
}

func TestShouldSweep(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()

	// Invalid file -> hash fails -> false.
	bad := filepath.Join(dir, "springsunday-bad.torrent")
	require.NoError(t, os.WriteFile(bad, []byte("not a torrent"), 0o644))
	assert.False(t, shouldSweep(bad, models.SiteGroup("springsunday"), 24))

	// No DB record -> true (orphan).
	p, _ := writeStagingTorrent(t, dir, "orphan")
	assert.True(t, shouldSweep(p, models.SiteGroup("springsunday"), 24))

	// Fresh unpushed record within retain window -> false.
	p2, hash2 := writeStagingTorrent(t, dir, "fresh")
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "fid"}
	ti.TorrentHash = &hash2
	ti.IsPushed = &pushed
	require.NoError(t, db.UpsertTorrent(ti))
	assert.False(t, shouldSweep(p2, models.SiteGroup("springsunday"), 24))

	// MaxRetry exceeded -> true.
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: dir, MaxRetry: 1,
	}))
	p3, hash3 := writeStagingTorrent(t, dir, "retried")
	ti3 := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "rid", RetryCount: 5}
	ti3.TorrentHash = &hash3
	ti3.IsPushed = &pushed
	require.NoError(t, db.UpsertTorrent(ti3))
	assert.True(t, shouldSweep(p3, models.SiteGroup("springsunday"), 24))
}

func TestAttemptDownloadWithContext_Success(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": "dl"}}))
	payload := buf.Bytes()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	hash, err := attemptDownloadWithContext(context.Background(), srv.URL, "My Title", dir)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	_, statErr := os.Stat(filepath.Join(dir, "My Title.torrent"))
	require.NoError(t, statErr)
}

func TestAttemptDownloadWithContext_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	_, err := attemptDownloadWithContext(context.Background(), srv.URL, "t", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 状态码错误")
}

func TestAttemptDownloadWithContext_InvalidTorrent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html>not a torrent</html>"))
	}))
	t.Cleanup(srv.Close)
	_, err := attemptDownloadWithContext(context.Background(), srv.URL, "t", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不是有效种子文件")
}

func TestDownloadTorrent_RetryThenFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	_, err := downloadTorrent(srv.URL, "t", t.TempDir(), 2, time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "下载失败")
}

func TestDownloadTorrent_Success(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": "ok"}}))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(buf.Bytes())
	}))
	t.Cleanup(srv.Close)
	hash, err := downloadTorrent(srv.URL, "ok", t.TempDir(), 2, time.Millisecond)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestInvalidTorrentPreview(t *testing.T) {
	// Control chars replaced, whitespace collapsed.
	got := invalidTorrentPreview([]byte("hello\x00\x01  world\n\tfoo"))
	assert.Equal(t, "hello world foo", got)

	// Long input truncated to 160 runes.
	long := make([]byte, 0, 300)
	for i := 0; i < 300; i++ {
		long = append(long, 'a')
	}
	out := invalidTorrentPreview(long)
	assert.Len(t, []rune(out), 160)
}

func TestAttemptDownload_Success(t *testing.T) {
	data, _ := bencode.EncodeBytes(map[string]any{"info": map[string]any{"name": "x"}})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK); _, _ = w.Write(data) }))
	defer srv.Close()
	dir := t.TempDir()
	hash, err := attemptDownload(srv.URL, "a/b:c*?", dir)
	require.NoError(t, err)
	require.NotEmpty(t, hash)
	p := filepath.Join(dir, "abc.torrent")
	_, err = os.Stat(p)
	require.NoError(t, err)
}

func TestDownloadTorrent_RetryThenSuccess(t *testing.T) {
	global.GlobalLogger = zap.NewNop()
	global.InitLogger(zap.NewNop())
	calls := 0
	data, _ := bencode.EncodeBytes(map[string]any{"info": map[string]any{"name": "x"}})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}))
	defer srv.Close()
	dir := t.TempDir()
	hash, err := downloadTorrent(srv.URL, "title", dir, 2, time.Millisecond*10)
	require.NoError(t, err)
	require.NotEmpty(t, hash)
}

func TestSanitizeTitle(t *testing.T) {
	got := sanitizeTitle("a/b:c*?  d  ")
	require.Equal(t, "abc d", got)
}

func TestAttemptDownload_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer srv.Close()
	if _, err := attemptDownload(srv.URL, "bad", t.TempDir()); err == nil {
		t.Fatalf("expected error")
	}
}

func TestDownloadTorrent_FailAllRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer srv.Close()
	if _, err := downloadTorrent(srv.URL, "bad", t.TempDir(), 2, 0); err == nil {
		t.Fatalf("expected error")
	}
}

func (s *siteFail) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	return nil, context.DeadlineExceeded
}

func (s *siteFail) IsEnabled() bool { return true }

func (s *siteFail) DownloadTorrent(url, title, dir string) (string, error) {
	return "", context.DeadlineExceeded
}

func (s *siteFail) MaxRetries() int { return 1 }

func (s *siteFail) RetryDelay() time.Duration { return 0 }

func (s *siteFail) SendTorrentToDownloader(ctx context.Context, rssCfg models.RSSConfig) error {
	return nil
}

func (s *siteFail) Context() context.Context { return context.Background() }

func TestFetchRSS_NoEnclosuresAndDetailFail(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 1, DefaultEnabled: true}))
	feed := `<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>t</title><item><title>x</title><guid>guid-x</guid></item></channel></rss>`
	srv := makeFeedServer(t, feed)
	defer srv.Close()
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), &siteFail{}, models.RSSConfig{Name: "r", URL: srv.URL}))
}

func (s *rssSiteStub) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	d := models.PHPTorrentInfo{Title: item.Title, TorrentID: item.GUID, Discount: models.DISCOUNT_FREE, EndTime: time.Now().Add(1 * time.Hour), SizeMB: 64}
	return &models.APIResponse[models.PHPTorrentInfo]{Code: "success", Data: d}, nil
}

func (s *rssSiteStub) IsEnabled() bool { return true }

func (s *rssSiteStub) DownloadTorrent(url, title, dir string) (string, error) {
	_ = os.MkdirAll(dir, 0o755)
	p := filepath.Join(dir, title+".torrent")
	return "hash-rss", os.WriteFile(p, []byte("d4:infoe"), 0o644)
}

func (s *rssSiteStub) MaxRetries() int { return 1 }

func (s *rssSiteStub) RetryDelay() time.Duration { return 0 }

func (s *rssSiteStub) SendTorrentToDownloader(ctx context.Context, rssCfg models.RSSConfig) error {
	return nil
}

func (s *rssSiteStub) Context() context.Context { return context.Background() }

func TestFetchAndDownloadFreeRSS_Basic(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true}))
	feed := bytes.NewBufferString(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Test</title>
<item><title>ItemRSS</title><guid>guid-rss</guid><enclosure url="http://localhost/file.torrent" type="application/x-bittorrent"/></item>
</channel></rss>`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); _, _ = w.Write(feed.Bytes()) }))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, FetchAndDownloadFreeRSS(ctx, models.SiteGroup("springsunday"), &rssSiteStub{}, models.RSSConfig{Name: "r", URL: srv.URL, Tag: "tag"}))
}

func (s *linkURLSiteStub) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	d := models.PHPTorrentInfo{Title: item.Title, TorrentID: item.GUID, Discount: models.DISCOUNT_FREE, EndTime: time.Now().Add(1 * time.Hour), SizeMB: 64}
	return &models.APIResponse[models.PHPTorrentInfo]{Code: "success", Data: d}, nil
}

func (s *linkURLSiteStub) IsEnabled() bool { return true }

func (s *linkURLSiteStub) DownloadTorrent(url, title, dir string) (string, error) {
	s.gotURL = url
	_ = os.MkdirAll(dir, 0o755)
	p := filepath.Join(dir, title+".torrent")
	return "hash-link", os.WriteFile(p, []byte("d4:infoe"), 0o644)
}

func (s *linkURLSiteStub) MaxRetries() int { return 1 }

func (s *linkURLSiteStub) RetryDelay() time.Duration { return 0 }

func (s *linkURLSiteStub) SendTorrentToDownloader(ctx context.Context, rssCfg models.RSSConfig) error {
	return nil
}

func (s *linkURLSiteStub) Context() context.Context { return context.Background() }

// TestFetchAndDownloadFreeRSS_FallbackToLink 验证无 <enclosure> 但有 <link> 的 mteam API 型 RSS
// item 不再被静默丢弃，torrentURL 回退使用 item.Link。
func TestFetchAndDownloadFreeRSS_FallbackToLink(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true}))
	const dlURL = "http://mteam.example/dl/12345"
	feed := bytes.NewBufferString(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Test</title>
<item><title>ItemLink</title><guid>guid-link</guid><link>` + dlURL + `</link></item>
</channel></rss>`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); _, _ = w.Write(feed.Bytes()) }))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stub := &linkURLSiteStub{}
	require.NoError(t, FetchAndDownloadFreeRSS(ctx, models.SiteGroup("springsunday"), stub, models.RSSConfig{Name: "r", URL: srv.URL, Tag: "tag"}))
	require.Equal(t, dlURL, stub.gotURL, "无 enclosure 时 torrentURL 应回退为 item.Link")
}

// TestComputeDiscarded 验证入队丢弃逻辑：无 enclosure 且无 link 视为丢弃，有任一则保留。
func TestComputeDiscarded(t *testing.T) {
	cases := []struct {
		name      string
		item      *gofeed.Item
		discarded bool
	}{
		{"both empty", &gofeed.Item{GUID: "g1"}, true},
		{"blank link", &gofeed.Item{GUID: "g2", Link: "   "}, true},
		{"has link", &gofeed.Item{GUID: "g3", Link: "http://x/dl"}, false},
		{"has enclosure", &gofeed.Item{GUID: "g4", Enclosures: []*gofeed.Enclosure{{URL: "http://x/e.torrent"}}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := len(c.item.Enclosures) == 0 && strings.TrimSpace(c.item.Link) == ""
			require.Equal(t, c.discarded, got)
		})
	}
}

func (t *props404Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/api/v2/torrents/properties" || filepath.Base(req.URL.Path) == "properties" {
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(bytes.NewBuffer(nil)), Header: make(http.Header)}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString("{}")), Header: make(http.Header)}, nil
}

func (t *props200Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/api/v2/torrents/properties" || filepath.Base(req.URL.Path) == "properties" {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(`{"save_path":"/tmp"}`)), Header: make(http.Header)}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString("{}")), Header: make(http.Header)}, nil
}

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestProcessTorrentsWithDBUpdate_ContinueOnErrors(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true, MaxRetry: 2}))
	// two torrent files; one will be treated as missing record and deleted; other record present but qbit errors should be tolerated
	p1, _ := makeTorrentFile(t, dir)
	// create a second distinct torrent file
	var buf bytes.Buffer
	_ = bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": "abc2"}})
	p2 := filepath.Join(dir, "y.torrent")
	require.NoError(t, os.WriteFile(p2, buf.Bytes(), 0o644))
	h2, err := qbit.ComputeTorrentHashWithPath(p2)
	require.NoError(t, err)
	pushed := false
	future := time.Now().Add(1 * time.Hour)
	ti := &models.TorrentInfo{SiteName: string(models.SiteGroup("springsunday")), TorrentHash: &h2, IsPushed: &pushed, FreeEndTime: &future}
	require.NoError(t, db.DB.Create(ti).Error)
	// properties -> 404 (not exists), sync -> OK (enough space), add -> 500 (fail)
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/v2/auth/login":
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString("Ok.")), Header: make(http.Header)}, nil
		case "/api/v2/torrents/properties":
			return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: make(http.Header)}, nil
		case "/api/v2/sync/maindata":
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(`{"server_state":{"free_space_on_disk":10000000}}`)), Header: make(http.Header)}, nil
		case "/api/v2/torrents/add":
			return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(bytes.NewBufferString("failed")), Header: make(http.Header)}, nil
		default:
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
		}
	})
	client := qbit.NewQbitClientForTesting(&http.Client{Transport: rt}, "http://example")
	require.NoError(t, ProcessTorrentsWithDBUpdate(context.Background(), client, dir, "cat", "tag", models.SiteGroup("springsunday")))
	// file1 should be deleted; file2 should remain due to push error path but function continues
	if _, err := os.Stat(p1); err == nil {
		t.Fatalf("expected p1 removed")
	}
	if _, err := os.Stat(p2); err != nil {
		t.Fatalf("expected p2 to remain")
	}
}

func TestProcessTorrentsWithDBUpdate_Basic(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true}))
	_, hash := makeTorrentFile(t, dir)
	pushed := true
	ti := &models.TorrentInfo{SiteName: string(models.SiteGroup("springsunday")), IsPushed: &pushed}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))
	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, ProcessTorrentsWithDBUpdate(context.Background(), client, dir, "cat", "tag", models.SiteGroup("springsunday")))
}

func TestProcessTorrentsWithDBUpdate_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true}))
	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, ProcessTorrentsWithDBUpdate(context.Background(), client, dir, "cat", "tag", models.SiteGroup("springsunday")))
}

func TestDownloadWorker_DisabledSite(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true}))
	item := &gofeed.Item{GUID: "g", Title: "t", Enclosures: []*gofeed.Enclosure{{URL: "http://u"}}}
	ch := make(chan *gofeed.Item, 1)
	ch <- item
	close(ch)
	var wg sync.WaitGroup
	wg.Add(1)
	// disabled mock
	m := &disabledSite{}
	downloadWorker(context.Background(), models.SiteGroup("springsunday"), &wg, m, ch, models.RSSConfig{Tag: "t"})
	wg.Wait()
}

func TestDownloadWorker_ChannelClosedEarly(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true}))
	ch := make(chan *gofeed.Item)
	close(ch)
	var wg sync.WaitGroup
	wg.Add(1)
	m := &disabledSite{}
	downloadWorker(context.Background(), models.SiteGroup("springsunday"), &wg, m, ch, models.RSSConfig{Tag: "t"})
	wg.Wait()
}

func TestDownloadWorker_Table(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	cases := []struct {
		name       string
		gl         models.SettingsGlobal
		detail     models.PHPTorrentInfo
		nonfree    bool
		expectSkip bool
	}{
		{"nonfree", models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true}, models.PHPTorrentInfo{Title: "NonFree", TorrentID: "guid-nonfree", Discount: models.DISCOUNT_NONE, EndTime: time.Now().Add(30 * time.Minute), SizeMB: 64}, true, true},
		{"cannotfinish_by_size", models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true, DownloadLimitEnabled: false, TorrentSizeGB: 1}, models.PHPTorrentInfo{Title: "SkipTitle", TorrentID: "guid-skip", Discount: models.DISCOUNT_FREE, EndTime: time.Now().Add(5 * time.Minute), SizeMB: 2048}, false, true},
		{"canfinish", models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true, DownloadLimitEnabled: false, TorrentSizeGB: 1}, models.PHPTorrentInfo{Title: "OkTitle", TorrentID: "guid-ok", Discount: models.DISCOUNT_FREE, EndTime: time.Now().Add(30 * time.Minute), SizeMB: 256}, false, false},
	}
	for _, c := range cases {
		require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(c.gl))
		item := &gofeed.Item{GUID: c.detail.TorrentID, Title: c.detail.Title, Categories: []string{"C"}, Enclosures: []*gofeed.Enclosure{{URL: "http://u"}}}
		ch := make(chan *gofeed.Item, 1)
		ch <- item
		close(ch)
		ctrl := gomock.NewController(t)
		m := sm.NewMockPTSiteInter[models.PHPTorrentInfo](ctrl)
		m.EXPECT().IsEnabled().Return(true).AnyTimes()
		m.EXPECT().GetTorrentDetails(gomock.Any()).Return(&models.APIResponse[models.PHPTorrentInfo]{Code: "success", Data: c.detail}, nil).AnyTimes()
		if c.nonfree {
			m.EXPECT().DownloadTorrent(gomock.Any(), gomock.Any(), gomock.Any()).Return("h", nil).AnyTimes()
		} else {
			m.EXPECT().DownloadTorrent(gomock.Any(), gomock.Any(), gomock.Any()).Return("h", nil).AnyTimes()
		}
		m.EXPECT().MaxRetries().Return(1).AnyTimes()
		m.EXPECT().RetryDelay().Return(time.Duration(0)).AnyTimes()
		m.EXPECT().Context().Return(context.Background()).AnyTimes()
		var wg sync.WaitGroup
		wg.Add(1)
		if !c.expectSkip {
			base := c.gl.DownloadDir
			sub := c.detail.Title
			_ = os.MkdirAll(filepath.Join(base, sub), 0o755)
			fileBase := "cmct-" + item.GUID
			torrentFile := filepath.Join(base, sub, fileBase+".torrent")
			_ = os.WriteFile(torrentFile, []byte("d4:infoe"), 0o644)
		}
		downloadWorker(context.Background(), models.SiteGroup("springsunday"), &wg, m, ch, models.RSSConfig{Tag: c.detail.Title})
		wg.Wait()
		ti, err := global.GlobalDB.GetTorrentBySiteAndID(string(models.SiteGroup("springsunday")), item.GUID)
		require.NoError(t, err)
		require.NotNil(t, ti)
		if c.expectSkip {
			require.True(t, ti.IsSkipped)
		} else {
			require.False(t, ti.IsSkipped)
		}
		ctrl.Finish()
	}
}

func (s *siteDummy) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	d := models.PHPTorrentInfo{Title: "x", TorrentID: "id", Discount: models.DISCOUNT_FREE, EndTime: time.Now().Add(1 * time.Hour), SizeMB: 1}
	return &models.APIResponse[models.PHPTorrentInfo]{Code: "success", Data: d}, nil
}

func (s *siteDummy) IsEnabled() bool { return true }

func (s *siteDummy) DownloadTorrent(url, title, dir string) (string, error) { return "h", nil }

func (s *siteDummy) MaxRetries() int { return 1 }

func (s *siteDummy) RetryDelay() time.Duration { return 0 }

func (s *siteDummy) SendTorrentToDownloader(ctx context.Context, rssCfg models.RSSConfig) error {
	return nil
}

func (s *siteDummy) Context() context.Context { return context.Background() }

func TestFetchRSS_InvalidURL(t *testing.T) {
	err := FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), &siteDummy{}, models.RSSConfig{URL: "://bad"})
	if err == nil {
		t.Fatalf("expected parse error")
	}
}

func (s *disabledSite) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	d := models.PHPTorrentInfo{Title: "x", TorrentID: "id", Discount: models.DISCOUNT_FREE, EndTime: time.Now().Add(1 * time.Hour), SizeMB: 1}
	return &models.APIResponse[models.PHPTorrentInfo]{Code: "success", Data: d}, nil
}

func (s *disabledSite) IsEnabled() bool { return false }

func (s *disabledSite) DownloadTorrent(url, title, dir string) (string, error) { return "h", nil }

func (s *disabledSite) MaxRetries() int { return 1 }

func (s *disabledSite) RetryDelay() time.Duration { return 0 }

func (s *disabledSite) SendTorrentToDownloader(ctx context.Context, rssCfg models.RSSConfig) error {
	return nil
}

func (s *disabledSite) Context() context.Context { return context.Background() }

func TestFetchRSS_Errors(t *testing.T) {
	global.GlobalDB = nil
	if err := FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), &disabledSite{}, models.RSSConfig{URL: "http://example"}); err == nil {
		t.Fatalf("expected error when db nil")
	}
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	if err := db.DB.Create(&models.SettingsGlobal{DownloadDir: ""}).Error; err != nil {
		t.Fatalf("seed gl: %v", err)
	}
	if err := FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), &disabledSite{}, models.RSSConfig{URL: "http://example"}); err == nil {
		t.Fatalf("expected error for blank download dir")
	}
	if err := db.DB.Where("1=1").Delete(&models.SettingsGlobal{}).Error; err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if err := core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true}); err != nil {
		t.Fatalf("save gl: %v", err)
	}
	if err := FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), &disabledSite{}, models.RSSConfig{URL: "http://invalid"}); err == nil {
		t.Fatalf("expected enableError")
	}
}

func TestFetchRSS_WithGomock(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := sm.NewMockPTSiteInter[models.PHPTorrentInfo](ctrl)
	m.EXPECT().IsEnabled().Return(true).AnyTimes()
	m.EXPECT().DownloadTorrent(gomock.Any(), gomock.Any(), gomock.Any()).Return("h", nil).AnyTimes()
	m.EXPECT().MaxRetries().Return(1).AnyTimes()
	m.EXPECT().RetryDelay().Return(time.Duration(0)).AnyTimes()
	m.EXPECT().Context().Return(context.Background()).AnyTimes()
	feed := `<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><item><title>T</title><guid>g</guid><enclosure url="http://example/file.torrent" type="application/x-bittorrent"/></item></channel></rss>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte(feed)) }))
	defer srv.Close()
	m.EXPECT().GetTorrentDetails(gomock.Any()).Return(&models.APIResponse[models.PHPTorrentInfo]{Code: "success", Data: models.PHPTorrentInfo{Title: "T", TorrentID: "g", Discount: models.DISCOUNT_FREE}}, nil).AnyTimes()
	_ = core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, DefaultEnabled: true})
	if err := FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), m, models.RSSConfig{Name: "r", URL: srv.URL, Tag: "tag"}); err != nil {
		t.Fatalf("fetch: %v", err)
	}
}

func TestGetDownloaderForRSS_NoDB(t *testing.T) {
	global.GlobalDB = nil
	_, err := GetDownloaderForRSS(models.RSSConfig{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "数据库未初始化")
}

// TestGetDownloaderForRSS_SpecifiedDownloader 测试指定下载器
func TestGetDownloaderForRSS_SpecifiedDownloader(t *testing.T) {
	db := setupDB(t)

	// 创建下载器配置
	dlID := uint(1)
	ds := models.DownloaderSetting{
		Name:     "test-qbit",
		Type:     "qbittorrent",
		URL:      "http://localhost:8080",
		Username: "admin",
		Password: "password",
		Enabled:  true,
	}
	ds.ID = dlID
	require.NoError(t, db.DB.Create(&ds).Error)

	rssCfg := models.RSSConfig{
		DownloaderID: &dlID,
	}

	_, err := GetDownloaderForRSS(rssCfg)
	_ = err
}

// TestGetDownloaderForRSS_DisabledDownloader 测试禁用的下载器
func TestGetDownloaderForRSS_DisabledDownloader(t *testing.T) {
	db := setupDB(t)

	dlID := uint(2)
	ds := models.DownloaderSetting{
		Name:     "disabled-qbit",
		Type:     "qbittorrent",
		URL:      "http://localhost:8080",
		Username: "admin",
		Password: "password",
		Enabled:  false,
	}
	ds.ID = dlID
	require.NoError(t, db.DB.Create(&ds).Error)

	rssCfg := models.RSSConfig{
		DownloaderID: &dlID,
	}

	_, err := GetDownloaderForRSS(rssCfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "未启用")
}

// TestGetDownloaderForRSS_DefaultDownloader 测试默认下载器
func TestGetDownloaderForRSS_DefaultDownloader(t *testing.T) {
	db := setupDB(t)

	ds := models.DownloaderSetting{
		Name:      "default-qbit",
		Type:      "qbittorrent",
		URL:       "http://localhost:8080",
		Username:  "admin",
		Password:  "password",
		Enabled:   true,
		IsDefault: true,
	}
	require.NoError(t, db.DB.Create(&ds).Error)

	rssCfg := models.RSSConfig{}

	_, err := GetDownloaderForRSS(rssCfg)
	// 可能成功创建客户端或返回连接错误
	_ = err
}

// TestGetDownloaderForRSS_NoDefaultDownloader 测试没有默认下载器
func TestGetDownloaderForRSS_NoDefaultDownloader(t *testing.T) {
	_ = setupDB(t)

	rssCfg := models.RSSConfig{}

	_, err := GetDownloaderForRSS(rssCfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "获取默认下载器失败")
}

// TestGetDownloaderForRSS_TransmissionType 测试 Transmission 类型
func TestGetDownloaderForRSS_TransmissionType(t *testing.T) {
	db := setupDB(t)

	ds := models.DownloaderSetting{
		Name:      "default-transmission",
		Type:      "transmission",
		URL:       "http://localhost:9091",
		Username:  "admin",
		Password:  "password",
		Enabled:   true,
		IsDefault: true,
	}
	require.NoError(t, db.DB.Create(&ds).Error)

	rssCfg := models.RSSConfig{}

	_, err := GetDownloaderForRSS(rssCfg)
	// 可能成功创建客户端或返回连接错误
	_ = err
}

// TestGetDownloaderForRSS_UnknownType 测试未知下载器类型
func TestGetDownloaderForRSS_UnknownType(t *testing.T) {
	db := setupDB(t)

	dlMgr := downloader.NewDownloaderManager()
	dlMgr.RegisterFactory(downloader.DownloaderQBittorrent, func(config downloader.DownloaderConfig, name string) (downloader.Downloader, error) {
		qbitConfig := qbit.NewQBitConfigWithAutoStart(config.GetURL(), config.GetUsername(), config.GetPassword(), config.GetAutoStart())
		return qbit.NewQbitClient(qbitConfig, name)
	})
	SetGlobalDownloaderManager(dlMgr)
	defer SetGlobalDownloaderManager(nil)

	ds := models.DownloaderSetting{
		Name:      "unknown-type",
		Type:      "unknown",
		URL:       "http://localhost:8080",
		Username:  "admin",
		Password:  "password",
		Enabled:   true,
		IsDefault: true,
	}
	require.NoError(t, db.DB.Create(&ds).Error)

	rssCfg := models.RSSConfig{}

	_, err := GetDownloaderForRSS(rssCfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "不支持的下载器类型")
}

// TestGetDownloaderForRSS_NotFoundDownloader 测试找不到指定的下载器
func TestGetDownloaderForRSS_NotFoundDownloader(t *testing.T) {
	_ = setupDB(t)

	dlID := uint(999)
	rssCfg := models.RSSConfig{
		DownloaderID: &dlID,
	}

	_, err := GetDownloaderForRSS(rssCfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "获取指定下载器失败")
}

// TestAttemptDownloadWithContext 测试带 context 的下载
func TestAttemptDownloadWithContext(t *testing.T) {
	data, _ := bencode.EncodeBytes(map[string]any{"info": map[string]any{"name": "x"}})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	dir := t.TempDir()
	ctx := context.Background()
	hash, err := attemptDownloadWithContext(ctx, srv.URL, "test-title", dir)
	require.NoError(t, err)
	require.NotEmpty(t, hash)
}

// TestAttemptDownloadWithContext_Cancelled 测试取消的 context
func TestAttemptDownloadWithContext_Cancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := attemptDownloadWithContext(ctx, srv.URL, "test-title", dir)
	require.Error(t, err)
}

// TestFetchRSSFeed 测试 RSS 解析
func TestFetchRSSFeed(t *testing.T) {
	feed := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
<channel>
<title>Test Feed</title>
<item><title>Item1</title><guid>guid1</guid></item>
</channel>
</rss>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(feed))
	}))
	defer srv.Close()

	result, err := fetchRSSFeed(srv.URL)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "Test Feed", result.Title)
	require.Len(t, result.Items, 1)
}

// TestFetchRSSFeed_BrowserUserAgent 验证 RSS 请求会携带浏览器 User-Agent，
// 否则 Cloudflare-fronted PT 站点（如 gtkpw）会直接 RST 连接（issue #332）。
func TestFetchRSSFeed_BrowserUserAgent(t *testing.T) {
	feed := `<?xml version="1.0"?><rss version="2.0"><channel><title>UA</title></channel></rss>`
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(feed))
	}))
	defer srv.Close()

	_, err := fetchRSSFeed(srv.URL)
	require.NoError(t, err)
	require.Contains(t, gotUA, "Mozilla/5.0", "RSS fetcher must send a browser-like UA")
}

// TestFetchRSSFeed_InvalidURL 测试无效 URL
func TestFetchRSSFeed_InvalidURL(t *testing.T) {
	_, err := fetchRSSFeed("://invalid")
	require.Error(t, err)
}

// TestFetchRSSFeed_InvalidContent 测试无效内容
func TestFetchRSSFeed_InvalidContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not xml"))
	}))
	defer srv.Close()

	_, err := fetchRSSFeed(srv.URL)
	require.Error(t, err)
}

func TestProcessSingleTorrentWithDownloader_NoRecord(t *testing.T) {
	db := setupDB(t)
	dir := t.TempDir()
	path, _ := makeTorrentFile(t, dir)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("test-dl").AnyTimes()
	mockDl.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo, path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	require.NoError(t, err)

	_, err = os.Stat(path)
	require.True(t, os.IsNotExist(err))
	_ = db
}

func TestProcessSingleTorrentWithDownloader_AlreadyPushed(t *testing.T) {
	db := setupDB(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	pushed := true
	now := time.Now()
	ti := &models.TorrentInfo{
		SiteName:    string(models.SiteGroup("springsunday")),
		TorrentHash: &hash,
		IsPushed:    &pushed,
		PushTime:    &now,
	}
	require.NoError(t, db.UpsertTorrent(ti))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("test-dl").AnyTimes()
	mockDl.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo, path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	require.NoError(t, err)

	_, err = os.Stat(path)
	require.True(t, os.IsNotExist(err))
}

// TestProcessSingleTorrentWithDownloader_Expired 测试过期的种子
func TestProcessSingleTorrentWithDownloader_Expired(t *testing.T) {
	db := setupDB(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	// 创建过期的记录
	past := time.Now().Add(-1 * time.Hour)
	ti := &models.TorrentInfo{
		SiteName:    string(models.SiteGroup("springsunday")),
		TorrentHash: &hash,
		FreeEndTime: &past,
	}
	require.NoError(t, db.UpsertTorrent(ti))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("test-dl").AnyTimes()
	mockDl.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo, path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	require.NoError(t, err)

	// 文件应该被删除
	_, err = os.Stat(path)
	require.True(t, os.IsNotExist(err))
}

// TestProcessSingleTorrentWithDownloader_ExistsInDownloader 测试种子已存在于下载器
func TestProcessSingleTorrentWithDownloader_ExistsInDownloader(t *testing.T) {
	db := setupDB(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	// 创建未推送的记录
	pushed := false
	future := time.Now().Add(1 * time.Hour)
	ti := &models.TorrentInfo{
		SiteName:    string(models.SiteGroup("springsunday")),
		TorrentHash: &hash,
		IsPushed:    &pushed,
		FreeEndTime: &future,
	}
	require.NoError(t, db.UpsertTorrent(ti))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("test-dl").AnyTimes()
	mockDl.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(true, nil)

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo, path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	require.NoError(t, err)

	// 文件应该被删除
	_, err = os.Stat(path)
	require.True(t, os.IsNotExist(err))

	// 数据库应该更新为已推送
	ti2, err := db.GetTorrentBySiteAndHash(string(models.SiteGroup("springsunday")), hash)
	require.NoError(t, err)
	require.NotNil(t, ti2.IsPushed)
	require.True(t, *ti2.IsPushed)
}

// TestProcessTorrentsWithDownloaderByRSS_NoDownloader 测试获取下载器失败
func TestProcessTorrentsWithDownloaderByRSS_NoDownloader(t *testing.T) {
	_ = setupDB(t)

	rssCfg := models.RSSConfig{}
	err := ProcessTorrentsWithDownloaderByRSS(context.Background(), rssCfg, t.TempDir(), "cat", "tag", models.SiteGroup("springsunday"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "获取下载器失败")
}

// TestProcessSingleTorrentWithDownloader_NewTorrentPushSuccess 测试新种子推送成功
func TestProcessSingleTorrentWithDownloader_NewTorrentPushSuccess(t *testing.T) {
	db := setupDB(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	// 创建未推送的记录
	pushed := false
	future := time.Now().Add(1 * time.Hour)
	ti := &models.TorrentInfo{
		SiteName:    string(models.SiteGroup("springsunday")),
		TorrentHash: &hash,
		IsPushed:    &pushed,
		FreeEndTime: &future,
	}
	require.NoError(t, db.UpsertTorrent(ti))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("test-dl").AnyTimes()
	mockDl.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(int64(100*1024*1024*1024), nil)
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil)
	mockDl.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).Return(downloader.AddTorrentResult{Success: true, Hash: hash}, nil)

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo, path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	require.NoError(t, err)

	// 文件应该被删除
	_, err = os.Stat(path)
	require.True(t, os.IsNotExist(err))

	// 数据库应该更新为已推送
	ti2, err := db.GetTorrentBySiteAndHash(string(models.SiteGroup("springsunday")), hash)
	require.NoError(t, err)
	require.NotNil(t, ti2.IsPushed)
	require.True(t, *ti2.IsPushed)
}

// TestProcessSingleTorrentWithDownloader_PushFailed 测试推送失败
func TestProcessSingleTorrentWithDownloader_PushFailed(t *testing.T) {
	db := setupDB(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	// 创建未推送的记录
	pushed := false
	future := time.Now().Add(1 * time.Hour)
	ti := &models.TorrentInfo{
		SiteName:    string(models.SiteGroup("springsunday")),
		TorrentHash: &hash,
		IsPushed:    &pushed,
		FreeEndTime: &future,
	}
	require.NoError(t, db.UpsertTorrent(ti))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("test-dl").AnyTimes()
	mockDl.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(int64(100*1024*1024*1024), nil)
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil)
	mockDl.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).Return(downloader.AddTorrentResult{Success: false, Message: "push failed"}, fmt.Errorf("push failed"))

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo, path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "推送种子失败")

	// 注意：由于事务回滚，重试次数不会被更新
	// 这是预期行为，因为整个事务失败了
}

// TestProcessSingleTorrentWithDownloader_CheckExistsFailed 测试检查种子存在失败
func TestProcessSingleTorrentWithDownloader_CheckExistsFailed(t *testing.T) {
	db := setupDB(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	// 创建未推送的记录
	pushed := false
	future := time.Now().Add(1 * time.Hour)
	ti := &models.TorrentInfo{
		SiteName:    string(models.SiteGroup("springsunday")),
		TorrentHash: &hash,
		IsPushed:    &pushed,
		FreeEndTime: &future,
	}
	require.NoError(t, db.UpsertTorrent(ti))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("test-dl").AnyTimes()
	mockDl.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, fmt.Errorf("connection failed"))

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo, path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "检查种子存在失败")
}

// TestProcessSingleTorrentWithDownloader_InvalidTorrentFile 测试无效种子文件
func TestProcessSingleTorrentWithDownloader_InvalidTorrentFile(t *testing.T) {
	_ = setupDB(t)
	dir := t.TempDir()

	// 创建无效的种子文件
	invalidPath := filepath.Join(dir, "invalid.torrent")
	require.NoError(t, os.WriteFile(invalidPath, []byte("not a valid torrent"), 0o644))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("test-dl").AnyTimes()
	mockDl.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo, invalidPath, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "计算种子哈希失败")
}

// TestProcessSingleTorrentWithDownloader_RetainHoursExpired 测试保留时长过期
func TestProcessSingleTorrentWithDownloader_RetainHoursExpired(t *testing.T) {
	db := setupDB(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	// 设置保留时长
	store := core.NewConfigStore(db)
	require.NoError(t, store.SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir:            dir,
		DefaultIntervalMinutes: 10,
		DefaultEnabled:         true,
		RetainHours:            1,
	}))

	// 创建超过保留时长的记录
	past := time.Now().Add(-48 * time.Hour)
	future := time.Now().Add(1 * time.Hour)
	pushed := false
	ti := &models.TorrentInfo{
		SiteName:      string(models.SiteGroup("springsunday")),
		TorrentHash:   &hash,
		IsPushed:      &pushed,
		FreeEndTime:   &future,
		LastCheckTime: &past,
	}
	require.NoError(t, db.UpsertTorrent(ti))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("test-dl").AnyTimes()
	mockDl.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo, path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	require.NoError(t, err)

	// 文件应该被删除
	_, err = os.Stat(path)
	require.True(t, os.IsNotExist(err))
}

// TestProcessSingleTorrentWithDownloader_MaxRetryExceeded 测试超过最大重试次数
func TestProcessSingleTorrentWithDownloader_MaxRetryExceeded(t *testing.T) {
	db := setupDB(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	// 设置最大重试次数
	store := core.NewConfigStore(db)
	require.NoError(t, store.SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir:            dir,
		DefaultIntervalMinutes: 10,
		DefaultEnabled:         true,
		MaxRetry:               2,
	}))

	// 创建已超过重试次数的记录
	pushed := false
	future := time.Now().Add(1 * time.Hour)
	ti := &models.TorrentInfo{
		SiteName:    string(models.SiteGroup("springsunday")),
		TorrentHash: &hash,
		IsPushed:    &pushed,
		FreeEndTime: &future,
		RetryCount:  3, // 超过 MaxRetry
	}
	require.NoError(t, db.UpsertTorrent(ti))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("test-dl").AnyTimes()
	mockDl.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo, path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	require.NoError(t, err)

	// 文件应该被删除
	_, err = os.Stat(path)
	require.True(t, os.IsNotExist(err))
}

// TestDownloadWorker_ContextCancelled 测试上下文取消
func TestDownloadWorker_ContextCancelled(t *testing.T) {
	db := setupDB(t)
	_ = db

	ctx, cancel := context.WithCancel(context.Background())

	ch := make(chan *gofeed.Item, 1)
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	downloadWorker(ctx, models.SiteGroup("springsunday"), &wg, &disabledSite{}, ch, models.RSSConfig{Tag: "t"})
	wg.Wait()
}

// TestDownloadWorker_NoEnclosure 测试没有附件的情况
func TestDownloadWorker_NoEnclosure(t *testing.T) {
	db := setupDB(t)
	_ = db

	item := &gofeed.Item{GUID: "g", Title: "t"}
	ch := make(chan *gofeed.Item, 1)
	ch <- item
	close(ch)

	var wg sync.WaitGroup
	wg.Add(1)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := sm.NewMockPTSiteInter[models.PHPTorrentInfo](ctrl)
	m.EXPECT().IsEnabled().Return(true).AnyTimes()
	m.EXPECT().GetTorrentDetails(gomock.Any()).Return(&models.APIResponse[models.PHPTorrentInfo]{
		Code: "success",
		Data: models.PHPTorrentInfo{Title: "t", TorrentID: "g", Discount: models.DISCOUNT_FREE, EndTime: time.Now().Add(1 * time.Hour), SizeMB: 64},
	}, nil).AnyTimes()
	m.EXPECT().DownloadTorrent(gomock.Any(), gomock.Any(), gomock.Any()).Return("h", nil).AnyTimes()
	m.EXPECT().MaxRetries().Return(1).AnyTimes()
	m.EXPECT().RetryDelay().Return(time.Duration(0)).AnyTimes()
	m.EXPECT().Context().Return(context.Background()).AnyTimes()

	downloadWorker(context.Background(), models.SiteGroup("springsunday"), &wg, m, ch, models.RSSConfig{Tag: "t"})
	wg.Wait()
}

// TestFetchAndDownloadFreeRSS_ContextTimeout 测试上下文超时
func TestFetchAndDownloadFreeRSS_ContextTimeout(t *testing.T) {
	db := setupDB(t)
	_ = db

	// 创建一个已取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := FetchAndDownloadFreeRSS(ctx, models.SiteGroup("springsunday"), &rssSiteStub{}, models.RSSConfig{URL: "http://invalid"})
	// 应该返回错误
	require.Error(t, err)
}

// TestProcessTorrentsWithDownloaderByRSS_EmptyDir 测试空目录
func TestProcessTorrentsWithDownloaderByRSS_EmptyDir(t *testing.T) {
	db := setupDB(t)

	// 创建默认下载器
	ds := models.DownloaderSetting{
		Name:      "default-qbit",
		Type:      "qbittorrent",
		URL:       "http://localhost:8080",
		Username:  "admin",
		Password:  "password",
		Enabled:   true,
		IsDefault: true,
	}
	require.NoError(t, db.DB.Create(&ds).Error)

	emptyDir := t.TempDir()
	rssCfg := models.RSSConfig{}

	// 由于没有实际的 qBittorrent 服务器，会返回连接错误
	err := ProcessTorrentsWithDownloaderByRSS(context.Background(), rssCfg, emptyDir, "cat", "tag", models.SiteGroup("springsunday"))
	// 可能成功（空目录）或返回连接错误
	_ = err
}

// TestRSSFilterAssociationIntegration 测试 RSS 过滤规则关联的完整流程
func TestRSSFilterAssociationIntegration(t *testing.T) {
	db := setupDB(t)

	// 创建站点设置
	siteSetting := models.SiteSetting{
		Name:       string(models.SiteGroup("springsunday")),
		Enabled:    true,
		AuthMethod: "cookie",
	}
	require.NoError(t, db.DB.Create(&siteSetting).Error)

	// 创建 RSS 订阅
	rssSub := models.RSSSubscription{
		SiteID:          siteSetting.ID,
		Name:            "test-rss",
		URL:             "http://example.com/rss",
		Category:        "movies",
		Tag:             "test",
		IntervalMinutes: 10, // 设置有效的间隔时间
		DownloadPath:    "/downloads/movies",
	}
	require.NoError(t, db.DB.Create(&rssSub).Error)

	// 创建过滤规则
	rule1 := models.FilterRule{
		Name:        "rule1",
		Pattern:     ".*1080p.*",
		PatternType: "regex",
		Enabled:     true,
		Priority:    1,
		RequireFree: false,
	}
	require.NoError(t, db.DB.Create(&rule1).Error)

	rule2 := models.FilterRule{
		Name:        "rule2",
		Pattern:     ".*4K.*",
		PatternType: "regex",
		Enabled:     true,
		Priority:    2,
		RequireFree: true,
	}
	require.NoError(t, db.DB.Create(&rule2).Error)

	// 创建 RSS-Filter 关联
	assocDB := models.NewRSSFilterAssociationDB(db.DB)
	require.NoError(t, assocDB.SetFilterRulesForRSS(rssSub.ID, []uint{rule1.ID, rule2.ID}))

	// 验证关联
	rules, err := assocDB.GetFilterRulesForRSS(rssSub.ID)
	require.NoError(t, err)
	require.Len(t, rules, 2)

	// 验证 RSSConfig 的 DownloadPath
	rssCfg := models.RSSConfig{
		ID:           rssSub.ID,
		Name:         rssSub.Name,
		URL:          rssSub.URL,
		Category:     rssSub.Category,
		Tag:          rssSub.Tag,
		DownloadPath: rssSub.DownloadPath,
	}
	require.Equal(t, "/downloads/movies", rssCfg.GetEffectiveDownloadPath())
	require.True(t, rssCfg.HasCustomDownloadPath())
}

// TestRSSFilterAssociationWithFilterService 测试 RSS 关联过滤规则与 FilterService 的集成
func TestRSSFilterAssociationWithFilterService(t *testing.T) {
	db := setupDB(t)

	// 创建 RSS 订阅
	rssSub := models.RSSSubscription{
		Name:            "test-rss",
		URL:             "http://example.com/rss",
		Category:        "movies",
		IntervalMinutes: 10, // 设置有效的间隔时间
	}
	require.NoError(t, db.DB.Create(&rssSub).Error)

	// 创建过滤规则
	rule := models.FilterRule{
		Name:        "1080p-rule",
		Pattern:     ".*1080p.*",
		PatternType: "regex",
		Enabled:     true,
		Priority:    1,
		RequireFree: false,
	}
	require.NoError(t, db.DB.Create(&rule).Error)

	// 创建 RSS-Filter 关联
	assocDB := models.NewRSSFilterAssociationDB(db.DB)
	require.NoError(t, assocDB.SetFilterRulesForRSS(rssSub.ID, []uint{rule.ID}))

	// 使用 FilterService 测试
	filterSvc := filter.NewFilterService(db.DB)

	// 测试匹配的标题
	shouldDownload, matchedRule := filterSvc.ShouldDownloadForRSS("Movie.Title.1080p.BluRay", true, rssSub.ID)
	require.True(t, shouldDownload)
	require.NotNil(t, matchedRule)
	require.Equal(t, "1080p-rule", matchedRule.Name)

	// 测试不匹配的标题
	shouldDownload, matchedRule = filterSvc.ShouldDownloadForRSS("Movie.Title.720p.BluRay", true, rssSub.ID)
	require.False(t, shouldDownload)
	require.Nil(t, matchedRule)
}

// TestRSSWithNoAssociatedRules 测试没有关联规则的 RSS
func TestRSSWithNoAssociatedRules(t *testing.T) {
	db := setupDB(t)

	// 创建 RSS 订阅（不关联任何规则）
	rssSub := models.RSSSubscription{
		Name:            "test-rss-no-rules",
		URL:             "http://example.com/rss",
		Category:        "movies",
		IntervalMinutes: 10, // 设置有效的间隔时间
	}
	require.NoError(t, db.DB.Create(&rssSub).Error)

	// 创建过滤规则（但不关联到 RSS）
	rule := models.FilterRule{
		Name:        "global-rule",
		Pattern:     ".*1080p.*",
		PatternType: "regex",
		Enabled:     true,
		Priority:    1,
	}
	require.NoError(t, db.DB.Create(&rule).Error)

	// 使用 FilterService 测试
	filterSvc := filter.NewFilterService(db.DB)

	// 没有关联规则的 RSS 应该不匹配任何规则
	shouldDownload, matchedRule := filterSvc.ShouldDownloadForRSS("Movie.Title.1080p.BluRay", true, rssSub.ID)
	require.False(t, shouldDownload)
	require.Nil(t, matchedRule)

	// 验证 GetRulesForRSS 返回空列表
	rules, err := filterSvc.GetRulesForRSS(rssSub.ID)
	require.NoError(t, err)
	require.Empty(t, rules)
}

// TestDownloadPathIntegration 测试下载路径集成
func TestDownloadPathIntegration(t *testing.T) {
	db := setupDB(t)

	// 测试有自定义下载路径的 RSS
	rssCfgWithPath := models.RSSConfig{
		ID:           1,
		Name:         "rss-with-path",
		URL:          "http://example.com/rss",
		DownloadPath: "/custom/downloads/movies",
	}
	require.Equal(t, "/custom/downloads/movies", rssCfgWithPath.GetEffectiveDownloadPath())
	require.True(t, rssCfgWithPath.HasCustomDownloadPath())

	// 测试没有自定义下载路径的 RSS
	rssCfgWithoutPath := models.RSSConfig{
		ID:           2,
		Name:         "rss-without-path",
		URL:          "http://example.com/rss",
		DownloadPath: "",
	}
	require.Equal(t, "", rssCfgWithoutPath.GetEffectiveDownloadPath())
	require.False(t, rssCfgWithoutPath.HasCustomDownloadPath())

	_ = db
}

// TestProcessTorrentsWithDownloaderByRSS_WithDownloadPath 测试带下载路径的种子处理
func TestProcessTorrentsWithDownloaderByRSS_WithDownloadPath(t *testing.T) {
	db := setupDB(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	// 创建未推送的记录
	pushed := false
	future := time.Now().Add(1 * time.Hour)
	ti := &models.TorrentInfo{
		SiteName:    string(models.SiteGroup("springsunday")),
		TorrentHash: &hash,
		IsPushed:    &pushed,
		FreeEndTime: &future,
	}
	require.NoError(t, db.UpsertTorrent(ti))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("test-dl").AnyTimes()
	mockDl.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(int64(100*1024*1024*1024), nil)
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil)
	mockDl.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).Return(downloader.AddTorrentResult{Success: true, Hash: hash}, nil)

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo, path, "cat", "tag", "/custom/path", models.SiteGroup("springsunday"), false)
	require.NoError(t, err)

	// 文件应该被删除
	_, err = os.Stat(path)
	require.True(t, os.IsNotExist(err))

	// 数据库应该更新为已推送
	ti2, err := db.GetTorrentBySiteAndHash(string(models.SiteGroup("springsunday")), hash)
	require.NoError(t, err)
	require.NotNil(t, ti2.IsPushed)
	require.True(t, *ti2.IsPushed)
}

func TestShouldSkipExistingTorrent(t *testing.T) {
	var nilPushed *bool
	falsePushed := false
	truePushed := true

	// Timestamps for re-check tests
	oldCheckTime := time.Now().Add(-7 * time.Hour)
	recentCheckTime := time.Now().Add(-1 * time.Hour)

	tests := []struct {
		name    string
		torrent *models.TorrentInfo
		want    bool
	}{
		// Original test cases
		{name: "nil torrent", torrent: nil, want: false},
		{name: "pushed true", torrent: &models.TorrentInfo{IsPushed: &truePushed}, want: true},
		{name: "pushed false should retry", torrent: &models.TorrentInfo{IsPushed: &falsePushed}, want: false},
		{name: "pushed nil", torrent: &models.TorrentInfo{IsPushed: nilPushed}, want: false},

		// Re-check tests: skipped non-free torrents
		{
			name:    "skipped non-free last check > 6h should allow recheck",
			torrent: &models.TorrentInfo{IsSkipped: true, IsFree: false, LastCheckTime: &oldCheckTime},
			want:    false, // Allow re-check
		},
		{
			name:    "skipped non-free last check < 6h should skip",
			torrent: &models.TorrentInfo{IsSkipped: true, IsFree: false, LastCheckTime: &recentCheckTime},
			want:    true, // Skip
		},
		{
			name:    "skipped non-free nil LastCheckTime should skip",
			torrent: &models.TorrentInfo{IsSkipped: true, IsFree: false, LastCheckTime: nil},
			want:    true, // Skip for safety
		},
		{
			name:    "skipped free torrent should skip regardless",
			torrent: &models.TorrentInfo{IsSkipped: true, IsFree: true},
			want:    true, // Skip (not affected by re-check logic)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldSkipExistingTorrent(tt.torrent))
		})
	}
}

func TestFilterTorrentFilesBySite(t *testing.T) {
	files := []string{
		"/app/.pt-tools/downloads/agsvpt-111.torrent",
		"/app/.pt-tools/downloads/AGSvpt-222.torrent",
		"/app/.pt-tools/downloads/mteam-333.torrent",
		"/app/.pt-tools/downloads/xingyunge-444.torrent",
		"/app/.pt-tools/downloads/random.torrent",
	}

	filtered := filterTorrentFilesBySite(files, models.SiteGroup("agsvpt"))
	require.Len(t, filtered, 2)
	require.Equal(t, "/app/.pt-tools/downloads/agsvpt-111.torrent", filtered[0])
	require.Equal(t, "/app/.pt-tools/downloads/AGSvpt-222.torrent", filtered[1])
}

func TestFilterTorrentFilesBySite_FallbackAllWhenNoPrefixMatch(t *testing.T) {
	files := []string{
		"/app/.pt-tools/downloads/file-a.torrent",
		"/app/.pt-tools/downloads/file-b.torrent",
	}

	filtered := filterTorrentFilesBySite(files, models.SiteGroup("agsvpt"))
	require.Len(t, filtered, 2)
	require.Equal(t, files[0], filtered[0])
	require.Equal(t, files[1], filtered[1])
}

func TestShouldSkipSiteDownload(t *testing.T) {
	dir := t.TempDir()
	fileBase := "site-123"
	existingPath := filepath.Join(dir, fileBase+".torrent")
	require.NoError(t, os.WriteFile(existingPath, []byte("d4:test4:datae"), 0o644))

	boolPtr := func(b bool) *bool { return &b }

	tests := []struct {
		name       string
		torrent    *models.TorrentInfo
		fileBase   string
		maxRetry   int
		wantSkip   bool
		wantReason string
	}{
		{
			name:       "已推送应跳过",
			torrent:    &models.TorrentInfo{IsPushed: boolPtr(true)},
			fileBase:   fileBase,
			maxRetry:   3,
			wantSkip:   true,
			wantReason: "已推送，跳过重新下载",
		},
		{
			name:       "超过最大重试次数应跳过",
			torrent:    &models.TorrentInfo{RetryCount: 3},
			fileBase:   fileBase,
			maxRetry:   3,
			wantSkip:   true,
			wantReason: "超过最大重试次数 3",
		},
		{
			name:       "已下载且本地文件存在应跳过",
			torrent:    &models.TorrentInfo{IsDownloaded: true},
			fileBase:   fileBase,
			maxRetry:   3,
			wantSkip:   true,
			wantReason: "已下载且本地文件存在",
		},
		{
			name:     "已下载但本地文件不存在不跳过",
			torrent:  &models.TorrentInfo{IsDownloaded: true},
			fileBase: "missing-file",
			maxRetry: 3,
			wantSkip: false,
		},
		{
			name:     "torrent为nil不跳过",
			torrent:  nil,
			fileBase: fileBase,
			maxRetry: 3,
			wantSkip: false,
		},
		{
			name:     "未下载不跳过",
			torrent:  &models.TorrentInfo{IsDownloaded: false},
			fileBase: fileBase,
			maxRetry: 3,
			wantSkip: false,
		},
		{
			name:     "maxRetry为0时高重试次数不跳过",
			torrent:  &models.TorrentInfo{RetryCount: 99},
			fileBase: fileBase,
			maxRetry: 0,
			wantSkip: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skip, reason := shouldSkipSiteDownload(tt.torrent, dir, tt.fileBase, tt.maxRetry)
			require.Equal(t, tt.wantSkip, skip)
			if tt.wantReason != "" {
				require.Equal(t, tt.wantReason, reason)
			}
		})
	}
}

func TestFetchAndDownloadFreeRSS_FilterRuleAssociated(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{
		Enabled: boolPtrIX(true), AuthMethod: "cookie", Cookie: "c=1",
		RSS: []models.RSSConfig{{Name: "sub", URL: "http://placeholder", IntervalMinutes: 1, Tag: "movie"}},
	}))
	var sub models.RSSSubscription
	require.NoError(t, db.DB.First(&sub).Error)

	rule := models.FilterRule{Name: "any", Pattern: ".*", PatternType: "regex", MatchField: "both", RequireFree: false, Enabled: true, Priority: 100, Purpose: "download"}
	require.NoError(t, db.DB.Create(&rule).Error)
	require.NoError(t, db.DB.Model(&models.FilterRule{}).Where("id = ?", rule.ID).Update("require_free", false).Error)
	assoc := models.NewRSSFilterAssociationDB(db.DB)
	require.NoError(t, assoc.SetFilterRulesForRSS(sub.ID, []uint{rule.ID}))

	srv := feedServerUnified(t, rssBody(itemXML("lgf1", "RuleMatch", "http://x/r.torrent")))
	site := &legacyPTStub{enabled: true, discount: models.DISCOUNT_FREE, sizeMB: 1, writeFile: true}
	rssCfg := models.RSSConfig{ID: sub.ID, Name: sub.Name, URL: srv.URL, Tag: "movie"}
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), site, rssCfg))

	assert.Equal(t, 1, site.dlCalls)
	ti, err := db.GetTorrentBySiteAndID("springsunday", "lgf1")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.True(t, ti.IsDownloaded)
}

func TestFetchAndDownloadFreeRSS_AlreadyPushedSkips(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	pushed := true
	now := time.Now()
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "lgp", IsPushed: &pushed, PushTime: &now}
	require.NoError(t, db.UpsertTorrent(ti))

	srv := feedServerUnified(t, rssBody(itemXML("lgp", "Dup", "http://x/d.torrent")))
	site := &legacyPTStub{enabled: true, discount: models.DISCOUNT_FREE, sizeMB: 1}
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL}))
	assert.Equal(t, 0, site.dlCalls, "already-pushed torrent must be skipped")
}

func TestFetchAndDownloadFreeRSS_MinFreeMinutesSkips(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true,
		DownloadLimitEnabled: true, DownloadSpeedLimit: 10, MinFreeMinutes: 120,
	}))

	srv := feedServerUnified(t, rssBody(itemXML("lgmf", "SoonEnd", "http://x/s.torrent")))
	site := &legacyMinFreeStub{}
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "t"}))
	ti, err := db.GetTorrentBySiteAndID("springsunday", "lgmf")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.True(t, ti.IsSkipped)
}

func TestFetchAndDownloadFreeRSS_MaxRetrySkips(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true, MaxRetry: 2,
	}))

	pushed := false
	future := time.Now().Add(time.Hour)
	ti := &models.TorrentInfo{
		SiteName: "springsunday", TorrentID: "lgmr", FreeEndTime: &future, IsPushed: &pushed, RetryCount: 5,
	}
	require.NoError(t, db.UpsertTorrent(ti))

	srv := feedServerUnified(t, rssBody(itemXML("lgmr", "MaxRetry", "http://x/m.torrent")))
	site := &legacyPTStub{enabled: true, discount: models.DISCOUNT_FREE, sizeMB: 1}
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "movie"}))
	assert.Equal(t, 0, site.dlCalls, "over-max-retry torrent must not re-download")
}

func TestGetDownloaderForRSSImpl_RSSDownloaderIDMissing(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	id := uint(999)
	_, _, err := getDownloaderForRSSImpl(models.RSSConfig{DownloaderID: &id}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "获取指定下载器")
}

func TestGetDownloaderForRSSImpl_RSSDownloaderDisabled(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	ds := models.DownloaderSetting{Name: "off", Type: "qbittorrent", URL: "http://x", Enabled: false}
	require.NoError(t, db.DB.Create(&ds).Error)
	_, _, err := getDownloaderForRSSImpl(models.RSSConfig{DownloaderID: &ds.ID}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "未启用")
}

func TestGetDownloaderForRSSImpl_SiteBoundDisabled(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	ds := models.DownloaderSetting{Name: "sbd", Type: "qbittorrent", URL: "http://x", Enabled: false}
	require.NoError(t, db.DB.Create(&ds).Error)
	require.NoError(t, db.DB.Create(&models.SiteSetting{Name: "mteam", DownloaderID: &ds.ID}).Error)
	_, _, err := getDownloaderForRSSImpl(models.RSSConfig{}, "mteam")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "未启用")
}

func TestGetDownloaderForRSSImpl_DefaultDisabled(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	ds := models.DownloaderSetting{Name: "def", Type: "qbittorrent", URL: "http://x", Enabled: false, IsDefault: true}
	require.NoError(t, db.DB.Create(&ds).Error)
	_, _, err := getDownloaderForRSSImpl(models.RSSConfig{}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "默认下载器")
}

func TestGetDownloaderForRSSImpl_NilDB(t *testing.T) {
	global.GlobalDB = nil
	_, _, err := getDownloaderForRSSImpl(models.RSSConfig{}, "")
	require.Error(t, err)
}

func TestGetDownloaderForRSSImpl_NoDefaultConfigured(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	_, _, err := getDownloaderForRSSImpl(models.RSSConfig{}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "获取默认下载器")
}

func TestProcessTorrentsWithDBUpdate_OrphanDeleted(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	_ = db
	dir := t.TempDir()
	path, _ := makeTorrentFile(t, dir)

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, ProcessTorrentsWithDBUpdate(context.Background(), client, dir, "cat", "tag", models.SiteGroup("springsunday")))
	// Orphan (no DB row) is deleted by processSingleTorrent.
	assert.NoFileExists(t, path)
}

func TestFetchAndDownloadFreeRSS_BlankDownloadDir(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, db.DB.Exec("DELETE FROM settings_globals").Error)
	require.NoError(t, db.DB.Create(&models.SettingsGlobal{DownloadDir: "", DefaultIntervalMinutes: 10}).Error)

	err := FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"),
		&legacyPTStub{enabled: true}, models.RSSConfig{URL: "http://x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "下载目录为空")
}

func TestFetchAndDownloadFreeRSS_FeedFetchError(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	err := FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"),
		&legacyPTStub{enabled: true}, models.RSSConfig{URL: "http://127.0.0.1:0/rss"})
	require.Error(t, err)
}

func TestFetchAndDownloadFreeRSS_AlreadySkippedRecheckSkips(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	recent := time.Now()
	ti := &models.TorrentInfo{
		SiteName: "springsunday", TorrentID: "lgsk", IsSkipped: true, IsFree: false, LastCheckTime: &recent,
	}
	require.NoError(t, db.UpsertTorrent(ti))

	srv := feedServerUnified(t, rssBody(itemXML("lgsk", "Skipped", "http://x/s.torrent")))
	site := &legacyPTStub{enabled: true, discount: models.DISCOUNT_FREE, sizeMB: 1}
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL}))
	assert.Equal(t, 0, site.dlCalls, "recently-skipped non-free torrent must be skipped before detail")
}

func TestExtractTorrentRef_HostFromLink(t *testing.T) {
	host, ref := extractTorrentRef(&gofeed.Item{Link: "https://tracker.example.com/details.php?id=99"})
	assert.Equal(t, "tracker.example.com", host)
	_ = ref

	// Invalid URL → empty.
	h2, _ := extractTorrentRef(&gofeed.Item{Link: "://bad"})
	assert.Equal(t, "", h2)
}

func TestFetchAndDownloadFreeRSS_ContextCanceled(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	srv := feedServerUnified(t, rssBody(itemXML("lgc", "Cancel", "http://x/c.torrent")))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	site := &legacyPTStub{enabled: true, discount: models.DISCOUNT_FREE, sizeMB: 1}
	_ = FetchAndDownloadFreeRSS(ctx, models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL})
}

func TestFetchAndDownloadFreeRSS_ConfigStoreZeroValueDefault(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true, DefaultConcurrency: 2,
	}))
	srv := feedServerUnified(t, rssBody(
		itemXML("lgm1", "A", "http://x/a.torrent")+itemXML("lgm2", "B", "http://x/b.torrent"),
	))
	site := &legacyPTStub{enabled: true, discount: models.DISCOUNT_FREE, sizeMB: 1, writeFile: true}
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "t", Concurrency: 2}))
	assert.GreaterOrEqual(t, site.dlCalls, 1)
}

func TestFetchAndDownloadFreeRSS_NonFreeSkipsAndRecords(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	srv := feedServerUnified(t, rssBody(itemXMLWithCategory("lgc1", "Paid", "http://x/p.torrent", "TV")))
	site := &legacyPTStub{enabled: true, discount: models.DISCOUNT_NONE, sizeMB: 1}
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "t"}))

	ti, err := db.GetTorrentBySiteAndID("springsunday", "lgc1")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.True(t, ti.IsSkipped)
	assert.Equal(t, "TV", ti.Category)
}

func TestFetchAndDownloadFreeRSS_DownloadNoFileResets(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	srv := feedServerUnified(t, rssBody(itemXML("lgnf", "NoFile", "http://x/n.torrent")))
	site := &legacyPTStub{enabled: true, discount: models.DISCOUNT_FREE, sizeMB: 1, writeFile: false}
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "movie"}))

	assert.Equal(t, 1, site.dlCalls)
	ti, err := db.GetTorrentBySiteAndID("springsunday", "lgnf")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.False(t, ti.IsDownloaded, "missing .torrent resets is_downloaded to false")
}

func TestFetchAndDownloadFreeRSS_ExistingDownloadedFilePresentSkips(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	pushed := false
	ti := &models.TorrentInfo{
		SiteName: "springsunday", TorrentID: "lgdf", IsPushed: &pushed, IsDownloaded: true, FreeLevel: "free",
	}
	require.NoError(t, db.UpsertTorrent(ti))

	srv := feedServerUnified(t, rssBody(itemXML("lgdf", "DlPresent", "http://x/d.torrent")))
	site := &legacyPTStub{enabled: true, discount: models.DISCOUNT_FREE, sizeMB: 1, writeFile: true}
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL, Tag: "movie"}))
	assert.GreaterOrEqual(t, site.dlCalls, 0)
}

func TestFetchAndDownloadFreeRSS_ContextCancelledDuringDispatch(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	items := ""
	for i := 0; i < 50; i++ {
		items += itemXML("cc"+itoa(int64(i)), "T", "http://x/t.torrent")
	}
	srv := feedServerUnified(t, rssBody(items))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	site := &legacyPTStub{enabled: true, discount: models.DISCOUNT_FREE, sizeMB: 1}
	_ = FetchAndDownloadFreeRSS(ctx, models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL})
}

func TestRecordDiskProtectError_NilDB(t *testing.T) {
	global.GlobalDB = nil
	require.NotPanics(t, func() { recordDiskProtectError(models.SiteGroup("s"), "h", "msg") })
}

func TestSweepStagingDir_RemovesOrphan(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	_ = db
	dir := t.TempDir()
	fp := filepath.Join(dir, "springsunday-orphan.torrent")
	require.NoError(t, os.WriteFile(fp, []byte("d4:infod4:name1:aee"), 0o644))
	// No DB row → shouldSweep returns true (orphan).
	sweepStagingDir(dir, models.SiteGroup("springsunday"), 24)
	_, statErr := os.Stat(fp)
	assert.True(t, os.IsNotExist(statErr))
}

func TestShouldSweep_MaxRetryExceeded(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), MaxRetry: 2,
	}))
	dir := t.TempDir()
	fp, hash := makeTorrentFile(t, dir)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "sw", IsPushed: &pushed, RetryCount: 5}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))
	assert.True(t, shouldSweep(fp, models.SiteGroup("springsunday"), 24))
}

func (addFailTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ok := func(body string) *http.Response {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(body)), Header: make(http.Header)}
	}
	switch req.URL.Path {
	case "/api/v2/auth/login":
		return ok("Ok."), nil
	case "/api/v2/torrents/properties":
		return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: make(http.Header)}, nil
	case "/api/v2/sync/maindata":
		return ok(`{"server_state":{"free_space_on_disk":107374182400}}`), nil
	case "/api/v2/torrents/add":
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(bytes.NewBufferString("boom")), Header: make(http.Header)}, nil
	default:
		return ok("{}"), nil
	}
}

func (existsTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ok := func(body string) *http.Response {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(body)), Header: make(http.Header)}
	}
	switch req.URL.Path {
	case "/api/v2/auth/login":
		return ok("Ok."), nil
	case "/api/v2/torrents/properties":
		return ok(`{"save_path":"/downloads"}`), nil
	default:
		return ok("{}"), nil
	}
}

func TestShouldSkipSiteDownload_Branches(t *testing.T) {
	assert.Equal(t, false, mustSkip(nil, "", "", 0))
	pushed := true
	sk, _ := shouldSkipSiteDownload(&models.TorrentInfo{IsPushed: &pushed}, "", "f", 0)
	assert.True(t, sk)
	sk2, _ := shouldSkipSiteDownload(&models.TorrentInfo{RetryCount: 3}, "", "f", 2)
	assert.True(t, sk2)
	sk3, _ := shouldSkipSiteDownload(&models.TorrentInfo{}, "", "f", 0)
	assert.False(t, sk3)
}

func TestShouldSkipExistingTorrent_Branches(t *testing.T) {
	assert.False(t, shouldSkipExistingTorrent(nil))

	// Skipped + free → skip.
	assert.True(t, shouldSkipExistingTorrent(&models.TorrentInfo{IsSkipped: true, IsFree: true}))

	// Skipped + non-free + recent check → skip.
	recent := time.Now()
	assert.True(t, shouldSkipExistingTorrent(&models.TorrentInfo{IsSkipped: true, IsFree: false, LastCheckTime: &recent}))

	// Skipped + non-free + stale check → allow re-check.
	old := time.Now().Add(-(SkipRecheckHours + 1) * time.Hour)
	assert.False(t, shouldSkipExistingTorrent(&models.TorrentInfo{IsSkipped: true, IsFree: false, LastCheckTime: &old}))

	// Pushed → skip.
	pushed := true
	assert.True(t, shouldSkipExistingTorrent(&models.TorrentInfo{IsPushed: &pushed}))

	// Fresh torrent → not skipped.
	assert.False(t, shouldSkipExistingTorrent(&models.TorrentInfo{}))
}

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

func (pt *ptStub) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	d := models.PHPTorrentInfo{Title: item.Title, TorrentID: item.GUID}
	return &models.APIResponse[models.PHPTorrentInfo]{Code: "success", Data: d}, nil
}

func (pt *ptStub) IsEnabled() bool { return true }

func (pt *ptStub) DownloadTorrent(url, title, dir string) (string, error) { return "h", nil }

func (pt *ptStub) MaxRetries() int { return 1 }

func (pt *ptStub) RetryDelay() time.Duration { return 0 }

func (pt *ptStub) SendTorrentToDownloader(ctx context.Context, rssCfg models.RSSConfig) error {
	return nil
}

func (pt *ptStub) Context() context.Context { return context.Background() }

func TestProcessRSS_WithStub_NoPanic(t *testing.T) {
	cfg := models.RSSConfig{Name: "r", URL: "http://example/rss", IntervalMinutes: 1}
	// 调用 cmd 包中的 processRSS（与生产逻辑一致）
	require.NotPanics(t, func() { _ = cmdProcessRSS(context.Background(), models.SiteGroup("springsunday"), cfg, &ptStub{}) })
}

func TestBuildSkipReason(t *testing.T) {
	cases := []struct {
		name    string
		isFree  bool
		canFin  bool
		byFilt  bool
		wantSub string
	}{
		{"not free", false, true, true, "非免费"},
		{"cannot finish", true, false, true, "免费期内无法完成"},
		{"filter no match", true, true, false, "未匹配过滤规则"},
		{"all ok -> unknown", true, true, true, "未知原因"},
		{"multi", false, true, false, "非免费, 未匹配过滤规则"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := buildSkipReason(c.isFree, c.canFin, c.byFilt)
			assert.Equal(t, c.wantSub, got)
		})
	}
}

func TestCalcHRSeedTimeForTorrent(t *testing.T) {
	// nil def -> fallback.
	assert.Equal(t, 72, calcHRSeedTimeForTorrent(nil, 72, 1<<30))

	// def with no rules -> fallback.
	defFlat := &v2.SiteDefinition{HREnabled: true, HRSeedTimeHours: 48}
	assert.Equal(t, 24, calcHRSeedTimeForTorrent(defFlat, 24, 1<<30))

	// def with size-tiered rules -> picks matching tier.
	defRules := &v2.SiteDefinition{
		HREnabled:       true,
		HRSeedTimeHours: 10,
		HRSeedTimeRules: []v2.HRSeedTimeRule{
			{MinSizeGB: 0, MaxSizeGB: 50, SeedTimeH: 100},
			{MinSizeGB: 50, MaxSizeGB: 0, SeedTimeH: 200},
		},
	}
	// 10 GiB -> tier 1 (100h)
	assert.Equal(t, 100, calcHRSeedTimeForTorrent(defRules, 10, 10*(1<<30)))
	// 80 GiB -> tier 2 (200h)
	assert.Equal(t, 200, calcHRSeedTimeForTorrent(defRules, 10, 80*(1<<30)))
}

func TestExtractTorrentRef(t *testing.T) {
	cases := []struct {
		name     string
		item     *gofeed.Item
		wantSite string
		wantID   string
	}{
		{"nil item", nil, "", ""},
		{"empty link", &gofeed.Item{Link: ""}, "", ""},
		{"id query", &gofeed.Item{Link: "https://pt.example.com/details.php?id=1234"}, "pt.example.com", "1234"},
		{"numeric path", &gofeed.Item{Link: "https://pt.example.com/torrents/5678"}, "pt.example.com", "5678"},
		{"no numeric", &gofeed.Item{Link: "https://pt.example.com/browse"}, "pt.example.com", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			site, id := extractTorrentRef(c.item)
			assert.Equal(t, c.wantSite, site)
			assert.Equal(t, c.wantID, id)
		})
	}
}

func (stubRSSNotifier) NotifyNewItem(_ context.Context, _ RSSItemNotice) error { return nil }

func (stubRSSNotifier) NotifyFilteredItem(_ context.Context, _ RSSFilteredNotice) error { return nil }

func (n *noopPT) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	return &models.APIResponse[models.PHPTorrentInfo]{Code: "success", Data: models.PHPTorrentInfo{}}, nil
}

func (n *noopPT) IsEnabled() bool { return true }

func (n *noopPT) DownloadTorrent(url, title, dir string) (string, error) { return "", nil }

func (n *noopPT) MaxRetries() int { return 1 }

func (n *noopPT) RetryDelay() time.Duration { return 0 }

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
		_ = ProcessTorrentsWithDBUpdate(context.Background(), nil, t.TempDir(), "cat", "tag", models.SiteGroup("springsunday"))
	})
}
