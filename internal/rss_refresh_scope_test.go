package internal

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/internal/filter"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// RSS 每轮刷新已有记录的范围（硬性语义 5，docs/design/free-expiry-safety.md B.3.3）。

// 硬性语义 5：已跳过的记录每轮也读详情、按这一轮整套刷新并重新判断（以前不免费的跳过后 6 小时内、
// 免费但因别的条件跳过的一直不再读）。
func TestRSSFreeSafety_SkippedRecordRefreshedEveryRound(t *testing.T) {
	t.Run("跳过时不免费，下一轮变成免费", func(t *testing.T) {
		r := newRSSRig(t)
		feed := newFeedServer(t, "k1")
		rss := r.addRSS(feed, models.RSSConfig{Name: "free", Tag: "movie", IntervalMinutes: 10})[0]
		require.NoError(t, os.MkdirAll(r.stagingDir(rss), 0o755)) // 第一轮什么都不暂存（生产里推送前会建好目录）
		r.site.set("k1", v2.TorrentItem{DiscountLevel: v2.DiscountNone})
		r.round(rss)
		ti := r.record("k1")
		require.True(t, ti.IsSkipped)
		require.Equal(t, string(v2.FreeNotFree), ti.FreeValidity)
		calls := r.site.calls("k1")

		end := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
		r.site.set("k1", v2.TorrentItem{DiscountLevel: v2.DiscountFree, DiscountEndTime: end})
		before := time.Now()
		r.round(rss)
		assert.Greater(t, r.site.calls("k1"), calls, "已跳过的记录这一轮也读了详情")
		ti = r.record("k1")
		assert.Equal(t, string(v2.FreeUntil), ti.FreeValidity)
		require.NotNil(t, ti.FreeEndTime)
		assert.True(t, ti.FreeEndTime.Equal(end))
		require.NotNil(t, ti.FreeVerifiedAt)
		assert.False(t, ti.FreeVerifiedAt.Before(before.Add(-time.Second)), "验证时间是这一轮的")
		assert.False(t, ti.IsSkipped, "按这一轮重新判断")
		assert.Equal(t, filter.SourceFreeDownload, ti.DownloadSource)
		assert.True(t, pushedOf(ti), "变成免费的这一轮就推送，不用等 6 小时")
		assert.Len(t, r.dls["qb"].addCalls(), 1)
	})

	t.Run("免费但超过大小上限被跳过，上限放宽后", func(t *testing.T) {
		r := newRSSRig(t)
		settings := models.SettingsGlobal{DownloadDir: r.base, DefaultIntervalMinutes: 10, DefaultEnabled: true, TorrentSizeGB: 1}
		require.NoError(t, core.NewConfigStore(r.db).SaveGlobalSettings(settings))
		feed := newFeedServer(t, "k2")
		rss := r.addRSS(feed, models.RSSConfig{Name: "free", Tag: "movie", IntervalMinutes: 10})[0]
		require.NoError(t, os.MkdirAll(r.stagingDir(rss), 0o755))
		big := freeUntil(48 * time.Hour)
		big.SizeBytes = 2 * gb
		r.site.set("k2", big)
		r.round(rss)
		ti := r.record("k2")
		require.True(t, ti.IsSkipped)
		require.True(t, ti.IsFree)

		settings.TorrentSizeGB = 10
		require.NoError(t, core.NewConfigStore(r.db).SaveGlobalSettings(settings))
		shorter := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
		big.DiscountEndTime = shorter
		r.site.set("k2", big)
		r.round(rss)
		ti = r.record("k2")
		require.NotNil(t, ti.FreeEndTime)
		assert.True(t, ti.FreeEndTime.Equal(shorter), "结束时间按这一轮的详情写入")
		assert.False(t, ti.IsSkipped)
		assert.True(t, pushedOf(ti))
	})
}

// 已推送、不受到期监控的记录每轮也读详情、整套刷新资格：免费缩短、延长、取消后，自动删种的「免费到期未完成删除」
// 按新的事实处理（以前留着推送时的结束时间：取消或缩短时该删不删、继续计费下载，延长时提前误删）。
func TestRSSFreeSafety_PushedRecordRefreshedEveryRound(t *testing.T) {
	r := newRSSRig(t)
	feed := newFeedServer(t, "q1")
	rss := r.addRSS(feed, models.RSSConfig{Name: "free", Tag: "movie", IntervalMinutes: 10})[0]
	at := func(d time.Duration) time.Time { return time.Now().Add(d).UTC().Truncate(time.Second) }
	setEnd := func(end time.Time) {
		r.site.set("q1", v2.TorrentItem{DiscountLevel: v2.DiscountFree, DiscountEndTime: end})
	}

	setEnd(at(2 * time.Hour))
	r.round(rss)
	require.True(t, pushedOf(r.record("q1")))
	calls := r.site.calls("q1")

	shorter := at(time.Hour)
	setEnd(shorter)
	r.round(rss)
	assert.Greater(t, r.site.calls("q1"), calls, "已推送的记录这一轮也读了详情")
	ti := r.record("q1")
	require.NotNil(t, ti.FreeEndTime)
	assert.True(t, ti.FreeEndTime.Equal(shorter), "免费缩短：按新的结束时间")

	longer := at(5 * time.Hour)
	setEnd(longer)
	r.round(rss)
	ti = r.record("q1")
	require.NotNil(t, ti.FreeEndTime)
	assert.True(t, ti.FreeEndTime.Equal(longer), "免费延长：按新的结束时间")

	assert.Nil(t, ti.FreeEndedAt)

	r.site.set("q1", v2.TorrentItem{DiscountLevel: v2.DiscountNone})
	before := time.Now()
	r.round(rss)
	ti = r.record("q1")
	assert.Equal(t, string(v2.FreeNotFree), ti.FreeValidity)
	assert.False(t, ti.IsFree)
	require.NotNil(t, ti.FreeEndedAt, "原来免费、现在不免费：记下免费结束时间，删种按它认")
	assert.False(t, ti.FreeEndedAt.Before(before.Add(-time.Second)))
	assert.False(t, ti.FreeEndedAt.After(time.Now()))
	ended := *ti.FreeEndedAt

	r.round(rss)
	ti = r.record("q1")
	require.NotNil(t, ti.FreeEndedAt)
	assert.True(t, ti.FreeEndedAt.Equal(ended), "之后还是不免费：结束时间不往后挪")

	setEnd(at(3 * time.Hour))
	r.round(rss)
	assert.Nil(t, r.record("q1").FreeEndedAt, "又变回免费：清掉")
	assert.Len(t, r.dls["qb"].addCalls(), 1, "只是刷新资格，不会再推")
}
