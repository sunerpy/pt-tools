package subscribe

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/bangumi"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

// 更多列表：电影的正在上映，剧集没有这个列表时写明能选哪些；标上已订阅。
func TestExploreMoreLists(t *testing.T) {
	e := newEnv(t)
	sub := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	e.tmdb.set("/3/movie/now_playing", `{"page":1,"total_pages":2,"results":[{"id":693134,"title":"沙丘2","release_date":"2024-02-27"}]}`)
	page, err := e.svc.Explore(e.ctx, models.MediaKindMovie, tmdb.ListNowPlaying, 1, "")
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, 2, page.TotalPages)
	assert.True(t, page.Items[0].Subscribed)
	assert.Equal(t, sub.ID, page.Items[0].SubscriptionID)

	_, err = e.svc.Explore(e.ctx, models.MediaKindTV, tmdb.ListNowPlaying, 1, "")
	require.ErrorIs(t, err, ErrInvalid)
	assert.ErrorContains(t, err, "on_the_air、airing_today")
	_, err = e.svc.Explore(e.ctx, models.MediaKindMovie, ExploreDiscover, 1, "")
	require.ErrorIs(t, err, ErrInvalid, "按条件发现走 Discover")
}

// 按条件发现：国家写小写也认；写错的国家与类型、别的类型报错；标上已入库。
func TestDiscoverExplore(t *testing.T) {
	e := newEnv(t)
	e.organized("h", models.MediaKindTV, 7, 1, 1, "/lib/x.mkv")
	e.tmdb.set("/3/discover/tv", `{"page":2,"total_pages":9,"results":[{"id":7,"name":"向往的生活","first_air_date":"2017-01-15","genre_ids":[10764]}]}`)
	page, err := e.svc.Discover(e.ctx, models.MediaKindTV, ExploreFilter{Country: " cn ", Genres: "10764|10767"}, 2)
	require.NoError(t, err)
	assert.Equal(t, 2, page.Page)
	assert.Equal(t, 9, page.TotalPages)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "向往的生活", page.Items[0].Title)
	assert.True(t, page.Items[0].InLibrary)

	for _, f := range []ExploreFilter{{Country: "CHN"}, {Country: "中国"}, {Genres: "16,99"}, {Genres: "综艺"}} {
		_, err = e.svc.Discover(e.ctx, models.MediaKindTV, f, 1)
		require.ErrorIs(t, err, ErrInvalid, f)
	}
	_, err = e.svc.Discover(e.ctx, "music", ExploreFilter{}, 1)
	require.ErrorIs(t, err, ErrInvalid)
	e.tmdb.set("/3/discover/movie", "404")
	_, err = e.svc.Discover(e.ctx, models.MediaKindMovie, ExploreFilter{}, 1)
	require.Error(t, err)
}

type fakeBangumi struct {
	days []bangumi.CalendarDay
	err  error
}

func (f fakeBangumi) Calendar(context.Context) ([]bangumi.CalendarDay, error) { return f.days, f.err }

var frieren = bangumi.CalendarItem{ID: 500001, Name: "葬送のフリーレン 第2期", NameCN: "葬送的芙莉莲 第二季", AirDate: "2026-01-16"}

func bangumiEnv(t *testing.T, items ...bangumi.CalendarItem) *env {
	t.Helper()
	e := newEnv(t)
	e.svc.cfg.Bangumi = fakeBangumi{days: []bangumi.CalendarDay{
		{Weekday: 1, Name: "星期一", Items: []bangumi.CalendarItem{}},
		{Weekday: 5, Name: "星期五", Items: items},
	}}
	return e
}

