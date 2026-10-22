package scrape

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/metadata"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"

	"github.com/sunerpy/pt-tools/internal/media/fanart"
)

var update = flag.Bool("update", false, "重写 testdata 里的 golden 文件")

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	p := filepath.Join("testdata", name)
	if *update {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(p, got, 0o644))
	}
	want, err := os.ReadFile(p)
	require.NoError(t, err, "缺 golden 文件时用 -update 生成")
	assert.Equal(t, string(want), string(got))
}

var (
	movie = &metadata.Item{
		Kind: metadata.KindMovie, IDs: map[string]string{"tmdb": "872585", "imdb": "tt15398776"},
		Title: "奥本海默", OriginalTitle: "Oppenheimer", Year: 2023, Premiered: "2023-07-19",
		Plot: "「原子弹之父」罗伯特·奥本海默的故事 & <曼哈顿计划>。", Tagline: "世界从此改变", Runtime: 181, Certification: "R",
		Ratings: []metadata.Rating{
			{Name: metadata.RatingIMDb, Value: 8.3, Max: 10, Votes: 900000},
			{Name: metadata.RatingRottenTomatoes, Value: 93, Max: 100},
			{Name: metadata.RatingTMDB, Value: 8.123, Max: 10, Votes: 9000},
		},
		Genres: []string{"剧情", "历史"}, Studios: []string{"Syncopy", "Universal Pictures"},
		Collection: &metadata.Collection{TMDBID: 1452, Name: "测试系列"},
		Directors:  []string{"Christopher Nolan"}, Writers: []string{"Christopher Nolan", "Kai Bird"},
		Cast: []metadata.Person{
			{Name: "Cillian Murphy", Role: "J. Robert Oppenheimer", Order: 0, Thumb: "https://image.tmdb.org/t/p/w185/c.jpg"},
			{Name: "Emily Blunt", Role: "Kitty Oppenheimer", Order: 1},
		},
	}
	show = &metadata.Item{
		Kind: metadata.KindTV, IDs: map[string]string{"tmdb": "100088", "imdb": "tt3581920", "tvdb": "392256"},
		Title: "最后生还者", OriginalTitle: "The Last of Us", Year: 2023, Premiered: "2023-01-15", Plot: "末日之后……",
		Runtime: 50, Certification: "TV-MA", Status: metadata.StatusContinuing,
		Ratings: []metadata.Rating{{Name: metadata.RatingTMDB, Value: 8.6, Max: 10, Votes: 5000}},
		Genres:  []string{"剧情"}, Studios: []string{"HBO"},
		Cast: []metadata.Person{{Name: "Pedro Pascal", Role: "Joel Miller", Order: 0}},
	}
	season1 = &metadata.Season{
		Number: 1, Title: "第 1 季", Plot: "第一季", Premiered: "2023-01-15",
		Episodes: []metadata.Episode{
			{
				Season: 1, Number: 1, IDs: map[string]string{"tmdb": "2181581"}, Title: "当你迷失在黑暗中", Plot: "……", Aired: "2023-01-15", Runtime: 81,
				Ratings: []metadata.Rating{{Name: metadata.RatingTMDB, Value: 8.2, Max: 10, Votes: 120}}, Directors: []string{"Craig Mazin"},
				Writers: []string{"Craig Mazin", "Neil Druckmann"},
			},
			{Season: 1, Number: 2, IDs: map[string]string{"tmdb": "4071039"}, Title: "感染", Aired: "2023-01-22", Runtime: 53},
		},
	}
)

