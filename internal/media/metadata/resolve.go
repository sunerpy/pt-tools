package metadata

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"
)

// Resolver 按媒体库的数据来源，从几个来源取数据并合在一起。TMDB 是基础：识别出的条目、季与集都以它为准，
// 别的来源只替换设置里选了它的那几项，找不到时用 TMDB 的。
type Resolver struct {
	providers map[string]Provider
}

// NewResolver 用这些来源建一个 Resolver（nil 的跳过）。没有的来源当作不可用：设置里选了它时用 TMDB 的，并写明原因。
func NewResolver(ps ...Provider) *Resolver {
	r := &Resolver{providers: map[string]Provider{}}
	for _, p := range ps {
		if p != nil && !isNil(p) {
			r.providers[p.ID()] = p
		}
	}
	return r
}

// isNil 报告接口里装的是不是空指针（调用方传了 (*Douban)(nil) 一类时也当作没有）。
func isNil(p Provider) bool {
	switch v := p.(type) {
	case *TMDB:
		return v == nil
	case *Douban:
		return v == nil
	case *Bangumi:
		return v == nil
	case *Fanart:
		return v == nil
	case *OMDb:
		return v == nil
	}
	return false
}

// Has 报告有没有这个来源。
func (r *Resolver) Has(id string) bool {
	_, ok := r.providers[id]
	return ok
}

// Need 是这次刮削要写的内容（由要写的文件推出来）。没有要写的 NFO 时不去问只提供文字的来源，没有要写的图片时不取图片。
type Need struct {
	// Item 表示要写电影或剧集的 NFO
	Item bool
	// Seasons 是要写 season.nfo 的季
	Seasons []int
	// Episodes 是要写每一集 NFO 的季（要每一集的标题与简介）
	Episodes []int
	// Art 是要的图片；Kinds 为空时不要
	Art ArtRequest
}

// Result 是合在一起的数据。Notes 是用了 TMDB 代替的说明（没找到、没填 Key 一类），Errors 是来源出错。
type Result struct {
	Item    *Item
	Seasons map[int]*Season
	// Used 是文字的每一项（plot、rating、episode）实际用了哪个来源
	Used   map[string]string
	Notes  []string
	Errors []string

	need      Need
	art       map[string][]Artwork
	artOrder  []string
	imageLang string
	recogLang string
	resolver  *Resolver
}

// Season 返回第 n 季；没有取到时返回只有季号的一季。
func (res *Result) Season(n int) *Season {
	if s := res.Seasons[n]; s != nil {
		return s
	}
	return &Season{Number: n}
}

// Pick 按图片的设置挑一张：kind 是种类，season 与 episode 是季海报与截图的季号、集号。按来源的先后，
// 第一个有这种图的来源里按语言挑（见 choose）。没有时返回 nil。
func (res *Result) Pick(kind string, season, episode int) *Artwork {
	if picks := res.Picks(kind, season, episode); len(picks) > 0 {
		return &picks[0]
	}
	return nil
}

// Picks 是每个有这种图的来源里挑出的一张，按来源的先后排：前一张下载不了时用后一张。
func (res *Result) Picks(kind string, season, episode int) []Artwork {
	var out []Artwork
	for _, src := range res.artOrder {
		var cands []Artwork
		for _, a := range res.art[src] {
			if a.Kind != kind {
				continue
			}
			if (kind == ArtSeasonPoster || kind == ArtThumb) && a.Season != season {
				continue
			}
			if kind == ArtThumb && a.Episode != episode {
				continue
			}
			cands = append(cands, a)
		}
		if a := choose(cands, kind, res.imageLang, res.recogLang); a != nil {
			out = append(out, *a)
		}
	}
	return out
}

// Download 下载 Pick 挑出的一张图。
func (res *Result) Download(ctx context.Context, a Artwork) ([]byte, error) {
	p, ok := res.resolver.providers[a.Source].(ArtSource)
	if !ok {
		return nil, fmt.Errorf("没有来源 %s", a.Source)
	}
	return p.Download(ctx, a)
}

// EpisodeRef 是一集：季号与集号。
type EpisodeRef struct{ Season, Episode int }

