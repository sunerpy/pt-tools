package scheduler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeebo/bencode"

	"github.com/sunerpy/pt-tools/global"
	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

// brushTorrentBytes 造一个体积为 size 的种子文件，返回内容与 info hash（小写）。
func brushTorrentBytes(t *testing.T, name string, size int64) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, bencode.NewEncoder(&buf).Encode(map[string]any{
		"info": map[string]any{"name": name, "length": size, "piece length": 16384},
	}))
	h, err := qbit.ComputeTorrentHash(buf.Bytes())
	require.NoError(t, err)
	return buf.Bytes(), strings.ToLower(h)
}

// fakeBrushSite 是只实现刷流用到的 Search / Download 的站点。
type fakeBrushSite struct {
	mu        sync.Mutex
	items     []v2.TorrentItem
	files     map[string][]byte
	searchErr error
	searches  []v2.SearchQuery
	downloads []string
}

func (s *fakeBrushSite) ID() string                                       { return "hdsky" }
func (s *fakeBrushSite) Name() string                                     { return "HDSky" }
func (s *fakeBrushSite) Kind() v2.SiteKind                                { return v2.SiteNexusPHP }
func (s *fakeBrushSite) Login(context.Context, v2.Credentials) error      { return nil }
func (s *fakeBrushSite) GetUserInfo(context.Context) (v2.UserInfo, error) { return v2.UserInfo{}, nil }
func (s *fakeBrushSite) Close() error                                     { return nil }

func (s *fakeBrushSite) Search(_ context.Context, q v2.SearchQuery) ([]v2.TorrentItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.searches = append(s.searches, q)
	return s.items, s.searchErr
}

func (s *fakeBrushSite) Download(_ context.Context, id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.downloads = append(s.downloads, id)
	data, ok := s.files[id]
	if !ok {
		return nil, fmt.Errorf("no file %s", id)
	}
	return data, nil
}

// brushRig 是一套刷流测试环境：真实的库（含全部表）、假站点、假下载器、记录推送请求的假推送。
type brushRig struct {
	db     *models.TorrentDB
	site   *fakeBrushSite
	dl     *schedFakeDownloader
	clock  *sitelogin.FakeClock
	mon    *BrushMonitor
	pushes []ptinternal.PushTorrentRequest
	// pushResult 不为空时推送直接返回它（模拟闸门拒绝）；为空时把种子加进假下载器
	pushResult *ptinternal.PushTorrentResult
	pushErr    error
}

func newBrushRig(t *testing.T) *brushRig {
	t.Helper()
	db := setupTestDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, db.DB.AutoMigrate(&models.BrushTask{}, &models.BrushTorrent{}, &models.BrushTorrentSample{}, &models.BrushDailyStat{}))
	r := &brushRig{
		db:    db,
		site:  &fakeBrushSite{files: map[string][]byte{}},
		dl:    newSchedFakeDownloader("qb"),
		clock: sitelogin.NewFakeClock(time.Date(2026, 10, 6, 4, 0, 0, 0, time.UTC)),
	}
	r.mon = NewBrushMonitor(BrushMonitorConfig{
		DB: db.DB,
		Sites: BrushSitesFunc(func(name string) (v2.Site, bool) {
			return r.site, name == "hdsky"
		}),
		Downloaders: BrushDownloadersFunc(func(id uint) (downloader.Downloader, string, error) {
			if id != 1 {
				return nil, "", errors.New("no such downloader")
			}
			return r.dl, "qb", nil
		}),
		Push: func(_ context.Context, req ptinternal.PushTorrentRequest) (*ptinternal.PushTorrentResult, error) {
			r.pushes = append(r.pushes, req)
			if r.pushErr != nil {
				return nil, r.pushErr
			}
			if r.pushResult != nil {
				return r.pushResult, nil
			}
			h, _ := qbit.ComputeTorrentHash(req.TorrentData)
			r.dl.torrents = append(r.dl.torrents, downloader.Torrent{
				ID: h, InfoHash: h, Name: req.Title, Tags: req.Tags, Progress: 0,
			})
			return &ptinternal.PushTorrentResult{Success: true, TorrentHash: h}, nil
		},
		Clock:    r.clock,
		Location: time.UTC,
	})
	return r
}

