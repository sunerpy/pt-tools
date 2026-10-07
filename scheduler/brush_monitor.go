package scheduler

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
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
	// 一轮里连续这么多个种子文件下载或解析失败就停止加种
	brushMaxFetchFailures = 3
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

	// root 在 Stop 时取消：定时与手动运行的每一轮都挂在它下面，Stop 等它们全部退出
	root       context.Context
	rootCancel context.CancelFunc
	runs       sync.WaitGroup

	mu        sync.Mutex
	running   bool
	stopped   bool
	wg        sync.WaitGroup
	busy      map[uint]bool
	siteLocks map[string]*sync.Mutex
	lastPrune time.Time
}

// ErrBrushStopped 表示刷流监控已经停止（进程正在退出）。
var ErrBrushStopped = errors.New("刷流服务已停止")

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
	root, cancel := context.WithCancel(context.Background())
	return &BrushMonitor{
		cfg: cfg, repo: models.NewBrushRepository(cfg.DB), busy: map[uint]bool{},
		root: root, rootCancel: cancel,
	}
}

// Start 启动调度循环；重复调用、Stop 之后再调用都无效果。
func (m *BrushMonitor) Start() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running || m.stopped {
		return
	}
	m.running = true
	m.wg.Go(func() { m.loop(m.root) })
}

// Stop 停止调度、取消正在跑的定时或手动的一轮，并等它们全部退出；之后的 RunTask 返回 ErrBrushStopped。
func (m *BrushMonitor) Stop() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return
	}
	m.stopped = true
	m.running = false
	m.mu.Unlock()
	m.rootCancel()
	m.wg.Wait()
	m.runs.Wait()
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
		if !brushDue(task, now) {
			continue
		}
		if !task.Enabled {
			// 关闭的任务不再加种，但已经加入的种子仍按删种规则处理，直到删完
			n, err := m.repo.CountActive(task.ID)
			if err != nil || n == 0 {
				continue
			}
		}
		_, err := m.RunTask(ctx, task.ID)
		switch {
		case errors.Is(err, ErrBrushStopped):
			return
		case err != nil && !errors.Is(err, ErrBrushBusy):
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

// WithTaskLock 在任务没有运行时执行 fn（期间定时与手动运行都会拿到 ErrBrushBusy）；任务正在运行时返回 ErrBrushBusy。
// 删除与修改任务走这里：一轮运行用的是开始时读到的配置，改动不能落在一轮的中途。
func (m *BrushMonitor) WithTaskLock(taskID uint, fn func() error) error {
	if m == nil {
		return fn()
	}
	m.mu.Lock()
	if m.busy[taskID] {
		m.mu.Unlock()
		return ErrBrushBusy
	}
	m.busy[taskID] = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.busy, taskID)
		m.mu.Unlock()
	}()
	return fn()
}

