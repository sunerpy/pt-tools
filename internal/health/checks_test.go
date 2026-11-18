package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/fsutil"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func byKey(list []Item) map[string]Item {
	out := map[string]Item{}
	for _, it := range list {
		out[it.Key] = it
	}
	return out
}

// fakeTMDB 回 /configuration（带 Date）、黑客帝国的详情与图片；status 不为 200 时接口都回它，imageStatus 同理。
type fakeTMDB struct {
	srv         *httptest.Server
	date        atomic.Value
	status      atomic.Int32
	imageStatus atomic.Int32
	imagePath   atomic.Value
}

func newFakeTMDB(t *testing.T) *fakeTMDB {
	f := &fakeTMDB{}
	f.status.Store(http.StatusOK)
	f.imageStatus.Store(http.StatusOK)
	f.date.Store("")
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/t/p/") {
			f.imagePath.Store(r.URL.Path)
			if s := int(f.imageStatus.Load()); s != http.StatusOK {
				w.WriteHeader(s)
				return
			}
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte{0xff, 0xd8, 0xff})
			return
		}
		if d := f.date.Load().(string); d == "-" {
			w.Header()["Date"] = nil
		} else if d != "" {
			w.Header().Set("Date", d)
		}
		if s := int(f.status.Load()); s != http.StatusOK {
			w.WriteHeader(s)
			return
		}
		switch r.URL.Path {
		case "/3/configuration":
			_, _ = w.Write([]byte(`{"images":{}}`))
		case "/3/movie/603":
			_, _ = w.Write([]byte(`{"id":603,"title":"黑客帝国","poster_path":"/matrix.jpg"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeTMDB) client(t *testing.T) func(context.Context) (*tmdb.Client, error) {
	c, err := tmdb.New(tmdb.Options{
		APIKey: "0123456789abcdef0123456789abcdef", BaseURL: f.srv.URL + "/3", ImageBaseURL: f.srv.URL + "/t/p", RatePerSecond: 1000,
	})
	require.NoError(t, err)
	return func(context.Context) (*tmdb.Client, error) { return c, nil }
}

func TestTMDBCheck(t *testing.T) {
	f := newFakeTMDB(t)
	now := time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	noPoster := func(context.Context) string { return "" }
	run := func(c Check) map[string]Item {
		got := c.Run(context.Background())
		require.Len(t, got, 3)
		return byKey(got)
	}

	// 都正常：订阅里没有海报时拿黑客帝国的海报试；时间相差 30 秒
	f.date.Store(now.Add(-30 * time.Second).Format(http.TimeFormat))
	got := run(TMDB(f.client(t), noPoster, clock))
	assert.Equal(t, Item{Key: "tmdb", Group: GroupTMDB, Title: "TMDB 接口", Status: StatusOK, Detail: "API Key 可用"}, got["tmdb"])
	assert.Equal(t, StatusOK, got["tmdb:image"].Status)
	assert.Equal(t, "/t/p/w92/matrix.jpg", f.imagePath.Load())
	assert.Equal(t, Item{Key: "time", Group: GroupTime, Title: "本机时间", Status: StatusOK, Detail: "和 TMDB 的时间相差 30 秒"}, got["time"])

	// 订阅里的海报优先；本机快 10 分钟、慢 2 小时都提醒
	f.date.Store(now.Add(-10 * time.Minute).Format(http.TimeFormat))
	got = run(TMDB(f.client(t), func(context.Context) string { return "/sub.jpg" }, clock))
	assert.Equal(t, "/t/p/w92/sub.jpg", f.imagePath.Load())
	assert.Equal(t, StatusWarn, got["time"].Status)
	assert.Equal(t, "本机时间比 TMDB 的快 10 分钟", got["time"].Detail)
	assert.Contains(t, got["time"].Hint, "NTP")
	f.date.Store(now.Add(2*time.Hour + time.Minute).Format(http.TimeFormat))
	got = run(TMDB(f.client(t), noPoster, clock))
	assert.Equal(t, "本机时间比 TMDB 的慢 2 小时", got["time"].Detail)
	f.date.Store(now.Add(-4 * time.Minute).Format(http.TimeFormat))
	assert.Equal(t, "和 TMDB 的时间相差 4 分钟", run(TMDB(f.client(t), noPoster, clock))["time"].Detail, "5 分钟以内不算不对")

	// 响应里没有时间；图片取不到
	f.date.Store("-")
	f.imageStatus.Store(http.StatusBadGateway)
	got = run(TMDB(f.client(t), noPoster, clock))
	assert.Equal(t, StatusSkip, got["time"].Status)
	assert.Equal(t, "TMDB 的响应里没有时间，没法比", got["time"].Detail)
	assert.Equal(t, StatusWarn, got["tmdb:image"].Status)
	assert.Contains(t, got["tmdb:image"].Detail, "HTTP 502")
	assert.Contains(t, got["tmdb:image"].Hint, "图片地址")
	f.imageStatus.Store(http.StatusOK)

	// 接口出错：Key 无效、限流、连不上；订阅有海报时照样试图片
	f.status.Store(http.StatusUnauthorized)
	got = run(TMDB(f.client(t), noPoster, clock))
	assert.Equal(t, StatusFail, got["tmdb"].Status)
	assert.Equal(t, "API Key 无效", got["tmdb"].Detail)
	assert.Equal(t, Item{Key: "tmdb:image", Group: GroupTMDB, Title: "TMDB 图片", Status: StatusSkip, Detail: "没有拿来试的海报"}, got["tmdb:image"])
	assert.Equal(t, "TMDB 接口出错，没法比本机时间", got["time"].Detail)
	f.status.Store(http.StatusTooManyRequests)
	got = run(TMDB(f.client(t), func(context.Context) string { return "/sub.jpg" }, clock))
	assert.Equal(t, StatusWarn, got["tmdb"].Status)
	assert.Equal(t, StatusOK, got["tmdb:image"].Status)
	f.status.Store(http.StatusBadGateway)
	got = run(TMDB(f.client(t), noPoster, clock))
	assert.Equal(t, StatusFail, got["tmdb"].Status)
	assert.Contains(t, got["tmdb"].Detail, "HTTP 502")
	assert.Contains(t, got["tmdb"].Hint, "代理")

	// 建不了客户端：没有 Key 只提醒，别的错误算失败；图片与时间都跳过
	got = run(TMDB(func(context.Context) (*tmdb.Client, error) { return nil, tmdb.ErrNoKey }, noPoster, clock))
	assert.Equal(t, StatusWarn, got["tmdb"].Status)
	assert.Contains(t, got["tmdb"].Hint, "API Key")
	assert.Equal(t, StatusSkip, got["tmdb:image"].Status)
	assert.Equal(t, StatusSkip, got["time"].Status)
	got = run(TMDB(func(context.Context) (*tmdb.Client, error) { return nil, errors.New("解密代理地址失败") }, noPoster, clock))
	assert.Equal(t, StatusFail, got["tmdb"].Status)
	assert.Equal(t, "解密代理地址失败", got["tmdb"].Detail)
}

func TestProbeLabels(t *testing.T) {
	assert.Equal(t, "会话已过期", ProbeLabel("SESSION_EXPIRED"))
	assert.Equal(t, "未知状态", ProbeLabel("NEW_STATE"))
	assert.Contains(t, AbnormalProbes(), "NETWORK_ERROR")
	assert.NotContains(t, AbnormalProbes(), "NOT_CONFIGURED")
	list := AbnormalProbes()
	list[0] = "x"
	assert.Equal(t, "SESSION_EXPIRED", AbnormalProbes()[0], "给出去的是副本")
}

func TestSpanAndShort(t *testing.T) {
	assert.Equal(t, "0 秒", span(0))
	assert.Equal(t, "59 秒", span(59*time.Second))
	assert.Equal(t, "1 分钟", span(time.Minute))
	assert.Equal(t, "3 小时", span(3*time.Hour+59*time.Minute))
	assert.Equal(t, "", short("  "))
	long := strings.Repeat("错", maxDetail+10)
	assert.Equal(t, strings.Repeat("错", maxDetail)+"…", short(long))
	assert.NotContains(t, short("GET https://pt.example/rss?passkey=0123456789abcdef0123456789abcdef failed"), "0123456789abcdef")
}

func TestSitesCheck(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	assert.Equal(t, []Item{{Key: "sites", Group: GroupSites, Title: "站点", Status: StatusSkip, Detail: "没有启用的站点"}}, Sites(db).Run(ctx))

	v2.GetDefinitionRegistry().Register(&v2.SiteDefinition{ID: "healthtestsite", Name: "体检测试站"})
	at := time.Date(2026, 11, 4, 8, 30, 0, 0, time.Local)
	for _, s := range []models.SiteSetting{
		{Name: "aa", DisplayName: "甲站", Enabled: true, AuthMethod: "cookie"},
		{Name: "bb", Enabled: true, AuthMethod: "cookie"},
		{Name: "cc", Enabled: true, AuthMethod: "cookie"},
		{Name: "dd", Enabled: true, AuthMethod: "cookie"},
		{Name: "ee", Enabled: true, AuthMethod: "cookie"},
		{Name: "ff", Enabled: true, AuthMethod: "cookie"},
		{Name: "healthtestsite", Enabled: true, AuthMethod: "cookie"},
		{Name: "off", Enabled: false, AuthMethod: "cookie"},
	} {
		require.NoError(t, db.Create(&s).Error)
	}
	for _, st := range []models.SiteLoginState{
		{SiteName: "aa", LastProbeAt: &at, LastProbeStatus: "OK"},
		{SiteName: "bb", LastProbeAt: &at, LastProbeStatus: "SESSION_EXPIRED", LastProbeError: "GET https://bb.example/index.php?passkey=0123456789abcdef0123456789abcdef 302"},
		{SiteName: "cc", LastProbeAt: &at, LastProbeStatus: "NETWORK_ERROR", ProbeMode: probeModeDisabled},
		{SiteName: "ee", LastProbeAt: &at, LastProbeStatus: "UNSUPPORTED"},
		{SiteName: "FF", LastProbeAt: &at, LastProbeStatus: "XYZ"},
		{SiteName: "healthtestsite", LastProbeAt: &at, LastProbeStatus: "NOT_CONFIGURED"},
		{SiteName: "off", LastProbeAt: &at, LastProbeStatus: "KEY_ERROR"},
	} {
		require.NoError(t, db.Create(&st).Error)
	}
	got := Sites(db).Run(ctx)
	require.Len(t, got, 7, "关掉的站点不列")
	m := byKey(got)
	assert.Equal(t, Item{Key: "site:aa", Group: GroupSites, Title: "甲站", Status: StatusOK, Detail: "最近一次探测（11-04 08:30）正常"}, m["site:aa"])
	assert.Equal(t, StatusWarn, m["site:bb"].Status)
	assert.True(t, strings.HasPrefix(m["site:bb"].Detail, "最近一次探测（11-04 08:30）会话已过期：GET https://bb.example/index.php"), m["site:bb"].Detail)
	assert.NotContains(t, m["site:bb"].Detail, "0123456789abcdef", "错误里的凭证遮掉")
	assert.Contains(t, m["site:bb"].Hint, "Cookie 过期")
	assert.Equal(t, "bb", m["site:bb"].Title, "没有显示名、站点定义里也没有时用编号")
	assert.Equal(t, Item{Key: "site:cc", Group: GroupSites, Title: "cc", Status: StatusSkip, Detail: "登录探测关着"}, m["site:cc"])
	assert.Equal(t, Item{Key: "site:dd", Group: GroupSites, Title: "dd", Status: StatusSkip, Detail: "没探测过登录状态"}, m["site:dd"])
	assert.Equal(t, Item{Key: "site:ee", Group: GroupSites, Title: "ee", Status: StatusSkip, Detail: "暂不支持探测"}, m["site:ee"])
	assert.Equal(t, StatusWarn, m["site:ff"].Status, "登录状态的站点名不分大小写")
	assert.Equal(t, "最近一次探测（11-04 08:30）未知状态", m["site:ff"].Detail)
	assert.Equal(t, probeHint["UNKNOWN"], m["site:ff"].Hint)
	assert.Equal(t, "体检测试站", m["site:healthtestsite"].Title, "站点定义里的名字")
	assert.Equal(t, StatusWarn, m["site:healthtestsite"].Status)
	assert.Contains(t, m["site:healthtestsite"].Hint, "没填 Cookie")

	require.NoError(t, db.Migrator().DropTable(&models.SiteLoginState{}))
	got = Sites(db).Run(ctx)
	require.Len(t, got, 1)
	assert.Equal(t, StatusFail, got[0].Status)
	assert.Contains(t, got[0].Detail, "读取登录状态失败")
	require.NoError(t, db.Migrator().DropTable(&models.SiteSetting{}))
	assert.Contains(t, Sites(db).Run(ctx)[0].Detail, "读取站点失败")
}

type fakeClient struct {
	version    string
	versionErr error
	free       int64
	freeErr    error
	block      chan struct{}
}

func (f *fakeClient) GetClientVersion() (string, error) {
	if f.block != nil {
		<-f.block
	}
	return f.version, f.versionErr
}

func (f *fakeClient) GetClientFreeSpace(context.Context) (int64, error) { return f.free, f.freeErr }

func TestDownloadersCheck(t *testing.T) {
	ctx := context.Background()
	const gb = 1 << 30
	client := func(c DownloaderClient, err error) func(context.Context) (DownloaderClient, error) {
		return func(context.Context) (DownloaderClient, error) { return c, err }
	}
	targets := []DownloaderTarget{
		{ID: 1, Name: "qb", Get: client(&fakeClient{version: "v5.0.4", free: 500 * gb}, nil)},
		{ID: 2, Name: "tr", Get: client(&fakeClient{version: "4.0.6", free: 10 * gb}, nil)},
		{ID: 3, Name: "qb2", Get: client(&fakeClient{version: "v4.6.7", freeErr: errors.New("403")}, nil)},
		{ID: 4, Name: "qb3", Get: client(&fakeClient{versionErr: errors.New("connection refused")}, nil)},
		{ID: 5, Name: "qb4", Get: client(nil, errors.New("认证失败"))},
		{ID: 6, Name: "qb5"},
		{ID: 7, Name: "qb6", Get: client(&fakeClient{free: -1}, nil)},
		{ID: 8, Name: "qb7", Get: client(nil, nil)},
	}
	got := Downloaders(func(context.Context) ([]DownloaderTarget, error) { return targets, nil }).Run(ctx)
	require.Len(t, got, 8)
	assert.Equal(t, Item{Key: "downloader:1", Group: GroupDownloaders, Title: "qb", Status: StatusOK, Detail: "v5.0.4 · 剩余 500.00 GiB"}, got[0], "顺序和列表一样")
	assert.Equal(t, StatusWarn, got[1].Status)
	assert.Equal(t, "4.0.6 · 剩余 10.00 GiB", got[1].Detail)
	assert.Contains(t, got[1].Hint, "快满了")
	assert.Equal(t, StatusWarn, got[2].Status)
	assert.Equal(t, "v4.6.7 · 读不到剩余空间：403", got[2].Detail)
	assert.Equal(t, Item{
		Key: "downloader:4", Group: GroupDownloaders, Title: "qb3", Status: StatusFail, Detail: "连不上：connection refused",
		Hint: "检查下载器是不是开着，以及地址、用户名与密码",
	}, got[3])
	assert.Equal(t, "连不上：认证失败", got[4].Detail)
	assert.Equal(t, "连不上：取不到下载器", got[5].Detail)
	assert.Equal(t, "剩余 0 B", got[6].Detail, "没有版本时只写剩余空间；负数当 0")
	assert.Equal(t, StatusWarn, got[6].Status)
	assert.Equal(t, "连不上：取不到下载器", got[7].Detail)

	got = Downloaders(func(context.Context) ([]DownloaderTarget, error) { return nil, nil }).Run(ctx)
	assert.Equal(t, StatusWarn, got[0].Status)
	assert.Contains(t, got[0].Detail, "没有启用的下载器")
	got = Downloaders(func(context.Context) ([]DownloaderTarget, error) { return nil, errors.New("库坏了") }).Run(ctx)
	assert.Equal(t, Item{Key: "downloaders", Group: GroupDownloaders, Title: "下载器", Status: StatusFail, Detail: "读取下载器失败：库坏了"}, got[0])
}

func TestWithin(t *testing.T) {
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	_, err := within(context.Background(), 20*time.Millisecond, func(context.Context) (int, error) {
		<-block
		return 1, nil
	})
	require.EqualError(t, err, "0 秒内没有回应")
	v, err := within(context.Background(), time.Second, func(context.Context) (int, error) { return 7, nil })
	require.NoError(t, err)
	assert.Equal(t, 7, v)
	assert.Equal(t, "a · b", joinDetail(" a ", "", "b"))
}

func TestServersCheck(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	test := func(_ context.Context, srv models.MediaServer) (ServerInfo, error) {
		if srv.ID == 4 {
			return ServerInfo{}, errors.New("401 Unauthorized")
		}
		return ServerInfo{Version: "10.11.2"}, nil
	}
	assert.Equal(t, StatusSkip, Servers(db, test).Run(ctx)[0].Status)

	at := time.Date(2026, 11, 4, 3, 0, 0, 0, time.Local)
	for _, s := range []models.MediaServer{
		{ID: 1, Name: "家里的 Jellyfin", Kind: models.MediaServerJellyfin, Enabled: true, IndexedAt: &at, IndexMovies: 120, IndexSeries: 30},
		{ID: 2, Name: "Emby", Kind: models.MediaServerEmby, Enabled: true, IndexError: "HTTP 500"},
		{ID: 3, Name: "Plex", Kind: models.MediaServerPlex, Enabled: true, IndexOff: true, IndexError: "旧的"},
		{ID: 4, Name: "坏的", Kind: "other", Enabled: true},
		{ID: 5, Name: "关着", Kind: models.MediaServerEmby, Enabled: false},
	} {
		require.NoError(t, db.Create(&s).Error)
	}
	got := Servers(db, test).Run(ctx)
	require.Len(t, got, 4)
	assert.Equal(t, Item{Key: "server:1", Group: GroupServers, Title: "家里的 Jellyfin", Status: StatusOK, Detail: "Jellyfin 10.11.2 · 索引 120 部电影、30 部剧，11-04 03:00 同步"}, got[0])
	assert.Equal(t, StatusWarn, got[1].Status)
	assert.Equal(t, "Emby 10.11.2 · 上次同步索引失败：HTTP 500", got[1].Detail)
	assert.Contains(t, got[1].Hint, "同步")
	assert.Equal(t, Item{Key: "server:3", Group: GroupServers, Title: "Plex", Status: StatusOK, Detail: "Plex 10.11.2"}, got[2], "不同步索引时不看索引的错误")
	assert.Equal(t, StatusFail, got[3].Status)
	assert.Equal(t, "连不上：401 Unauthorized", got[3].Detail)

	require.NoError(t, db.Migrator().DropTable(&models.MediaServer{}))
	assert.Equal(t, StatusFail, Servers(db, test).Run(ctx)[0].Status)
}

func TestPathMapsCheck(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	assert.Equal(t, StatusSkip, PathMaps(db).Run(ctx)[0].Status)

	dir := t.TempDir()
	ok := filepath.Join(dir, "downloads")
	require.NoError(t, os.Mkdir(ok, 0o755))
	file := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))
	require.NoError(t, db.Create(&models.DownloaderSetting{ID: 1, Name: "qb", Type: "qbittorrent", URL: "http://qb"}).Error)
	for _, m := range []models.MediaPathMap{
		{ID: 1, DownloaderID: 1, DownloaderPrefix: "/downloads", LocalPrefix: ok},
		{ID: 2, DownloaderID: 1, DownloaderPrefix: "/missing", LocalPrefix: filepath.Join(dir, "none")},
		{ID: 3, DownloaderID: 9, DownloaderPrefix: "/file", LocalPrefix: file},
	} {
		require.NoError(t, db.Create(&m).Error)
	}
	got := PathMaps(db).Run(ctx)
	require.Len(t, got, 3)
	assert.Equal(t, Item{Key: "path:1", Group: GroupPaths, Title: "下载器「qb」的路径映射", Status: StatusOK, Detail: "/downloads → " + ok}, got[0])
	assert.Equal(t, StatusFail, got[1].Status)
	assert.Equal(t, "/missing → "+filepath.Join(dir, "none")+"：pt-tools 里找不到这个目录", got[1].Detail)
	assert.Contains(t, got[1].Hint, "挂进容器")
	assert.Equal(t, "下载器「#9」的路径映射", got[2].Title, "下载器删了时写编号")
	assert.True(t, strings.HasSuffix(got[2].Detail, "：不是目录"))

	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		locked := filepath.Join(dir, "locked")
		require.NoError(t, os.Mkdir(locked, 0o000))
		t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
		assert.True(t, strings.HasPrefix(readableDir(locked), "读不了："))
	}

	require.NoError(t, db.Migrator().DropTable(&models.MediaPathMap{}))
	assert.Equal(t, StatusFail, PathMaps(db).Run(ctx)[0].Status)
}

// stubDisks 把读盘换成固定的结果（路径 → 用量；不在表里的读不到）。
func stubDisks(t *testing.T, disks map[string]fsutil.Usage) {
	t.Helper()
	old := diskUsage
	diskUsage = func(p string) (fsutil.Usage, error) {
		if u, ok := disks[p]; ok {
			return u, nil
		}
		return fsutil.Usage{}, errors.New("读不到")
	}
	t.Cleanup(func() { diskUsage = old })
}

func TestLibrariesCheck(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	var calls atomic.Int32
	saves := func(context.Context) []SavePath {
		calls.Add(1)
		return []SavePath{{Downloader: "qb", Path: "/dl"}, {Downloader: "tr", Path: "/tr"}, {Downloader: "x", Path: "/gone"}}
	}
	probes := map[string][]Probe{
		"/movies": {{Name: "库目录存在", OK: true}, {Name: "库目录可写", OK: true}},
		"/tv":     {{Name: "库目录存在", OK: true}, {Name: "库目录可写", OK: true}, {Name: "硬链接：/dl", OK: true}},
		"/anime":  {{Name: "库目录存在", OK: true}, {Name: "库目录可写", OK: true}, {Name: "硬链接：/dl", Message: "invalid cross-device link"}},
		"/ro":     {{Name: "库目录存在", OK: true}, {Name: "库目录可写", Message: "permission denied"}},
		"/none":   {{Name: "库目录存在", Message: "pt-tools 里找不到 /none"}},
	}
	check := func(_ context.Context, path, _ string) ([]Probe, error) {
		if p, ok := probes[path]; ok {
			return p, nil
		}
		return nil, errors.New("库目录要是绝对路径")
	}
	assert.Equal(t, StatusSkip, Libraries(db, check, saves).Run(ctx)[0].Status)

	stubDisks(t, map[string]fsutil.Usage{"/tv": {Device: "1"}, "/dl": {Device: "1"}, "/tr": {Device: "2"}, "/same": {Device: "1"}})
	for _, l := range []models.MediaLibrary{
		{ID: 1, Name: "电影", Path: "/movies", Mode: models.MediaModeCopy},
		{ID: 2, Name: "剧集", Path: "/tv", Mode: models.MediaModeHardlink},
		{ID: 3, Name: "动画", Path: "/anime", Mode: models.MediaModeHardlink},
		{ID: 4, Name: "只读", Path: "/ro", Mode: models.MediaModeCopy},
		{ID: 5, Name: "没挂载", Path: "/none", Mode: models.MediaModeCopy},
		{ID: 6, Name: "相对", Path: "rel", Mode: models.MediaModeCopy},
	} {
		require.NoError(t, db.Create(&l).Error)
	}
	got := Libraries(db, check, saves).Run(ctx)
	require.Len(t, got, 6)
	assert.Equal(t, Item{Key: "library:1", Group: GroupLibraries, Title: "媒体库「电影」", Status: StatusOK, Detail: "/movies"}, got[0])
	assert.Equal(t, StatusWarn, got[1].Status, "硬链接库：tr 的默认保存路径在另一块盘")
	assert.Equal(t, "/tv：和下载器「tr」的 /tr 不在同一个文件系统", got[1].Detail)
	assert.Contains(t, got[1].Hint, "改成复制")
	assert.Equal(t, StatusFail, got[2].Status)
	assert.Equal(t, "/anime：硬链接：/dl失败：invalid cross-device link", got[2].Detail)
	assert.Contains(t, got[2].Hint, "同一个文件系统")
	assert.Contains(t, got[3].Hint, "写这个目录的权限")
	assert.Contains(t, got[4].Hint, "挂进容器")
	assert.Equal(t, "rel：库目录要是绝对路径", got[5].Detail)
	assert.Equal(t, int32(1), calls.Load(), "默认保存路径只读一次")

	assert.Empty(t, otherDevice("/unknown", []SavePath{{Downloader: "qb", Path: "/tr"}}), "读不到库的盘时不比")
	assert.Empty(t, otherDevice("/tv", []SavePath{{Downloader: "qb", Path: "/same"}}))

	require.NoError(t, db.Migrator().DropTable(&models.MediaLibrary{}))
	assert.Equal(t, StatusFail, Libraries(db, check, saves).Run(ctx)[0].Status)
}

func TestDisksCheck(t *testing.T) {
	const gb = 1 << 30
	stubDisks(t, map[string]fsutil.Usage{
		"/data":  {Free: 900 * gb, Total: 1000 * gb, Device: "1"},
		"/media": {Free: 900 * gb, Total: 1000 * gb, Device: "1"},
		"/dl":    {Free: 30 * gb, Total: 1000 * gb, Device: "2"},
		"/dl2":   {Free: 30 * gb, Total: 1000 * gb, Device: "2"},
		"/nas":   {Free: 100 * gb, Device: ""},
		"/nas2":  {Free: 100 * gb, Device: ""},
	})
	got := Disks(func(context.Context) []DiskPath {
		return []DiskPath{
			{Label: "数据目录", Path: "/data"},
			{Label: "媒体库「电影」", Path: "/media"},
			{Label: "下载目录（qb）", Path: "/dl"},
			{Label: "下载目录（qb）", Path: "/dl2"},
			{Label: "空的", Path: " "},
			{Label: "读不到", Path: "/gone"},
			{Label: "网盘", Path: "/nas"},
			{Label: "网盘2", Path: "/nas2"},
		}
	}).Run(context.Background())
	require.Len(t, got, 4, "同一块盘只列一次，读不到的跳过；没有设备号的不合并")
	assert.Equal(t, Item{Key: "disk:/data", Group: GroupDisks, Title: "数据目录、媒体库「电影」", Status: StatusOK, Detail: "/data：剩余 900.00 GiB，共 1000.00 GiB"}, got[0])
	assert.Equal(t, StatusWarn, got[1].Status, "剩不到 5%")
	assert.Equal(t, "下载目录（qb）", got[1].Title, "同一块盘上同名的两个目录只写一次名字")
	assert.NotEmpty(t, got[1].Hint)
	assert.Equal(t, "/nas：剩余 100.00 GiB", got[2].Detail, "不知道总量时只写剩余")
	assert.Equal(t, "网盘2", got[3].Title)

	got = Disks(func(context.Context) []DiskPath { return []DiskPath{{Label: "读不到", Path: "/gone"}} }).Run(context.Background())
	assert.Equal(t, []Item{{Key: "disks", Group: GroupDisks, Title: "磁盘空间", Status: StatusSkip, Detail: "读不到磁盘的剩余空间"}}, got)
	assert.Equal(t, "8.00 EiB", sizeText(1<<63+5), "超过 int64 时按最大值写")
}

func TestBackupCheck(t *testing.T) {
	now := time.Date(2026, 11, 4, 9, 0, 0, 0, time.Local)
	ago := func(d time.Duration) *time.Time { v := now.Add(-d); return &v }
	run := func(st BackupState, err error) Item {
		got := Backup(func() (BackupState, error) { return st, err }, func() time.Time { return now }).Run(context.Background())
		require.Len(t, got, 1)
		return got[0]
	}
	assert.Equal(t, StatusFail, run(BackupState{}, errors.New("坏了")).Status)
	assert.Equal(t, Item{Key: "backup", Group: GroupBackup, Title: "备份", Status: StatusSkip, Detail: "每天定时备份关着", Hint: "在「全局设置 → 备份」里打开每天定时备份"}, run(BackupState{LastAt: ago(time.Hour), LastError: "x"}, nil))
	assert.Equal(t, "没做过备份", run(BackupState{Enabled: true}, nil).Detail)

	it := run(BackupState{Enabled: true, LastAt: ago(5 * time.Hour), LastError: "目录写不进去", LastOKAt: ago(30 * time.Hour)}, nil)
	assert.Equal(t, StatusWarn, it.Status)
	assert.Equal(t, "最近一次备份（11-04 04:00）失败：目录写不进去", it.Detail)
	it = run(BackupState{Enabled: true, LastAt: ago(73 * time.Hour), LastOKAt: ago(73 * time.Hour), LastName: "a.tar.gz"}, nil)
	assert.Equal(t, StatusWarn, it.Status)
	assert.Equal(t, "3 天没备份成功了", it.Detail)
	it = run(BackupState{Enabled: true, LastAt: ago(5 * time.Hour), LastOKAt: ago(5 * time.Hour), LastName: "b.tar.gz"}, nil)
	assert.Equal(t, Item{Key: "backup", Group: GroupBackup, Title: "备份", Status: StatusOK, Detail: "最近一次 11-04 04:00 成功（b.tar.gz）"}, it)
	it = run(BackupState{Enabled: true, LastAt: ago(5 * time.Hour)}, nil)
	assert.Equal(t, "最近一次 11-04 04:00 成功", it.Detail, "旧版本没记成功时间：最近一次没出错就是成功的那次")

	days, late := BackupLate(BackupState{Enabled: true, LastAt: ago(time.Hour), LastError: "x"}, now)
	assert.Equal(t, -1, days, "从没成功过")
	assert.True(t, late)
	days, late = BackupLate(BackupState{Enabled: true, LastAt: ago(47 * time.Hour), LastOKAt: ago(47 * time.Hour)}, now)
	assert.Equal(t, 1, days)
	assert.False(t, late, "两天以内不提醒")
	_, late = BackupLate(BackupState{LastAt: ago(100 * time.Hour), LastOKAt: ago(100 * time.Hour)}, now)
	assert.False(t, late, "定时备份关着时不提醒")
}

func TestSecretCheck(t *testing.T) {
	run := func(path string, generated bool, err error) Item {
		got := Secret(func() (string, bool, error) { return path, generated, err }).Run(context.Background())
		require.Len(t, got, 1)
		return got[0]
	}
	file := filepath.Join(t.TempDir(), "secret.key")
	require.NoError(t, os.WriteFile(file, []byte("k"), 0o600))
	it := run(file, false, errors.New("格式不对"))
	assert.Equal(t, StatusFail, it.Status)
	assert.Equal(t, file+" 读不出来：格式不对", it.Detail)
	assert.Contains(t, it.Hint, "secret import")
	assert.Equal(t, Item{Key: "secret", Group: GroupSecret, Title: "加密密钥", Status: StatusOK, Detail: "用的是环境变量 PT_TOOLS_SECRET_KEY"}, run("", false, nil))
	assert.Equal(t, "密钥在 "+file, run(file, false, nil).Detail)
	assert.Equal(t, "密钥在 "+file+"（这次启动时新生成的）", run(file, true, nil).Detail)
	it = run(filepath.Join(t.TempDir(), "gone.key"), false, nil)
	assert.Equal(t, StatusFail, it.Status)
	assert.Contains(t, it.Detail, "不见了")
	assert.Contains(t, it.Hint, "带上密钥")
}
