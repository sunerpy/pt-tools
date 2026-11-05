package subscribe

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func TestCreateSubscriptionPaused(t *testing.T) {
	e := newEnv(t)
	sub, err := e.svc.CreateSubscription(e.ctx, SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, Paused: true}, models.MediaSubFromServarr)
	require.NoError(t, err)
	row := e.subRow(sub.ID)
	assert.Equal(t, models.MediaSubPaused, row.Status)
	assert.Equal(t, models.MediaSubFromServarr, row.Source)

	// 待确认优先于暂停
	tv, err := e.svc.CreateSubscription(e.ctx, SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Paused: true, Pending: true}, models.MediaSubFromServarr)
	require.NoError(t, err)
	assert.Equal(t, models.MediaSubPending, e.subRow(tv.ID).Status)
}

func TestMovieLibrary(t *testing.T) {
	e := newEnv(t)
	got, err := e.svc.MovieLibrary(e.ctx, 693134)
	require.NoError(t, err)
	assert.Equal(t, LibraryMovie{}, got)

	h := e.organized("h1", models.MediaKindMovie, 693134, 0, 0, "/lib/m/a.mkv")
	require.NoError(t, e.db.Model(&h).Update("size", 3000).Error)
	h2 := e.organized("h1sub", models.MediaKindMovie, 693134, 0, 0, "/lib/m/a.zh.srt")
	require.NoError(t, e.db.Model(&h2).Update("size", 20).Error)
	// 没整理成的、别的条目的不算
	bad := e.organized("h2", models.MediaKindMovie, 693134, 0, 0, "/lib/m/b.mkv")
	require.NoError(t, e.db.Model(&bad).Updates(map[string]any{"status": models.MediaTransferFailed, "size": 99}).Error)
	e.organized("h3", models.MediaKindMovie, 1, 0, 0, "/lib/m/c.mkv")
	e.organized("h4", models.MediaKindTV, 693134, 1, 1, "/lib/t/d.mkv")

	got, err = e.svc.MovieLibrary(e.ctx, 693134)
	require.NoError(t, err)
	assert.Equal(t, LibraryMovie{Have: true, Size: 3020, Dir: filepath.FromSlash("/lib/m")}, got)

	// 目录是最近整理的那个文件所在的
	newer := e.organized("h5", models.MediaKindMovie, 693134, 0, 0, "/lib/m2/a.2160p.mkv")
	require.NoError(t, e.db.Model(&newer).Update("size", 4000).Error)
	got, err = e.svc.MovieLibrary(e.ctx, 693134)
	require.NoError(t, err)
	assert.Equal(t, LibraryMovie{Have: true, Size: 7020, Dir: filepath.FromSlash("/lib/m2")}, got)
}

func TestLibrarySeasons(t *testing.T) {
	e := newEnv(t)
	got, err := e.svc.LibrarySeasons(e.ctx, 100088)
	require.NoError(t, err)
	assert.Empty(t, got)

	sized := func(h models.MediaTransferHistory, size int64, end int) {
		require.NoError(t, e.db.Model(&h).Updates(map[string]any{"size": size, "episode_end": end}).Error)
	}
	sized(e.organized("a", models.MediaKindTV, 100088, 1, 1, "/lib/s1e1.mkv"), 100, 0)
	sized(e.organized("a", models.MediaKindTV, 100088, 1, 2, "/lib/s1e2.mkv"), 200, 3)   // 一个文件是第 2–3 集
	sized(e.organized("b", models.MediaKindTV, 100088, 1, 2, "/lib/s1e2.v2.mkv"), 50, 0) // 同一集的另一个文件：集数不重复算，大小算上
	sized(e.organized("c", models.MediaKindTV, 100088, 2, 1, "/lib/s2e1.mkv"), 400, 0)
	sized(e.organized("d", models.MediaKindTV, 100088, 0, 1, "/lib/s0e1.mkv"), 7, 0)
	// 没有集号的（例如原盘的附件）不算集，也不算大小
	sized(e.organized("e", models.MediaKindTV, 100088, 1, 0, "/lib/extra.mkv"), 900, 0)
	// 别的剧、电影、没整理成的不算
	e.organized("f", models.MediaKindTV, 1, 1, 1, "/lib/x.mkv")
	e.organized("g", models.MediaKindMovie, 100088, 0, 0, "/lib/y.mkv")
	fail := e.organized("h", models.MediaKindTV, 100088, 2, 2, "/lib/s2e2.mkv")
	require.NoError(t, e.db.Model(&fail).Update("status", models.MediaTransferFailed).Error)

	got, err = e.svc.LibrarySeasons(e.ctx, 100088)
	require.NoError(t, err)
	assert.Equal(t, map[int]LibrarySeason{
		0: {Episodes: 1, Size: 7},
		1: {Episodes: 3, Size: 350},
		2: {Episodes: 1, Size: 400},
	}, got)
}