// RunTask 立即运行一轮（定时调度与「立即运行」接口共用）；同一任务正在运行时返回 ErrBrushBusy，
// 监控已停止时返回 ErrBrushStopped。关闭的任务也能手动运行：删种照常执行，加种只在任务开启时进行。
func (m *BrushMonitor) RunTask(ctx context.Context, taskID uint) (BrushRunResult, error) {
	res := BrushRunResult{TaskID: taskID}
	if m == nil || m.cfg.DB == nil {
		return res, errors.New("刷流服务不可用")
	}
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return res, ErrBrushStopped
	}
	if m.busy[taskID] {
		m.mu.Unlock()
		return res, ErrBrushBusy
	}
	m.busy[taskID] = true
	m.runs.Add(1)
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.busy, taskID)
		m.mu.Unlock()
		m.runs.Done()
	}()

	task, err := m.repo.GetTask(taskID)
	if err != nil {
		return res, err
	}
	ctx, cancel := context.WithTimeout(ctx, brushRunTimeout)
	defer cancel()
	stopWithRoot := context.AfterFunc(m.root, cancel)
	defer stopWithRoot()

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
	rows, err := m.repo.ActiveTorrents(task.ID)
	if err != nil {
		return err
	}
	// 种子按它们各自加入时的下载器管理：任务换了下载器之后，旧下载器里的种子照样采样、按规则删除。
	// 有种子的下载器按 ID 顺序逐个处理，哪个连不上都不影响其他的；任务当前的下载器只在加种时才需要。
	groups := map[uint][]models.BrushTorrent{}
	for _, bt := range rows {
		id := bt.DownloaderID
		if id == 0 {
			id = task.DownloaderID
		}
		groups[id] = append(groups[id], bt)
	}
	ids := make([]uint, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	type listed struct {
		dl     downloader.Downloader
		byHash map[string]downloader.Torrent
		err    error
	}
	seen := map[uint]listed{}
	list := func(id uint) listed {
		if l, ok := seen[id]; ok {
			return l
		}
		var l listed
		dl, name, err := m.cfg.Downloaders.BrushDownloader(id)
		if err != nil {
			l.err = fmt.Errorf("下载器不可用: %w", err)
		} else if torrents, err := dl.GetAllTorrents(); err != nil {
			l.err = fmt.Errorf("读取下载器 %s 的种子失败: %w", name, err)
		} else {
			l.dl = dl
			l.byHash = make(map[string]downloader.Torrent, len(torrents))
			for _, t := range torrents {
				if t.InfoHash != "" {
					l.byHash[strings.ToLower(t.InfoHash)] = t
				}
			}
		}
		seen[id] = l
		return l
	}
	var active []models.BrushTorrent
	for _, id := range ids {
		l := list(id)
		if l.err != nil {
			// 这个下载器暂时连不上：它名下的种子保持原状，不当成已经没了
			res.Errors = append(res.Errors, fmt.Sprintf("下载器 %d 名下的 %d 个种子这一轮没处理: %v", id, len(groups[id]), l.err))
			active = append(active, groups[id]...)
			continue
		}
		active = append(active, m.sampleAndRemove(task, l.dl, groups[id], l.byHash, res)...)
	}
	if !task.Enabled {
		res.Stopped = "任务已关闭，不加种"
		return nil
	}
	cur := list(task.DownloaderID)
	if cur.err != nil {
		return cur.err
	}
	return m.admit(ctx, task, cur.dl, active, cur.byHash, res)
}

// sampleAndRemove 给一个下载器里的刷流种子采样并按规则删除；返回删完之后仍在做的种子（含这轮没法判断的）。
func (m *BrushMonitor) sampleAndRemove(task *models.BrushTask, dl downloader.Downloader, rows []models.BrushTorrent, byHash map[string]downloader.Torrent, res *BrushRunResult) []models.BrushTorrent {
	taskTag := models.BrushTaskTag(task.ID)
	kept := make([]models.BrushTorrent, 0, len(rows))
	all := make([]downloader.Torrent, 0, len(byHash))
	for _, t := range byHash {
		all = append(all, t)
	}
	removed := map[string]bool{} // 这一轮已删掉的（键见 downloader.TorrentKey）
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
		if sampleErr := m.repo.RecordSample(&bt, sample, t.Progress, t.Ratio, t.SeedingTime, m.cfg.Location); sampleErr != nil {
			res.Errors = append(res.Errors, sampleErr.Error())
		} else {
			res.Sampled++
		}
		var samples []models.BrushTorrentSample
		if task.RemoveLowSpeedKBs > 0 && task.RemoveLowSpeedWindowMin > 0 {
			var err error
			samples, err = m.repo.SamplesSince(bt.ID, now.Add(-time.Duration(task.RemoveLowSpeedWindowMin)*time.Minute))
			if err != nil {
				res.Errors = append(res.Errors, err.Error())
			}
		}
		remove, reason := brushRemoval(*task, bt, t, samples, now)
		if !remove {
			kept = append(kept, withProgress(bt, t))
			continue
		}
		// 数据还被别的种子用着（如给它加的辅种）时只删种子、保留数据
		withData := task.RemoveWithData && !downloader.SharesData(all, t, removed)
		if task.RemoveWithData && !withData {
			reason += "；数据还被别的种子用着，保留数据"
		}
		if err := dl.RemoveTorrent(t.ID, withData); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("删除 %s 失败: %v", bt.Title, err))
			kept = append(kept, withProgress(bt, t))
			continue
		}
		removed[downloader.TorrentKey(t)] = true
		res.Removed++
		m.cfg.Logger.Infof("[刷流] %s 删除 %s：%s", task.Name, bt.Title, reason)
		if markErr := m.repo.MarkEnded(&bt, models.BrushTorrentRemoved, reason, now, day); markErr != nil {
			res.Errors = append(res.Errors, markErr.Error())
		}
	}
	return kept
}

// withProgress 记下这一轮看到的完成状态，供限额计算用（Progress 字段用 brushCompleted 的口径：没真正下完就小于 1）。
func withProgress(bt models.BrushTorrent, t downloader.Torrent) models.BrushTorrent {
	if brushCompleted(t) {
		bt.Progress = 1
	} else if t.Progress >= 1 {
		bt.Progress = 0.999
	} else {
		bt.Progress = t.Progress
	}
	return bt
}