func TestNFOGolden(t *testing.T) {
	b, err := MovieNFO(movie)
	require.NoError(t, err)
	golden(t, "movie.nfo", b)

	b, err = MovieNFO(&metadata.Item{Kind: metadata.KindMovie, IDs: map[string]string{"tmdb": "1"}, Title: "Same", OriginalTitle: "Same"})
	require.NoError(t, err)
	golden(t, "movie-bare.nfo", b)

	// 豆瓣的评分是主要的：写在最后（Jellyfin 与 Emby 按顺序读每个 <rating>，最后一个生效，不看 default）
	db := &metadata.Item{
		Kind: metadata.KindMovie, IDs: map[string]string{"tmdb": "872585", "imdb": "tt15398776", "douban": "35593344"},
		Title: "奥本海默", OriginalTitle: "Oppenheimer", Year: 2023, Premiered: "2023-07-19", Plot: "豆瓣的简介 & <更多>。", Runtime: 181,
		Ratings: []metadata.Rating{{Name: metadata.RatingTMDB, Value: 8.1, Max: 10}, {Name: metadata.RatingDouban, Value: 8.8, Max: 10, Votes: 850981}},
		Genres:  []string{"剧情", "历史"},
	}
	b, err = MovieNFO(db)
	require.NoError(t, err)
	golden(t, "movie-douban.nfo", b)

	b, err = TVShowNFO(show)
	require.NoError(t, err)
	golden(t, "tvshow.nfo", b)

	b, err = SeasonNFO(season1)
	require.NoError(t, err)
	golden(t, "season.nfo", b)
	b, err = SeasonNFO(&metadata.Season{Number: 0})
	require.NoError(t, err)
	golden(t, "season-bare.nfo", b)

	b, err = EpisodeNFO(show, season1, 1, 2, 0)
	require.NoError(t, err)
	golden(t, "episode.nfo", b)
	b, err = EpisodeNFO(show, season1, 1, 1, 3)
	require.NoError(t, err)
	golden(t, "episode-multi.nfo", b)
	b, err = EpisodeNFO(show, nil, 0, 5, 0)
	require.NoError(t, err)
	assert.Contains(t, string(b), "<title>第 5 集</title>", "季里没有的集用「第 N 集」")
}

// fakeTMDB 是假的 TMDB（metadata.TMDBAPI）：奥本海默与最后生还者（第 1 季两集）。
type fakeTMDB struct {
	mu    sync.Mutex
	calls []string
	fail  map[string]error
	// onDetails 在取详情时调用（模拟取数据的这段时间里文件被改了）
	onDetails func()
}

func (f *fakeTMDB) hit(s string) {
	f.mu.Lock()
	f.calls = append(f.calls, s)
	f.mu.Unlock()
}

func (f *fakeTMDB) called() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeTMDB) Details(_ context.Context, kind string, id int) (*tmdb.Result, error) {
	f.hit("details")
	if f.onDetails != nil {
		f.onDetails()
	}
	if kind == tmdb.KindTV {
		return &tmdb.Result{ID: id, MediaType: kind, Title: "最后生还者", Year: 2023, PosterPath: "/show.jpg", BackdropPath: "/show-bd.jpg"}, nil
	}
	return &tmdb.Result{ID: id, MediaType: kind, Title: "奥本海默", Year: 2023, Overview: "TMDB 的简介", PosterPath: "/poster.jpg", BackdropPath: "/backdrop.jpg", VoteAverage: 8.1}, nil
}

func (f *fakeTMDB) Extras(context.Context, string, int) (*tmdb.Extras, error) {
	f.hit("extras")
	return &tmdb.Extras{Tagline: "宣传语", VoteCount: 10}, nil
}

func (f *fakeTMDB) Season(_ context.Context, _, n int) (*tmdb.Season, error) {
	f.hit(fmt.Sprintf("season%d", n))
	if n != 1 {
		return nil, tmdb.ErrNotFound
	}
	s := &tmdb.Season{Number: 1, Name: "第 1 季", AirDate: "2023-01-15", PosterPath: "/s1.jpg"}
	for i := 1; i <= 12; i++ {
		s.Episodes = append(s.Episodes, tmdb.Episode{ID: 100 + i, Number: i, Name: fmt.Sprintf("E%d", i), StillPath: fmt.Sprintf("/e%d.jpg", i)})
	}
	return s, nil
}

func (f *fakeTMDB) Images(context.Context, string, int) (*tmdb.Images, error) {
	f.hit("images")
	return &tmdb.Images{Logos: []tmdb.Image{{Path: "/logo.png", Lang: "en"}}}, nil
}

func (f *fakeTMDB) Image(_ context.Context, path, size string) ([]byte, error) {
	f.hit(size + path)
	if err := f.fail[path]; err != nil {
		return nil, err
	}
	return []byte("IMG" + path), nil
}

func (f *fakeTMDB) ProfileURL(p string) string { return "https://img/w185" + p }

func listFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	require.NoError(t, filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			rel, _ := filepath.Rel(root, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return err
	}))
	sort.Strings(out)
	return out
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	return string(b)
}

