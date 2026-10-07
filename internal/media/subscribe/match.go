package subscribe

import (
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
// 两边都有 IMDb 编号时只看编号；否则解析出的中文名或英文名要和订阅的名字之一相同；电影年份差不超过 1 年；
// 剧集要有季（只写了集的动画当作第 1 季），季对上，要么是整季包要么写了集。
func (mt matcher) match(m meta.Meta, it v2.TorrentItem) (span, bool) {
	imdb := v2.NormalizeIMDbID(it.IMDbID)
	if imdb != "" && mt.sub.IMDbID != "" {
		if !strings.EqualFold(imdb, mt.sub.IMDbID) {
			return span{}, false
		}
	} else if !mt.names[recognize.Normalize(m.NameCN)] && !mt.names[recognize.Normalize(m.NameEN)] {
		return span{}, false
	}
	if mt.sub.MediaType == models.MediaKindMovie {
		if m.Type == meta.TypeTV {
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
