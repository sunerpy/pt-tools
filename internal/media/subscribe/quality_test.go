package subscribe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func cand(title, sub string, mod func(*v2.TorrentItem)) Candidate {
	it := v2.TorrentItem{ID: "1", Title: title, Subtitle: sub, SizeBytes: 20 * gib, Seeders: 10}
	if mod != nil {
		mod(&it)
	}
	return Candidate{Site: "s", Item: it, Meta: meta.Parse(title, sub), From: fromSearch}
}

func TestScoreOrdersByResolutionSourceCodec(t *testing.T) {
	p := Profile{}
	uhd := p.Score(cand("Dune.Part.Two.2024.2160p.UHD.BluRay.x265-FRDS", "", nil), 1)
	bd1080 := p.Score(cand("Dune.Part.Two.2024.1080p.BluRay.x264-CMCT", "", nil), 1)
	web2160 := p.Score(cand("Dune.Part.Two.2024.2160p.WEB-DL.H265-HHWEB", "", nil), 1)
	web1080 := p.Score(cand("Dune.Part.Two.2024.1080p.WEB-DL.H264-HHWEB", "", nil), 1)
	for _, v := range []Verdict{uhd, bd1080, web2160, web1080} {
		require.Empty(t, v.Reject)
	}
	assert.Greater(t, uhd.Score, web2160.Score, "同分辨率时来源好的高")
	assert.Greater(t, web2160.Score, bd1080.Score, "分辨率比来源重要")
	assert.Greater(t, bd1080.Score, web1080.Score)

	// 档案里排了编码：同分辨率与来源时按编码
	pc := Profile{Codecs: []string{"H.265", "H.264"}}
	h265 := pc.Score(cand("A.2024.1080p.WEB-DL.H265-X", "", nil), 1)
	h264 := pc.Score(cand("A.2024.1080p.WEB-DL.H264-X", "", nil), 1)
	assert.Greater(t, h265.Score, h264.Score)
	assert.Contains(t, pc.Score(cand("A.2024.1080p.WEB-DL.AV1-X", "", nil), 1).Reject, "编码 AV1 不在档案里")
}

func TestScoreHardRules(t *testing.T) {
	cases := []struct {
		name   string
		p      Profile
		c      Candidate
		eps    int
		reject string
	}{
		{"分辨率", Profile{Resolutions: []string{"2160p"}}, cand("A.2024.1080p.BluRay.x264-X", "", nil), 1, "分辨率 1080p 不在档案里"},
		{"分辨率认不出来", Profile{Resolutions: []string{"2160p"}}, cand("A.2024.BluRay.x264-X", "", nil), 1, "分辨率（认不出来）不在档案里"},
		{"来源", Profile{Sources: []string{"BluRay"}}, cand("A.2024.1080p.WEB-DL.x264-X", "", nil), 1, "来源 WEB-DL 不在档案里"},
		{"必须 Remux", Profile{Remux: models.MediaPrefRequire}, cand("A.2024.1080p.BluRay.x264-X", "", nil), 1, "没有Remux"},
		{"不要 HDR", Profile{HDR: models.MediaPrefAvoid}, cand("A.2024.2160p.WEB-DL.DV.HDR.H265-X", "", nil), 1, "有HDR"},
		{"必须中字", Profile{ChineseSubs: models.MediaPrefRequire}, cand("A.2024.1080p.WEB-DL.H264-X", "沙丘", nil), 1, "没有中字"},
		{"必须免费", Profile{Free: models.MediaPrefRequire}, cand("A.2024.1080p.WEB-DL.H264-X", "", nil), 1, "不是免费"},
		{"排除 H&R", Profile{ExcludeHR: true}, cand("A.2024.1080p.WEB-DL.H264-X", "", func(it *v2.TorrentItem) { it.HasHR = true }), 1, "H&R"},
		{"做种数", Profile{MinSeeders: 20}, cand("A.2024.1080p.WEB-DL.H264-X", "", nil), 1, "做种 10 个，少于 20 个"},
		{"太大", Profile{MaxSizeGB: 15}, cand("A.2024.1080p.WEB-DL.H264-X", "", nil), 1, "大于 15.0 GB"},
		{"剧集按每集算", Profile{MaxSizeGB: 15}, cand("Show.S01.1080p.WEB-DL.H264-X", "", nil), 10, ""},
		{"太小", Profile{MinSizeGB: 3}, cand("Show.S01.1080p.WEB-DL.H264-X", "", nil), 10, "小于 3.0 GB"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := c.p.Score(c.c, c.eps)
			if c.reject == "" {
				assert.Empty(t, v.Reject)
				return
			}
			assert.Contains(t, v.Reject, c.reject)
		})
	}

	// RSS 里的种子不看做种数；站点整站 H&R 也算
	rss := cand("A.2024.1080p.WEB-DL.H264-X", "", func(it *v2.TorrentItem) { it.Seeders = 0 })
	rss.From = fromRSS
	assert.Empty(t, Profile{MinSeeders: 5}.Score(rss, 1).Reject)
	rss.SiteHR = true
	assert.Contains(t, Profile{ExcludeHR: true}.Score(rss, 1).Reject, "H&R")
}

