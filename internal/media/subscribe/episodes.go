package subscribe

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

// 综艺的集号映射：种子标题里的集号常常不是 TMDB 的集号。综艺在 TMDB 上一期常拆成「第1期上」「第1期下」两集，
// 制作组有的按 TMDB 的集号编（E03 = 第2期上），有的写「E01 Part 2」（第1期下），有的写「第1期下」或播出日期。
// 映射的顺序：这个追剧给这个组设的规则 → 写了 Part 或上下的按（期，部分）找 → 写了日期的按播出日期找
// （同一天几集时再按部分分）→ 只有 E{n} 的就是 TMDB 的第 n 集。拿不准时不要这个种子（「集号对不上」）。

// TMDB 的类型：真人秀与脱口秀。
const (
	genreReality = 10764
	genreTalk    = 10767
)

// part 是一期里的部分：上、中、下，加更与会员版排在后面。
var partOrder = []string{"上", "中", "下", "加更", "会员"}

func partRank(p string) int {
	if i := slices.Index(partOrder, p); i >= 0 {
		return i
	}
	return len(partOrder)
}

var (
	// 集名里的「第1期上」「第1期（下）」「第 12 期加更」「第3期会员版」
	periodNameRe = regexp.MustCompile(`第\s*(\d{1,4})\s*期\s*[（(]?\s*(上|中|下|加更|会员版?|纯享版?)?`)
	// 占位的集名「第 5 集」「Episode 5」：不参与期号索引
	placeholderRe = regexp.MustCompile(`(?i)^\s*(第\s*\d+\s*集|episode\s*\d+)\s*$`)
	// 标题里的「E01 Part 2」「E01.Part.2」「E02 part1」
	partHintRe = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:S\d{1,4})?E(\d{1,4})[\s._-]*Part[\s._-]*(\d)(?:[^0-9]|$)`)
	// 标题里的完整日期：20261018、2026.10.18、2026-10-18
	dateHintRe = regexp.MustCompile(`(?:^|[^0-9])(20\d{2})[.\-]?(0[1-9]|1[0-2])[.\-]?(0[1-9]|[12][0-9]|3[01])(?:[^0-9]|$)`)
	// 标题里只有月日的「1018期」
	monthDayHintRe = regexp.MustCompile(`(?:^|[^0-9])(0[1-9]|1[0-2])(0[1-9]|[12][0-9]|3[01])\s*期`)
)

// normPart 把「会员版」「纯享版」规整成会员、纯享。
func normPart(p string) string {
	return strings.TrimSuffix(p, "版")
}

// epHint 是从标题里补充解析出的集号线索（meta.Parse 不认这些写法）。
type epHint struct {
	// period、part：第几期、哪一部分（上中下加更会员；Part n 时是 n 的序号写法 "#n"）
	period int
	part   string
	// date 是完整日期（YYYY-MM-DD），monthDay 是只有月日（MM-DD）
	date, monthDay string
}

func (h epHint) empty() bool { return h.period == 0 && h.date == "" && h.monthDay == "" }

// parseHint 从种子标题（与副标题）里找期号、部分与日期。
func parseHint(title, subtitle string) epHint {
	var h epHint
	for _, s := range []string{title, subtitle} {
		if s == "" {
			continue
		}
		if h.period == 0 {
			if m := partHintRe.FindStringSubmatch(s); m != nil {
				h.period, _ = strconv.Atoi(m[1])
				h.part = "#" + m[2]
			} else if m := periodNameRe.FindStringSubmatch(s); m != nil {
				h.period, _ = strconv.Atoi(m[1])
				h.part = normPart(m[2])
			}
		}
		if h.date == "" && h.monthDay == "" {
			if m := dateHintRe.FindStringSubmatch(s); m != nil {
				h.date = m[1] + "-" + m[2] + "-" + m[3]
			} else if m := monthDayHintRe.FindStringSubmatch(s); m != nil {
				h.monthDay = m[1] + "-" + m[2]
			}
		}
	}
	return h
}

// periodKey 是（期号，部分）。
type periodKey struct {
	period int
	part   string
}

// episodeIndex 是一季的集号映射用的索引。
type episodeIndex struct {
	variety bool
	// byPeriod：（期号，部分）→ TMDB 集号；periods：期号 → 这一期的各集（按部分排好）
	byPeriod map[periodKey]int
	periods  map[int][]int
	// byDate：播出日期 → 那天播出的集
	byDate  map[string][]int
	partOf  map[int]string
	numbers map[int]bool
}

// newEpisodeIndex 按 TMDB 这一季的集列表建索引。genres 是剧集的类型编号（判断是不是综艺）。
func newEpisodeIndex(season *tmdb.Season, genres []int) *episodeIndex {
	ix := &episodeIndex{
		byPeriod: map[periodKey]int{}, periods: map[int][]int{}, byDate: map[string][]int{}, partOf: map[int]string{}, numbers: map[int]bool{},
	}
	if season == nil {
		return ix
	}
	named, periodic := 0, 0
	for _, e := range season.Episodes {
		ix.numbers[e.Number] = true
		if e.AirDate != "" {
			ix.byDate[e.AirDate] = append(ix.byDate[e.AirDate], e.Number)
		}
		if e.Name == "" || placeholderRe.MatchString(e.Name) {
			continue
		}
		named++
		m := periodNameRe.FindStringSubmatch(e.Name)
		if m == nil {
			continue
		}
		periodic++
		n, _ := strconv.Atoi(m[1])
		part := normPart(m[2])
		key := periodKey{n, part}
		if _, dup := ix.byPeriod[key]; dup {
			continue
		}
		ix.byPeriod[key] = e.Number
		ix.partOf[e.Number] = part
		ix.periods[n] = append(ix.periods[n], e.Number)
	}
	for n := range ix.periods {
		slices.SortFunc(ix.periods[n], func(a, b int) int { return partRank(ix.partOf[a]) - partRank(ix.partOf[b]) })
	}
	ix.variety = slices.Contains(genres, genreReality) || slices.Contains(genres, genreTalk) || (named > 0 && periodic*2 >= named)
	return ix
}

// byPart 找一期里的一部分：中文的部分直接对；Part n 按这一期的第 n 个部分（上、下……）。
func (ix *episodeIndex) byPart(period int, part string) (int, bool) {
	if strings.HasPrefix(part, "#") {
		i, _ := strconv.Atoi(part[1:])
		eps := ix.periods[period]
		if i < 1 || i > len(eps) {
			return 0, false
		}
		return eps[i-1], true
	}
	n, ok := ix.byPeriod[periodKey{period, part}]
	return n, ok
}

// byDay 找某天播出的集：同一天几集时按部分分（Part n 是那天的第 n 集）。
func (ix *episodeIndex) byDay(date, part string) (int, bool) {
	eps := ix.byDate[date]
	switch {
	case len(eps) == 1 && part == "":
		return eps[0], true
	case len(eps) == 0:
		return 0, false
	}
	sorted := slices.Clone(eps)
	slices.Sort(sorted)
	if strings.HasPrefix(part, "#") {
		i, _ := strconv.Atoi(part[1:])
		if i >= 1 && i <= len(sorted) {
			return sorted[i-1], true
		}
		return 0, false
	}
	for _, n := range sorted {
		if part != "" && ix.partOf[n] == part {
			return n, true
		}
	}
	return 0, false
}

// byMonthDay 找只写了月日的集：这一季里只有一个那样的日期时才算。
func (ix *episodeIndex) byMonthDay(md, part string) (int, bool) {
	var found string
	for d := range ix.byDate {
		if strings.HasSuffix(d, "-"+md) {
			if found != "" {
				return 0, false
			}
			found = d
		}
	}
	if found == "" {
		return 0, false
	}
	return ix.byDay(found, part)
}

// episodeMapper 把种子标题里的集对到 TMDB 的集。
type episodeMapper struct {
	index *episodeIndex
	// rules：制作组（小写，空 = 所有组）→ 规则
	rules map[string]models.MediaEpisodeRule
}

func newEpisodeMapper(ix *episodeIndex, rules []models.MediaEpisodeRule) *episodeMapper {
	mp := &episodeMapper{index: ix, rules: map[string]models.MediaEpisodeRule{}}
	for _, r := range rules {
		mp.rules[strings.ToLower(strings.TrimSpace(r.Group))] = r
	}
	return mp
}

func (mp *episodeMapper) rule(group string) (models.MediaEpisodeRule, bool) {
	if r, ok := mp.rules[strings.ToLower(group)]; ok && group != "" {
		return r, true
	}
	r, ok := mp.rules[""]
	return r, ok
}

// mapEpisodes 把解析出的集（sp 是 meta 给的：E{n} 或整季）对到 TMDB 的集。第二个返回值为假时不要这个种子，第三个是原因。
func (mp *episodeMapper) mapEpisodes(m meta.Meta, title, subtitle string, sp span) (span, bool, string) {
	if mp == nil || mp.index == nil {
		return sp, true, ""
	}
	ix := mp.index
	h := parseHint(title, subtitle)
	one := func(n int) span { return span{Start: n, End: n} }
	if r, ok := mp.rule(m.Group); ok {
		switch r.Mode {
		case models.MediaEpisodeRuleOffset:
			switch {
			case sp.Whole && h.empty():
				return sp, true, ""
			case sp.Whole || sp.Start == 0:
				return sp, false, "集号对不上：规则是按偏移算，标题里没有 E 集号"
			}
			return span{Start: sp.Start + r.Offset, End: sp.End + r.Offset}, true, ""
		case models.MediaEpisodeRuleTMDB:
			if !h.empty() && (sp.Whole || sp.Start == 0) {
				return sp, false, "集号对不上：规则是按 TMDB 集号算，标题里没有 E 集号"
			}
			return sp, true, ""
		case models.MediaEpisodeRulePeriodPart:
			period, part := h.period, h.part
			if period == 0 && sp.Start > 0 && !sp.Whole && sp.End == sp.Start {
				period = sp.Start
			}
			if period == 0 {
				return sp, false, "集号对不上：规则是按期号算，标题里没有期号"
			}
			if part == "" {
				// 只写了第几期：这一期的几集都在里面
				eps := ix.periods[period]
				if len(eps) == 0 {
					return sp, false, fmt.Sprintf("集号对不上：TMDB 上没有第 %d 期", period)
				}
				return span{Start: slices.Min(eps), End: slices.Max(eps)}, true, ""
			}
			if n, ok := ix.byPart(period, part); ok {
				return one(n), true, ""
			}
			return sp, false, fmt.Sprintf("集号对不上：TMDB 上没有第 %d 期%s", period, partLabel(part))
		case models.MediaEpisodeRuleDate:
			return mp.byDateHint(h, sp)
		}
	}
	// 写了期号与部分的（Part n、第N期上下）：按（期，部分）找
	if h.period > 0 {
		if !ix.variety {
			return sp, false, "集号对不上：标题写的是第几期，TMDB 上这一季不是按期编的"
		}
		if h.part == "" {
			eps := ix.periods[h.period]
			switch len(eps) {
			case 0:
				return sp, false, fmt.Sprintf("集号对不上：TMDB 上没有第 %d 期", h.period)
			case 1:
				return one(eps[0]), true, ""
			}
			return span{Start: slices.Min(eps), End: slices.Max(eps)}, true, ""
		}
		if n, ok := ix.byPart(h.period, h.part); ok {
			return one(n), true, ""
		}
		return sp, false, fmt.Sprintf("集号对不上：TMDB 上没有第 %d 期%s", h.period, partLabel(h.part))
	}
	// 写了日期的：按播出日期找
	if h.date != "" || h.monthDay != "" {
		return mp.byDateHint(h, sp)
	}
	return sp, true, ""
}

func (mp *episodeMapper) byDateHint(h epHint, sp span) (span, bool, string) {
	ix := mp.index
	var n int
	var ok bool
	switch {
	case h.date != "":
		n, ok = ix.byDay(h.date, h.part)
	case h.monthDay != "":
		n, ok = ix.byMonthDay(h.monthDay, h.part)
	default:
		return sp, false, "集号对不上：规则是按日期算，标题里没有日期"
	}
	if !ok {
		day := h.date
		if day == "" {
			day = h.monthDay
		}
		return sp, false, fmt.Sprintf("集号对不上：TMDB 上找不到 %s 播出的这一集", day)
	}
	return span{Start: n, End: n}, true, ""
}

func partLabel(part string) string {
	if strings.HasPrefix(part, "#") {
		return " Part " + part[1:]
	}
	return part
}