func (r *brushRig) addItem(t *testing.T, id string, mod func(*v2.TorrentItem)) string {
	t.Helper()
	data, hash := brushTorrentBytes(t, "t-"+id, 4*gib)
	r.site.files[id] = data
	it := v2.TorrentItem{
		ID: id, Title: "Title " + id, SizeBytes: 4 * gib, Seeders: 2, Leechers: 30,
		UploadedAt: r.clock.Now().Add(-10 * time.Minute).Unix(), DiscountLevel: v2.DiscountFree,
		DiscountEndTime: r.clock.Now().Add(12 * time.Hour),
	}
	if mod != nil {
		mod(&it)
	}
	r.site.items = append(r.site.items, it)
	return hash
}

func (r *brushRig) task(t *testing.T, mod func(*models.BrushTask)) *models.BrushTask {
	t.Helper()
	task := &models.BrushTask{
		Name: "hdsky 刷流", Enabled: true, SiteName: "hdsky", DownloaderID: 1, IntervalMin: 10,
		MaxDownloading: 2, ExcludeHR: true, RemoveFreeExpiredIncomplete: true, RemoveWithData: true, Tags: "我的刷流",
	}
	if mod != nil {
		mod(task)
	}
	require.NoError(t, models.NewBrushRepository(r.db.DB).SaveTask(task))
	return task
}

func TestBrushMonitor_AdmitsWithinLimits(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, nil)
	r.addItem(t, "1", func(i *v2.TorrentItem) { i.DiscountLevel = v2.DiscountNone }) // M-Team 式混入的普通种
	r.addItem(t, "2", func(i *v2.TorrentItem) { i.HasHR = true })
	r.addItem(t, "3", func(i *v2.TorrentItem) { i.Leechers = 10 })
	r.addItem(t, "4", func(i *v2.TorrentItem) { i.Leechers = 90; i.IMDbID = "tt0111161"; i.DoubanID = "1292052" })
	r.addItem(t, "5", func(i *v2.TorrentItem) { i.Leechers = 50 })

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, 5, res.Listed)
	assert.Equal(t, 3, res.Eligible)
	assert.Equal(t, 2, res.Added, "同时下载数上限 2")
	assert.Contains(t, res.Stopped, "达到上限")
	require.Len(t, r.site.searches, 1, "每个周期只请求一次列表")
	assert.True(t, r.site.searches[0].FreeOnly)

	require.Len(t, r.pushes, 2)
	assert.Equal(t, "4", r.pushes[0].TorrentID, "下载人数多的先进")
	assert.Equal(t, "5", r.pushes[1].TorrentID)
	p := r.pushes[0]
	assert.Equal(t, "brush", p.Source)
	assert.Equal(t, fmt.Sprintf("hdsky,pt-tools-brush,pt-brush-%d,我的刷流", task.ID), p.Tags,
		"站点名让站点容量闸门计入，pt-tools-brush 让全局自动清理避开，任务标签限定删种范围")
	require.NotNil(t, p.Meta)
	assert.EqualValues(t, 4*gib, p.Meta.SizeBytes)
	assert.True(t, p.Meta.IsFree)
	require.NotNil(t, p.Meta.FreeEndTime)
	assert.Equal(t, "tt0111161", p.IMDbID, "搜索结果里的外部编号随推送带上")
	assert.Equal(t, "1292052", p.DoubanID)
	assert.Empty(t, r.pushes[1].IMDbID)

	repo := models.NewBrushRepository(r.db.DB)
	active, err := repo.ActiveTorrents(task.ID)
	require.NoError(t, err)
	require.Len(t, active, 2)
	stats, err := repo.DailyStats(task.ID, "2026-10-06", "2026-10-06")
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.Equal(t, 2, stats[0].Added)

	got, err := repo.GetTask(task.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastRunAt)
	assert.Contains(t, got.LastResult, "加入 2 个")

	// 下一轮：两个都还在下载，限额满了，不再请求列表
	r.clock.Advance(10 * time.Minute)
	res, err = r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, res.Added)
	assert.Len(t, r.site.searches, 1, "限额已满时不请求站点")
	assert.Equal(t, 2, res.Sampled)
}

