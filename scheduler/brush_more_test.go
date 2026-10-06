package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func TestBrushMonitor_DefaultsAndLifecycle(t *testing.T) {
	m := NewBrushMonitor(BrushMonitorConfig{})
	require.NotNil(t, m.cfg.Clock)
	require.NotNil(t, m.cfg.Logger)
	require.NotNil(t, m.cfg.Push)
	assert.Equal(t, brushTick, m.cfg.Tick)
	assert.Equal(t, time.Local, m.cfg.Location)
	m.RunOnce(context.Background()) // 没有数据库：什么也不做
	_, err := m.RunTask(context.Background(), 1)
	require.Error(t, err)

	m.Start()
	m.Start()
	m.Stop()
	m.Stop()

	var nilMon *BrushMonitor
	nilMon.Start()
	nilMon.Stop()
	nilMon.RunOnce(context.Background())
	_, err = nilMon.RunTask(context.Background(), 1)
	require.Error(t, err)
}

// RunOnce 每小时清一次过期采样；读不出任务时直接返回。
func TestBrushMonitor_RunOncePrunesAndSurvivesErrors(t *testing.T) {
	r := newBrushRig(t)
	repo := models.NewBrushRepository(r.db.DB)
	bt := &models.BrushTorrent{TaskID: 1, InfoHash: "h", SiteName: "hdsky", TorrentID: "1", AddedAt: r.clock.Now(), State: models.BrushTorrentActive}
	require.NoError(t, repo.RecordAdded(bt, "2026-10-06"))
	require.NoError(t, repo.RecordSample(bt, models.BrushTorrentSample{At: r.clock.Now().Add(-72 * time.Hour), Uploaded: 1}, 0, 0, 0, time.UTC))
	r.mon.RunOnce(context.Background())
	var n int64
	require.NoError(t, r.db.DB.Model(&models.BrushTorrentSample{}).Count(&n).Error)
	assert.Zero(t, n, "48 小时以前的采样被清掉")

	require.NoError(t, r.db.DB.Migrator().DropTable(&models.BrushTask{}))
	r.mon.RunOnce(context.Background()) // 不 panic
}

// 关闭监控后定时一轮遇到 ErrBrushStopped 直接停止，不再逐个任务报错。
func TestBrushMonitor_RunOnceStopsWhenMonitorStopped(t *testing.T) {
	r := newBrushRig(t)
	r.task(t, nil)
	r.mon.Stop()
	r.mon.RunOnce(context.Background())
	assert.Empty(t, r.site.searches)
}

func TestBrushMonitor_RunWithoutDownloaderService(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, nil)
	r.mon.cfg.Downloaders = nil
	_, err := r.mon.RunTask(context.Background(), task.ID)
	assert.ErrorContains(t, err, "下载器服务不可用")
	r.mon.cfg.Downloaders = BrushDownloadersFunc(func(uint) (downloader.Downloader, string, error) { return r.dl, "qb", nil })
	r.dl.getAllErr = errors.New("timeout")
	_, err = r.mon.RunTask(context.Background(), task.ID)
	assert.ErrorContains(t, err, "读取下载器 qb 的种子失败")
	r.dl.getAllErr = nil
	r.mon.cfg.Sites = nil
	_, err = r.mon.RunTask(context.Background(), task.ID)
	assert.ErrorContains(t, err, "站点服务不可用")
}

// 删不掉的种子留在任务里，记一条错误，下一轮再试。
func TestBrushMonitor_RemoveFailureKeepsTorrent(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, func(bt *models.BrushTask) { bt.RemoveRatio = 1 })
	repo := models.NewBrushRepository(r.db.DB)
	require.NoError(t, repo.RecordAdded(&models.BrushTorrent{
		TaskID: task.ID, InfoHash: "h1", SiteName: "hdsky", TorrentID: "old", Title: "old",
		AddedAt: r.clock.Now(), State: models.BrushTorrentActive, DownloaderID: 1,
	}, "2026-10-06"))
	r.dl.torrents = []downloader.Torrent{{ID: "id1", InfoHash: "h1", Tags: models.BrushTaskTag(task.ID), Progress: 1, State: downloader.TorrentSeeding, Ratio: 3}}
	r.dl.removeErr = errors.New("busy")

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Zero(t, res.Removed)
	require.NotEmpty(t, res.Errors)
	assert.Contains(t, res.Errors[0], "删除 old 失败: busy")
	n, err := repo.CountActive(task.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
}

