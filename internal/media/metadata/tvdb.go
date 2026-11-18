package metadata

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/sunerpy/pt-tools/internal/media/tvdb"
)

// TVDBAPI 是 TheTVDB 适配要用的接口（生产环境是 *tvdb.Client）。
type TVDBAPI interface {
	Episodes(ctx context.Context, seriesID int, lang string) (*tvdb.SeriesEpisodes, error)
	Download(ctx context.Context, url string) ([]byte, error)
}

// TVDB 是 TheTVDB 来源：剧集每一集的标题、简介（识别语言的）与截图。按 TMDB 上的 TVDB 编号找，按播出顺序（Aired Order），
// 不按名字找。只用识别语言的文字（这一集在 TheTVDB 上有这种语言的翻译，或者剧集的原语言就是它），占位的标题（「第 1 集」、
// 纯数字、Episode 1 一类）不用；第 0 季（特别篇）不用。一次刮削里同一部剧只取一次。
type TVDB struct {
	c    TVDBAPI
	lang string
	once memo
}

// NewTVDB 建 TheTVDB 来源：recogLang 是识别语言（例如 zh-CN），按它选 TheTVDB 的语言。c 为 nil 时返回 nil（没填 Key）。
func NewTVDB(c TVDBAPI, recogLang string) *TVDB {
	if c == nil {
		return nil
	}
	return &TVDB{c: c, lang: tvdbLang(recogLang)}
}

// ID 是来源编号。
func (t *TVDB) ID() string { return SourceTVDB }

// tvdbLang 是识别语言在 TheTVDB 上的三个字母的代码：简体中文 zho，繁体中文 zhtw，日文 jpn，其余用英文 eng。
func tvdbLang(recog string) string {
	switch strings.ToLower(strings.TrimSpace(recog)) {
	case "zh-cn", "zh", "zh-hans", "zh-sg":
		return "zho"
	case "zh-tw", "zh-hk", "zh-hant", "zh-mo":
		return "zhtw"
	case "ja-jp", "ja":
		return "jpn"
	}
	return "eng"
}

// series 取这部剧在 TheTVDB 上的每一集。没有 TVDB 编号、集太多时返回 Note。
func (t *TVDB) series(ctx context.Context, q Query) (*tvdb.SeriesEpisodes, error) {
	id, _ := strconv.Atoi(q.IDs[IDTVDB])
	if id <= 0 {
		return nil, Note("TheTVDB 按 TVDB 编号找剧集，TMDB 上这部没有 TVDB 编号，每一集与集截图用 TMDB 的")
	}
	v, err := t.once.do(fmt.Sprintf("episodes:%d:%s", id, t.lang), func() (any, error) { return t.c.Episodes(ctx, id, t.lang) })
	if errors.Is(err, tvdb.ErrTooMany) {
		return nil, Note(fmt.Sprintf("TheTVDB 上这部的集太多（超过 %d 页），没有用，每一集与集截图用 TMDB 的", tvdb.MaxPages))
	}
	if err != nil {
		return nil, err
	}
	se, _ := v.(*tvdb.SeriesEpisodes)
	return se, nil
}

// Season 取一季每一集的标题、简介（能用的才填）、播出日期与 TheTVDB 上的编号。没有这部或这一季时返回 nil、nil。
func (t *TVDB) Season(ctx context.Context, q Query, ref SeasonRef) (*Season, error) {
	if q.Kind != KindTV || ref.Number < 1 {
		return nil, nil
	}
	se, err := t.series(ctx, q)
	if err != nil || se == nil {
		return nil, err
	}
	var out *Season
	for _, e := range se.Episodes {
		if e.Season != ref.Number || e.Number < 1 {
			continue
		}
		if out == nil {
			out = &Season{Number: ref.Number}
		}
		ep := Episode{
			Season: ref.Number, Number: e.Number, Aired: dateOnly(e.Aired),
			Title: t.usable(e.Name, e.NameLangs, se, true),
			Plot:  t.usable(e.Overview, e.OverviewLangs, se, false),
		}
		if e.ID > 0 {
			ep.IDs = setID(ep.IDs, IDTVDB, strconv.Itoa(e.ID))
		}
		out.Episodes = append(out.Episodes, ep)
	}
	if out != nil {
		slices.SortStableFunc(out.Episodes, func(a, b Episode) int { return a.Number - b.Number })
	}
	return out, nil
}

