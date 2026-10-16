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

// 已推送、不受到期监控的记录不再读详情：免费到期后它要保持「免费、结束时间已过」，自动删种的「免费到期未完成删除」
// 按这个认出免费已过、没下完的任务（刷成「不免费」就认不出，任务会继续计费下载）。
func TestRSSFreeSafety_PushedUnmonitoredKeepsFreeEnd(t *testing.T) {
	r := newRSSRig(t)
	feed := newFeedServer(t, "q1")
	rss := r.addRSS(feed, models.RSSConfig{Name: "free", Tag: "movie", IntervalMinutes: 10})[0]
	end := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	r.site.set("q1", v2.TorrentItem{DiscountLevel: v2.DiscountFree, DiscountEndTime: end})
	r.round(rss)
	require.True(t, pushedOf(r.record("q1")))
	calls := r.site.calls("q1")

	r.site.set("q1", v2.TorrentItem{DiscountLevel: v2.DiscountNone})
	r.round(rss)
	assert.Equal(t, calls, r.site.calls("q1"), "已推送、不受到期监控的记录不再读详情")
	ti := r.record("q1")
	assert.True(t, ti.IsFree)
	require.NotNil(t, ti.FreeEndTime)
	assert.True(t, ti.FreeEndTime.Equal(end), "保留推送时的免费结束时间")
}
