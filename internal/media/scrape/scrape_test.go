package scrape

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/tmdb"
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
	movie = tmdb.Result{
		ID: 872585, MediaType: tmdb.KindMovie, Title: "奥本海默", OriginalTitle: "Oppenheimer", Year: 2023, Date: "2023-07-19",
		Overview: "「原子弹之父」罗伯特·奥本海默的故事 & <曼哈顿计划>。", PosterPath: "/poster.jpg", BackdropPath: "/backdrop.jpg",
		VoteAverage: 8.123, IMDbID: "tt15398776", Genres: []string{"剧情", "历史"}, Runtime: 181,
	}
	show = tmdb.Result{
		ID: 100088, MediaType: tmdb.KindTV, Title: "最后生还者", OriginalTitle: "The Last of Us", Year: 2023, Date: "2023-01-15",
		Overview: "末日之后……", PosterPath: "/show.jpg", BackdropPath: "/show-bd.jpg", VoteAverage: 8.6, IMDbID: "tt3581920",
		Genres: []string{"剧情"}, Runtime: 50,
	}
	season1 = &tmdb.Season{
		Number: 1, Name: "第 1 季", Overview: "第一季", AirDate: "2023-01-15", PosterPath: "/s1.jpg",
		Episodes: []tmdb.Episode{
			{ID: 2181581, Number: 1, Name: "当你迷失在黑暗中", Overview: "……", AirDate: "2023-01-15", StillPath: "/e1.jpg", Runtime: 81},
			{ID: 4071039, Number: 2, Name: "感染", AirDate: "2023-01-22", StillPath: "/e2.jpg", Runtime: 53},
		},
	}
)

func TestNFOGolden(t *testing.T) {
	b, err := MovieNFO(movie)
	require.NoError(t, err)
	golden(t, "movie.nfo", b)

	bare := tmdb.Result{ID: 1, Title: "Same", OriginalTitle: "Same"}
	b, err = MovieNFO(bare)
	require.NoError(t, err)
	golden(t, "movie-bare.nfo", b)

	b, err = TVShowNFO(show)
	require.NoError(t, err)
	golden(t, "tvshow.nfo", b)

	b, err = SeasonNFO(1, season1)
	require.NoError(t, err)
	golden(t, "season.nfo", b)
	b, err = SeasonNFO(0, nil)
	require.NoError(t, err)
	golden(t, "season-bare.nfo", b)

	b, err = EpisodeNFO(show, 1, 2, 0, season1)
	require.NoError(t, err)
	golden(t, "episode.nfo", b)
	b, err = EpisodeNFO(show, 1, 1, 3, season1)
	require.NoError(t, err)
	golden(t, "episode-multi.nfo", b)
}

type fakeImages struct {
	calls []string
	fail  map[string]error
}

func (f *fakeImages) Image(_ context.Context, path, size string) ([]byte, error) {
	f.calls = append(f.calls, size+path)
	if err := f.fail[path]; err != nil {
		return nil, err
	}
	return []byte("IMG" + path), nil
}

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

