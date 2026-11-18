package subscribe

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/redact"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 下载前让 AI 复核（媒体中心 M9b）：订阅设置里打开以后，推送一个候选以前问 AI 助手的模型，这个种子是不是订阅要的条目
// （剧集还要是这一季）。「不是」的候选按拒绝处理，接着看下一个；调用失败时照常推送并写进运行记录，这一轮后面的候选不再复核
// （模型不通时不让每个候选都等一次）。每次运行最多问 maxReviewsPerRun 次，问满了剩下的候选下次再看；同一订阅、同一种子的
// 结论在内存里记 24 小时，不重复问。用户在一键追剧里自己选的种子不复核。

// AI 复核的范围。
const (
	// ReviewByName 只复核按名字对上的（种子与订阅没有都写 IMDb 编号）
	ReviewByName = "name"
	// ReviewAll 每个要推送的候选都复核
	ReviewAll = "all"
)

const (
	maxReviewsPerRun = 10
	reviewCacheTTL   = 24 * time.Hour
	// reviewCacheMax 是记下的结论的条数上限：超过时先清掉过期的，还是多就全清掉
	reviewCacheMax = 4096
)

// ReviewInput 是交给模型复核的一个候选：订阅的条目与站点上的种子。
type ReviewInput struct {
	// Kind 是 movie 或 tv
	Kind          string
	Title         string
	OriginalTitle string
	Year          int
	// Season 是剧集的季（电影为 0）
	Season  int
	Aliases []string
	// TorrentTitle、TorrentSubtitle 是站点上的标题与副标题；Parsed 是从标题解析出的信息与对到的季集（一行）
	TorrentTitle    string
	TorrentSubtitle string
	Parsed          string
}

// ReviewResult 是模型的结论：Match 为真是「是」；Reason 是它给的理由；Tokens 是这次用的 token 数（不知道时为 0）。
type ReviewResult struct {
	Match  bool
	Reason string
	Tokens int64
}

// Reviewer 复核一个候选（cmd 接到 AI 助手的 ReviewCandidate）。
type Reviewer func(ctx context.Context, in ReviewInput) (ReviewResult, error)

// ErrReviewUnclear 是模型的回答看不出是还是不是：只是这一个候选没复核成，后面的候选照常复核。
var ErrReviewUnclear = errors.New("看不出模型回答的是还是不是")

// SetReviewer 接上 AI 复核（订阅服务启动以后也能接；nil 断开）。
func (s *Service) SetReviewer(r Reviewer) {
	s.reviewMu.Lock()
	s.reviewer = r
	s.reviewMu.Unlock()
}

type reviewKey struct {
	sub      uint
	site, id string
}

type reviewEntry struct {
	res ReviewResult
	at  time.Time
}

// reviewOutcome 是复核以后推不推这个候选。
type reviewOutcome int

const (
	reviewPass reviewOutcome = iota
	reviewReject
	// reviewStop：这一轮问满了，这个和后面的都不推，下次再看
	reviewStop
)

// review 在推送 o 以前按订阅设置让 AI 复核。没开复核、范围之外、用户自己选的种子直接推。
func (s *Service) review(ctx context.Context, sub *models.MediaSubscription, o scored, set Settings) reviewOutcome {
	if !set.AIReviewEnabled || o.c.Chosen {
		return reviewPass
	}
	if set.AIReviewScope != ReviewAll && v2.NormalizeIMDbID(o.c.Item.IMDbID) != "" && sub.IMDbID != "" {
		// 两边都写了 IMDb 编号、按编号对上的
		return reviewPass
	}
	key := reviewKey{sub: sub.ID, site: strings.ToLower(o.c.Site), id: o.c.Item.ID}
	now := s.cfg.Now()
	s.reviewMu.Lock()
	e, hit := s.reviews[key]
	r := s.reviewer
	s.reviewMu.Unlock()
	if hit && now.Sub(e.at) < reviewCacheTTL {
		if !e.res.Match {
			step(ctx, stepReject, "%s：AI 复核：%s（%s 问过）", o.c.Item.Title, reviewWhy(e.res), e.at.In(time.Local).Format("01-02 15:04"))
			return reviewReject
		}
		return reviewPass
	}
	down, full, first := "", false, false
	markRun(ctx, func(rl *runLog) {
		down = rl.reviewDown
		switch {
		case down != "":
		case rl.reviews >= maxReviewsPerRun:
			full, first = true, !rl.reviewFull
			rl.reviewFull, rl.waiting = true, true
			if rl.unfinished == "" {
				rl.unfinished = fmt.Sprintf("这一轮 AI 复核问满了 %d 个", maxReviewsPerRun)
			}
		default:
			rl.reviews++
		}
	})
	switch {
	case down != "":
		return reviewPass
	case full:
		if first {
			step(ctx, stepWait, "这一轮已经让 AI 复核了 %d 个，剩下的候选下次再看", maxReviewsPerRun)
		}
		return reviewStop
	case r == nil:
		markRun(ctx, func(rl *runLog) { rl.reviewDown = "AI 助手没有启动" })
		step(ctx, stepInfo, "%s：AI 复核没做成：AI 助手没有启动，照常推送", o.c.Item.Title)
		return reviewPass
	}
	res, err := r(ctx, reviewInput(sub, o))
	if err != nil {
		why := redact.Text(err.Error())
		if errors.Is(err, ErrReviewUnclear) {
			step(ctx, stepInfo, "%s：AI 复核没做成：%s，照常推送", o.c.Item.Title, why)
			return reviewPass
		}
		markRun(ctx, func(rl *runLog) { rl.reviewDown = why })
		step(ctx, stepInfo, "%s：AI 复核没做成：%s，照常推送（这一轮后面的候选不再复核）", o.c.Item.Title, why)
		return reviewPass
	}
	res.Reason = redact.Text(strings.TrimSpace(res.Reason))
	s.remember(key, res, now)
	used := ""
	if res.Tokens > 0 {
		used = fmt.Sprintf("（%d tokens）", res.Tokens)
	}
	if !res.Match {
		step(ctx, stepReject, "%s：AI 复核：%s%s", o.c.Item.Title, reviewWhy(res), used)
		return reviewReject
	}
	if res.Reason != "" {
		step(ctx, stepInfo, "%s：AI 复核：是，%s%s", o.c.Item.Title, res.Reason, used)
	} else {
		step(ctx, stepInfo, "%s：AI 复核：是%s", o.c.Item.Title, used)
	}
	return reviewPass
}