// Summary 是这次写的文字每一项实际用了哪些来源，例如「简介 豆瓣，评分 豆瓣，每一集 Bangumi、TMDB」：简介看电影或剧集
// （要写它的 NFO 时）与要写的每一季，评分看电影或剧集，每一集看 eps 这几集的标题与简介（混着用时都列出来）。
func (res *Result) Summary(eps []EpisodeRef) string {
	plot := map[string]bool{}
	if v, ok := res.Used["plot"]; ok && res.need.Item {
		plot[v] = true
	}
	for _, n := range res.need.Seasons {
		if s := res.Seasons[n]; s != nil && s.Plot != "" {
			plot[or(s.From["plot"])] = true
		}
	}
	episode := map[string]bool{}
	for _, ref := range eps {
		e := res.Seasons[ref.Season].Episode(ref.Episode)
		if e == nil {
			continue
		}
		if e.Title != "" {
			episode[or(e.From["title"])] = true
		}
		if e.Plot != "" {
			episode[or(e.From["plot"])] = true
		}
	}
	var parts []string
	if len(plot) > 0 {
		parts = append(parts, "简介 "+sourceList(plot))
	}
	if v, ok := res.Used["rating"]; ok && res.need.Item {
		parts = append(parts, "评分 "+SourceName(v))
	}
	if len(episode) > 0 {
		parts = append(parts, "每一集 "+sourceList(episode))
	}
	return strings.Join(parts, "，")
}

// sourceList 是几个来源的名字：别的来源在前、TMDB 在后。
func sourceList(set map[string]bool) string {
	var names []string
	for _, id := range []string{SourceDouban, SourceBangumi, SourceOMDb, SourceFanart, SourceTMDB} {
		if set[id] {
			names = append(names, SourceName(id))
		}
	}
	return strings.Join(names, "、")
}

// fieldsUsing 是设置里选了这个来源的那几项（说明里用）。
func fieldsUsing(src Sources, source string) string {
	var f []string
	if src.Plot == source {
		f = append(f, "简介")
	}
	if src.Rating == source {
		f = append(f, "评分")
	}
	if src.Episode == source {
		f = append(f, "每一集的标题与简介")
	}
	if src.Artwork == source {
		f = append(f, "图片")
	}
	return strings.Join(f, "与")
}

// applies 报告这个来源能不能用于这个条目：Bangumi 只有动画，OMDb 要有 IMDb 编号。
func applies(source string, q Query) bool {
	switch source {
	case SourceBangumi:
		return q.Anime
	case SourceOMDb:
		return q.IDs[IDIMDb] != ""
	}
	return true
}