func TestWriterMovie(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	img := &fakeImages{}
	w := Writer{Images: img}
	video := filepath.Join(root, "奥本海默 (2023)", "奥本海默 (2023) - 2160p.mkv")
	rep := w.Movie(ctx, video, true, movie)
	assert.Empty(t, rep.Errors)
	assert.Len(t, rep.Written, 3)
	assert.Equal(t, []string{
		"奥本海默 (2023)/fanart.jpg",
		"奥本海默 (2023)/poster.jpg",
		"奥本海默 (2023)/奥本海默 (2023) - 2160p.nfo",
	}, listFiles(t, root))
	assert.Equal(t, []string{"w780/poster.jpg", "w1280/backdrop.jpg"}, img.calls)

	// 不覆盖：已有的文件不动，也不再下载图片
	require.NoError(t, os.WriteFile(filepath.Join(root, "奥本海默 (2023)", "poster.jpg"), []byte("mine"), 0o644))
	img.calls = nil
	rep = w.Movie(ctx, video, true, movie)
	assert.Empty(t, rep.Written)
	assert.Empty(t, img.calls)
	b, _ := os.ReadFile(filepath.Join(root, "奥本海默 (2023)", "poster.jpg"))
	assert.Equal(t, "mine", string(b))

	// 覆盖
	w.Overwrite = true
	rep = w.Movie(ctx, video, true, movie)
	assert.Len(t, rep.Written, 3)
	b, _ = os.ReadFile(filepath.Join(root, "奥本海默 (2023)", "poster.jpg"))
	assert.Equal(t, "IMG/poster.jpg", string(b))

	// 放在库目录根下：图片带上文件名
	flat := t.TempDir()
	rep = Writer{Images: &fakeImages{}}.Movie(ctx, filepath.Join(flat, "Movie.mkv"), false, movie)
	assert.Empty(t, rep.Errors)
	assert.Equal(t, []string{"Movie-fanart.jpg", "Movie-poster.jpg", "Movie.nfo"}, listFiles(t, flat))

	// 图片下载失败只记错误，NFO 照写
	bad := t.TempDir()
	rep = Writer{Images: &fakeImages{fail: map[string]error{"/backdrop.jpg": errors.New("boom")}}}.Movie(ctx, filepath.Join(bad, "M", "M.mkv"), true, movie)
	require.Len(t, rep.Errors, 1)
	assert.Contains(t, rep.Errors[0], "fanart.jpg: boom")
	assert.Equal(t, []string{"M/M.nfo", "M/poster.jpg"}, listFiles(t, bad))

	// 没有图片下载器、条目没有图片时只写 NFO
	none := t.TempDir()
	rep = Writer{}.Movie(ctx, filepath.Join(none, "M", "M.mkv"), true, movie)
	assert.Empty(t, rep.Errors)
	assert.Equal(t, []string{"M/M.nfo"}, listFiles(t, none))

	left, _ := filepath.Glob(filepath.Join(root, "*", "*.part"))
	assert.Empty(t, left, "不留临时文件")
}

func TestWriterTV(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	img := &fakeImages{}
	w := Writer{Images: img}
	showDir := filepath.Join(root, "最后生还者 (2023)")
	seasonDir := filepath.Join(showDir, "Season 1")
	video := filepath.Join(seasonDir, "最后生还者 - S01E02 - 感染.mkv")

	var rep Report
	rep.Merge(w.Show(ctx, showDir, show))
	rep.Merge(w.Season(ctx, showDir, seasonDir, 1, season1))
	rep.Merge(w.Episode(ctx, video, show, 1, 2, 0, season1))
	assert.Empty(t, rep.Errors)
	assert.Equal(t, []string{
		"最后生还者 (2023)/Season 1/season.nfo",
		"最后生还者 (2023)/Season 1/最后生还者 - S01E02 - 感染-thumb.jpg",
		"最后生还者 (2023)/Season 1/最后生还者 - S01E02 - 感染.nfo",
		"最后生还者 (2023)/fanart.jpg",
		"最后生还者 (2023)/poster.jpg",
		"最后生还者 (2023)/season01-poster.jpg",
		"最后生还者 (2023)/tvshow.nfo",
	}, listFiles(t, root))
	b, _ := os.ReadFile(filepath.Join(seasonDir, "最后生还者 - S01E02 - 感染-thumb.jpg"))
	assert.Equal(t, "IMG/e2.jpg", string(b))

	// 没有季目录：不写 season.nfo；季详情为空时不写季海报与截图
	flat := t.TempDir()
	rep = w.Season(ctx, flat, flat, 0, nil)
	assert.Empty(t, rep.Written)
	rep = w.Episode(ctx, filepath.Join(flat, "S00E05.mkv"), show, 0, 5, 0, nil)
	assert.Equal(t, []string{"S00E05.nfo"}, listFiles(t, flat))
	b, _ = os.ReadFile(filepath.Join(flat, "S00E05.nfo"))
	assert.True(t, strings.Contains(string(b), "<title>第 5 集</title>"))
	assert.Equal(t, "season-specials-poster.jpg", SeasonPosterName(0))
	assert.Equal(t, "season12-poster.jpg", SeasonPosterName(12))
}

func TestWriterFailsOnBadDir(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "file")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	rep := Writer{}.Movie(context.Background(), filepath.Join(blocker, "M", "M.mkv"), true, movie)
	require.Len(t, rep.Errors, 1)
	assert.Contains(t, rep.Errors[0], "M.nfo")
}
