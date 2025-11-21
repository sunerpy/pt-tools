package internal

import (
	"bytes"
	"context"
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
	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	sm "github.com/sunerpy/pt-tools/mocks"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/qbit"
	"github.com/zeebo/bencode"
	"go.uber.org/zap"
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
func (s *siteFail) MaxRetries() int                                                      { return 1 }
func (s *siteFail) RetryDelay() time.Duration                                            { return 0 }
func (s *siteFail) SendTorrentToQbit(ctx context.Context, rssCfg models.RSSConfig) error { return nil }
func (s *siteFail) Context() context.Context                                             { return context.Background() }
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
	require.NoError(t, FetchAndDownloadFreeRSS(context.Background(), models.CMCT, &siteFail{}, models.RSSConfig{Name: "r", URL: srv.URL}))
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
func (s *rssSiteStub) SendTorrentToQbit(ctx context.Context, rssCfg models.RSSConfig) error {
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
	require.NoError(t, FetchAndDownloadFreeRSS(ctx, models.CMCT, &rssSiteStub{}, models.RSSConfig{Name: "r", URL: srv.URL, Tag: "tag"}))
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
	client := &qbit.QbitClient{Client: &http.Client{Transport: &props404Transport{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer client.RateLimiter.Stop()
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.CMCT))
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
	ti1 := &models.TorrentInfo{SiteName: string(models.CMCT), IsPushed: &pushed, PushTime: &now}
	ti1.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti1))
	client := &qbit.QbitClient{Client: &http.Client{Transport: &props404Transport{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer client.RateLimiter.Stop()
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.CMCT))
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected file removed")
	}
}

func TestProcessSingle_ExistsUpdatesAndDeletes(t *testing.T) {
	db := setupDB(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	pushed := false
	ti2 := &models.TorrentInfo{SiteName: string(models.CMCT), IsPushed: &pushed}
	end := time.Now().Add(1 * time.Hour)
	ti2.FreeEndTime = &end
	ti2.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti2))
	client := &qbit.QbitClient{Client: &http.Client{Transport: &props200Transport{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer client.RateLimiter.Stop()
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.CMCT))
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected file removed")
	}
	ti, err := db.GetTorrentBySiteAndHash(string(models.CMCT), hash)
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
	ti := &models.TorrentInfo{SiteName: string(models.CMCT), FreeEndTime: &past}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))
	client := &qbit.QbitClient{Client: &http.Client{Transport: &props404Transport{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer client.RateLimiter.Stop()
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.CMCT))
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
	ti := &models.TorrentInfo{SiteName: string(models.CMCT), LastCheckTime: &past, FreeEndTime: &future}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))
	store := core.NewConfigStore(db)
	require.NoError(t, store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true, RetainHours: 1}))
	client := &qbit.QbitClient{Client: &http.Client{Transport: &props404Transport{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer client.RateLimiter.Stop()
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.CMCT))
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
	ti3 := &models.TorrentInfo{SiteName: string(models.CMCT), RetryCount: 1}
	ti3.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti3))
	client := &qbit.QbitClient{Client: &http.Client{Transport: &props404Transport{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer client.RateLimiter.Stop()
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.CMCT))
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
	ti := &models.TorrentInfo{SiteName: string(models.CMCT), TorrentHash: &h2, IsPushed: &pushed, FreeEndTime: &future}
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
	client := &qbit.QbitClient{Client: &http.Client{Transport: rt}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer client.RateLimiter.Stop()
	require.NoError(t, ProcessTorrentsWithDBUpdate(context.Background(), client, dir, "cat", "tag", models.CMCT))
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
	ti := &models.TorrentInfo{SiteName: string(models.CMCT), IsPushed: &pushed}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))
	client := &qbit.QbitClient{Client: &http.Client{Transport: &props404Transport{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer client.RateLimiter.Stop()
	require.NoError(t, ProcessTorrentsWithDBUpdate(context.Background(), client, dir, "cat", "tag", models.CMCT))
}

func TestProcessTorrentsWithDBUpdate_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true}))
	client := &qbit.QbitClient{Client: &http.Client{Transport: &props404Transport{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer client.RateLimiter.Stop()
	require.NoError(t, ProcessTorrentsWithDBUpdate(context.Background(), client, dir, "cat", "tag", models.CMCT))
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
	downloadWorker(context.Background(), models.CMCT, &wg, m, ch, models.RSSConfig{Tag: "t"})
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
	downloadWorker(context.Background(), models.CMCT, &wg, m, ch, models.RSSConfig{Tag: "t"})
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
		{"cannotfinish", models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true, DownloadLimitEnabled: true, DownloadSpeedLimit: 0, TorrentSizeGB: 200}, models.PHPTorrentInfo{Title: "SkipTitle", TorrentID: "guid-skip", Discount: models.DISCOUNT_FREE, EndTime: time.Now().Add(5 * time.Minute), SizeMB: 512}, false, true},
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
		downloadWorker(context.Background(), models.CMCT, &wg, m, ch, models.RSSConfig{Tag: c.detail.Title})
		wg.Wait()
		ti, err := global.GlobalDB.GetTorrentBySiteAndID(string(models.CMCT), item.GUID)
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
func (s *siteDummy) IsEnabled() bool                                                      { return true }
func (s *siteDummy) DownloadTorrent(url, title, dir string) (string, error)               { return "h", nil }
func (s *siteDummy) MaxRetries() int                                                      { return 1 }
func (s *siteDummy) RetryDelay() time.Duration                                            { return 0 }
func (s *siteDummy) SendTorrentToQbit(ctx context.Context, rssCfg models.RSSConfig) error { return nil }
func (s *siteDummy) Context() context.Context                                             { return context.Background() }
func TestFetchRSS_InvalidURL(t *testing.T) {
	err := FetchAndDownloadFreeRSS(context.Background(), models.CMCT, &siteDummy{}, models.RSSConfig{URL: "://bad"})
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
func (s *disabledSite) SendTorrentToQbit(ctx context.Context, rssCfg models.RSSConfig) error {
	return nil
}
func (s *disabledSite) Context() context.Context { return context.Background() }
func TestFetchRSS_Errors(t *testing.T) {
	global.GlobalDB = nil
	if err := FetchAndDownloadFreeRSS(context.Background(), models.CMCT, &disabledSite{}, models.RSSConfig{URL: "http://example"}); err == nil {
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
	if err := FetchAndDownloadFreeRSS(context.Background(), models.CMCT, &disabledSite{}, models.RSSConfig{URL: "http://example"}); err == nil {
		t.Fatalf("expected error for blank download dir")
	}
	if err := db.DB.Where("1=1").Delete(&models.SettingsGlobal{}).Error; err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if err := core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true}); err != nil {
		t.Fatalf("save gl: %v", err)
	}
	if err := FetchAndDownloadFreeRSS(context.Background(), models.CMCT, &disabledSite{}, models.RSSConfig{URL: "http://invalid"}); err == nil {
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
	if err := FetchAndDownloadFreeRSS(context.Background(), models.CMCT, m, models.RSSConfig{Name: "r", URL: srv.URL, Tag: "tag"}); err != nil {
		t.Fatalf("fetch: %v", err)
	}
}