// 推送过的种子（不论现在是否已被删）不会再加回来；下载器里已经有的也跳过。
func TestBrushMonitor_SkipsSeenAndExisting(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 5 })
	r.addItem(t, "1", nil)
	existing := r.addItem(t, "2", nil)
	r.dl.torrents = append(r.dl.torrents, downloader.Torrent{ID: existing, InfoHash: existing, Name: "手动加的"})
	repo := models.NewBrushRepository(r.db.DB)
	require.NoError(t, repo.RecordAdded(&models.BrushTorrent{
		TaskID: task.ID, InfoHash: "old", SiteName: "hdsky", TorrentID: "1",
		AddedAt: r.clock.Now(), State: models.BrushTorrentRemoved,
	}, "2026-10-05"))

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, res.Added)
	assert.Len(t, res.Skipped, 1)
	assert.Contains(t, res.Skipped[0], "下载器里已经有了")
	assert.Empty(t, r.pushes)
}

func TestBrushMonitor_SamplesAndRemoves(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, func(bt *models.BrushTask) { bt.RemoveRatio = 2; bt.Enabled = false })
	tag := models.BrushTaskTag(task.ID)
	repo := models.NewBrushRepository(r.db.DB)
	add := func(hash string) *models.BrushTorrent {
		bt := &models.BrushTorrent{
			TaskID: task.ID, InfoHash: hash, SiteName: "hdsky", TorrentID: hash, Title: "T-" + hash,
			AddedAt: r.clock.Now().Add(-time.Hour), State: models.BrushTorrentActive, DownloaderID: 1,
		}
		require.NoError(t, repo.RecordAdded(bt, "2026-10-06"))
		return bt
	}
	add("done")    // 分享率达标：删
	add("keep")    // 还没达标：留
	add("missing") // 下载器里没了：记为 gone
	add("untag")   // 用户摘了任务标签：交还给用户
	r.dl.torrents = []downloader.Torrent{
		{ID: "id-done", InfoHash: "DONE", Tags: "hdsky," + tag, Progress: 1, Ratio: 2.5, TotalUploaded: 9000, TotalDownloaded: 3000},
		{ID: "id-keep", InfoHash: "keep", Tags: tag, Progress: 1, Ratio: 1.2, TotalUploaded: 100},
		{ID: "id-untag", InfoHash: "untag", Tags: "hdsky", Progress: 1, Ratio: 5},
		// 带任务标签但没有记录（用户自己打的标签）：刷流不碰
		{ID: "id-foreign", InfoHash: "foreign", Tags: tag, Progress: 1, Ratio: 9},
	}

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"id-done"}, r.dl.removedSingle, "只删达标且归任务管的种子")
	assert.Equal(t, []bool{true}, r.dl.removeDataFlags)
	assert.Equal(t, 1, res.Removed)
	assert.Equal(t, 2, res.Gone)
	assert.Equal(t, 2, res.Sampled)
	assert.Contains(t, res.Stopped, "任务已关闭")
	assert.Empty(t, r.site.searches, "关闭的任务只删种不加种")

	rows, _, err := repo.ListTorrents(task.ID, "", 1, 50)
	require.NoError(t, err)
	states := map[string]string{}
	reasons := map[string]string{}
	for _, row := range rows {
		states[row.InfoHash] = row.State
		reasons[row.InfoHash] = row.RemoveReason
	}
	assert.Equal(t, models.BrushTorrentRemoved, states["done"])
	assert.Contains(t, reasons["done"], "分享率 2.50")
	assert.Equal(t, models.BrushTorrentActive, states["keep"])
	assert.Equal(t, models.BrushTorrentGone, states["missing"])
	assert.Equal(t, models.BrushTorrentGone, states["untag"])

	stats, err := repo.DailyStats(task.ID, "2026-10-06", "2026-10-06")
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.EqualValues(t, 9100, stats[0].Uploaded, "两个还在的种子第一次采样的累计上传")
	assert.Equal(t, 1, stats[0].Removed)
}

