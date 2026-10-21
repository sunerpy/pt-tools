package subscribe

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func varietyIndex(t *testing.T) *episodeIndex {
	t.Helper()
	var season tmdb.Season
	require.NoError(t, json.Unmarshal([]byte(varietySeason4), &season))
	return newEpisodeIndex(&season, []int{genreReality})
}

func varietySub() models.MediaSubscription {
	return models.MediaSubscription{MediaType: models.MediaKindTV, TMDBID: 231620, Season: 4, Title: "现在就出发", Aliases: `["Natural High"]`, Year: 2023, TotalEpisodes: 6}
}

// mapTitle 按订阅对一个标题：返回对到的集（整季是 0,0）与原因。
func mapTitle(t *testing.T, mp *episodeMapper, title string) (span, bool, string) {
	t.Helper()
	mt := newMatcher(varietySub()).withMapper(mp)
	return mt.matchMapped(meta.Parse(title, ""), v2.TorrentItem{Title: title})
}

// 语料来自 M-Team 上实际看到的标题（2026-10-21）：HHWEB 按 TMDB 的集号编，别的组写 Part 或第几期上下。
func TestEpisodeMappingCorpus(t *testing.T) {
	mp := newEpisodeMapper(varietyIndex(t), nil)
	cases := []struct {
		title      string
		start, end int
	}{
		{"Natural.High.S04E03.2023.1080p.TX.WEB-DL.H.264.DDP2.0-HHWEB", 3, 3},
		{"Natural High S04E01 Part 2 2026 1080p WEB-DL H264 AAC", 2, 2},
		{"Natural.High.S04E01.Part.2.2026.1080p.WEB-DL", 2, 2},
		{"Natural High 2026 S04E02 part1 2160p WEB-DL H265 DDP2.0-PTerWEB", 3, 3},
		{"现在就出发 第四季 第1期下 2026 1080p WEB-DL", 2, 2},
		{"现在就出发 第四季 第1期（下） 2026 1080p WEB-DL", 2, 2},
		{"现在就出发 第四季 第2期 2026 1080p WEB-DL", 3, 4},
		{"Natural.High.S04.20261002.1080p.WEB-DL-XX", 2, 2},
		{"现在就出发 第四季 1015期 2026 1080p WEB-DL", 5, 5},
	}
	for _, c := range cases {
		sp, ok, why := mapTitle(t, mp, c.title)
		require.True(t, ok, "%s：%s", c.title, why)
		assert.Equal(t, span{Start: c.start, End: c.end}, sp, c.title)
	}
	// 同一天两集、标题里只有日期，分不出是哪一集：不要
	_, ok, why := mapTitle(t, mp, "Natural.High.S04.20261008.1080p.WEB-DL-XX")
	assert.False(t, ok)
	assert.Contains(t, why, "集号对不上")
	// TMDB 上没有的期
	_, ok, why = mapTitle(t, mp, "现在就出发 第四季 第9期上 2026 1080p WEB-DL")
	assert.False(t, ok)
	assert.Contains(t, why, "第 9 期上")
	// 整季包照常是整季
	sp, ok, _ := mapTitle(t, mp, "Natural.High.S04.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB")
	assert.True(t, ok)
	assert.True(t, sp.Whole)
}

