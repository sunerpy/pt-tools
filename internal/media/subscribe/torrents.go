package subscribe

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/models"
)

// 追剧详情里下载过的种子：按 info hash 关联整理记录，写明每个文件整理、刮削得怎样；整理没成的能「重新整理」，
// 整理好的能「重新刮削」（整季包和单集一样，一个种子一个按钮）。

// TorrentView 是订阅下载过的一个种子，带上它整理进媒体库的情况。
type TorrentView struct {
	models.MediaSubscriptionTorrent
	// Files 是这个种子的整理记录（每个视频文件一条；还没整理过时为空）
	Files []TorrentFile `json:"files"`
	// CanOrganize 是能「重新整理」：有整理没成（失败、跳过）且不在自动重试的文件，或者种子标成了失败、还没有整理记录
	CanOrganize bool `json:"can_organize"`
	// CanRescrape 是能「重新刮削」：有整理好的文件
	CanRescrape bool `json:"can_rescrape"`
}

// TorrentFile 是种子里一个视频文件的整理记录。
type TorrentFile struct {
	ID           uint       `json:"id"`
	Name         string     `json:"name"`
	Episode      int        `json:"episode,omitempty"`
	EpisodeEnd   int        `json:"episode_end,omitempty"`
	Status       string     `json:"status"`
	Message      string     `json:"message,omitempty"`
	ScrapeStatus string     `json:"scrape_status,omitempty"`
	ScrapeNote   string     `json:"scrape_note,omitempty"`
	NextRetryAt  *time.Time `json:"next_retry_at,omitempty"`
	// Retrying 是整理队列里正在重试它
	Retrying bool `json:"retrying,omitempty"`
}

