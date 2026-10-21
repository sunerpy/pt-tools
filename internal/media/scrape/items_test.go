package scrape

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/metadata"
)

func TestItems(t *testing.T) {
	assert.Equal(t, DefaultItems, ParseItems("").List(), "没选过时 Logo 与只有 Fanart.tv 有的以外都写")
	assert.False(t, ParseItems("").Logo)
	assert.False(t, ParseItems("").Clearart)
	assert.Equal(t, []string{ItemPoster, ItemLogo, ItemBanner}, ParseItems("banner,logo, poster,unknown").List(), "按界面顺序，不认识的忽略")

	got, err := NormalizeItems([]string{"thumb", " nfo", "thumb", "logo", "discart"})
	require.NoError(t, err)
	assert.Equal(t, "nfo,logo,discart,thumb", got)
	got, err = NormalizeItems(nil)
	require.NoError(t, err)
	assert.Empty(t, got)
	_, err = NormalizeItems([]string{"trailer"})
	require.Error(t, err)

	assert.False(t, ParseItems("season_poster,thumb").Any(false), "电影没有季海报与集截图")
	assert.True(t, ParseItems("season_poster,thumb").Any(true))
	assert.True(t, ParseItems("logo").Any(false))
	assert.True(t, ParseItems("discart").Any(false))
	assert.False(t, ParseItems("discart").Any(true), "剧集没有碟片图")
}

func names(files []File) []string {
	var out []string
	for _, f := range files {
		out = append(out, filepath.Base(f.Path))
	}
	return out
}

// 文件名：电影有自己的目录时放在目录里，平铺时带上文件名；剧集的放在剧集目录里，季海报也是
func TestFiles(t *testing.T) {
	all := ParseItems("nfo,poster,fanart,logo,clearart,banner,landscape,discart,season_poster,thumb")
	video := filepath.Join("/lib", "奥本海默 (2023)", "奥本海默 (2023).mkv")
	assert.Equal(t, []string{
		"奥本海默 (2023).nfo", "poster.jpg", "fanart.jpg", "clearlogo.png", "clearart.png", "banner.jpg", "landscape.jpg", "discart.png",
	}, names(MovieFiles(video, true, all)))
	flat := MovieFiles(filepath.Join("/lib", "M.mkv"), false, all)
	assert.Equal(t, []string{
		"M.nfo", "M-poster.jpg", "M-fanart.jpg", "M-clearlogo.png", "M-clearart.png", "M-banner.jpg", "M-landscape.jpg", "M-discart.png",
	}, names(flat))
	assert.Equal(t, NFOMovie, flat[0].NFO)
	assert.Equal(t, metadata.ArtDisc, flat[7].Art)

	show := filepath.Join("/tv", "S (2023)")
	assert.Equal(t, []string{"tvshow.nfo", "poster.jpg", "fanart.jpg", "clearlogo.png", "clearart.png", "banner.jpg", "landscape.jpg"},
		names(ShowFiles(show, all)), "剧集没有碟片图")
	season := SeasonFiles(show, filepath.Join(show, "Season 1"), 1, all)
	assert.Equal(t, []string{"season.nfo", "season01-poster.jpg"}, names(season))
	assert.Equal(t, filepath.Join(show, "season01-poster.jpg"), season[1].Path, "季海报放在剧集目录里")
	assert.Equal(t, []string{"season-specials-poster.jpg"}, names(SeasonFiles(show, show, 0, all)), "没有季目录时不写 season.nfo")
	ep := EpisodeFiles(filepath.Join(show, "Season 1", "S01E01.mkv"), 1, 1, 0, all)
	assert.Equal(t, []string{"S01E01.nfo", "S01E01-thumb.jpg"}, names(ep))
	assert.Equal(t, 1, ep[0].EpisodeEnd, "单集的 EpisodeEnd 是这一集")
	assert.Empty(t, MovieFiles(video, true, Items{}))

	for _, n := range []string{"poster.jpg", "fanart.jpg", "clearlogo.png", "clearart.png", "banner.jpg", "landscape.jpg", "discart.png", "tvshow.nfo", "season.nfo", "season01-poster.jpg", "season-specials-poster.jpg"} {
		assert.True(t, IsDirArtifact(n), n)
	}
	for _, n := range []string{"M-poster.jpg", "M.nfo", "S01E01-thumb.jpg", "season1-poster.png"} {
		assert.False(t, IsDirArtifact(n), n)
	}
}

func TestJoin(t *testing.T) {
	assert.Equal(t, "来源里没有 Logo、集截图", Join("来源里没有", "Logo、集截图"))
	assert.Equal(t, "Clearart 只有 Fanart.tv 有", Join("Clearart", "只有 Fanart.tv 有"))
	assert.Equal(t, "海报、横幅只有 Fanart.tv 有", Join("海报、横幅", "只有 Fanart.tv 有"))
	assert.Equal(t, "来源里没有横幅", Join("来源里没有", "横幅"))
}
