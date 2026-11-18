package metadata

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/tvdb"
)

// fakeTVDB 是假的 TheTVDB：最后生还者（TVDB 392256）第 1 季三集与一集特别篇。
type fakeTVDB struct {
	mu    sync.Mutex
	calls int
	langs []string
	err   error
	// aired 不为空时替换每一集的播出日期（按集号）
	aired map[int]string
}

func (f *fakeTVDB) Episodes(_ context.Context, id int, lang string) (*tvdb.SeriesEpisodes, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.langs = append(f.langs, lang)
	if f.err != nil {
		return nil, f.err
	}
	if id != 392256 {
		return &tvdb.SeriesEpisodes{SeriesID: id}, nil
	}
	eps := []tvdb.Episode{
		{
			ID: 901, Season: 1, Number: 1, Name: "TheTVDB 的第一集", Overview: "第一集的简介", Aired: "2023-01-15", Image: "https://artworks.thetvdb.com/e1.jpg",
			NameLangs: []string{"zho", "eng"}, OverviewLangs: []string{"zho"},
		},
		{
			ID: 902, Season: 1, Number: 2, Name: "第2集", Overview: "第二集的简介", Aired: "2023-01-22",
			NameLangs: []string{"zho"}, OverviewLangs: []string{"zho"},
		},
		{
			ID: 903, Season: 1, Number: 3, Name: "漫长的岁月", Overview: "TBA", Aired: "2023-01-29", Image: "https://artworks.thetvdb.com/e3.jpg",
			NameLangs: []string{"zho"}, OverviewLangs: []string{"zho"},
		},
		{
			ID: 904, Season: 1, Number: 4, Name: "Please Hold to My Hand", Overview: "English only", Aired: "2023-02-05",
			NameLangs: []string{"eng"}, OverviewLangs: []string{"eng"},
		},
		{ID: 905, Season: 1, Number: 5, Name: "かな混じりの題名", Aired: "2023-02-12", NameLangs: []string{"zho"}},
		{ID: 990, Season: 0, Number: 1, Name: "特别篇", Image: "https://artworks.thetvdb.com/s0.jpg", NameLangs: []string{"zho"}},
	}
	for i := range eps {
		if d, ok := f.aired[eps[i].Number]; ok && eps[i].Season == 1 {
			eps[i].Aired = d
		}
	}
	return &tvdb.SeriesEpisodes{SeriesID: id, OriginalLanguage: "eng", Episodes: eps}, nil
}

func (f *fakeTVDB) Download(_ context.Context, u string) ([]byte, error) {
	return []byte("tvdb:" + u), nil
}

func (f *fakeTVDB) hits() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestTVDBLang(t *testing.T) {
	for in, want := range map[string]string{"zh-CN": "zho", "zh-TW": "zhtw", "zh-HK": "zhtw", "en-US": "eng", "ja-JP": "jpn", "": "eng", "fr-FR": "eng"} {
		assert.Equal(t, want, tvdbLang(in), in)
	}
	assert.Nil(t, NewTVDB(nil, "zh-CN"))
}

// 每一集的文字：要有识别语言的翻译（或者原语言就是它）、像这种语言的字、不是占位的；特别篇不用。
func TestTVDBSeason(t *testing.T) {
	ctx := context.Background()
	f := &fakeTVDB{}
	src := NewTVDB(f, "zh-CN")
	q := QueryFor(tlouResult, "")
	q.IDs = setID(q.IDs, IDTVDB, "392256")
	s, err := src.Season(ctx, q, SeasonRef{Number: 1, Episodes: true})
	require.NoError(t, err)
	require.Len(t, s.Episodes, 5)
	e := func(n int) Episode { return *s.Episode(n) }
	assert.Equal(t, "TheTVDB 的第一集", e(1).Title)
	assert.Equal(t, "第一集的简介", e(1).Plot)
	assert.Equal(t, "901", e(1).IDs[IDTVDB])
	assert.Equal(t, "", e(2).Title, "占位的标题不用")
	assert.Equal(t, "第二集的简介", e(2).Plot)
	assert.Equal(t, "漫长的岁月", e(3).Title)
	assert.Equal(t, "", e(3).Plot, "TBA 不用")
	assert.Equal(t, Episode{Season: 1, Number: 4, Aired: "2023-02-05", IDs: map[string]string{IDTVDB: "904"}}, e(4), "没有中文的翻译")
	assert.Equal(t, "", e(5).Title, "有假名的不当中文")
	_, err = src.Season(ctx, q, SeasonRef{Number: 2})
	require.NoError(t, err)
	assert.Equal(t, 1, f.hits(), "一次刮削里同一部只取一次")
	assert.Equal(t, []string{"zho"}, f.langs)

	none, err := src.Season(ctx, q, SeasonRef{Number: 0})
	require.NoError(t, err)
	assert.Nil(t, none, "特别篇不用")
	none, err = src.Season(ctx, q, SeasonRef{Number: 2})
	require.NoError(t, err)
	assert.Nil(t, none, "没有这一季")

	// 英文：原语言就是英文，没有翻译列表也用；有汉字的不用
	en := NewTVDB(&fakeTVDB{}, "en-US")
	s, err = en.Season(ctx, q, SeasonRef{Number: 1})
	require.NoError(t, err)
	assert.Equal(t, "Please Hold to My Hand", s.Episode(4).Title)
	assert.Equal(t, "", s.Episode(1).Title, "有汉字的不当英文")

	for _, p := range []string{"第 1 集", "第一期", "第12话", "第3話", "第2回", "12", "Episode 3", "EP 4", "ep.5", "E06", "S01E07", "tba", " TBD "} {
		assert.True(t, placeholderTitleRe.MatchString(p), p)
	}
	for _, p := range []string{"第一集的故事", "Episode One", "TBA 之后", "第一集与第二集"} {
		assert.False(t, placeholderTitleRe.MatchString(p), p)
	}
}

