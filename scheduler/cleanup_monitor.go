package scheduler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

const (
	cleanupDefaultInterval = 30 * time.Minute
	cleanupMinInterval     = 5 * time.Minute
	emergencyBufferMinGB   = 10.0
	emergencyBufferPercent = 0.2
	diskEventDebounce      = 3 * time.Second
	// diskBudgetResetInterval 控制 CleanupEnabled=false 但 CleanupDiskProtect=true 时
	// 的 DiskBudget Reset 节奏。Issue #374 修复：原本这条路径下 Reset 永不触发。
	diskBudgetResetInterval = 5 * time.Minute
)

type CleanupMonitor struct {
	mu            sync.Mutex
	ctx           context.Context
	cancel        context.CancelFunc
	db            *gorm.DB
	downloaderMgr *downloader.DownloaderManager
	logger        *zap.SugaredLogger
	running       bool
	// wg 跟踪 runLoop：Stop 返回前等它退出，被替换的旧实例不会再执行一轮删种或暂停。
	wg sync.WaitGroup
	// startDelay 是启动后第一次检查前的等待，测试里调小。
	startDelay time.Duration
	// hist 记最近一轮的结果与下一轮的时间（Manager 给的，换监控时不丢）；为空时不记
	hist *CleanupHistory
	// quiet 是预览用的实例：只算不删，不写「紧急清理、额外删除」这类像是真的删了的日志
	quiet bool
}

// CleanupItem 是一个要删的种子与为什么删。
type CleanupItem struct {
	Name      string  `json:"name"`
	InfoHash  string  `json:"info_hash"`
	Size      int64   `json:"size"`
	SeedingH  float64 `json:"seeding_h"`
	Ratio     float64 `json:"ratio"`
	UploadKBs int64   `json:"upload_kbs"`
	// Reasons 是对上了的删种条件（紧急清理多挑的写磁盘空间不足）
	Reasons []string `json:"reasons"`
	// KeepData 是数据还被别的种子用着：只删种子、保留数据
	KeepData bool `json:"keep_data,omitempty"`
}

// CleanupResult 是一台下载器一轮自动删种（或预览）的结果：管理范围里多少个、受保护的、剩下的候选、要删的与为什么。
type CleanupResult struct {
	Downloader string `json:"downloader"`
	Managed    int    `json:"managed"`
	Protected  int    `json:"protected"`
	Candidates int    `json:"candidates"`
	// Emergency 是剩余空间低于下限、按紧急清理多挑了种子；FreeGB 是当时的剩余空间
	Emergency bool          `json:"emergency,omitempty"`
	FreeGB    float64       `json:"free_gb,omitempty"`
	Items     []CleanupItem `json:"items"`
	// Notes 是要让用户知道的情况（例如紧急清理腾不出空间）
	Notes []string `json:"notes,omitempty"`
	// Deleted 是真的删掉的个数（预览时为 0）；Error 是这台下载器出的错
	Deleted int    `json:"deleted"`
	Error   string `json:"error,omitempty"`
}

// CleanupRun 是一轮自动删种。
type CleanupRun struct {
	At      time.Time       `json:"at"`
	Results []CleanupResult `json:"results"`
}

// CleanupHistory 记最近一轮自动删种的结果与下一轮大约什么时候（在内存里，重启以后重新算）。
type CleanupHistory struct {
	mu   sync.Mutex
	last *CleanupRun
	next time.Time
}

func (h *CleanupHistory) record(run CleanupRun) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.last = &run
	h.mu.Unlock()
}

func (h *CleanupHistory) setNext(t time.Time) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.next = t
	h.mu.Unlock()
}

// Snapshot 是最近一轮的结果（没有时为空）与下一轮的时间（零值是不知道，例如没启用）。
func (h *CleanupHistory) Snapshot() (*CleanupRun, time.Time) {
	if h == nil {
		return nil, time.Time{}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.last == nil {
		return nil, h.next
	}
	run := *h.last
	return &run, h.next
}

func NewCleanupMonitor(db *gorm.DB, downloaderMgr *downloader.DownloaderManager) *CleanupMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	logger := global.GetSlogger()
	return &CleanupMonitor{
		ctx:           ctx,
		cancel:        cancel,
		db:            db,
		downloaderMgr: downloaderMgr,
		logger:        logger,
		startDelay:    10 * time.Second,
	}
}

