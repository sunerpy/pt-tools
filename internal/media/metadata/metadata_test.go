package metadata

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/bangumi"
	"github.com/sunerpy/pt-tools/internal/media/douban"
	"github.com/sunerpy/pt-tools/internal/media/fanart"
	"github.com/sunerpy/pt-tools/internal/media/omdb"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
)

// fakeTMDB 是假的 TMDB：奥本海默（电影）与最后生还者（剧集，第 1 季两集），葬送的芙莉莲（动画剧集）。
type fakeTMDB struct {
	mu        sync.Mutex
	extrasErr error
	imagesErr error
	calls     map[string]int
}

func (f *fakeTMDB) hit(k string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[k]++
}

var (
	oppResult = tmdb.Result{
		ID: 872585, MediaType: tmdb.KindMovie, Title: "奥本海默", OriginalTitle: "Oppenheimer", Year: 2023, Date: "2023-07-19",
		Overview: "TMDB 的简介", PosterPath: "/opp.jpg", BackdropPath: "/opp-bd.jpg", VoteAverage: 8.1, IMDbID: "tt15398776",
		Genres: []string{"剧情"}, Runtime: 181, Countries: []string{"US"},
	}
	tlouResult = tmdb.Result{
		ID: 100088, MediaType: tmdb.KindTV, Title: "最后生还者", OriginalTitle: "The Last of Us", Year: 2023, Date: "2023-01-15",
		Overview: "末日之后", PosterPath: "/tlou.jpg", VoteAverage: 8.6,
	}
	frierenResult = tmdb.Result{
		ID: 209867, MediaType: tmdb.KindTV, Title: "葬送的芙莉莲", OriginalTitle: "葬送のフリーレン", Year: 2023, Date: "2023-09-29",
		Overview: "TMDB：芙莉莲", GenreIDs: []int{16},
	}
)

func (f *fakeTMDB) Details(_ context.Context, kind string, id int) (*tmdb.Result, error) {
	f.hit("details")
	for _, r := range []tmdb.Result{oppResult, tlouResult, frierenResult} {
		if r.ID == id && r.MediaType == kind {
			out := r
			return &out, nil
		}
	}
	return nil, tmdb.ErrNotFound
}

func (f *fakeTMDB) Extras(_ context.Context, kind string, id int) (*tmdb.Extras, error) {
	f.hit("extras")
	if f.extrasErr != nil {
		return nil, f.extrasErr
	}
	switch id {
	case 872585:
		return &tmdb.Extras{
			Tagline: "宣传语", VoteCount: 9000, Studios: []string{"Syncopy"}, Certifications: map[string]string{"US": "R", "CN": "x"},
			Collection: &tmdb.Collection{ID: 1, Name: "系列"}, Directors: []string{"Christopher Nolan"}, Writers: []string{"Kai Bird"},
			Cast: []tmdb.CastMember{{Name: "Cillian Murphy", Character: "Oppenheimer", Order: 0, ProfilePath: "/c.jpg"}},
		}, nil
	case 100088:
		return &tmdb.Extras{Status: "Returning Series", TVDBID: 392256, IMDbID: "tt3581920", Studios: []string{"HBO"}}, nil
	}
	return &tmdb.Extras{Status: "Ended"}, nil
}

func (f *fakeTMDB) Season(_ context.Context, tvID, n int) (*tmdb.Season, error) {
	f.hit("season")
	switch {
	case tvID == 100088 && n == 1:
		return &tmdb.Season{Number: 1, Name: "第 1 季", Overview: "TMDB 第一季", AirDate: "2023-01-15", PosterPath: "/s1.jpg", Episodes: []tmdb.Episode{
			{ID: 11, Number: 1, Name: "当你迷失在黑暗中", StillPath: "/e1.jpg", VoteAverage: 8.2, VoteCount: 10, Directors: []string{"D"}},
			{ID: 12, Number: 2, Name: "感染", StillPath: "/e2.jpg"},
		}}, nil
	case tvID == 100088 && n == 2:
		return &tmdb.Season{Number: 2, AirDate: "2025-04-13"}, nil
	case tvID == 209867 && n == 1:
		return &tmdb.Season{Number: 1, AirDate: "2023-09-29", Episodes: []tmdb.Episode{{ID: 21, Number: 1, Name: "第 1 集", Overview: "TMDB 第一集"}}}, nil
	}
	return nil, tmdb.ErrNotFound
}

