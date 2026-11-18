package subscribe

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func titled(title, subtitle string) v2.TorrentItem {
	return v2.TorrentItem{Title: title, Subtitle: subtitle}
}

func TestKeywordReject(t *testing.T) {
	sub := &models.MediaSubscription{IncludeKeywords: "CHDWEB\n3Audio Atmos", ExcludeKeywords: "dv，60fps"}
	cases := []struct {
		title, subtitle, want string
	}{
		{"Dream S01 2026 2160p WEB-DL H.265 HDR HFR DDP 5.1 3Audio Atmos-CHDWEB", "", ""},
		{"Dream.S01.2026.2160p.WEB-DL.H.265.HDR.HFR.DDP.5.1.3Audio.Atmos-HHWEB", "", ""},
		{"Dream S01 2026 2160p WEB-DL H265 HDR DDP5.1-HHWEB", "", "标题不含任何包含词"},
		{"Dream S01 2026 2160p WEB-DL H265 DV-HHWEB", "梦想 全集 CHDWEB 出品", "标题含排除词 dv"},
		{"Dream S01 2026 2160p WEB-DL H265 60FPS-CHDWEB", "", "标题含排除词 60fps"},
	}
	for _, c := range cases {
		it := titled(c.title, c.subtitle)
		assert.Equal(t, c.want, keywordReject(sub, it), c.title)
	}
	assert.Empty(t, keywordReject(&models.MediaSubscription{}, titled("anything", "")), "没填关键词时不限")
	assert.Empty(t, keywordReject(&models.MediaSubscription{IncludeKeywords: " ,\n "}, titled("anything", "")), "只有分隔符等于没填")
}

func TestSearchKeepsOnlyKeywordMatches(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	inc, exc := "CHDWEB", "HDR"
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, IncludeKeywords: &inc, ExcludeKeywords: &exc})
	assert.Equal(t, "CHDWEB", m.IncludeKeywords)
	assert.Equal(t, "HDR", m.ExcludeKeywords)
	e.search.set(
		e.item("hdsky", "11", "Dune.Part.Two.2024.2160p.UHD.BluRay.REMUX.HEVC-FRDS", "", 60, 80),
		e.item("hdsky", "12", "Dune.Part.Two.2024.2160p.WEB-DL.H265.HDR-CHDWEB", "", 20, 30),
		e.item("hdsky", "13", "Dune.Part.Two.2024.1080p.WEB-DL.H264-CHDWEB", "", 8, 10),
	)
	msg, err := e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	assert.Contains(t, msg, "下载了")
	pushes := e.gotPushes()
	require.Len(t, pushes, 1)
	assert.Equal(t, "13", pushes[0].TorrentID, "只要标题含 CHDWEB、不含 HDR 的")
}

func TestSearchKeywordRejectsExplained(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	inc := "CHDWEB"
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, IncludeKeywords: &inc})
	e.search.set(e.item("hdsky", "11", "Dune.Part.Two.2024.2160p.WEB-DL.H265-X", "", 20, 30))
	msg, err := e.svc.SearchNow(e.ctx, m.ID)
	require.NoError(t, err)
	assert.Contains(t, msg, "都不符合质量档案或标题关键词")
	assert.Contains(t, msg, "标题不含任何包含词")
	assert.Empty(t, e.gotPushes())
}

func TestUpdateSubscriptionKeywords(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	assert.Empty(t, m.IncludeKeywords)

	later := e.Now().Add(6 * time.Hour)
	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", m.ID).Update("next_search_at", later).Error)
	inc, exc := "  CHDWEB\nHHWEB  ", "DV"
	row, err := e.svc.UpdateSubscription(e.ctx, m.ID, SubscriptionInput{IncludeKeywords: &inc, ExcludeKeywords: &exc})
	require.NoError(t, err)
	assert.Equal(t, "CHDWEB\nHHWEB", row.IncludeKeywords)
	assert.Equal(t, "DV", row.ExcludeKeywords)
	require.NotNil(t, row.NextSearchAt)
	assert.True(t, row.NextSearchAt.Equal(e.Now()), "改了关键词尽快搜一次")

	// 不带这两项（App API、ChatOps 的修改）时不变
	row, err = e.svc.UpdateSubscription(e.ctx, m.ID, SubscriptionInput{})
	require.NoError(t, err)
	assert.Equal(t, "CHDWEB\nHHWEB", row.IncludeKeywords)
	assert.Equal(t, "DV", row.ExcludeKeywords)

	empty := ""
	row, err = e.svc.UpdateSubscription(e.ctx, m.ID, SubscriptionInput{IncludeKeywords: &empty, ExcludeKeywords: &empty})
	require.NoError(t, err)
	assert.Empty(t, row.IncludeKeywords)
	assert.Empty(t, row.ExcludeKeywords)

	long := strings.Repeat("x", 1025)
	_, err = e.svc.UpdateSubscription(e.ctx, m.ID, SubscriptionInput{IncludeKeywords: &long})
	assert.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.UpdateSubscription(e.ctx, m.ID, SubscriptionInput{ExcludeKeywords: &long})
	assert.ErrorIs(t, err, ErrInvalid)
}
