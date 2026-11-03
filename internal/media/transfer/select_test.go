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
		{Rel: "Show.S01.1080p/Sample/sample.chs.srt", Size: 0},
		{Rel: "Show.S01.1080p/Extras/commentary.srt", Size: 0},
	}
	sel := Select(files, 0)
	assert.Empty(t, sel.Disc)
	assert.Equal(t, []string{
		"Show.S01.1080p/Show.S01E01.1080p.mkv",
		"Show.S01.1080p/Show.S01E02.1080p.MKV",
		"Show.S01.1080p/disc.iso",
		"Show.S01.1080p/Specials/Show.S00E01.mkv",
		"Trailer.Park.Boys.S01E01.mkv",
	}, rels(sel.Videos), "Specials 是第 0 季正片；片名里的 Trailer 不当作预告；ISO 当作视频（剧集里的 ISO 由整理那边跳过）")
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
	assert.Contains(t, reasons["[Group] Anime/[Group] Anime [NCOP01][1080p].mkv"], "片头片尾")
	assert.Contains(t, reasons["[Group] Anime/SPs/[Group] Anime [Menu01].mkv"], "SPs")
	assert.Contains(t, reasons["Show.S01.1080p/Sample/sample.chs.srt"], "Sample", "花絮、样片目录里的字幕也不整理")
	assert.Contains(t, reasons["Show.S01.1080p/Extras/commentary.srt"], "Extras")
	assert.NotContains(t, reasons, "Show.S01.1080p/Show.nfo", "不是视频的文件不列出")

	small := Select([]File{{Rel: "a.mkv", Size: 20 * mb}}, 10*mb)
	require.Len(t, small.Videos, 1, "可以调低最小体积")

	iso := Select([]File{
		{Rel: "Movie.2020/Movie.2020.ISO", Size: 8000 * mb},
		{Rel: "Movie.2020/sample.iso", Size: 20 * mb},
		{Rel: "Movie.2020/Extras/bonus.iso", Size: 900 * mb},
	}, 0)
	assert.Equal(t, []string{"Movie.2020/Movie.2020.ISO"}, rels(iso.Videos), "ISO 照视频的规则挑：太小的、花絮目录里的不要")
	assert.Len(t, iso.Skipped, 2)
}

