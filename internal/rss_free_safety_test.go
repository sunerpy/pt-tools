package internal

import (
	"context"
	"errors"
	"fmt"
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

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	sm "github.com/sunerpy/pt-tools/mocks"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

// 免费资格安全（docs/design/free-expiry-safety.md）在 RSS 两阶段上的回归测试。
// 每一轮都和生产一样：同一个开始时间下先取详情、下载暂存文件，再推送这条 RSS 的暂存文件。

// errNoDetail 是假站点读不到详情时的错误。
var errNoDetail = errors.New("找不到种子详情")

// roundSite 是按轮次改详情的假站点。下载的 .torrent 按编号区分内容，返回真实哈希。
type roundSite struct {
	mu          sync.Mutex
	details     map[string]*v2.TorrentItem
	errs        map[string]error
	detailCalls map[string]int
}

func newRoundSite() *roundSite {
	return &roundSite{details: map[string]*v2.TorrentItem{}, errs: map[string]error{}, detailCalls: map[string]int{}}
}

func (s *roundSite) set(id string, d v2.TorrentItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d.ID = id
	if d.Title == "" {
		d.Title = "T" + id
	}
	if d.SizeBytes == 0 {
		d.SizeBytes = gb
	}
	s.details[id] = &d
	delete(s.errs, id)
}

func (s *roundSite) fail(id string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errs[id] = err
}

func (s *roundSite) calls(id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.detailCalls[id]
}

func (s *roundSite) GetTorrentDetailsContext(_ context.Context, item *gofeed.Item) (*v2.TorrentItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.detailCalls[item.GUID]++
	if err := s.errs[item.GUID]; err != nil {
		return nil, err
	}
	d, ok := s.details[item.GUID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errNoDetail, item.GUID)
	}
	cp := *d
	return &cp, nil
}

func (s *roundSite) GetTorrentDetails(item *gofeed.Item) (*v2.TorrentItem, error) {
	return s.GetTorrentDetailsContext(context.Background(), item)
}

func (s *roundSite) IsEnabled() bool { return true }

func (s *roundSite) DownloadTorrent(_, title, downloadDir string) (string, error) {
	data := roundTorrentBytes(title)
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(downloadDir, title+".torrent"), data, 0o644); err != nil {
		return "", err
	}
	return qbit.ComputeTorrentHash(data)
}

func (s *roundSite) MaxRetries() int { return 1 }

func (s *roundSite) RetryDelay() time.Duration { return time.Millisecond }

func (s *roundSite) SendTorrentToDownloader(context.Context, models.RSSConfig) error { return nil }

func (s *roundSite) Context() context.Context { return context.Background() }

func (s *roundSite) SiteGroup() models.SiteGroup { return "springsunday" }

var _ UnifiedPTSite = (*roundSite)(nil)

// roundTorrentBytes 是一个按名字区分的单文件 .torrent（1 GiB）。
func roundTorrentBytes(name string) []byte {
	return fmt.Appendf(nil, "d4:infod6:lengthi%de4:name%d:%s12:piece lengthi16384eee", gb, len(name), name)
}

// feedServer 是条目可以按轮次改的 RSS 源。
type feedServer struct {
	mu    sync.Mutex
	items []string
	srv   *httptest.Server
}

func newFeedServer(t *testing.T, ids ...string) *feedServer {
	t.Helper()
	f := &feedServer{items: ids}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		var b strings.Builder
		for _, id := range f.items {
			fmt.Fprintf(&b, `<item><title>T%s</title><guid>%s</guid><link>http://x/details.php?id=%s</link>`+
				`<enclosure url="http://x/download.php?id=%s" type="application/x-bittorrent"/></item>`, id, id, id, id)
		}
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(rssBody(b.String())))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *feedServer) setItems(ids ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items = ids
}

// fakeDL 是记下加入、开始与暂停调用的下载器。free 是可用空间。
type fakeDL struct {
	*sm.MockDownloader
	mu      sync.Mutex
	free    int64
	adds    []downloader.AddTorrentOptions
	added   map[string]bool
	addedAt map[string]time.Time
	resumes []string
	// dupOnAdd：预检查不到，加入时下载器才说已经有了（Transmission 会这样）
	dupOnAdd map[string]bool
}