// torrentViews 给订阅的种子带上整理记录（一次查完这些种子的记录）。
func (s *Service) torrentViews(ctx context.Context, torrents []models.MediaSubscriptionTorrent) ([]TorrentView, error) {
	hashes := make([]string, 0, len(torrents))
	for _, t := range torrents {
		if t.InfoHash != "" {
			hashes = append(hashes, strings.ToLower(t.InfoHash))
		}
	}
	byHash := map[string][]models.MediaTransferHistory{}
	if len(hashes) > 0 {
		var hist []models.MediaTransferHistory
		if err := s.cfg.DB.WithContext(ctx).Where("info_hash IN ?", hashes).Order("id").Find(&hist).Error; err != nil {
			return nil, fmt.Errorf("读取整理记录失败: %w", err)
		}
		for _, h := range hist {
			byHash[strings.ToLower(h.InfoHash)] = append(byHash[strings.ToLower(h.InfoHash)], h)
		}
	}
	out := make([]TorrentView, 0, len(torrents))
	for _, t := range torrents {
		v := TorrentView{MediaSubscriptionTorrent: t, Files: []TorrentFile{}}
		for _, h := range byHash[strings.ToLower(t.InfoHash)] {
			f := TorrentFile{
				ID: h.ID, Name: path.Base(strings.ReplaceAll(h.SourcePath, `\`, "/")), Episode: h.Episode, EpisodeEnd: h.EpisodeEnd,
				Status: h.Status, Message: h.Message, ScrapeStatus: h.ScrapeStatus, ScrapeNote: h.ScrapeNote, NextRetryAt: h.NextRetryAt,
			}
			if h.Status == models.MediaTransferFailed && s.cfg.Organizer != nil {
				f.Retrying = s.cfg.Organizer.Retrying(h.ID)
			}
			switch {
			case h.Status == models.MediaTransferDone:
				v.CanRescrape = true
			case (h.Status == models.MediaTransferFailed || h.Status == models.MediaTransferSkipped) && !f.Retrying:
				v.CanOrganize = true
			}
			v.Files = append(v.Files, f)
		}
		if len(v.Files) == 0 && t.Status == models.MediaSubTorrentFailed {
			v.CanOrganize = true
		}
		// 洗版换掉的旧版本是特意从库里删掉的，不再整理、刮削
		if t.InfoHash == "" || s.cfg.Organizer == nil || t.Status == models.MediaSubTorrentReplaced {
			v.CanOrganize, v.CanRescrape = false, false
		}
		out = append(out, v)
	}
	return out, nil
}

// errReplacedTorrent 是洗版换掉的旧版本：它的文件是特意从库里删掉的，不再整理、刮削（详情里的 CanOrganize、CanRescrape 也是假）。
// 动作本身也要拒绝：App API、MCP（带 confirm=true）直接调动作，不经过详情；旧版本还在下载器里做种时，重新整理会把它放回库里。
var errReplacedTorrent = fmt.Errorf("%w: 这个版本已经被洗版换掉（库里的文件是特意删掉的），不再整理、刮削", ErrInvalid)

// subTorrent 是订阅下载过的一个种子（不是这个订阅的回 ErrNotFound）。
func (s *Service) subTorrent(ctx context.Context, subID, torrentID uint) (models.MediaSubscription, models.MediaSubscriptionTorrent, error) {
	var t models.MediaSubscriptionTorrent
	sub, err := s.subRow(ctx, subID)
	if err != nil {
		return sub, t, err
	}
	if err := s.cfg.DB.WithContext(ctx).Where("id = ? AND subscription_id = ?", torrentID, subID).Limit(1).Find(&t).Error; err != nil {
		return sub, t, fmt.Errorf("读取订阅的种子失败: %w", err)
	}
	if t.ID == 0 {
		return sub, t, ErrNotFound
	}
	return sub, t, nil
}

// OrganizeTorrent 重新整理订阅下载的一个种子（整季包与单集一样）：按订阅的条目与媒体库整理（不靠识别，
// 订阅知道它下载的是哪部剧哪一季），已经整理好的文件不动。整理完马上更新种子的状态，运行记录里记一条。
func (s *Service) OrganizeTorrent(ctx context.Context, subID, torrentID uint) (*organize.Result, error) {
	if s.cfg.Organizer == nil {
		return nil, fmt.Errorf("%w: 整理入库没有启动", ErrInvalid)
	}
	sub, t, err := s.subTorrent(ctx, subID, torrentID)
	if err != nil {
		return nil, err
	}
	if t.Status == models.MediaSubTorrentReplaced {
		return nil, errReplacedTorrent
	}
	if t.InfoHash == "" || t.DownloaderID == 0 {
		return nil, fmt.Errorf("%w: 这个种子没有记下下载器或 info hash，到下载器里手动整理", ErrInvalid)
	}
	res, err := s.cfg.Organizer.Organize(ctx, organize.Request{
		DownloaderID: t.DownloaderID, Hash: t.InfoHash, MediaType: sub.MediaType, TMDBID: sub.TMDBID, LibraryID: sub.LibraryID,
	})
	if err != nil {
		s.manualRun(ctx, &sub, fmt.Sprintf("重新整理「%s」没有成功：%v", t.Title, err))
		return nil, err
	}
	// 标成失败的种子改回等整理（7 天从现在重新算），整理在后台排队时之后每一拍照样看它；整理成了的马上标成已入库
	set, _ := s.Settings(ctx)
	if t.Status == models.MediaSubTorrentFailed {
		s.setTorrent(ctx, t.ID, models.MediaSubTorrentDownloading, "重新整理了，等整理入库")
		if _, t, err = s.subTorrent(ctx, subID, torrentID); err != nil {
			return res, nil
		}
	}
	s.refreshTorrent(ctx, &t, set)
	s.afterOrganize(ctx, &sub, set)
	s.manualRun(ctx, &sub, fmt.Sprintf("重新整理「%s」：%s", t.Title, organizeText(res)))
	return res, nil
}

// afterOrganize 在重新整理以后马上重建追剧台账、看订阅是不是完成了（和后台刷新一样）：
// 每一集的情况跟着变，不用等下一次后台刷新。
func (s *Service) afterOrganize(ctx context.Context, sub *models.MediaSubscription, set Settings) {
	var lg *ledger
	if sub.MediaType == models.MediaKindTV {
		var err error
		s.mu.Lock()
		lg, err = s.rebuildLedger(ctx, sub, rebuildOpts{set: set})
		s.mu.Unlock()
		if err != nil {
			s.cfg.Logger.Debugf("[订阅] 重新整理以后重建「%s」的追剧台账失败: %v", sub.Title, err)
			return
		}
	}
	if sub.Status == models.MediaSubActive {
		s.checkDone(ctx, sub, set, lg)
	}
}

// RescrapeTorrent 重新刮削订阅下载的一个种子整理进的电影或剧集（同一部在这个媒体库里的都刮削）。
func (s *Service) RescrapeTorrent(ctx context.Context, subID, torrentID uint) (*organize.RescrapeResult, error) {
	if s.cfg.Organizer == nil {
		return nil, fmt.Errorf("%w: 整理入库没有启动", ErrInvalid)
	}
	sub, t, err := s.subTorrent(ctx, subID, torrentID)
	if err != nil {
		return nil, err
	}
	if t.Status == models.MediaSubTorrentReplaced {
		return nil, errReplacedTorrent
	}
	hist, err := s.historyOf(ctx, t.InfoHash)
	if err != nil {
		return nil, err
	}
	var id uint
	for _, h := range hist {
		if h.Status == models.MediaTransferDone {
			id = h.ID
			break
		}
	}
	if id == 0 {
		return nil, fmt.Errorf("%w: 这个种子还没有整理好的文件，先重新整理", ErrInvalid)
	}
	res, err := s.cfg.Organizer.Rescrape(ctx, id)
	if err != nil {
		return nil, err
	}
	msg := fmt.Sprintf("重新刮削「%s」：刮削了 %d 条整理记录、写了 %d 个文件", t.Title, res.Records, res.Written)
	if res.Queued {
		msg = fmt.Sprintf("重新刮削「%s」：排上队了，稍后在整理历史里看结果", t.Title)
	}
	s.manualRun(ctx, &sub, msg)
	return res, nil
}

// organizeText 是一次整理的结果。
func organizeText(r *organize.Result) string {
	if r == nil {
		return "没有结果"
	}
	if r.Queued {
		return "排上队了，稍后在整理历史里看结果"
	}
	// Created 是这次新放进库的，Done 是库里本来就是这个文件的
	parts := []string{fmt.Sprintf("新入库 %d 个文件", r.Created)}
	if r.Done > 0 {
		parts = append(parts, fmt.Sprintf("%d 个已在库里", r.Done))
	}
	if r.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d 个跳过", r.Skipped))
	}
	if r.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%d 个失败", r.Failed))
	}
	switch {
	case len(r.Messages) > 0:
		parts = append(parts, r.Messages[0])
	case r.Plan != nil && r.Plan.Problem != "" && r.Created == 0:
		parts = append(parts, r.Plan.Problem)
	}
	return strings.Join(parts, "，")
}

// UnknownSites 是给的站点里没有注册的那些（没配置、没启用或不认识的站点名）：App API 与 MCP 改订阅的站点时先查一下。
// 按站点查找函数判断，没配置时按站点名单；两个都没配置时不查。
func (s *Service) UnknownSites(names []string) []string {
	var known func(string) bool
	switch {
	case s.cfg.Sites != nil:
		known = func(n string) bool { _, ok := s.cfg.Sites(n); return ok }
	case s.cfg.SiteNames != nil:
		set := map[string]bool{}
		for _, n := range s.cfg.SiteNames() {
			set[n] = true
		}
		known = func(n string) bool { return set[n] }
	default:
		return nil
	}
	var out []string
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" && !known(n) {
			out = append(out, n)
		}
	}
	return out
}

// UnknownDownloader 是给的下载器编号不存在（0 是用设置里的，不算）：App API 与 MCP 改订阅的下载器时先查一下。
func (s *Service) UnknownDownloader(ctx context.Context, id uint) (bool, error) {
	if id == 0 {
		return false, nil
	}
	var n int64
	if err := s.cfg.DB.WithContext(ctx).Model(&models.DownloaderSetting{}).Where("id = ?", id).Count(&n).Error; err != nil {
		return false, fmt.Errorf("读取下载器失败: %w", err)
	}
	return n == 0, nil
}