// 番剧日历里的一部按去掉第几季的名字在 TMDB 上找：名字完全相同的动画只有一个时用它。
func TestBangumiToTMDB(t *testing.T) {
	e := bangumiEnv(t, frieren, bangumi.CalendarItem{ID: 9, Name: "Unknown Show", AirDate: "2026-01-01"})
	e.tmdb.set("/3/search/tv", `{"results":[
		{"id":209867,"name":"葬送的芙莉莲","original_name":"葬送のフリーレン","first_air_date":"2023-09-29","genre_ids":[16,10759]},
		{"id":2,"name":"葬送的芙莉莲","original_name":"Frieren","first_air_date":"2023-09-29","genre_ids":[10759]},
		{"id":3,"name":"葬送的芙莉莲","original_name":"葬送のフリーレン","first_air_date":"2028-01-01","genre_ids":[16]}]}`)
	days, err := e.svc.BangumiCalendar(e.ctx)
	require.NoError(t, err)
	require.Len(t, days, 2)

	got, err := e.svc.BangumiToTMDB(e.ctx, frieren.ID)
	require.NoError(t, err)
	assert.Equal(t, &BangumiMatch{MediaType: models.MediaKindTV, TMDBID: 209867, Title: "葬送的芙莉莲"}, got,
		"不是动画的、首播比这一季晚两年的不算")

	_, err = e.svc.BangumiToTMDB(e.ctx, 9)
	require.ErrorIs(t, err, ErrNotFound, "名字对不上的不用")
	assert.ErrorContains(t, err, "搜过「Unknown Show」")

	_, err = e.svc.BangumiToTMDB(e.ctx, 404)
	require.ErrorIs(t, err, ErrNotFound)
	assert.ErrorContains(t, err, "番剧日历里没有这一部")
}

// 名字没有完全相同的、以这个名字开头的动画只有一个时用它；以它开头的不止一个时拿不准；搜不到时写明搜过的名字（中文名、
// 原名去重）；TMDB 出错时照实报错。
func TestBangumiToTMDBFallback(t *testing.T) {
	e := bangumiEnv(t, bangumi.CalendarItem{ID: 1, Name: "Kusuriya no Hitorigoto 2nd Season", NameCN: "药屋少女的呢喃 第二季", AirDate: "2025-01-10"})
	e.tmdb.set("/3/search/tv", `{"results":[
		{"id":220542,"name":"药屋少女的呢喃：猫猫的后宫谜案","original_name":"薬屋のひとりごと","first_air_date":"2023-10-22","genre_ids":[16]},
		{"id":5,"name":"别的动画","original_name":"Other","first_air_date":"2023-10-22","genre_ids":[16]}]}`)
	got, err := e.svc.BangumiToTMDB(e.ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, 220542, got.TMDBID)

	e = bangumiEnv(t, bangumi.CalendarItem{ID: 3, Name: "Show", NameCN: "某某 第二季"})
	e.tmdb.set("/3/search/tv", `{"results":[
		{"id":31,"name":"某某甲","first_air_date":"2020-01-01","genre_ids":[16]},{"id":32,"name":"某某乙","first_air_date":"2020-01-01","genre_ids":[16]}]}`)
	_, err = e.svc.BangumiToTMDB(e.ctx, 3)
	require.ErrorIs(t, err, ErrNotFound, "以这个名字开头的不止一个：拿不准")

	e = bangumiEnv(t, bangumi.CalendarItem{ID: 2, Name: "Same", NameCN: "Same 第2期"})
	e.tmdb.set("/3/search/tv", `{"results":[]}`)
	_, err = e.svc.BangumiToTMDB(e.ctx, 2)
	require.ErrorIs(t, err, ErrNotFound)
	assert.ErrorContains(t, err, "（搜过「Same」）")

	e = bangumiEnv(t, bangumi.CalendarItem{ID: 4, Name: "Broken"})
	e.tmdb.set("/3/search/tv", "not json")
	_, err = e.svc.BangumiToTMDB(e.ctx, 4)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotFound, "TMDB 出错不是没找到")
}

func TestBangumiCalendarErrors(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.BangumiCalendar(e.ctx)
	require.ErrorContains(t, err, "没有接上 Bangumi")
	e.svc.cfg.Bangumi = fakeBangumi{err: errors.New("连不上 Bangumi")}
	_, err = e.svc.BangumiToTMDB(e.ctx, 1)
	require.ErrorContains(t, err, "连不上 Bangumi")
}

func TestBaseName(t *testing.T) {
	for in, want := range map[string]string{
		"葬送的芙莉莲 第二季":                        "葬送的芙莉莲",
		"葬送のフリーレン 第2期":                      "葬送のフリーレン",
		"Kusuriya no Hitorigoto 2nd Season": "Kusuriya no Hitorigoto",
		"Re:Zero Season 3":                  "Re:Zero",
		"呪術廻戦 第2クール":                        "呪術廻戦",
		"进击的巨人 最终季":                         "进击的巨人 最终季",
		"Attack on Titan: Final Season":     "Attack on Titan",
		"Mob Psycho 100":                    "Mob Psycho 100",
		"第2期":                               "第2期",
		" 某某 第二季 Part 2 ":                   "某某",
	} {
		assert.Equal(t, want, baseName(in), in)
	}
}