func newFakeDL(t *testing.T, name string) *fakeDL {
	t.Helper()
	ctrl := gomock.NewController(t)
	m := sm.NewMockDownloader(ctrl)
	d := &fakeDL{MockDownloader: m, free: 1 << 50, added: map[string]bool{}, addedAt: map[string]time.Time{}}
	m.EXPECT().GetName().Return(name).AnyTimes()
	m.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()
	m.EXPECT().Close().Return(nil).AnyTimes()
	m.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil).AnyTimes()
	m.EXPECT().GetAllTorrents().Return(nil, nil).AnyTimes()
	m.EXPECT().GetClientFreeSpace(gomock.Any()).DoAndReturn(func(context.Context) (int64, error) {
		d.mu.Lock()
		defer d.mu.Unlock()
		return d.free, nil
	}).AnyTimes()
	m.EXPECT().CheckTorrentExists(gomock.Any()).DoAndReturn(func(h string) (bool, error) {
		d.mu.Lock()
		defer d.mu.Unlock()
		return d.added[h], nil
	}).AnyTimes()
	m.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).DoAndReturn(func(data []byte, opt downloader.AddTorrentOptions) (downloader.AddTorrentResult, error) {
		h, err := qbit.ComputeTorrentHash(data)
		if err != nil {
			return downloader.AddTorrentResult{}, err
		}
		d.mu.Lock()
		defer d.mu.Unlock()
		d.adds = append(d.adds, opt)
		if d.dupOnAdd[h] {
			return downloader.AddTorrentResult{Success: true, Duplicate: true, Hash: h}, nil
		}
		d.added[h] = true
		d.addedAt[h] = time.Now()
		return downloader.AddTorrentResult{Success: true, Hash: h}, nil
	}).AnyTimes()
	m.EXPECT().GetTorrent(gomock.Any()).DoAndReturn(func(h string) (downloader.Torrent, error) {
		d.mu.Lock()
		defer d.mu.Unlock()
		if !d.added[h] {
			return downloader.Torrent{}, downloader.ErrTorrentNotFound
		}
		return downloader.Torrent{ID: h, InfoHash: h, DateAdded: d.addedAt[h].Unix(), State: downloader.TorrentPaused}, nil
	}).AnyTimes()
	m.EXPECT().ResumeTorrent(gomock.Any()).DoAndReturn(func(id string) error {
		d.mu.Lock()
		defer d.mu.Unlock()
		d.resumes = append(d.resumes, id)
		return nil
	}).AnyTimes()
	return d
}

func (d *fakeDL) setFree(n int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.free = n
}

func (d *fakeDL) addCalls() []downloader.AddTorrentOptions {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]downloader.AddTorrentOptions(nil), d.adds...)
}

func (d *fakeDL) resumeCalls() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.resumes...)
}

// rssRig 是真实数据库 + 假站点 + 假下载器（经 DownloaderManager 按名字取）。
type rssRig struct {
	t    *testing.T
	db   *models.TorrentDB
	site *roundSite
	dls  map[string]*fakeDL
	base string
}

func newRSSRig(t *testing.T, dlNames ...string) *rssRig {
	t.Helper()
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	GetDiskBudget().Reset()
	t.Cleanup(func() { GetDiskBudget().Reset() })
	gl, err := core.NewConfigStore(db).GetGlobalOnly()
	require.NoError(t, err)
	r := &rssRig{t: t, db: db, site: newRoundSite(), dls: map[string]*fakeDL{}, base: gl.DownloadDir}
	if len(dlNames) == 0 {
		dlNames = []string{"qb"}
	}
	dm := downloader.NewDownloaderManager()
	dm.RegisterFactory(downloader.DownloaderQBittorrent, func(_ downloader.DownloaderConfig, name string) (downloader.Downloader, error) {
		if d, ok := r.dls[name]; ok {
			return d, nil
		}
		return nil, fmt.Errorf("没有下载器 %s", name)
	})
	SetGlobalDownloaderManager(dm)
	t.Cleanup(func() { SetGlobalDownloaderManager(nil) })
	for i, name := range dlNames {
		r.dls[name] = newFakeDL(t, name)
		ds := models.DownloaderSetting{Name: name, Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true, AutoStart: true, IsDefault: i == 0}
		require.NoError(t, db.DB.Create(&ds).Error)
	}
	return r
}

