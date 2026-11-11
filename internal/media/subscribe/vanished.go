package subscribe

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 下载中的订阅种子从下载器里没了（刷流、手动或别的工具删掉了）：以前要等 7 天才判失败，这几集一直显示「在下载」、
// 也不去找别的种子（2026-11-09 刷流的季包被认领成第 54 集的来源，几小时后删了）。现在连着 vanishGrace 都看不到就判失败，
// 这几集马上重新算缺的，这个订阅马上搜一次。
//
// 再推过一次、还在下载器里的种子（见 repush.go）以前整理过，整理入库的定期补查不再看它，只靠下载完成的事件整理；事件丢了、
// 或者在下载器算出新的进度以前就发了，这几集就一直不入库。所以这样的种子在下载器里下完了、过了 vanishGrace 还没有新的
// 整理记录时，订阅自己让整理入库整理一次（每个种子每 reorgEvery 最多一次）。

// vanishGrace 是下载中的订阅种子在下载器里连着看不到多久才判失败：下载器一时列不全（刚重启、刚推送）的不判。
const vanishGrace = 10 * time.Minute

// reorgEvery 是订阅让整理入库整理同一个再推过的种子的最短间隔。
const reorgEvery = time.Hour

// dropVanished 把下载器里已经没有了的下载中的订阅种子判成失败（rows 是这一拍刷新以前读到的下载中的种子；
// 刷新时已经标成入库或失败的，条件更新不会再改）。下载器列不出来、列出来是空的不判；第一次看不到时只记下时间，
// 连着 vanishGrace 都看不到、记录也已经记下 vanishGrace 以上才判。
func (s *Service) dropVanished(ctx context.Context, rows []models.MediaSubscriptionTorrent) {
	if s.cfg.Downloaders == nil {
		return
	}
	now := s.cfg.Now()
	lists := map[uint]map[string]downloader.Torrent{}
	current := make(map[uint]bool, len(rows))
	for i := range rows {
		t := &rows[i]
		current[t.ID] = true
		if t.InfoHash == "" || t.DownloaderID == 0 {
			continue
		}
		have, ok := lists[t.DownloaderID]
		if !ok {
			have = s.presentHashes(ctx, t.DownloaderID)
			lists[t.DownloaderID] = have
		}
		if have == nil {
			continue
		}
		if dt, found := have[strings.ToLower(t.InfoHash)]; found {
			s.forgetGone(t.ID)
			if t.RepushedAt != nil && dt.Progress >= 1 {
				s.organizeRepushed(ctx, t, now)
			}
			continue
		}
		first := s.noteGone(t.ID, now)
		if now.Sub(first) < vanishGrace || now.Sub(laterOf(t.CreatedAt, t.UpdatedAt)) < vanishGrace {
			continue
		}
		s.failVanished(ctx, t, now)
	}
	s.goneMu.Lock()
	for id := range s.gone {
		if !current[id] {
			delete(s.gone, id)
		}
	}
	for id := range s.reorg {
		if !current[id] {
			delete(s.reorg, id)
		}
	}
	s.goneMu.Unlock()
}

// presentHashes 是下载器里现在的种子（按小写的 info hash）；列不出来或是空的时返回 nil（看不出种子在不在）。
func (s *Service) presentHashes(ctx context.Context, dlID uint) map[string]downloader.Torrent {
	list, err := s.downloaderTorrents(ctx, dlID)
	if err != nil || len(list) == 0 {
		return nil
	}
	have := make(map[string]downloader.Torrent, len(list))
	for _, d := range list {
		have[strings.ToLower(d.InfoHash)] = d
	}
	return have
}