func (c *CleanupMonitor) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil
	}
	c.running = true

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.runLoop()
	}()
	c.logger.Info("[自动删种] 监控服务已启动")
	return nil
}

func (c *CleanupMonitor) Stop() {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	c.cancel()
	c.running = false
	c.mu.Unlock()
	// 锁外等待：正在执行的这一轮跑完、循环退出后才返回
	c.wg.Wait()
	c.logger.Info("[自动删种] 监控服务已停止")
}

func (c *CleanupMonitor) runLoop() {
	// 启动后稍等再开始；可被 Stop 打断，不会在停止之后醒来再跑一轮
	select {
	case <-c.ctx.Done():
		return
	case <-time.After(c.startDelay):
	}

	_, eventCh, cancelSub := events.Subscribe(8)
	defer cancelSub()

	for {
		cfg := c.loadConfig()
		c.maybeResetDiskBudget(cfg)

		if cfg == nil || !cfg.CleanupEnabled {
			c.hist.setNext(time.Time{})
			c.logger.Debug("[自动删种] 功能未启用，等待下次检查")
			select {
			case <-c.ctx.Done():
				return
			case <-time.After(diskBudgetResetInterval):
				continue
			case ev := <-eventCh:
				if ev.Type == events.DiskSpaceLow {
					c.logger.Info("[自动删种] 收到磁盘空间不足信号，但功能未启用，忽略")
				}
				continue
			}
		}

		c.logger.Infof("[自动删种] 开始检查 (间隔=%d分钟, 范围=%s, 磁盘保护=%v)",
			cfg.CleanupIntervalMin, cfg.CleanupScope, cfg.CleanupDiskProtect)
		c.runOnce(cfg)

		interval := time.Duration(cfg.CleanupIntervalMin) * time.Minute
		if interval < cleanupMinInterval {
			interval = cleanupDefaultInterval
		}
		c.hist.setNext(time.Now().Add(interval))

		select {
		case <-c.ctx.Done():
			return
		case <-time.After(interval):
		case ev := <-eventCh:
			if ev.Type == events.DiskSpaceLow {
				c.logger.Info("[自动删种] 收到磁盘空间不足信号，等待短暂去抖后立即执行清理")
				c.drainAndDebounce(eventCh)
			}
		}
	}
}

// maybeResetDiskBudget 在每次 runLoop 迭代顶部决定是否归还 DiskBudget。
// Issue #374 修复点：原本 Reset 仅在 runOnce 内被调用，而 runOnce 又被
// CleanupEnabled gate 拦下，导致 CleanupDiskProtect=true + CleanupEnabled=false
// 配置组合下预留永不归零。这里改为只要 CleanupDiskProtect=true 就 Reset，
// 与 CleanupEnabled 解耦。
//
// 抽成独立方法以便单测可在不进入完整 runLoop（含 10 秒 Sleep）的前提下
// 验证 fix 的契约。
func (c *CleanupMonitor) maybeResetDiskBudget(cfg *models.SettingsGlobal) {
	if cfg == nil || !cfg.CleanupDiskProtect {
		return
	}
	c.resetDiskBudget()
}

func (c *CleanupMonitor) drainAndDebounce(ch <-chan events.Event) {
	timer := time.NewTimer(diskEventDebounce)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			return
		case ev := <-ch:
			if ev.Type == events.DiskSpaceLow {
				timer.Reset(diskEventDebounce)
			}
		case <-c.ctx.Done():
			return
		}
	}
}

func (c *CleanupMonitor) loadConfig() *models.SettingsGlobal {
	var cfg models.SettingsGlobal
	if err := c.db.First(&cfg).Error; err != nil {
		return nil
	}
	return &cfg
}