func statuses(rep Report) map[string]string {
	out := map[string]string{}
	for _, f := range rep.Files {
		out[filepath.Base(f.Path)] = f.Status
	}
	return out
}

var (
	defaults   = ParseItems("")
	movieQuery = metadata.Query{Kind: metadata.KindMovie, IDs: map[string]string{"tmdb": "872585"}, Title: "奥本海默", Year: 2023}
	movieBase  = &metadata.Item{Kind: metadata.KindMovie, IDs: map[string]string{"tmdb": "872585"}, Title: "奥本海默", Year: 2023, Plot: "识别出的简介"}
	showQuery  = metadata.Query{Kind: metadata.KindTV, IDs: map[string]string{"tmdb": "100088"}, Title: "最后生还者", Year: 2023}
	showBase   = &metadata.Item{Kind: metadata.KindTV, IDs: map[string]string{"tmdb": "100088"}, Title: "最后生还者", Year: 2023}
)

func writer(f *fakeTMDB) Writer {
	return Writer{Resolver: metadata.NewResolver(metadata.NewTMDB(f))}
}

func TestWriteMovie(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	f := &fakeTMDB{}
	w := writer(f)
	video := filepath.Join(root, "奥本海默 (2023)", "奥本海默 (2023) - 2160p.mkv")
	files := MovieFiles(video, true, defaults)
	rep := w.Write(ctx, movieQuery, movieBase, files)
	assert.Empty(t, rep.Problems())
	assert.Len(t, rep.Written(), 3)
	assert.Equal(t, []string{
		"奥本海默 (2023)/fanart.jpg",
		"奥本海默 (2023)/poster.jpg",
		"奥本海默 (2023)/奥本海默 (2023) - 2160p.nfo",
	}, listFiles(t, root))
	nfo := read(t, filepath.Join(root, "奥本海默 (2023)", "奥本海默 (2023) - 2160p.nfo"))
	assert.Contains(t, nfo, "<plot>TMDB 的简介</plot>", "用 TMDB 的完整信息")
	assert.Contains(t, nfo, "<tagline>宣传语</tagline>")
	assert.Equal(t, "简介 TMDB，评分 TMDB", rep.Sources)
	assert.Equal(t, "IMG/poster.jpg", read(t, filepath.Join(root, "奥本海默 (2023)", "poster.jpg")))

	// 不覆盖：已有的文件不动，也不去取数据
	f.calls = nil
	require.NoError(t, os.WriteFile(filepath.Join(root, "奥本海默 (2023)", "poster.jpg"), []byte("mine"), 0o644))
	rep = w.Write(ctx, movieQuery, movieBase, files)
	assert.Empty(t, rep.Written())
	assert.Equal(t, map[string]string{"poster.jpg": FileKept, "fanart.jpg": FileKept, "奥本海默 (2023) - 2160p.nfo": FileKept}, statuses(rep))
	assert.Empty(t, f.called(), "都有了：不取数据")
	assert.Equal(t, "mine", read(t, filepath.Join(root, "奥本海默 (2023)", "poster.jpg")))

	// 覆盖：内容一样的不重写
	w.Overwrite = true
	rep = w.Write(ctx, movieQuery, movieBase, files)
	assert.Equal(t, map[string]string{"poster.jpg": FileWritten, "fanart.jpg": FileUnchanged, "奥本海默 (2023) - 2160p.nfo": FileUnchanged}, statuses(rep))
	assert.Equal(t, "IMG/poster.jpg", read(t, filepath.Join(root, "奥本海默 (2023)", "poster.jpg")))

	// 不覆盖、Replace 认可的（pt-tools 自己写的）照样换
	require.NoError(t, os.WriteFile(filepath.Join(root, "奥本海默 (2023)", "poster.jpg"), []byte("old"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "奥本海默 (2023)", "fanart.jpg"), []byte("user"), 0o644))
	w.Overwrite = false
	w.Replace = func(p string) bool { return filepath.Base(p) == "poster.jpg" }
	rep = w.Write(ctx, movieQuery, movieBase, files)
	assert.Equal(t, map[string]string{"poster.jpg": FileWritten, "fanart.jpg": FileKept, "奥本海默 (2023) - 2160p.nfo": FileKept}, statuses(rep))
	assert.Equal(t, "user", read(t, filepath.Join(root, "奥本海默 (2023)", "fanart.jpg")))

	// 放在库目录根下：图片带上文件名
	flat := t.TempDir()
	rep = writer(&fakeTMDB{}).Write(ctx, movieQuery, movieBase, MovieFiles(filepath.Join(flat, "Movie.mkv"), false, defaults))
	assert.Empty(t, rep.Problems())
	assert.Equal(t, []string{"Movie-fanart.jpg", "Movie-poster.jpg", "Movie.nfo"}, listFiles(t, flat))

	// 图片下载失败只记错误，NFO 照写
	bad := t.TempDir()
	rep = writer(&fakeTMDB{fail: map[string]error{"/backdrop.jpg": errors.New("boom")}}).Write(ctx, movieQuery, movieBase, MovieFiles(filepath.Join(bad, "M", "M.mkv"), true, defaults))
	assert.Equal(t, []string{"fanart.jpg: boom"}, rep.Problems())
	assert.Equal(t, []string{"M/M.nfo", "M/poster.jpg"}, listFiles(t, bad))

	// 取不到 TMDB 时 NFO 写识别出的基本信息，没有图片
	none := t.TempDir()
	rep = Writer{Resolver: metadata.NewResolver()}.Write(ctx, movieQuery, movieBase, MovieFiles(filepath.Join(none, "M", "M.mkv"), true, defaults))
	assert.Empty(t, rep.Problems())
	assert.Equal(t, []string{"M/M.nfo"}, listFiles(t, none))
	assert.Contains(t, read(t, filepath.Join(none, "M", "M.nfo")), "<plot>识别出的简介</plot>")
	assert.Equal(t, FileMissing, statuses(rep)["poster.jpg"])

	left, _ := filepath.Glob(filepath.Join(root, "*", "*.part"))
	assert.Empty(t, left, "不留临时文件")
}