// 推送被磁盘或站点容量闸门拒绝：这一轮停止加种，不记录种子。
func TestBrushMonitor_StopsWhenPushRejected(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 5 })
	r.addItem(t, "1", nil)
	r.addItem(t, "2", nil)
	r.pushResult = &ptinternal.PushTorrentResult{Success: false, Message: "磁盘空间不足 (有效 1.0 GB <= 50.0 GB)，暂停推送"}

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Len(t, r.pushes, 1, "第一个被拒之后不再尝试")
	assert.Contains(t, res.Stopped, "磁盘空间不足")
	active, err := models.NewBrushRepository(r.db.DB).ActiveTorrents(task.ID)
	require.NoError(t, err)
	assert.Empty(t, active)

	r.pushResult = nil
	r.pushErr = errors.New("connection refused")
	r.clock.Advance(10 * time.Minute)
	res, err = r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Contains(t, res.Stopped, "推送失败")
	assert.Len(t, res.Errors, 1)
}

func TestBrushMonitor_LimitsBySizeAndDailyVolume(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 10; bt.MaxDailyDownloadGB = 9 })
	r.addItem(t, "1", nil)
	r.addItem(t, "2", nil)
	r.addItem(t, "3", nil)
	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Added, "每个 4 GB，今天最多 9 GB")
	assert.Contains(t, res.Stopped, "今天加入的体积将超过 9.0 GB")

	r2 := newBrushRig(t)
	task2 := r2.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 10; bt.MaxTotalSizeGB = 5 })
	r2.addItem(t, "1", nil)
	r2.addItem(t, "2", nil)
	res, err = r2.mon.RunTask(context.Background(), task2.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Added)
	assert.Contains(t, res.Stopped, "任务总体积将超过 5.0 GB")
}

func TestBrushMonitor_RunOnceHonorsIntervalAndEnabled(t *testing.T) {
	r := newBrushRig(t)
	on := r.task(t, func(bt *models.BrushTask) { bt.Name = "on" })
	off := r.task(t, func(bt *models.BrushTask) { bt.Name = "off"; bt.Enabled = false })
	r.addItem(t, "1", nil)

	r.mon.RunOnce(context.Background())
	repo := models.NewBrushRepository(r.db.DB)
	gotOn, _ := repo.GetTask(on.ID)
	gotOff, _ := repo.GetTask(off.ID)
	require.NotNil(t, gotOn.LastRunAt)
	assert.Nil(t, gotOff.LastRunAt, "关闭的任务不定时运行")
	assert.Len(t, r.site.searches, 1)

	r.clock.Advance(5 * time.Minute)
	r.mon.RunOnce(context.Background())
	assert.Len(t, r.site.searches, 1, "没到间隔")
	r.clock.Advance(5 * time.Minute)
	r.mon.RunOnce(context.Background())
	assert.Len(t, r.site.searches, 2, "满 10 分钟再跑一轮")
}

func TestBrushMonitor_BusyAndErrors(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, nil)
	r.mon.mu.Lock()
	r.mon.busy[task.ID] = true
	r.mon.mu.Unlock()
	_, err := r.mon.RunTask(context.Background(), task.ID)
	assert.ErrorIs(t, err, ErrBrushBusy)
	r.mon.mu.Lock()
	delete(r.mon.busy, task.ID)
	r.mon.mu.Unlock()

	_, err = r.mon.RunTask(context.Background(), 999)
	assert.ErrorIs(t, err, models.ErrBrushTaskNotFound)

	r.site.searchErr = errors.New("站点 502")
	_, err = r.mon.RunTask(context.Background(), task.ID)
	require.Error(t, err)
	got, _ := models.NewBrushRepository(r.db.DB).GetTask(task.ID)
	assert.Contains(t, got.LastError, "站点 502", "失败原因写进任务，界面上看得到")

	bad := r.task(t, func(bt *models.BrushTask) { bt.Name = "bad"; bt.DownloaderID = 7 })
	_, err = r.mon.RunTask(context.Background(), bad.ID)
	assert.ErrorContains(t, err, "下载器不可用")
	other := r.task(t, func(bt *models.BrushTask) { bt.Name = "other"; bt.SiteName = "ourbits" })
	r.site.searchErr = nil
	_, err = r.mon.RunTask(context.Background(), other.ID)
	assert.ErrorContains(t, err, "未启用或未注册")
}

func TestDownhashOf(t *testing.T) {
	assert.Equal(t, "abc", downhashOf("/api/site/hddolby/torrent/1/download?downhash=abc"))
	assert.Empty(t, downhashOf("https://x/download.php?id=1"))
	assert.Empty(t, downhashOf(""))
}

