package transfer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mb = int64(1) << 20

func rels(fs []File) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Rel)
	}
	return out
}

func TestSelect(t *testing.T) {
	files := []File{
		{Rel: "Show.S01.1080p/Show.S01E01.1080p.mkv", Size: 900 * mb},
		{Rel: "Show.S01.1080p/Show.S01E02.1080p.MKV", Size: 900 * mb},
		{Rel: "Show.S01.1080p/Show.S01E01.1080p.chs.ass", Size: 1 * mb},
		{Rel: "Show.S01.1080p/Show.S01E01.1080p.eng.SRT", Size: 1 * mb},
		{Rel: "Show.S01.1080p/Sample/show.sample.mkv", Size: 80 * mb},
		{Rel: "Show.S01.1080p/Featurettes/Making Of.mkv", Size: 400 * mb},
		{Rel: "Show.S01.1080p/Show.S01E03-sample.mkv", Size: 60 * mb},
		{Rel: "Show.S01.1080p/tiny.mp4", Size: 10 * mb},
		{Rel: "Show.S01.1080p/Show.nfo", Size: 1024},
		{Rel: "Show.S01.1080p/poster.jpg", Size: 2 * mb},
		{Rel: "Show.S01.1080p/disc.iso", Size: 4000 * mb},
		{Rel: "Show.S01.1080p/Specials/Show.S00E01.mkv", Size: 700 * mb},
		{Rel: "[Group] Anime/[Group] Anime [NCOP01][1080p].mkv", Size: 120 * mb},
		{Rel: "[Group] Anime/SPs/[Group] Anime [Menu01].mkv", Size: 120 * mb},
		{Rel: "Trailer.Park.Boys.S01E01.mkv", Size: 500 * mb},
	}
	sel := Select(files, 0)
	assert.Empty(t, sel.Disc)
	assert.Equal(t, []string{
		"Show.S01.1080p/Show.S01E01.1080p.mkv",
		"Show.S01.1080p/Show.S01E02.1080p.MKV",
		"Show.S01.1080p/Specials/Show.S00E01.mkv",
		"Trailer.Park.Boys.S01E01.mkv",
	}, rels(sel.Videos), "Specials 是第 0 季正片；片名里的 Trailer 不当作预告")
	assert.Equal(t, []string{
		"Show.S01.1080p/Show.S01E01.1080p.chs.ass",
		"Show.S01.1080p/Show.S01E01.1080p.eng.SRT",
	}, rels(sel.Subtitles))
	reasons := map[string]string{}
	for _, s := range sel.Skipped {
		reasons[s.File.Rel] = s.Reason
	}
	assert.Contains(t, reasons["Show.S01.1080p/Sample/show.sample.mkv"], "Sample")
	assert.Contains(t, reasons["Show.S01.1080p/Featurettes/Making Of.mkv"], "Featurettes")
	assert.Contains(t, reasons["Show.S01.1080p/Show.S01E03-sample.mkv"], "样片")
	assert.Contains(t, reasons["Show.S01.1080p/tiny.mp4"], "50 MB")
	assert.Contains(t, reasons["Show.S01.1080p/disc.iso"], "ISO")
	assert.Contains(t, reasons["[Group] Anime/[Group] Anime [NCOP01][1080p].mkv"], "片头片尾")
	assert.Contains(t, reasons["[Group] Anime/SPs/[Group] Anime [Menu01].mkv"], "SPs")
	assert.NotContains(t, reasons, "Show.S01.1080p/Show.nfo", "不是视频的文件不列出")

	small := Select([]File{{Rel: "a.mkv", Size: 20 * mb}}, 10*mb)
	require.Len(t, small.Videos, 1, "可以调低最小体积")
}

func TestSelectDisc(t *testing.T) {
	for _, tc := range []struct{ rel, disc string }{
		{"Movie.2019.BluRay/BDMV/STREAM/00001.m2ts", "BDMV"},
		{"Movie DVD/VIDEO_TS/VTS_01_1.VOB", "VIDEO_TS"},
		{"bdmv/index.bdmv", "BDMV"},
	} {
		sel := Select([]File{{Rel: "Movie/cover.jpg", Size: mb}, {Rel: tc.rel, Size: 30000 * mb}}, 0)
		assert.Equal(t, tc.disc, sel.Disc, tc.rel)
		assert.Empty(t, sel.Videos, "原盘整个跳过")
	}
}

func TestIsVideoSubtitle(t *testing.T) {
	assert.True(t, IsVideo("a.M2TS"))
	assert.True(t, IsVideo("dir/a.rmvb"))
	assert.False(t, IsVideo("a.srt"))
	assert.True(t, IsSubtitle("a.SUP"))
	assert.False(t, IsSubtitle("a.mkv"))
}