func (r *rssRig) dlID(name string) uint {
	r.t.Helper()
	var ds models.DownloaderSetting
	require.NoError(r.t, r.db.DB.Where("name = ?", name).First(&ds).Error)
	return ds.ID
}

// addRSS 建站点与 RSS 订阅（真实编号），返回运行用的配置。
func (r *rssRig) addRSS(feed *feedServer, cfgs ...models.RSSConfig) []models.RSSConfig {
	r.t.Helper()
	enabled := true
	for i := range cfgs {
		cfgs[i].URL = feed.srv.URL + "/?rss=" + cfgs[i].Name
	}
	require.NoError(r.t, core.NewConfigStore(r.db).UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{
		Enabled: &enabled, AuthMethod: "cookie", Cookie: "c=1", RSS: cfgs,
	}))
	var subs []models.RSSSubscription
	require.NoError(r.t, r.db.DB.Order("id").Find(&subs).Error)
	require.Len(r.t, subs, len(cfgs))
	out := make([]models.RSSConfig, len(subs))
	for i, s := range subs {
		out[i] = cfgs[i]
		out[i].ID = s.ID
	}
	return out
}

// round 跑一轮：同一个开始时间下取详情、下载暂存，再推送暂存文件。
func (r *rssRig) round(rss models.RSSConfig) {
	r.t.Helper()
	ctx := WithRSSAttempt(context.Background(), time.Now())
	require.NoError(r.t, FetchAndDownloadFreeRSSUnified(ctx, r.site, rss))
	r.push(ctx, rss)
}

// fetch 只跑取详情这一阶段。
func (r *rssRig) fetch(rss models.RSSConfig) context.Context {
	r.t.Helper()
	ctx := WithRSSAttempt(context.Background(), time.Now())
	require.NoError(r.t, FetchAndDownloadFreeRSSUnified(ctx, r.site, rss))
	return ctx
}

func (r *rssRig) push(ctx context.Context, rss models.RSSConfig) {
	r.t.Helper()
	require.NoError(r.t, ProcessTorrentsWithDownloaderByRSS(ctx, rss, r.stagingDir(rss), rss.Category, rss.Tag,
		r.site.SiteGroup(), RevalidateFromSite(r.site)))
}

func (r *rssRig) stagingDir(rss models.RSSConfig) string {
	return filepath.Join(r.base, strings.TrimSpace(rss.Tag))
}

func (r *rssRig) staged(rss models.RSSConfig, id string) bool {
	_, err := os.Stat(filepath.Join(r.stagingDir(rss), "springsunday-"+id+".torrent"))
	return err == nil
}

func (r *rssRig) record(id string) models.TorrentInfo {
	r.t.Helper()
	ti, err := r.db.GetTorrentBySiteAndID("springsunday", id)
	require.NoError(r.t, err)
	require.NotNil(r.t, ti, "种子 %s 没有记录", id)
	return *ti
}

func freeUntil(d time.Duration) v2.TorrentItem {
	return v2.TorrentItem{DiscountLevel: v2.DiscountFree, DiscountEndTime: time.Now().Add(d)}
}

func pushedOf(ti models.TorrentInfo) bool { return ti.IsPushed != nil && *ti.IsPushed }

// 第一次读到的免费没有结束时间（unknown）：不推送；下一轮详情给出结束时间后整套刷新并推送。
func TestRSSFreeSafety_UnknownEndThenRefreshPushes(t *testing.T) {
	r := newRSSRig(t)
	feed := newFeedServer(t, "u1")
	rss := r.addRSS(feed, models.RSSConfig{Name: "free", Tag: "movie", IntervalMinutes: 10})[0]

	r.site.set("u1", v2.TorrentItem{DiscountLevel: v2.DiscountFree})
	r.round(rss)
	ti := r.record("u1")
	assert.Equal(t, string(v2.FreeUnknown), ti.FreeValidity)
	assert.True(t, ti.RequireFree, "免费通道要求免费")
	assert.Nil(t, ti.FreeEndTime)
	assert.False(t, pushedOf(ti))
	assert.Empty(t, r.dls["qb"].addCalls(), "结束时间未知时不加入下载器")
	assert.True(t, r.staged(rss, "u1"), "暂存文件留着，等下一轮重新验证")
	assert.Contains(t, ti.LastError, "未知")

	end := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	r.site.set("u1", v2.TorrentItem{DiscountLevel: v2.DiscountFree, DiscountEndTime: end})
	before := time.Now()
	r.round(rss)
	ti = r.record("u1")
	assert.Equal(t, string(v2.FreeUntil), ti.FreeValidity)
	require.NotNil(t, ti.FreeEndTime)
	assert.True(t, ti.FreeEndTime.Equal(end), "结束时间按这一轮的详情写入")
	require.NotNil(t, ti.FreeVerifiedAt)
	assert.False(t, ti.FreeVerifiedAt.Before(before.Add(-time.Second)), "验证时间是这一轮的")
	assert.True(t, pushedOf(ti))
	assert.Len(t, r.dls["qb"].addCalls(), 1)
	assert.False(t, r.staged(rss, "u1"))
}