func TestSearchSoon(t *testing.T) {
	e := newEnv(t)
	sub := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	later := e.now.Add(6 * time.Hour)
	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).Update("next_search_at", later).Error)
	e.advance(time.Minute)

	require.NoError(t, e.svc.SearchSoon(e.ctx, sub.ID))
	row := e.subRow(sub.ID)
	require.NotNil(t, row.NextSearchAt)
	assert.True(t, row.NextSearchAt.Equal(e.Now()), "下次搜索改成现在：%v", row.NextSearchAt)

	_, err := e.svc.SetStatus(e.ctx, sub.ID, models.MediaSubPaused)
	require.NoError(t, err)
	assert.ErrorIs(t, e.svc.SearchSoon(e.ctx, sub.ID), ErrInvalid)
	assert.ErrorIs(t, e.svc.SearchSoon(e.ctx, sub.ID+100), ErrNotFound)
}

func TestCreateSubscriptionDeferred(t *testing.T) {
	e := newEnv(t)
	movie, err := e.svc.CreateSubscription(e.ctx, SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134, Deferred: true}, models.MediaSubFromServarr)
	require.NoError(t, err)
	row := e.subRow(movie.ID)
	require.NotNil(t, row.NextSearchAt)
	// 电影没有定时：按订阅设置的间隔（默认 12 小时）
	assert.True(t, row.NextSearchAt.Equal(e.now.Add(DefaultSearchIntervalHours*time.Hour)), "%v", row.NextSearchAt)

	in := SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Deferred: true}
	FollowDefaults(&in)
	tv, err := e.svc.CreateSubscription(e.ctx, in, models.MediaSubFromServarr)
	require.NoError(t, err)
	row = e.subRow(tv.ID)
	require.NotNil(t, row.NextSearchAt)
	// 按播出时间的追剧还没有台账：一天以后
	assert.True(t, row.NextSearchAt.Equal(e.now.Add(followCheckInterval)), "%v", row.NextSearchAt)

	// 不推迟的马上搜
	now, err := e.svc.CreateSubscription(e.ctx, SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 231620, Season: 4}, models.MediaSubFromServarr)
	require.NoError(t, err)
	assert.True(t, e.subRow(now.ID).NextSearchAt.Equal(e.now))
}

func TestSubscriptionsOf(t *testing.T) {
	e := newEnv(t)
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	s2 := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	v := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 231620, Season: 4})

	rows, err := e.svc.SubscriptionsOf(e.ctx, models.MediaKindTV, 0)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, []uint{s2.ID, v.ID}, []uint{rows[0].ID, rows[1].ID})

	rows, err = e.svc.SubscriptionsOf(e.ctx, models.MediaKindTV, 100088)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, s2.ID, rows[0].ID)

	rows, err = e.svc.SubscriptionsOf(e.ctx, models.MediaKindMovie, 0)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, m.ID, rows[0].ID)

	got, err := e.svc.SubscriptionRow(e.ctx, m.ID)
	require.NoError(t, err)
	assert.Equal(t, 693134, got.TMDBID)
	_, err = e.svc.SubscriptionRow(e.ctx, 999)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestAiredEpisodes(t *testing.T) {
	e := newEnv(t)
	tv := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	row := e.subRow(tv.ID)
	// 第 3 集 2026-12-01 才播
	assert.Equal(t, 2, e.svc.AiredEpisodes(e.ctx, &row))
	m := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	mr := e.subRow(m.ID)
	assert.Equal(t, 0, e.svc.AiredEpisodes(e.ctx, &mr))
}

