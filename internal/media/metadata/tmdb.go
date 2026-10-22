package metadata

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/sunerpy/pt-tools/internal/media/tmdb"
)

// TMDBAPI 是 TMDB 适配要用的接口（生产环境是 *tmdb.Client）。
type TMDBAPI interface {
	Details(ctx context.Context, kind string, id int) (*tmdb.Result, error)
	Extras(ctx context.Context, kind string, id int) (*tmdb.Extras, error)
	Season(ctx context.Context, tvID, number int) (*tmdb.Season, error)
	Images(ctx context.Context, kind string, id int) (*tmdb.Images, error)
	Image(ctx context.Context, path, size string) ([]byte, error)
	ProfileURL(path string) string
}

// TMDB 是 TMDB 来源：电影与剧集的完整信息、每一季与每一集、海报、背景、Logo、季海报与集截图。
// 一次刮削里同一个条目的详情与同一季只取一次（几处同时要时等第一次的结果）。
type TMDB struct {
	c    TMDBAPI
	once memo
}

// memo 记下一次刮削里取过的结果：同一个键只取一次，同时来的等第一次的结果。
type memo struct {
	mu    sync.Mutex
	calls map[string]*memoCall
}

type memoCall struct {
	done chan struct{}
	v    any
	err  error
}

func (m *memo) do(key string, fetch func() (any, error)) (any, error) {
	m.mu.Lock()
	if c, ok := m.calls[key]; ok {
		m.mu.Unlock()
		<-c.done
		return c.v, c.err
	}
	if m.calls == nil {
		m.calls = map[string]*memoCall{}
	}
	c := &memoCall{done: make(chan struct{})}
	m.calls[key] = c
	m.mu.Unlock()
	c.v, c.err = fetch()
	close(c.done)
	return c.v, c.err
}

func (t *TMDB) details(ctx context.Context, kind string, id int) (*tmdb.Result, error) {
	v, err := t.once.do(fmt.Sprintf("details:%s:%d", kind, id), func() (any, error) { return t.c.Details(ctx, kind, id) })
	r, _ := v.(*tmdb.Result)
	return r, err
}

func (t *TMDB) season(ctx context.Context, id, n int) (*tmdb.Season, error) {
	v, err := t.once.do(fmt.Sprintf("season:%d:%d", id, n), func() (any, error) { return t.c.Season(ctx, id, n) })
	s, _ := v.(*tmdb.Season)
	return s, err
}

// NewTMDB 建 TMDB 来源；c 为 nil 时返回 nil（TMDB 不能用）。
func NewTMDB(c TMDBAPI) *TMDB {
	if c == nil {
		return nil
	}
	return &TMDB{c: c}
}

// ID 是来源编号。
func (t *TMDB) ID() string { return SourceTMDB }

// FromTMDB 是识别出的 TMDB 条目对应的基本信息（取不到完整信息时 NFO 写它）。
func FromTMDB(r tmdb.Result) *Item {
	it := &Item{
		Kind: r.MediaType, Title: r.Title, OriginalTitle: r.OriginalTitle, Year: r.Year, Premiered: r.Date, Plot: r.Overview,
		Runtime: r.Runtime, Genres: slices.Clone(r.Genres), Countries: slices.Clone(r.Countries),
	}
	if r.ID > 0 {
		it.IDs = setID(it.IDs, IDTMDB, strconv.Itoa(r.ID))
	}
	it.IDs = setID(it.IDs, IDIMDb, r.IMDbID)
	if r.VoteAverage > 0 {
		it.Ratings = []Rating{{Name: RatingTMDB, Value: r.VoteAverage, Max: 10}}
	}
	return it
}

// QueryFor 是用识别出的 TMDB 条目去别的来源找时的条件（doubanID 是站点给的豆瓣编号，没有时为空）。
func QueryFor(r tmdb.Result, doubanID string) Query {
	q := Query{
		Kind: r.MediaType, Title: r.Title, OriginalTitle: r.OriginalTitle, Year: r.Year, Premiered: r.Date, Anime: r.IsAnimation(),
	}
	if r.ID > 0 {
		q.IDs = setID(q.IDs, IDTMDB, strconv.Itoa(r.ID))
	}
	q.IDs = setID(q.IDs, IDIMDb, r.IMDbID)
	q.IDs = setID(q.IDs, IDDouban, doubanID)
	return q
}

func tmdbID(q Query) (int, error) {
	id, err := strconv.Atoi(q.IDs[IDTMDB])
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("没有 TMDB 编号")
	}
	return id, nil
}

// statusOf 把 TMDB 的剧集状态换成 Kodi 与 Jellyfin 认的写法。
func statusOf(s string) string {
	switch s {
	case "Returning Series", "In Production", "Pilot", "Planned":
		return StatusContinuing
	case "Ended", "Canceled":
		return StatusEnded
	}
	return ""
}

// certificationOf 取美国的分级，没有时取识别语言所在地区的（zh-CN 是 CN），都没有时为空。
func certificationOf(certs map[string]string, lang string) string {
	if c := certs["US"]; c != "" {
		return c
	}
	if _, region, ok := strings.Cut(lang, "-"); ok {
		return certs[strings.ToUpper(region)]
	}
	return ""
}

