package subscribe

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// 设置「等满几天改用别的版本」：默认 0（一直等），0 到 90 天，越界的不收。
func TestUnlockAfterDaysSettings(t *testing.T) {
	e := newEnv(t)
	set, err := e.svc.Settings(e.ctx)
	require.NoError(t, err)
	assert.Zero(t, set.UnlockAfterDays, "默认一直等")

	for _, bad := range []int{-1, 91} {
		_, err = e.svc.SaveSettings(e.ctx, Settings{UnlockAfterDays: bad})
		require.ErrorIs(t, err, ErrInvalid, bad)
	}
	set = e.enable(func(s *Settings) { s.UnlockAfterDays = 7 })
	assert.Equal(t, 7, set.UnlockAfterDays)
	set, err = e.svc.Settings(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, 7, set.UnlockAfterDays, "存进去以后读得出来")
	set = e.enable(func(s *Settings) { s.UnlockAfterDays = 90 })
	assert.Equal(t, 90, set.UnlockAfterDays)
}

// unlockEnv 建一个锁住 HHWEB 2160p 的追剧：第 1、2 集播出满 7 天还没有这个版本，进入等版本（开始等的时间是现在）。
func unlockEnv(t *testing.T, days int) (*env, *models.MediaSubscription) {
	t.Helper()
	e := newEnv(t)
	group, res := "HHWEB", "2160p"
	sub := e.follow(func(in *SubscriptionInput) { in.LockGroup, in.LockResolution = &group, &res })
	e.enable(func(s *Settings) { s.UnlockAfterDays = days })
	e1, ok := airTime(sub, "2026-10-01")
	require.True(t, ok)
	e.setNow(e1.Add(8 * 24 * time.Hour))
	e.svc.refresh(e.ctx)
	require.Equal(t, models.MediaEpisodeWaitingVersion, e.state(sub, 1))
	require.Equal(t, models.MediaEpisodeWaitingVersion, e.state(sub, 2))
	return e, sub
}

func (e *env) unlocked(sub *models.MediaSubscription, n int) bool {
	e.t.Helper()
	var row models.MediaSubscriptionEpisode
	require.NoError(e.t, e.db.Where("subscription_id = ? AND episode = ?", sub.ID, n).Limit(1).Find(&row).Error)
	return row.ManualUnlock
}

// 等满设置的天数、开始等以后也搜过一次：这几集像点了「这一集换别的版本」一样放开版本锁，马上搜一次，记一条运行记录；
// 台账里回到「缺」，不再提醒。之后的搜索下别的版本，锁住的版本不变。
func TestAutoUnlockAfterDays(t *testing.T) {
	e, sub := unlockEnv(t, 3)
	e.advance(time.Minute)
	_, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	runs := len(e.runs(sub.ID))

	e.advance(3 * 24 * time.Hour)
	e.svc.refresh(e.ctx)
	assert.True(t, e.unlocked(sub, 1))
	assert.True(t, e.unlocked(sub, 2))
	assert.Equal(t, models.MediaEpisodeMissing, e.state(sub, 1), "放开以后这一集回到缺的")
	assert.Equal(t, models.MediaEpisodeMissing, e.state(sub, 2))
	got := e.subRow(sub.ID)
	require.NotNil(t, got.NextSearchAt)
	assert.False(t, got.NextSearchAt.After(e.svc.cfg.Now()), "马上搜一次")
	all := e.runs(sub.ID)
	require.Len(t, all, runs+1)
	assert.Equal(t, models.MediaRunSchedule, all[0].Trigger, "不是用户点的")
	assert.Equal(t, models.MediaRunChanged, all[0].Status)
	assert.Equal(t, "第 1–2 集等 HHWEB 2160p 的版本满 3 天，按设置改用别的版本", all[0].Summary)
	assert.Contains(t, stepsOf(all[0]), "按设置改用别的版本")

	// 再过一拍：已经放开的不重复记
	e.advance(time.Minute)
	e.svc.refresh(e.ctx)
	assert.Len(t, e.runs(sub.ID), runs+1)

	e.search.set(
		e.item("mteam", "701", "Natural.High.S04E01.2023.1080p.WEB-DL.H.264-PTerWEB", "", 1, 10),
		e.item("mteam", "702", "Natural.High.S04E03.2023.1080p.WEB-DL.H.264-PTerWEB", "", 1, 10),
	)
	_, err = e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"701"}, pushedIDs(e), "第 1 集换了版本；第 3 集还在锁里")
	got = e.subRow(sub.ID)
	assert.Equal(t, "HHWEB", got.LockGroup, "锁住的版本不变")
	assert.Equal(t, "2160p", got.LockResolution)
}