func tlouTVDBQuery() Query { return QueryFor(tlouResult, "") }

// 每一集选了 TheTVDB：按 TMDB 详情里的 TVDB 编号找，有文字的集换成 TheTVDB 的，TMDB 上没有的集加进来，记下来源。
func TestResolveTVDBEpisodes(t *testing.T) {
	ctx := context.Background()
	f := &fakeTVDB{}
	r := NewResolver(NewTMDB(&fakeTMDB{}), NewTVDB(f, "zh-CN"))
	src := Sources{Episode: SourceTVDB}
	res := r.Resolve(ctx, tlouTVDBQuery(), FromTMDB(tlouResult), src, Need{Episodes: []int{1}})
	s := res.Season(1)
	assert.Equal(t, "TheTVDB 的第一集", s.Episode(1).Title)
	assert.Equal(t, map[string]string{"title": SourceTVDB, "plot": SourceTVDB}, s.Episode(1).From)
	assert.Equal(t, "感染", s.Episode(2).Title, "占位的标题不用，用 TMDB 的")
	assert.Equal(t, "第二集的简介", s.Episode(2).Plot)
	require.NotNil(t, s.Episode(3), "TMDB 上没有的集用 TheTVDB 的补上")
	assert.Equal(t, "漫长的岁月", s.Episode(3).Title)
	assert.Nil(t, s.Episode(4), "没有文字的集不加进来")
	assert.Equal(t, SourceTVDB, res.Used["episode"])
	assert.Equal(t, "每一集 TheTVDB、TMDB", res.Summary([]EpisodeRef{{1, 2}}))
	assert.Empty(t, res.Notes)
	assert.Empty(t, res.Errors)

	// 没选 TheTVDB：文字不用它的（只有截图是备用的）
	res = r.Resolve(ctx, tlouTVDBQuery(), FromTMDB(tlouResult), Sources{}, Need{Episodes: []int{1}, Art: ArtRequest{Kinds: []string{ArtThumb}, Seasons: []int{1}, Thumbs: []EpisodeRef{{1, 3}}}})
	assert.Equal(t, "当你迷失在黑暗中", res.Season(1).Episode(1).Title)
	assert.Nil(t, res.Season(1).Episode(3))
	assert.Equal(t, SourceTMDB, res.Used["episode"])

	// 电影：不问
	f.calls = 0
	_ = r.Resolve(ctx, movieQuery(), FromTMDB(oppResult), src, Need{Item: true, Art: ArtRequest{Kinds: []string{ArtPoster}}})
	assert.Zero(t, f.hits())
}