// Art 取要的那几季的集截图（只有集截图）。
func (t *TVDB) Art(ctx context.Context, q Query, req ArtRequest) ([]Artwork, error) {
	if q.Kind != KindTV || !slices.Contains(req.Kinds, ArtThumb) {
		return nil, nil
	}
	se, err := t.series(ctx, q)
	if err != nil || se == nil {
		return nil, err
	}
	var out []Artwork
	for _, e := range se.Episodes {
		if e.Season < 1 || e.Number < 1 || e.Image == "" || !slices.Contains(req.Seasons, e.Season) {
			continue
		}
		out = append(out, Artwork{Kind: ArtThumb, Season: e.Season, Episode: e.Number, Default: true, Ref: e.Image})
	}
	return out, nil
}

// Download 下载一张 TheTVDB 图片。
func (t *TVDB) Download(ctx context.Context, a Artwork) ([]byte, error) {
	return t.c.Download(ctx, a.Ref)
}

// usable 是能用的文字：这一集有识别语言的翻译（langs 里有），或者剧集的原语言就是它；看起来是这种语言的字；
// 不是占位的标题或简介。取到的是不带语言的列表（se.Untranslated，文字是原语言的）时，只有原语言就是识别语言才用。
// 不能用时为空串。
func (t *TVDB) usable(text string, langs []string, se *tvdb.SeriesEpisodes, title bool) string {
	text = strings.TrimSpace(text)
	original := se.OriginalLanguage
	if text == "" || (se.Untranslated && original != t.lang) || (!slices.Contains(langs, t.lang) && original != t.lang) || !looksLike(text, t.lang) {
		return ""
	}
	if title && placeholderTitleRe.MatchString(text) || !title && placeholderPlotRe.MatchString(text) {
		return ""
	}
	return text
}

// looksLike 报告文字像不像这种语言：中文要有汉字、没有假名；日文要有假名或汉字；英文（其余）里没有汉字、假名与谚文。
func looksLike(s, lang string) bool {
	var han, kana, hangul bool
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Han, r):
			han = true
		case unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r):
			kana = true
		case unicode.Is(unicode.Hangul, r):
			hangul = true
		}
	}
	switch lang {
	case "zho", "zhtw":
		return han && !kana
	case "jpn":
		return kana || han
	}
	return !han && !kana && !hangul
}

var (
	// placeholderTitleRe 是占位的集标题：「第 1 集」「第一期」「第1话」、纯数字、Episode 1、EP 1、E01、S01E01、TBA、TBD
	placeholderTitleRe = regexp.MustCompile(`(?i)^\s*(第\s*[0-9０-９一二三四五六七八九十百千零〇两]+\s*[集期话話回]|[0-9０-９]+|episode\s*[0-9]+|ep\.?\s*[0-9]+|e[0-9]+|s[0-9]+\s*e[0-9]+|tba|tbd)\s*$`)
	// placeholderPlotRe 是占位的简介
	placeholderPlotRe = regexp.MustCompile(`(?i)^\s*(tba|tbd|n/?a)\s*$`)
)

// dateOnly 是 yyyy-mm-dd（接口有时带时间）；认不出时为空串。
func dateOnly(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 {
		if _, err := time.Parse(time.DateOnly, s[:10]); err == nil {
			return s[:10]
		}
	}
	return ""
}

// misalignedEpisode 是两边都有播出日期、日期差一天以上的第一集的集号（TheTVDB 与 TMDB 的集号对不上）；对得上或没有共同的日期时为 0。
func misalignedEpisode(tm, tv *Season) int {
	if tm == nil || tv == nil {
		return 0
	}
	for _, e := range tm.Episodes {
		a, err := time.Parse(time.DateOnly, dateOnly(e.Aired))
		if err != nil {
			continue
		}
		o := tv.Episode(e.Number)
		if o == nil {
			continue
		}
		b, err := time.Parse(time.DateOnly, o.Aired)
		if err != nil {
			continue
		}
		if d := a.Sub(b); d > 24*time.Hour || d < -24*time.Hour {
			return e.Number
		}
	}
	return 0
}

// withText 是这一季里有标题或简介的集（合并时只用这些，没有文字的集不加进来）。
func withText(s *Season) *Season {
	out := *s
	out.Episodes = nil
	for _, e := range s.Episodes {
		if e.Title != "" || e.Plot != "" {
			out.Episodes = append(out.Episodes, e)
		}
	}
	return &out
}
