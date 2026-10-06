package scheduler

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

const (
	brushTick         = time.Minute
	brushStartupDelay = 45 * time.Second
	// 一轮运行（列种子、搜索、逐个下载推送、删种）的总预算
	brushRunTimeout    = 5 * time.Minute
	brushSearchTimeout = 60 * time.Second
	brushFetchTimeout  = 60 * time.Second
	// 采样只为低速规则保留；窗口最长 24 小时，留两倍
	brushSampleRetention = 48 * time.Hour
	brushPruneEvery      = time.Hour
	brushSource          = "brush"
)

// ErrBrushBusy 表示这个任务正在运行（定时一轮与手动「立即运行」不会同时跑）。
var ErrBrushBusy = errors.New("刷流任务正在运行，请稍后再试")

// BrushSites 按站点名取共享站点实例；生产环境取 UserInfoService 里注册的实例，与搜索、登录探测共用限速器。
type BrushSites interface {
	BrushSite(siteName string) (v2.Site, bool)
}

// BrushSitesFunc 把函数适配为 BrushSites。
type BrushSitesFunc func(siteName string) (v2.Site, bool)

func (f BrushSitesFunc) BrushSite(siteName string) (v2.Site, bool) { return f(siteName) }

// BrushDownloaders 按 ID 取下载器实例（列种子、删种）与它的名称。
type BrushDownloaders interface {
	BrushDownloader(id uint) (downloader.Downloader, string, error)
}

// BrushDownloadersFunc 把函数适配为 BrushDownloaders。
type BrushDownloadersFunc func(id uint) (downloader.Downloader, string, error)

func (f BrushDownloadersFunc) BrushDownloader(id uint) (downloader.Downloader, string, error) {
	return f(id)
}

// BrushPushFunc 是推送入口，默认 internal.PushTorrentToDownloader（磁盘预算与站点容量闸门都在里面）。
type BrushPushFunc func(ctx context.Context, req ptinternal.PushTorrentRequest) (*ptinternal.PushTorrentResult, error)

// BrushMonitorConfig 是 BrushMonitor 的依赖。
type BrushMonitorConfig struct {
	DB          *gorm.DB
	Sites       BrushSites
	Downloaders BrushDownloaders
	Push        BrushPushFunc
	Clock       sitelogin.Clock
	Logger      *zap.SugaredLogger
	Tick        time.Duration
	// Location 是统计按天归档的时区，默认进程时区（time.Local）。
	Location *time.Location
}

// BrushRunResult 是一轮运行的摘要，也是「立即运行」接口的返回值。
type BrushRunResult struct {
	TaskID   uint     `json:"task_id"`
	Sampled  int      `json:"sampled"`
	Removed  int      `json:"removed"`
	Gone     int      `json:"gone"`
	Listed   int      `json:"listed"`
	Eligible int      `json:"eligible"`
	Added    int      `json:"added"`
	Skipped  []string `json:"skipped,omitempty"`
	Errors   []string `json:"errors,omitempty"`
	// Stopped 说明这一轮为什么没有继续加种（限额已满、推送被闸门拒绝等）；为空表示列表里能加的都加了。
	Stopped string `json:"stopped,omitempty"`
}

// Summary 是写进任务 last_result 的一句话。
func (r BrushRunResult) Summary() string {
	parts := []string{fmt.Sprintf("列表 %d 个、符合条件 %d 个、加入 %d 个", r.Listed, r.Eligible, r.Added)}
	if r.Removed > 0 {
		parts = append(parts, fmt.Sprintf("删除 %d 个", r.Removed))
	}
	if r.Gone > 0 {
		parts = append(parts, fmt.Sprintf("%d 个已不在下载器里", r.Gone))
	}
	if r.Stopped != "" {
		parts = append(parts, r.Stopped)
	}
	return strings.Join(parts, "，")
}

// BrushMonitor 按任务的间隔运行刷流：每一轮先给任务名下的种子采样、按删种规则删除，再从站点的免费列表里
// 按入场条件挑种子，在限额之内经 PushTorrentToDownloader 推送（磁盘与站点容量闸门不变）。
// 只处理带本任务标签、并且有 BrushTorrent 记录的种子。
type BrushMonitor struct {
	cfg  BrushMonitorConfig
	repo *models.BrushRepository

	mu        sync.Mutex
	running   bool
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	busy      map[uint]bool
	lastPrune time.Time
}