func (f *fakeTMDB) Images(_ context.Context, _ string, _ int) (*tmdb.Images, error) {
	f.hit("images")
	if f.imagesErr != nil {
		return nil, f.imagesErr
	}
	return &tmdb.Images{
		Posters: []tmdb.Image{{Path: "/p-en.jpg", Lang: "en", Vote: 7}, {Path: "/p-zh.jpg", Lang: "zh", Vote: 6}},
		Logos:   []tmdb.Image{{Path: "/logo-en.png", Lang: "en", Vote: 5}},
	}, nil
}

func (f *fakeTMDB) Image(_ context.Context, path, size string) ([]byte, error) {
	return []byte(size + path), nil
}

func (f *fakeTMDB) ProfileURL(path string) string { return "https://img/w185" + path }

func (f *fakeTMDB) Language() string { return "zh-CN" }

type fakeDouban struct {
	mu    sync.Mutex
	finds []douban.Query
	err   error
}

func (f *fakeDouban) Find(_ context.Context, q douban.Query) (*douban.Subject, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finds = append(f.finds, q)
	if f.err != nil {
		return nil, f.err
	}
	switch {
	case q.Kind == douban.KindMovie && q.Title == "奥本海默":
		return &douban.Subject{ID: "35593344", Intro: "豆瓣：原子弹之父", Rating: 8.8, Votes: 850981}, nil
	case q.Kind == douban.KindTV && q.Title == "最后生还者" && q.Season <= 1:
		return &douban.Subject{ID: "25848328", Intro: "豆瓣：第一季", Rating: 9.0, Votes: 1000}, nil
	}
	return nil, nil
}

func (f *fakeDouban) Subject(_ context.Context, _, id string) (*douban.Subject, error) {
	return &douban.Subject{ID: id, Title: "奥本海默", Year: 2023, Intro: "站点编号取到的"}, nil
}

type fakeBangumi struct {
	finds  []bangumi.Query
	epErr  error
	epHits int
}

func (f *fakeBangumi) Find(_ context.Context, q bangumi.Query) (*bangumi.Subject, error) {
	f.finds = append(f.finds, q)
	if q.OriginalTitle == "葬送のフリーレン" && q.Date == "2023-09-29" {
		return &bangumi.Subject{ID: 400602, Summary: "Bangumi：芙莉莲", Score: 8.5, Votes: 36689}, nil
	}
	return nil, nil
}

func (f *fakeBangumi) Episodes(_ context.Context, _ int) ([]bangumi.Episode, error) {
	f.epHits++
	if f.epErr != nil {
		return nil, f.epErr
	}
	return []bangumi.Episode{
		{ID: 1, Ep: 1, NameCN: "冒险结束", Desc: "魔王を倒し"},
		{ID: 2, Ep: 2, NameCN: "", Desc: "中文的简介"},
	}, nil
}

type fakeFanart struct{ calls int }

func (f *fakeFanart) Movie(_ context.Context, id string) (fanart.Images, error) {
	f.calls++
	if id != "872585" {
		return fanart.Images{}, nil
	}
	return fanart.Images{
		"logo":     {{URL: "https://assets.fanart.tv/logo-zh.png", Lang: "zh", Likes: 3}, {URL: "https://assets.fanart.tv/logo-en.png", Lang: "en", Likes: 9}},
		"clearart": {{URL: "https://assets.fanart.tv/clearart.png", Lang: "", Likes: 1}},
		"poster":   {{URL: "https://assets.fanart.tv/poster-en.jpg", Lang: "en", Likes: 2}},
	}, nil
}

func (f *fakeFanart) TV(_ context.Context, tvdb int) (fanart.Images, error) {
	f.calls++
	return fanart.Images{"season_poster": {{URL: "https://assets.fanart.tv/s1.jpg", Lang: "en", Season: 1}, {URL: "https://assets.fanart.tv/s2.jpg", Season: 2}}}, nil
}

func (f *fakeFanart) Download(_ context.Context, u string) ([]byte, error) { return []byte(u), nil }

