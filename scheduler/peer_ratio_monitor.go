package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

const (
	peerRatioDefaultInterval = 10 * time.Minute
	peerRatioMinInterval     = 5 * time.Minute
	PauseReasonPeerRatio     = "做种竞争度过高"
)

type PeerRatioMonitor struct {
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
}

func NewPeerRatioMonitor(db *gorm.DB, downloaderMgr *downloader.DownloaderManager) *PeerRatioMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	logger := global.GetSlogger()
	return &PeerRatioMonitor{
		ctx:           ctx,
		cancel:        cancel,
		db:            db,
		downloaderMgr: downloaderMgr,
		logger:        logger,
		startDelay:    15 * time.Second,
	}
}

func (p *PeerRatioMonitor) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return nil
	}
	p.running = true

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.runLoop()
	}()
	p.logger.Info("[竞争度监控] 服务已启动")
	return nil
}

func (p *PeerRatioMonitor) Stop() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	p.cancel()
	p.running = false
	p.mu.Unlock()
	// 锁外等待：正在执行的这一轮跑完、循环退出后才返回
	p.wg.Wait()
	p.logger.Info("[竞争度监控] 服务已停止")
}

func (p *PeerRatioMonitor) runLoop() {
	// 启动后稍等再开始；可被 Stop 打断，不会在停止之后醒来再跑一轮
	select {
	case <-p.ctx.Done():
		return
	case <-time.After(p.startDelay):
	}

	_, eventCh, cancelSub := events.Subscribe(8)
	defer cancelSub()

	for {
		cfg := p.loadConfig()
		if cfg == nil || !cfg.PeerRatioEnabled {
			p.logger.Debug("[竞争度监控] 功能未启用，等待下次检查")
			select {
			case <-p.ctx.Done():
				return
			case <-time.After(5 * time.Minute):
				continue
			case <-eventCh:
				continue
			}
		}

		maxSL := cfg.GetEffectivePeerRatioMaxSL()
		p.logger.Infof("[竞争度监控] 开始检查 (间隔=%d分钟, 阈值=%.1f)",
			cfg.GetEffectivePeerRatioIntervalMin(), maxSL)
		p.runOnce(cfg, maxSL)

		interval := time.Duration(cfg.GetEffectivePeerRatioIntervalMin()) * time.Minute
		if interval < peerRatioMinInterval {
			interval = peerRatioDefaultInterval
		}

		select {
		case <-p.ctx.Done():
			return
		case <-time.After(interval):
		case <-eventCh:
		}
	}
}

func (p *PeerRatioMonitor) loadConfig() *models.SettingsGlobal {
	var cfg models.SettingsGlobal
	if err := p.db.First(&cfg).Error; err != nil {
		return nil
	}
	return &cfg
}

func (p *PeerRatioMonitor) runOnce(cfg *models.SettingsGlobal, maxSL float64) {
	dlNames := p.downloaderMgr.ListDownloaders()
	if len(dlNames) == 0 {
		return
	}

	for _, name := range dlNames {
		dl, err := p.downloaderMgr.GetDownloader(name)
		if err != nil || !dl.IsHealthy() {
			continue
		}
		p.processDownloader(dl, name, maxSL, cfg.PeerRatioRemoveData)
	}
}

// peerOver 是做种竞争度超标、这一轮要处理的种子。
type peerOver struct {
	t              downloader.Torrent
	seeds, leeches int
	ratio          float64
}