// addingDownloader 在假下载器之上让 AddTorrentFileEx 真的「加进去」，供真实推送路径的成功分支用。
type addingDownloader struct {
	*schedFakeDownloader
	opts []downloader.AddTorrentOptions
}

func (a *addingDownloader) AddTorrentFileEx(data []byte, opt downloader.AddTorrentOptions) (downloader.AddTorrentResult, error) {
	h, err := qbit.ComputeTorrentHash(data)
	if err != nil {
		return downloader.AddTorrentResult{}, err
	}
	a.opts = append(a.opts, opt)
	a.torrents = append(a.torrents, downloader.Torrent{ID: h, InfoHash: h, Tags: opt.Tags})
	return downloader.AddTorrentResult{Success: true, Hash: h}, nil
}

func (a *addingDownloader) CheckTorrentExists(hash string) (bool, error) {
	for _, t := range a.torrents {
		if strings.EqualFold(t.InfoHash, hash) {
			return true, nil
		}
	}
	return false, nil
}

// 真实推送路径（internal.PushTorrentToDownloader）：磁盘保护与站点容量闸门照常生效，被拒时刷流停下、不记录种子；
// 放行后推送带着刷流标签与来源进下载器，TorrentInfo 记下 H&R 与体积。
func TestBrushMonitor_RealPushGates(t *testing.T) {
	r := newBrushRig(t)
	dl := &addingDownloader{schedFakeDownloader: r.dl}
	ds := models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true}
	require.NoError(t, r.db.DB.Create(&ds).Error)
	require.NoError(t, r.db.DB.Create(&models.SiteSetting{Name: "hdsky", Enabled: true, SeedingCapacityGB: 3}).Error)
	restore := ptinternal.SwapPushDownloaderFactory(func(models.DownloaderSetting) (downloader.Downloader, error) { return dl, nil })
	t.Cleanup(restore)
	ptinternal.GetDiskBudget().Reset()
	t.Cleanup(func() { ptinternal.GetDiskBudget().Reset() })
	r.mon.cfg.Push = ptinternal.PushTorrentToDownloader
	r.mon.cfg.Downloaders = BrushDownloadersFunc(func(uint) (downloader.Downloader, string, error) { return dl, "qb", nil })
	task := r.task(t, func(bt *models.BrushTask) { bt.DownloaderID = ds.ID })
	r.addItem(t, "1", nil) // 4 GB

	// 1. 默认开着磁盘保护，下载器报告的可用空间是 0：磁盘闸门拒绝
	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Contains(t, res.Stopped, "磁盘空间不足")
	assert.Equal(t, 0, res.Added)

	// 2. 关掉磁盘保护：4 GB 超过站点容量 3 GB，站点容量闸门拒绝
	require.NoError(t, r.db.DB.Create(&models.SettingsGlobal{DownloadDir: t.TempDir()}).Error)
	require.NoError(t, r.db.DB.Model(&models.SettingsGlobal{}).Where("1 = 1").Update("cleanup_disk_protect", false).Error)
	r.clock.Advance(10 * time.Minute)
	res, err = r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Contains(t, res.Stopped, "站点容量已满")
	active, err := models.NewBrushRepository(r.db.DB).ActiveTorrents(task.ID)
	require.NoError(t, err)
	assert.Empty(t, active)

	// 3. 放宽容量：推送成功，标签与来源都在
	require.NoError(t, r.db.DB.Model(&models.SiteSetting{}).Where("name = ?", "hdsky").Update("seeding_capacity_gb", 100).Error)
	r.clock.Advance(10 * time.Minute)
	res, err = r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Added)
	require.Len(t, dl.opts, 1)
	assert.Contains(t, dl.opts[0].Tags, "pt-tools-brush")
	var info models.TorrentInfo
	require.NoError(t, r.db.DB.Where("site_name = ? AND torrent_id = ?", "hdsky", "1").First(&info).Error)
	assert.Equal(t, "brush", info.DownloadSource)
	assert.EqualValues(t, 4*gib, info.TorrentSize)
	active, err = models.NewBrushRepository(r.db.DB).ActiveTorrents(task.ID)
	require.NoError(t, err)
	assert.Len(t, active, 1)
}