type fakeOMDb struct{ err error }

func (f *fakeOMDb) ByIMDb(_ context.Context, id string) (*omdb.Title, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &omdb.Title{IMDbID: id, Rated: "R", IMDbRating: 8.3, IMDbVotes: 100, RottenTomatoes: 93, Metacritic: 90}, nil
}

func ratingNames(rs []Rating) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Name)
	}
	return out
}

func movieQuery() Query { return QueryFor(oppResult, "") }

func TestResolveTMDBOnly(t *testing.T) {
	ctx := context.Background()
	r := NewResolver(NewTMDB(&fakeTMDB{}), NewDouban(nil), nil)
	res := r.Resolve(ctx, movieQuery(), FromTMDB(oppResult), Sources{}, Need{Item: true})
	it := res.Item
	assert.Equal(t, "TMDB 的简介", it.Plot)
	assert.Equal(t, "宣传语", it.Tagline)
	assert.Equal(t, "R", it.Certification, "美国的分级优先")
	assert.Equal(t, []string{"Syncopy"}, it.Studios)
	assert.Equal(t, &Collection{TMDBID: 1, Name: "系列"}, it.Collection)
	require.Len(t, it.Cast, 1)
	assert.Equal(t, "https://img/w185/c.jpg", it.Cast[0].Thumb)
	assert.Equal(t, []Rating{{Name: RatingTMDB, Value: 8.1, Max: 10, Votes: 9000}}, it.Ratings)
	assert.Equal(t, map[string]string{"tmdb": "872585", "imdb": "tt15398776"}, it.IDs)
	assert.Equal(t, map[string]string{"plot": SourceTMDB, "rating": SourceTMDB}, res.Used)
	assert.Empty(t, res.Notes)
	assert.Empty(t, res.Errors)
	assert.Equal(t, "简介 TMDB，评分 TMDB", res.Summary(nil))

	// 取不到完整信息：用识别出的条目，写明原因
	f := &fakeTMDB{extrasErr: errors.New("boom")}
	res = NewResolver(NewTMDB(f)).Resolve(ctx, movieQuery(), FromTMDB(oppResult), Sources{}, Need{Item: true})
	assert.Equal(t, "TMDB 的简介", res.Item.Plot)
	assert.Empty(t, res.Item.Tagline)
	require.Len(t, res.Errors, 1)
	assert.Contains(t, res.Errors[0], "TMDB 的完整信息取不到")
}