func TestEpisodeMappingRules(t *testing.T) {
	ix := varietyIndex(t)
	// 偏移：这个组的 E02 是 TMDB 的第 3 集
	mp := newEpisodeMapper(ix, []models.MediaEpisodeRule{{Group: "XX", Mode: models.MediaEpisodeRuleOffset, Offset: 1}})
	sp, ok, _ := mapTitle(t, mp, "Natural.High.S04E02.1080p.WEB-DL-XX")
	require.True(t, ok)
	assert.Equal(t, span{Start: 3, End: 3}, sp)
	sp, ok, _ = mapTitle(t, mp, "Natural.High.S04E02.1080p.WEB-DL-YY")
	require.True(t, ok)
	assert.Equal(t, span{Start: 2, End: 2}, sp, "别的组不受这条规则影响")

	// 按期号：这个组的 E02 是第 2 期（上下两集都在里面），E01 Part 2 是第 1 期下
	mp = newEpisodeMapper(ix, []models.MediaEpisodeRule{{Group: "PTerWEB", Mode: models.MediaEpisodeRulePeriodPart}})
	sp, ok, _ = mapTitle(t, mp, "Natural High 2026 S04E02 2160p WEB-DL H265-PTerWEB")
	require.True(t, ok)
	assert.Equal(t, span{Start: 3, End: 4}, sp)

	// 按 TMDB 集号的规则对只写了第几期的标题：认不出集号就不要
	mp = newEpisodeMapper(ix, []models.MediaEpisodeRule{{Mode: models.MediaEpisodeRuleTMDB}})
	_, ok, why := mapTitle(t, mp, "现在就出发 第四季 第1期下 2026 1080p WEB-DL")
	assert.False(t, ok)
	assert.Contains(t, why, "TMDB 集号")
	sp, ok, _ = mapTitle(t, mp, "Natural High S04E01 Part 2 2026 1080p WEB-DL")
	require.True(t, ok)
	assert.Equal(t, span{Start: 1, End: 1}, sp, "规则说按 TMDB 集号时 Part 不再换算")

	// 按日期的规则
	mp = newEpisodeMapper(ix, []models.MediaEpisodeRule{{Mode: models.MediaEpisodeRuleDate}})
	sp, ok, _ = mapTitle(t, mp, "Natural.High.S04.2026.10.15.1080p.WEB-DL")
	require.True(t, ok)
	assert.Equal(t, span{Start: 5, End: 5}, sp)
}

func TestEpisodeMappingNotVariety(t *testing.T) {
	var season tmdb.Season
	require.NoError(t, json.Unmarshal([]byte(`{"episodes":[{"episode_number":1,"name":"未来","air_date":"2025-04-13"},{"episode_number":2,"name":"穿越","air_date":"2025-04-20"}]}`), &season))
	ix := newEpisodeIndex(&season, nil)
	assert.False(t, ix.variety)
	mp := newEpisodeMapper(ix, nil)
	sub := models.MediaSubscription{MediaType: models.MediaKindTV, Season: 2, Title: "最后生还者", OriginalTitle: "The Last of Us"}
	mt := newMatcher(sub).withMapper(mp)
	title := "The.Last.of.Us.S02E01.Part.2.1080p.WEB-DL"
	_, ok, why := mt.matchMapped(meta.Parse(title, ""), v2.TorrentItem{Title: title})
	assert.False(t, ok, "不是按期编的剧集写了 Part：拿不准就不要")
	assert.Contains(t, why, "不是按期编的")
	title = "The.Last.of.Us.S02E02.1080p.WEB-DL"
	sp, ok, _ := mt.matchMapped(meta.Parse(title, ""), v2.TorrentItem{Title: title})
	require.True(t, ok)
	assert.Equal(t, span{Start: 2, End: 2}, sp)
}

func TestEpisodeIndexDetectsVarietyByNames(t *testing.T) {
	var season tmdb.Season
	require.NoError(t, json.Unmarshal([]byte(varietySeason4), &season))
	ix := newEpisodeIndex(&season, nil)
	assert.True(t, ix.variety, "一半以上的集名是第几期：没有类型也算综艺")
	n, ok := ix.byPart(2, "下")
	require.True(t, ok)
	assert.Equal(t, 4, n)
	_, ok = ix.byPart(5, "")
	assert.False(t, ok, "占位的集名不参与期号索引")
}

// 剧集的年份不参与匹配：综艺常写播出的年份（2026），TMDB 这一季是 2023。
func TestTVYearIgnored(t *testing.T) {
	sub := varietySub()
	title := "Natural High S04E01 Part 2 2026 1080p WEB-DL"
	_, ok := newMatcher(sub).match(meta.Parse(title, ""), v2.TorrentItem{Title: title})
	assert.True(t, ok)
}
