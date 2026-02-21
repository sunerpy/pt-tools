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
)

type CleanupMonitor struct {
	mu            sync.Mutex
	ctx           context.Context
	cancel        context.CancelFunc
	db            *gorm.DB
	downloaderMgr *downloader.DownloaderManager
	logger        *zap.SugaredLogger
	running       bool
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
	}
}

func (c *CleanupMonitor) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil
	}
	c.running = true

	go c.runLoop()
	c.logger.Info("[自动删种] 监控服务已启动")
	return nil
}

func (c *CleanupMonitor) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return
	}
	c.cancel()
	c.running = false
	c.logger.Info("[自动删种] 监控服务已停止")
}

func (c *CleanupMonitor) runLoop() {
	time.Sleep(10 * time.Second)

	_, eventCh, cancelSub := events.Subscribe(8)
	defer cancelSub()

	for {
		cfg := c.loadConfig()
		if cfg == nil || !cfg.CleanupEnabled {
			c.logger.Debug("[自动删种] 功能未启用，等待下次检查")
			select {
			case <-c.ctx.Done():
				return
			case <-time.After(5 * time.Minute):
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
	dlNames := c.downloaderMgr.ListDownloaders()
	if len(dlNames) == 0 {
		return
	}

	for _, name := range dlNames {
		dl, err := c.downloaderMgr.GetDownloader(name)
		if err != nil || !dl.IsHealthy() {
			continue
		}
		c.processDownloader(cfg, dl, name)
	}
}

func (c *CleanupMonitor) processDownloader(cfg *models.SettingsGlobal, dl downloader.Downloader, dlName string) {
	allTorrents, err := dl.GetAllTorrents()
	if err != nil {
		c.logger.Errorf("[自动删种] %s: 获取种子列表失败: %v", dlName, err)
		return
	}

	managed := c.filterManagedTorrents(cfg, allTorrents, dlName)
	if len(managed) == 0 {
		c.logger.Infof("[自动删种] %s: 管理范围内无种子", dlName)
		return
	}

	protected, candidates := c.splitProtected(cfg, managed)
	_ = protected

	var toDelete []downloader.Torrent
	for _, t := range candidates {
		if c.shouldDelete(cfg, t) {
			toDelete = append(toDelete, t)
		}
	}

	if cfg.CleanupDiskProtect && cfg.CleanupMinDiskSpaceGB > 0 {
		diskInfo, err := dl.GetDiskInfo()
		if err == nil {
			freeGB := float64(diskInfo.FreeSpace) / (1024 * 1024 * 1024)
			if freeGB < cfg.CleanupMinDiskSpaceGB {
				c.logger.Warnf("[自动删种] %s: 磁盘空间不足 (%.1f GB < %.1f GB)，启动紧急清理",
					dlName, freeGB, cfg.CleanupMinDiskSpaceGB)
				toDelete = c.emergencyCleanup(cfg, candidates, toDelete, freeGB)
			}
		}
	}

	if len(toDelete) == 0 {
		c.logger.Infof("[自动删种] %s: 检查完成 (管理=%d, 保护=%d, 候选=%d, 无需删除)",
			dlName, len(managed), len(protected), len(candidates))
		return
	}

	ids := make([]string, 0, len(toDelete))
	for _, t := range toDelete {
		ids = append(ids, t.ID)
	}

	c.logger.Infof("[自动删种] %s: 准备删除 %d 个种子", dlName, len(ids))
	for _, t := range toDelete {
		seedTimeH := float64(t.SeedingTime) / 3600
		c.logger.Infof("[自动删种] 删除: %s (做种%.1fh, 分享率%.2f, 上传速度%d KB/s)",
			t.Name, seedTimeH, t.Ratio, t.UploadSpeed/1024)
	}

	if err := dl.RemoveTorrents(ids, cfg.CleanupRemoveData); err != nil {
		c.logger.Errorf("[自动删种] %s: 批量删除失败: %v", dlName, err)
		return
	}

	c.updateDatabase(toDelete, dlName)
	c.logger.Infof("[自动删种] %s: 成功删除 %d 个种子", dlName, len(toDelete))
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

	siteHRMap := make(map[string]hrInfo)
	for _, def := range v2.GetDefinitionRegistry().GetAll() {
		if def.HREnabled {
			siteHRMap[def.ID] = hrInfo{HasHR: true, HRSeedTimeH: def.HRSeedTimeHours}
		}
	}

	var records []struct {
		TorrentHash string
		SiteName    string
		HasHR       bool
		HRSeedTimeH int
	}
	c.db.Model(&models.TorrentInfo{}).
		Select("torrent_hash, site_name, has_hr, hr_seed_time_h").
		Where("torrent_hash IS NOT NULL AND torrent_hash != ''").
		Find(&records)

	for _, r := range records {
		hash := strings.ToLower(r.TorrentHash)
		if r.HasHR {
			result[hash] = hrInfo{HasHR: true, HRSeedTimeH: r.HRSeedTimeH}
		} else if siteInfo, ok := siteHRMap[r.SiteName]; ok {
			result[hash] = siteInfo
		}
	}
	return result
}

func (c *CleanupMonitor) shouldDelete(cfg *models.SettingsGlobal, t downloader.Torrent) bool {
	if cfg.CleanupDelFreeExpired && c.isFreeExpiredIncomplete(t) {
		return true
	}

	mode := cfg.CleanupConditionMode
	if mode == "" {
		mode = "or"
	}

	seedTimeMatch := cfg.CleanupMaxSeedTimeH > 0 && t.SeedingTime >= int64(cfg.CleanupMaxSeedTimeH)*3600
	ratioMatch := cfg.CleanupMinRatio > 0 && t.Ratio >= cfg.CleanupMinRatio
	inactiveMatch := cfg.CleanupMaxInactiveH > 0 && t.UploadSpeed == 0 &&
		t.State == downloader.TorrentSeeding && t.SeedingTime > int64(cfg.CleanupMaxInactiveH)*3600
	slowSeedMatch := cfg.CleanupSlowSeedTimeH > 0 && cfg.CleanupSlowMaxRatio > 0 &&
		t.SeedingTime >= int64(cfg.CleanupSlowSeedTimeH)*3600 && t.Ratio < cfg.CleanupSlowMaxRatio &&
		t.IsCompleted

	hasAnyCondition := cfg.CleanupMaxSeedTimeH > 0 || cfg.CleanupMinRatio > 0 ||
		cfg.CleanupMaxInactiveH > 0 || (cfg.CleanupSlowSeedTimeH > 0 && cfg.CleanupSlowMaxRatio > 0)
	if !hasAnyCondition {
		return false
	}

	if mode == "and" {
		conditions := 0
		matched := 0
		if cfg.CleanupMaxSeedTimeH > 0 {
			conditions++
			if seedTimeMatch {
				matched++
			}
		}
		if cfg.CleanupMinRatio > 0 {
			conditions++
			if ratioMatch {
				matched++
			}
		}
		if cfg.CleanupMaxInactiveH > 0 {
			conditions++
			if inactiveMatch {
				matched++
			}
		}
		if cfg.CleanupSlowSeedTimeH > 0 && cfg.CleanupSlowMaxRatio > 0 {
			conditions++
			if slowSeedMatch {
				matched++
			}
		}
		return conditions > 0 && matched == conditions
	}

	return seedTimeMatch || ratioMatch || inactiveMatch || slowSeedMatch
}

func (c *CleanupMonitor) isFreeExpiredIncomplete(t downloader.Torrent) bool {
	if t.Progress >= 1.0 {
		return false
	}

	hash := strings.ToLower(t.InfoHash)
	var info models.TorrentInfo
	err := c.db.Where("LOWER(torrent_hash) = ? AND free_end_time IS NOT NULL AND free_end_time < ?",
		hash, time.Now()).First(&info).Error
	return err == nil
}

func (c *CleanupMonitor) emergencyCleanup(cfg *models.SettingsGlobal, candidates, alreadyMarked []downloader.Torrent, currentFreeGB float64) []downloader.Torrent {
	markedSet := make(map[string]struct{})
	for _, t := range alreadyMarked {
		markedSet[t.ID] = struct{}{}
	}

	type scored struct {
		torrent downloader.Torrent
		score   float64
	}

	var extras []scored
	for _, t := range candidates {
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
	var freedBytes float64

	for _, e := range extras {
		if freedBytes >= neededBytes {
			break
		}
		result = append(result, e.torrent)
		freedBytes += float64(e.torrent.TotalSize)
	}

	freedGB := freedBytes / (1024 * 1024 * 1024)
	c.logger.Infof("[自动删种] 紧急清理: 当前 %.1f GB, 目标 %.1f GB (阈值 %.1f + 缓冲 %.1f), 预计释放 %.1f GB, 额外删除 %d 个种子",
		currentFreeGB, targetGB, cfg.CleanupMinDiskSpaceGB, bufferGB, freedGB, len(result)-len(alreadyMarked))

	return result
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