// Resolve 取数据并合在一起：base 是识别出的 TMDB 条目（取不到 TMDB 的完整信息时用它）。不会返回错误：
// 来源出错时写进 Errors、用 TMDB 的。
func (r *Resolver) Resolve(ctx context.Context, q Query, base *Item, src Sources, need Need) *Result {
	src = src.ForKind(q.Kind)
	res := &Result{
		Item: cloneItem(base), Seasons: map[int]*Season{}, Used: map[string]string{}, need: need, art: map[string][]Artwork{},
		imageLang: need.Art.ImageLang, recogLang: need.Art.RecogLang, resolver: r,
	}
	q.IDs = maps.Clone(q.IDs)
	var mu sync.Mutex
	note := func(s string) {
		mu.Lock()
		res.Notes = append(res.Notes, s)
		mu.Unlock()
	}
	fail := func(s string) {
		mu.Lock()
		res.Errors = append(res.Errors, s)
		mu.Unlock()
	}
	// down 是出错了的来源：后面的季不再去问
	down := map[string]bool{}
	snaps := map[string]*Item{}
	fetchItem := func(id string) {
		p, _ := r.providers[id].(ItemSource)
		if p == nil {
			return
		}
		it, err := p.Item(ctx, q)
		mu.Lock()
		defer mu.Unlock()
		var n Note
		switch {
		case errors.As(err, &n):
			down[id] = true
			res.Notes = append(res.Notes, string(n))
		case err != nil:
			down[id] = true
			res.Errors = append(res.Errors, unavailable(id, src, err))
		case it == nil && id != SourceTMDB && id != SourceOMDb:
			what := "这部"
			if q.Kind == KindTV {
				what = "第 1 季"
			}
			res.Notes = append(res.Notes, fmt.Sprintf("%s上没找到%s，%s用 TMDB 的", SourceLead(id), what, fieldsUsing(src, id)))
		case it != nil:
			snaps[id] = it
		}
	}

	// 先取 TMDB 的完整信息：别的来源要用它的编号（Fanart.tv 的剧集要 TVDB 编号）
	wantFanart := len(need.Art.Kinds) > 0 && r.Has(SourceFanart) && (src.Artwork == SourceFanart ||
		slices.ContainsFunc(need.Art.Kinds, func(k string) bool { return FanartOnly(k) || k == ArtLogo }))
	if need.Item || (wantFanart && q.Kind == KindTV) {
		fetchItem(SourceTMDB)
		if t := snaps[SourceTMDB]; t != nil {
			q.IDs = mergeIDs(q.IDs, t.IDs)
		}
	}

	seasonSet := sortedUnion(need.Seasons, need.Episodes, need.Art.Seasons)
	tmdbSeasons := map[int]*Season{}
	var wg sync.WaitGroup
	if p, ok := r.providers[SourceTMDB].(SeasonSource); ok && q.Kind == KindTV {
		for _, n := range seasonSet {
			wg.Go(func() {
				s, err := p.Season(ctx, q, SeasonRef{Number: n})
				switch {
				case err != nil:
					fail(fmt.Sprintf("TMDB 上第 %d 季的详情取不到（%v），这一季只写季号与集号", n, err))
					return
				case s == nil:
					note(fmt.Sprintf("TMDB 上没有第 %d 季的详情，这一季只写季号与集号", n))
					return
				}
				mu.Lock()
				tmdbSeasons[n] = s
				mu.Unlock()
			})
		}
	}
	// 只提供文字的来源：设置里选了它、适用于这个条目、这次要写电影或剧集的 NFO
	for _, id := range []string{SourceDouban, SourceBangumi, SourceOMDb} {
		if !src.uses(id) && (id != SourceOMDb || !r.Has(id)) {
			continue
		}
		if !applies(id, q) {
			continue
		}
		if !r.Has(id) {
			note(missing(id, src))
			continue
		}
		if need.Item {
			wg.Go(func() { fetchItem(id) })
		}
	}
	if len(need.Art.Kinds) > 0 {
		res.artOrder = []string{SourceTMDB, SourceFanart}
		if src.Artwork == SourceFanart {
			res.artOrder = []string{SourceFanart, SourceTMDB}
			if !r.Has(SourceFanart) {
				note(missing(SourceFanart, src))
			}
		}
		for _, id := range res.artOrder {
			p, ok := r.providers[id].(ArtSource)
			if !ok || (id == SourceFanart && !wantFanart) {
				continue
			}
			wg.Go(func() {
				arts, err := p.Art(ctx, q, need.Art)
				var n Note
				switch {
				case errors.As(err, &n):
					note(string(n))
				case err != nil:
					fail(artUnavailable(id, err))
				}
				for k := range arts {
					arts[k].Source = id
				}
				mu.Lock()
				res.art[id] = arts
				mu.Unlock()
			})
		}
	}
	wg.Wait()

	// 豆瓣、Bangumi 的每一季：一季是一个条目，按 TMDB 上这一季的首播日期找
	otherSeasons := map[string]map[int]*Season{}
	for _, id := range []string{SourceDouban, SourceBangumi} {
		p, ok := r.providers[id].(SeasonSource)
		if !ok || !applies(id, q) || down[id] {
			continue
		}
		var want []int
		if src.Plot == id {
			want = append(want, need.Seasons...)
		}
		if src.Episode == id {
			want = append(want, need.Episodes...)
		}
		for _, n := range sortedUnion(want) {
			if n < 1 || down[id] {
				// 第 0 季（特别篇）豆瓣与 Bangumi 上没有对应的条目
				continue
			}
			ref := SeasonRef{Number: n, Episodes: src.Episode == id && slices.Contains(need.Episodes, n)}
			if s := tmdbSeasons[n]; s != nil {
				ref.Premiered = s.Premiered
			}
			if n == 1 && ref.Premiered == "" {
				ref.Premiered = q.Premiered
			}
			s, err := p.Season(ctx, q, ref)
			switch {
			case err != nil && s != nil:
				// 这一季找到了、每一集取不到：简介照样用
				down[id] = true
				res.Errors = append(res.Errors, fmt.Sprintf("%s上第 %d 季的每一集取不到（%v），每一集的标题与简介用 TMDB 的", SourceLead(id), n, err))
				if otherSeasons[id] == nil {
					otherSeasons[id] = map[int]*Season{}
				}
				otherSeasons[id][n] = s
			case err != nil:
				down[id] = true
				res.Errors = append(res.Errors, unavailable(id, src, err))
			case s == nil:
				res.Notes = append(res.Notes, fmt.Sprintf("%s上没找到第 %d 季，这一季的%s用 TMDB 的", SourceLead(id), n, fieldsUsing(src, id)))
			default:
				if otherSeasons[id] == nil {
					otherSeasons[id] = map[int]*Season{}
				}
				otherSeasons[id][n] = s
			}
		}
	}

	res.composeItem(snaps, src)
	for _, n := range seasonSet {
		s := tmdbSeasons[n]
		if s == nil {
			s = &Season{Number: n}
		}
		// 别的来源找到的这一季：编号都记下（和电影、剧集一样），简介没用上（比如是日文的）也记
		for _, id := range []string{SourceDouban, SourceBangumi} {
			if o := otherSeasons[id][n]; o != nil {
				s.IDs = mergeIDs(s.IDs, o.IDs)
			}
		}
		if src.Plot != "" && slices.Contains(need.Seasons, n) {
			if o := otherSeasons[src.Plot][n]; o != nil && o.Plot != "" {
				s.Plot = o.Plot
				s.From = setID(s.From, "plot", src.Plot)
			}
		}
		if src.Episode != "" && slices.Contains(need.Episodes, n) {
			if o := otherSeasons[src.Episode][n]; o != nil && mergeEpisodes(s, o, src.Episode) {
				res.Used["episode"] = src.Episode
			}
		}
		res.Seasons[n] = s
	}
	if _, ok := res.Used["episode"]; !ok && len(need.Episodes) > 0 {
		res.Used["episode"] = SourceTMDB
	}
	return res
}

