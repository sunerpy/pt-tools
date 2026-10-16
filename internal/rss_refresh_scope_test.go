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

// 硬性语义 5 的「来源和过滤规则」：这条 RSS 推的种子每轮也按这一轮的判断刷新来源与命中的过滤规则
// （规则改了、不再命中就清掉）；别的入口推的（手动推送等）保持那个入口写的来源。
func TestRSSFreeSafety_PushedRecordRefreshesSourceAndRule(t *testing.T) {
	r := newRSSRig(t)
	feed := newFeedServer(t, "s9", "m9")
	rss := r.addRSS(feed, models.RSSConfig{Name: "rule", Tag: "tv", IntervalMinutes: 10})[0]
	rule := models.FilterRule{Name: "any", Pattern: ".*", PatternType: "regex", MatchField: "both", Enabled: true, Priority: 100, Purpose: "download"}
	require.NoError(t, r.db.DB.Create(&rule).Error)
	require.NoError(t, r.db.DB.Model(&models.FilterRule{}).Where("id = ?", rule.ID).Update("require_free", false).Error)
	require.NoError(t, models.NewRSSFilterAssociationDB(r.db.DB).SetFilterRulesForRSS(rss.ID, []uint{rule.ID}))
	pushed := true
	manual := models.TorrentInfo{SiteName: "springsunday", TorrentID: "m9", Title: "Tm9", IsPushed: &pushed, DownloadSource: "manual_push"}
	require.NoError(t, r.db.DB.Create(&manual).Error)

	r.site.set("s9", v2.TorrentItem{DiscountLevel: v2.DiscountNone})
	r.site.set("m9", v2.TorrentItem{DiscountLevel: v2.DiscountNone})
	r.round(rss)
	ti := r.record("s9")
	require.True(t, pushedOf(ti))
	require.Equal(t, filter.SourceFilterRule, ti.DownloadSource)
	require.NotNil(t, ti.FilterRuleID)
	require.NotNil(t, ti.PushedFilterRuleID, "推送时记下把它推进下载器的规则")
	assert.Equal(t, rule.ID, *ti.PushedFilterRuleID)

	require.NoError(t, r.db.DB.Model(&models.FilterRule{}).Where("id = ?", rule.ID).Update("pattern", "^nothing-matches$").Error)
	r.site.set("s9", freeUntil(48*time.Hour))
	r.round(rss)
	ti = r.record("s9")
	assert.Nil(t, ti.FilterRuleID, "规则不再命中：清掉")
	require.NotNil(t, ti.PushedFilterRuleID, "推送时的规则不改（规则命中数按它统计）")
	assert.Equal(t, rule.ID, *ti.PushedFilterRuleID)
	assert.Equal(t, filter.SourceFreeDownload, ti.DownloadSource, "按这一轮的判断写来源")
	assert.Equal(t, "manual_push", r.record("m9").DownloadSource, "别的入口推的保持原来的来源")
	assert.Len(t, r.dls["qb"].addCalls(), 1, "只是刷新，不会再推")
}

// 没有所有者的已推送记录（升级前推送的）或所有者已删除的：本站点只有这一条 RSS 时接管并刷新来源与规则；
// 有几条 RSS 时分不清该归谁，谁也不改（不然几条 RSS 轮流按各自的规则改写）。
func TestRSSFreeSafety_OwnerlessPushedRecordSourceRefresh(t *testing.T) {
	legacy := func(t *testing.T, r *rssRig, ruleID uint) {
		pushed := true
		ti := models.TorrentInfo{
			SiteName: "springsunday", TorrentID: "l1", Title: "Tl1", IsPushed: &pushed,
			DownloadSource: filter.SourceFilterRule, FilterRuleID: &ruleID,
		}
		require.NoError(t, r.db.DB.Create(&ti).Error)
	}
	newRule := func(t *testing.T, r *rssRig, pattern string) uint {
		rule := models.FilterRule{Name: pattern, Pattern: pattern, PatternType: "regex", MatchField: "both", Enabled: true, Priority: 100, Purpose: "download"}
		require.NoError(t, r.db.DB.Create(&rule).Error)
		require.NoError(t, r.db.DB.Model(&models.FilterRule{}).Where("id = ?", rule.ID).Update("require_free", false).Error)
		return rule.ID
	}

	t.Run("本站点只有这一条 RSS：接管并刷新", func(t *testing.T) {
		r := newRSSRig(t)
		feed := newFeedServer(t, "l1")
		rss := r.addRSS(feed, models.RSSConfig{Name: "only", Tag: "tv", IntervalMinutes: 10})[0]
		old := newRule(t, r, "^nothing-matches$")
		cur := newRule(t, r, ".*")
		require.NoError(t, models.NewRSSFilterAssociationDB(r.db.DB).SetFilterRulesForRSS(rss.ID, []uint{cur}))
		legacy(t, r, old)
		r.site.set("l1", v2.TorrentItem{DiscountLevel: v2.DiscountNone})
		require.NoError(t, os.MkdirAll(r.stagingDir(rss), 0o755)) // 什么都不暂存（生产里推送前会建好目录）
		r.round(rss)
		ti := r.record("l1")
		require.NotNil(t, ti.SourceRSSID)
		assert.Equal(t, rss.ID, *ti.SourceRSSID, "接管成所有者")
		require.NotNil(t, ti.FilterRuleID)
		assert.Equal(t, cur, *ti.FilterRuleID, "按这一轮命中的规则刷新")
	})

	t.Run("本站点有两条 RSS：谁也不改", func(t *testing.T) {
		r := newRSSRig(t)
		feed := newFeedServer(t, "l1")
		rs := r.addRSS(feed,
			models.RSSConfig{Name: "a", Tag: "tv", IntervalMinutes: 10},
			models.RSSConfig{Name: "b", Tag: "movie", IntervalMinutes: 10},
		)
		old := newRule(t, r, "^nothing-matches$")
		ruleA, ruleB := newRule(t, r, ".*"), newRule(t, r, "^T")
		require.NoError(t, models.NewRSSFilterAssociationDB(r.db.DB).SetFilterRulesForRSS(rs[0].ID, []uint{ruleA}))
		require.NoError(t, models.NewRSSFilterAssociationDB(r.db.DB).SetFilterRulesForRSS(rs[1].ID, []uint{ruleB}))
		legacy(t, r, old)
		r.site.set("l1", v2.TorrentItem{DiscountLevel: v2.DiscountNone})
		require.NoError(t, os.MkdirAll(r.stagingDir(rs[0]), 0o755))
		require.NoError(t, os.MkdirAll(r.stagingDir(rs[1]), 0o755))
		r.round(rs[0])
		r.round(rs[1])
		ti := r.record("l1")
		assert.Nil(t, ti.SourceRSSID)
		require.NotNil(t, ti.FilterRuleID)
		assert.Equal(t, old, *ti.FilterRuleID, "分不清归谁：保持原来的规则")
	})
}
