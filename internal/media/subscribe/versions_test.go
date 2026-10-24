package subscribe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// 锁住的版本等不到（2026-10-24 用户：「清理了如何重新设置条件」）：搜索记下找到了哪些版本、各能补哪几集，
// 「修改」里选一个改锁，下一次搜索就按新锁推。
func TestSeenVersionsRecordedAndUsedToRelock(t *testing.T) {
	e := newEnv(t)
	g, r := "MWeb", "2160p"
	sub := e.follow(func(in *SubscriptionInput) { in.LockGroup, in.LockResolution = &g, &r })
	e.enable(nil)
	e.search.set(lockItems(e)...)

	msg, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Empty(t, pushedIDs(e), msg)
	assert.Contains(t, msg, "找到的都不是锁住的版本（MWeb 2160p）")

	view, err := e.svc.Subscription(e.ctx, sub.ID)
	require.NoError(t, err)
	require.NotNil(t, view.SeenVersions)
	assert.NotEmpty(t, view.SeenVersions.At)
	assert.Equal(t, []SeenVersion{
		{Group: "HHWEB", Resolution: "2160p", Episodes: []int{1, 2}, Sites: []string{"mteam"}},
		{Group: "FRDS", Resolution: "2160p", Episodes: []int{3}, Sites: []string{"mteam"}},
		{Group: "PTerWEB", Resolution: "1080p", Episodes: []int{1}, Sites: []string{"mteam"}},
		{Group: "PTerWEB", Resolution: "2160p", Episodes: []int{2}, Sites: []string{"mteam"}},
	}, view.SeenVersions.Versions, "能补的集多的在前，同样多时按组名、再按分辨率")

	// 选 HHWEB 2160p 改锁：下一次搜索推它的第 1、2 集
	hg, hr := "HHWEB", "2160p"
	_, err = e.svc.UpdateSubscription(e.ctx, sub.ID, SubscriptionInput{LockGroup: &hg, LockResolution: &hr})
	require.NoError(t, err)
	got := e.subRow(sub.ID)
	assert.Equal(t, "HHWEB", got.LockGroup)
	assert.False(t, got.LockFromNext)
	msg, err = e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"201", "204"}, pushedIDs(e), msg)
}

// 只认制作组（分辨率留空）：同组的哪个分辨率都要。
func TestLockGroupOnly(t *testing.T) {
	e := newEnv(t)
	g, r := "PTerWEB", ""
	sub := e.follow(func(in *SubscriptionInput) { in.LockGroup, in.LockResolution = &g, &r })
	e.enable(nil)
	e.search.set(lockItems(e)...)
	msg, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"202", "203"}, pushedIDs(e), msg)
}

// RSS 推来的单条不覆盖搜索记下的版本；认不出组和分辨率的候选不记。
func TestSeenVersionsOnlyFromSearch(t *testing.T) {
	e := newEnv(t)
	sub := e.follow(nil)
	missing := map[int]bool{1: true}
	rss := scored{c: Candidate{Site: "mteam", From: fromRSS, Meta: e.svc.parse(e.ctx, "Natural.High.S04E01.2023.2160p.WEB-DL-HHWEB", "")}, sp: span{Start: 1, End: 1}}
	e.svc.recordSeenVersions(e.ctx, sub, []scored{rss}, missing)
	assert.Empty(t, e.subRow(sub.ID).SeenVersions, "只有 RSS 的候选：不记")

	noVersion := scored{c: Candidate{Site: "mteam", From: fromSearch, Meta: e.svc.parse(e.ctx, "Natural High 第一集", "")}, sp: span{Start: 1, End: 1}}
	assert.Empty(t, seenVersionsOf([]scored{noVersion}, missing))
	pack := scored{c: Candidate{Site: "hdsky", From: fromSearch, Meta: e.svc.parse(e.ctx, "Natural.High.S04.2023.1080p.WEB-DL-ADWeb", "")}, sp: span{Whole: true}}
	assert.Equal(t, []SeenVersion{{Group: "ADWeb", Resolution: "1080p", Episodes: []int{1}, Sites: []string{"hdsky"}}},
		seenVersionsOf([]scored{pack}, missing), "整季包能补上缺的每一集")

	e.svc.recordSeenVersions(e.ctx, sub, []scored{pack, rss}, missing)
	row := e.subRow(sub.ID)
	view := seenVersionsView(&row)
	require.NotNil(t, view)
	assert.Len(t, view.Versions, 2)
	assert.Nil(t, seenVersionsView(&models.MediaSubscription{SeenVersions: "{bad"}))
}