// 集截图：TMDB 有的用 TMDB 的（下载失败时换成 TheTVDB 的），TMDB 没有的用 TheTVDB 的；TMDB 都有时不问 TheTVDB。
func TestResolveTVDBThumbs(t *testing.T) {
	ctx := context.Background()
	f := &fakeTVDB{}
	r := NewResolver(NewTMDB(&fakeTMDB{}), NewTVDB(f, "zh-CN"))
	need := Need{Art: ArtRequest{Kinds: []string{ArtThumb}, Seasons: []int{1}, Thumbs: []EpisodeRef{{1, 1}, {1, 3}}}}
	res := r.Resolve(ctx, tlouTVDBQuery(), FromTMDB(tlouResult), Sources{}, need)
	assert.Equal(t, "/e1.jpg", res.Pick(ArtThumb, 1, 1).Ref)
	picks := res.Picks(ArtThumb, 1, 1)
	require.Len(t, picks, 2, "TMDB 的下载不了时用 TheTVDB 的")
	assert.Equal(t, SourceTVDB, picks[1].Source)
	e3 := res.Pick(ArtThumb, 1, 3)
	require.NotNil(t, e3)
	assert.Equal(t, SourceTVDB, e3.Source)
	b, err := res.Download(ctx, *e3)
	require.NoError(t, err)
	assert.Equal(t, "tvdb:https://artworks.thetvdb.com/e3.jpg", string(b))
	assert.Nil(t, res.Pick(ArtThumb, 1, 4), "两边都没有")
	assert.Empty(t, res.Notes)

	f.calls = 0
	need.Art.Thumbs = []EpisodeRef{{1, 1}, {1, 2}}
	res = r.Resolve(ctx, tlouTVDBQuery(), FromTMDB(tlouResult), Sources{}, need)
	assert.Zero(t, f.hits(), "TMDB 每一集都有截图：不问 TheTVDB")
	assert.Len(t, res.Picks(ArtThumb, 1, 1), 1)

	// 特别篇不问
	f.calls = 0
	_ = r.Resolve(ctx, tlouTVDBQuery(), FromTMDB(tlouResult), Sources{}, Need{Art: ArtRequest{Kinds: []string{ArtThumb}, Seasons: []int{0}, Thumbs: []EpisodeRef{{0, 1}}}})
	assert.Zero(t, f.hits())
}

// 播出日期和 TMDB 对不上（差一天以上）：这一季的文字与截图都不用，写明；差一天以内照样用。
func TestResolveTVDBMisaligned(t *testing.T) {
	ctx := context.Background()
	tm := &fakeTMDB{aired: map[int]string{1: "2023-01-15", 2: "2023-01-22"}}
	f := &fakeTVDB{aired: map[int]string{2: "2023-01-29"}}
	r := NewResolver(NewTMDB(tm), NewTVDB(f, "zh-CN"))
	need := Need{Episodes: []int{1}, Art: ArtRequest{Kinds: []string{ArtThumb}, Seasons: []int{1}, Thumbs: []EpisodeRef{{1, 3}}}}
	res := r.Resolve(ctx, tlouTVDBQuery(), FromTMDB(tlouResult), Sources{Episode: SourceTVDB}, need)
	assert.Equal(t, "当你迷失在黑暗中", res.Season(1).Episode(1).Title)
	assert.Nil(t, res.Pick(ArtThumb, 1, 3))
	assert.Contains(t, res.Notes, "TheTVDB 上第 1 季的集号和 TMDB 对不上（第 2 集的播出日期不同），这一季的每一集与集截图用 TMDB 的")

	f.aired = map[int]string{2: "2023-01-23"}
	r = NewResolver(NewTMDB(tm), NewTVDB(f, "zh-CN"))
	res = r.Resolve(ctx, tlouTVDBQuery(), FromTMDB(tlouResult), Sources{Episode: SourceTVDB}, need)
	assert.Equal(t, "TheTVDB 的第一集", res.Season(1).Episode(1).Title, "差一天以内（时区）照样用")
	assert.NotNil(t, res.Pick(ArtThumb, 1, 3))
}

