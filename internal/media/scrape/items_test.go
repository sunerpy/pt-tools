package scrape

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/tmdb"
)

func TestItems(t *testing.T) {
	assert.Equal(t, DefaultItems, ParseItems("").List(), "没选过时 Logo 以外都写")
	assert.False(t, ParseItems("").Logo)
	assert.Equal(t, []string{ItemPoster, ItemLogo}, ParseItems("logo, poster,unknown").List(), "按界面顺序，不认识的忽略")

	got, err := NormalizeItems([]string{"thumb", " nfo", "thumb", "logo"})
	require.NoError(t, err)
	assert.Equal(t, "nfo,logo,thumb", got)
	got, err = NormalizeItems(nil)
	require.NoError(t, err)
	assert.Empty(t, got)
	_, err = NormalizeItems([]string{"banner"})
	require.Error(t, err)

	assert.False(t, ParseItems("season_poster,thumb").Any(false), "电影没有季海报与集截图")
	assert.True(t, ParseItems("season_poster,thumb").Any(true))
	assert.True(t, ParseItems("logo").Any(false))
}

func TestChooseArt(t *testing.T) {
	r := tmdb.Result{PosterPath: "/default.jpg", BackdropPath: "/default-bd.jpg"}
	imgs := &tmdb.Images{
		Posters:   []tmdb.Image{{Path: "/en.jpg", Lang: "en"}, {Path: "/zh.jpg", Lang: "zh"}, {Path: "/none.jpg"}},
		Backdrops: []tmdb.Image{{Path: "/bd-zh.jpg", Lang: "zh"}, {Path: "/bd-none.jpg"}},
		Logos:     []tmdb.Image{{Path: "/logo-ja.png", Lang: "ja"}, {Path: "/logo-en.png", Lang: "en"}, {Path: "/logo-zh.png", Lang: "zh"}},
	}
	cases := []struct {
		name, lang, recog string
		imgs              *tmdb.Images
		want              Art
	}{
		{"没取图片列表：用 TMDB 给的、不写 Logo", ImageLangAuto, "zh-CN", nil, Art{Poster: "/default.jpg", Fanart: "/default-bd.jpg"}},
		{"跟 TMDB：海报背景不换，Logo 用识别语言", ImageLangAuto, "zh-CN", imgs, Art{Poster: "/default.jpg", Fanart: "/default-bd.jpg", Logo: "/logo-zh.png"}},
		{"英文海报，背景用没有文字的", "en", "zh-CN", imgs, Art{Poster: "/en.jpg", Fanart: "/bd-none.jpg", Logo: "/logo-en.png"}},
		{"无文字", ImageLangNone, "ja-JP", imgs, Art{Poster: "/none.jpg", Fanart: "/bd-none.jpg", Logo: "/logo-ja.png"}},
		{"没有这种语言的海报时用 TMDB 给的", "ja", "zh-CN", imgs, Art{Poster: "/default.jpg", Fanart: "/bd-none.jpg", Logo: "/logo-ja.png"}},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, ChooseArt(r, c.imgs, c.lang, c.recog), c.name)
	}

	// Logo：识别语言与英文都没有时用没有文字的，再没有时用评分最高的一张
	only := &tmdb.Images{Logos: []tmdb.Image{{Path: "/ko.png", Lang: "ko"}, {Path: "/plain.png"}}}
	assert.Equal(t, "/plain.png", ChooseArt(r, only, ImageLangAuto, "zh-CN").Logo)
	only.Logos = only.Logos[:1]
	assert.Equal(t, "/ko.png", ChooseArt(r, only, ImageLangAuto, "zh-CN").Logo)
	assert.Empty(t, ChooseArt(r, &tmdb.Images{}, "zh", "zh-CN").Logo)
}

// 只写选了的内容；Logo 写成 clearlogo.png（库目录根下的电影带上文件名）
func TestWriterItems(t *testing.T) {
	ctx := context.Background()
	art := Art{Poster: "/p.jpg", Fanart: "/bd.jpg", Logo: "/logo.png"}

	root := t.TempDir()
	img := &fakeImages{}
	w := Writer{Images: img, Items: ParseItems("poster,logo")}
	rep := w.Movie(ctx, filepath.Join(root, "M (2023)", "M.mkv"), true, movie, art)
	assert.Empty(t, rep.Errors)
	assert.Equal(t, []string{"M (2023)/clearlogo.png", "M (2023)/poster.jpg"}, listFiles(t, root))
	assert.Equal(t, []string{"w780/p.jpg", "w500/logo.png"}, img.calls)

	flat := t.TempDir()
	rep = Writer{Images: &fakeImages{}, Items: ParseItems("logo")}.Movie(ctx, filepath.Join(flat, "M.mkv"), false, movie, art)
	assert.Empty(t, rep.Errors)
	assert.Equal(t, []string{"M-clearlogo.png"}, listFiles(t, flat))

	// 剧集：只要季海报与 Logo
	tv := t.TempDir()
	showDir := filepath.Join(tv, "S (2023)")
	seasonDir := filepath.Join(showDir, "Season 1")
	w = Writer{Images: &fakeImages{}, Items: ParseItems("season_poster,logo")}
	var all Report
	all.Merge(w.Show(ctx, showDir, show, art))
	all.Merge(w.Season(ctx, showDir, seasonDir, 1, season1))
	all.Merge(w.Episode(ctx, filepath.Join(seasonDir, "S01E01.mkv"), show, 1, 1, 0, season1))
	assert.Empty(t, all.Errors)
	assert.Equal(t, []string{"S (2023)/clearlogo.png", "S (2023)/season01-poster.jpg"}, listFiles(t, tv))

	// 只要 NFO 与集截图：不下载海报
	tv2 := t.TempDir()
	img = &fakeImages{}
	w = Writer{Images: img, Items: ParseItems("nfo,thumb")}
	showDir = filepath.Join(tv2, "S")
	seasonDir = filepath.Join(showDir, "Season 1")
	all = Report{}
	all.Merge(w.Show(ctx, showDir, show, art))
	all.Merge(w.Season(ctx, showDir, seasonDir, 1, season1))
	all.Merge(w.Episode(ctx, filepath.Join(seasonDir, "S01E02.mkv"), show, 1, 2, 0, season1))
	assert.Empty(t, all.Errors)
	assert.Equal(t, []string{"S/Season 1/S01E02-thumb.jpg", "S/Season 1/S01E02.nfo", "S/Season 1/season.nfo", "S/tvshow.nfo"}, listFiles(t, tv2))
	assert.Equal(t, []string{"w300/e2.jpg"}, img.calls)

	// 什么都没选：什么都不写
	empty := t.TempDir()
	rep = Writer{Images: &fakeImages{}}.Movie(ctx, filepath.Join(empty, "M", "M.mkv"), true, movie, art)
	assert.Empty(t, rep.Written)
	_, err := os.Stat(filepath.Join(empty, "M"))
	assert.True(t, os.IsNotExist(err))
}