// composeItem 把几个来源的电影或剧集合在一起：TMDB 的完整信息（取不到时是识别出的条目）为底，
// 简介按设置换成别的来源的，评分合在一起、主要的排在最后，编号合在一起。
func (res *Result) composeItem(snaps map[string]*Item, src Sources) {
	if t := snaps[SourceTMDB]; t != nil {
		ids := res.Item.IDs
		res.Item = cloneItem(t)
		res.Item.IDs = mergeIDs(res.Item.IDs, ids)
	}
	for _, id := range []string{SourceDouban, SourceBangumi, SourceOMDb} {
		if s := snaps[id]; s != nil {
			res.Item.IDs = mergeIDs(res.Item.IDs, s.IDs)
		}
	}
	if res.Item.Plot != "" {
		res.Used["plot"] = SourceTMDB
	}
	if src.Plot != "" {
		if s := snaps[src.Plot]; s != nil && s.Plot != "" {
			res.Item.Plot = s.Plot
			res.Used["plot"] = src.Plot
		}
	}
	if res.Item.Certification == "" {
		if s := snaps[SourceOMDb]; s != nil {
			res.Item.Certification = s.Certification
		}
	}
	var used string
	res.Item.Ratings, used = composeRatings(snaps, res.Item.Ratings, src.Rating)
	if used != "" {
		res.Used["rating"] = used
	}
}

// primaryRating 是每个来源的主要评分的名称。
var primaryRating = map[string]string{
	SourceTMDB: RatingTMDB, SourceDouban: RatingDouban, SourceBangumi: RatingBangumi, SourceOMDb: RatingIMDb,
}

// composeRatings 把几个来源的评分合在一起（同名的只留一个）：按 TMDB、IMDb、烂番茄、Metacritic、豆瓣、Bangumi 的顺序，
// 主要的评分挪到最后（Jellyfin 与 Emby 按顺序读，最后一个生效）。主要的是设置里选的来源的；它没有评分时用 TMDB 的，
// TMDB 也没有时依次用 IMDb、豆瓣、Bangumi 的。烂番茄与 Metacritic 是百分制，不当主要的评分。返回主要评分的来源。
func composeRatings(snaps map[string]*Item, tmdbRatings []Rating, want string) ([]Rating, string) {
	all := map[string]Rating{}
	for _, r := range tmdbRatings {
		all[r.Name] = r
	}
	for _, id := range []string{SourceTMDB, SourceOMDb, SourceDouban, SourceBangumi} {
		if s := snaps[id]; s != nil {
			for _, r := range s.Ratings {
				if r.Value > 0 {
					all[r.Name] = r
				}
			}
		}
	}
	order := []string{RatingTMDB, RatingIMDb, RatingRottenTomatoes, RatingMetacritic, RatingDouban, RatingBangumi}
	primary, used := "", ""
	for _, source := range []string{or(want), SourceTMDB, SourceOMDb, SourceDouban, SourceBangumi} {
		if _, ok := all[primaryRating[source]]; ok {
			primary, used = primaryRating[source], source
			break
		}
	}
	var out []Rating
	for _, name := range order {
		if r, ok := all[name]; ok && name != primary {
			out = append(out, r)
		}
	}
	if primary != "" {
		out = append(out, all[primary])
	}
	return out, used
}