func (c *CleanupMonitor) runOnce(cfg *models.SettingsGlobal) {
	c.resetDiskBudget()

	run := CleanupRun{At: time.Now()}
	for _, name := range c.downloaderMgr.ListDownloaders() {
		dl, err := c.downloaderMgr.GetDownloader(name)
		if err != nil || !dl.IsHealthy() {
			run.Results = append(run.Results, CleanupResult{Downloader: name, Error: "下载器连不上，这一轮没检查"})
			continue
		}
		run.Results = append(run.Results, c.processDownloader(cfg, dl, name))
	}
	c.hist.record(run)
}

// Preview 算出按 cfg 现在会删哪些种子，不删、不动磁盘预留：每台连得上的下载器一个结果。取下载器以 ctx 为限
// （连不上的下载器不等它重连）。
func (c *CleanupMonitor) Preview(ctx context.Context, cfg *models.SettingsGlobal) []CleanupResult {
	var out []CleanupResult
	for _, name := range c.downloaderMgr.ListDownloaders() {
		dl, err := c.downloaderMgr.GetDownloaderContext(ctx, name)
		if err != nil || !dl.IsHealthy() {
			out = append(out, CleanupResult{Downloader: name, Error: "下载器连不上"})
			continue
		}
		res, _ := c.decide(cfg, dl, name)
		out = append(out, res)
	}
	return out
}

// resetDiskBudget 在 PushMutex 内清零 DiskBudget。
// PushMutex 内 Reset 是必须的：否则可能在某个 worker 的「Reserve → push」中间
// 清零，让下一个 worker 看到 pre_reserved=0 而重复借用同一份磁盘空间
// （Issue #299 race 的另一形态）。
func (c *CleanupMonitor) resetDiskBudget() {
	mu := ptinternal.PushMutex()
	mu.Lock()
	ptinternal.GetDiskBudget().Reset()
	mu.Unlock()
}

// cleanupPlan 是要删的种子：连数据一起删的与只删种子、保留数据的（数据还被别的种子用着）。
type cleanupPlan struct {
	withData, keepData []downloader.Torrent
}

// decide 算出这台下载器这一轮要删哪些种子与为什么，不删（预览与真的删共用）。
func (c *CleanupMonitor) decide(cfg *models.SettingsGlobal, dl downloader.Downloader, dlName string) (CleanupResult, cleanupPlan) {
	res := CleanupResult{Downloader: dlName, Items: []CleanupItem{}}
	allTorrents, err := dl.GetAllTorrents()
	if err != nil {
		res.Error = "读取种子列表失败: " + err.Error()
		return res, cleanupPlan{}
	}

	managed := c.filterManagedTorrents(cfg, allTorrents, dlName)
	res.Managed = len(managed)
	if len(managed) == 0 {
		return res, cleanupPlan{}
	}

	protected, candidates := c.splitProtected(cfg, managed)
	res.Protected, res.Candidates = len(protected), len(candidates)

	reasons := map[string][]string{}
	var toDelete []downloader.Torrent
	for _, t := range candidates {
		// 刷流种子由刷流任务自己的删种规则管，常规规则不碰，免得两套规则争抢；紧急清理仍然覆盖它们
		if hasTag(t, models.BrushTagAll) {
			continue
		}
		if c.isPausedForPeerRatio(t.InfoHash) {
			continue
		}
		if why := c.deleteReasons(cfg, t); len(why) > 0 {
			toDelete = append(toDelete, t)
			reasons[t.ID] = why
		}
	}

	sharing := downloader.NewDataSharing(allTorrents, downloader.FilesOf(dl))
	if cfg.CleanupDiskProtect && cfg.CleanupMinDiskSpaceGB > 0 {
		if diskInfo, derr := dl.GetDiskInfo(); derr == nil {
			freeGB := float64(diskInfo.FreeSpace) / (1024 * 1024 * 1024)
			if freeGB < cfg.CleanupMinDiskSpaceGB {
				res.Emergency, res.FreeGB = true, freeGB
				if !cfg.CleanupRemoveData {
					res.Notes = append(res.Notes, "剩余空间低于下限，但没打开「删除时连数据文件一起删」：紧急清理腾不出空间，没有多删")
				}
				before := len(toDelete)
				toDelete = c.emergencyCleanup(cfg, sharing, c.withoutLinked(candidates, dlName), toDelete, freeGB)
				why := fmt.Sprintf("剩余空间不足（剩 %.1f GB，下限 %.1f GB），紧急清理", freeGB, cfg.CleanupMinDiskSpaceGB)
				for _, t := range toDelete[before:] {
					reasons[t.ID] = []string{why}
				}
			}
		}
	}
	if len(toDelete) == 0 {
		return res, cleanupPlan{}
	}

	// 数据还被别的种子用着（如辅种与原种子共用同一份文件）的只删种子、保留数据，否则另一个种子就坏了
	plan := cleanupPlan{withData: toDelete}
	if cfg.CleanupRemoveData {
		plan.withData, plan.keepData = sharing.KeepSharedData(toDelete)
	}
	keep := make(map[string]bool, len(plan.keepData))
	for _, t := range plan.keepData {
		keep[t.ID] = true
	}
	for _, t := range toDelete {
		res.Items = append(res.Items, CleanupItem{
			Name: t.Name, InfoHash: strings.ToLower(t.InfoHash), Size: t.TotalSize, SeedingH: float64(t.SeedingTime) / 3600,
			Ratio: t.Ratio, UploadKBs: t.UploadSpeed / 1024, Reasons: reasons[t.ID], KeepData: keep[t.ID],
		})
	}
	return res, plan
}

