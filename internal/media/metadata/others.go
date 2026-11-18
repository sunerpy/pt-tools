package metadata

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/sunerpy/pt-tools/internal/media/bangumi"
	"github.com/sunerpy/pt-tools/internal/media/douban"
	"github.com/sunerpy/pt-tools/internal/media/fanart"
	"github.com/sunerpy/pt-tools/internal/media/omdb"
)

// Note 是不算出错的说明（例如 Fanart.tv 要 TVDB 编号、TMDB 上这部没有）：来源返回它时写进 Result.Notes，不写进 Errors。
type Note string

func (n Note) Error() string { return string(n) }

// ---- 豆瓣 ----

// DoubanAPI 是豆瓣适配要用的接口（生产环境是 *douban.Client）。
type DoubanAPI interface {
	Find(ctx context.Context, q douban.Query) (*douban.Subject, error)
	Subject(ctx context.Context, kind, id string) (*douban.Subject, error)
}

// Douban 是豆瓣来源：电影、剧集每一季的简介与评分。豆瓣上剧集一季是一个条目，整部剧集用第 1 季的。
type Douban struct {
	c DoubanAPI
}

// NewDouban 建豆瓣来源；c 为 nil 时返回 nil。
func NewDouban(c DoubanAPI) *Douban {
	if c == nil {
		return nil
	}
	return &Douban{c: c}
}

// ID 是来源编号。
func (d *Douban) ID() string { return SourceDouban }

// find 在豆瓣上找这部电影或这一季：站点给了豆瓣编号的电影先按编号取，名字与年份都和这次的条目对得上
// （见 douban.Subject.Matches；站点编号填错、或者手动换了条目时对不上）才用；其余按名字与年份找（见 douban.Client.Find）。
func (d *Douban) find(ctx context.Context, q Query, season, year int) (*douban.Subject, error) {
	dq := douban.Query{Kind: q.Kind, Title: q.Title, OriginalTitle: q.OriginalTitle, Year: year, Season: season}
	if q.Kind != KindTV && q.IDs[IDDouban] != "" {
		if subj, err := d.c.Subject(ctx, douban.KindMovie, q.IDs[IDDouban]); err == nil && subj.Matches(dq) {
			return subj, nil
		}
	}
	return d.c.Find(ctx, dq)
}

// Item 取电影或剧集第 1 季的简介与评分。
func (d *Douban) Item(ctx context.Context, q Query) (*Item, error) {
	season := 0
	if q.Kind == KindTV {
		season = 1
	}
	subj, err := d.find(ctx, q, season, q.Year)
	if err != nil || subj == nil {
		return nil, err
	}
	it := &Item{Kind: q.Kind, Plot: subj.Intro}
	it.IDs = setID(it.IDs, IDDouban, subj.ID)
	if subj.Rating > 0 {
		it.Ratings = []Rating{{Name: RatingDouban, Value: subj.Rating, Max: 10, Votes: subj.Votes}}
	}
	return it, nil
}

// Season 取一季的简介：第 1 季的年份用整部的，后面几季用这一季首播的年份（不知道时不看年份）。
func (d *Douban) Season(ctx context.Context, q Query, ref SeasonRef) (*Season, error) {
	if ref.Number < 1 {
		return nil, nil
	}
	year := q.Year
	if ref.Number > 1 {
		year = yearOf(ref.Premiered)
	}
	subj, err := d.find(ctx, q, ref.Number, year)
	if err != nil || subj == nil {
		return nil, err
	}
	s := &Season{Number: ref.Number, Plot: subj.Intro}
	s.IDs = setID(s.IDs, IDDouban, subj.ID)
	return s, nil
}

// yearOf 是 yyyy-mm-dd 里的年份（认不出时为 0）。
func yearOf(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, _ := strconv.Atoi(date[:4])
	return y
}

// ---- Bangumi ----

