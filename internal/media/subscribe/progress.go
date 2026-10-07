package subscribe

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// Progress 是订阅的进度。电影：Total 为 1，入了库 InLibrary 为 1。剧集按 TMDB 这一季的分集算，只算已经播出的。
type Progress struct {
	Total       int   `json:"total"`
	Aired       int   `json:"aired"`
	InLibrary   int   `json:"in_library"`
	Downloading int   `json:"downloading"`
	Missing     []int `json:"missing,omitempty"`
	// Episodes 是每一集的情况（详情里才有）
	Episodes []EpisodeState `json:"episodes,omitempty"`
}

// EpisodeState 是一集的情况：library 已入库、downloading 下载中、missing 缺、upcoming 还没播出。
type EpisodeState struct {
	Number  int    `json:"number"`
	Name    string `json:"name,omitempty"`
	AirDate string `json:"air_date,omitempty"`
	State   string `json:"state"`
}

// 一集的情况。
const (
	EpisodeLibrary     = "library"
	EpisodeDownloading = "downloading"
	EpisodeMissing     = "missing"
	EpisodeUpcoming    = "upcoming"
)

func yearOf(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, _ := strconv.Atoi(date[:4])
	return y
}

// libraryEpisodes 是整理记录里这一季已入库的集。
func (s *Service) libraryEpisodes(ctx context.Context, sub *models.MediaSubscription) (map[int]bool, error) {
	var rows []models.MediaTransferHistory
	if err := s.cfg.DB.WithContext(ctx).Select("episode", "episode_end").
		Where("status = ? AND media_type = ? AND tmdb_id = ? AND season = ?", models.MediaTransferDone, models.MediaKindTV, sub.TMDBID, sub.Season).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取整理记录失败: %w", err)
	}
	have := map[int]bool{}
	for _, r := range rows {
		for n := r.Episode; n > 0 && n <= max(r.EpisodeEnd, r.Episode); n++ {
			have[n] = true
		}
	}
	return have, nil
}

// movieInLibrary 报告电影有没有已整理的记录。
func (s *Service) movieInLibrary(ctx context.Context, sub *models.MediaSubscription) (bool, error) {
	var n int64
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).
		Where("status = ? AND media_type = ? AND tmdb_id = ?", models.MediaTransferDone, models.MediaKindMovie, sub.TMDBID).
		Count(&n).Error; err != nil {
		return false, fmt.Errorf("读取整理记录失败: %w", err)
	}
	return n > 0, nil
}

func (s *Service) torrents(ctx context.Context, subID uint, statuses ...string) ([]models.MediaSubscriptionTorrent, error) {
	var rows []models.MediaSubscriptionTorrent
	db := s.cfg.DB.WithContext(ctx).Where("subscription_id = ?", subID)
	if len(statuses) > 0 {
		db = db.Where("status IN ?", statuses)
	}
	if err := db.Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取订阅的种子失败: %w", err)
	}
	return rows, nil
}

func spanOf(t models.MediaSubscriptionTorrent) span {
	return span{Start: t.Episode, End: max(t.EpisodeEnd, t.Episode), Whole: t.Complete}
}

// progress 算订阅的进度（detail 为真时带上每一集）。
func (s *Service) progress(ctx context.Context, sub *models.MediaSubscription, detail bool) (*Progress, error) {
	downloading, err := s.torrents(ctx, sub.ID, models.MediaSubTorrentDownloading)
	if err != nil {
		return nil, err
	}
	if sub.MediaType == models.MediaKindMovie {
		in, merr := s.movieInLibrary(ctx, sub)
		if merr != nil {
			return nil, merr
		}
		p := &Progress{Total: 1, Aired: 1}
		switch {
		case in:
			p.InLibrary = 1
		case len(downloading) > 0:
			p.Downloading = 1
		default:
			p.Missing = []int{1}
		}
		return p, nil
	}
	c, err := s.cfg.Recognizer.TMDB(ctx)
	if err != nil {
		return nil, err
	}
	season, err := c.Season(ctx, sub.TMDBID, sub.Season)
	if err != nil {
		return nil, err
	}
	have, err := s.libraryEpisodes(ctx, sub)
	if err != nil {
		return nil, err
	}
	today := s.cfg.Now().Format(time.DateOnly)
	p := &Progress{Total: len(season.Episodes)}
	for _, e := range season.Episodes {
		state := EpisodeUpcoming
		aired := e.AirDate != "" && e.AirDate <= today
		if aired {
			p.Aired++
		}
		switch {
		case have[e.Number]:
			state = EpisodeLibrary
			p.InLibrary++
		case slices.ContainsFunc(downloading, func(t models.MediaSubscriptionTorrent) bool { return spanOf(t).covers(e.Number) }):
			state = EpisodeDownloading
			p.Downloading++
		case aired:
			state = EpisodeMissing
			p.Missing = append(p.Missing, e.Number)
		}
		if detail {
			p.Episodes = append(p.Episodes, EpisodeState{Number: e.Number, Name: e.Name, AirDate: e.AirDate, State: state})
		}
	}
	return p, nil
}