// 关闭的任务还有在做的种子时照样定时运行（只删不加），删完就不再运行。
func TestBrushMonitor_DisabledTaskKeepsRemoving(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, func(bt *models.BrushTask) { bt.Enabled = false; bt.RemoveRatio = 1 })
	repo := models.NewBrushRepository(r.db.DB)
	require.NoError(t, repo.RecordAdded(&models.BrushTorrent{
		TaskID: task.ID, InfoHash: "h1", SiteName: "hdsky", TorrentID: "1",
		AddedAt: r.clock.Now(), State: models.BrushTorrentActive,
	}, "2026-10-06"))
	r.dl.torrents = []downloader.Torrent{{ID: "id1", InfoHash: "h1", Tags: models.BrushTaskTag(task.ID), Progress: 1, Ratio: 1.5}}

	r.mon.RunOnce(context.Background())
	assert.Equal(t, []string{"id1"}, r.dl.removedSingle)
	assert.Empty(t, r.site.searches, "关闭的任务不加种")
	n, err := repo.CountActive(task.ID)
	require.NoError(t, err)
	assert.Zero(t, n)

	r.clock.Advance(time.Hour)
	before, _ := repo.GetTask(task.ID)
	r.mon.RunOnce(context.Background())
	after, _ := repo.GetTask(task.ID)
	assert.True(t, before.LastRunAt.Equal(*after.LastRunAt), "删完之后不再运行")
}

func TestBrushMonitor_WithTaskLock(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, nil)
	ran := false
	require.NoError(t, r.mon.WithTaskLock(task.ID, func() error {
		_, err := r.mon.RunTask(context.Background(), task.ID)
		assert.ErrorIs(t, err, ErrBrushBusy, "持锁期间运行拿到 busy")
		ran = true
		return nil
	}))
	assert.True(t, ran)
	r.mon.mu.Lock()
	r.mon.busy[task.ID] = true
	r.mon.mu.Unlock()
	assert.ErrorIs(t, r.mon.WithTaskLock(task.ID, func() error { return nil }), ErrBrushBusy)
	var nilMon *BrushMonitor
	assert.NoError(t, nilMon.WithTaskLock(1, func() error { return nil }))
}

// 站点连着几个种子文件都下不下来：这一轮停止加种，不把整张列表都下一遍。
func TestBrushMonitor_StopsAfterConsecutiveFetchFailures(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 10 })
	for i := range 6 {
		id := fmt.Sprintf("x%d", i)
		r.site.items = append(r.site.items, v2.TorrentItem{
			ID: id, Title: "T" + id, SizeBytes: gib, Leechers: 10, DiscountLevel: v2.DiscountFree,
		})
		r.site.files[id] = []byte("not a torrent")
	}
	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Len(t, r.site.downloads, 3)
	assert.Len(t, res.Errors, 3)
	assert.Contains(t, res.Stopped, "连续 3 个种子文件下载失败")
}

// 任务换了下载器：旧下载器里的种子照样按它自己的下载器采样、删种，不会被当成已经没了。
func TestBrushMonitor_ManagesTorrentsInPreviousDownloader(t *testing.T) {
	r := newBrushRig(t)
	old := newSchedFakeDownloader("qb-old")
	r.mon.cfg.Downloaders = BrushDownloadersFunc(func(id uint) (downloader.Downloader, string, error) {
		switch id {
		case 1:
			return r.dl, "qb", nil
		case 9:
			return old, "qb-old", nil
		}
		return nil, "", errors.New("no such downloader")
	})
	task := r.task(t, func(bt *models.BrushTask) { bt.Enabled = false; bt.RemoveRatio = 2 })
	repo := models.NewBrushRepository(r.db.DB)
	tag := models.BrushTaskTag(task.ID)
	require.NoError(t, repo.RecordAdded(&models.BrushTorrent{
		TaskID: task.ID, InfoHash: "h-old", SiteName: "hdsky", TorrentID: "1",
		AddedAt: r.clock.Now(), State: models.BrushTorrentActive, DownloaderID: 9,
	}, "2026-10-06"))
	old.torrents = []downloader.Torrent{{ID: "o1", InfoHash: "h-old", Tags: tag, Progress: 1, State: downloader.TorrentSeeding, Ratio: 3}}

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, res.Gone, "不在任务当前下载器里不等于没了")
	assert.Equal(t, 1, res.Removed)
	assert.Equal(t, []string{"o1"}, old.removedSingle, "在它自己的下载器里删")

	// 旧下载器连不上：它名下的种子这一轮不动，也不记成没了
	require.NoError(t, repo.RecordAdded(&models.BrushTorrent{
		TaskID: task.ID, InfoHash: "h-gone-dl", SiteName: "hdsky", TorrentID: "2",
		AddedAt: r.clock.Now(), State: models.BrushTorrentActive, DownloaderID: 7,
	}, "2026-10-06"))
	r.clock.Advance(10 * time.Minute)
	res, err = r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, res.Gone)
	assert.Contains(t, res.Errors[0], "下载器 7 名下的 1 个种子这一轮没处理")
	n, err := repo.CountActive(task.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
}