// BangumiAPI 是 Bangumi 适配要用的接口（生产环境是 *bangumi.Client）。
type BangumiAPI interface {
	Find(ctx context.Context, q bangumi.Query) (*bangumi.Subject, error)
	Episodes(ctx context.Context, id int) ([]bangumi.Episode, error)
}

// Bangumi 是 Bangumi 来源：动画的中文简介、评分与每一集的中文标题。Bangumi 上一季是一个条目，整部剧集用第 1 季的。
// 只用于动画（Query.Anime）。日文的简介不用（含假名的当作日文），用 TMDB 的。
type Bangumi struct {
	c BangumiAPI
}

// NewBangumi 建 Bangumi 来源；c 为 nil 时返回 nil。
func NewBangumi(c BangumiAPI) *Bangumi {
	if c == nil {
		return nil
	}
	return &Bangumi{c: c}
}

// ID 是来源编号。
func (b *Bangumi) ID() string { return SourceBangumi }

// hasKana 报告文字里有没有平假名或片假名（Bangumi 上有的简介是日文的）。
func hasKana(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana) {
			return true
		}
	}
	return false
}

// chinese 是中文的文字：含假名（日文）时为空。
func chinese(s string) string {
	if s = strings.TrimSpace(s); hasKana(s) {
		return ""
	}
	return s
}

// Item 取电影或剧集第 1 季的简介与评分（按首播日期找）。
func (b *Bangumi) Item(ctx context.Context, q Query) (*Item, error) {
	subj, err := b.c.Find(ctx, bangumi.Query{Title: q.Title, OriginalTitle: q.OriginalTitle, Date: q.Premiered})
	if err != nil || subj == nil {
		return nil, err
	}
	it := &Item{Kind: q.Kind, Plot: chinese(subj.Summary)}
	it.IDs = setID(it.IDs, IDBangumi, strconv.Itoa(subj.ID))
	if subj.Score > 0 {
		it.Ratings = []Rating{{Name: RatingBangumi, Value: subj.Score, Max: 10, Votes: subj.Votes}}
	}
	return it, nil
}

// Season 取一季：简介与每一集的中文标题（没有中文标题的集不填，用 TMDB 的）。按这一季在 TMDB 上的首播日期找。
func (b *Bangumi) Season(ctx context.Context, q Query, ref SeasonRef) (*Season, error) {
	if ref.Number < 1 {
		return nil, nil
	}
	date := ref.Premiered
	if date == "" && ref.Number == 1 {
		date = q.Premiered
	}
	if date == "" {
		// 后面几季不知道首播日期时找不准（同名的几季都对得上）
		return nil, nil
	}
	subj, err := b.c.Find(ctx, bangumi.Query{Title: q.Title, OriginalTitle: q.OriginalTitle, Date: date})
	if err != nil || subj == nil {
		return nil, err
	}
	s := &Season{Number: ref.Number, Plot: chinese(subj.Summary)}
	s.IDs = setID(s.IDs, IDBangumi, strconv.Itoa(subj.ID))
	if !ref.Episodes {
		return s, nil
	}
	eps, err := b.c.Episodes(ctx, subj.ID)
	if err != nil {
		// 这一季找到了：和出错一起返回，简介照样用
		return s, err
	}
	for _, e := range eps {
		ep := Episode{Season: ref.Number, Number: e.Ep, Title: chinese(e.NameCN), Plot: chinese(e.Desc), Aired: e.AirDate}
		ep.IDs = setID(ep.IDs, IDBangumi, strconv.Itoa(e.ID))
		s.Episodes = append(s.Episodes, ep)
	}
	return s, nil
}

// ---- Fanart.tv ----

// FanartAPI 是 Fanart.tv 适配要用的接口（生产环境是 *fanart.Client）。
type FanartAPI interface {
	Movie(ctx context.Context, id string) (fanart.Images, error)
	TV(ctx context.Context, tvdbID int) (fanart.Images, error)
	Download(ctx context.Context, url string) ([]byte, error)
}

