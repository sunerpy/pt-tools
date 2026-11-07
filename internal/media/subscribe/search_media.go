package subscribe

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/media/serverindex"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 按条目搜索：在各站点上搜一个条目（剧集是其中一季）的种子，用订阅的办法对条目、按质量档案打分，把同一资源在各站点上的
// 种子合成一组，再给出这一季每集现在的情况，供挑一个推送。只读：不推送、不写运行记录、不重建追剧台账。

// mediaSearchTimeout 是按条目搜索的总时限（几个关键字一起搜）。
const mediaSearchTimeout = 60 * time.Second

// 不按档案打分时 profile_id 传它。
const noProfile = -1

// MediaSearchInput 是按条目搜索的请求。
type MediaSearchInput struct {
	MediaType string `json:"media_type"`
	TMDBID    int    `json:"tmdb_id"`
	// Season 是剧集的季，0 是最新的一季；电影不用
	Season int `json:"season"`
	// ProfileID 只影响这一次搜索，不改订阅：大于 0 用这个档案；0 用这一季订阅的档案，没有订阅或订阅没设档案时用设置里的
	// 默认档案（都没有时按分辨率与来源从好到差比）；-1 不按档案
	ProfileID int `json:"profile_id"`
	// Sites 只搜这些站点；为空时搜全部
	Sites []string `json:"sites"`
	// IncludeUnmatched 为真时把对不上这个条目的结果也返回，写明原因
	IncludeUnmatched bool `json:"include_unmatched"`
	// Draft 是还没保存的表单（试搜索）：给了它时质量档案、标题包含与排除按它来，不按库里的订阅；不能和 ProfileID 同时给
	Draft *MediaSearchDraft `json:"draft,omitempty"`
	// Deadline 只管站点搜索这一段：不为零时站点搜索最晚到这个时间（比 mediaSearchTimeout 早时），到点没回来的站点写进 Errors
	// 并标 TimedOut，已经回来的照常出结果。只由调用方的代码设（App 的试搜索），请求体里设不了
	Deadline time.Time `json:"-"`
}

// MediaSearchDraft 是试搜索时还没保存的表单。ProfileID 为 0 时用设置里的默认档案（和保存这张表单以后一样）；
// IncludeKeywords、ExcludeKeywords 是表单里的原样文字（写法见 models.MediaSubscription）。
// 别名、集号规则、剧集组与手填的集数照旧用库里这一季的订阅，没有订阅时按 TMDB。
type MediaSearchDraft struct {
	ProfileID       uint   `json:"profile_id"`
	IncludeKeywords string `json:"include_keywords"`
	ExcludeKeywords string `json:"exclude_keywords"`
}

// MediaSearchResult 是按条目搜索的结果。
type MediaSearchResult struct {
	Item MediaSearchItem `json:"item"`
	// Subscription 是这一季（电影就是这部）的订阅，没有时为空
	Subscription *MediaSearchSubscription `json:"subscription"`
	// Profile 是这次打分用的档案；不按档案时为空，没有档案（按分辨率与来源比）时编号为 0
	Profile  *MediaSearchProfile `json:"profile"`
	Keywords []string            `json:"keywords"`
	// Episodes 是这一季每集现在的情况（剧集）；Movie 是电影现在的情况
	Episodes []MediaSearchEpisode `json:"episodes"`
	Movie    *MediaSearchMovie    `json:"movie,omitempty"`
	// Groups 是对上这个条目的结果，同一资源一组，排好了序
	Groups []MediaSearchGroup `json:"groups"`
	// Total 是对上这个条目的结果数（各组成员数之和）
	Total int `json:"total"`
	// BlockedCount 是对上了、但拉黑了的结果数（成员数）
	BlockedCount int `json:"blocked_count"`
	// UnmatchedCount 是对不上这个条目的结果数；Unmatched 只在请求 include_unmatched 时带上
	UnmatchedCount int                 `json:"unmatched_count"`
	Unmatched      []MediaSearchMember `json:"unmatched,omitempty"`
	Errors         []MediaSearchError  `json:"errors"`
}

// MediaSearchItem 是搜的条目。
type MediaSearchItem struct {
	MediaType     string `json:"media_type"`
	TMDBID        int    `json:"tmdb_id"`
	Season        int    `json:"season,omitempty"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title,omitempty"`
	// Year 是剧集这一季的年份
	Year       int    `json:"year,omitempty"`
	PosterPath string `json:"poster_path,omitempty"`
	// Seasons 是剧集一共几季；TotalEpisodes 是这一季几集
	Seasons       int `json:"seasons,omitempty"`
	TotalEpisodes int `json:"total_episodes,omitempty"`
}

