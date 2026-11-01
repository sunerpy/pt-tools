package scheduler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 做种竞争度：自动删种的一条删除条件（v16 以前是单独的监控）。正在做种的 pt-tools 推送的种子，整个种子群的做种人数比下载人数
// 超过上限时，按设置删除或只暂停。竞争度暂停过、用户又手动开始了的种子不再暂停。

// swarmCount 是种子整个种子群的做种与下载人数。
type swarmCount struct{ seeds, leeches int }

// swarmReader 读种子整个种子群的做种与下载人数：种子列表里给了就用（qBittorrent）；没给时这一轮一次读出全部种子的 tracker 状态
// （Transmission，见 downloader.BulkTrackerReader），读不出或其中没有这个种子时逐个读 tracker。
// tracker 只看工作中的（状态 2 以上），做种、下载人数各取最大值。
type swarmReader struct {
	ctx      context.Context
	dl       downloader.Downloader
	bulk     map[string][]downloader.TorrentTracker
	bulkRead bool
}

func (r *swarmReader) counts(t downloader.Torrent) (swarmCount, error) {
	if t.SwarmSeeds >= 0 && t.SwarmLeechers >= 0 {
		return swarmCount{seeds: t.SwarmSeeds, leeches: t.SwarmLeechers}, nil
	}
	if !r.bulkRead {
		r.bulkRead = true
		if br, ok := r.dl.(downloader.BulkTrackerReader); ok {
			if m, err := br.GetAllTorrentTrackers(r.ctx); err == nil {
				r.bulk = m
			}
		}
	}
	trackers, ok := r.bulk[strings.ToLower(t.InfoHash)]
	if !ok {
		var err error
		if trackers, err = r.dl.GetTorrentTrackers(t.ID); err != nil {
			return swarmCount{}, err
		}
	}
	var sc swarmCount
	for _, tr := range trackers {
		if tr.Status < 2 {
			continue
		}
		sc.seeds, sc.leeches = max(sc.seeds, tr.Seeds), max(sc.leeches, tr.Leeches)
	}
	return sc, nil
}

// peerRecord 是竞争度要看的一条推送记录：这台下载器上 pt-tools 推送过的（is_pushed 不为空；不看归档表，与原来的竞争度监控一样）。
type peerRecord struct {
	Hash string
	// Overridden 是竞争度暂停以后用户手动开始过；Paused 是记录标着竞争度暂停
	Overridden bool
	Paused     bool
}

// peerRecordsOn 是这台下载器上 pt-tools 推送过的记录（按下载器编号认，见 recordsOn），键是小写 info hash。
func (c *CleanupMonitor) peerRecordsOn(dlName string) (map[string][]peerRecord, error) {
	var rows []peerRecord
	if err := recordsOn(c.db.Model(&models.TorrentInfo{}), c.downloaderID(dlName), dlName).
		Select("LOWER(torrent_hash) AS hash, peer_pause_overridden AS overridden, "+
			"CASE WHEN is_paused_by_system = ? AND pause_reason = ? THEN 1 ELSE 0 END AS paused", true, PauseReasonPeerRatio).
		Where("torrent_hash IS NOT NULL AND torrent_hash != '' AND is_pushed IS NOT NULL").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取推送记录失败: %w", err)
	}
	out := make(map[string][]peerRecord, len(rows))
	for _, r := range rows {
		out[r.Hash] = append(out[r.Hash], r)
	}
	return out, nil
}

// peerJudge 判断这一轮的做种竞争度（打开了竞争度时）。
type peerJudge struct {
	maxSL   float64
	records map[string][]peerRecord
	swarm   *swarmReader
	// unknown 是读不到整个种子群人数、这一轮没判断的种子数
	unknown int
}

