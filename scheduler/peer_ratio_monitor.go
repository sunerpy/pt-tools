package scheduler

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
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
	}
}

func (p *PeerRatioMonitor) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return nil
	}
	p.running = true

	go p.runLoop()
	p.logger.Info("[竞争度监控] 服务已启动")
	return nil
}

func (p *PeerRatioMonitor) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return
	}
	p.cancel()
	p.running = false
	p.logger.Info("[竞争度监控] 服务已停止")
}

func (p *PeerRatioMonitor) runLoop() {
	time.Sleep(15 * time.Second)

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

func (p *PeerRatioMonitor) processDownloader(dl downloader.Downloader, dlName string, maxSL float64, removeData bool) {
	allTorrents, err := dl.GetAllTorrents()
	if err != nil {
		p.logger.Errorf("[竞争度监控] %s: 获取种子列表失败: %v", dlName, err)
		return
	}

	managed := p.filterManagedSeedingTorrents(allTorrents, dlName)
	if len(managed) == 0 {
		p.logger.Debugf("[竞争度监控] %s: 无做种中的管理种子", dlName)
		return
	}

	var acted int
	for _, t := range managed {
		if p.ctx.Err() != nil {
			return
		}

		seeds, leeches, err := p.getTrackerPeerCounts(dl, t)
		if err != nil {
			p.logger.Debugf("[竞争度监控] %s: 获取 tracker 信息失败: %s: %v", dlName, t.Name, err)
			continue
		}

		p.updateDBPeerCounts(t.InfoHash, seeds, leeches)

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

		if p.isAlreadyPausedForPeerRatio(t.InfoHash) {
			continue
		}

		if removeData {
			p.logger.Infof("[竞争度监控] 删除: %s (S/L=%d/%d=%.1f > %.1f)", t.Name, seeds, leeches, ratio, maxSL)
			if err := dl.RemoveTorrent(t.ID, true); err != nil {
				p.logger.Errorf("[竞争度监控] 删除失败: %s: %v", t.Name, err)
				continue
			}
			p.markDeletedForPeerRatio(t.InfoHash, seeds, leeches)
		} else {
			p.logger.Infof("[竞争度监控] 暂停: %s (S/L=%d/%d=%.1f > %.1f)", t.Name, seeds, leeches, ratio, maxSL)
			if err := dl.PauseTorrent(t.ID); err != nil {
				p.logger.Errorf("[竞争度监控] 暂停失败: %s: %v", t.Name, err)
				continue
			}
			p.markPausedForPeerRatio(t.InfoHash, seeds, leeches)
		}
		acted++
	}

	action := "暂停"
	if removeData {
		action = "删除"
	}
	p.logger.Infof("[竞争度监控] %s: 检查完成 (管理做种=%d, %s=%d)", dlName, len(managed), action, acted)
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

func (p *PeerRatioMonitor) getManagedCompletedHashes(dlName string) map[string]struct{} {
	hashes := make(map[string]struct{})
	var dbHashes []string
	p.db.Model(&models.TorrentInfo{}).
		Where("torrent_hash IS NOT NULL AND torrent_hash != '' AND is_pushed IS NOT NULL AND is_completed = ? AND downloader_name = ?", true, dlName).
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

func (p *PeerRatioMonitor) isAlreadyPausedForPeerRatio(infoHash string) bool {
	var count int64
	p.db.Model(&models.TorrentInfo{}).
		Where("torrent_hash = ? AND is_paused_by_system = ? AND pause_reason = ?",
			strings.ToLower(infoHash), true, PauseReasonPeerRatio).
		Count(&count)
	return count > 0
}

func (p *PeerRatioMonitor) updateDBPeerCounts(infoHash string, seeds, leeches int) {
	now := time.Now()
	p.db.Model(&models.TorrentInfo{}).
		Where("torrent_hash = ?", strings.ToLower(infoHash)).
		Updates(map[string]any{
			"seeders":         seeds,
			"leechers":        leeches,
			"last_check_time": now,
		})
}

func (p *PeerRatioMonitor) markPausedForPeerRatio(infoHash string, seeds, leeches int) {
	now := time.Now()
	p.db.Model(&models.TorrentInfo{}).
		Where("torrent_hash = ?", strings.ToLower(infoHash)).
		Updates(map[string]any{
			"is_paused_by_system": true,
			"paused_at":           now,
			"pause_reason":        PauseReasonPeerRatio,
			"seeders":             seeds,
			"leechers":            leeches,
			"last_check_time":     now,
		})
}

func (p *PeerRatioMonitor) markDeletedForPeerRatio(infoHash string, seeds, leeches int) {
	now := time.Now()
	p.db.Model(&models.TorrentInfo{}).
		Where("torrent_hash = ?", strings.ToLower(infoHash)).
		Updates(map[string]any{
			"is_expired":      true,
			"pause_reason":    PauseReasonPeerRatio,
			"seeders":         seeds,
			"leechers":        leeches,
			"last_check_time": now,
		})
}