func (c *CleanupMonitor) processDownloader(cfg *models.SettingsGlobal, dl downloader.Downloader, dlName string) CleanupResult {
	res, plan := c.decide(cfg, dl, dlName)
	switch {
	case res.Error != "":
		c.logger.Errorf("[自动删种] %s: %s", dlName, res.Error)
		return res
	case res.Managed == 0:
		c.logger.Infof("[自动删种] %s: 管理范围内无种子", dlName)
		return res
	}
	if res.Emergency {
		c.logger.Warnf("[自动删种] %s: 磁盘空间不足 (%.1f GB < %.1f GB)，启动紧急清理", dlName, res.FreeGB, cfg.CleanupMinDiskSpaceGB)
	}
	if len(res.Items) == 0 {
		c.logger.Infof("[自动删种] %s: 检查完成 (管理=%d, 保护=%d, 候选=%d, 无需删除)",
			dlName, res.Managed, res.Protected, res.Candidates)
		return res
	}

	c.logger.Infof("[自动删种] %s: 准备删除 %d 个种子", dlName, len(res.Items))
	for _, it := range res.Items {
		c.logger.Infof("[自动删种] 删除: %s (做种%.1fh, 分享率%.2f, 上传速度%d KB/s, %s)",
			it.Name, it.SeedingH, it.Ratio, it.UploadKBs, strings.Join(it.Reasons, "；"))
	}

	var errs []string
	deleted := make([]downloader.Torrent, 0, len(plan.withData)+len(plan.keepData))
	if len(plan.withData) > 0 {
		if err := dl.RemoveTorrents(torrentIDs(plan.withData), cfg.CleanupRemoveData); err != nil {
			c.logger.Errorf("[自动删种] %s: 批量删除失败: %v", dlName, err)
			errs = append(errs, "删除失败: "+err.Error())
		} else {
			deleted = append(deleted, plan.withData...)
		}
	}
	if len(plan.keepData) > 0 {
		for _, t := range plan.keepData {
			c.logger.Infof("[自动删种] %s: %s 的数据还被别的种子用着，只删种子、保留数据", dlName, t.Name)
		}
		if err := dl.RemoveTorrents(torrentIDs(plan.keepData), false); err != nil {
			c.logger.Errorf("[自动删种] %s: 批量删除（保留数据）失败: %v", dlName, err)
			errs = append(errs, "删除（保留数据）失败: "+err.Error())
		} else {
			deleted = append(deleted, plan.keepData...)
		}
	}
	res.Deleted, res.Error = len(deleted), strings.Join(errs, "；")
	if len(deleted) == 0 {
		return res
	}
	c.updateDatabase(deleted, dlName)
	c.logger.Infof("[自动删种] %s: 成功删除 %d 个种子", dlName, len(deleted))
	return res
}