// 同一个站点种子只会被一个刷流任务加入：另一个任务（另一个下载器）跳过它。
func TestBrushMonitor_SkipsTorrentsOfOtherTasksOnSameSite(t *testing.T) {
	r := newBrushRig(t)
	a := r.task(t, func(bt *models.BrushTask) { bt.Name = "a"; bt.Enabled = false })
	b := r.task(t, func(bt *models.BrushTask) { bt.Name = "b"; bt.MaxDownloading = 5 })
	r.addItem(t, "1", nil)
	r.addItem(t, "2", nil)
	require.NoError(t, models.NewBrushRepository(r.db.DB).RecordAdded(&models.BrushTorrent{
		TaskID: a.ID, InfoHash: "elsewhere", SiteName: "hdsky",
		TorrentID: "1", AddedAt: r.clock.Now(), State: models.BrushTorrentActive, DownloaderID: 3,
	}, "2026-10-06"))
	r.mon.cfg.Downloaders = BrushDownloadersFunc(func(id uint) (downloader.Downloader, string, error) {
		if id == 3 {
			return newSchedFakeDownloader("other"), "other", nil
		}
		return r.dl, "qb", nil
	})
	res, err := r.mon.RunTask(context.Background(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Added)
	require.Len(t, r.pushes, 1)
	assert.Equal(t, "2", r.pushes[0].TorrentID)
}

// 推送成功但记录写不进库：把刚加的种子从下载器撤回，本轮停止加种。
func TestBrushMonitor_RollsBackPushWhenRecordFails(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 5 })
	hash := r.addItem(t, "1", nil)
	r.addItem(t, "2", nil)
	require.NoError(t, r.db.DB.Exec(`CREATE TRIGGER fail_brush_insert BEFORE INSERT ON brush_torrents
		BEGIN SELECT RAISE(ABORT, 'disk I/O error'); END`).Error)
	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, res.Added)
	assert.Contains(t, res.Stopped, "记录种子失败")
	require.Len(t, r.pushes, 1, "第一个失败后不再继续")
	assert.Equal(t, []string{hash}, r.dl.removedSingle)
	assert.Equal(t, []bool{true}, r.dl.removeDataFlags)
}

// blockingSite 的 Search 一直等到 ctx 结束。
type blockingSite struct {
	*fakeBrushSite
	entered chan struct{}
}

func (s blockingSite) Search(ctx context.Context, _ v2.SearchQuery) ([]v2.TorrentItem, error) {
	close(s.entered)
	<-ctx.Done()
	return nil, ctx.Err()
}

// Stop 取消正在跑的手动运行并等它退出；之后再运行拿到 ErrBrushStopped。
func TestBrushMonitor_StopCancelsManualRun(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, nil)
	site := blockingSite{fakeBrushSite: r.site, entered: make(chan struct{})}
	r.mon.cfg.Sites = BrushSitesFunc(func(string) (v2.Site, bool) { return site, true })

	done := make(chan error, 1)
	go func() {
		_, err := r.mon.RunTask(context.Background(), task.ID)
		done <- err
	}()
	<-site.entered
	r.mon.Stop()
	// Stop 等的是这一轮本身（runs.Done 在 RunTask 的 defer 里）；RunTask 返回到协程写入 done 之间还隔着一步，
	// 所以这里给一个短时限，而不是非阻塞地读（负载高时那一步会落在读之后）
	select {
	case err := <-done:
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("Stop 之后手动运行没有退出")
	}
	_, err := r.mon.RunTask(context.Background(), task.ID)
	assert.ErrorIs(t, err, ErrBrushStopped)
	r.mon.Start() // 停止之后不再启动
	assert.False(t, r.mon.running)
}

