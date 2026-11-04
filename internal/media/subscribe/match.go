package subscribe

import (
	"fmt"
	"strings"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// span 是种子里的集：Whole 为真时是整季（或包含这一季的多季合集），否则是 [Start, End]。电影为零值。
type span struct {
	Start, End int
	Whole      bool
}

// count 是这段有几集；整季时用这一季的集数（不知道时当作 1）。
func (s span) count(total int) int {
	if s.Whole {
		return max(total, 1)
	}
	if s.Start == 0 {
		return 1
	}
	return s.End - s.Start + 1
}

// covers 报告这段包不包括第 n 集。
func (s span) covers(n int) bool {
	return s.Whole || (n >= s.Start && n <= s.End)
}

// matcher 是一个订阅用来对种子的信息。
type matcher struct {
	sub   models.MediaSubscription
	names map[string]bool
	// mapper 是追剧的集号映射（综艺的期上下、Part、日期）；为空时标题里的集号就是 TMDB 的集号
	mapper *episodeMapper
}

// withMapper 带上集号映射。
func (mt matcher) withMapper(mp *episodeMapper) matcher {
	mt.mapper = mp
	return mt
}

// matchMapped 和 match 一样对种子，剧集先按追剧选的剧集组换成 TMDB 默认的季集（见 regroup），再按集号映射对到 TMDB 的集：
// 对不上时第二个返回值为假、第三个写明原因（名字、季对不上的不写原因，不是这个订阅的种子）；对上了时第三个是要写进运行记录的
// 说明（剧集组里没有写的集、按默认的季集算的），没有时为空。剧集的年份不参与（综艺常写播出的年份）。
func (mt matcher) matchMapped(m meta.Meta, it v2.TorrentItem) (span, bool, string) {
	note := ""
	if mt.grouped() {
		if !mt.sameTitle(m, it) {
			return span{}, false, ""
		}
		if mt.mapper == nil {
			return span{}, false, "读不到剧集组"
		}
		out, r := mt.mapper.regroup(m)
		switch {
		case r.reject != "":
			return span{}, false, r.reject
		case r.other:
			return span{}, false, ""
		}
		m, note = out, r.note
	}
	sp, ok := mt.match(m, it)
	if !ok {
		if note != "" {
			// 剧集组里没有、按默认的季集算又不是这一季的（多半是剧集组还没加上的新的集）：写明，看得出为什么没要
			return sp, false, fmt.Sprintf("%s，不是第 %d 季的", note, mt.sub.Season)
		}
		return sp, false, ""
	}
	if mt.sub.MediaType != models.MediaKindTV || mt.mapper == nil {
		return sp, true, note
	}
	sp, ok, why := mt.mapper.mapEpisodes(m, it.Title, it.Subtitle, sp)
	if ok && why == "" {
		why = note
	}
	return sp, ok, why
}

// grouped 报告这个追剧选了剧集组（种子里的季集要先换算）。
func (mt matcher) grouped() bool {
	return mt.sub.MediaType == models.MediaKindTV && mt.sub.EpisodeGroupID != ""
}

// mayMatch 是 RSS 交来的种子的快速筛：选了剧集组的追剧只看是不是这部剧（季集要按剧集组换算，评估时再对），其余同 match。
func (mt matcher) mayMatch(m meta.Meta, it v2.TorrentItem) bool {
	if mt.grouped() {
		return mt.sameTitle(m, it)
	}
	_, ok := mt.match(m, it)
	return ok
}

// sameTitle 报告种子是不是这部片：两边都有 IMDb 编号时只看编号，否则解析出的中文名或英文名要和订阅的名字之一相同。
func (mt matcher) sameTitle(m meta.Meta, it v2.TorrentItem) bool {
	if imdb := v2.NormalizeIMDbID(it.IMDbID); imdb != "" && mt.sub.IMDbID != "" {
		return strings.EqualFold(imdb, mt.sub.IMDbID)
	}
	return mt.names[recognize.Normalize(m.NameCN)] || mt.names[recognize.Normalize(m.NameEN)]
}

func newMatcher(sub models.MediaSubscription) matcher {
	m := matcher{sub: sub, names: map[string]bool{}}
	for _, n := range append([]string{sub.Title, sub.OriginalTitle}, decodeList(sub.Aliases)...) {
		if k := recognize.Normalize(n); k != "" {
			m.names[k] = true
		}
	}
	return m
}

// match 报告候选种子是不是这个订阅要的，是时返回种子里的集（剧集）：
// 两边都有 IMDb 编号时只看编号；否则解析出的中文名或英文名要和订阅的名字之一相同；电影年份差不超过 1 年，
// 只按名字对时种子要写了年份（同名的电影不同年份的多）；剧集要有季（只写了集的动画当作第 1 季），季对上，
// 要么是整季包要么写了集。
func (mt matcher) match(m meta.Meta, it v2.TorrentItem) (span, bool) {
	byID := v2.NormalizeIMDbID(it.IMDbID) != "" && mt.sub.IMDbID != ""
	if !mt.sameTitle(m, it) {
		return span{}, false
	}
	if mt.sub.MediaType == models.MediaKindMovie {
		if m.Type == meta.TypeTV {
			return span{}, false
		}
		if !byID && m.Year == 0 && mt.sub.Year > 0 {
			return span{}, false
		}
		if m.Year > 0 && mt.sub.Year > 0 && (m.Year-mt.sub.Year > 1 || mt.sub.Year-m.Year > 1) {
			return span{}, false
		}
		return span{}, true
	}
	season, seasonEnd := m.Season, m.SeasonEnd
	if season == 0 && m.Episode > 0 {
		season = 1
	}
	if season == 0 {
		return span{}, false
	}
	if seasonEnd > season {
		if mt.sub.Season < season || mt.sub.Season > seasonEnd {
			return span{}, false
		}
		return span{Whole: true}, true
	}
	if season != mt.sub.Season {
		return span{}, false
	}
	if m.Episode > 0 {
		return span{Start: m.Episode, End: max(m.EpisodeEnd, m.Episode)}, true
	}
	return span{Whole: true}, true
}
