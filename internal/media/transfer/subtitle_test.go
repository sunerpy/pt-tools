package transfer

import (
	"path"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSubtitleLang(t *testing.T) {
	for name, want := range map[string]string{
		"Movie.2023.1080p.chs.ass":          "zh-CN",
		"Movie.2023.1080p.chs&eng.ass":      "zh-CN",
		"Movie.2023.简体中文.srt":               "zh-CN",
		"Movie.2023.zh-CN.srt":              "zh-CN",
		"Movie.2023.zh-Hans.srt":            "zh-CN",
		"Movie.2023.SC.ass":                 "zh-CN",
		"Movie.2023.cht.ass":                "zh-TW",
		"Movie.2023.繁體.ass":                 "zh-TW",
		"Movie.2023.zh-TW.srt":              "zh-TW",
		"Movie.2023.Big5.srt":               "zh-TW",
		"Movie.2023.chi.srt":                "zh",
		"Movie.2023.zh.srt":                 "zh",
		"[Group] Anime - 01 [中文字幕].ass":     "zh",
		"Movie.2023.eng.srt":                "en",
		"Movie.2023.English.SDH.srt":        "en",
		"Movie.2023.jpn.ass":                "ja",
		"Movie.2023.kor.srt":                "ko",
		"Movie.2023.1080p.BluRay.srt":       "",
		"The.Ten.Commandments.1956.srt":     "",
		"dir/Movie.2023.chs/Movie.2023.ass": "",
	} {
		assert.Equal(t, want, SubtitleLang(name), name)
	}
}

func TestSubtitleTarget(t *testing.T) {
	v := "/lib/Movie (2023)/Movie (2023) - 1080p.mkv"
	assert.Equal(t, "/lib/Movie (2023)/Movie (2023) - 1080p.zh-CN.ass", SubtitleTarget(v, "x.chs.ASS", 1))
	assert.Equal(t, "/lib/Movie (2023)/Movie (2023) - 1080p.zh-CN.2.srt", SubtitleTarget(v, "x.chs.srt", 2))
	assert.Equal(t, "/lib/Movie (2023)/Movie (2023) - 1080p.srt", SubtitleTarget(v, "x.srt", 1))
	assert.Equal(t, "/lib/Movie (2023)/Movie (2023) - 1080p.12.srt", SubtitleTarget(v, "x.srt", 12))
}

var epRe = regexp.MustCompile(`(?i)S(\d+)E(\d+)`)

func epKey(f File) string {
	if m := epRe.FindStringSubmatch(path.Base(f.Rel)); m != nil {
		return m[1] + "x" + m[2]
	}
	return ""
}

func TestMatchSubtitles(t *testing.T) {
	one := []File{{Rel: "M/Movie.2023.mkv"}}
	subs := []File{{Rel: "M/Subs/1.srt"}, {Rel: "M/chs.ass"}}
	got := MatchSubtitles(one, subs, nil)
	assert.Equal(t, [][]File{subs}, got, "只有一个视频时字幕全给它")

	videos := []File{
		{Rel: "S/Show.S01E01.1080p.mkv"},
		{Rel: "S/Show.S01E02.1080p.mkv"},
		{Rel: "S/Show.S01E02.1080p.Directors.Cut.mkv"},
	}
	subs = []File{
		{Rel: "S/Show.S01E01.1080p.chs.ass"},
		{Rel: "S/Subs/S01E02.eng.srt"},
		{Rel: "S/Show.S01E02.1080p.Directors.Cut.chs.ass"},
		{Rel: "S/Subs/random.srt"},
	}
	got = MatchSubtitles(videos, subs, epKey)
	assert.Equal(t, []File{subs[0]}, got[0])
	assert.Empty(t, got[1], "S01E02 有两个视频，按集数分不清就不分")
	assert.Equal(t, []File{subs[2]}, got[2], "前缀最长的那个视频")

	got = MatchSubtitles(videos[:2], subs[:2], epKey)
	assert.Equal(t, []File{subs[1]}, got[1], "按季集分")
	assert.Len(t, MatchSubtitles(nil, subs, epKey), 0)
}