func TestWriteTV(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	f := &fakeTMDB{}
	showDir := filepath.Join(root, "最后生还者 (2023)")
	seasonDir := filepath.Join(showDir, "Season 1")
	var files []File
	files = append(files, ShowFiles(showDir, defaults)...)
	files = append(files, SeasonFiles(showDir, seasonDir, 1, defaults)...)
	for n := 1; n <= 12; n++ {
		files = append(files, EpisodeFiles(filepath.Join(seasonDir, fmt.Sprintf("S01E%02d.mkv", n)), 1, n, 0, defaults)...)
	}
	rep := writer(f).Write(ctx, showQuery, showBase, files)
	assert.Empty(t, rep.Problems())
	got := listFiles(t, root)
	assert.Len(t, got, 5+12*2, "tvshow.nfo、海报、背景、季海报与 season.nfo，每集 NFO 与截图")
	assert.Contains(t, got, "最后生还者 (2023)/season01-poster.jpg")
	assert.Equal(t, "IMG/e7.jpg", read(t, filepath.Join(seasonDir, "S01E07-thumb.jpg")))
	assert.Contains(t, read(t, filepath.Join(seasonDir, "S01E07.nfo")), "<title>E7</title>")
	assert.Contains(t, read(t, filepath.Join(showDir, "tvshow.nfo")), "<tagline>宣传语</tagline>")
	assert.Equal(t, 1, strings.Count(strings.Join(f.called(), ","), "season1"), "一季只取一次")

	// 第 2 季 TMDB 上没有：season.nfo 只写季号，没有季海报与截图
	flat := t.TempDir()
	files = append(SeasonFiles(flat, filepath.Join(flat, "S2"), 2, defaults), EpisodeFiles(filepath.Join(flat, "S2", "S02E05.mkv"), 2, 5, 0, defaults)...)
	rep = writer(&fakeTMDB{}).Write(ctx, showQuery, showBase, files)
	assert.Equal(t, []string{"TMDB 上没有第 2 季的详情，这一季只写季号与集号"}, rep.Problems())
	assert.Equal(t, []string{"S2/S02E05.nfo", "S2/season.nfo"}, listFiles(t, flat))
	assert.Contains(t, read(t, filepath.Join(flat, "S2", "S02E05.nfo")), "<title>第 5 集</title>")
}