func TestResolveDouban(t *testing.T) {
	ctx := context.Background()
	d := &fakeDouban{}
	r := NewResolver(NewTMDB(&fakeTMDB{}), NewDouban(d))
	src := ParseSources("", LegacyDouban)
	assert.Equal(t, Sources{Plot: SourceDouban, Rating: SourceDouban}, src, "旧版本的豆瓣是简介与评分都用豆瓣")
	res := r.Resolve(ctx, movieQuery(), FromTMDB(oppResult), src, Need{Item: true})
	assert.Equal(t, "豆瓣：原子弹之父", res.Item.Plot)
	assert.Equal(t, []string{RatingTMDB, RatingDouban}, ratingNames(res.Item.Ratings), "主要的评分（豆瓣）排在最后")
	assert.Equal(t, "35593344", res.Item.ID(IDDouban))
	assert.Equal(t, map[string]string{"plot": SourceDouban, "rating": SourceDouban}, res.Used)

	// 只有简介用豆瓣：评分的主要来源还是 TMDB，豆瓣的评分也写上
	res = r.Resolve(ctx, movieQuery(), FromTMDB(oppResult), Sources{Plot: SourceDouban}, Need{Item: true})
	assert.Equal(t, []string{RatingDouban, RatingTMDB}, ratingNames(res.Item.Ratings))

	// 站点给的豆瓣编号：名字与年份对得上才用
	res = r.Resolve(ctx, QueryFor(oppResult, "1"), FromTMDB(oppResult), src, Need{Item: true})
	assert.Equal(t, "站点编号取到的", res.Item.Plot)

	// 没有要写的 NFO 时不去找
	d.finds = nil
	_ = r.Resolve(ctx, movieQuery(), FromTMDB(oppResult), src, Need{Art: ArtRequest{Kinds: []string{ArtPoster}}})
	assert.Empty(t, d.finds)

	// 豆瓣出错：用 TMDB 的，后面的季不再问
	d.err = errors.New("HTTP 403")
	q := QueryFor(tlouResult, "")
	res = r.Resolve(ctx, q, FromTMDB(tlouResult), src, Need{Item: true, Seasons: []int{1, 2}})
	assert.Equal(t, "末日之后", res.Item.Plot)
	require.Len(t, res.Errors, 1)
	assert.Contains(t, res.Errors[0], "豆瓣取不到（HTTP 403），简介与评分用 TMDB 的")
	assert.Len(t, d.finds, 1, "出错以后不再找每一季")

	// 剧集：第 1 季找到了（tvshow.nfo 与 season.nfo），第 2 季没有，写明
	d.err, d.finds = nil, nil
	res = r.Resolve(ctx, q, FromTMDB(tlouResult), src, Need{Item: true, Seasons: []int{0, 1, 2}})
	assert.Equal(t, "豆瓣：第一季", res.Item.Plot)
	assert.Equal(t, "豆瓣：第一季", res.Season(1).Plot)
	assert.Equal(t, "25848328", res.Season(1).IDs[IDDouban])
	assert.Equal(t, "", res.Season(2).Plot, "第 2 季 TMDB 上也没有")
	assert.Contains(t, res.Notes, "豆瓣上没找到第 2 季，这一季的简介与评分用 TMDB 的")
	assert.Equal(t, "Fanart.tv 的图片取不到（HTTP 401），用 TMDB 的", artUnavailable(SourceFanart, errors.New("HTTP 401")))
	assert.Equal(t, "豆瓣取不到（HTTP 403），简介用 TMDB 的", unavailable(SourceDouban, Sources{Plot: SourceDouban}, errors.New("HTTP 403")))
	for _, f := range d.finds {
		assert.NotEqual(t, 0, f.Season, "第 0 季不找")
		if f.Season == 2 {
			assert.Equal(t, 2025, f.Year, "第 2 季用这一季首播的年份")
		}
	}
	assert.Equal(t, "392256", res.Item.ID(IDTVDB))
	assert.Equal(t, StatusContinuing, res.Item.Status)
}

func TestResolveOMDb(t *testing.T) {
	ctx := context.Background()
	f := &fakeTMDB{}
	r := NewResolver(NewTMDB(f), NewOMDb(&fakeOMDb{}))
	res := r.Resolve(ctx, movieQuery(), FromTMDB(oppResult), Sources{}, Need{Item: true})
	assert.Equal(t, []string{RatingIMDb, RatingRottenTomatoes, RatingMetacritic, RatingTMDB}, ratingNames(res.Item.Ratings),
		"填了 OMDb 的 Key 就写上 IMDb 等评分，主要的还是 TMDB")
	res = r.Resolve(ctx, movieQuery(), FromTMDB(oppResult), Sources{Rating: SourceOMDb}, Need{Item: true})
	assert.Equal(t, []string{RatingTMDB, RatingRottenTomatoes, RatingMetacritic, RatingIMDb}, ratingNames(res.Item.Ratings))
	assert.Equal(t, SourceOMDb, res.Used["rating"])

	// TMDB 没有分级时用 OMDb 的
	f.extrasErr = errors.New("x")
	res = r.Resolve(ctx, movieQuery(), FromTMDB(oppResult), Sources{}, Need{Item: true})
	assert.Equal(t, "R", res.Item.Certification)

	// 没填 OMDb 的 Key 却选了 IMDb：写明，用 TMDB 的
	res = NewResolver(NewTMDB(&fakeTMDB{})).Resolve(ctx, movieQuery(), FromTMDB(oppResult), Sources{Rating: SourceOMDb}, Need{Item: true})
	assert.Equal(t, SourceTMDB, res.Used["rating"])
	require.Len(t, res.Notes, 1)
	assert.Contains(t, res.Notes[0], "评分选了 OMDb，但没有填它的 API Key")

	// OMDb 出错：只写明没有 IMDb 等评分
	res = NewResolver(NewTMDB(&fakeTMDB{}), NewOMDb(&fakeOMDb{err: omdb.ErrRateLimited})).Resolve(ctx, movieQuery(), FromTMDB(oppResult), Sources{}, Need{Item: true})
	require.Len(t, res.Errors, 1)
	assert.Contains(t, res.Errors[0], "没有写 IMDb 等评分")
}

