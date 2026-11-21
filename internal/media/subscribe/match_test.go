package subscribe

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestMatchMovie(t *testing.T) {
	mt := newMatcher(models.MediaSubscription{
		MediaType: models.MediaKindMovie, TMDBID: 693134, Title: "沙丘2", OriginalTitle: "Dune: Part Two", Year: 2024,
		Aliases: encodeList([]string{"Dune Part Two"}), IMDbID: "tt15239678",
	})
	ok := func(title, sub string, it v2.TorrentItem) bool {
		_, got := mt.match(meta.Parse(title, sub), it)
		return got
	}
	assert.True(t, ok("Dune.Part.Two.2024.2160p.UHD.BluRay.x265-FRDS", "", v2.TorrentItem{}))
	assert.True(t, ok("Dune.Part.Two.2023.1080p.WEB-DL.x264-X", "", v2.TorrentItem{}), "年份差 1 年也算")
	assert.False(t, ok("Dune.Part.Two.2021.1080p.WEB-DL.x264-X", "", v2.TorrentItem{}), "年份差太多")
	assert.True(t, ok("Some.Weird.Name.2024.1080p.WEB-DL.x264-X", "沙丘2 | 中字", v2.TorrentItem{}), "副标题里的中文名")
	assert.False(t, ok("Dune.2021.1080p.BluRay.x264-X", "沙丘", v2.TorrentItem{}), "前作不算")
	assert.True(t, ok("Totally.Different.2024.1080p.WEB-DL.x264-X", "", v2.TorrentItem{IMDbID: "tt15239678"}), "IMDb 编号对上就算")
	assert.False(t, ok("Dune.Part.Two.2024.1080p.WEB-DL.x264-X", "", v2.TorrentItem{IMDbID: "tt0000001"}), "编号不同时名字对上也不算")
	assert.False(t, ok("Dune.Part.Two.S01E01.2024.1080p.WEB-DL.x264-X", "", v2.TorrentItem{}), "剧集不算电影")
	assert.False(t, ok("Dune.Part.Two.1080p.WEB-DL.x264-X", "", v2.TorrentItem{}), "没有年份、没有 IMDb 编号时认不准（同名的不同年份）")
	assert.True(t, ok("Dune.Part.Two.1080p.WEB-DL.x264-X", "", v2.TorrentItem{IMDbID: "tt15239678"}), "没有年份时 IMDb 编号对上也算")
	assert.True(t, ok("Dune.Part.Two.1080p.WEB-DL.x264-X", "沙丘2 2024 | 中字", v2.TorrentItem{}), "年份写在副标题里也算")
}

func TestMatchTV(t *testing.T) {
	mt := newMatcher(models.MediaSubscription{MediaType: models.MediaKindTV, TMDBID: 100088, Title: "最后生还者", OriginalTitle: "The Last of Us", Season: 2})
	sp := func(title string) (span, bool) { return mt.match(meta.Parse(title, ""), v2.TorrentItem{}) }

	s, ok := sp("The.Last.of.Us.S02E03.1080p.WEB-DL.H264-X")
	assert.True(t, ok)
	assert.Equal(t, span{Start: 3, End: 3}, s)
	s, ok = sp("The.Last.of.Us.S02E01-E04.1080p.WEB-DL.H264-X")
	assert.True(t, ok)
	assert.Equal(t, span{Start: 1, End: 4}, s)
	s, ok = sp("The.Last.of.Us.S02.2160p.WEB-DL.H265-X")
	assert.True(t, ok)
	assert.True(t, s.Whole)
	s, ok = sp("The.Last.of.Us.S01-S02.2160p.WEB-DL.H265-X")
	assert.True(t, ok, "多季合集里有这一季")
	assert.True(t, s.Whole)
	_, ok = sp("The.Last.of.Us.S01E03.1080p.WEB-DL.H264-X")
	assert.False(t, ok, "别的季")
	_, ok = sp("The.Last.of.Us.2023.1080p.WEB-DL.H264-X")
	assert.False(t, ok, "没写季的不算")

	anime := newMatcher(models.MediaSubscription{MediaType: models.MediaKindTV, Title: "葬送的芙莉莲", OriginalTitle: "Sousou no Frieren", Season: 1})
	s, ok = anime.match(meta.Parse("[QA] Sousou no Frieren - 05 [1080p]", ""), v2.TorrentItem{})
	assert.True(t, ok, "只写了集的动画当作第 1 季")
	assert.Equal(t, span{Start: 5, End: 5}, s)
}

func TestSpan(t *testing.T) {
	assert.Equal(t, 12, span{Whole: true}.count(12))
	assert.Equal(t, 1, span{Whole: true}.count(0))
	assert.Equal(t, 4, span{Start: 1, End: 4}.count(12))
	assert.Equal(t, 1, span{}.count(0))
	assert.True(t, span{Start: 2, End: 4}.covers(3))
	assert.False(t, span{Start: 2, End: 4}.covers(5))
	assert.True(t, span{Whole: true}.covers(99))
}