func (c *CleanupMonitor) filterManagedTorrents(cfg *models.SettingsGlobal, torrents []downloader.Torrent, dlName string) []downloader.Torrent {
	switch cfg.CleanupScope {
	case "tag":
		tags := splitTags(cfg.CleanupScopeTags)
		if len(tags) == 0 {
			return nil
		}
		var result []downloader.Torrent
		for _, t := range torrents {
			tTags := splitTags(t.Tags + "," + t.Category + "," + t.Label)
			for _, tag := range tags {
				if containsIgnoreCase(tTags, tag) {
					result = append(result, t)
					break
				}
			}
		}
		return result

	case "all":
		return torrents

	default:
		managedHashes := c.getManagedHashes(dlName)
		if len(managedHashes) == 0 {
			return nil
		}
		var result []downloader.Torrent
		for _, t := range torrents {
			if _, ok := managedHashes[strings.ToLower(t.InfoHash)]; ok {
				result = append(result, t)
			}
		}
		return result
	}
}

func (c *CleanupMonitor) getManagedHashes(dlName string) map[string]struct{} {
	hashes := make(map[string]struct{})

	var dbHashes []string
	c.db.Model(&models.TorrentInfo{}).
		Where("torrent_hash IS NOT NULL AND torrent_hash != '' AND is_pushed IS NOT NULL AND downloader_name = ?", dlName).
		Pluck("torrent_hash", &dbHashes)

	var archiveHashes []string
	c.db.Model(&models.TorrentInfoArchive{}).
		Where("torrent_hash IS NOT NULL AND torrent_hash != '' AND is_pushed IS NOT NULL AND downloader_name = ?", dlName).
		Pluck("torrent_hash", &archiveHashes)

	for _, h := range append(dbHashes, archiveHashes...) {
		hashes[strings.ToLower(h)] = struct{}{}
	}
	return hashes
}

func (c *CleanupMonitor) splitProtected(cfg *models.SettingsGlobal, torrents []downloader.Torrent) (protected, candidates []downloader.Torrent) {
	protectTags := splitTags(cfg.CleanupProtectTags)
	now := time.Now().Unix()

	hrInfoMap := c.getHRInfoMap()

	for _, t := range torrents {
		if cfg.CleanupProtectDL && (t.State == downloader.TorrentDownloading || t.State == downloader.TorrentChecking) {
			protected = append(protected, t)
			continue
		}

		if cfg.CleanupMinRetainH > 0 && t.DateAdded > 0 {
			retainUntil := t.DateAdded + int64(cfg.CleanupMinRetainH*3600)
			if now < retainUntil {
				protected = append(protected, t)
				continue
			}
		}

		if cfg.CleanupProtectHR {
			if hrInfo, ok := hrInfoMap[strings.ToLower(t.InfoHash)]; ok && hrInfo.HasHR {
				requiredSeedTimeS := int64(hrInfo.HRSeedTimeH) * 3600
				if requiredSeedTimeS > 0 && t.SeedingTime < requiredSeedTimeS {
					c.logger.Debugf("[自动删种] H&R 保护: %s (需做种%dh, 已做种%.1fh)",
						t.Name, hrInfo.HRSeedTimeH, float64(t.SeedingTime)/3600)
					protected = append(protected, t)
					continue
				}
			}
		}

		if len(protectTags) > 0 {
			tTags := splitTags(t.Tags + "," + t.Category + "," + t.Label)
			isProtected := false
			for _, pt := range protectTags {
				if containsIgnoreCase(tTags, pt) {
					isProtected = true
					break
				}
			}
			if isProtected {
				protected = append(protected, t)
				continue
			}
		}

		candidates = append(candidates, t)
	}
	return protected, candidates
}

type hrInfo struct {
	HasHR       bool
	HRSeedTimeH int
}