// stuckAfter 是订阅下载的种子过了多久还没整理入库就算失败（这些集重新算缺的，之后的搜索会再找）。
const stuckAfter = 7 * 24 * time.Hour

// refresh 更新订阅下载的种子与订阅的状态：入库了的种子标成完成（洗版时替换旧版本），太久没入库的标成失败；
// 电影入了库、剧集这一季都入了库的订阅标成完成（洗版要达到目标质量）。
func (s *Service) refresh(ctx context.Context) {
	set, err := s.Settings(ctx)
	if err != nil {
		return
	}
	var rows []models.MediaSubscriptionTorrent
	if err := s.cfg.DB.WithContext(ctx).Where("status = ?", models.MediaSubTorrentDownloading).Order("id").Find(&rows).Error; err != nil {
		s.cfg.Logger.Warnf("[订阅] 读取下载中的种子失败: %v", err)
		return
	}
	for i := range rows {
		s.refreshTorrent(ctx, &rows[i], set)
	}
	var subs []models.MediaSubscription
	if err := s.cfg.DB.WithContext(ctx).Where("status = ?", models.MediaSubActive).Find(&subs).Error; err != nil {
		s.cfg.Logger.Warnf("[订阅] 读取订阅失败: %v", err)
		return
	}
	for i := range subs {
		s.checkDone(ctx, &subs[i], set)
	}
}

func (s *Service) setTorrent(ctx context.Context, id uint, status, msg string) {
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscriptionTorrent{}).Where("id = ?", id).
		Updates(map[string]any{"status": status, "message": truncate(msg, 1024), "updated_at": s.cfg.Now()}).Error; err != nil {
		s.cfg.Logger.Warnf("[订阅] 更新种子记录失败: %v", err)
	}
}

func (s *Service) refreshTorrent(ctx context.Context, t *models.MediaSubscriptionTorrent, set Settings) {
	var hist []models.MediaTransferHistory
	if t.InfoHash != "" {
		if err := s.cfg.DB.WithContext(ctx).Where("info_hash = ?", strings.ToLower(t.InfoHash)).Find(&hist).Error; err != nil {
			return
		}
	}
	done := slices.ContainsFunc(hist, func(h models.MediaTransferHistory) bool { return h.Status == models.MediaTransferDone })
	sub, err := s.subRow(ctx, t.SubscriptionID)
	if err != nil {
		return
	}
	if done {
		s.setTorrent(ctx, t.ID, models.MediaSubTorrentDone, "已整理入库")
		if sub.Upgrade {
			s.replaceOlder(ctx, &sub, t, set)
		}
		return
	}
	// 洗版的新版本和旧版本整理到同一个文件名时，整理记成「目标已有同名文件」：先删掉旧版本库里的文件再重试
	if sub.Upgrade {
		for _, h := range hist {
			if h.Status == models.MediaTransferSkipped && s.freeTarget(ctx, &sub, &h, t) {
				if _, err := s.cfg.Organizer.Retry(ctx, h.ID); err != nil {
					s.cfg.Logger.Warnf("[订阅] 洗版后重新整理失败 (%s): %v", h.TorrentName, err)
				}
			}
		}
	}
	if s.cfg.Now().Sub(t.CreatedAt) > stuckAfter {
		s.setTorrent(ctx, t.ID, models.MediaSubTorrentFailed, "7 天还没整理入库，这些集重新算缺的")
	}
}

// sameMedia 是和订阅同一个条目（剧集还要同一季）的已整理记录。
func (s *Service) sameMedia(ctx context.Context, sub *models.MediaSubscription) ([]models.MediaTransferHistory, error) {
	db := s.cfg.DB.WithContext(ctx).Where("status = ? AND media_type = ? AND tmdb_id = ?", models.MediaTransferDone, sub.MediaType, sub.TMDBID)
	if sub.MediaType == models.MediaKindTV {
		db = db.Where("season = ?", sub.Season)
	}
	var rows []models.MediaTransferHistory
	if err := db.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取整理记录失败: %w", err)
	}
	return rows, nil
}

// freeTarget 在洗版的新版本整理时撞上旧版本的同名文件时，删掉旧版本库里的文件；删了返回真。
func (s *Service) freeTarget(ctx context.Context, sub *models.MediaSubscription, skipped *models.MediaTransferHistory, t *models.MediaSubscriptionTorrent) bool {
	if skipped.TargetPath == "" || s.cfg.Organizer == nil {
		return false
	}
	old, err := s.sameMedia(ctx, sub)
	if err != nil {
		return false
	}
	freed := false
	for _, o := range old {
		if o.TargetPath != skipped.TargetPath || strings.EqualFold(o.InfoHash, t.InfoHash) {
			continue
		}
		if _, err := s.cfg.Organizer.Retire(ctx, o.ID, "洗版：换成了 "+t.Title); err != nil {
			s.cfg.Logger.Warnf("[订阅] 洗版时删除旧版本失败 (%s): %v", o.TargetPath, err)
			continue
		}
		freed = true
	}
	return freed
}