// MediaSearchSubscription 是条目的订阅。
type MediaSearchSubscription struct {
	ID     uint   `json:"id"`
	Status string `json:"status"`
	// Lock 是锁住的版本，没开版本锁时为空
	Lock *MediaSearchLock `json:"lock"`
}

// MediaSearchLock 是锁住的版本：制作组与分辨率，认不出的那一项为空。
type MediaSearchLock struct {
	Group      string `json:"group"`
	Resolution string `json:"resolution"`
}

// MediaSearchProfile 是打分用的档案。
type MediaSearchProfile struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// MediaSearchEpisode 是一集现在的情况：library 已入库、downloading 在下载（Site 是来源站点，下载器里认领的种子为空）、
// waiting_version 等锁住的版本、missing 缺、upcoming 还没播。
type MediaSearchEpisode struct {
	Episode int    `json:"episode"`
	State   string `json:"state"`
	Site    string `json:"site,omitempty"`
	AirDate string `json:"air_date,omitempty"`
	Name    string `json:"name,omitempty"`
}

// MediaSearchMovie 是电影现在的情况：library、downloading 或 missing。
type MediaSearchMovie struct {
	State string `json:"state"`
	Site  string `json:"site,omitempty"`
}

// MediaSearchVerdict 是按档案的结果：Reject 不为空时不符合（全部原因），Score 是 0–100 的分。
type MediaSearchVerdict struct {
	Score  int    `json:"score"`
	Reject string `json:"reject,omitempty"`
}