func (c *CleanupMonitor) getHRInfoMap() map[string]hrInfo {
	result := make(map[string]hrInfo)

	siteDefMap := make(map[string]*v2.SiteDefinition)
	for _, def := range v2.GetDefinitionRegistry().GetAll() {
		if def.HREnabled {
			siteDefMap[def.ID] = def
		}
	}

	var records []struct {
		TorrentHash string
		SiteName    string
		HasHR       bool
		HRSeedTimeH int
		TorrentSize int64
	}
	c.db.Model(&models.TorrentInfo{}).
		Select("torrent_hash, site_name, has_hr, hr_seed_time_h, torrent_size").
		Where("torrent_hash IS NOT NULL AND torrent_hash != ''").
		Find(&records)

	for _, r := range records {
		hash := strings.ToLower(r.TorrentHash)
		if r.HasHR {
			result[hash] = hrInfo{HasHR: true, HRSeedTimeH: r.HRSeedTimeH}
		} else if def, ok := siteDefMap[r.SiteName]; ok {
			// Use per-torrent size-based calculation when available
			result[hash] = hrInfo{HasHR: true, HRSeedTimeH: def.CalcHRSeedTimeH(r.TorrentSize)}
		}
	}
	return result
}

func (c *CleanupMonitor) shouldDelete(cfg *models.SettingsGlobal, t downloader.Torrent) bool {
	return len(c.deleteReasons(cfg, t)) > 0
}

// deleteReasons 是这个种子对上的删种条件；为空时不删。条件之间按 cfg.CleanupConditionMode：or（默认）对上一个就删，
// and 设了的都对上才删。打开了「免费到期没下完就删」时，免费期过了还没下完的直接删。
func (c *CleanupMonitor) deleteReasons(cfg *models.SettingsGlobal, t downloader.Torrent) []string {
	if cfg.CleanupDelFreeExpired && c.isFreeExpiredIncomplete(t) {
		return []string{"免费期过了还没下完"}
	}
	conds := []struct {
		on, hit bool
		text    string
	}{
		{
			cfg.CleanupMaxSeedTimeH > 0, t.SeedingTime >= int64(cfg.CleanupMaxSeedTimeH)*3600,
			fmt.Sprintf("做种超过 %d 小时", cfg.CleanupMaxSeedTimeH),
		},
		{
			cfg.CleanupMinRatio > 0, t.Ratio >= cfg.CleanupMinRatio,
			fmt.Sprintf("分享率达到 %g", cfg.CleanupMinRatio),
		},
		{
			cfg.CleanupMaxInactiveH > 0,
			t.UploadSpeed == 0 && t.State == downloader.TorrentSeeding && t.SeedingTime > int64(cfg.CleanupMaxInactiveH)*3600,
			fmt.Sprintf("做种超过 %d 小时、现在没有上传", cfg.CleanupMaxInactiveH),
		},
		{
			cfg.CleanupSlowSeedTimeH > 0 && cfg.CleanupSlowMaxRatio > 0,
			t.SeedingTime >= int64(cfg.CleanupSlowSeedTimeH)*3600 && t.Ratio < cfg.CleanupSlowMaxRatio && t.IsCompleted,
			fmt.Sprintf("做种 %d 小时分享率还不到 %g", cfg.CleanupSlowSeedTimeH, cfg.CleanupSlowMaxRatio),
		},
	}
	on := 0
	var hit []string
	for _, cd := range conds {
		if !cd.on {
			continue
		}
		on++
		if cd.hit {
			hit = append(hit, cd.text)
		}
	}
	if on == 0 || (cfg.CleanupConditionMode == "and" && len(hit) < on) {
		return nil
	}
	return hit
}

func (c *CleanupMonitor) isPausedForPeerRatio(infoHash string) bool {
	var count int64
	c.db.Model(&models.TorrentInfo{}).
		Where("torrent_hash = ? AND is_paused_by_system = ? AND pause_reason = ?",
			strings.ToLower(infoHash), true, PauseReasonPeerRatio).
		Count(&count)
	return count > 0
}

