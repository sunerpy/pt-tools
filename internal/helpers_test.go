// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeebo/bencode"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	_ "github.com/sunerpy/pt-tools/site/v2/definitions"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

func writeStagingTorrent(t *testing.T, dir, name string) (string, string) {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": name}}))
	p := filepath.Join(dir, "springsunday-"+name+".torrent")
	require.NoError(t, os.WriteFile(p, buf.Bytes(), 0o644))
	h := hashOfBytes(t, buf.Bytes())
	return p, h
}

func hashOfBytes(t *testing.T, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "tmp.torrent")
	require.NoError(t, os.WriteFile(p, data, 0o644))
	return computeHash(t, p)
}

func computeHash(t *testing.T, path string) string {
	t.Helper()
	h, err := qbit.ComputeTorrentHashWithPath(path)
	require.NoError(t, err)
	return h
}

type siteFail struct{}

type rssSiteStub struct{}

// linkURLSiteStub 记录传入 DownloadTorrent 的 url，用于断言无 enclosure 时回退 item.Link
type linkURLSiteStub struct{ gotURL string }

type props404Transport struct{}

type props200Transport struct{}

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

type roundTripFunc func(*http.Request) (*http.Response, error)

type siteDummy struct{}

type disabledSite struct{}

func makeFeedServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	buf := bytes.NewBufferString(body)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); _, _ = w.Write(buf.Bytes()) }))
}

// legacyMinFreeStub returns a free torrent whose EndTime is close enough that
// CanbeFinished + MinFreeMinutes forces a skip.
type legacyMinFreeStub struct{ legacyPTStub }

func addResultFailServer(t *testing.T, freeSpace int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case r.URL.Path == "/api/v2/app/version":
			_, _ = w.Write([]byte("v4.6.0"))
		case strings.HasPrefix(r.URL.Path, "/api/v2/torrents/properties"):
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == "/api/v2/sync/maindata":
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":` + itoa(freeSpace) + `}}`))
		case r.URL.Path == "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/api/v2/torrents/add":
			w.WriteHeader(http.StatusConflict)
		default:
			_, _ = w.Write([]byte("{}"))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

type checkExistsErrTransport struct{}

func badJSONServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{not-json`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func itemXMLWithCategory(guid, title, enclosure, category string) string {
	return fmt.Sprintf(`<item><title>%s</title><guid>%s</guid><category>%s</category>`+
		`<enclosure url="%s" type="application/x-bittorrent"/></item>`, title, guid, category, enclosure)
}

func itemXMLLinkOnly(guid, title, link string) string {
	return fmt.Sprintf(`<item><title>%s</title><guid>%s</guid><link>%s</link></item>`, title, guid, link)
}

var _ = assert.True

func seedQbitDownloaderNamed(t *testing.T, name, url string) uint {
	t.Helper()
	ds := models.DownloaderSetting{
		Name: name, Type: "qbittorrent", URL: url,
		Username: "admin", Password: "pw", Enabled: true, AutoStart: true,
	}
	require.NoError(t, global.GlobalDB.DB.Create(&ds).Error)
	return ds.ID
}

func boolPtrIX(b bool) *bool { return &b }

var _ = strings.Contains

type addFailTransport struct{}

type existsTransport struct{}

var assertDLErr = &dlGenericErr{"disk read boom"}

type dlGenericErr struct{ s string }

func mustSkip(ti *models.TorrentInfo, path, base string, mr int) bool {
	s, _ := shouldSkipSiteDownload(ti, path, base, mr)
	return s
}

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

const gb = int64(1024 * 1024 * 1024)

// resetGlobalBudget 把全局 budget 清零，避免测试之间互相污染。
func resetGlobalBudget() { GetDiskBudget().Reset() }

type ptStub struct{}

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

type recordingNotifier struct {
	newItems      atomic.Int32
	filteredItems atomic.Int32
}

func boolPtrFW(b bool) *bool { return &b }

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

type stubRSSNotifier struct{}

// addOKTransport answers auth + properties(404) + maindata + add(200) so
// processSingleTorrent reaches and completes the successful push branch.
type addOKTransport struct{}

func boolPtrPS(b bool) *bool { return &b }

var _ = filepath.Base

type noopPT struct{}

// makeSizedTorrentBytes builds a valid single-file .torrent payload of the
// given content length so ComputeTorrentSize returns sizeBytes.
func makeSizedTorrentBytes(t *testing.T, name string, sizeBytes int64) []byte {
	t.Helper()
	var buf bytes.Buffer
	torrent := map[string]any{
		"info": map[string]any{"name": name, "length": sizeBytes, "piece length": 16384},
	}
	require.NoError(t, bencode.NewEncoder(&buf).Encode(torrent))
	return buf.Bytes()
}

// fakeQbitServer answers the auth + properties + add + info endpoints
// PushTorrentToDownloader touches. `exists` toggles CheckTorrentExists.
func fakeQbitServer(t *testing.T, exists bool, freeSpace int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case r.URL.Path == "/api/v2/app/version":
			_, _ = w.Write([]byte("v4.6.0"))
		case strings.HasPrefix(r.URL.Path, "/api/v2/torrents/properties"):
			if exists {
				_, _ = w.Write([]byte(`{"save_path":"/downloads"}`))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		case r.URL.Path == "/api/v2/sync/maindata":
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":` + itoa(freeSpace) + `}}`))
		case r.URL.Path == "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/api/v2/torrents/add":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}

func seedQbitDownloader(t *testing.T, url string) uint {
	t.Helper()
	ds := models.DownloaderSetting{
		Name: "qb", Type: "qbittorrent", URL: url,
		Username: "admin", Password: "pw", Enabled: true, AutoStart: true,
	}
	require.NoError(t, global.GlobalDB.DB.Create(&ds).Error)
	return ds.ID
}

func setupFactoryTestDB(t *testing.T) func() {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.InitLogger(zap.NewNop())
	global.GlobalDB = db
	return func() {}
}

func boolPtrTD(b bool) *bool { return &b }

func nexusDetailServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><body>
			<input name="torrent_name" value="My.Movie.2024">
			<input name="detail_torrent_id" value="42">
			<h1><font class="free">免费</font><span title="2026-01-20 15:30:00">2天</span></h1>
			<td class="rowhead">基本信息</td><td>大小：16.87 GB</td>
		</body></html>`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func setupUnifiedDB(t *testing.T) *models.TorrentDB {
	t.Helper()
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })
	return db
}

func boolPtrUS(b bool) *bool { return &b }

func setupTestDB(t *testing.T) func() {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.InitLogger(zap.NewNop())
	global.GlobalDB = db
	return func() {
		// cleanup if needed
	}
}
