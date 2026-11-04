package epgroup

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
)

// eps 是第 season 季的第 from 到第 to 集（剧集组里的集，order 从 start 起）。
func eps(season, from, to, start int) []tmdb.EpisodeGroupEpisode {
	var out []tmdb.EpisodeGroupEpisode
	for n := from; n <= to; n++ {
		out = append(out, tmdb.EpisodeGroupEpisode{Season: season, Episode: n, Order: start})
		start++
	}
	return out
}

// absolute 是绝对集号的剧集组：一组，TMDB 默认的第 1 季 24 集、第 2 季 12 集。
func absolute() *Map {
	all := append(eps(1, 1, 24, 0), eps(2, 1, 12, 24)...)
	return New(&tmdb.EpisodeGroup{
		EpisodeGroupSummary: tmdb.EpisodeGroupSummary{ID: "5b11ba820e0a265a53000ce9", Name: "Absolute Order", Type: tmdb.EpisodeGroupAbsolute},
		Groups:              []tmdb.EpisodeGroupPart{{Name: "Episodes", Order: 1, Episodes: all}},
	})
}

// dvd 是另一种分季：TMDB 默认只有一季 26 集，剧集组分成两组（13 集一组），前面有一个特别篇的组。
func dvd() *Map {
	return New(&tmdb.EpisodeGroup{
		EpisodeGroupSummary: tmdb.EpisodeGroupSummary{ID: "5c8a1ef80e0a2625f5a4f1b0", Name: "DVD", Type: tmdb.EpisodeGroupDVD},
		Groups: []tmdb.EpisodeGroupPart{
			{Name: "Specials", Order: 0, Episodes: eps(0, 1, 3, 0)},
			{Name: "Season 1", Order: 1, Episodes: eps(1, 1, 13, 0)},
			{Name: "Season 2", Order: 2, Episodes: eps(1, 14, 26, 0)},
			{Name: "Season 3", Order: 3},
		},
	})
}

func convert(t *testing.T, m *Map, md meta.Meta, season int) ([]Ref, bool) {
	t.Helper()
	c, ok := CoordOf(md, season)
	require.True(t, ok)
	return m.Convert(c)
}

func TestConvertAbsolute(t *testing.T) {
	m := absolute()
	assert.Equal(t, 1, m.Parts())

	refs, ok := convert(t, m, meta.Meta{Episode: 25}, 0)
	require.True(t, ok, "只写了集号的按第 1 组算")
	assert.Equal(t, []Ref{{Season: 2, Episode: 1}}, refs, "绝对集号 25 是第 2 季第 1 集")

	refs, ok = convert(t, m, meta.Meta{Season: 1, Episode: 7}, 0)
	require.True(t, ok)
	assert.Equal(t, []Ref{{Season: 1, Episode: 7}}, refs)

	refs, ok = convert(t, m, meta.Meta{Episode: 23, EpisodeEnd: 26}, 0)
	require.True(t, ok, "跨季的范围")
	assert.Equal(t, []Ref{{1, 23}, {1, 24}, {2, 1}, {2, 2}}, refs)

	_, ok = convert(t, m, meta.Meta{Episode: 37}, 0)
	assert.False(t, ok, "剧集组里没有第 37 集")
	_, ok = convert(t, m, meta.Meta{Episode: 35, EpisodeEnd: 37}, 0)
	assert.False(t, ok, "范围里有一集没有时整个不换算")
	_, ok = convert(t, m, meta.Meta{Season: 2, Episode: 1}, 0)
	assert.False(t, ok, "写了第 2 季：剧集组只有一组，是按 TMDB 默认的季集写的")

	refs, ok = convert(t, m, meta.Meta{Season: 1}, 0)
	require.True(t, ok, "一整组")
	assert.Len(t, refs, 36)
	assert.Equal(t, Ref{2, 12}, refs[35])
}

func TestConvertSeasons(t *testing.T) {
	m := dvd()
	assert.Equal(t, 3, m.Parts(), "只有特别篇的组不算")

	refs, ok := convert(t, m, meta.Meta{Season: 2, Episode: 1}, 0)
	require.True(t, ok)
	assert.Equal(t, []Ref{{1, 14}}, refs, "第 2 组第 1 集是 TMDB 第 1 季第 14 集（特别篇的组不占组号）")

	refs, ok = convert(t, m, meta.Meta{Season: 1, Episode: 1}, 0)
	require.True(t, ok)
	assert.Equal(t, []Ref{{1, 1}}, refs)

	refs, ok = convert(t, m, meta.Meta{Season: 2}, 0)
	require.True(t, ok, "一整组")
	assert.Equal(t, 13, len(refs))
	assert.Equal(t, Ref{1, 14}, refs[0])
	assert.Equal(t, Ref{1, 26}, refs[12])

	refs, ok = convert(t, m, meta.Meta{Season: 1, SeasonEnd: 2}, 0)
	require.True(t, ok, "几组的合集")
	assert.Len(t, refs, 26)

	_, ok = convert(t, m, meta.Meta{Season: 1, SeasonEnd: 3}, 0)
	assert.False(t, ok, "第 3 组是空的")
	_, ok = convert(t, m, meta.Meta{Season: 3}, 0)
	assert.False(t, ok)
	_, ok = convert(t, m, meta.Meta{Season: 4, Episode: 1}, 0)
	assert.False(t, ok, "没有第 4 组")
	_, ok = convert(t, m, meta.Meta{Season: 1, Episode: 14}, 0)
	assert.False(t, ok, "第 1 组只有 13 集")

	refs, ok = convert(t, m, meta.Meta{Episode: 3}, 2)
	require.True(t, ok, "文件名里没写季：按种子标题写的组")
	assert.Equal(t, []Ref{{1, 16}}, refs)
}