// Fanart 是 Fanart.tv 来源：海报、背景、高清 Logo、clearart、横幅、横版海报、碟片与季海报。
// 电影按 TMDB 编号找，剧集按 TVDB 编号找（TMDB 的详情里有时才有）。
type Fanart struct {
	c FanartAPI
}

// NewFanart 建 Fanart.tv 来源；c 为 nil 时返回 nil（没填 API Key）。
func NewFanart(c FanartAPI) *Fanart {
	if c == nil {
		return nil
	}
	return &Fanart{c: c}
}

// ID 是来源编号。
func (f *Fanart) ID() string { return SourceFanart }

// Art 取图片：只返回要的种类，季海报只返回要的那几季的。
func (f *Fanart) Art(ctx context.Context, q Query, req ArtRequest) ([]Artwork, error) {
	var imgs fanart.Images
	var err error
	if q.Kind == KindTV {
		tvdb, _ := strconv.Atoi(q.IDs[IDTVDB])
		if tvdb <= 0 {
			return nil, Note("Fanart.tv 按 TVDB 编号找剧集，TMDB 上这部没有 TVDB 编号，图片用 TMDB 的")
		}
		imgs, err = f.c.TV(ctx, tvdb)
	} else {
		id := q.IDs[IDTMDB]
		if id == "" {
			id = q.IDs[IDIMDb]
		}
		if id == "" {
			return nil, Note("没有 TMDB 或 IMDb 编号，Fanart.tv 查不了")
		}
		imgs, err = f.c.Movie(ctx, id)
	}
	if err != nil {
		return nil, err
	}
	var out []Artwork
	for _, kind := range req.Kinds {
		for _, im := range imgs[kind] {
			if kind == ArtSeasonPoster && !slices.Contains(req.Seasons, im.Season) {
				continue
			}
			out = append(out, Artwork{Kind: kind, Season: im.Season, Lang: im.Lang, Likes: float64(im.Likes), Ref: im.URL})
		}
	}
	sortArt(out)
	return out, nil
}

// Download 下载一张 Fanart.tv 图片。
func (f *Fanart) Download(ctx context.Context, a Artwork) ([]byte, error) {
	return f.c.Download(ctx, a.Ref)
}

// ---- OMDb ----

// OMDbAPI 是 OMDb 适配要用的接口（生产环境是 *omdb.Client）。
type OMDbAPI interface {
	ByIMDb(ctx context.Context, imdbID string) (*omdb.Title, error)
}

// OMDb 是 OMDb 来源：按 IMDb 编号取 IMDb 评分、烂番茄、Metacritic 与分级。
type OMDb struct {
	c OMDbAPI
}

// NewOMDb 建 OMDb 来源；c 为 nil 时返回 nil（没填 API Key）。
func NewOMDb(c OMDbAPI) *OMDb {
	if c == nil {
		return nil
	}
	return &OMDb{c: c}
}

// ID 是来源编号。
func (o *OMDb) ID() string { return SourceOMDb }

// Item 取评分与分级。
func (o *OMDb) Item(ctx context.Context, q Query) (*Item, error) {
	t, err := o.c.ByIMDb(ctx, q.IDs[IDIMDb])
	if err != nil || t == nil {
		return nil, err
	}
	it := &Item{Kind: q.Kind, Certification: t.Rated}
	it.IDs = setID(it.IDs, IDIMDb, t.IMDbID)
	if t.IMDbRating > 0 {
		it.Ratings = append(it.Ratings, Rating{Name: RatingIMDb, Value: t.IMDbRating, Max: 10, Votes: t.IMDbVotes})
	}
	if t.RottenTomatoes > 0 {
		it.Ratings = append(it.Ratings, Rating{Name: RatingRottenTomatoes, Value: float64(t.RottenTomatoes), Max: 100})
	}
	if t.Metacritic > 0 {
		it.Ratings = append(it.Ratings, Rating{Name: RatingMetacritic, Value: float64(t.Metacritic), Max: 100})
	}
	return it, nil
}
