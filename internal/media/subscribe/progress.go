package subscribe

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
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

func (s *Service) historyOf(ctx context.Context, hash string) ([]models.MediaTransferHistory, error) {
	var hist []models.MediaTransferHistory
	if hash == "" {
		return nil, nil
	}
	if err := s.cfg.DB.WithContext(ctx).Where("info_hash = ?", strings.ToLower(hash)).Order("id").Find(&hist).Error; err != nil {
		return nil, fmt.Errorf("读取整理记录失败: %w", err)
	}
	return hist, nil
}

// refreshTorrent 看订阅下载的一个种子整理得怎样：有文件入了库、别的文件也不再自动重试时标成已入库
// （洗版时接着换掉旧版本）；过了 7 天还没入库的标成失败。还要重试的：记录里有重试时间，或者整理服务说它排上队了。
func (s *Service) refreshTorrent(ctx context.Context, t *models.MediaSubscriptionTorrent, set Settings) {
	hist, err := s.historyOf(ctx, t.InfoHash)
	if err != nil {
		return
	}
	sub, err := s.subRow(ctx, t.SubscriptionID)
	if err != nil {
		return
	}
	// 洗版的新版本和旧版本整理到同一个文件名时，整理记成「目标已有同名文件」：先删掉旧版本库里的那个文件再重试
	if sub.Upgrade && s.cfg.Organizer != nil {
		prof := s.profileFor(ctx, &sub, set)
		score := s.versionPoints(ctx, prof, t.Title, t.Subtitle)
		retried := false
		for i := range hist {
			h := &hist[i]
			if h.Status == models.MediaTransferSkipped && s.freeTarget(ctx, &sub, h, t, prof, score) {
				if _, rerr := s.cfg.Organizer.Retry(ctx, h.ID); rerr != nil {
					s.cfg.Logger.Warnf("[订阅] 洗版后重新整理失败 (%s): %v", h.TorrentName, rerr)
				}
				retried = true
			}
		}
		if retried {
			if hist, err = s.historyOf(ctx, t.InfoHash); err != nil {
				return
			}
		}
	}
	done, notDone, retrying := 0, 0, false
	for _, h := range hist {
		switch {
		case h.Status == models.MediaTransferDone:
			done++
		case h.Status == models.MediaTransferFailed && (h.NextRetryAt != nil || (s.cfg.Organizer != nil && s.cfg.Organizer.Retrying(h.ID))):
			retrying = true
		case h.Status == models.MediaTransferFailed || h.Status == models.MediaTransferSkipped:
			notDone++
		}
	}
	if done > 0 && !retrying {
		msg := "已整理入库"
		if notDone > 0 {
			msg = fmt.Sprintf("已整理入库，%d 个文件没整理成", notDone)
		}
		s.setTorrent(ctx, t.ID, models.MediaSubTorrentDone, msg)
		if sub.Upgrade {
			s.replaceOlder(ctx, &sub, t, hist, set)
		}
		return
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
	if err := db.Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取整理记录失败: %w", err)
	}
	return rows, nil
}

// version 是这个条目现在库里的一个文件，带上打分用的标题：订阅下载的用站点上的标题与副标题，
// 别的用种子名（没有时用源文件名）。
type version struct {
	row             models.MediaTransferHistory
	title, subtitle string
}

// libraryVersions 是库里这个条目（剧集是这一季）已整理的文件，不管是不是订阅下载的。
func (s *Service) libraryVersions(ctx context.Context, sub *models.MediaSubscription) ([]version, error) {
	rows, err := s.sameMedia(ctx, sub)
	if err != nil {
		return nil, err
	}
	linked, err := s.torrents(ctx, sub.ID)
	if err != nil {
		return nil, err
	}
	byHash := map[string]models.MediaSubscriptionTorrent{}
	for _, t := range linked {
		if t.InfoHash != "" {
			byHash[strings.ToLower(t.InfoHash)] = t
		}
	}
	out := make([]version, 0, len(rows))
	for _, r := range rows {
		v := version{row: r, title: r.TorrentName}
		if t, ok := byHash[strings.ToLower(r.InfoHash)]; ok {
			v.title, v.subtitle = t.Title, t.Subtitle
		} else if v.title == "" {
			v.title = filepath.Base(r.SourcePath)
		}
		out = append(out, v)
	}
	return out, nil
}