// judge 报告种子 t 的做种竞争度是否超标，超标时给原因与人数。只判断正在做种、已经下完、这台下载器上有 pt-tools 推送记录
// （推送前的暂存记录不算）、记录都没记着手动开始过的种子。记录标着竞争度暂停、下载器里却在做种的，是用户手动开始了：
// 真的执行时这一轮开头已经改成手动开始过（见 releasePeerPauses）；预览不改库，在这里同样不判断。
// 做种人数为 0 时不判断；没有人在下载时 S/L 按做种人数算。
func (p *peerJudge) judge(t downloader.Torrent) (string, swarmCount, bool) {
	if t.State != downloader.TorrentSeeding || t.Progress < 1 {
		return "", swarmCount{}, false
	}
	recs := p.records[strings.ToLower(t.InfoHash)]
	if len(recs) == 0 {
		return "", swarmCount{}, false
	}
	for _, r := range recs {
		if r.Overridden || r.Paused {
			return "", swarmCount{}, false
		}
	}
	sc, err := p.swarm.counts(t)
	if err != nil {
		p.unknown++
		return "", swarmCount{}, false
	}
	if sc.seeds <= 0 {
		return "", sc, false
	}
	ratio := float64(sc.seeds)
	if sc.leeches > 0 {
		ratio /= float64(sc.leeches)
	}
	if ratio <= p.maxSL {
		return "", sc, false
	}
	if sc.leeches == 0 {
		return fmt.Sprintf("做种竞争度过高：%d 人做种、没有人在下载（S/L 按 %d 算），超过 %g", sc.seeds, sc.seeds, p.maxSL), sc, true
	}
	return fmt.Sprintf("做种竞争度过高：%d 人做种、%d 人下载（S/L %.1f），超过 %g", sc.seeds, sc.leeches, ratio, p.maxSL), sc, true
}

// releasePeerPauses 认出竞争度暂停以后用户又手动开始了的种子（在下载器里，或在「暂停任务」页）：记录标着竞争度暂停，
// 下载器里却在做种、下载或排队。清掉暂停标记、记下手动开始过，竞争度以后不再暂停它（别的删除条件照常）。
// 暂停、停止着的仍算系统暂停的（qB 新版的暂停是 stop）；校验、出错、状态不知道的这一轮不判断。
// 只在真的执行的一轮做（竞争度关着也做，「暂停任务」页上才不会留着早就开始了的种子）；预览不改库。
func (c *CleanupMonitor) releasePeerPauses(dlName string, all []downloader.Torrent) {
	started := map[string]string{} // 小写 info hash → 种子名
	for _, t := range all {
		switch t.State {
		case downloader.TorrentSeeding, downloader.TorrentDownloading, downloader.TorrentQueued:
			if t.InfoHash != "" {
				started[strings.ToLower(t.InfoHash)] = t.Name
			}
		}
	}
	if len(started) == 0 {
		return
	}
	dlID := c.downloaderID(dlName)
	pausedOn := func() *gorm.DB {
		return recordsOn(c.db.Model(&models.TorrentInfo{}), dlID, dlName).
			Where("is_paused_by_system = ? AND pause_reason = ?", true, PauseReasonPeerRatio)
	}
	var hashes []string
	if err := pausedOn().Pluck("torrent_hash", &hashes).Error; err != nil {
		c.logger.Errorf("[自动删种] %s: 读取竞争度暂停的记录失败: %v", dlName, err)
		return
	}
	now := time.Now()
	done := map[string]bool{}
	for _, h := range hashes {
		h = strings.ToLower(h)
		name, ok := started[h]
		if !ok || done[h] {
			continue
		}
		done[h] = true
		if err := pausedOn().Where("LOWER(torrent_hash) = ?", h).Updates(map[string]any{
			"is_paused_by_system":   false,
			"pause_reason":          "",
			"paused_at":             nil,
			"peer_pause_overridden": true,
			"last_check_time":       now,
		}).Error; err != nil {
			c.logger.Errorf("[自动删种] %s: 记下 %s 手动开始过失败: %v", dlName, name, err)
			continue
		}
		c.logger.Infof("[自动删种] %s: %s 竞争度暂停以后手动开始了，竞争度不再暂停它", dlName, name)
	}
}

// markPeerPaused 把这一轮因竞争度暂停的种子在这台下载器上的记录标成系统暂停，记下当时的人数：「暂停任务」页列出它们，可以手动恢复。
func (c *CleanupMonitor) markPeerPaused(dlName string, paused []downloader.Torrent, peers map[string]swarmCount) {
	dlID := c.downloaderID(dlName)
	now := time.Now()
	for _, t := range paused {
		sc := peers[t.ID]
		if err := recordsOn(c.db.Model(&models.TorrentInfo{}), dlID, dlName).
			Where("LOWER(torrent_hash) = ?", strings.ToLower(t.InfoHash)).
			Updates(map[string]any{
				"is_paused_by_system": true,
				"paused_at":           now,
				"pause_reason":        PauseReasonPeerRatio,
				"seeders":             sc.seeds,
				"leechers":            sc.leeches,
				"last_check_time":     now,
			}).Error; err != nil {
			c.logger.Errorf("[自动删种] %s: 记下 %s 已暂停失败: %v", dlName, t.Name, err)
		}
	}
}