// 列表里看不出体积的种子，下完种子文件按文件算体积，再判任务总体积。
func TestBrushMonitor_UnknownSizeCheckedAfterDownload(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 5; bt.MaxTotalSizeGB = 6 })
	repo := models.NewBrushRepository(r.db.DB)
	require.NoError(t, repo.RecordAdded(&models.BrushTorrent{
		TaskID: task.ID, InfoHash: "h1", SiteName: "hdsky", TorrentID: "old", Title: "old", SizeBytes: 4 * gib,
		AddedAt: r.clock.Now(), State: models.BrushTorrentActive, DownloaderID: 1,
	}, "2026-10-06"))
	r.dl.torrents = []downloader.Torrent{{ID: "id1", InfoHash: "h1", Tags: models.BrushTaskTag(task.ID), Progress: 0.5, State: downloader.TorrentDownloading}}
	r.addItem(t, "1", func(i *v2.TorrentItem) { i.SizeBytes = 0 })

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, "任务总体积将超过 6.0 GB", res.Stopped)
	assert.Equal(t, []string{"1"}, r.site.downloads)
	assert.Empty(t, r.pushes)
}

// 已经在下载器里的种子不再推送：列表带 hash 的在挑选时排除，不带 hash 的下完种子文件再比对。
func TestBrushMonitor_SkipsTorrentsAlreadyInDownloader(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, nil)
	h1 := r.addItem(t, "1", nil)
	r.site.items[0].InfoHash = strings.ToUpper(h1)
	h2 := r.addItem(t, "2", nil)
	r.dl.torrents = []downloader.Torrent{{ID: h1, InfoHash: h1}, {ID: h2, InfoHash: h2}}

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Eligible, "带 hash 的那个在挑选时就排除")
	assert.Equal(t, []string{"Title 2：下载器里已经有了"}, res.Skipped)
	assert.Empty(t, r.pushes)
}

// 推送时下载器里已经有了（推送结果 Skipped）：记一条跳过，不算加入，接着看下一个。
func TestBrushMonitor_PushSkippedContinues(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, nil)
	r.addItem(t, "1", nil)
	r.addItem(t, "2", nil)
	r.pushResult = &ptinternal.PushTorrentResult{Success: true, Skipped: true, Message: "种子已存在于下载器中"}

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Zero(t, res.Added)
	require.Len(t, res.Skipped, 2)
	assert.Contains(t, res.Skipped[0], "：种子已存在于下载器中")
	assert.Empty(t, res.Stopped)
}

// 种子文件下载失败记一条错误、接着下一个；连续 3 个失败本轮停止。
func TestBrushMonitor_FetchErrorsStopAfterThree(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 10 })
	for _, id := range []string{"1", "2", "3", "4"} {
		r.addItem(t, id, nil)
		delete(r.site.files, id)
	}

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	require.Len(t, res.Errors, 3)
	assert.Contains(t, res.Errors[0], "的种子文件失败: no file")
	assert.Equal(t, "连续 3 个种子文件下载失败，本轮停止加种", res.Stopped)
	assert.Len(t, r.site.downloads, 3)
}

// 这一轮的时间用完（ctx 已结束）时不再加种。
func TestBrushMonitor_StopsAdmittingWhenContextDone(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, nil)
	r.addItem(t, "1", nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := r.mon.RunTask(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, "本轮时间用完", res.Stopped)
	assert.Empty(t, r.pushes)
}

