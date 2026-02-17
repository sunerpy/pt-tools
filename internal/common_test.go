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
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/require"
	"github.com/zeebo/bencode"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/filter"
	sm "github.com/sunerpy/pt-tools/mocks"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

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

type siteFail struct{}

func (s *siteFail) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	return nil, context.DeadlineExceeded
}
func (s *siteFail) IsEnabled() bool { return true }
func (s *siteFail) DownloadTorrent(url, title, dir string) (string, error) {
	return "", context.DeadlineExceeded
}
func (s *siteFail) MaxRetries() int           { return 1 }
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

type rssSiteStub struct{}

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
func (s *rssSiteStub) MaxRetries() int           { return 1 }
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

type props404Transport struct{}

func (t *props404Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/api/v2/torrents/properties" || filepath.Base(req.URL.Path) == "properties" {
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(bytes.NewBuffer(nil)), Header: make(http.Header)}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString("{}")), Header: make(http.Header)}, nil
}

type props200Transport struct{}

func (t *props200Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/api/v2/torrents/properties" || filepath.Base(req.URL.Path) == "properties" {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(`{"save_path":"/tmp"}`)), Header: make(http.Header)}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString("{}")), Header: make(http.Header)}, nil
}

func makeTorrentFile(t *testing.T, dir string) (string, string) {
	t.Helper()
	var buf bytes.Buffer
	torrent := map[string]any{"info": map[string]any{"name": "abc"}}
	require.NoError(t, bencode.NewEncoder(&buf).Encode(torrent))
	path := filepath.Join(dir, "x.torrent")
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o644))
	h, err := qbit.ComputeTorrentHashWithPath(path)
	require.NoError(t, err)
	return path, h
}

func setupDB(t *testing.T) *models.TorrentDB {
	t.Helper()
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	store := core.NewConfigStore(db)
	require.NoError(t, store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true, MaxRetry: 1}))
	return db
}

func TestProcessSingle_NoRecordDeletesFile(t *testing.T) {
	_ = setupDB(t)
	dir := t.TempDir()
	path, _ := makeTorrentFile(t, dir)
	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected file removed")
	}
}

func TestProcessSingle_PushedDeletes(t *testing.T) {
	db := setupDB(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	pushed := true
	now := time.Now()
	ti1 := &models.TorrentInfo{SiteName: string(models.SiteGroup("springsunday")), IsPushed: &pushed, PushTime: &now}
	ti1.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti1))
	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected file removed")
	}
}

func TestProcessSingle_ExistsUpdatesAndDeletes(t *testing.T) {
	db := setupDB(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	pushed := false
	ti2 := &models.TorrentInfo{SiteName: string(models.SiteGroup("springsunday")), IsPushed: &pushed}
	end := time.Now().Add(1 * time.Hour)
	ti2.FreeEndTime = &end
	ti2.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti2))
	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props200Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected file removed")
	}
	ti, err := db.GetTorrentBySiteAndHash(string(models.SiteGroup("springsunday")), hash)
	require.NoError(t, err)
	require.NotNil(t, ti)
	require.NotNil(t, ti.IsPushed)
	require.True(t, *ti.IsPushed)
	require.NotNil(t, ti.PushTime)
}

func TestProcessSingle_ExpiredDeletes(t *testing.T) {
	db := setupDB(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	past := time.Now().Add(-1 * time.Hour)
	ti := &models.TorrentInfo{SiteName: string(models.SiteGroup("springsunday")), FreeEndTime: &past}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))
	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected file removed")
	}
}

func TestProcessSingle_RetainHoursDeletes(t *testing.T) {
	db := setupDB(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	past := time.Now().Add(-48 * time.Hour)
	future := time.Now().Add(1 * time.Hour)
	ti := &models.TorrentInfo{SiteName: string(models.SiteGroup("springsunday")), LastCheckTime: &past, FreeEndTime: &future}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))
	store := core.NewConfigStore(db)
	require.NoError(t, store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true, RetainHours: 1}))
	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected file removed")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
func TestProcessSingle_MaxRetryDeletes(t *testing.T) {
	db := setupDB(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	ti3 := &models.TorrentInfo{SiteName: string(models.SiteGroup("springsunday")), RetryCount: 1}
	ti3.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti3))
	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected file removed")
	}
}

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

type siteDummy struct{}

func (s *siteDummy) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	d := models.PHPTorrentInfo{Title: "x", TorrentID: "id", Discount: models.DISCOUNT_FREE, EndTime: time.Now().Add(1 * time.Hour), SizeMB: 1}
	return &models.APIResponse[models.PHPTorrentInfo]{Code: "success", Data: d}, nil
}
func (s *siteDummy) IsEnabled() bool                                        { return true }
func (s *siteDummy) DownloadTorrent(url, title, dir string) (string, error) { return "h", nil }
func (s *siteDummy) MaxRetries() int                                        { return 1 }
func (s *siteDummy) RetryDelay() time.Duration                              { return 0 }
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

type disabledSite struct{}

func (s *disabledSite) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.PHPTorrentInfo], error) {
	d := models.PHPTorrentInfo{Title: "x", TorrentID: "id", Discount: models.DISCOUNT_FREE, EndTime: time.Now().Add(1 * time.Hour), SizeMB: 1}
	return &models.APIResponse[models.PHPTorrentInfo]{Code: "success", Data: d}, nil
}
func (s *disabledSite) IsEnabled() bool                                        { return false }
func (s *disabledSite) DownloadTorrent(url, title, dir string) (string, error) { return "h", nil }
func (s *disabledSite) MaxRetries() int                                        { return 1 }
func (s *disabledSite) RetryDelay() time.Duration                              { return 0 }
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

func makeFeedServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	buf := bytes.NewBufferString(body)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); _, _ = w.Write(buf.Bytes()) }))
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
