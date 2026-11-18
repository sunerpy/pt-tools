package transfer

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "重写 testdata 里的 golden 文件")

// golden 比较 got 与 testdata/name；带 -update 运行时改写文件。
func golden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("testdata", name)
	if *update {
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(got), 0o644))
	}
	want, err := os.ReadFile(p)
	require.NoError(t, err, "缺 golden 文件时用 -update 生成")
	assert.Equal(t, string(want), got)
}

func TestRenderGolden(t *testing.T) {
	multi := SampleEpisode
	multi.Episode, multi.EpisodeEnd, multi.SeasonEpisode = 1, 2, SeasonEpisode(1, 1, 2)
	noYear := SampleMovie
	noYear.Year, noYear.Quality = 0, ""
	colon := SampleMovie
	colon.Title = `碟中谍7：致命清算（上）`
	colon.OriginalTitle = `Mission: Impossible - Dead Reckoning Part One`
	weird := SampleMovie
	weird.Title = "  A/B\\C*D?E\"F<G>H|I\x01J. "
	special := SampleEpisode
	special.Season, special.Episode, special.SeasonEpisode, special.EpisodeTitle = 0, 1, SeasonEpisode(0, 1, 0), ""

	cases := []struct {
		name, tpl string
		v         Vars
	}{
		{"movie-default", DefaultMovieTemplate, SampleMovie},
		{"movie-no-year-quality", DefaultMovieTemplate, noYear},
		{"movie-colon", DefaultMovieTemplate, colon},
		{"movie-original-colon", `{{.OriginalTitle}} ({{.Year}})/{{.OriginalTitle}}`, colon},
		{"movie-illegal-chars", DefaultMovieTemplate, weird},
		{"movie-emby-tmdbid", `{{.Title}} ({{.Year}}) [tmdbid={{.TMDBID}}]/{{.Title}} ({{.Year}})`, SampleMovie},
		{"movie-jellyfin-tmdbid", `{{.Title}} ({{.Year}}) [tmdbid-{{.TMDBID}}]/{{.Title}} ({{.Year}})`, SampleMovie},
		{"movie-plex-tmdb", `{{.Title}} ({{.Year}}) {tmdb-{{.TMDBID}}}/{{.Title}} ({{.Year}})`, SampleMovie},
		{"movie-group", `{{.Title}}.{{.Year}}.{{.Resolution}}.{{.Source}}.{{.VideoCodec}}-{{.Group}}`, SampleMovie},
		{"tv-default", DefaultTVTemplate, SampleEpisode},
		{"tv-multi-episode", DefaultTVTemplate, multi},
		{"tv-specials", DefaultTVTemplate, special},
		{"tv-pad", `{{.Title}}/Season {{pad .Season 2}}/{{.Title}} S{{pad .Season 2}}E{{pad .Episode 3}}`, SampleEpisode},
		{"tv-empty-segments", `{{.Title}}//{{.Group}}/./{{.SeasonEpisode}}`, SampleEpisode},
	}
	var b strings.Builder
	for _, c := range cases {
		got, err := Render(c.tpl, c.v)
		require.NoError(t, err, c.name)
		fmt.Fprintf(&b, "%s\t%s\n", c.name, got)
	}
	golden(t, "render.golden", b.String())
}

func TestRenderErrors(t *testing.T) {
	for _, tc := range []struct{ name, tpl, want string }{
		{"empty", "  ", "模板为空"},
		{"parse", "{{.Title", "命名模板不能用"},
		{"unknown field", "{{.Nope}}", "Nope"},
		{"only separators", "{{if .Episode}}x{{end}}/ / .", "渲染结果为空"},
		{"too long segment", strings.Repeat("长", 80), "超过 200 字节"},
		{"too long output", "{{range 10000}}abcdef{{end}}", "超过 2048 字节"},
	} {
		_, err := Render(tc.tpl, SampleMovie)
		require.Error(t, err, tc.name)
		assert.ErrorIs(t, err, ErrTemplate, tc.name)
		assert.Contains(t, err.Error(), tc.want, tc.name)
	}
}

func TestRenderDotDotSegmentsDropped(t *testing.T) {
	// 全是点的一级（. 与 ..）去掉首尾的点后是空的，直接跳过，算出的路径不会跑到库目录外面
	v := SampleMovie
	v.Title = ".."
	got, err := Render(`{{.Title}}/x`, v)
	require.NoError(t, err)
	assert.Equal(t, "x", got)
	got, err = Render(`../{{.Year}}/./../y`, SampleMovie)
	require.NoError(t, err)
	assert.Equal(t, "2023/y", got)
}

func TestCleanName(t *testing.T) {
	assert.Equal(t, "Mission - Impossible", CleanName("Mission: Impossible"))
	assert.Equal(t, "复仇者联盟4 - 终局之战", CleanName("复仇者联盟4：终局之战"))
	assert.Equal(t, "What If", CleanName("What If...?"))
	assert.Equal(t, "a b", CleanName("a\tb"))
	assert.Equal(t, "", CleanName(" ... "))
}

func TestSeasonEpisodeAndCheck(t *testing.T) {
	assert.Equal(t, "S01E02", SeasonEpisode(1, 2, 0))
	assert.Equal(t, "S01E02-E05", SeasonEpisode(1, 2, 5))
	assert.Equal(t, "S02", SeasonEpisode(2, 0, 0))
	got, err := Check(DefaultTVTemplate, "tv")
	require.NoError(t, err)
	assert.Equal(t, "最后生还者 (2023)/Season 1/最后生还者 - S01E02 - 感染", got)
	got, err = Check(DefaultMovieTemplate, "movie")
	require.NoError(t, err)
	assert.Equal(t, "奥本海默 (2023)/奥本海默 (2023) - 2160p BluRay HDR10 H.265", got)
}