func TestSelectDisc(t *testing.T) {
	files := []File{
		{Rel: "Movie.2019.BluRay/BDMV/index.bdmv", Path: "/dl/Movie.2019.BluRay/BDMV/index.bdmv", Size: 1024},
		{Rel: "Movie.2019.BluRay/BDMV/STREAM/00001.m2ts", Path: "/dl/Movie.2019.BluRay/BDMV/STREAM/00001.m2ts", Size: 30000 * mb},
		{Rel: "Movie.2019.BluRay/BDMV/STREAM/00002.m2ts", Path: "/dl/Movie.2019.BluRay/BDMV/STREAM/00002.m2ts", Size: 300 * mb},
		{Rel: "Movie.2019.BluRay/BDMV/STREAM/SSIF/00001.ssif", Path: "/dl/Movie.2019.BluRay/BDMV/STREAM/SSIF/00001.ssif", Size: 31000 * mb},
		{Rel: "Movie.2019.BluRay/CERTIFICATE/id.bdmv", Path: "/dl/Movie.2019.BluRay/CERTIFICATE/id.bdmv", Size: 1024},
		{Rel: "Movie.2019.BluRay/Movie.2019.nfo", Path: "/dl/Movie.2019.BluRay/Movie.2019.nfo", Size: 1024},
		{Rel: "Movie.2019.BluRay/sample.mkv", Path: "/dl/Movie.2019.BluRay/sample.mkv", Size: 90 * mb},
		{Rel: "Movie.2019.BluRay/Subs/movie.chs.srt", Path: "/dl/Movie.2019.BluRay/Subs/movie.chs.srt", Size: mb},
	}
	sel := Select(files, 0)
	assert.Equal(t, "BDMV", sel.Disc)
	assert.Empty(t, sel.Videos, "原盘的种子不再挑散的视频")
	assert.Empty(t, sel.Subtitles)
	require.Len(t, sel.Discs, 1)
	d := sel.Discs[0]
	assert.Equal(t, "BDMV", d.Kind)
	assert.Equal(t, "Movie.2019.BluRay", d.Root)
	assert.Equal(t, "/dl/Movie.2019.BluRay", d.Path)
	ins := make([]string, 0, len(d.Files))
	for _, f := range d.Files {
		ins = append(ins, f.In)
	}
	assert.Equal(t, []string{
		"BDMV/index.bdmv", "BDMV/STREAM/00001.m2ts", "BDMV/STREAM/00002.m2ts", "BDMV/STREAM/SSIF/00001.ssif", "CERTIFICATE/id.bdmv",
	}, ins, "只放原盘结构目录里的文件，保持相对原盘根目录的路径")
	require.GreaterOrEqual(t, d.Main, 0)
	assert.Equal(t, "BDMV/STREAM/00001.m2ts", d.Files[d.Main].In, "主文件是 STREAM 里最大的 m2ts（SSIF 不算）")
	assert.Equal(t, 30000*mb+300*mb+31000*mb+2048, d.Size)
	reasons := map[string]string{}
	for _, s := range sel.Skipped {
		reasons[s.File.Rel] = s.Reason
	}
	assert.Len(t, reasons, 3)
	assert.Contains(t, reasons["Movie.2019.BluRay/sample.mkv"], "原盘以外")
	assert.Contains(t, reasons["Movie.2019.BluRay/Subs/movie.chs.srt"], "原盘以外")

	dvd := Select([]File{
		{Rel: "Movie DVD/VIDEO_TS/VIDEO_TS.IFO", Path: "/w/Movie DVD/VIDEO_TS/VIDEO_TS.IFO", Size: mb},
		{Rel: "Movie DVD/VIDEO_TS/VTS_01_0.VOB", Path: "/w/Movie DVD/VIDEO_TS/VTS_01_0.VOB", Size: 2 * mb},
		{Rel: "Movie DVD/VIDEO_TS/VTS_01_1.VOB", Path: "/w/Movie DVD/VIDEO_TS/VTS_01_1.VOB", Size: 1000 * mb},
		{Rel: "Movie DVD/AUDIO_TS/x", Path: "/w/Movie DVD/AUDIO_TS/x", Size: 1},
		{Rel: "Other/readme.txt", Path: "/w/Other/readme.txt", Size: 1},
	}, 0)
	require.Len(t, dvd.Discs, 1)
	assert.Equal(t, "VIDEO_TS", dvd.Discs[0].Kind)
	assert.Equal(t, "Movie DVD", dvd.Discs[0].Root)
	assert.Equal(t, "/w/Movie DVD", dvd.Discs[0].Path)
	assert.Len(t, dvd.Discs[0].Files, 4)
	assert.Equal(t, "VIDEO_TS/VTS_01_1.VOB", dvd.Discs[0].Files[dvd.Discs[0].Main].In)
	require.Len(t, dvd.Skipped, 1, "原盘根目录以外的文件")

	// 目录名不分大小写；没有视频流时没有主文件；BDMV 直接在保存目录里时根目录为空
	bare := Select([]File{{Rel: "bdmv/index.bdmv", Path: "/dl/bdmv/index.bdmv", Size: 1024}}, 0)
	require.Len(t, bare.Discs, 1)
	assert.Equal(t, "BDMV", bare.Discs[0].Kind)
	assert.Empty(t, bare.Discs[0].Root)
	assert.Equal(t, "/dl", bare.Discs[0].Path)
	assert.Equal(t, -1, bare.Discs[0].Main)

	// 两张原盘（一张在另一张的目录里也算）；BDMV 与 VIDEO_TS 在同一个根目录里是一张，按 BDMV
	two := Select([]File{
		{Rel: "Box/Disc1/BDMV/STREAM/1.m2ts", Path: "/dl/Box/Disc1/BDMV/STREAM/1.m2ts", Size: 9000 * mb},
		{Rel: "Box/Disc2/BDMV/STREAM/1.m2ts", Path: "/dl/Box/Disc2/BDMV/STREAM/1.m2ts", Size: 9000 * mb},
		{Rel: "Box/Disc2/Inner/BDMV/STREAM/1.m2ts", Path: "/dl/Box/Disc2/Inner/BDMV/STREAM/1.m2ts", Size: 9000 * mb},
		{Rel: "Box/Disc2/CERTIFICATE/id.bdmv", Path: "/dl/Box/Disc2/CERTIFICATE/id.bdmv", Size: 1},
	}, 0)
	require.Len(t, two.Discs, 3)
	assert.Equal(t, []string{"Box/Disc1", "Box/Disc2", "Box/Disc2/Inner"}, []string{two.Discs[0].Root, two.Discs[1].Root, two.Discs[2].Root})
	assert.Len(t, two.Discs[1].Files, 2, "CERTIFICATE 归到最近的那个根目录")
	assert.Len(t, two.Discs[2].Files, 1, "里面那张盘的文件归它自己，不当作外面那张的「原盘以外的文件」")
	hybrid := Select([]File{
		{Rel: "H/VIDEO_TS/VTS_01_1.VOB", Path: "/dl/H/VIDEO_TS/VTS_01_1.VOB", Size: 900 * mb},
		{Rel: "H/BDMV/STREAM/1.m2ts", Path: "/dl/H/BDMV/STREAM/1.m2ts", Size: 9000 * mb},
	}, 0)
	require.Len(t, hybrid.Discs, 1)
	assert.Equal(t, "BDMV", hybrid.Discs[0].Kind)
	assert.Equal(t, "BDMV/STREAM/1.m2ts", hybrid.Discs[0].Files[hybrid.Discs[0].Main].In)
}

func TestIsVideoSubtitle(t *testing.T) {
	assert.True(t, IsISO("dir/A.ISO"))
	assert.False(t, IsVideo("a.iso"), "ISO 不算普通的视频文件（媒体库里另外认，见 IsLibraryVideo）")
	assert.True(t, IsLibraryVideo("a.iso"))
	assert.True(t, IsLibraryVideo("VTS_01_1.VOB"))
	assert.True(t, IsLibraryVideo("00001.m2ts"))
	assert.False(t, IsLibraryVideo("poster.jpg"))
	assert.True(t, IsVideo("a.M2TS"))
	assert.True(t, IsVideo("dir/a.rmvb"))
	assert.False(t, IsVideo("a.srt"))
	assert.True(t, IsSubtitle("a.SUP"))
	assert.False(t, IsSubtitle("a.mkv"))
}