// 只写选了的内容；只有 Fanart.tv 有的图没填它的 Key 时写明
func TestWriteItems(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	f := &fakeTMDB{}
	rep := writer(f).Write(ctx, movieQuery, movieBase, MovieFiles(filepath.Join(root, "M (2023)", "M.mkv"), true, ParseItems("poster,logo,clearart,banner")))
	assert.Equal(t, []string{"M (2023)/clearlogo.png", "M (2023)/poster.jpg"}, listFiles(t, root))
	assert.Equal(t, []string{"Clearart、横幅只有 Fanart.tv 有：在「媒体识别」里填了 Fanart.tv 的 API Key 才写"}, rep.Problems())
	assert.NotContains(t, f.called(), "extras", "不写 NFO 时不取完整信息")

	// 什么都没选：什么都不写，不建目录
	empty := t.TempDir()
	rep = writer(&fakeTMDB{}).Write(ctx, movieQuery, movieBase, MovieFiles(filepath.Join(empty, "M", "M.mkv"), true, Items{}))
	assert.Empty(t, rep.Files)
	_, err := os.Stat(filepath.Join(empty, "M"))
	assert.True(t, os.IsNotExist(err))
}

// 覆盖已有的 NFO：媒体服务器里锁定了的不改；pt-tools 不管的元素留着，锁定的字段用原来的，别的工具写的评分去掉
func TestWriteMergesExistingNFO(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	video := filepath.Join(dir, "M", "M.mkv")
	nfo := filepath.Join(dir, "M", "M.nfo")
	require.NoError(t, os.MkdirAll(filepath.Dir(nfo), 0o755))
	w := writer(&fakeTMDB{})
	w.Overwrite = true
	files := MovieFiles(video, true, ParseItems("nfo"))

	old := "\xEF\xBB\xBF" + `<?xml version="1.0" encoding="utf-8"?>
<movie>
  <title>旧标题</title>
  <plot>用户改过的简介</plot>
  <rating>6.5</rating>
  <dateadded>2024-01-02 03:04:05</dateadded>
  <playcount>2</playcount>
  <fileinfo><streamdetails><video><codec>hevc</codec></video></streamdetails></fileinfo>
  <lockedfields>Overview</lockedfields>
  <uniqueid type="tmdb">999</uniqueid>
</movie>`
	require.NoError(t, os.WriteFile(nfo, []byte(old), 0o644))
	rep := w.Write(ctx, movieQuery, movieBase, files)
	require.Empty(t, rep.Problems())
	got := read(t, nfo)
	assert.Contains(t, got, "<title>奥本海默</title>")
	assert.Contains(t, got, "<plot>用户改过的简介</plot>", "锁定的简介用原来的")
	assert.Less(t, strings.Index(got, "<title>"), strings.Index(got, "<plot>"))
	assert.Less(t, strings.Index(got, "<plot>"), strings.Index(got, "<uniqueid"), "写在简介本来的位置，不挪到后面")
	assert.NotContains(t, got, "TMDB 的简介")
	assert.Contains(t, got, "<dateadded>2024-01-02 03:04:05</dateadded>")
	assert.Contains(t, got, "<playcount>2</playcount>")
	assert.Contains(t, got, "<fileinfo><streamdetails><video><codec>hevc</codec></video></streamdetails></fileinfo>")
	assert.NotContains(t, got, "<rating>6.5</rating>")
	assert.NotContains(t, got, ">999<")
	assert.Contains(t, got, `<uniqueid type="tmdb" default="true">872585</uniqueid>`)
	// 再写一次：内容一样，不重写
	rep = w.Write(ctx, movieQuery, movieBase, files)
	assert.Equal(t, FileUnchanged, rep.Files[0].Status)

	// 锁定了：不改
	lockedNFO := `<movie><title>锁定的</title><lockdata>true</lockdata></movie>`
	require.NoError(t, os.WriteFile(nfo, []byte(lockedNFO), 0o644))
	rep = w.Write(ctx, movieQuery, movieBase, files)
	assert.Equal(t, FileLocked, rep.Files[0].Status)
	assert.Equal(t, lockedNFO, read(t, nfo))
	// 读不出来的 NFO 里写着锁定也不改
	broken := `<movie><title>坏的 &bogus;</title><lockdata>TRUE</lockdata>`
	require.NoError(t, os.WriteFile(nfo, []byte(broken), 0o644))
	rep = w.Write(ctx, movieQuery, movieBase, files)
	assert.Equal(t, FileLocked, rep.Files[0].Status)

	// 不是 XML（只有一个网址的 NFO）：整个换掉
	require.NoError(t, os.WriteFile(nfo, []byte("https://www.imdb.com/title/tt15398776/"), 0o644))
	rep = w.Write(ctx, movieQuery, movieBase, files)
	assert.Equal(t, FileWritten, rep.Files[0].Status)
	assert.NotContains(t, read(t, nfo), "imdb.com")
}