// replaceOlder 在洗版的新版本入库以后，删掉这个条目旧版本在库里的文件，旧种子按设置继续做种或删除。
// 剧集只换新种子包括的集（整季包换整季）：一集一集下载时，新的一集不会把前面的集当成旧版本。
func (s *Service) replaceOlder(ctx context.Context, sub *models.MediaSubscription, t *models.MediaSubscriptionTorrent, set Settings) {
	sp := spanOf(*t)
	covered := func(start int) bool { return sub.MediaType == models.MediaKindMovie || sp.covers(start) }
	if s.cfg.Organizer != nil {
		old, err := s.sameMedia(ctx, sub)
		if err != nil {
			return
		}
		for _, o := range old {
			if strings.EqualFold(o.InfoHash, t.InfoHash) || !covered(o.Episode) {
				continue
			}
			if _, err := s.cfg.Organizer.Retire(ctx, o.ID, "洗版：换成了 "+t.Title); err != nil {
				s.cfg.Logger.Warnf("[订阅] 洗版时删除旧版本失败 (%s): %v", o.TargetPath, err)
			}
		}
	}
	prev, err := s.torrents(ctx, sub.ID, models.MediaSubTorrentDone)
	if err != nil {
		return
	}
	for _, p := range prev {
		if p.ID == t.ID || p.Score >= t.Score || (sub.MediaType == models.MediaKindTV && !sp.Whole && !covered(p.Episode)) {
			continue
		}
		msg := "洗版：换成了 " + t.Title
		if set.UpgradeOld == models.MediaUpgradeDelete {
			msg += "；" + s.deleteOld(ctx, &p)
		}
		s.setTorrent(ctx, p.ID, models.MediaSubTorrentReplaced, msg)
	}
}

// deleteOld 按设置删掉洗版换下来的旧种子（连数据）：有 H&R 要求的不删，留着做种。返回结果说明。
func (s *Service) deleteOld(ctx context.Context, p *models.MediaSubscriptionTorrent) string {
	var info models.TorrentInfo
	if err := s.cfg.DB.WithContext(ctx).Where("site_name = ? AND torrent_id = ?", p.SiteName, p.TorrentID).Limit(1).Find(&info).Error; err == nil && info.HasHR {
		return "旧种子有 H&R 要求，留着做种"
	}
	if s.cfg.Downloaders == nil || p.InfoHash == "" {
		return "旧种子没有删（找不到下载器）"
	}
	dl, _, err := s.cfg.Downloaders.Get(ctx, p.DownloaderID)
	if err != nil {
		return "旧种子没有删（下载器连不上）"
	}
	if err := dl.RemoveTorrent(p.InfoHash, true); err != nil && !errors.Is(err, downloader.ErrTorrentNotFound) {
		return fmt.Sprintf("删除旧种子失败：%v", err)
	}
	return "旧种子连数据删掉了"
}

// checkDone 看订阅是不是完成了。
func (s *Service) checkDone(ctx context.Context, sub *models.MediaSubscription, set Settings) {
	done := false
	if sub.MediaType == models.MediaKindMovie {
		in, err := s.movieInLibrary(ctx, sub)
		if err != nil {
			return
		}
		if !in && s.cfg.Library != nil && !sub.Upgrade {
			in, _ = s.cfg.Library.InLibrary(ctx, sub.MediaType, sub.TMDBID, sub.IMDbID, sub.Title)
		}
		done = in
	} else {
		p, err := s.progress(ctx, sub, false)
		if err != nil {
			return
		}
		done = p.Total > 0 && p.Aired == p.Total && p.InLibrary == p.Total
	}
	if !done {
		return
	}
	msg := "已经入库"
	if sub.MediaType == models.MediaKindTV {
		msg = "这一季都入库了"
	}
	if sub.Upgrade {
		// 洗版要等达到目标的那个版本入了库
		best, err := s.torrents(ctx, sub.ID, models.MediaSubTorrentDone)
		if err != nil {
			return
		}
		profile := s.profileFor(ctx, sub, set)
		if !slices.ContainsFunc(best, func(t models.MediaSubscriptionTorrent) bool {
			return profile.TargetReached(s.parse(ctx, t.Title, ""))
		}) {
			return
		}
		msg += "，达到了洗版的目标"
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("id = ? AND status = ?", sub.ID, models.MediaSubActive).
		Updates(map[string]any{"status": models.MediaSubDone, "message": msg, "updated_at": s.cfg.Now()}).Error; err != nil {
		s.cfg.Logger.Warnf("[订阅] 更新订阅状态失败: %v", err)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}