func (p *PeerRatioMonitor) processDownloader(dl downloader.Downloader, dlName string, maxSL float64, removeData bool) {
	allTorrents, err := dl.GetAllTorrents()
	if err != nil {
		p.logger.Errorf("[竞争度监控] %s: 获取种子列表失败: %v", dlName, err)
		return
	}

	dlID := downloaderIDByName(p.db, dlName)
	managed := p.filterManagedSeedingTorrents(allTorrents, dlName)
	if len(managed) == 0 {
		p.logger.Debugf("[竞争度监控] %s: 无做种中的管理种子", dlName)
		return
	}

	// 和自动删种一样遵守保护规则（见 protection.go），刷流任务正在管的种子留给刷流；读不出这些信息时这一轮不处理
	cfg, err := p.protectionConfig()
	if err != nil {
		p.logger.Errorf("[竞争度监控] %s: %v，这一轮不处理", dlName, err)
		return
	}
	hr, err := loadHRInfo(p.db)
	if err != nil {
		p.logger.Errorf("[竞争度监控] %s: %v，这一轮不处理", dlName, err)
		return
	}
	brushOwned, err := brushOwnedHashes(p.db, dlID)
	if err != nil {
		p.logger.Errorf("[竞争度监控] %s: %v，这一轮不处理", dlName, err)
		return
	}
	protectTags := splitTags(cfg.CleanupProtectTags)
	now := time.Now().Unix()

	var over []peerOver
	for _, t := range managed {
		if p.ctx.Err() != nil {
			return
		}
		if brushOwned[strings.ToLower(t.InfoHash)] {
			continue
		}
		if why := hardProtection(cfg, t, hr, protectTags); why != "" {
			p.logger.Debugf("[竞争度监控] %s: %s 受保护（%s），跳过", dlName, t.Name, why)
			continue
		}
		if why := softProtection(cfg, t, now); why != "" {
			p.logger.Debugf("[竞争度监控] %s: %s 受保护（%s），跳过", dlName, t.Name, why)
			continue
		}

		seeds, leeches, err := p.getTrackerPeerCounts(dl, t)
		if err != nil {
			p.logger.Debugf("[竞争度监控] %s: 获取 tracker 信息失败: %s: %v", dlName, t.Name, err)
			continue
		}

		p.updateDBPeerCounts(dlID, dlName, t.InfoHash, seeds, leeches)

		if seeds == 0 {
			continue
		}

		var ratio float64
		if leeches == 0 {
			ratio = float64(seeds)
		} else {
			ratio = float64(seeds) / float64(leeches)
		}

		if ratio <= maxSL {
			continue
		}

		if p.isAlreadyPausedForPeerRatio(dlID, dlName, t.InfoHash) {
			continue
		}
		over = append(over, peerOver{t: t, seeds: seeds, leeches: leeches, ratio: ratio})
	}
	acted := 0
	if len(over) > 0 {
		acted = p.act(dl, dlID, dlName, over, maxSL, removeData, protectTags)
	}

	action := "暂停"
	if removeData {
		action = "删除"
	}
	p.logger.Infof("[竞争度监控] %s: 检查完成 (管理做种=%d, %s=%d)", dlName, len(managed), action, acted)
}

// act 在删除锁里删除或暂停竞争度超标的种子（见 internal.RemovalMutex）：重新读种子列表，不在了的、刚带上保留或保护标签的、
// 不再做种的不处理。返回处理了几个。
func (p *PeerRatioMonitor) act(dl downloader.Downloader, dlID *uint, dlName string, over []peerOver, maxSL float64, removeData bool, protectTags []string) int {
	mu := ptinternal.RemovalMutex()
	mu.Lock()
	defer mu.Unlock()
	fresh, err := dl.GetAllTorrents()
	if err != nil {
		p.logger.Errorf("[竞争度监控] %s: 处理前重新读种子列表失败: %v，这一轮不处理", dlName, err)
		return 0
	}
	byID := make(map[string]downloader.Torrent, len(fresh))
	for _, t := range fresh {
		byID[t.ID] = t
	}
	sharing := downloader.NewDataSharing(fresh, downloader.FilesOf(dl))
	removed := map[string]bool{} // 这一轮已删掉的（键见 downloader.TorrentKey）
	acted := 0
	for _, o := range over {
		t, ok := byID[o.t.ID]
		if !ok || t.State != downloader.TorrentSeeding || tagProtection(t, protectTags) != "" {
			p.logger.Infof("[竞争度监控] %s: %s 处理前不在下载器里了、不在做种或者带上了保留、保护标签，跳过", dlName, o.t.Name)
			continue
		}
		if removeData {
			// 数据还被别的种子用着（如辅种）时只删种子、保留数据
			withData := !sharing.Shares(t, removed)
			p.logger.Infof("[竞争度监控] 删除: %s (S/L=%d/%d=%.1f > %.1f, 删除数据=%v)", t.Name, o.seeds, o.leeches, o.ratio, maxSL, withData)
			if err := dl.RemoveTorrent(t.ID, withData); err != nil {
				p.logger.Errorf("[竞争度监控] 删除失败: %s: %v", t.Name, err)
				continue
			}
			removed[downloader.TorrentKey(t)] = true
			p.markDeletedForPeerRatio(dlID, dlName, t.InfoHash, o.seeds, o.leeches)
		} else {
			p.logger.Infof("[竞争度监控] 暂停: %s (S/L=%d/%d=%.1f > %.1f)", t.Name, o.seeds, o.leeches, o.ratio, maxSL)
			if err := dl.PauseTorrent(t.ID); err != nil {
				p.logger.Errorf("[竞争度监控] 暂停失败: %s: %v", t.Name, err)
				continue
			}
			p.markPausedForPeerRatio(dlID, dlName, t.InfoHash, o.seeds, o.leeches)
		}
		acted++
	}
	return acted
}