// brokenFanart 是列得出图片、却下载不了的 Fanart.tv。
type brokenFanart struct{}

func (brokenFanart) Movie(context.Context, string) (fanart.Images, error) {
	return fanart.Images{"poster": {{URL: "https://assets.fanart.tv/p.jpg", Lang: "zh", Likes: 9}}}, nil
}

func (brokenFanart) TV(context.Context, int) (fanart.Images, error) { return nil, nil }

func (brokenFanart) Download(context.Context, string) ([]byte, error) {
	return nil, errors.New("HTTP 404")
}

// 图片来源选了 Fanart.tv、它的图下载不了：用 TMDB 的那张，并写明
func TestWriteArtFallsBack(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	video := filepath.Join(dir, "M", "M.mkv")
	f := &fakeTMDB{}
	w := Writer{
		Resolver: metadata.NewResolver(metadata.NewTMDB(f), metadata.NewFanart(brokenFanart{})),
		Sources:  metadata.Sources{Artwork: metadata.SourceFanart},
	}
	rep := w.Write(ctx, movieQuery, movieBase, MovieFiles(video, true, ParseItems("poster")))
	require.Len(t, rep.Files, 1)
	assert.Equal(t, FileWritten, rep.Files[0].Status)
	assert.Equal(t, "IMG/poster.jpg", read(t, filepath.Join(dir, "M", "poster.jpg")))
	assert.Equal(t, []string{"poster.jpg：Fanart.tv 的下载不了（HTTP 404），用了 TMDB 的"}, rep.Errors, "选的来源出错：算来源出错")
	assert.Empty(t, rep.Notes)
}

// 取数据的这段时间里文件被改了（媒体服务器锁定了 NFO、有人新放了图片）：写之前再看一次，变了就不换
func TestWriteKeepsFilesChangedMeanwhile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	video := filepath.Join(dir, "M", "M.mkv")
	nfo := filepath.Join(dir, "M", "M.nfo")
	poster := filepath.Join(dir, "M", "poster.jpg")
	require.NoError(t, os.MkdirAll(filepath.Dir(nfo), 0o755))
	require.NoError(t, os.WriteFile(nfo, []byte("<movie><title>旧的</title></movie>"), 0o644))
	locked := "<movie><title>旧的</title><lockdata>true</lockdata></movie>"
	f := &fakeTMDB{onDetails: func() {
		_ = os.WriteFile(nfo, []byte(locked), 0o644)
		_ = os.WriteFile(poster, []byte("mine"), 0o644)
	}}
	w := writer(f)
	w.Overwrite = true
	rep := w.Write(ctx, movieQuery, movieBase, MovieFiles(video, true, ParseItems("nfo,poster")))
	require.Len(t, rep.Files, 2)
	for _, r := range rep.Files {
		assert.Equal(t, FileKept, r.Status, r.Path)
		assert.Empty(t, r.SHA256)
	}
	assert.Equal(t, locked, read(t, nfo), "期间锁定了的 NFO 没被覆盖")
	assert.Equal(t, "mine", read(t, poster), "期间新放的图片没被覆盖")
	entries, err := os.ReadDir(filepath.Dir(nfo))
	require.NoError(t, err)
	assert.Len(t, entries, 2, "没有留下临时文件")
}

func TestWriteFailsOnBadDir(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "file")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	rep := writer(&fakeTMDB{}).Write(context.Background(), movieQuery, movieBase, MovieFiles(filepath.Join(blocker, "M", "M.mkv"), true, ParseItems("nfo")))
	require.Len(t, rep.Problems(), 1)
	assert.Contains(t, rep.Problems()[0], "M.nfo")
	assert.Equal(t, FileFailed, rep.Files[0].Status)
}