func TestConvertSpecialsInsideGroup(t *testing.T) {
	// 组里夹着特别篇：组里的位置要算上它
	m := New(&tmdb.EpisodeGroup{Groups: []tmdb.EpisodeGroupPart{{Order: 1, Episodes: []tmdb.EpisodeGroupEpisode{
		{Season: 1, Episode: 1, Order: 0}, {Season: 0, Episode: 1, Order: 1}, {Season: 1, Episode: 2, Order: 2},
		{Season: 1, Episode: 0, Order: 3}, {Season: -1, Episode: 4, Order: 4},
	}}}})
	assert.Equal(t, 1, m.Parts())
	refs, ok := m.Convert(Coord{Season: 1, Episode: 2, EpisodeEnd: 3})
	require.True(t, ok)
	assert.Equal(t, []Ref{{0, 1}, {1, 2}}, refs, "集号或季号不对的集不要")
	_, ok = m.Convert(Coord{Season: 1, Episode: 4})
	assert.False(t, ok)
}

func TestConvertDedup(t *testing.T) {
	// 同一集在剧集组里出现两次（编辑写重了）：结果里只有一次
	m := New(&tmdb.EpisodeGroup{Groups: []tmdb.EpisodeGroupPart{{Episodes: []tmdb.EpisodeGroupEpisode{{Season: 1, Episode: 1}, {Season: 1, Episode: 1}, {Season: 1, Episode: 2}}}}})
	refs, ok := m.Convert(Coord{Season: 1})
	require.True(t, ok)
	assert.Equal(t, []Ref{{1, 1}, {1, 2}}, refs)
}

func TestNilAndInvalid(t *testing.T) {
	var m *Map
	assert.Nil(t, New(nil))
	assert.Equal(t, 0, m.Parts())
	assert.Equal(t, "剧集组", m.Label())
	_, ok := m.Convert(Coord{Season: 1, Episode: 1})
	assert.False(t, ok)
	_, ok = absolute().Convert(Coord{})
	assert.False(t, ok)
	assert.Equal(t, "剧集组「Absolute Order」", absolute().Label())
	assert.Equal(t, "剧集组", New(&tmdb.EpisodeGroup{}).Label())
}

func TestCoordOf(t *testing.T) {
	_, ok := CoordOf(meta.Meta{}, 3)
	assert.False(t, ok, "季与集都没写")
	c, ok := CoordOf(meta.Meta{Episode: 5}, 0)
	require.True(t, ok)
	assert.Equal(t, Coord{Season: 1, Episode: 5, EpisodeEnd: 5}, c)
	c, _ = CoordOf(meta.Meta{Episode: 5, EpisodeEnd: 7}, 2)
	assert.Equal(t, Coord{Season: 2, Episode: 5, EpisodeEnd: 7}, c)
	c, _ = CoordOf(meta.Meta{Season: 3, SeasonEnd: 4}, 2)
	assert.Equal(t, Coord{Season: 3, SeasonEnd: 4}, c, "写了季时不用种子标题的")

	assert.Equal(t, 2, TitleSeason(meta.Meta{Season: 2, Episode: 3}))
	assert.Equal(t, 1, TitleSeason(meta.Meta{Episode: 25}))
	assert.Equal(t, 0, TitleSeason(meta.Meta{}))
}

func TestCoordString(t *testing.T) {
	assert.Equal(t, "第 2 组第 13 集", Coord{Season: 2, Episode: 13, EpisodeEnd: 13}.String())
	assert.Equal(t, "第 2 组第 13–14 集", Coord{Season: 2, Episode: 13, EpisodeEnd: 14}.String())
	assert.Equal(t, "第 2 组", Coord{Season: 2}.String())
	assert.Equal(t, "第 1–3 组", Coord{Season: 1, SeasonEnd: 3, Episode: 4}.String())
}

func TestSeasonContiguous(t *testing.T) {
	refs := []Ref{{1, 24}, {2, 2}, {1, 23}, {2, 1}}
	assert.Equal(t, []int{23, 24}, Season(refs, 1))
	assert.Equal(t, []int{1, 2}, Season(refs, 2))
	assert.Empty(t, Season(refs, 3))
	assert.True(t, Contiguous([]int{3, 4, 5}))
	assert.True(t, Contiguous([]int{7}))
	assert.False(t, Contiguous([]int{3, 5}))
	assert.False(t, Contiguous(nil))
}