// NewBrushMonitor 构造刷流监控，不启动；调用 Start 开始调度。
func NewBrushMonitor(cfg BrushMonitorConfig) *BrushMonitor {
	if cfg.Clock == nil {
		cfg.Clock = sitelogin.NewRealClock()
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop().Sugar()
	}
	if cfg.Tick <= 0 {
		cfg.Tick = brushTick
	}
	if cfg.Location == nil {
		cfg.Location = time.Local
	}
	if cfg.Push == nil {
		cfg.Push = ptinternal.PushTorrentToDownloader
	}
	return &BrushMonitor{cfg: cfg, repo: models.NewBrushRepository(cfg.DB), busy: map[uint]bool{}}
}

// Start 启动调度循环；重复调用无效果。
func (m *BrushMonitor) Start() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.running = true
	m.wg.Go(func() { m.loop(ctx) })
}

// Stop 停止调度循环并等正在跑的一轮退出。
func (m *BrushMonitor) Stop() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	cancel := m.cancel
	m.mu.Unlock()
	cancel()
	m.wg.Wait()
}

func (m *BrushMonitor) loop(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(brushStartupDelay):
		m.RunOnce(ctx)
	}
	ticker := time.NewTicker(m.cfg.Tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.RunOnce(ctx)
		}
	}
}

func (m *BrushMonitor) dayOf(t time.Time) string {
	return t.In(m.cfg.Location).Format("2006-01-02")
}

// RunOnce 依次运行到期的已启用任务（距上一轮满了任务的间隔），并每小时清理一次过期采样。
func (m *BrushMonitor) RunOnce(ctx context.Context) {
	if m == nil || m.cfg.DB == nil {
		return
	}
	tasks, err := m.repo.ListTasks()
	if err != nil {
		m.cfg.Logger.Warnf("[刷流] 读取任务失败: %v", err)
		return
	}
	now := m.cfg.Clock.Now()
	for _, task := range tasks {
		if ctx.Err() != nil {
			return
		}
		if !task.Enabled || !brushDue(task, now) {
			continue
		}
		if _, err := m.RunTask(ctx, task.ID); err != nil && !errors.Is(err, ErrBrushBusy) {
			m.cfg.Logger.Warnf("[刷流] 任务 %s 运行失败: %v", task.Name, err)
		}
	}
	m.maybePrune(now)
}

func brushDue(task models.BrushTask, now time.Time) bool {
	if task.LastRunAt == nil {
		return true
	}
	interval := time.Duration(max(task.IntervalMin, 1)) * time.Minute
	return !now.Before(task.LastRunAt.Add(interval))
}

func (m *BrushMonitor) maybePrune(now time.Time) {
	m.mu.Lock()
	if now.Sub(m.lastPrune) < brushPruneEvery {
		m.mu.Unlock()
		return
	}
	m.lastPrune = now
	m.mu.Unlock()
	if n, err := m.repo.PruneSamples(now.Add(-brushSampleRetention)); err != nil {
		m.cfg.Logger.Warnf("[刷流] 清理采样失败: %v", err)
	} else if n > 0 {
		m.cfg.Logger.Debugf("[刷流] 清理了 %d 条过期采样", n)
	}
}