func TestResolveBangumi(t *testing.T) {
	ctx := context.Background()
	b := &fakeBangumi{}
	r := NewResolver(NewTMDB(&fakeTMDB{}), NewBangumi(b))
	src := Sources{Plot: SourceBangumi, Rating: SourceBangumi, Episode: SourceBangumi}
	q := QueryFor(frierenResult, "")
	require.True(t, q.Anime)
	res := r.Resolve(ctx, q, FromTMDB(frierenResult), src, Need{Item: true, Seasons: []int{1}, Episodes: []int{1}})
	assert.Equal(t, "Bangumi：芙莉莲", res.Item.Plot)
	assert.Equal(t, "Bangumi：芙莉莲", res.Season(1).Plot)
	assert.Equal(t, RatingBangumi, res.Item.Ratings[len(res.Item.Ratings)-1].Name)
	e1, e2 := res.Season(1).Episode(1), res.Season(1).Episode(2)
	require.NotNil(t, e1)
	require.NotNil(t, e2, "TMDB 上没有的集用 Bangumi 的")
	assert.Equal(t, "冒险结束", e1.Title)
	assert.Equal(t, "TMDB 第一集", e1.Plot, "日文的简介不用")
	assert.Equal(t, "中文的简介", e2.Plot)
	assert.Equal(t, SourceBangumi, res.Used["episode"])
	assert.Equal(t, map[string]string{"title": SourceBangumi}, e1.From, "标题来自 Bangumi，简介还是 TMDB 的")
	assert.Equal(t, map[string]string{"plot": SourceBangumi}, e2.From)
	assert.Equal(t, map[string]string{"plot": SourceBangumi}, res.Season(1).From)
	assert.Equal(t, "简介 Bangumi，评分 Bangumi，每一集 Bangumi、TMDB", res.Summary([]EpisodeRef{{1, 1}}), "这一集混着用：都列出来")
	assert.Equal(t, "简介 Bangumi，评分 Bangumi，每一集 Bangumi", res.Summary([]EpisodeRef{{1, 2}}))
	assert.Equal(t, "400602", res.Item.ID(IDBangumi))

	// 只有每一集用 Bangumi 的：这一季的简介用 TMDB 的，Bangumi 的编号照样记下
	res = r.Resolve(ctx, q, FromTMDB(frierenResult), Sources{Episode: SourceBangumi}, Need{Seasons: []int{1}, Episodes: []int{1}})
	assert.Equal(t, "冒险结束", res.Season(1).Episode(1).Title)
	assert.Empty(t, res.Season(1).Plot, "TMDB 上这一季没有简介，也不用 Bangumi 的")
	assert.Equal(t, "400602", res.Season(1).IDs[IDBangumi])

	// 只有简介用 Bangumi 的：不去取每一集；每一集取不到时，这一季的简介照样用
	b.epHits = 0
	res = r.Resolve(ctx, q, FromTMDB(frierenResult), Sources{Plot: SourceBangumi}, Need{Seasons: []int{1}, Episodes: []int{1}})
	assert.Zero(t, b.epHits, "每一集不用 Bangumi 的：不去取")
	assert.Equal(t, "Bangumi：芙莉莲", res.Season(1).Plot)
	b.epErr = errors.New("HTTP 500")
	res = r.Resolve(ctx, q, FromTMDB(frierenResult), src, Need{Seasons: []int{1}, Episodes: []int{1}})
	assert.Equal(t, "Bangumi：芙莉莲", res.Season(1).Plot, "每一集取不到也用这一季的简介")
	assert.Equal(t, "第 1 集", res.Season(1).Episode(1).Title, "每一集用 TMDB 的")
	assert.Contains(t, res.Errors, "Bangumi 上第 1 季的每一集取不到（HTTP 500），每一集的标题与简介用 TMDB 的")
	b.epErr = nil

	// 不是动画：不去 Bangumi 找，也不写说明
	b.finds = nil
	res = r.Resolve(ctx, movieQuery(), FromTMDB(oppResult), src, Need{Item: true})
	assert.Empty(t, b.finds)
	assert.Empty(t, res.Notes)
	assert.Equal(t, SourceTMDB, res.Used["plot"])
}