// reviewWhy 是「不是」的说明：模型给了理由时是理由。
func reviewWhy(res ReviewResult) string {
	if res.Reason != "" {
		return "不是，" + res.Reason
	}
	return "不是"
}

// remember 记下一个结论（24 小时内不再问）。
func (s *Service) remember(key reviewKey, res ReviewResult, at time.Time) {
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	if s.reviews == nil {
		s.reviews = map[reviewKey]reviewEntry{}
	}
	if len(s.reviews) >= reviewCacheMax {
		for k, e := range s.reviews {
			if at.Sub(e.at) >= reviewCacheTTL {
				delete(s.reviews, k)
			}
		}
		if len(s.reviews) >= reviewCacheMax {
			clear(s.reviews)
		}
	}
	s.reviews[key] = reviewEntry{res: res, at: at}
}

// reviewInput 是交给模型的一个候选：订阅的名字、年份、季、别名，以及种子的标题、副标题与解析出的信息。
func reviewInput(sub *models.MediaSubscription, o scored) ReviewInput {
	in := ReviewInput{
		Kind: sub.MediaType, Title: sub.Title, OriginalTitle: sub.OriginalTitle, Year: sub.Year, Aliases: decodeList(sub.Aliases),
		TorrentTitle: o.c.Item.Title, TorrentSubtitle: o.c.Item.Subtitle,
	}
	if sub.MediaType == models.MediaKindTV {
		in.Season = sub.Season
	}
	m := o.c.Meta
	var parts []string
	if m.NameCN != "" {
		parts = append(parts, "中文名 "+m.NameCN)
	}
	if m.NameEN != "" {
		parts = append(parts, "英文名 "+m.NameEN)
	}
	if m.Year > 0 {
		parts = append(parts, fmt.Sprintf("年份 %d", m.Year))
	}
	switch {
	case m.Season > 0 && m.SeasonEnd > m.Season:
		parts = append(parts, fmt.Sprintf("第 %d–%d 季", m.Season, m.SeasonEnd))
	case m.Season > 0:
		parts = append(parts, fmt.Sprintf("第 %d 季", m.Season))
	}
	switch {
	case m.Episode > 0 && m.EpisodeEnd > m.Episode:
		parts = append(parts, fmt.Sprintf("第 %d–%d 集", m.Episode, m.EpisodeEnd))
	case m.Episode > 0:
		parts = append(parts, fmt.Sprintf("第 %d 集", m.Episode))
	}
	if m.Resolution != "" {
		parts = append(parts, m.Resolution)
	}
	in.Parsed = strings.Join(parts, "，")
	if sub.MediaType == models.MediaKindTV {
		got := fmt.Sprintf("第 %d 季整季", sub.Season)
		switch {
		case o.sp.Whole:
		case o.sp.End > o.sp.Start:
			got = fmt.Sprintf("第 %d 季第 %d–%d 集", sub.Season, o.sp.Start, o.sp.End)
		default:
			got = fmt.Sprintf("第 %d 季第 %d 集", sub.Season, o.sp.Start)
		}
		if in.Parsed != "" {
			in.Parsed += "；"
		}
		in.Parsed += "按集号对到这部剧的" + got
	}
	return in
}

// pushReviewed 推送 list 里第一个通过 AI 复核的候选（没开复核时就是第一个）。复核都说不是、问满了时返回说明。
func (s *Service) pushReviewed(ctx context.Context, sub *models.MediaSubscription, list []scored, set Settings, want map[int]bool) string {
	rejected := 0
	for _, o := range list {
		switch s.review(ctx, sub, o, set) {
		case reviewReject:
			rejected++
			continue
		case reviewStop:
			return reviewStopMsg(rejected)
		}
		return s.pushOne(ctx, sub, o, set, want)
	}
	return fmt.Sprintf("找到 %d 个，AI 复核都说不是", rejected)
}

func reviewStopMsg(rejected int) string {
	return fmt.Sprintf("AI 复核说 %d 个不是，这一轮问满了 %d 个，剩下的下次再看", rejected, maxReviewsPerRun)
}