// 推送成功后发现别的任务刚记下了同一个种子：撤回这一份并跳过；撤回失败时提示手动删除。
func TestBrushMonitor_TakenByOtherTaskWithdraws(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, nil)
	other := r.task(t, func(bt *models.BrushTask) { bt.Name = "另一个" })
	r.addItem(t, "1", nil)
	repo := models.NewBrushRepository(r.db.DB)
	push := r.mon.cfg.Push
	r.mon.cfg.Push = func(ctx context.Context, req ptinternal.PushTorrentRequest) (*ptinternal.PushTorrentResult, error) {
		// 另一个进程在这一轮检查之后、记录之前加入了同一个种子
		require.NoError(t, repo.RecordAdded(&models.BrushTorrent{
			TaskID: other.ID, InfoHash: "other-hash", SiteName: "hdsky", TorrentID: req.TorrentID,
			AddedAt: r.clock.Now(), State: models.BrushTorrentActive, DownloaderID: 1,
		}, "2026-10-06"))
		return push(ctx, req)
	}
	r.dl.removeErr = errors.New("busy")

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Zero(t, res.Added)
	assert.Equal(t, []string{"Title 1：已由别的刷流任务加入，撤回了这一份"}, res.Skipped)
	require.Len(t, res.Errors, 1)
	assert.Contains(t, res.Errors[0], "撤回 Title 1 失败，需要在下载器里手动删除")
}

// 站点规定了 H&R 时长（hdfans 72 小时）时，不排除 H&R 的任务可以加入，并记下要求的时长；体积从种子文件算出。
func TestBrushMonitor_RecordsHRSeedTime(t *testing.T) {
	r := newBrushRig(t)
	r.mon.cfg.Sites = BrushSitesFunc(func(string) (v2.Site, bool) { return r.site, true })
	task := r.task(t, func(bt *models.BrushTask) { bt.SiteName = "hdfans"; bt.ExcludeHR = false })
	r.addItem(t, "1", func(i *v2.TorrentItem) { i.SizeBytes = 0 })

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, 1, res.Added, "%+v", res)
	require.Len(t, r.pushes, 1)
	require.NotNil(t, r.pushes[0].Meta)
	assert.True(t, r.pushes[0].Meta.HasHR)
	assert.Equal(t, 72, r.pushes[0].Meta.HRSeedTimeH)
	assert.EqualValues(t, 4*gib, r.pushes[0].Meta.SizeBytes)
}

// 低速规则经监控走通：从窗口起点的采样到现在，平均上传低于阈值就删。
func TestBrushMonitor_RemovesLowSpeed(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, func(bt *models.BrushTask) { bt.RemoveLowSpeedKBs = 100; bt.RemoveLowSpeedWindowMin = 30 })
	repo := models.NewBrushRepository(r.db.DB)
	bt := &models.BrushTorrent{
		TaskID: task.ID, InfoHash: "h1", SiteName: "hdsky", TorrentID: "old", Title: "old",
		AddedAt: r.clock.Now().Add(-2 * time.Hour), State: models.BrushTorrentActive, DownloaderID: 1,
	}
	require.NoError(t, repo.RecordAdded(bt, "2026-10-06"))
	require.NoError(t, repo.RecordSample(bt, models.BrushTorrentSample{At: r.clock.Now().Add(-40 * time.Minute), Uploaded: 1 << 20}, 1, 1, 3600, time.UTC))
	r.dl.torrents = []downloader.Torrent{{
		ID: "id1", InfoHash: "h1", Tags: models.BrushTaskTag(task.ID), Progress: 1, State: downloader.TorrentSeeding,
		TotalUploaded: 2 << 20, SeedingTime: 7200,
	}}

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Removed, res.Errors)
	ended, _, err := repo.ListTorrents(task.ID, models.BrushTorrentRemoved, 1, 10)
	require.NoError(t, err)
	require.Len(t, ended, 1)
	assert.Contains(t, ended[0].RemoveReason, "最近 30 分钟平均上传")
}

// 定时一轮里单个任务失败只记日志，不影响其他任务；读不出采样表时清理失败也只记日志。
func TestBrushMonitor_RunOnceLogsFailures(t *testing.T) {
	r := newBrushRig(t)
	r.task(t, func(bt *models.BrushTask) { bt.DownloaderID = 2 })
	ok := r.task(t, func(bt *models.BrushTask) { bt.Name = "正常" })
	r.addItem(t, "1", nil)
	require.NoError(t, r.db.DB.Migrator().DropTable(&models.BrushTorrentSample{}))
	r.mon.RunOnce(context.Background())
	got, err := models.NewBrushRepository(r.db.DB).GetTask(ok.ID)
	require.NoError(t, err)
	assert.NotNil(t, got.LastRunAt, "第一个任务失败之后第二个照常运行")
}