func TestSetProfile(t *testing.T) {
	e := newEnv(t)
	p, err := e.svc.SaveProfile(e.ctx, 0, ProfileInput{Name: "1080p"})
	require.NoError(t, err)
	sub := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	later := e.now.Add(6 * time.Hour)
	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).Update("next_search_at", later).Error)

	// 没变时不动下次搜索
	got, err := e.svc.SetProfile(e.ctx, sub.ID, 0)
	require.NoError(t, err)
	assert.True(t, got.NextSearchAt.Equal(later))

	e.advance(time.Minute)
	got, err = e.svc.SetProfile(e.ctx, sub.ID, p.ID)
	require.NoError(t, err)
	assert.Equal(t, p.ID, got.ProfileID)
	assert.True(t, got.NextSearchAt.Equal(e.Now()), "改了档案尽快搜一次")

	_, err = e.svc.SetProfile(e.ctx, sub.ID, p.ID+100)
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.SetProfile(e.ctx, sub.ID+100, 0)
	require.ErrorIs(t, err, ErrNotFound)

	// 暂停的不提前搜
	_, err = e.svc.SetStatus(e.ctx, sub.ID, models.MediaSubPaused)
	require.NoError(t, err)
	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).Update("next_search_at", later).Error)
	got, err = e.svc.SetProfile(e.ctx, sub.ID, 0)
	require.NoError(t, err)
	assert.Equal(t, uint(0), got.ProfileID)
	assert.True(t, got.NextSearchAt.Equal(later))
}

func TestSetMonitored(t *testing.T) {
	e := newEnv(t)
	sub := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	later := e.now.Add(6 * time.Hour)
	setNext := func(at time.Time) {
		require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).Update("next_search_at", at).Error)
	}
	setNext(later)

	// 在找的：关上是暂停
	got, err := e.svc.SetMonitored(e.ctx, sub.ID, false, true)
	require.NoError(t, err)
	assert.Equal(t, models.MediaSubPaused, got.Status)
	assert.True(t, got.NextSearchAt.Equal(later))
	// 已经暂停的再关上不变
	got, err = e.svc.SetMonitored(e.ctx, sub.ID, false, false)
	require.NoError(t, err)
	assert.Equal(t, models.MediaSubPaused, got.Status)

	// 打开、不马上搜：按定时排（电影是订阅设置的间隔）
	e.advance(time.Minute)
	got, err = e.svc.SetMonitored(e.ctx, sub.ID, true, false)
	require.NoError(t, err)
	assert.Equal(t, models.MediaSubActive, got.Status)
	assert.True(t, got.NextSearchAt.Equal(e.Now().Add(DefaultSearchIntervalHours*time.Hour)), "%v", got.NextSearchAt)
	// 已经在找、不马上搜：不动
	setNext(later)
	got, err = e.svc.SetMonitored(e.ctx, sub.ID, true, false)
	require.NoError(t, err)
	assert.True(t, got.NextSearchAt.Equal(later))
	// 已经在找、马上搜：提前到现在
	got, err = e.svc.SetMonitored(e.ctx, sub.ID, true, true)
	require.NoError(t, err)
	assert.True(t, got.NextSearchAt.Equal(e.Now()))

	// 已完成与待确认的：关上不动，打开改成在找
	for _, st := range []string{models.MediaSubDone, models.MediaSubPending} {
		require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).Update("status", st).Error)
		got, err = e.svc.SetMonitored(e.ctx, sub.ID, false, true)
		require.NoError(t, err)
		assert.Equal(t, st, got.Status)
		got, err = e.svc.SetMonitored(e.ctx, sub.ID, true, true)
		require.NoError(t, err)
		assert.Equal(t, models.MediaSubActive, got.Status, st)
		assert.True(t, got.NextSearchAt.Equal(e.Now()))
	}

	// 剧集按台账排：第 3 集播出前几小时打开，排到它播出加延迟的时刻（没有台账时是一天以后）
	tv := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Schedule: &Schedule{Kind: models.MediaScheduleAirtime}})
	_, err = e.svc.SetStatus(e.ctx, tv.ID, models.MediaSubPaused)
	require.NoError(t, err)
	at, ok := airTime(tv, "2026-12-01")
	require.True(t, ok)
	e.advance(at.Add(defaultAirDelay - 5*time.Hour).Sub(e.Now()))
	got, err = e.svc.SetMonitored(e.ctx, tv.ID, true, false)
	require.NoError(t, err)
	assert.Equal(t, models.MediaSubActive, got.Status)
	require.NotNil(t, got.NextSearchAt)
	assert.True(t, got.NextSearchAt.Equal(at.Add(defaultAirDelay)), "%v", got.NextSearchAt)

	_, err = e.svc.SetMonitored(e.ctx, 999, true, true)
	assert.ErrorIs(t, err, ErrNotFound)
}