// 任务当前的下载器不可用：旧下载器里的种子照样处理（按下载器 ID 顺序，不受 map 遍历顺序影响）；关闭的任务根本不需要当前下载器。
func TestBrushMonitor_CurrentDownloaderDownStillManagesOldOnes(t *testing.T) {
	for round := range 5 {
		r := newBrushRig(t)
		old := newSchedFakeDownloader("qb-old")
		r.mon.cfg.Downloaders = BrushDownloadersFunc(func(id uint) (downloader.Downloader, string, error) {
			if id == 9 {
				return old, "qb-old", nil
			}
			return nil, "", errors.New("current downloader disabled")
		})
		task := r.task(t, func(bt *models.BrushTask) { bt.RemoveRatio = 2; bt.Enabled = round%2 == 0 })
		require.NoError(t, models.NewBrushRepository(r.db.DB).RecordAdded(&models.BrushTorrent{
			TaskID: task.ID, InfoHash: "h-old", SiteName: "hdsky",
			TorrentID: "1", AddedAt: r.clock.Now(), State: models.BrushTorrentActive, DownloaderID: 9,
		}, "2026-10-06"))
		old.torrents = []downloader.Torrent{{ID: "o1", InfoHash: "h-old", Tags: models.BrushTaskTag(task.ID), Progress: 1, State: downloader.TorrentSeeding, Ratio: 3}}

		res, err := r.mon.RunTask(context.Background(), task.ID)
		assert.Equal(t, []string{"o1"}, old.removedSingle, "第 %d 次：旧下载器里的种子照常删", round)
		assert.Equal(t, 1, res.Removed)
		if task.Enabled {
			assert.ErrorContains(t, err, "下载器不可用", "开启的任务要加种，当前下载器不可用就报错")
		} else {
			assert.NoError(t, err, "关闭的任务不需要当前下载器")
		}
	}
}

// 同一站点的两个任务同时运行：加种串行，两边不会加同一个种子。
func TestBrushMonitor_SameSiteTasksDoNotAddTheSameTorrent(t *testing.T) {
	r := newBrushRig(t)
	a := r.task(t, func(bt *models.BrushTask) { bt.Name = "a"; bt.MaxDownloading = 1 })
	b := r.task(t, func(bt *models.BrushTask) { bt.Name = "b"; bt.MaxDownloading = 1; bt.DownloaderID = 2 })
	r.addItem(t, "1", nil)
	r.addItem(t, "2", nil)
	second := newSchedFakeDownloader("qb2")
	r.mon.cfg.Downloaders = BrushDownloadersFunc(func(id uint) (downloader.Downloader, string, error) {
		if id == 2 {
			return second, "qb2", nil
		}
		return r.dl, "qb", nil
	})
	var mu sync.Mutex
	pushed := map[string]int{}
	release := make(chan struct{})
	first := true
	r.mon.cfg.Push = func(_ context.Context, req ptinternal.PushTorrentRequest) (*ptinternal.PushTorrentResult, error) {
		mu.Lock()
		pushed[req.TorrentID]++
		wait := first
		first = false
		mu.Unlock()
		if wait {
			<-release // 第一个推送卡住，另一个任务这时也在运行
		}
		h, _ := qbit.ComputeTorrentHash(req.TorrentData)
		return &ptinternal.PushTorrentResult{Success: true, TorrentHash: h}, nil
	}
	var wg sync.WaitGroup
	for _, id := range []uint{a.ID, b.ID} {
		wg.Go(func() {
			_, err := r.mon.RunTask(context.Background(), id)
			assert.NoError(t, err)
		})
	}
	time.Sleep(200 * time.Millisecond) // 让第二个任务也走到加种这一步（被站点锁挡住）
	close(release)
	wg.Wait()
	assert.Equal(t, map[string]int{"1": 1, "2": 1}, pushed, "两个任务各加了不同的种子")
}