// RunTask 立即运行一轮（定时调度与「立即运行」接口共用）；同一任务正在运行时返回 ErrBrushBusy。
// 关闭的任务也能手动运行：删种照常执行，加种只在任务开启时进行。
func (m *BrushMonitor) RunTask(ctx context.Context, taskID uint) (BrushRunResult, error) {
	res := BrushRunResult{TaskID: taskID}
	if m == nil || m.cfg.DB == nil {
		return res, errors.New("刷流服务不可用")
	}
	m.mu.Lock()
	if m.busy[taskID] {
		m.mu.Unlock()
		return res, ErrBrushBusy
	}
	m.busy[taskID] = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.busy, taskID)
		m.mu.Unlock()
	}()

	task, err := m.repo.GetTask(taskID)
	if err != nil {
		return res, err
	}
	ctx, cancel := context.WithTimeout(ctx, brushRunTimeout)
	defer cancel()

	runErr := m.run(ctx, task, &res)
	now := m.cfg.Clock.Now()
	errText := ""
	if runErr != nil {
		errText = runErr.Error()
	} else if len(res.Errors) > 0 {
		errText = strings.Join(res.Errors, "；")
	}
	if err := m.repo.MarkRun(task.ID, now, res.Summary(), errText); err != nil {
		m.cfg.Logger.Warnf("[刷流] 记录任务 %s 的运行结果失败: %v", task.Name, err)
	}
	if runErr != nil {
		return res, runErr
	}
	m.cfg.Logger.Infof("[刷流] %s：%s", task.Name, res.Summary())
	return res, nil
}

func (m *BrushMonitor) run(ctx context.Context, task *models.BrushTask, res *BrushRunResult) error {
	if m.cfg.Downloaders == nil {
		return errors.New("下载器服务不可用")
	}
	dl, dlName, err := m.cfg.Downloaders.BrushDownloader(task.DownloaderID)
	if err != nil {
		return fmt.Errorf("下载器不可用: %w", err)
	}
	torrents, err := dl.GetAllTorrents()
	if err != nil {
		return fmt.Errorf("读取下载器 %s 的种子失败: %w", dlName, err)
	}
	byHash := make(map[string]downloader.Torrent, len(torrents))
	for _, t := range torrents {
		if t.InfoHash != "" {
			byHash[strings.ToLower(t.InfoHash)] = t
		}
	}

	active, err := m.sampleAndRemove(task, dl, byHash, res)
	if err != nil {
		return err
	}
	if !task.Enabled {
		res.Stopped = "任务已关闭，不加种"
		return nil
	}
	return m.admit(ctx, task, active, byHash, res)
}

// sampleAndRemove 给任务名下仍在做的种子采样并按规则删除；返回删完之后仍在做的种子。
func (m *BrushMonitor) sampleAndRemove(task *models.BrushTask, dl downloader.Downloader, byHash map[string]downloader.Torrent, res *BrushRunResult) ([]models.BrushTorrent, error) {
	rows, err := m.repo.ActiveTorrents(task.ID)
	if err != nil {
		return nil, err
	}
	taskTag := models.BrushTaskTag(task.ID)
	kept := make([]models.BrushTorrent, 0, len(rows))
	for i := range rows {
		bt := rows[i]
		now := m.cfg.Clock.Now()
		day := m.dayOf(now)
		t, ok := byHash[bt.InfoHash]
		switch {
		case !ok:
			res.Gone++
			if markErr := m.repo.MarkEnded(&bt, models.BrushTorrentGone, "下载器里已经没有这个种子", now, day); markErr != nil {
				res.Errors = append(res.Errors, markErr.Error())
			}
			continue
		case !hasTag(t, taskTag):
			// 用户摘掉了任务标签：不再归这个任务管，删种规则也不再作用于它
			res.Gone++
			if markErr := m.repo.MarkEnded(&bt, models.BrushTorrentGone, "种子不再带任务标签，交还给用户", now, day); markErr != nil {
				res.Errors = append(res.Errors, markErr.Error())
			}
			continue
		}
		sample := models.BrushTorrentSample{At: now, Uploaded: t.TotalUploaded, Downloaded: t.TotalDownloaded}
		if sampleErr := m.repo.RecordSample(&bt, sample, t.Progress, t.Ratio, t.SeedingTime, day); sampleErr != nil {
			res.Errors = append(res.Errors, sampleErr.Error())
		} else {
			res.Sampled++
		}
		var samples []models.BrushTorrentSample
		if task.RemoveLowSpeedKBs > 0 && task.RemoveLowSpeedWindowMin > 0 {
			samples, err = m.repo.SamplesSince(bt.ID, now.Add(-time.Duration(task.RemoveLowSpeedWindowMin)*time.Minute))
			if err != nil {
				res.Errors = append(res.Errors, err.Error())
			}
		}
		remove, reason := brushRemoval(*task, bt, t, samples, now)
		if !remove {
			kept = append(kept, bt)
			continue
		}
		if err := dl.RemoveTorrent(t.ID, task.RemoveWithData); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("删除 %s 失败: %v", bt.Title, err))
			kept = append(kept, bt)
			continue
		}
		res.Removed++
		m.cfg.Logger.Infof("[刷流] %s 删除 %s：%s", task.Name, bt.Title, reason)
		if err := m.repo.MarkEnded(&bt, models.BrushTorrentRemoved, reason, now, day); err != nil {
			res.Errors = append(res.Errors, err.Error())
		}
	}
	return kept, nil
}