// admit 在限额之内从站点的免费列表里加种。
func (m *BrushMonitor) admit(ctx context.Context, task *models.BrushTask, dl downloader.Downloader, active []models.BrushTorrent, byHash map[string]downloader.Torrent, res *BrushRunResult) error {
	downloading := 0
	var totalSize int64
	for _, bt := range active {
		totalSize += bt.SizeBytes
		if bt.Progress < 1 {
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
	// 同一站点的加种串行：「读已加过的种子 → 推送 → 记录」之间不能插进另一个任务的同一段，
	// 否则两个任务会同时把同一个种子各加一份（库里另有 site_name+torrent_id 唯一约束兜底）
	unlock := m.lockSite(task.SiteName)
	defer unlock()
	// 任何刷流任务在这个站点加过的种子都不再加：两个任务各加一份会争用同一条 TorrentInfo 记录
	seen, err := m.repo.SeenSiteTorrentIDs(task.SiteName)
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
		hr := 0
		if (it.HasHR || siteHR) && def != nil {
			hr = def.CalcHRSeedTimeH(it.SizeBytes)
		}
		if ok, _ := admitBrushItem(*task, it, siteHR, hr, now); ok {
			candidates = append(candidates, it)
		}
	}
	res.Eligible = len(candidates)
	sortBrushCandidates(candidates)

	fetchFailures := 0
	for _, it := range candidates {
		if ctx.Err() != nil {
			res.Stopped = "本轮时间用完"
			return nil
		}
		// 连着几个种子都下不下来（站点出错、返回的不是种子文件）：这一轮不再继续，免得把整张列表都下一遍
		if fetchFailures >= brushMaxFetchFailures {
			res.Stopped = fmt.Sprintf("连续 %d 个种子文件下载失败，本轮停止加种", fetchFailures)
			return nil
		}
		if stop := brushLimitReached(task, downloading, totalSize, todayAdded, it.SizeBytes); stop != "" {
			res.Stopped = stop
			return nil
		}
		data, err := m.fetchTorrent(ctx, site, it)
		if err != nil {
			fetchFailures++
			res.Errors = append(res.Errors, fmt.Sprintf("下载 %s 的种子文件失败: %v", it.Title, err))
			continue
		}
		hash, err := qbit.ComputeTorrentHash(data)
		if err != nil {
			fetchFailures++
			res.Errors = append(res.Errors, fmt.Sprintf("%s 的种子文件无法解析: %v", it.Title, err))
			continue
		}
		fetchFailures = 0
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
			// 记不下来就撤回：没有记录的刷流种子既不归刷流管，也被常规清理跳过，会一直留在下载器里
			taken := errors.Is(err, models.ErrBrushTorrentTaken)
			if taken {
				res.Skipped = append(res.Skipped, it.Title+"：已由别的刷流任务加入，撤回了这一份")
			} else {
				res.Errors = append(res.Errors, fmt.Sprintf("%s 已加入下载器但记录失败，已撤回: %v", it.Title, err))
			}
			if rmErr := dl.RemoveTorrent(hash, true); rmErr != nil {
				m.cfg.Logger.Errorf("[刷流] %s 撤回 %s（%s）失败，需要手动删除: %v", task.Name, it.Title, hash, rmErr)
				res.Errors = append(res.Errors, fmt.Sprintf("撤回 %s 失败，需要在下载器里手动删除: %v", it.Title, rmErr))
			}
			if taken {
				continue
			}
			res.Stopped = "记录种子失败，本轮停止加种"
			return nil
		}
		res.Added++
		downloading++
		totalSize += size
		todayAdded += size
		m.cfg.Logger.Infof("[刷流] %s 加入 %s（%s，%.2f GB）", task.Name, it.Title, it.DiscountLevel, float64(size)/gib)
	}
	return nil
}

// lockSite 拿到站点的加种锁，返回释放函数。
func (m *BrushMonitor) lockSite(site string) func() {
	m.mu.Lock()
	if m.siteLocks == nil {
		m.siteLocks = map[string]*sync.Mutex{}
	}
	l, ok := m.siteLocks[site]
	if !ok {
		l = &sync.Mutex{}
		m.siteLocks[site] = l
	}
	m.mu.Unlock()
	l.Lock()
	return l.Unlock
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