// versionPoints 按现在的档案重算一个已有版本的分数（档案改过以后，推送时记下的分数没法比）。不看硬条件。
func (s *Service) versionPoints(ctx context.Context, p Profile, title, subtitle string) int {
	return p.points(s.parse(ctx, title, subtitle))
}

// baseline 是现在的版本按当前档案的最高分：下载中与已入库的订阅种子，加上库里这个条目已整理的文件。
// 失败、已替换的不算。第二个返回值报告有没有现在的版本。
func (s *Service) baseline(ctx context.Context, p Profile, linked []models.MediaSubscriptionTorrent, vers []version) (int, bool) {
	best, has := 0, false
	for _, t := range linked {
		if t.Status == models.MediaSubTorrentDownloading || t.Status == models.MediaSubTorrentDone {
			best, has = max(best, s.versionPoints(ctx, p, t.Title, t.Subtitle)), true
		}
	}
	for _, v := range vers {
		best, has = max(best, s.versionPoints(ctx, p, v.title, v.subtitle)), true
	}
	return best, has
}

// freeTarget 在洗版的新版本整理时撞上旧版本的同名文件时，删掉旧版本库里的那个文件再重新整理；删了返回真。
// 先删后整理，所以只删分数比新版本低的：同分的留着（同一个文件名，库里本来就只有一份）。
func (s *Service) freeTarget(ctx context.Context, sub *models.MediaSubscription, skipped *models.MediaTransferHistory, t *models.MediaSubscriptionTorrent, prof Profile, score int) bool {
	if skipped.TargetPath == "" {
		return false
	}
	vers, err := s.libraryVersions(ctx, sub)
	if err != nil {
		return false
	}
	freed := false
	for _, v := range vers {
		o := v.row
		if o.TargetPath != skipped.TargetPath || strings.EqualFold(o.InfoHash, t.InfoHash) || s.versionPoints(ctx, prof, v.title, v.subtitle) >= score {
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

// replaceOlder 在洗版的新版本入库以后换掉旧版本：库里被新版本盖住、按现在的档案分数不比新版本高的文件
// （同分的也换，库里不留两份），按整理历史的删除规则删掉（不管是不是订阅下载的）；比新版本好的留着。
// 订阅下载的旧种子在库里的文件都换掉以后标成已替换，
// 按设置继续做种或删除。剧集只算新种子真正整理进库的集：整季包有几集没整理成，那几集的旧版本留着；
// 旧文件是几集的合集时，这几集新版本都有才换。
func (s *Service) replaceOlder(ctx context.Context, sub *models.MediaSubscription, t *models.MediaSubscriptionTorrent, hist []models.MediaTransferHistory, set Settings) {
	prof := s.profileFor(ctx, sub, set)
	score := s.versionPoints(ctx, prof, t.Title, t.Subtitle)
	got := map[int]bool{}
	for _, h := range hist {
		if h.Status == models.MediaTransferDone {
			for n := h.Episode; n > 0 && n <= max(h.EpisodeEnd, h.Episode); n++ {
				got[n] = true
			}
		}
	}
	covered := func(start, end int) bool {
		if sub.MediaType == models.MediaKindMovie {
			return true
		}
		if start <= 0 {
			return false
		}
		for n := start; n <= max(end, start); n++ {
			if !got[n] {
				return false
			}
		}
		return true
	}
	if s.cfg.Organizer != nil {
		vers, err := s.libraryVersions(ctx, sub)
		if err != nil {
			s.cfg.Logger.Warnf("[订阅] 洗版时读取库里的版本失败: %v", err)
			return
		}
		for _, v := range vers {
			o := v.row
			if strings.EqualFold(o.InfoHash, t.InfoHash) || !covered(o.Episode, o.EpisodeEnd) || s.versionPoints(ctx, prof, v.title, v.subtitle) > score {
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
		if p.ID == t.ID || s.versionPoints(ctx, prof, p.Title, p.Subtitle) > score {
			continue
		}
		// 旧种子还有文件在库里（新版本没有那几集，或者删除没成功）时不算换下来，种子也不动
		var left int64
		if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).
			Where("info_hash = ? AND status = ?", strings.ToLower(p.InfoHash), models.MediaTransferDone).Count(&left).Error; err != nil || left > 0 {
			continue
		}
		msg := "洗版：换成了 " + t.Title
		if set.UpgradeOld == models.MediaUpgradeDelete {
			msg += "；" + s.deleteOld(ctx, &p)
		}
		s.setTorrent(ctx, p.ID, models.MediaSubTorrentReplaced, msg)
	}
}

// deleteOld 按设置删掉洗版换下来的旧种子（连数据）。拿不准时不删：下载器里原来就有的（不是订阅加进去的）、
// 推送时知道有 H&R 要求的、种子记录里写着有 H&R 的、读不到或找不到种子记录的都留着。返回结果说明。
func (s *Service) deleteOld(ctx context.Context, p *models.MediaSubscriptionTorrent) string {
	if p.Adopted {
		return "旧种子不是订阅加进下载器的，留着"
	}
	if p.HasHR {
		return "旧种子有 H&R 要求，留着做种"
	}
	var info models.TorrentInfo
	if err := s.cfg.DB.WithContext(ctx).Where("site_name = ? AND torrent_id = ?", p.SiteName, p.TorrentID).Limit(1).Find(&info).Error; err != nil {
		return "旧种子没有删（读不到种子记录）"
	}
	if info.ID == 0 {
		return "旧种子没有删（找不到种子记录，不知道有没有 H&R 要求）"
	}
	if info.HasHR {
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

// episodeFloor 是这一季每一集现在的版本里最差的那一集的分数（一集有几个版本时按最好的算）：
// 整季包要比它高，也就是至少有一集会变好；比新版本好的集入库以后也留着，不会变差。
func (s *Service) episodeFloor(ctx context.Context, p Profile, vers []version, total int) int {
	best := map[int]int{}
	for _, v := range vers {
		pts := s.versionPoints(ctx, p, v.title, v.subtitle)
		for n := v.row.Episode; n > 0 && n <= max(v.row.EpisodeEnd, v.row.Episode); n++ {
			best[n] = max(best[n], pts)
		}
	}
	floor := -1
	for n := 1; n <= total; n++ {
		if floor < 0 || best[n] < floor {
			floor = best[n]
		}
	}
	return max(floor, 0)
}

// targetReached 报告库里的版本是不是达到了洗版的目标：电影是有一个文件达到，剧集是这一季的每一集都有达到的文件。
func (s *Service) targetReached(ctx context.Context, sub *models.MediaSubscription, prof Profile, total int) (bool, error) {
	vers, err := s.libraryVersions(ctx, sub)
	if err != nil {
		return false, err
	}
	ok := map[int]bool{}
	for _, v := range vers {
		if !prof.TargetReached(s.parse(ctx, v.title, v.subtitle)) {
			continue
		}
		if sub.MediaType == models.MediaKindMovie {
			return true, nil
		}
		for n := v.row.Episode; n > 0 && n <= max(v.row.EpisodeEnd, v.row.Episode); n++ {
			ok[n] = true
		}
	}
	if sub.MediaType == models.MediaKindMovie || total <= 0 {
		return false, nil
	}
	for n := 1; n <= total; n++ {
		if !ok[n] {
			return false, nil
		}
	}
	return true, nil
}

// checkDone 看订阅是不是完成了。
func (s *Service) checkDone(ctx context.Context, sub *models.MediaSubscription, set Settings) {
	done, total := false, 0
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
		done, total = p.Total > 0 && p.Aired == p.Total && p.InLibrary == p.Total, p.Total
	}
	if !done {
		return
	}
	msg := "已经入库"
	if sub.MediaType == models.MediaKindTV {
		msg = "这一季都入库了"
	}
	if sub.Upgrade {
		// 洗版要等库里的版本达到目标
		reached, err := s.targetReached(ctx, sub, s.profileFor(ctx, sub, set), total)
		if err != nil || !reached {
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
