package subscribe

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 推送一个有几集的种子时只下当时缺的集，别的集在下载器里设成不下载（见 packfiles.go）。设成不下载的集之后又缺了——
// 当时以为它在别处下载（认领的种子被删了）、或者库里那一集被删了——以前这个种子已经记在订阅里，搜索时一律跳过，
// 版本锁又只要这个版本，这一集就一直等（2026-11-04 LongWeb S01E44-S01E45 只下了 E45，第 44 集再也没下）。
// 现在这样的种子可以再推一次：下载器里还有就把这几集的文件改回下载，没有了就重新加进去、只下这几集。

// skippedOf 是这个订阅种子推送时设成不下载的集：记下了的按记下的；升级前推送的、不是整季包的按种子名的集减去实际要下的集；
// 别的（整季包、单集、没选过文件）为空。
func skippedOf(t models.MediaSubscriptionTorrent) []int {
	if t.SkippedEpisodes != "" {
		var eps []int
		if json.Unmarshal([]byte(t.SkippedEpisodes), &eps) == nil {
			return eps
		}
		return nil
	}
	if t.Complete || t.CoveredEpisodes == "" || t.EpisodeEnd <= t.Episode {
		return nil
	}
	has := covered(t)
	var out []int
	for n := t.Episode; n <= t.EpisodeEnd; n++ {
		if !has(n) {
			out = append(out, n)
		}
	}
	return out
}

// repushable 报告订阅已经下载过的这个种子要不要再推一次：订阅自己推送、已经整理入库，推送时设成不下载的集里有现在缺的。
func repushable(t models.MediaSubscriptionTorrent, lg *ledger) bool {
	if lg == nil || t.Status != models.MediaSubTorrentDone || t.Adopted || strings.HasPrefix(t.SiteName, models.MediaAdoptedSitePrefix) {
		return false
	}
	skipped := skippedOf(t)
	return slices.ContainsFunc(lg.missing, func(n int) bool { return slices.Contains(skipped, n) })
}

// wantedSkipped 是推送时设成不下载、现在又缺的集（want 是这一轮缺的集）。
func wantedSkipped(t models.MediaSubscriptionTorrent, want map[int]bool) []int {
	var out []int
	for _, n := range skippedOf(t) {
		if want[n] {
			out = append(out, n)
		}
	}
	return out
}

// wantFiles 在下载器里把种子中有要的集、设成了不下载的视频文件改回下载，返回种子里有的要的集与改回下载的文件个数。
// 下载器不支持时返回 downloader.ErrCapabilityUnsupported；文件列表里对不出要的集时返回错误。
func (s *Service) wantFiles(ctx context.Context, dlID uint, hash string, mp *episodeMapper, want map[int]bool) ([]int, int, error) {
	if s.cfg.Downloaders == nil {
		return nil, 0, downloader.ErrCapabilityUnsupported
	}
	dl, _, err := s.cfg.Downloaders.Get(ctx, dlID)
	if err != nil {
		return nil, 0, err
	}
	fs, ok := dl.(downloader.FileSelector)
	if !ok {
		return nil, 0, downloader.ErrCapabilityUnsupported
	}
	files, err := waitFiles(ctx, dl, hash)
	if err != nil {
		return nil, 0, err
	}
	kept := map[int]bool{}
	var on []int
	for _, f := range files {
		eps, _ := s.videoEpisodes(ctx, mp, f.Name)
		hit := false
		for _, n := range eps {
			if want[n] {
				kept[n], hit = true, true
			}
		}
		if hit && f.Priority == 0 {
			on = append(on, f.Index)
		}
	}
	if len(kept) == 0 {
		return nil, 0, errors.New("下载器的文件列表里对不出缺的集")
	}
	if len(on) > 0 {
		if err := fs.SetFilePriority(ctx, hash, on, 1); err != nil {
			return nil, 0, err
		}
	}
	return slices.Sorted(maps.Keys(kept)), len(on), nil
}

// markRepushed 记下再推了一次的订阅种子：又在下载这几集（kept），这一次设成不下载的是 skipped，记下这一次的时间。
// 只改还是已入库的那条（条件更新）。
func (s *Service) markRepushed(ctx context.Context, t *models.MediaSubscriptionTorrent, hash string, dlID uint, kept, skipped []int, now time.Time) {
	cov, _ := json.Marshal(kept)
	// 没有设成不下载的集时记空数组：空串会按种子名重新算（见 skippedOf）
	skip := "[]"
	if len(skipped) > 0 {
		b, _ := json.Marshal(skipped)
		skip = string(b)
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscriptionTorrent{}).
		Where("id = ? AND status = ?", t.ID, models.MediaSubTorrentDone).
		Updates(map[string]any{
			"status": models.MediaSubTorrentDownloading, "covered_episodes": string(cov), "skipped_episodes": skip,
			"info_hash": hash, "downloader_id": dlID, "repushed_at": now, "updated_at": now,
			"message": truncate("再下一次，补上当时没下的 "+episodeList(kept), 1024),
		}).Error; err != nil {
		s.cfg.Logger.Warnf("[订阅] 记下再推的种子 %s 失败: %v", t.Title, err)
	}
}

// resetCompleted 把种子记录改回没下完（下载器里还有这个种子、刚把一些文件改回下载）：进度监控接着看它，
// 下完时再发完成事件，整理入库整理这一次下的文件。
func (s *Service) resetCompleted(ctx context.Context, site, torrentID string) {
	if err := s.cfg.DB.WithContext(ctx).Model(&models.TorrentInfo{}).
		Where("site_name = ? AND torrent_id = ?", site, torrentID).
		Updates(map[string]any{"is_completed": false, "completed_at": nil}).Error; err != nil {
		s.cfg.Logger.Warnf("[订阅] 把种子记录改回没下完失败 (%s %s): %v", site, torrentID, err)
	}
}