// admit 在限额之内从站点的免费列表里加种。
func (m *BrushMonitor) admit(ctx context.Context, task *models.BrushTask, active []models.BrushTorrent, byHash map[string]downloader.Torrent, res *BrushRunResult) error {
	downloading := 0
	var totalSize int64
	for _, bt := range active {
		totalSize += bt.SizeBytes
		if t, ok := byHash[bt.InfoHash]; ok && !(t.IsCompleted || t.Progress >= 1) {
			downloading++
		}
	}
	now := m.cfg.Clock.Now()
	day := m.dayOf(now)
	var todayAdded int64
	if stats, err := m.repo.DailyStats(task.ID, day, day); err == nil && len(stats) > 0 {
		todayAdded = stats[0].AddedBytes
	}
	if stop := brushLimitReached(task, downloading, totalSize, todayAdded, 0); stop != "" {
		res.Stopped = stop
		return nil
	}

	if m.cfg.Sites == nil {
		return errors.New("站点服务不可用")
	}
	site, ok := m.cfg.Sites.BrushSite(task.SiteName)
	if !ok || site == nil {
		return fmt.Errorf("站点 %s 未启用或未注册", task.SiteName)
	}
	searchCtx, cancel := context.WithTimeout(ctx, brushSearchTimeout)
	items, err := site.Search(searchCtx, v2.SearchQuery{FreeOnly: true})
	cancel()
	if err != nil {
		return fmt.Errorf("读取 %s 的种子列表失败: %w", task.SiteName, err)
	}
	res.Listed = len(items)

	var def *v2.SiteDefinition
	if d, ok := v2.GetDefinitionRegistry().Get(task.SiteName); ok {
		def = d
	}
	siteHR := def != nil && def.HREnabled
	seen, err := m.repo.SeenTorrentIDs(task.ID)
	if err != nil {
		return err
	}
	candidates := make([]v2.TorrentItem, 0, len(items))
	for _, it := range items {
		if seen[it.ID] {
			continue
		}
		if it.InfoHash != "" {
			if _, inDL := byHash[strings.ToLower(it.InfoHash)]; inDL {
				continue
			}
		}
		if ok, _ := admitBrushItem(*task, it, siteHR, now); ok {
			candidates = append(candidates, it)
		}
	}
	res.Eligible = len(candidates)
	sortBrushCandidates(candidates)

	for _, it := range candidates {
		if ctx.Err() != nil {
			res.Stopped = "本轮时间用完"
			return nil
		}
		if stop := brushLimitReached(task, downloading, totalSize, todayAdded, it.SizeBytes); stop != "" {
			res.Stopped = stop
			return nil
		}
		data, err := m.fetchTorrent(ctx, site, it)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("下载 %s 的种子文件失败: %v", it.Title, err))
			continue
		}
		hash, err := qbit.ComputeTorrentHash(data)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s 的种子文件无法解析: %v", it.Title, err))
			continue
		}
		hash = strings.ToLower(hash)
		if _, inDL := byHash[hash]; inDL {
			res.Skipped = append(res.Skipped, it.Title+"：下载器里已经有了")
			continue
		}
		size := it.SizeBytes
		if size <= 0 {
			if s, sizeErr := qbit.ComputeTorrentSize(data); sizeErr == nil {
				size = s
			}
			if stop := brushLimitReached(task, downloading, totalSize, todayAdded, size); stop != "" {
				res.Stopped = stop
				return nil
			}
		}
		hasHR := it.HasHR || siteHR
		hrHours := 0
		if hasHR && def != nil {
			hrHours = def.CalcHRSeedTimeH(size)
		}
		var freeEnd *time.Time
		if !it.DiscountEndTime.IsZero() {
			fe := it.DiscountEndTime.UTC()
			freeEnd = &fe
		}
		pushRes, err := m.cfg.Push(ctx, ptinternal.PushTorrentRequest{
			SiteID:       task.SiteName,
			TorrentID:    it.ID,
			TorrentData:  data,
			Title:        it.Title,
			Category:     task.Category,
			Tags:         strings.Join(task.TagList(), ","),
			SavePath:     task.SavePath,
			DownloaderID: task.DownloaderID,
			Source:       brushSource,
			Meta: &ptinternal.PushTorrentMeta{
				SizeBytes: size, HasHR: hasHR, HRSeedTimeH: hrHours,
				IsFree: v2.IsFreeTorrent(it.DiscountLevel), FreeLevel: string(it.DiscountLevel), FreeEndTime: freeEnd,
			},
		})
		if err != nil {
			// 推送出错（下载器不可达等）：这一轮不再继续，免得对着同一个错误把列表里的种子全下一遍
			res.Errors = append(res.Errors, fmt.Sprintf("推送 %s 失败: %v", it.Title, err))
			res.Stopped = "推送失败，本轮停止加种"
			return nil
		}
		if pushRes.Skipped {
			res.Skipped = append(res.Skipped, it.Title+"："+pushRes.Message)
			continue
		}
		if !pushRes.Success {
			// 磁盘保护或站点容量闸门拒绝：后面的种子同样会被拒，停在这里
			res.Stopped = "推送被拒绝：" + pushRes.Message
			return nil
		}
		now = m.cfg.Clock.Now()
		bt := &models.BrushTorrent{
			TaskID: task.ID, InfoHash: hash, SiteName: task.SiteName, TorrentID: it.ID, Title: it.Title,
			SizeBytes: size, Discount: string(it.DiscountLevel), FreeEndAt: freeEnd, HasHR: hasHR, HRSeedTimeH: hrHours,
			DownloaderID: task.DownloaderID, State: models.BrushTorrentActive, AddedAt: now.UTC(),
		}
		if err := m.repo.RecordAdded(bt, m.dayOf(now)); err != nil {
			res.Errors = append(res.Errors, err.Error())
		}
		res.Added++
		downloading++
		totalSize += size
		todayAdded += size
		m.cfg.Logger.Infof("[刷流] %s 加入 %s（%s，%.2f GB）", task.Name, it.Title, it.DiscountLevel, float64(size)/gib)
	}
	return nil
}