// isFreeExpiredIncomplete 报告种子没下载完、免费期已经过了。只认免费的记录（is_free）：以前 RSS 把半价等优惠的
// 结束时间也记成了 free_end_time，库里留着的这类记录不当成免费到期
func (c *CleanupMonitor) isFreeExpiredIncomplete(t downloader.Torrent) bool {
	if t.Progress >= 1.0 {
		return false
	}

	hash := strings.ToLower(t.InfoHash)
	var info models.TorrentInfo
	err := c.db.Where("LOWER(torrent_hash) = ? AND is_free = ? AND free_end_time IS NOT NULL AND free_end_time < ?",
		hash, true, time.Now()).First(&info).Error
	return err == nil
}

// 数据还被别的种子用着时，删了也腾不出空间（只删种子、保留数据）：共用这份数据的种子都在候选里时整组一起挑、空间只算一次；
// 其中有不能删的（受保护、不在管理范围）时整组不挑。sharing 由下载器里的全部种子建。
func (c *CleanupMonitor) emergencyCleanup(cfg *models.SettingsGlobal, sharing *downloader.DataSharing, candidates, alreadyMarked []downloader.Torrent, currentFreeGB float64) []downloader.Torrent {
	if !cfg.CleanupRemoveData {
		// 只从下载器移除任务、不删数据文件，磁盘空间一点也不会释放；
		// 继续按体积挑种子只会一轮轮删掉任务和做种状态，空间照样不够。
		if !c.quiet {
			c.logger.Warnf("[自动删种] 磁盘空间不足，但「删除时连数据文件一起删」未开启，紧急清理释放不了空间，跳过额外删除")
		}
		return alreadyMarked
	}

	markedSet := make(map[string]struct{})
	deleting := make(map[string]bool, len(alreadyMarked))
	for _, t := range alreadyMarked {
		markedSet[t.ID] = struct{}{}
		deleting[downloader.TorrentKey(t)] = true
	}
	// freed 是删掉 t 能腾出的空间：数据还被不删的种子用着的不算；共用同一份数据的种子只算一个
	//（同一目录下文件不同的平铺种子不是同一份数据，各算各的）
	counted := map[string]bool{}
	freed := func(t downloader.Torrent) float64 {
		if sharing.Shares(t, deleting) {
			return 0
		}
		for _, o := range sharing.Sharers(t) {
			if counted[downloader.TorrentKey(o)] {
				return 0
			}
		}
		counted[downloader.TorrentKey(t)] = true
		return float64(t.TotalSize)
	}

	type scored struct {
		torrent downloader.Torrent
		score   float64
	}

	candidateKeys := make(map[string]bool, len(candidates))
	var extras []scored
	for _, t := range candidates {
		candidateKeys[downloader.TorrentKey(t)] = true
		if _, ok := markedSet[t.ID]; ok {
			continue
		}
		s := c.calcPriority(t)
		extras = append(extras, scored{torrent: t, score: s})
	}

	sort.Slice(extras, func(i, j int) bool {
		return extras[i].score > extras[j].score
	})

	result := make([]downloader.Torrent, len(alreadyMarked))
	copy(result, alreadyMarked)

	bufferGB := cfg.CleanupMinDiskSpaceGB * emergencyBufferPercent
	if bufferGB < emergencyBufferMinGB {
		bufferGB = emergencyBufferMinGB
	}
	targetGB := cfg.CleanupMinDiskSpaceGB + bufferGB
	neededBytes := (targetGB - currentFreeGB) * 1024 * 1024 * 1024
	// 按常规规则已经要删的种子也会释放空间，先算进去，免得再多删
	var freedBytes float64
	for _, t := range alreadyMarked {
		freedBytes += freed(t)
	}

	for _, e := range extras {
		if freedBytes >= neededBytes {
			break
		}
		if deleting[downloader.TorrentKey(e.torrent)] {
			continue // 已经跟着共用数据的种子一起挑上了
		}
		group, ok := []downloader.Torrent{e.torrent}, true
		for _, o := range sharing.Sharers(e.torrent) {
			k := downloader.TorrentKey(o)
			if deleting[k] {
				continue
			}
			if !candidateKeys[k] {
				ok = false
				break
			}
			group = append(group, o)
		}
		if !ok {
			continue
		}
		for _, g := range group {
			deleting[downloader.TorrentKey(g)] = true
			result = append(result, g)
		}
		freedBytes += freed(e.torrent)
	}

	freedGB := freedBytes / (1024 * 1024 * 1024)
	if !c.quiet {
		c.logger.Infof("[自动删种] 紧急清理: 当前 %.1f GB, 目标 %.1f GB (阈值 %.1f + 缓冲 %.1f), 预计释放 %.1f GB, 额外删除 %d 个种子",
			currentFreeGB, targetGB, cfg.CleanupMinDiskSpaceGB, bufferGB, freedGB, len(result)-len(alreadyMarked))
	}

	return result
}