// 免费变成不免费：记录整套刷新（不留旧的免费级别与结束时间），暂存文件删除，不加入下载器。
func TestRSSFreeSafety_FreeBecomesNotFree(t *testing.T) {
	r := newRSSRig(t)
	require.NoError(t, core.NewConfigStore(r.db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: r.base, DefaultIntervalMinutes: 10, DefaultEnabled: true, MaxRetry: 3,
		CleanupDiskProtect: true, CleanupMinDiskSpaceGB: 10,
	}))
	feed := newFeedServer(t, "n1")
	rss := r.addRSS(feed, models.RSSConfig{Name: "free", Tag: "movie", IntervalMinutes: 10})[0]

	r.dls["qb"].setFree(5 * gb) // 低于保底线
	r.site.set("n1", freeUntil(48*time.Hour))
	r.round(rss)
	assert.Empty(t, r.dls["qb"].addCalls())
	assert.True(t, r.staged(rss, "n1"), "磁盘不足时暂存文件留着")
	assert.Equal(t, string(v2.FreeUntil), r.record("n1").FreeValidity)

	r.dls["qb"].setFree(1 << 50)
	r.site.set("n1", v2.TorrentItem{DiscountLevel: v2.DiscountNone})
	r.round(rss)
	ti := r.record("n1")
	assert.Equal(t, string(v2.FreeNotFree), ti.FreeValidity)
	assert.False(t, ti.IsFree)
	assert.Equal(t, string(v2.DiscountNone), ti.FreeLevel)
	assert.Nil(t, ti.FreeEndTime, "不再免费时清掉旧的结束时间")
	assert.True(t, ti.IsSkipped)
	assert.False(t, pushedOf(ti))
	assert.Empty(t, r.dls["qb"].addCalls(), "不再免费的种子不加入下载器")
	assert.False(t, r.staged(rss, "n1"), "暂存文件删除")
}