// 不放开的情况：设置是 0（一直等）、天数没满、开始等以后还没搜过。
func TestAutoUnlockWaits(t *testing.T) {
	t.Run("设置是 0", func(t *testing.T) {
		e, sub := unlockEnv(t, 0)
		e.advance(time.Minute)
		_, err := e.svc.SearchNow(e.ctx, sub.ID)
		require.NoError(t, err)
		e.advance(60 * 24 * time.Hour)
		e.svc.refresh(e.ctx)
		assert.False(t, e.unlocked(sub, 1))
		assert.Equal(t, models.MediaEpisodeWaitingVersion, e.state(sub, 1))
	})
	t.Run("天数没满", func(t *testing.T) {
		e, sub := unlockEnv(t, 3)
		e.advance(time.Minute)
		_, err := e.svc.SearchNow(e.ctx, sub.ID)
		require.NoError(t, err)
		e.advance(3*24*time.Hour - 2*time.Minute)
		e.svc.refresh(e.ctx)
		assert.False(t, e.unlocked(sub, 1))
		assert.Equal(t, models.MediaEpisodeWaitingVersion, e.state(sub, 1))
	})
	t.Run("开始等以后没搜过", func(t *testing.T) {
		e, sub := unlockEnv(t, 3)
		e.advance(10 * 24 * time.Hour)
		e.svc.refresh(e.ctx)
		assert.False(t, e.unlocked(sub, 1))
		assert.Equal(t, models.MediaEpisodeWaitingVersion, e.state(sub, 1))
	})
}

// 用户已经手动放开的集不算进去：运行记录只写按设置放开的那几集。
func TestAutoUnlockSkipsManual(t *testing.T) {
	e, sub := unlockEnv(t, 3)
	e.advance(time.Minute)
	_, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	_, err = e.svc.UnlockEpisode(e.ctx, sub.ID, 1, true)
	require.NoError(t, err)
	runs := len(e.runs(sub.ID))

	e.advance(3 * 24 * time.Hour)
	e.svc.refresh(e.ctx)
	assert.True(t, e.unlocked(sub, 2))
	all := e.runs(sub.ID)
	require.Len(t, all, runs+1)
	assert.Equal(t, "第 2 集等 HHWEB 2160p 的版本满 3 天，按设置改用别的版本", all[0].Summary)
}

// 没开版本锁、或者锁还没定下来时不动（这时也不会有等版本的集，这里直接给一份台账）。
func TestAutoUnlockNeedsLock(t *testing.T) {
	e, sub := unlockEnv(t, 3)
	row := e.subRow(sub.ID)
	since := e.svc.cfg.Now().Add(-10 * 24 * time.Hour)
	searched := e.svc.cfg.Now().Add(-time.Hour)
	row.LastSearchAt = &searched
	lg := &ledger{rows: []models.MediaSubscriptionEpisode{{SubscriptionID: sub.ID, Episode: 1, State: models.MediaEpisodeWaitingVersion, WaitingSince: &since}}}
	set := Settings{Enabled: true, UnlockAfterDays: 3}

	noLock := row
	noLock.LockVersion = false
	assert.False(t, e.svc.autoUnlock(e.ctx, &noLock, lg, set))
	unset := row
	unset.LockGroup, unset.LockResolution = "", ""
	assert.False(t, e.svc.autoUnlock(e.ctx, &unset, lg, set))
	assert.False(t, e.unlocked(sub, 1))

	assert.True(t, e.svc.autoUnlock(e.ctx, &row, lg, set), "锁住了：放开")
	assert.True(t, e.unlocked(sub, 1))
}