// withoutLinked 去掉已经硬链接整理进媒体库的种子：删掉它们连数据也腾不出空间（库里的链接还占着），
// 紧急清理不挑它们。打开了「删种时一并删除入库链接」时不去掉：删种后链接也会被清理，空间能腾出来。
func (c *CleanupMonitor) withoutLinked(candidates []downloader.Torrent, dlName string) []downloader.Torrent {
	if c.db == nil || len(candidates) == 0 || !c.db.Migrator().HasTable(&models.MediaTransferHistory{}) {
		return candidates
	}
	var set models.MediaOrganizeSetting
	if err := c.db.Where("id = ?", 1).Limit(1).Find(&set).Error; err == nil && set.DeleteLinksOnRemove {
		return candidates
	}
	var hashes []string
	if err := c.db.Model(&models.MediaTransferHistory{}).
		Where("status = ? AND mode = ? AND downloader_name = ?", models.MediaTransferDone, models.MediaModeHardlink, dlName).
		Distinct().Pluck("info_hash", &hashes).Error; err != nil || len(hashes) == 0 {
		return candidates
	}
	linked := make(map[string]bool, len(hashes))
	for _, h := range hashes {
		linked[strings.ToLower(h)] = true
	}
	out := make([]downloader.Torrent, 0, len(candidates))
	skipped := 0
	for _, t := range candidates {
		if linked[strings.ToLower(t.InfoHash)] {
			skipped++
			continue
		}
		out = append(out, t)
	}
	if skipped > 0 && !c.quiet {
		c.logger.Infof("[自动删种] %s: %d 个种子已经硬链接整理进媒体库，删掉也腾不出空间，紧急清理不挑它们", dlName, skipped)
	}
	return out
}

func (c *CleanupMonitor) calcPriority(t downloader.Torrent) float64 {
	var score float64

	if t.State == downloader.TorrentPaused {
		score += 50
	}

	seedTimeH := float64(t.SeedingTime) / 3600
	score += seedTimeH * 0.5

	score += t.Ratio * 10

	if t.UploadSpeed == 0 {
		score += 20
	}

	sizeGB := float64(t.TotalSize) / (1024 * 1024 * 1024)
	score += sizeGB * 2

	return score
}

func (c *CleanupMonitor) updateDatabase(deleted []downloader.Torrent, dlName string) {
	for _, t := range deleted {
		hash := strings.ToLower(t.InfoHash)
		now := time.Now()
		c.db.Model(&models.TorrentInfo{}).
			Where("LOWER(torrent_hash) = ? AND downloader_name = ?", hash, dlName).
			Updates(map[string]any{
				"is_expired":      true,
				"last_check_time": &now,
			})
	}
}

func splitTags(s string) []string {
	var tags []string
	for _, t := range strings.Split(s, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

func containsIgnoreCase(slice []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, s := range slice {
		if strings.ToLower(strings.TrimSpace(s)) == target {
			return true
		}
	}
	return false
}

func (c *CleanupMonitor) RunManual() (int, error) {
	cfg := c.loadConfig()
	if cfg == nil {
		return 0, fmt.Errorf("无法加载配置")
	}
	if !cfg.CleanupEnabled {
		return 0, fmt.Errorf("自动删种未启用")
	}
	c.runOnce(cfg)
	return 0, nil
}

func torrentIDs(ts []downloader.Torrent) []string {
	ids := make([]string, 0, len(ts))
	for _, t := range ts {
		ids = append(ids, t.ID)
	}
	return ids
}