// 磁盘不足被拒 → 优惠结束 → 空间恢复：下一次推送必须重新读详情，下载器 Add 调用为 0。
func TestRSSFreeSafety_DiskRejectThenPromotionEndsNoAdd(t *testing.T) {
	cases := []struct {
		name   string
		inFeed bool // 第二轮 RSS 里还有没有这个条目
		change func(s *roundSite)
		check  func(t *testing.T, r *rssRig, rss models.RSSConfig)
	}{
		{
			name: "条目消失，详情显示优惠已结束",
			change: func(s *roundSite) {
				s.set("d1", v2.TorrentItem{DiscountLevel: v2.DiscountFree, DiscountEndTime: time.Now().Add(-time.Minute)})
			},
			check: func(t *testing.T, r *rssRig, rss models.RSSConfig) {
				ti := r.record("d1")
				assert.True(t, ti.IsExpired)
				assert.Equal(t, string(v2.FreeNotFree), ti.FreeValidity, "按重新读到的详情写回")
				assert.False(t, r.staged(rss, "d1"))
			},
		},
		{
			name: "条目消失，详情读取失败",
			change: func(s *roundSite) {
				s.fail("d1", fmt.Errorf("%w: 站点返回 404", errNoDetail))
			},
			check: func(t *testing.T, r *rssRig, rss models.RSSConfig) {
				ti := r.record("d1")
				assert.False(t, pushedOf(ti))
				assert.True(t, r.staged(rss, "d1"), "读不到详情时不推送，也不删除")
				assert.Contains(t, ti.LastError, "本次")
			},
		},
		{
			name:   "条目还在，剩余时间不够安全余量",
			inFeed: true,
			change: func(s *roundSite) { s.set("d1", freeUntil(3*time.Minute)) },
			check: func(t *testing.T, r *rssRig, rss models.RSSConfig) {
				ti := r.record("d1")
				assert.False(t, pushedOf(ti))
				assert.False(t, r.staged(rss, "d1"))
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRSSRig(t)
			require.NoError(t, core.NewConfigStore(r.db).SaveGlobalSettings(models.SettingsGlobal{
				DownloadDir: r.base, DefaultIntervalMinutes: 10, DefaultEnabled: true, MaxRetry: 3,
				CleanupDiskProtect: true, CleanupMinDiskSpaceGB: 10,
			}))
			feed := newFeedServer(t, "d1")
			rss := r.addRSS(feed, models.RSSConfig{Name: "free", Tag: "movie", IntervalMinutes: 10})[0]

			r.dls["qb"].setFree(5 * gb)
			r.site.set("d1", freeUntil(2*time.Hour))
			r.round(rss)
			require.Empty(t, r.dls["qb"].addCalls())
			require.True(t, r.staged(rss, "d1"))
			callsAfterFirst := r.site.calls("d1")

			tc.change(r.site)
			if !tc.inFeed {
				feed.setItems()
			}
			r.dls["qb"].setFree(1 << 50)
			r.round(rss)

			assert.Empty(t, r.dls["qb"].addCalls(), "优惠结束后空间恢复也不能加入")
			assert.Greater(t, r.site.calls("d1"), callsAfterFirst, "推送前重新读了详情")
			tc.check(t, r, rss)
		})
	}
}

// 同站两条 RSS 用同一个标签（同一个暂存目录）、暂停设置不同：各推各的文件，不用对方的下载器、路径与暂停开关。
func TestRSSFreeSafety_TwoRSSSameTagDifferentPause(t *testing.T) {
	r := newRSSRig(t, "qb-a", "qb-b")
	idA, idB := r.dlID("qb-a"), r.dlID("qb-b")
	feedA := newFeedServer(t, "a1", "z1")
	feedB := newFeedServer(t, "b1", "z1")
	rs := r.addRSS(feedA,
		models.RSSConfig{Name: "A", Tag: "movie", IntervalMinutes: 10, PauseOnFreeEnd: true, DownloaderID: &idA, DownloadPath: "/data/a"},
		models.RSSConfig{Name: "B", Tag: "movie", IntervalMinutes: 10, PauseOnFreeEnd: false, DownloaderID: &idB, DownloadPath: "/data/b"},
	)
	a, b := rs[0], rs[1]
	b.URL = feedB.srv.URL + "/?rss=B"
	for _, id := range []string{"a1", "b1", "z1"} {
		r.site.set(id, freeUntil(48*time.Hour))
	}

	ctxA := r.fetch(a) // A 先看到 z1，z1 归 A
	ctxB := r.fetch(b)
	require.True(t, r.staged(a, "a1"))
	require.True(t, r.staged(b, "b1"))
	z := r.record("z1")
	require.NotNil(t, z.SourceRSSID)
	assert.Equal(t, a.ID, *z.SourceRSSID, "z1 归先决定下载它的 A")
	assert.True(t, z.RequireFree)

	// B 先推：只推自己的 b1
	r.push(ctxB, b)
	addsB := r.dls["qb-b"].addCalls()
	require.Len(t, addsB, 1, "B 只推自己的文件")
	assert.Equal(t, "/data/b", addsB[0].SavePath)
	assert.False(t, addsB[0].AddAtPaused, "B 不承诺暂停，按自动开始加入")
	assert.Empty(t, r.dls["qb-a"].addCalls())
	assert.True(t, r.staged(a, "a1"), "B 不处理 A 的文件")
	assert.True(t, r.staged(a, "z1"))
	tb := r.record("b1")
	require.NotNil(t, tb.DownloaderID)
	assert.Equal(t, idB, *tb.DownloaderID)
	assert.False(t, tb.PauseOnFreeEnd)

	// A 再推：a1 与 z1 都用 A 的下载器、路径，先暂停加入再开始
	r.push(ctxA, a)
	addsA := r.dls["qb-a"].addCalls()
	require.Len(t, addsA, 2)
	for _, o := range addsA {
		assert.Equal(t, "/data/a", o.SavePath)
		assert.True(t, o.AddAtPaused, "承诺到期暂停的先暂停加入")
	}
	assert.Len(t, r.dls["qb-a"].resumeCalls(), 2, "记账与监控登记成功后开始")
	for _, id := range []string{"a1", "z1"} {
		ti := r.record(id)
		require.NotNil(t, ti.DownloaderID)
		assert.Equal(t, idA, *ti.DownloaderID, id)
		assert.True(t, ti.PauseOnFreeEnd, id)
		require.NotNil(t, ti.SourceRSSID)
		assert.Equal(t, a.ID, *ti.SourceRSSID, id)
		require.NotNil(t, ti.PausePlannedAt, id)
	}
	assert.Len(t, r.dls["qb-b"].addCalls(), 1, "B 的下载器没有收到 A 的种子")
}