// 没有 TVDB 编号、集太多、出错与没填 Key。
func TestResolveTVDBProblems(t *testing.T) {
	ctx := context.Background()
	epNeed := Need{Episodes: []int{1}}
	thumbNeed := Need{Art: ArtRequest{Kinds: []string{ArtThumb}, Seasons: []int{1}, Thumbs: []EpisodeRef{{1, 3}}}}

	// 没有 TVDB 编号（芙莉莲的 TMDB 详情里没有）
	f := &fakeTVDB{}
	r := NewResolver(NewTMDB(&fakeTMDB{}), NewTVDB(f, "zh-CN"))
	res := r.Resolve(ctx, QueryFor(frierenResult, ""), FromTMDB(frierenResult), Sources{Episode: SourceTVDB}, epNeed)
	assert.Contains(t, res.Notes, "TheTVDB 按 TVDB 编号找剧集，TMDB 上这部没有 TVDB 编号，每一集与集截图用 TMDB 的")
	assert.Zero(t, f.hits())

	// 集太多：什么都不用，写明
	f = &fakeTVDB{err: tvdb.ErrTooMany}
	r = NewResolver(NewTMDB(&fakeTMDB{}), NewTVDB(f, "zh-CN"))
	res = r.Resolve(ctx, tlouTVDBQuery(), FromTMDB(tlouResult), Sources{Episode: SourceTVDB}, Need{Episodes: []int{1, 2}})
	assert.Equal(t, "当你迷失在黑暗中", res.Season(1).Episode(1).Title)
	assert.Contains(t, res.Notes, "TheTVDB 上这部的集太多（超过 20 页），没有用，每一集与集截图用 TMDB 的")
	assert.Empty(t, res.Errors)
	assert.Equal(t, 1, f.hits(), "后面的季不再问")

	// 出错：每一集选了它的写进 Errors；只是备用截图的也写进 Errors（图片的说法）
	f = &fakeTVDB{err: errors.New("HTTP 502")}
	r = NewResolver(NewTMDB(&fakeTMDB{}), NewTVDB(f, "zh-CN"))
	res = r.Resolve(ctx, tlouTVDBQuery(), FromTMDB(tlouResult), Sources{Episode: SourceTVDB}, epNeed)
	assert.Equal(t, []string{"TheTVDB 取不到（HTTP 502），每一集的标题与简介用 TMDB 的"}, res.Errors)
	res = r.Resolve(ctx, tlouTVDBQuery(), FromTMDB(tlouResult), Sources{}, thumbNeed)
	assert.Equal(t, []string{"TheTVDB 的图片取不到（HTTP 502），用 TMDB 的"}, res.Errors)

	// 没填 Key：每一集选了它时写明；只是备用截图时不写
	r = NewResolver(NewTMDB(&fakeTMDB{}))
	res = r.Resolve(ctx, tlouTVDBQuery(), FromTMDB(tlouResult), Sources{Episode: SourceTVDB}, epNeed)
	assert.Equal(t, []string{"每一集的标题与简介选了 TheTVDB，但没有填它的 API Key（在「媒体识别」里填），用 TMDB 的"}, res.Notes)
	res = r.Resolve(ctx, tlouTVDBQuery(), FromTMDB(tlouResult), Sources{}, thumbNeed)
	assert.Empty(t, res.Notes)
}

func TestTVDBSourcesAndNames(t *testing.T) {
	raw, err := Sources{Episode: SourceTVDB}.Normalize()
	require.NoError(t, err)
	assert.Equal(t, Sources{Episode: SourceTVDB}, ParseSources(raw, ""))
	_, err = Sources{Plot: SourceTVDB}.Normalize()
	require.Error(t, err, "简介不能选 TheTVDB")
	assert.Equal(t, "TheTVDB", SourceName(SourceTVDB))
	assert.Equal(t, "TheTVDB ", SourceLead(SourceTVDB))
	assert.False(t, NewResolver((*TVDB)(nil)).Has(SourceTVDB))
}

// TVDB 编号要用到 TheTVDB 时才去 TMDB 取：TMDB 每一集都有截图时不多取一次，取不到也不算出错；要用 TheTVDB、
// TMDB 的完整信息取不到时写明原因（不说「没有 TVDB 编号」）。
func TestResolveTVDBFetchesItemOnlyWhenNeeded(t *testing.T) {
	ctx := context.Background()
	tm := &fakeTMDB{extrasErr: errors.New("HTTP 503")}
	f := &fakeTVDB{}
	r := NewResolver(NewTMDB(tm), NewTVDB(f, "zh-CN"))
	need := Need{Art: ArtRequest{Kinds: []string{ArtThumb}, Seasons: []int{1}, Thumbs: []EpisodeRef{{1, 1}, {1, 2}}}}
	res := r.Resolve(ctx, tlouTVDBQuery(), FromTMDB(tlouResult), Sources{}, need)
	assert.Empty(t, res.Errors, "TMDB 每一集都有截图：不取完整信息")
	assert.Zero(t, tm.calls["extras"])
	assert.Zero(t, f.hits())

	need.Art.Thumbs = []EpisodeRef{{1, 3}}
	res = r.Resolve(ctx, tlouTVDBQuery(), FromTMDB(tlouResult), Sources{}, need)
	assert.Equal(t, []string{"TMDB 的完整信息取不到（HTTP 503），不知道 TVDB 编号，每一集与集截图没有用 TheTVDB"}, res.Errors)
	assert.Empty(t, res.Notes)
	assert.Zero(t, f.hits())

	// 要写 tvshow.nfo、完整信息取不到：TMDB 的错误写过了，不问 TheTVDB、不再写一条
	res = r.Resolve(ctx, tlouTVDBQuery(), FromTMDB(tlouResult), Sources{Episode: SourceTVDB}, Need{Item: true, Episodes: []int{1}})
	require.Len(t, res.Errors, 1)
	assert.Contains(t, res.Errors[0], "TMDB 的完整信息取不到")
	assert.Empty(t, res.Notes)
	assert.Zero(t, f.hits())
}