// protectionConfig 是保护规则用的全局设置；还没有全局设置时按默认的空设置（只认保留标签）。读库出错时返回错误。
func (p *PeerRatioMonitor) protectionConfig() (*models.SettingsGlobal, error) {
	var cfg models.SettingsGlobal
	err := p.db.First(&cfg).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return &models.SettingsGlobal{}, nil
	case err != nil:
		return nil, fmt.Errorf("读取全局设置失败: %w", err)
	}
	return &cfg, nil
}

func (p *PeerRatioMonitor) filterManagedSeedingTorrents(torrents []downloader.Torrent, dlName string) []downloader.Torrent {
	managedHashes := p.getManagedCompletedHashes(dlName)
	if len(managedHashes) == 0 {
		return nil
	}

	var result []downloader.Torrent
	for _, t := range torrents {
		if t.State != downloader.TorrentSeeding {
			continue
		}
		if _, ok := managedHashes[strings.ToLower(t.InfoHash)]; ok {
			result = append(result, t)
		}
	}
	return result
}

// getManagedCompletedHashes 是推到这台下载器（按编号认，见 recordsOn）、已经下完的 pt-tools 推送记录。
func (p *PeerRatioMonitor) getManagedCompletedHashes(dlName string) map[string]struct{} {
	hashes := make(map[string]struct{})
	var dbHashes []string
	recordsOn(p.db.Model(&models.TorrentInfo{}), downloaderIDByName(p.db, dlName), dlName).
		Where("torrent_hash IS NOT NULL AND torrent_hash != '' AND is_pushed IS NOT NULL AND is_completed = ?", true).
		Pluck("torrent_hash", &dbHashes)

	for _, h := range dbHashes {
		hashes[strings.ToLower(h)] = struct{}{}
	}
	return hashes
}

func (p *PeerRatioMonitor) getTrackerPeerCounts(dl downloader.Downloader, t downloader.Torrent) (seeds, leeches int, err error) {
	trackers, err := dl.GetTorrentTrackers(t.ID)
	if err != nil {
		return 0, 0, err
	}

	for _, tr := range trackers {
		if tr.Status < 2 {
			continue
		}
		if tr.Seeds > seeds {
			seeds = tr.Seeds
		}
		if tr.Leeches > leeches {
			leeches = tr.Leeches
		}
	}
	return seeds, leeches, nil
}

// peerRecords 是这台下载器上这个种子的推送记录（按编号认，见 recordsOn）：竞争度只读写这些，不碰别的下载器上同一个种子的记录。
func (p *PeerRatioMonitor) peerRecords(dlID *uint, dlName, infoHash string) *gorm.DB {
	return recordsOn(p.db.Model(&models.TorrentInfo{}), dlID, dlName).
		Where("LOWER(torrent_hash) = ?", strings.ToLower(infoHash))
}

func (p *PeerRatioMonitor) isAlreadyPausedForPeerRatio(dlID *uint, dlName, infoHash string) bool {
	var count int64
	p.peerRecords(dlID, dlName, infoHash).
		Where("is_paused_by_system = ? AND pause_reason = ?", true, PauseReasonPeerRatio).
		Count(&count)
	return count > 0
}

func (p *PeerRatioMonitor) updateDBPeerCounts(dlID *uint, dlName, infoHash string, seeds, leeches int) {
	now := time.Now()
	p.peerRecords(dlID, dlName, infoHash).
		Updates(map[string]any{
			"seeders":         seeds,
			"leechers":        leeches,
			"last_check_time": now,
		})
}

func (p *PeerRatioMonitor) markPausedForPeerRatio(dlID *uint, dlName, infoHash string, seeds, leeches int) {
	now := time.Now()
	p.peerRecords(dlID, dlName, infoHash).
		Updates(map[string]any{
			"is_paused_by_system": true,
			"paused_at":           now,
			"pause_reason":        PauseReasonPeerRatio,
			"seeders":             seeds,
			"leechers":            leeches,
			"last_check_time":     now,
		})
}

func (p *PeerRatioMonitor) markDeletedForPeerRatio(dlID *uint, dlName, infoHash string, seeds, leeches int) {
	now := time.Now()
	p.peerRecords(dlID, dlName, infoHash).
		Updates(map[string]any{
			"is_expired":      true,
			"pause_reason":    PauseReasonPeerRatio,
			"seeders":         seeds,
			"leechers":        leeches,
			"last_check_time": now,
		})
}