// 过滤规则明确允许非免费：不是免费也照样推送；RSS 开着到期暂停也不伪造免费监控。
func TestRSSFreeSafety_AllowNonFreeRulePushes(t *testing.T) {
	r := newRSSRig(t)
	feed := newFeedServer(t, "p1")
	rss := r.addRSS(feed, models.RSSConfig{Name: "rule", Tag: "tv", IntervalMinutes: 10, PauseOnFreeEnd: true})[0]
	rule := models.FilterRule{Name: "any", Pattern: ".*", PatternType: "regex", MatchField: "both", Enabled: true, Priority: 100, Purpose: "download"}
	require.NoError(t, r.db.DB.Create(&rule).Error)
	require.NoError(t, r.db.DB.Model(&models.FilterRule{}).Where("id = ?", rule.ID).Update("require_free", false).Error)
	require.NoError(t, models.NewRSSFilterAssociationDB(r.db.DB).SetFilterRulesForRSS(rss.ID, []uint{rule.ID}))

	r.site.set("p1", v2.TorrentItem{DiscountLevel: v2.DiscountNone})
	r.round(rss)
	ti := r.record("p1")
	assert.False(t, ti.RequireFree)
	assert.Equal(t, "filter_rule", ti.DownloadSource)
	assert.True(t, pushedOf(ti), "明确允许非免费的规则照样推送")
	adds := r.dls["qb"].addCalls()
	require.Len(t, adds, 1)
	assert.False(t, adds[0].AddAtPaused, "不免费的种子没有免费结束，不需要暂停加入")
	assert.Nil(t, ti.PausePlannedAt)
}

// 详情里非空的结束时间解析失败：记成 unknown，错误写进记录（可观察），要求免费的不推送。
func TestRSSFreeSafety_ParseErrorObservable(t *testing.T) {
	r := newRSSRig(t)
	feed := newFeedServer(t, "e1")
	rss := r.addRSS(feed, models.RSSConfig{Name: "free", Tag: "movie", IntervalMinutes: 10})[0]

	r.site.set("e1", v2.TorrentItem{DiscountLevel: v2.DiscountFree, DiscountValidity: v2.FreeUnknown, DiscountErr: "endTime 无法解析"})
	r.round(rss)
	ti := r.record("e1")
	assert.Equal(t, string(v2.FreeUnknown), ti.FreeValidity)
	assert.Contains(t, ti.FreeStatusError, "endTime")
	assert.Empty(t, r.dls["qb"].addCalls())
}