func TestScoreBonuses(t *testing.T) {
	p := Profile{Remux: models.MediaPrefPrefer, HDR: models.MediaPrefPrefer, ChineseSubs: models.MediaPrefPrefer, Groups: []string{"frds"}}
	base := p.Score(cand("A.2024.2160p.BluRay.x265-CMCT", "", nil), 1).Score
	assert.Equal(t, base+bonusRemux, p.Score(cand("A.2024.2160p.BluRay.REMUX.x265-CMCT", "", nil), 1).Score)
	assert.Equal(t, base+bonusHDR, p.Score(cand("A.2024.2160p.BluRay.HDR.x265-CMCT", "", nil), 1).Score)
	assert.Equal(t, base+bonusChineseSubs, p.Score(cand("A.2024.2160p.BluRay.x265-CMCT", "沙丘 中字", nil), 1).Score)
	assert.Equal(t, base+bonusGroup, p.Score(cand("A.2024.2160p.BluRay.x265-FRDS", "", nil), 1).Score, "制作组不分大小写")
}

func TestTargetReachedAndBetter(t *testing.T) {
	p := Profile{Resolutions: []string{"2160p", "1080p"}, Sources: []string{"UHD BluRay", "WEB-DL"}}
	assert.True(t, p.TargetReached(meta.Parse("A.2024.2160p.UHD.BluRay.x265-X", "")))
	assert.False(t, p.TargetReached(meta.Parse("A.2024.2160p.WEB-DL.x265-X", "")))
	assert.False(t, p.TargetReached(meta.Parse("A.2024.1080p.UHD.BluRay.x265-X", "")))
	assert.True(t, Profile{}.TargetReached(meta.Parse("A.2024.720p.HDTV.x264-X", "")), "没排的不限")
	assert.False(t, Profile{Remux: models.MediaPrefPrefer}.TargetReached(meta.Parse("A.2024.2160p.BluRay.x265-X", "")))

	pf := Profile{Free: models.MediaPrefPrefer}
	a := cand("A.2024.1080p.WEB-DL.H264-X", "", func(it *v2.TorrentItem) { it.DiscountLevel = v2.DiscountFree })
	b := cand("A.2024.1080p.WEB-DL.H264-X", "", func(it *v2.TorrentItem) { it.Seeders = 99 })
	va, vb := pf.Score(a, 1), pf.Score(b, 1)
	assert.True(t, pf.better(a, b, va, vb), "同分时要免费的免费先")
	assert.False(t, Profile{}.better(a, b, va, vb), "不要求免费时做种多的先")
}

func TestProfileJSONRoundTrip(t *testing.T) {
	r := models.MediaQualityProfile{ID: 3, Name: "4K", Resolutions: encodeList([]string{"2160p"}), Groups: encodeList(nil)}
	p := profileOf(r)
	assert.Equal(t, []string{"2160p"}, p.Resolutions)
	assert.Empty(t, p.Groups)
	assert.Empty(t, r.Groups, "空列表存成空串")
}

// 分辨率 > 来源 > 编码 > 加分：高一档胜过下面各项加起来的最大值
func TestScoreLevelsDominate(t *testing.T) {
	p := Profile{
		Codecs: []string{"H.265", "H.264"}, Remux: models.MediaPrefPrefer, HDR: models.MediaPrefPrefer,
		ChineseSubs: models.MediaPrefPrefer, Groups: []string{"FRDS"},
	}
	lowSrcAll := cand("A.2024.1080p.WEB-DL.REMUX.HDR.H265-FRDS", "A 中字", nil)
	highSrcNone := cand("A.2024.1080p.BluRay.H264-X", "", nil)
	require.True(t, lowSrcAll.Meta.Remux && len(lowSrcAll.Meta.HDR) > 0 && lowSrcAll.Meta.ChineseSubs, lowSrcAll.Meta)
	a, b := p.Score(lowSrcAll, 1), p.Score(highSrcNone, 1)
	require.Empty(t, a.Reject)
	require.Empty(t, b.Reject)
	assert.Greater(t, b.Score, a.Score, "来源高一档胜过编码与所有加分")

	lowCodecAll := p.Score(cand("A.2024.1080p.BluRay.REMUX.HDR.H264-FRDS", "A 中字", nil), 1)
	highCodecNone := p.Score(cand("A.2024.1080p.BluRay.H265-X", "", nil), 1)
	assert.Greater(t, highCodecNone.Score, lowCodecAll.Score, "编码高一档胜过所有加分")

	lowResAll := p.Score(cand("A.2024.720p.BluRay.REMUX.HDR.H265-FRDS", "A 中字", nil), 1)
	highResNone := p.Score(cand("A.2024.1080p.DVD.H264-X", "", nil), 1)
	assert.Greater(t, highResNone.Score, lowResAll.Score, "分辨率高一档胜过下面所有项")
}