// MediaSearchMember 是一个站点上的一个种子。
type MediaSearchMember struct {
	Site       string `json:"site"`
	TorrentID  string `json:"torrent_id"`
	Title      string `json:"title"`
	Subtitle   string `json:"subtitle,omitempty"`
	InfoHash   string `json:"info_hash,omitempty"`
	SizeBytes  int64  `json:"size_bytes"`
	UploadedAt int64  `json:"uploaded_at,omitempty"`
	Seeders    int    `json:"seeders"`
	Leechers   int    `json:"leechers"`
	Snatched   int    `json:"snatched,omitempty"`
	// Discount 是优惠（站点的 DiscountLevel 原样）；Free 是现在免费，FreeLeftSeconds 是免费还剩几秒（不知道结束时间时为 0）
	Discount        string `json:"discount"`
	Free            bool   `json:"free"`
	FreeLeftSeconds int64  `json:"free_left_seconds,omitempty"`
	// HR 是有 H&R 要求（种子自己有，或者站点整站有）；HRHours 是要做种多少小时，不知道时为 0
	HR          bool   `json:"hr"`
	HRHours     int    `json:"hr_hours,omitempty"`
	URL         string `json:"url,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
	Category    string `json:"category,omitempty"`
	// ByIMDb 是按 IMDb 编号对上的（不是按名字）
	ByIMDb  bool                `json:"by_imdb,omitempty"`
	Meta    meta.Meta           `json:"meta"`
	Verdict *MediaSearchVerdict `json:"verdict"`
	// Reason 是对不上这个条目的原因（只在 unmatched 里）
	Reason string `json:"reason,omitempty"`
	// Block 是这个种子拉黑了（对这个条目，或对所有条目）；没拉黑时为空
	Block *MediaSearchBlock `json:"block,omitempty"`
}

// MediaSearchBlock 是拉黑的记录：ID 用来移出黑名单，All 为真时是对所有条目拉黑的。
type MediaSearchBlock struct {
	ID     uint   `json:"id"`
	Reason string `json:"reason,omitempty"`
	All    bool   `json:"all,omitempty"`
}

// MediaSearchGroup 是同一个资源：Members 的第一个是首选。
type MediaSearchGroup struct {
	GroupID string              `json:"group_id"`
	Members []MediaSearchMember `json:"members"`
	// Meta 与 Verdict 是首选的
	Meta    meta.Meta           `json:"meta"`
	Verdict *MediaSearchVerdict `json:"verdict"`
	// Span 是剧集里有的集（电影为空）
	Span *MediaSearchSpan `json:"span,omitempty"`
	// Covers 是能补上的集（缺的与等版本的）；Downloading 是这一组里正在下载的集；InLibrary 是这一组的集都已入库（电影是库里有）
	Covers      []int `json:"covers"`
	Downloading []int `json:"downloading"`
	InLibrary   bool  `json:"in_library"`
	// Blocked 是这一组的成员都拉黑了（排在最后）
	Blocked bool `json:"blocked,omitempty"`
}

// MediaSearchSpan 是种子里的集：Whole 为真时是整季，否则是 [Start, End]。
type MediaSearchSpan struct {
	Start int  `json:"start,omitempty"`
	End   int  `json:"end,omitempty"`
	Whole bool `json:"whole,omitempty"`
}

// MediaSearchError 是一个站点（Site 为空时是一次搜索）的错误。TimedOut 为真时是到了时限还没有回应。
type MediaSearchError struct {
	Site     string `json:"site"`
	Error    string `json:"error"`
	TimedOut bool   `json:"timed_out,omitempty"`
}

// searchHit 是一个对上了条目的结果。
type searchHit struct {
	c       Candidate
	sp      span
	v       Verdict
	scored  bool
	reasons []string
	member  MediaSearchMember
	blocked bool
}

func (in *MediaSearchInput) check() error {
	if in.MediaType != models.MediaKindMovie && in.MediaType != models.MediaKindTV {
		return fmt.Errorf("%w: 类型要选电影或剧集", ErrInvalid)
	}
	if in.TMDBID <= 0 {
		return fmt.Errorf("%w: 要指定 TMDB 编号", ErrInvalid)
	}
	if in.MediaType == models.MediaKindMovie {
		in.Season = 0
	} else if in.Season < 0 || in.Season > 200 {
		return fmt.Errorf("%w: 季号不对", ErrInvalid)
	}
	if in.ProfileID < noProfile {
		return fmt.Errorf("%w: 质量档案不对", ErrInvalid)
	}
	if d := in.Draft; d != nil {
		if in.ProfileID != 0 {
			return fmt.Errorf("%w: draft 与 profile_id 不能同时给", ErrInvalid)
		}
		for _, v := range []struct{ name, v string }{{"标题包含", d.IncludeKeywords}, {"标题排除", d.ExcludeKeywords}} {
			if len(strings.TrimSpace(v.v)) > maxKeywordsLen {
				return fmt.Errorf("%w: %s的关键词太长", ErrInvalid, v.name)
			}
		}
	}
	sites, err := cleanSites(in.Sites)
	if err != nil {
		return err
	}
	// 站点排好序：搜索缓存的键是整个查询，调用方给的顺序（比如按启用的站点表取的）不同也要命中同一条缓存
	slices.Sort(sites)
	in.Sites = sites
	return nil
}

// Validate 检查请求（和 SearchMedia 开头的检查一样），不搜索：调用方要先过滤站点、可能不搜时用它，参数不对照样报错。
func (in MediaSearchInput) Validate() error {
	return in.check()
}

// SearchMedia 按条目搜索（见 MediaSearchInput）。这一季已经订阅时按订阅本身对（别名、集号规则、标题关键词），没有时按 TMDB 上的条目。
func (s *Service) SearchMedia(ctx context.Context, in MediaSearchInput) (*MediaSearchResult, error) {
	if err := in.check(); err != nil {
		return nil, err
	}
	if s.cfg.Search == nil {
		return nil, fmt.Errorf("搜索服务没有启动")
	}
	set, err := s.Settings(ctx)
	if err != nil {
		return nil, err
	}
	c, err := s.cfg.Recognizer.TMDB(ctx)
	if err != nil {
		return nil, err
	}
	d, err := c.Details(ctx, in.MediaType, in.TMDBID)
	if err != nil {
		return nil, fmt.Errorf("取 TMDB 条目失败: %w", err)
	}
	if in.MediaType == models.MediaKindTV && in.Season == 0 {
		in.Season = max(d.Seasons, 1)
	}
	sub, subscribed, err := s.searchSubject(ctx, in)
	if err != nil {
		return nil, err
	}
	res := &MediaSearchResult{
		Item: MediaSearchItem{
			MediaType: in.MediaType, TMDBID: in.TMDBID, Season: sub.Season, Title: sub.Title, OriginalTitle: sub.OriginalTitle,
			Year: sub.Year, PosterPath: sub.PosterPath,
		},
		Episodes: []MediaSearchEpisode{}, Groups: []MediaSearchGroup{}, Errors: []MediaSearchError{},
	}
	if subscribed {
		res.Subscription = &MediaSearchSubscription{ID: sub.ID, Status: sub.Status}
		if g, r, ok := lockOf(sub); ok {
			res.Subscription.Lock = &MediaSearchLock{Group: g, Resolution: r}
		}
	}
	// judge 是打分与判断包含、排除用的订阅：试搜索时是换上表单三项的副本（不写库）
	judge, profileID := sub, in.ProfileID
	if d := in.Draft; d != nil {
		cp := *sub
		cp.ProfileID = d.ProfileID
		cp.IncludeKeywords, cp.ExcludeKeywords = strings.TrimSpace(d.IncludeKeywords), strings.TrimSpace(d.ExcludeKeywords)
		judge, profileID = &cp, int(d.ProfileID)
	}
	profile, scored, err := s.searchProfile(ctx, judge, set, profileID)
	if err != nil {
		return nil, err
	}
	if scored {
		res.Profile = &MediaSearchProfile{ID: profile.ID, Name: profile.Name}
	}

	mt := newMatcher(*sub)
	var season *tmdb.Season
	total := 0
	if in.MediaType == models.MediaKindTV {
		res.Item.Seasons = d.Seasons
		if season, err = s.seasonFor(ctx, sub, false); err != nil {
			return nil, fmt.Errorf("取 TMDB 季详情失败: %w", err)
		}
		total = len(season.Episodes)
		if sub.TotalEpisodesManual {
			total = sub.TotalEpisodes
		} else {
			total = max(total, sub.TotalEpisodes)
		}
		res.Item.TotalEpisodes = total
		mt = mt.withMapper(s.mapperFor(ctx, sub, season))
	}

	res.Keywords = keywords(sub)
	items, errs, err := s.searchKeywords(ctx, res.Keywords, in.Sites, in.Deadline)
	if err != nil {
		return nil, err
	}
	// 调用方取消了（客户端断开、隧道里的 CANCEL）：不再对条目、读库
	if cerr := ctx.Err(); cerr != nil {
		return nil, cerr
	}
	res.Errors = append(res.Errors, errs...)

	blocks, err := s.blocksFor(ctx, in.MediaType, in.TMDBID, sub.Season)
	if err != nil {
		return nil, err
	}
	now := s.cfg.Now()
	var hits []searchHit
	var unmatched []MediaSearchMember
	for _, it := range items {
		hr, def := siteHR(it.SourceSite)
		cand := Candidate{Site: it.SourceSite, Item: it, Meta: s.parse(ctx, it.Title, it.Subtitle), From: fromSearch, SiteHR: hr, SeenAt: now}
		m := memberOf(cand, def, now)
		sp, ok, why := mt.matchMapped(cand.Meta, it)
		if !ok {
			if why == "" {
				why = "名字或季对不上这个条目"
			}
			m.Reason = why
			unmatched = append(unmatched, m)
			continue
		}
		m.ByIMDb = mt.byIMDb(it)
		h := searchHit{c: cand, sp: sp, scored: scored}
		if scored {
			h.reasons = profile.rejects(cand, sp.count(total), false)
			h.v = Verdict{Score: profile.points(cand.Meta)}
		}
		if why := keywordReject(judge, it); why != "" {
			h.reasons = append(h.reasons, why)
			h.scored = true
		}
		if h.scored {
			if len(h.reasons) > 0 {
				h.v.Reject = strings.Join(h.reasons, "；")
			}
			m.Verdict = &MediaSearchVerdict{Score: profile.percent(h.v.Score), Reject: h.v.Reject}
		}
		if b, hit := blocks.get(it.SourceSite, it.ID); hit {
			h.blocked = true
			m.Block = &MediaSearchBlock{ID: b.ID, Reason: b.Reason, All: b.TMDBID == 0}
			res.BlockedCount++
		}
		h.member = m
		hits = append(hits, h)
	}
	res.Total, res.UnmatchedCount = len(hits), len(unmatched)
	if in.IncludeUnmatched {
		slices.SortFunc(unmatched, func(a, b MediaSearchMember) int {
			return cmp.Or(cmp.Compare(strings.ToLower(a.Site), strings.ToLower(b.Site)), cmp.Compare(a.TorrentID, b.TorrentID))
		})
		res.Unmatched = unmatched
	}

	states := map[int]MediaSearchEpisode{}
	if in.MediaType == models.MediaKindTV {
		if res.Episodes, err = s.searchEpisodes(ctx, sub, subscribed, season, total, mt.mapper); err != nil {
			return nil, err
		}
		for _, e := range res.Episodes {
			states[e.Episode] = e
		}
	} else if res.Movie, err = s.searchMovie(ctx, sub, subscribed); err != nil {
		return nil, err
	}
	res.Groups = groupHits(hits, profile, total, states, res.Movie)
	return res, nil
}

// searchSubject 是对条目用的订阅：这一季（电影就是这部）的订阅；没有时按 TMDB 上的条目拼一份（不写库）。
func (s *Service) searchSubject(ctx context.Context, in MediaSearchInput) (*models.MediaSubscription, bool, error) {
	var row models.MediaSubscription
	if err := s.cfg.DB.WithContext(ctx).Where("media_type = ? AND tmdb_id = ? AND season = ?", in.MediaType, in.TMDBID, in.Season).
		Order("id").Limit(1).Find(&row).Error; err != nil {
		return nil, false, fmt.Errorf("读取订阅失败: %w", err)
	}
	if row.ID != 0 {
		return &row, true, nil
	}
	draft, err := s.draft(ctx, in.MediaType, in.TMDBID, in.Season)
	return draft, false, err
}

// searchProfile 是这次打分用的档案（见 MediaSearchInput.ProfileID）；第二个返回值为假时不打分。
func (s *Service) searchProfile(ctx context.Context, sub *models.MediaSubscription, set Settings, id int) (Profile, bool, error) {
	switch {
	case id == noProfile:
		return Profile{}, false, nil
	case id > 0:
		row, err := s.profileRow(ctx, uint(id))
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return Profile{}, false, fmt.Errorf("%w: 质量档案不存在", ErrInvalid)
			}
			return Profile{}, false, err
		}
		return profileOf(row), true, nil
	}
	return s.profileFor(ctx, sub, set), true, nil
}

// searchKeywords 用几个关键字同时搜（总时限 mediaSearchTimeout，deadline 不为零且更早时到它为止），合并结果并按站点与编号去重；
// 同一个站点的错误只留第一条，到了时限还没回应的标 TimedOut。每个关键字都搜失败时报错。
// 传给多站点搜索的 Timeout 固定是 mediaSearchTimeout：它是搜索缓存键的一部分，跟着剩余时间变就命中不了缓存。
func (s *Service) searchKeywords(ctx context.Context, kws, sites []string, deadline time.Time) ([]v2.TorrentItem, []MediaSearchError, error) {
	end := time.Now().Add(mediaSearchTimeout)
	if !deadline.IsZero() && deadline.Before(end) {
		end = deadline
	}
	sctx, cancel := context.WithDeadline(ctx, end)
	defer cancel()
	type out struct {
		res *v2.MultiSiteSearchResult
		err error
	}
	outs := make([]out, len(kws))
	var wg sync.WaitGroup
	for i, kw := range kws {
		wg.Go(func() {
			res, err := s.cfg.Search.Search(sctx, v2.MultiSiteSearchQuery{
				SearchQuery: v2.SearchQuery{Keyword: kw}, Sites: sites, Timeout: mediaSearchTimeout, RawTitles: true, KeepDuplicates: true,
			})
			outs[i] = out{res: res, err: err}
		})
	}
	wg.Wait()
	var items []v2.TorrentItem
	var errs []MediaSearchError
	seen, seenErr := map[string]bool{}, map[string]bool{}
	failed := 0
	for i, o := range outs {
		if o.err != nil {
			failed++
			errs = append(errs, MediaSearchError{Error: fmt.Sprintf("搜索「%s」失败：%v", kws[i], o.err)})
			continue
		}
		for _, e := range o.res.Errors {
			if !seenErr[e.Site] {
				seenErr[e.Site] = true
				timedOut := errors.Is(sctx.Err(), context.DeadlineExceeded) && strings.Contains(e.Error, context.DeadlineExceeded.Error())
				errs = append(errs, MediaSearchError{Site: e.Site, Error: e.Error, TimedOut: timedOut})
			}
		}
		for _, it := range o.res.Items {
			key := strings.ToLower(it.SourceSite) + "|" + it.ID
			if !seen[key] {
				seen[key] = true
				items = append(items, it)
			}
		}
	}
	if len(kws) > 0 && failed == len(kws) {
		return nil, nil, fmt.Errorf("搜索失败：%v", outs[0].err)
	}
	return items, errs, nil
}

// memberOf 是一个结果给页面看的样子。
func memberOf(c Candidate, def *v2.SiteDefinition, now time.Time) MediaSearchMember {
	it := c.Item
	m := MediaSearchMember{
		Site: it.SourceSite, TorrentID: it.ID, Title: it.Title, Subtitle: it.Subtitle, InfoHash: strings.ToLower(it.InfoHash),
		SizeBytes: it.SizeBytes, UploadedAt: it.UploadedAt, Seeders: it.Seeders, Leechers: it.Leechers, Snatched: it.Snatched,
		Discount: string(it.DiscountLevel), Free: it.IsFree(), HR: it.HasHR || c.SiteHR, URL: it.URL, DownloadURL: it.DownloadURL,
		Category: it.Category, Meta: c.Meta,
	}
	if m.Free && !it.DiscountEndTime.IsZero() {
		m.FreeLeftSeconds = max(int64(it.DiscountEndTime.Sub(now)/time.Second), 0)
	}
	if m.HR {
		m.HRHours = ptinternal.HRSeedTimeH(it.SourceSite, def, it.SizeBytes)
	}
	return m
}

// byIMDb 报告种子是不是按 IMDb 编号对上的（两边都有编号时只看编号，见 match）。
func (mt matcher) byIMDb(it v2.TorrentItem) bool {
	imdb := v2.NormalizeIMDbID(it.IMDbID)
	return imdb != "" && mt.sub.IMDbID != "" && strings.EqualFold(imdb, mt.sub.IMDbID)
}

// searchEpisodes 是这一季每集现在的情况（见 episodeStates）。
func (s *Service) searchEpisodes(ctx context.Context, sub *models.MediaSubscription, subscribed bool, season *tmdb.Season, total int, mp *episodeMapper) ([]MediaSearchEpisode, error) {
	states, err := s.episodeStates(ctx, sub, subscribed, season, total, mp)
	if err != nil {
		return nil, err
	}
	out := make([]MediaSearchEpisode, 0, len(states))
	for _, st := range states {
		out = append(out, st.MediaSearchEpisode)
	}
	return out, nil
}

// dlRef 是在下载的一集用的种子：哪个下载器、哪个 info hash（读进度用）。
type dlRef struct {
	downloaderID uint
	hash         string
	seen         time.Time // 记录记下的时间（刚记下的，缓存的下载器列表里可能还没有这个种子）
}

// episodeState 是一集现在的情况，带上入库的那条整理记录与在下载的种子（条目详情要版本、体积与进度）。
type episodeState struct {
	MediaSearchEpisode
	lib *models.MediaTransferHistory
	dl  dlRef
	// server 是算作已入库的集在哪台媒体服务器里（整理记录里没有、开了「算作已入库」的服务器里有）
	server *serverindex.SeasonHit
}

// episodeStates 是这一季每集现在的情况，只读库：整理记录里已入库的；订阅的种子与带这个条目、还没下完的推送记录在下载的；
// 订阅台账里等版本的；其余按 TMDB 的播出日期分成缺的与还没播的。
func (s *Service) episodeStates(ctx context.Context, sub *models.MediaSubscription, subscribed bool, season *tmdb.Season, total int, mp *episodeMapper) ([]episodeState, error) {
	lib, err := s.libraryRows(ctx, sub)
	if err != nil {
		return nil, err
	}
	onServer := s.countedEpisodes(ctx, sub)
	type downloading struct {
		site string
		ref  dlRef
	}
	down := map[int]downloading{}
	mark := func(covers func(int) bool, site string, ref dlRef) {
		for n := 1; n <= total; n++ {
			if _, ok := down[n]; !ok && covers(n) {
				down[n] = downloading{site: site, ref: ref}
			}
		}
	}
	waiting := map[int]bool{}
	if subscribed {
		linked, terr := s.torrents(ctx, sub.ID, models.MediaSubTorrentDownloading)
		if terr != nil {
			return nil, terr
		}
		for _, t := range linked {
			site := t.SiteName
			if strings.HasPrefix(site, models.MediaAdoptedSitePrefix) {
				site = ""
			}
			mark(covered(t), site, dlRef{downloaderID: t.DownloaderID, hash: t.InfoHash, seen: t.CreatedAt})
		}
		var rows []models.MediaSubscriptionEpisode
		if qerr := s.cfg.DB.WithContext(ctx).Select("episode", "state").Where("subscription_id = ? AND state = ?", sub.ID, models.MediaEpisodeWaitingVersion).
			Find(&rows).Error; qerr != nil {
			return nil, fmt.Errorf("读取追剧台账失败: %w", qerr)
		}
		for _, r := range rows {
			waiting[r.Episode] = true
		}
	}
	pushed, err := s.pushedTorrents(ctx, sub)
	if err != nil {
		return nil, err
	}
	for _, t := range pushed {
		m := s.parse(ctx, t.Title, "")
		if mp.grouped() {
			out, r := mp.regroup(m)
			if r.other || r.reject != "" {
				continue
			}
			m = out
		}
		sp := span{Whole: true}
		if m.Episode > 0 {
			sp = span{Start: m.Episode, End: max(m.EpisodeEnd, m.Episode)}
		}
		if mapped, ok, _ := mp.mapEpisodes(m, t.Title, "", sp); ok {
			sp = mapped
		}
		var ref dlRef
		if t.DownloaderID != nil && t.TorrentHash != nil {
			ref = dlRef{downloaderID: *t.DownloaderID, hash: *t.TorrentHash, seen: recordSeen(t.CreatedAt, t.PushTime)}
		}
		mark(sp.covers, t.SiteName, ref)
	}
	today := s.cfg.Now().In(time.Local).Format(time.DateOnly)
	out := make([]episodeState, 0, total)
	for n := 1; n <= total; n++ {
		e := MediaSearchEpisode{Episode: n}
		if ep := season.Episode(n); ep != nil {
			e.AirDate, e.Name = ep.AirDate, ep.Name
		}
		aired := e.AirDate != "" && e.AirDate <= today
		d, isDown := down[n]
		h, inLib := lib[n]
		hit, inServer := onServer[n]
		st := episodeState{}
		switch {
		case inLib:
			e.State = models.MediaEpisodeLibrary
			st.lib = &h
		case inServer:
			e.State = models.MediaEpisodeLibrary
			st.server = &hit
		case isDown:
			e.State, e.Site = models.MediaEpisodeDownloading, d.site
			st.dl = d.ref
		case aired && waiting[n]:
			e.State = models.MediaEpisodeWaitingVersion
		case aired:
			e.State = models.MediaEpisodeMissing
		default:
			e.State = models.MediaEpisodeUpcoming
		}
		st.MediaSearchEpisode = e
		out = append(out, st)
	}
	return out, nil
}

// pushedTorrents 是推送时说明了这个条目（剧集是这一季）、还没下完的种子记录。
func (s *Service) pushedTorrents(ctx context.Context, sub *models.MediaSubscription) ([]models.TorrentInfo, error) {
	var rows []models.TorrentInfo
	if err := s.cfg.DB.WithContext(ctx).Select("site_name", "title", "torrent_hash", "downloader_id", "push_time", "created_at").
		Where("media_type = ? AND tmdb_id = ? AND media_season = ? AND is_pushed = ? AND is_completed = ?", sub.MediaType, sub.TMDBID, sub.Season, true, false).
		Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取种子记录失败: %w", err)
	}
	return rows, nil
}

// searchMovie 是电影现在的情况：库里有、在下载（订阅的种子或推送记录），或者没有。
func (s *Service) searchMovie(ctx context.Context, sub *models.MediaSubscription, subscribed bool) (*MediaSearchMovie, error) {
	in, err := s.movieInLibrary(ctx, sub)
	if err != nil {
		return nil, err
	}
	if in {
		return &MediaSearchMovie{State: models.MediaEpisodeLibrary}, nil
	}
	if subscribed {
		linked, terr := s.torrents(ctx, sub.ID, models.MediaSubTorrentDownloading)
		if terr != nil {
			return nil, terr
		}
		if len(linked) > 0 {
			site := linked[0].SiteName
			if strings.HasPrefix(site, models.MediaAdoptedSitePrefix) {
				site = ""
			}
			return &MediaSearchMovie{State: models.MediaEpisodeDownloading, Site: site}, nil
		}
	}
	pushed, err := s.pushedTorrents(ctx, sub)
	if err != nil {
		return nil, err
	}
	if len(pushed) > 0 {
		return &MediaSearchMovie{State: models.MediaEpisodeDownloading, Site: pushed[0].SiteName}, nil
	}
	return &MediaSearchMovie{State: models.MediaEpisodeMissing}, nil
}

// passes 报告结果过了档案（不打分时都算过）。
func (h searchHit) passes() bool { return !h.scored || h.v.Reject == "" }

// hitBefore 是结果的先后：拉黑的在最后，过了档案的在前，再按档案的比法，最后按站点名与编号（同一批结果不管到达的顺序，
// 排出来都一样）。
func hitBefore(p Profile, a, b searchHit) int {
	if a.blocked != b.blocked {
		if b.blocked {
			return -1
		}
		return 1
	}
	if a.passes() != b.passes() {
		if a.passes() {
			return -1
		}
		return 1
	}
	if p.better(a.c, b.c, a.v, b.v) {
		return -1
	}
	if p.better(b.c, a.c, b.v, a.v) {
		return 1
	}
	return cmp.Or(cmp.Compare(strings.ToLower(a.c.Site), strings.ToLower(b.c.Site)), cmp.Compare(a.c.Item.ID, b.c.Item.ID))
}

// featureOf 是没有 InfoHash 时认同一资源的特征：体积（字节）、分辨率、制作组与集；体积不知道时没有特征。
func featureOf(h searchHit) string {
	if h.c.Item.SizeBytes <= 0 {
		return ""
	}
	eps := ""
	switch {
	case h.sp.Whole:
		eps = "all"
	case h.sp.Start > 0:
		eps = strconv.Itoa(h.sp.Start) + "-" + strconv.Itoa(h.sp.End)
	}
	return strings.Join([]string{strconv.FormatInt(h.c.Item.SizeBytes, 10), strings.ToLower(h.c.Meta.Resolution), strings.ToLower(h.c.Meta.Group), eps}, "|")
}

func digest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:6])
}

// groupHits 把结果合成组并排好序。分组按整批结果一次算出，和到达的顺序无关：
//  1. 有 InfoHash 的按 InfoHash 分组，不同的 InfoHash 一定不在一组；
//  2. 没有 InfoHash 的按特征：正好对上一个有 InfoHash 的组时并进去；一个都对不上时，同特征的没有 InfoHash 的结果一组；
//     对上两个以上时这些结果自己一组，不并进任何一个，也不把那几个组连起来；没有特征（体积不知道）的各自一组；
//  3. 组内按 hitBefore 排，第一个是首选；组与组按各组首选同样排。
func groupHits(hits []searchHit, p Profile, total int, states map[int]MediaSearchEpisode, movie *MediaSearchMovie) []MediaSearchGroup {
	byHash := map[string][]searchHit{}
	hashFeatures := map[string]map[string]bool{}
	for _, h := range hits {
		hash := strings.ToLower(h.c.Item.InfoHash)
		if hash == "" {
			continue
		}
		byHash[hash] = append(byHash[hash], h)
		if f := featureOf(h); f != "" {
			if hashFeatures[f] == nil {
				hashFeatures[f] = map[string]bool{}
			}
			hashFeatures[f][hash] = true
		}
	}
	groups := map[string][]searchHit{}
	for hash, list := range byHash {
		groups["h:"+hash] = list
	}
	for _, h := range hits {
		if h.c.Item.InfoHash != "" {
			continue
		}
		f := featureOf(h)
		switch owners := hashFeatures[f]; {
		case f == "":
			id := "s:" + strings.ToLower(h.c.Site) + ":" + h.c.Item.ID
			groups[id] = append(groups[id], h)
		case len(owners) == 1:
			for hash := range owners {
				groups["h:"+hash] = append(groups["h:"+hash], h)
			}
		default:
			id := "f:" + digest(f)
			groups[id] = append(groups[id], h)
		}
	}
	type built struct {
		g    MediaSearchGroup
		best searchHit
	}
	list := make([]built, 0, len(groups))
	for id, members := range groups {
		slices.SortFunc(members, func(a, b searchHit) int { return hitBefore(p, a, b) })
		best := members[0]
		// 组内拉黑的排在最后：首选是拉黑的，就是整组都拉黑了
		g := MediaSearchGroup{GroupID: id, Meta: best.c.Meta, Verdict: best.member.Verdict, Covers: []int{}, Downloading: []int{}, Blocked: best.blocked}
		for _, m := range members {
			g.Members = append(g.Members, m.member)
		}
		if movie != nil {
			g.InLibrary = movie.State == models.MediaEpisodeLibrary
		} else {
			g.Span = &MediaSearchSpan{Start: best.sp.Start, End: best.sp.End, Whole: best.sp.Whole}
			coveredAny, all := false, true
			for n := 1; n <= total; n++ {
				if !best.sp.covers(n) {
					continue
				}
				coveredAny = true
				switch states[n].State {
				case models.MediaEpisodeMissing, models.MediaEpisodeWaitingVersion:
					g.Covers = append(g.Covers, n)
				case models.MediaEpisodeDownloading:
					g.Downloading = append(g.Downloading, n)
				}
				if states[n].State != models.MediaEpisodeLibrary {
					all = false
				}
			}
			g.InLibrary = coveredAny && all
		}
		list = append(list, built{g: g, best: best})
	}
	slices.SortFunc(list, func(a, b built) int {
		return cmp.Or(hitBefore(p, a.best, b.best), cmp.Compare(a.g.GroupID, b.g.GroupID))
	})
	out := make([]MediaSearchGroup, 0, len(list))
	for _, b := range list {
		out = append(out, b.g)
	}
	return out
}