// 通知按钮推的是另一条 RSS 暂存过的种子：记录的所有者不变，所有者的暂存文件由它自己在推送循环里清掉。
func TestRSSFreeSafety_ButtonPushKeepsOwner(t *testing.T) {
	r := newRSSRig(t, "qb-a", "qb-b")
	idA, idB := r.dlID("qb-a"), r.dlID("qb-b")
	feed := newFeedServer(t, "o1")
	rs := r.addRSS(feed,
		models.RSSConfig{Name: "A", Tag: "movie", IntervalMinutes: 10, DownloaderID: &idA},
		models.RSSConfig{Name: "B", Tag: "tv", IntervalMinutes: 10, DownloaderID: &idB},
	)
	a, b := rs[0], rs[1]
	r.site.set("o1", freeUntil(48*time.Hour))
	r.dls["qb-a"].setFree(5 * gb) // A 这一轮推不出去：暂存着
	require.NoError(t, core.NewConfigStore(r.db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: r.base, DefaultIntervalMinutes: 10, DefaultEnabled: true, MaxRetry: 3,
		CleanupDiskProtect: true, CleanupMinDiskSpaceGB: 10,
	}))
	r.round(a)
	require.True(t, r.staged(a, "o1"))

	// B 的通知里点了「立即下载」：经共享推送加到 B 的下载器
	t.Cleanup(SwapPushDownloaderFactory(func(s models.DownloaderSetting) (downloader.Downloader, error) { return r.dls[s.Name], nil }))
	bID := b.ID
	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "o1", TorrentData: roundTorrentBytes("springsunday-o1"), DownloaderID: idB,
		Policy: AdmissionPolicy{SourceKind: SourceKindRSSNotify, SourceRSSID: &bID, AttemptStart: time.Now()},
	})
	require.NoError(t, err)
	require.True(t, res.Success, res.Message)
	ti := r.record("o1")
	require.NotNil(t, ti.SourceRSSID)
	assert.Equal(t, a.ID, *ti.SourceRSSID, "所有者还是 A")

	r.dls["qb-a"].setFree(1 << 50)
	r.round(a)
	assert.False(t, r.staged(a, "o1"), "A 的推送循环看到已推送，清掉自己的暂存文件")
	assert.Empty(t, r.dls["qb-a"].addCalls(), "已经在 B 的下载器里，A 不再推")
}

// 已推送、正受到期监控的种子：之后的 RSS 轮次照样读详情刷新资格；免费缩短把计划暂停提前，
// 不再免费时改成现在（监控器马上暂停），并交给监控器重新预约。
func TestRSSFreeSafety_PushedMonitoredRefreshMovesPausePlan(t *testing.T) {
	r := newRSSRig(t)
	feed := newFeedServer(t, "m1")
	rss := r.addRSS(feed, models.RSSConfig{Name: "free", Tag: "movie", IntervalMinutes: 10, PauseOnFreeEnd: true})[0]
	var scheduled []models.TorrentInfo
	RegisterTorrentScheduler(func(ti models.TorrentInfo) { scheduled = append(scheduled, ti) })
	t.Cleanup(func() { RegisterTorrentScheduler(nil) })

	r.site.set("m1", freeUntil(48*time.Hour))
	r.round(rss)
	ti := r.record("m1")
	require.True(t, pushedOf(ti))
	require.NotNil(t, ti.PausePlannedAt)
	firstPlan := *ti.PausePlannedAt

	// 站点把免费缩短到 3 小时以后
	shorter := time.Now().Add(3 * time.Hour).UTC().Truncate(time.Second)
	r.site.set("m1", v2.TorrentItem{DiscountLevel: v2.DiscountFree, DiscountEndTime: shorter})
	r.round(rss)
	ti = r.record("m1")
	require.NotNil(t, ti.PausePlannedAt)
	assert.True(t, ti.PausePlannedAt.Before(firstPlan), "计划暂停跟着缩短的结束时间提前")
	require.NotNil(t, ti.FreeEndTime)
	assert.True(t, ti.FreeEndTime.Equal(shorter))
	require.NotEmpty(t, scheduled)
	assert.True(t, scheduled[len(scheduled)-1].PausePlannedAt.Equal(*ti.PausePlannedAt), "交给监控器按新的时刻预约")

	// 站点取消了免费
	r.site.set("m1", v2.TorrentItem{DiscountLevel: v2.DiscountNone})
	before := time.Now()
	r.round(rss)
	ti = r.record("m1")
	assert.Equal(t, string(v2.FreeNotFree), ti.FreeValidity)
	require.NotNil(t, ti.PausePlannedAt)
	assert.False(t, ti.PausePlannedAt.After(time.Now()), "不再免费：计划暂停改成现在")
	assert.False(t, ti.PausePlannedAt.Before(before.Add(-time.Second)))
	assert.Len(t, r.dls["qb"].addCalls(), 1, "只是刷新资格，不会再推")
}