// 2026-11-21 生产：《谷雨》（2026）的英文名 The Silent Storm 和《宿敌》（2024）一样，MWeb 的《宿敌》单集只按英文名就对上了，
// 第 11、12、15、16 集下成了《宿敌》。副标题写的是另一部剧的中文名、年份又比这部第一季的播出早两年以上时，不是这部剧。
func TestMatchRejectsOtherShowWithSameEnglishName(t *testing.T) {
	mt := newMatcher(models.MediaSubscription{
		MediaType: models.MediaKindTV, TMDBID: 279155, Title: "谷雨", OriginalTitle: "谷雨", Season: 1, Year: 2026,
		Aliases: encodeList([]string{"The Silent Storm", "穀雨", "곡우"}),
	})
	check := func(title, subtitle string, it v2.TorrentItem) (bool, string) {
		it.Title, it.Subtitle = title, subtitle
		_, ok, why := mt.matchMapped(meta.Parse(title, subtitle), it)
		return ok, why
	}

	ok, why := check("The Silent Storm S01E11 2160p TX WEB-DL AAC2.0 H.265-MWeb", "宿敌 | 2024 | 中国大陆 | 剧情 悬疑 犯罪 | 奇道 | 廖凡 朱珠 | 第1季第11集", v2.TorrentItem{})
	assert.False(t, ok, "副标题是《宿敌》、2024 年")
	assert.Contains(t, why, "宿敌")
	assert.False(t, mt.mayMatch(meta.Parse("The Silent Storm S01E11 2160p TX WEB-DL AAC2.0 H.265-MWeb", "宿敌 | 2024 | 第1季第11集"),
		v2.TorrentItem{Title: "The Silent Storm S01E11 2160p TX WEB-DL AAC2.0 H.265-MWeb", Subtitle: "宿敌 | 2024 | 第1季第11集"}), "RSS 的快速筛也不要")

	ok, _ = check("The Silent Storm S01E11-E16 2026 2160p WEB-DL AAC H265 60fps HDR-HDSWEB", "谷雨 第11-16集 | 主演: 井柏然 孙艺洲 张颂文 [60帧]", v2.TorrentItem{})
	assert.True(t, ok, "真的《谷雨》")
	ok, _ = check("The Silent Storm S01E11 2024 2160p WEB-DL H265-X", "穀雨 第11集", v2.TorrentItem{})
	assert.True(t, ok, "副标题里有这部剧的别名：年份写错也照收")
	ok, _ = check("The Silent Storm S01E11 2024 2160p WEB-DL H265-X", "", v2.TorrentItem{})
	assert.True(t, ok, "没有副标题时看不出是哪一部，照旧按名字")
	ok, _ = check("The Silent Storm S01E11 2160p WEB-DL H265-X", "宿敌 第11集", v2.TorrentItem{})
	assert.True(t, ok, "没写年份时只凭中文名不下结论（副标题的中文名常常是分类或活动字样）")
	ok, _ = check("The Silent Storm S01E11 2025 2160p WEB-DL H265-X", "宿敌 第11集", v2.TorrentItem{})
	assert.True(t, ok, "只早一年不算（有的写制作年份）")
	ok, _ = check("The Silent Storm S01E11 2160p TX WEB-DL H.265-MWeb", "宿敌 | 2024 | 第1季第11集", v2.TorrentItem{IMDbID: "tt0000001"})
	assert.False(t, ok, "IMDb 编号照旧优先：订阅没有编号时不看编号")

	// 布鲁伊：副标题解析出的中文名是活动、分类字样，但副标题里有这部剧的名字
	bluey := newMatcher(models.MediaSubscription{MediaType: models.MediaKindTV, Title: "布鲁伊", OriginalTitle: "Bluey", Season: 1, Year: 2018})
	for _, c := range [][2]string{
		{"Bluey S01 2018 2160p WEB-DL H.265 AAC-CHDWEB", "*活動置頂220*布鲁伊 第一季/Bluey Season 1 全52集 | 国语/简中硬字幕"},
		{"Bluey S01 2016 1080p WEB-DL H.264-HDSWEB", "动漫: 布鲁伊 全52集 [内封简繁英多国软字幕]"},
	} {
		_, got, reason := bluey.matchMapped(meta.Parse(c[0], c[1]), v2.TorrentItem{Title: c[0], Subtitle: c[1]})
		assert.True(t, got, "%s：%s", c[0], reason)
	}

	// 后面的季：种子常写这部剧第一季的年份，订阅的年份是这一季的，不按年份判断
	s2 := newMatcher(models.MediaSubscription{MediaType: models.MediaKindTV, Title: "最后生还者", OriginalTitle: "The Last of Us", Season: 2, Year: 2025})
	_, ok, why = s2.matchMapped(meta.Parse("The.Last.of.Us.S02E03.2023.1080p.WEB-DL.H264-X", "国语中字分集"),
		v2.TorrentItem{Title: "The.Last.of.Us.S02E03.2023.1080p.WEB-DL.H264-X", Subtitle: "国语中字分集"})
	assert.True(t, ok, why)
}