func TestResolveArt(t *testing.T) {
	ctx := context.Background()
	fa := &fakeFanart{}
	r := NewResolver(NewTMDB(&fakeTMDB{}), NewFanart(fa))
	kinds := []string{ArtPoster, ArtFanart, ArtLogo, ArtClearart}
	need := Need{Art: ArtRequest{Kinds: kinds, RecogLang: "zh-CN"}}
	res := r.Resolve(ctx, movieQuery(), FromTMDB(oppResult), Sources{}, need)
	assert.Equal(t, "/opp.jpg", res.Pick(ArtPoster, 0, 0).Ref, "默认用 TMDB 按识别语言给的海报")
	assert.Equal(t, "/opp-bd.jpg", res.Pick(ArtFanart, 0, 0).Ref)
	assert.Equal(t, "/logo-en.png", res.Pick(ArtLogo, 0, 0).Ref, "TMDB 有 Logo 时先用 TMDB 的")
	clear := res.Pick(ArtClearart, 0, 0)
	require.NotNil(t, clear, "clearart 只有 Fanart.tv 有")
	assert.Equal(t, SourceFanart, clear.Source)
	b, err := res.Download(ctx, *clear)
	require.NoError(t, err)
	assert.Equal(t, "https://assets.fanart.tv/clearart.png", string(b))
	b, err = res.Download(ctx, *res.Pick(ArtPoster, 0, 0))
	require.NoError(t, err)
	assert.Equal(t, "w780/opp.jpg", string(b))

	// 图片选了 Fanart.tv：Logo 按识别语言挑 Fanart.tv 的，海报用 Fanart.tv 的
	res = r.Resolve(ctx, movieQuery(), FromTMDB(oppResult), Sources{Artwork: SourceFanart}, need)
	assert.Equal(t, "https://assets.fanart.tv/logo-zh.png", res.Pick(ArtLogo, 0, 0).Ref)
	assert.Equal(t, "https://assets.fanart.tv/poster-en.jpg", res.Pick(ArtPoster, 0, 0).Ref)
	assert.Equal(t, "/opp-bd.jpg", res.Pick(ArtFanart, 0, 0).Ref, "Fanart.tv 没有背景时用 TMDB 的")

	// 选了图片语言：TMDB 的海报优先用这种语言的
	need.Art.ImageLang = "zh"
	res = r.Resolve(ctx, movieQuery(), FromTMDB(oppResult), Sources{}, need)
	assert.Equal(t, "/p-zh.jpg", res.Pick(ArtPoster, 0, 0).Ref)

	// 取不到 TMDB 的图片列表：海报与背景用详情里的，写明原因
	r2 := NewResolver(NewTMDB(&fakeTMDB{imagesErr: errors.New("503")}))
	res = r2.Resolve(ctx, movieQuery(), FromTMDB(oppResult), Sources{}, need)
	assert.Equal(t, "/opp.jpg", res.Pick(ArtPoster, 0, 0).Ref)
	assert.Nil(t, res.Pick(ArtLogo, 0, 0))
	require.Len(t, res.Errors, 1)
	assert.Contains(t, res.Errors[0], "取 TMDB 图片列表失败")

	// 剧集：季海报与集截图；Fanart.tv 的季海报按季号
	tv := Need{Art: ArtRequest{Kinds: []string{ArtSeasonPoster, ArtThumb}, Seasons: []int{1}}}
	res = r.Resolve(ctx, QueryFor(tlouResult, ""), FromTMDB(tlouResult), Sources{Artwork: SourceFanart}, tv)
	assert.Equal(t, "https://assets.fanart.tv/s1.jpg", res.Pick(ArtSeasonPoster, 1, 0).Ref, "TVDB 编号从 TMDB 的详情里来")
	assert.Nil(t, res.Pick(ArtSeasonPoster, 2, 0), "没要第 2 季")
	assert.Equal(t, "/e2.jpg", res.Pick(ArtThumb, 1, 2).Ref)
	assert.Nil(t, res.Pick(ArtThumb, 1, 3))

	// 剧集没有 TVDB 编号：Fanart.tv 查不了，写明
	res = r.Resolve(ctx, QueryFor(frierenResult, ""), FromTMDB(frierenResult), Sources{Artwork: SourceFanart}, tv)
	assert.Contains(t, res.Notes, "Fanart.tv 按 TVDB 编号找剧集，TMDB 上这部没有 TVDB 编号，图片用 TMDB 的")

	// 没填 Fanart.tv 的 Key 却选了它
	res = NewResolver(NewTMDB(&fakeTMDB{})).Resolve(ctx, movieQuery(), FromTMDB(oppResult), Sources{Artwork: SourceFanart}, need)
	require.Len(t, res.Notes, 1)
	assert.Contains(t, res.Notes[0], "图片选了 Fanart.tv，但没有填它的 API Key")
	assert.Equal(t, "/p-zh.jpg", res.Pick(ArtPoster, 0, 0).Ref)

	// 只要海报与背景、没选 Fanart.tv：不去问 Fanart.tv
	fa.calls = 0
	_ = r.Resolve(ctx, movieQuery(), FromTMDB(oppResult), Sources{}, Need{Art: ArtRequest{Kinds: []string{ArtPoster, ArtFanart}}})
	assert.Zero(t, fa.calls)
}

