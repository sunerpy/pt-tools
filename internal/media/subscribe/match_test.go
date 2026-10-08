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