// brushLimitReached 返回已经达到的限额（加上下一个种子的体积 next 之后）；都没到时为空。
func brushLimitReached(task *models.BrushTask, downloading int, totalSize, todayAdded, next int64) string {
	if task.MaxDownloading > 0 && downloading >= task.MaxDownloading {
		return fmt.Sprintf("同时下载已有 %d 个，达到上限", downloading)
	}
	if task.MaxTotalSizeGB > 0 && float64(totalSize+next) > task.MaxTotalSizeGB*gib {
		return fmt.Sprintf("任务总体积将超过 %.1f GB", task.MaxTotalSizeGB)
	}
	if task.MaxDailyDownloadGB > 0 && float64(todayAdded+next) > task.MaxDailyDownloadGB*gib {
		return fmt.Sprintf("今天加入的体积将超过 %.1f GB", task.MaxDailyDownloadGB)
	}
	return ""
}

// fetchTorrent 经站点实例下载种子文件（走站点限速）；HDDolby 这类要 hash 的站点从下载地址里取 downhash。
func (m *BrushMonitor) fetchTorrent(ctx context.Context, site v2.Site, it v2.TorrentItem) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, brushFetchTimeout)
	defer cancel()
	if hd, ok := site.(v2.HashDownloader); ok {
		if h := downhashOf(it.DownloadURL); h != "" {
			return hd.DownloadWithHash(ctx, it.ID, h)
		}
	}
	return site.Download(ctx, it.ID)
}

func downhashOf(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Query().Get("downhash")
}