// Item 取完整信息：详情（多半读缓存）加上演职员、分级、出品公司等（见 tmdb.Extras）。
func (t *TMDB) Item(ctx context.Context, q Query) (*Item, error) {
	id, err := tmdbID(q)
	if err != nil {
		return nil, err
	}
	d, err := t.details(ctx, q.Kind, id)
	if err != nil {
		return nil, err
	}
	x, err := t.c.Extras(ctx, q.Kind, id)
	if err != nil {
		return nil, err
	}
	it := FromTMDB(*d)
	it.Tagline, it.Studios = x.Tagline, slices.Clone(x.Studios)
	it.Certification = certificationOf(x.Certifications, languageOf(t.c))
	if q.Kind == KindTV {
		it.Status = statusOf(x.Status)
	}
	it.Directors, it.Writers = slices.Clone(x.Directors), slices.Clone(x.Writers)
	for _, m := range x.Cast {
		it.Cast = append(it.Cast, Person{Name: m.Name, Role: m.Character, Order: m.Order, Thumb: t.c.ProfileURL(m.ProfilePath)})
	}
	if x.Collection != nil {
		it.Collection = &Collection{TMDBID: x.Collection.ID, Name: x.Collection.Name}
	}
	it.IDs = setID(it.IDs, IDTVDB, x.TVDBIDString())
	if it.ID(IDIMDb) == "" {
		it.IDs = setID(it.IDs, IDIMDb, x.IMDbID)
	}
	for i := range it.Ratings {
		if it.Ratings[i].Name == RatingTMDB {
			it.Ratings[i].Votes = x.VoteCount
		}
	}
	return it, nil
}

// languageOf 是客户端的识别语言（*tmdb.Client 才有；测试里的假客户端没有时为空）。
func languageOf(c TMDBAPI) string {
	if l, ok := c.(interface{ Language() string }); ok {
		return l.Language()
	}
	return ""
}

// Season 取一季（含每一集的标题、简介、播出日期、导演、编剧与评分）。
func (t *TMDB) Season(ctx context.Context, q Query, ref SeasonRef) (*Season, error) {
	id, err := tmdbID(q)
	if err != nil {
		return nil, err
	}
	s, err := t.season(ctx, id, ref.Number)
	if errors.Is(err, tmdb.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := &Season{Number: ref.Number, Title: s.Name, Plot: s.Overview, Premiered: s.AirDate}
	for _, e := range s.Episodes {
		ep := Episode{
			Season: ref.Number, Number: e.Number, Title: e.Name, Plot: e.Overview, Aired: e.AirDate, Runtime: e.Runtime,
			Directors: slices.Clone(e.Directors), Writers: slices.Clone(e.Writers),
		}
		if e.ID > 0 {
			ep.IDs = setID(ep.IDs, IDTMDB, strconv.Itoa(e.ID))
		}
		if e.VoteAverage > 0 {
			ep.Ratings = []Rating{{Name: RatingTMDB, Value: e.VoteAverage, Max: 10, Votes: e.VoteCount}}
		}
		out.Episodes = append(out.Episodes, ep)
	}
	return out, nil
}

// Art 取图片：详情里 TMDB 按识别语言挑的海报与背景（Default）；要 Logo 或选了图片语言时再取图片列表（缓存一周），
// 取不到时只有详情里的那两张，同时返回错误；剧集再取要的那几季的季海报与集截图。
func (t *TMDB) Art(ctx context.Context, q Query, req ArtRequest) ([]Artwork, error) {
	id, err := tmdbID(q)
	if err != nil {
		return nil, err
	}
	want := func(k string) bool { return slices.Contains(req.Kinds, k) }
	var out []Artwork
	d, err := t.details(ctx, q.Kind, id)
	if err != nil {
		return nil, err
	}
	if want(ArtPoster) && d.PosterPath != "" {
		out = append(out, Artwork{Kind: ArtPoster, Default: true, Ref: d.PosterPath})
	}
	if want(ArtFanart) && d.BackdropPath != "" {
		out = append(out, Artwork{Kind: ArtFanart, Default: true, Ref: d.BackdropPath})
	}
	var listErr error
	if want(ArtLogo) || (req.ImageLang != LangAuto && (want(ArtPoster) || want(ArtFanart))) {
		imgs, err := t.c.Images(ctx, q.Kind, id)
		if err != nil {
			listErr = err
		} else {
			add := func(kind string, list []tmdb.Image) {
				if !want(kind) {
					return
				}
				for _, im := range list {
					out = append(out, Artwork{Kind: kind, Lang: im.Lang, Likes: im.Vote, Ref: im.Path})
				}
			}
			add(ArtPoster, imgs.Posters)
			add(ArtFanart, imgs.Backdrops)
			add(ArtLogo, imgs.Logos)
		}
	}
	if q.Kind == KindTV && (want(ArtSeasonPoster) || want(ArtThumb)) {
		for _, n := range req.Seasons {
			s, err := t.season(ctx, id, n)
			if err != nil {
				// 这一季取不到：Resolve 里取季详情时已经写明
				continue
			}
			if want(ArtSeasonPoster) && s.PosterPath != "" {
				out = append(out, Artwork{Kind: ArtSeasonPoster, Season: n, Default: true, Ref: s.PosterPath})
			}
			if want(ArtThumb) {
				for _, e := range s.Episodes {
					if e.StillPath != "" {
						out = append(out, Artwork{Kind: ArtThumb, Season: n, Episode: e.Number, Default: true, Ref: e.StillPath})
					}
				}
			}
		}
	}
	return out, listErr
}

// sizes 是每种图片下载的尺寸。
var sizes = map[string]string{
	ArtPoster: tmdb.SizePoster, ArtSeasonPoster: tmdb.SizePoster, ArtFanart: tmdb.SizeBackdrop, ArtLogo: tmdb.SizeLogo, ArtThumb: tmdb.SizeStill,
}

// Download 下载一张 TMDB 图片。
func (t *TMDB) Download(ctx context.Context, a Artwork) ([]byte, error) {
	size, ok := sizes[a.Kind]
	if !ok {
		return nil, fmt.Errorf("TMDB 没有这种图片: %s", a.Kind)
	}
	return t.c.Image(ctx, a.Ref, size)
}