// mergeEpisodes 用别的来源（source）的一季（o）补全每一集的标题与简介：那个来源有的就用它的（记进 From），
// TMDB 上没有的集加进来。报告有没有用上。
func mergeEpisodes(s, o *Season, source string) bool {
	used := false
	for _, oe := range o.Episodes {
		e := s.Episode(oe.Number)
		if e == nil {
			s.Episodes = append(s.Episodes, Episode{Season: s.Number, Number: oe.Number})
			e = &s.Episodes[len(s.Episodes)-1]
		}
		if oe.Title != "" {
			e.Title, used = oe.Title, true
			e.From = setID(e.From, "title", source)
		}
		if oe.Plot != "" {
			e.Plot, used = oe.Plot, true
			e.From = setID(e.From, "plot", source)
		}
		if e.Aired == "" {
			e.Aired = oe.Aired
		}
		e.IDs = mergeIDs(e.IDs, oe.IDs)
	}
	slices.SortStableFunc(s.Episodes, func(a, b Episode) int { return a.Number - b.Number })
	return used
}

func sortedUnion(lists ...[]int) []int {
	var out []int
	for _, l := range lists {
		for _, n := range l {
			if !slices.Contains(out, n) {
				out = append(out, n)
			}
		}
	}
	slices.Sort(out)
	return out
}

// unavailable 是来源出错时的说明。
func unavailable(id string, src Sources, err error) string {
	if id == SourceTMDB {
		return fmt.Sprintf("TMDB 的完整信息取不到（%v），NFO 只写识别出的基本信息", err)
	}
	what := fieldsUsing(src, id)
	if id == SourceOMDb && what == "" {
		return fmt.Sprintf("OMDb 取不到（%v），没有写 IMDb 等评分", err)
	}
	return fmt.Sprintf("%s取不到（%v），%s用 TMDB 的", SourceLead(id), err, what)
}

// missing 是设置里选了、却没有这个来源时的说明（Fanart.tv 与 OMDb 要在媒体识别里填 API Key）。
func missing(id string, src Sources) string {
	if id == SourceFanart || id == SourceOMDb {
		return fmt.Sprintf("%s选了 %s，但没有填它的 API Key（在「媒体识别」里填），用 TMDB 的", fieldsUsing(src, id), SourceName(id))
	}
	return fmt.Sprintf("%s选了 %s，但它现在不能用，用 TMDB 的", fieldsUsing(src, id), SourceName(id))
}

// SourceLead 是写在中文前面的来源名：英文名后面空一格（豆瓣不空）。
func SourceLead(id string) string {
	n := SourceName(id)
	if r, _ := utf8.DecodeLastRuneInString(n); r < utf8.RuneSelf {
		return n + " "
	}
	return n
}

// artUnavailable 是图片来源出错时的说明。
func artUnavailable(id string, err error) string {
	if id == SourceTMDB {
		return "取 TMDB 图片列表失败，海报与背景用默认的那张、不写 Logo：" + err.Error()
	}
	return fmt.Sprintf("%s的图片取不到（%v），用 TMDB 的", SourceLead(id), err)
}

// cloneItem 复制一份（切片与编号表各自独立），nil 时返回空的。
func cloneItem(in *Item) *Item {
	if in == nil {
		return &Item{}
	}
	out := *in
	out.IDs = maps.Clone(in.IDs)
	out.Genres, out.Countries, out.Studios = slices.Clone(in.Genres), slices.Clone(in.Countries), slices.Clone(in.Studios)
	out.Ratings, out.Directors, out.Writers, out.Cast = slices.Clone(in.Ratings), slices.Clone(in.Directors), slices.Clone(in.Writers), slices.Clone(in.Cast)
	if in.Collection != nil {
		c := *in.Collection
		out.Collection = &c
	}
	return &out
}