func TestSources(t *testing.T) {
	raw, err := Sources{Plot: SourceDouban, Episode: SourceBangumi}.Normalize()
	require.NoError(t, err)
	assert.Equal(t, Sources{Plot: SourceDouban, Episode: SourceBangumi}, ParseSources(raw, ""))
	assert.Equal(t, Sources{Plot: SourceDouban}, ParseSources(raw, "").ForKind(KindMovie), "电影没有每一集")
	empty, err := Sources{}.Normalize()
	require.NoError(t, err)
	assert.Empty(t, empty)
	_, err = Sources{Plot: SourceFanart}.Normalize()
	assert.ErrorContains(t, err, "简介的来源要选 tmdb、douban、bangumi 之一")
	assert.Equal(t, Sources{}, ParseSources(`{"plot":"x"}`, ""), "设置坏了当作全用 TMDB")
	assert.Equal(t, Sources{}, ParseSources(`{`, LegacyDouban))
	assert.Equal(t, Sources{Plot: SourceDouban}, ParseSources(`{"plot":"douban"}`, LegacyDouban), "新的设置优先")
}

func TestChoose(t *testing.T) {
	cands := []Artwork{{Kind: ArtLogo, Lang: "en", Ref: "en"}, {Kind: ArtLogo, Lang: "ja", Ref: "ja"}, {Kind: ArtLogo, Lang: "", Ref: "none"}}
	assert.Equal(t, "en", choose(cands, ArtLogo, LangAuto, "zh-CN").Ref, "识别语言没有时用英文")
	assert.Equal(t, "ja", choose(cands, ArtLogo, LangAuto, "ja-JP").Ref)
	assert.Equal(t, "none", choose(cands, ArtLogo, LangNone, "zh-CN").Ref)
	assert.Nil(t, choose(nil, ArtLogo, LangAuto, ""))
	posters := []Artwork{{Kind: ArtPoster, Lang: "en", Ref: "en"}, {Kind: ArtPoster, Default: true, Ref: "default"}}
	assert.Equal(t, "default", choose(posters, ArtPoster, LangAuto, "zh-CN").Ref)
	assert.Equal(t, "en", choose(posters, ArtPoster, "en", "zh-CN").Ref)
	assert.Equal(t, "default", choose(posters, ArtPoster, "ja", "zh-CN").Ref, "这种语言没有时用来源挑的那张")
	assert.Equal(t, "en", choose(posters[:1], ArtPoster, LangAuto, "zh-CN").Ref, "没有来源挑的那张时按语言找，最后是第一张")
	fan := []Artwork{{Kind: ArtFanart, Lang: "en", Ref: "en"}, {Kind: ArtFanart, Lang: "", Ref: "none"}}
	assert.Equal(t, "none", choose(fan, ArtFanart, LangAuto, "").Ref, "背景优先没有文字的")
	assert.True(t, FanartOnly(ArtDisc))
	assert.False(t, FanartOnly(ArtPoster))
}
