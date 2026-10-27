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

	// 期号只在副标题里：要和标题的集号对得上。SpringSunday 上 HHWEB 的第 4 集副标题写成「第04期下」（其实是第2期下），
	// 等 TMDB 列出第4期下（第 8 集）以后会被当成第 8 集；对不上的不要
	title := "Natural.High.S04E04.2026.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB"
	_, ok, why = mapItem(t, mp, title, "现在就出发 / NATURAL HIGH 第四季 | 第04集 (第04期下)")
	assert.False(t, ok, "TMDB 上还没有第4期下")
	assert.Contains(t, why, "标题写的是第 4 集，副标题写的是第 4 期下")
	sp, ok, why = mapItem(t, mp, title, "现在就出发 第四季 | 第04集（第2期下）")
	require.True(t, ok, why)
	assert.Equal(t, span{Start: 4, End: 4}, sp, "两边对得上")
	_, ok, why = mapItem(t, mp, title, "现在就出发 第四季 | 第1期上")
	assert.False(t, ok, "副标题说是第 1 集")
	assert.Contains(t, why, "副标题写的是第 1 期上")
	sp, ok, why = mapItem(t, mp, title, "现在就出发 / NATURAL HIGH 第四季 | 第04集 | 4K")
	require.True(t, ok, why)
	assert.Equal(t, span{Start: 4, End: 4}, sp, "副标题没写期号：按标题")
	// 标题自己写了期号的（Part、第N期）照旧按期号
	sp, ok, why = mapItem(t, mp, "Natural High S04E02 Part 1 2026 1080p WEB-DL H.264 AAC 2.0", "现在就出发 第四季 | 第二集上")
	require.True(t, ok, why)
	assert.Equal(t, span{Start: 3, End: 3}, sp)
}

// mapItem 是带副标题的 mapTitle。
func mapItem(t *testing.T, mp *episodeMapper, title, subtitle string) (span, bool, string) {
	t.Helper()
	mt := newMatcher(varietySub()).withMapper(mp)
	return mt.matchMapped(meta.Parse(title, subtitle), v2.TorrentItem{Title: title, Subtitle: subtitle})
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

// productionSeason4 是 2026-10-27 生产上 TMDB 的现在就出发第 4 季：第 1–3 期（6 集）有名字、已播出，后面是占位的集名。
const productionSeason4 = `{"season_number":4,"episodes":[
	{"episode_number":1,"name":"第1期上：看见蔚蓝生机","air_date":"2026-10-10"},
	{"episode_number":2,"name":"第1期下：神秘的热带雨林","air_date":"2026-10-11"},
	{"episode_number":3,"name":"第2期上：哞哞来了！","air_date":"2026-10-17"},
	{"episode_number":4,"name":"第2期下：热舞的哞哞","air_date":"2026-10-18"},
	{"episode_number":5,"name":"第3期上：神奇的九寨","air_date":"2026-10-24"},
	{"episode_number":6,"name":"第3期下：松潘之旅","air_date":"2026-10-25"},
	{"episode_number":7,"name":"第 7 集","air_date":"2026-10-31"},
	{"episode_number":8,"name":"第 8 集","air_date":"2026-11-01"},
	{"episode_number":9,"name":"第 9 集","air_date":"2026-11-07"}]}`

// SpringSunday 上 HHWEB 的单集副标题把集号写成了期号（第 5 集写「第5期上」，其实是第3期上）。副标题写的那一期按 TMDB 的
// 播出进度还不可能播出时（才播到第 3 期），当作副标题写错了、按标题的集号算；能核对的（TMDB 上有那一期）照旧要对得上。
func TestSubtitlePeriodTooNewTrustsTitle(t *testing.T) {
	var season tmdb.Season
	require.NoError(t, json.Unmarshal([]byte(productionSeason4), &season))
	mp := newEpisodeMapper(newEpisodeIndex(&season, []int{genreReality}), nil)
	mp.today = "2026-10-27"
	cases := []struct {
		title, subtitle string
		ep              int
	}{
		{"Natural.High.S04E05.2026.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "现在就出发 第四季 | 第05集（第5期上）", 5},
		{"Natural.High.S04E06.2026.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "现在就出发 第四季 | 第06集（第6期下）", 6},
		{"Natural.High.S04E05.2026.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "现在就出发 第四季 | 第3期上", 5},
	}
	for _, c := range cases {
		sp, ok, why := mapItem(t, mp, c.title, c.subtitle)
		require.True(t, ok, "%s / %s：%s", c.title, c.subtitle, why)
		assert.Equal(t, span{Start: c.ep, End: c.ep}, sp, c.subtitle)
	}

	// TMDB 上有那一期、和标题对不上：拿不准，照旧不要，并提示可以设集号规则
	_, ok, why := mapItem(t, mp, "Natural.High.S04E05.2026.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "现在就出发 第四季 | 第2期上")
	assert.False(t, ok)
	assert.Contains(t, why, "标题写的是第 5 集，副标题写的是第 2 期上")
	assert.Contains(t, why, "集号规则")
	// 副标题写的是下一期（第 4 期，会员可能已经提前看到）：拿不准，不要
	_, ok, _ = mapItem(t, mp, "Natural.High.S04E06.2026.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "现在就出发 第四季 | 第4期上")
	assert.False(t, ok)
	_, ok, _ = mapItem(t, mp, "Natural.High.S04E04.2026.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "现在就出发 第四季 | 第04集（第4期下）")
	assert.False(t, ok, "第 4 期离播出进度只差一期，留着余量")
	// 标题的那一集还没播出：不要
	_, ok, _ = mapItem(t, mp, "Natural.High.S04E07.2026.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "现在就出发 第四季 | 第07集（第7期上）")
	assert.False(t, ok)
	// 不知道今天是哪天时不判断（照旧不要）
	mp.today = ""
	_, ok, _ = mapItem(t, mp, "Natural.High.S04E05.2026.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "现在就出发 第四季 | 第05集（第5期上）")
	assert.False(t, ok)
}