func TestBrushMonitor_FetchUsesDownhash(t *testing.T) {
	r := newBrushRig(t)
	site := &hashSite{fakeBrushSite: r.site}
	data, err := r.mon.fetchTorrent(context.Background(), site, v2.TorrentItem{ID: "9", DownloadURL: "/api/site/hddolby/torrent/9/download?downhash=abc"})
	require.NoError(t, err)
	assert.Equal(t, "9:abc", string(data))
	r.site.files["9"] = []byte("plain")
	data, err = r.mon.fetchTorrent(context.Background(), site, v2.TorrentItem{ID: "9"})
	require.NoError(t, err)
	assert.Equal(t, "plain", string(data), "没有 downhash 时走普通下载")
}

type hashSite struct{ *fakeBrushSite }

func (h *hashSite) DownloadWithHash(_ context.Context, id, hash string) ([]byte, error) {
	return []byte(fmt.Sprintf("%s:%s", id, hash)), nil
}

func TestBrushRunResultSummary(t *testing.T) {
	s := BrushRunResult{Listed: 3, Eligible: 2, Added: 1, Removed: 2, Gone: 1, Stopped: "限额已满"}.Summary()
	assert.Equal(t, "列表 3 个、符合条件 2 个、加入 1 个，删除 2 个，1 个已不在下载器里，限额已满", s)
}

func TestWithProgress(t *testing.T) {
	assert.InDelta(t, 1, withProgress(models.BrushTorrent{}, downloader.Torrent{Progress: 1, State: downloader.TorrentSeeding}).Progress, 1e-9)
	assert.Less(t, withProgress(models.BrushTorrent{}, downloader.Torrent{Progress: 1, State: downloader.TorrentChecking}).Progress, 1.0)
	assert.InDelta(t, 0.4, withProgress(models.BrushTorrent{}, downloader.Torrent{Progress: 0.4}).Progress, 1e-9)
}

func TestManager_BrushMonitorAndDownloader(t *testing.T) {
	db := setupTestDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	m := &Manager{downloaderManager: downloader.NewDownloaderManager()}
	first := NewBrushMonitor(BrushMonitorConfig{DB: db.DB})
	m.SetBrushMonitor(first)
	assert.Same(t, first, m.GetBrushMonitor())
	second := NewBrushMonitor(BrushMonitorConfig{DB: db.DB})
	m.SetBrushMonitor(second)
	_, err := first.RunTask(context.Background(), 1)
	assert.ErrorIs(t, err, ErrBrushStopped, "换上新监控时停掉旧的")
	m.SetBrushMonitor(second)

	_, _, err = m.BrushDownloader(5)
	assert.ErrorContains(t, err, "不存在")
	ds := models.DownloaderSetting{Name: "qb1", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true}
	require.NoError(t, db.DB.Create(&ds).Error)
	fake := newSchedFakeDownloader("qb1")
	registerFakeDownloader(t, m.downloaderManager, fake, true)
	dl, name, err := m.BrushDownloader(ds.ID)
	require.NoError(t, err)
	assert.Equal(t, "qb1", name)
	assert.NotNil(t, dl)
	require.NoError(t, db.DB.Model(&ds).Update("enabled", false).Error)
	_, _, err = m.BrushDownloader(ds.ID)
	assert.ErrorContains(t, err, "未启用")

	other := models.DownloaderSetting{Name: "not-registered", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true}
	require.NoError(t, db.DB.Create(&other).Error)
	_, _, err = m.BrushDownloader(other.ID)
	assert.Error(t, err, "下载器管理器里没有这个实例")

	saved := global.GlobalDB
	global.GlobalDB = nil
	_, _, err = m.BrushDownloader(ds.ID)
	assert.ErrorContains(t, err, "数据库未初始化")
	global.GlobalDB = saved

	m.StopAll()
	_, err = second.RunTask(context.Background(), 1)
	assert.ErrorIs(t, err, ErrBrushStopped, "StopAll 停掉刷流监控")
}