// organizeRepushed 让整理入库整理一个再推过一次、在下载器里已经下完的订阅种子：再推以后过了 vanishGrace、
// 还没有新的整理记录，离订阅上次让它整理也过了 reorgEvery。按订阅的条目与媒体库整理（和「重新整理」一样），
// 已经整理好的文件不动；整理出新的记录以后，下一拍的刷新把这个种子标成已入库。
func (s *Service) organizeRepushed(ctx context.Context, t *models.MediaSubscriptionTorrent, now time.Time) {
	if s.cfg.Organizer == nil || t.InfoHash == "" || now.Sub(*t.RepushedAt) < vanishGrace {
		return
	}
	if hist, err := s.historyFor(ctx, t); err != nil || len(hist) > 0 {
		return
	}
	s.goneMu.Lock()
	last, tried := s.reorg[t.ID]
	if tried && now.Sub(last) < reorgEvery {
		s.goneMu.Unlock()
		return
	}
	s.reorg[t.ID] = now
	s.goneMu.Unlock()
	sub, err := s.subRow(ctx, t.SubscriptionID)
	if err != nil {
		return
	}
	res, err := s.cfg.Organizer.Organize(ctx, organize.Request{
		DownloaderID: t.DownloaderID, Hash: t.InfoHash, MediaType: sub.MediaType, TMDBID: sub.TMDBID, LibraryID: sub.LibraryID,
	})
	if err != nil {
		s.cfg.Logger.Warnf("[订阅] 再下的「%s」下完了还没整理入库，让整理入库整理没有成功: %v", t.Title, err)
		return
	}
	s.cfg.Logger.Infof("[订阅] 再下的「%s」下完了还没整理入库，让整理入库整理了一次：%s", t.Title, organizeText(res))
}

// noteGone 记下种子第一次看不到的时间（已经记过的不改），返回第一次看不到的时间。
func (s *Service) noteGone(id uint, now time.Time) time.Time {
	s.goneMu.Lock()
	defer s.goneMu.Unlock()
	if at, ok := s.gone[id]; ok {
		return at
	}
	s.gone[id] = now
	return now
}

func (s *Service) forgetGone(id uint) {
	s.goneMu.Lock()
	delete(s.gone, id)
	s.goneMu.Unlock()
}

// failVanished 把一个已经不在下载器里的下载中的订阅种子判成失败；判成了的话订阅马上搜一次（追剧记一条运行记录）。
func (s *Service) failVanished(ctx context.Context, t *models.MediaSubscriptionTorrent, now time.Time) {
	eps := "这些集"
	if list := rowEpisodes(*t); len(list) > 0 {
		eps = episodesText(list)
	}
	res := s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscriptionTorrent{}).
		Where("id = ? AND status = ?", t.ID, models.MediaSubTorrentDownloading).
		Updates(map[string]any{
			"status": models.MediaSubTorrentFailed, "message": truncate("种子已经不在下载器里了（被删掉了），"+eps+"重新算缺的", 1024),
			"updated_at": now,
		})
	if res.Error != nil {
		s.cfg.Logger.Warnf("[订阅] 标记不在下载器里的种子 %s 失败: %v", t.Title, res.Error)
		return
	}
	s.forgetGone(t.ID)
	if res.RowsAffected != 1 {
		return
	}
	what := "「" + t.Title + "」"
	if t.Adopted && strings.HasPrefix(t.SiteName, models.MediaAdoptedSitePrefix) {
		what = "认领的" + what
	}
	msg := fmt.Sprintf("%s已经不在下载器里了（可能被刷流或手动删掉），%s重新算缺的，马上重新找", what, eps)
	s.cfg.Logger.Infof("[订阅] %s", msg)
	sub, err := s.subRow(ctx, t.SubscriptionID)
	if err != nil || sub.Status != models.MediaSubActive {
		return
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).
		Update("next_search_at", now).Error; err != nil {
		s.cfg.Logger.Warnf("[订阅] 提前「%s」的下次搜索失败: %v", subTitle(&sub), err)
	}
	rctx, run := s.startRun(ctx, &sub, models.MediaRunSchedule)
	step(rctx, stepInfo, "%s", msg)
	markRun(rctx, func(rl *runLog) { rl.changed = true })
	s.finishRun(rctx, &sub, run, msg, false)
}

// rowEpisodes 是订阅的一个种子里的集：核对过文件列表的按核对的，否则按记下的集数；整季包、电影为空。
func rowEpisodes(t models.MediaSubscriptionTorrent) []int {
	if t.CoveredEpisodes != "" {
		var eps []int
		if json.Unmarshal([]byte(t.CoveredEpisodes), &eps) == nil && len(eps) > 0 {
			return eps
		}
	}
	if t.Complete || t.Episode <= 0 {
		return nil
	}
	out := make([]int, 0, max(t.EpisodeEnd, t.Episode)-t.Episode+1)
	for n := t.Episode; n <= max(t.EpisodeEnd, t.Episode); n++ {
		out = append(out, n)
	}
	return out
}
