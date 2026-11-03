package subscribe

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

// airtimeAt 把订阅改成按播出时间搜索：每集 at 播出，播出后 delay 小时开始搜（0 是默认的 2 小时）。
func (e *env) airtimeAt(sub *models.MediaSubscription, at string, delay int) {
	e.t.Helper()
	require.NoError(e.t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).Updates(map[string]any{
		"schedule_kind": models.MediaScheduleAirtime, "schedule_at": at, "air_delay_hours": delay,
	}).Error)
}

func calendarLines(cal *Calendar) []string {
	out := []string{}
	for _, en := range cal.Entries {
		out = append(out, fmt.Sprintf("%s %s E%d %s", en.AirDate, en.Title, en.Episode, en.State))
	}
	return out
}

// 日历：暂停的、待确认的订阅不列；集的状态入库按整理记录、下载中与不追按台账、其余按播出日期，还没到播出时刻的写还没播；
// 电影按上映日期；TMDB 上读不到的订阅写进 errors，别的照样列。
func TestCalendar(t *testing.T) {
	e := newEnv(t)
	e.setNow(time.Date(2026, 10, 8, 20, 0, 0, 0, time.Local))
	e.tmdb.set("/3/movie/693134", `{"id":693134,"title":"沙丘2","original_title":"Dune: Part Two","release_date":"2026-10-10","imdb_id":"tt15239678","poster_path":"/dune2.jpg"}`)
	nh := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 231620, Season: 4})
	e.airtimeAt(nh, "21:30", 3)
	e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	e.tmdb.set("/3/tv/231620/season/3", `{"season_number":3,"episodes":[{"episode_number":1,"name":"旧的一季","air_date":"2026-10-09"}]}`)
	require.NoError(t, e.db.Create(&models.MediaSubscription{
		MediaType: models.MediaKindTV, TMDBID: 231620, Season: 3, Title: "现在就出发", Status: models.MediaSubPaused,
	}).Error)
	e.tmdb.set("/3/tv/231620/season/2", `{"season_number":2,"episodes":[{"episode_number":1,"name":"豆瓣想看的一季","air_date":"2026-10-09"}]}`)
	require.NoError(t, e.db.Create(&models.MediaSubscription{
		MediaType: models.MediaKindTV, TMDBID: 231620, Season: 2, Title: "现在就出发", Status: models.MediaSubPending,
	}).Error)
	require.NoError(t, e.db.Create(&models.MediaSubscription{
		MediaType: models.MediaKindTV, TMDBID: 424242, Season: 1, Title: "坏的剧", Year: 2020, Status: models.MediaSubActive,
	}).Error)

	// 台账：第 1 集还写着缺（整理记录里已入库），第 2 集在下载，第 4 集不要了，第 5 集写着入库（整理记录里没有）
	require.NoError(t, e.db.Where("subscription_id = ?", nh.ID).Delete(&models.MediaSubscriptionEpisode{}).Error)
	for _, r := range []models.MediaSubscriptionEpisode{
		{SubscriptionID: nh.ID, Episode: 1, State: models.MediaEpisodeMissing},
		{SubscriptionID: nh.ID, Episode: 2, State: models.MediaEpisodeDownloading},
		{SubscriptionID: nh.ID, Episode: 4, State: models.MediaEpisodeSkipped, ManualSkip: true},
		{SubscriptionID: nh.ID, Episode: 5, State: models.MediaEpisodeLibrary},
	} {
		require.NoError(t, e.db.Create(&r).Error)
	}
	e.organized("h1", models.MediaKindTV, 231620, 4, 1, "/lib/nh/e01.mkv")

	cal, err := e.svc.Calendar(e.ctx, "2026-10-01", "2026-10-31")
	require.NoError(t, err)
	assert.Equal(t, "2026-10-08", cal.Today)
	assert.Equal(t, "2026-10-01", cal.From)
	assert.Equal(t, "2026-10-31", cal.To)
	require.Len(t, cal.Errors, 1)
	assert.Contains(t, cal.Errors[0], "坏的剧 (2020) 第 1 季：")
	assert.Equal(t, []string{
		"2026-10-01 现在就出发 E1 library",
		"2026-10-02 现在就出发 E2 downloading",
		"2026-10-08 现在就出发 E3 upcoming",
		"2026-10-08 现在就出发 E4 skipped",
		"2026-10-10 沙丘2 E0 upcoming",
		"2026-10-15 现在就出发 E5 upcoming",
	}, calendarLines(cal))
	e3 := cal.Entries[2]
	assert.Equal(t, nh.ID, e3.SubscriptionID)
	assert.Equal(t, models.MediaKindTV, e3.MediaType)
	assert.Equal(t, 231620, e3.TMDBID)
	assert.Equal(t, 4, e3.Season)
	assert.Equal(t, "第2期上：哞哞来了！", e3.Name)
	assert.Equal(t, "/nh.jpg", e3.PosterPath)
	require.NotNil(t, e3.AirAt)
	assert.True(t, time.Date(2026, 10, 8, 21, 30, 0, 0, time.Local).Equal(*e3.AirAt))
	require.NotNil(t, e3.SearchAt)
	assert.True(t, time.Date(2026, 10, 9, 0, 30, 0, 0, time.Local).Equal(*e3.SearchAt), "播出后 3 小时")
	movie := cal.Entries[4]
	assert.Equal(t, models.MediaKindMovie, movie.MediaType)
	assert.Equal(t, 693134, movie.TMDBID)
	assert.Nil(t, movie.AirAt)
	assert.Zero(t, movie.Season)

	// 过了播出时刻是缺；电影入库了；只查几天
	e.setNow(time.Date(2026, 10, 8, 22, 0, 0, 0, time.Local))
	require.NoError(t, e.db.Create(&models.MediaTransferHistory{
		SourcePath: "/dl/dune", MediaType: models.MediaKindMovie, TMDBID: 693134, Status: models.MediaTransferDone,
	}).Error)
	cal, err = e.svc.Calendar(e.ctx, "2026-10-08", "2026-10-10")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-10-08 现在就出发 E3 missing",
		"2026-10-08 现在就出发 E4 skipped",
		"2026-10-10 沙丘2 E0 library",
	}, calendarLines(cal))

	// 不按播出时间的：没有播出时刻，今天播出的就算缺；起始集以前、台账里没有的是不追
	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", nh.ID).Updates(map[string]any{
		"schedule_kind": models.MediaScheduleDaily, "start_episode": 4,
	}).Error)
	require.NoError(t, e.db.Where("subscription_id = ? AND episode = ?", nh.ID, 4).Delete(&models.MediaSubscriptionEpisode{}).Error)
	e.setNow(time.Date(2026, 10, 8, 9, 0, 0, 0, time.Local))
	cal, err = e.svc.Calendar(e.ctx, "2026-10-08", "2026-10-08")
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-10-08 现在就出发 E3 skipped", "2026-10-08 现在就出发 E4 missing"}, calendarLines(cal))
	assert.Nil(t, cal.Entries[0].AirAt)
	assert.Nil(t, cal.Entries[0].SearchAt)
}

// 日期范围：默认从今天起 7 天；写法不对、倒过来、超过 62 天的都不行；TMDB 没配时报错。
func TestCalendarRange(t *testing.T) {
	e := newEnv(t)
	e.setNow(time.Date(2026, 10, 8, 20, 0, 0, 0, time.Local))
	cal, err := e.svc.Calendar(e.ctx, "", "")
	require.NoError(t, err)
	assert.Equal(t, "2026-10-08", cal.From)
	assert.Equal(t, "2026-10-14", cal.To)
	assert.NotNil(t, cal.Entries, "空的也是数组")
	assert.NotNil(t, cal.Errors)
	cal, err = e.svc.Calendar(e.ctx, "2026-10-01", "")
	require.NoError(t, err)
	assert.Equal(t, "2026-10-07", cal.To)

	for _, c := range [][2]string{
		{"2026-10-1", ""}, {"2026-10-01", "10-31"}, {"2026-10-31", "2026-10-01"}, {"2026-10-01", "2026-12-02"},
	} {
		_, err = e.svc.Calendar(e.ctx, c[0], c[1])
		require.ErrorIs(t, err, ErrInvalid, c)
	}
	_, err = e.svc.Calendar(e.ctx, "2026-10-01", "2026-12-01")
	require.NoError(t, err, "62 天可以")
	cal, err = e.svc.Today(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, "2026-10-08", cal.From)
	assert.Equal(t, "2026-10-08", cal.To)

	e.svc.cfg.Recognizer = noKeyRecognizer{e.svc.cfg.Recognizer}
	_, err = e.svc.Calendar(e.ctx, "", "")
	require.ErrorIs(t, err, tmdb.ErrNoKey)
}

// 今日更新：到了设置的时刻发一次（同一天不再发），没有播出的集不发，通道没设时用订阅通知的通道；取不到时过一会儿再试。
func TestTodayPush(t *testing.T) {
	e := newEnv(t)
	e.setNow(time.Date(2026, 10, 8, 20, 0, 0, 0, time.Local))
	nh := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 231620, Season: 4})
	e.airtimeAt(nh, "21:30", 0)
	set := e.enable(func(s *Settings) { s.NotifyChannels = []uint{7} })
	assert.False(t, set.TodayEnabled)
	assert.Equal(t, "09:00", set.TodayTime, "默认 09:00")
	assert.Empty(t, set.TodayChannels)
	e.svc.Tick(e.ctx)
	assert.Empty(t, e.noticesOf(NoticeCalendarToday), "没开")

	set = e.enable(func(s *Settings) { s.NotifyChannels, s.TodayEnabled, s.TodayTime = []uint{7}, true, "20:30" })
	assert.Equal(t, "20:30", set.TodayTime)
	e.svc.Tick(e.ctx)
	assert.Empty(t, e.noticesOf(NoticeCalendarToday), "还没到 20:30")

	e.setNow(time.Date(2026, 10, 8, 20, 31, 0, 0, time.Local))
	e.svc.Tick(e.ctx)
	got := e.noticesOf(NoticeCalendarToday)
	require.Len(t, got, 1)
	n := got[0]
	assert.Equal(t, []uint{7}, n.Channels)
	assert.Equal(t, "calendar", n.Subject)
	assert.Equal(t, "today-2026-10-08", n.EventKey)
	assert.Equal(t, "📺 今日更新 2026-10-08", n.Title)
	assert.Equal(t, "今天播出 2 集：\n· 现在就出发 (2026) 第 4 季 E03「第2期上：哞哞来了！」 21:30 · 还没播\n"+
		"· 现在就出发 (2026) 第 4 季 E04「第2期下：热舞的哞哞」 21:30 · 还没播", n.Body)
	require.NotNil(t, n.Event)
	assert.Equal(t, notify.EventCalendarToday, n.Event.Type)
	assert.Equal(t, "2026-10-08", n.Event.Fields["day"])
	assert.Equal(t, int64(2), n.Event.Fields["count"])
	assert.Equal(t, int64(0), n.Event.Fields["in_library"])
	items := n.Event.Fields["items"].([]any)
	require.Len(t, items, 2)
	first := items[0].(map[string]any)
	assert.Equal(t, "21:30", first["air_time"])
	assert.Equal(t, int64(3), first["episode"])
	assert.Equal(t, "upcoming", first["state"])
	assert.Equal(t, "还没播", first["state_label"])

	e.setNow(time.Date(2026, 10, 8, 23, 0, 0, 0, time.Local))
	e.svc.Tick(e.ctx)
	assert.Len(t, e.noticesOf(NoticeCalendarToday), 1, "同一天不再发")

	e.setNow(time.Date(2026, 10, 9, 21, 0, 0, 0, time.Local))
	e.svc.Tick(e.ctx)
	assert.Len(t, e.noticesOf(NoticeCalendarToday), 1, "这天没有播出的集，不发")

	// 改用今日更新自己的通道；TMDB 取不到时 10 分钟以后再试
	e.enable(func(s *Settings) { s.NotifyChannels, s.TodayEnabled, s.TodayChannels = []uint{7}, true, []uint{9} })
	rec := e.svc.cfg.Recognizer
	e.svc.cfg.Recognizer = noKeyRecognizer{rec}
	e.setNow(time.Date(2026, 10, 15, 9, 0, 0, 0, time.Local))
	e.svc.Tick(e.ctx)
	assert.Len(t, e.noticesOf(NoticeCalendarToday), 1)
	e.svc.cfg.Recognizer = rec
	e.setNow(time.Date(2026, 10, 15, 9, 5, 0, 0, time.Local))
	e.svc.Tick(e.ctx)
	assert.Len(t, e.noticesOf(NoticeCalendarToday), 1, "还没到再试的时候")
	e.setNow(time.Date(2026, 10, 15, 9, 11, 0, 0, time.Local))
	e.svc.Tick(e.ctx)
	got = e.noticesOf(NoticeCalendarToday)
	require.Len(t, got, 2)
	assert.Equal(t, []uint{9}, got[1].Channels)
	assert.Equal(t, "today-2026-10-15", got[1].EventKey)
	assert.Equal(t, "今天播出 1 集：\n· 现在就出发 (2026) 第 4 季 E05「第 5 集」 21:30 · 还没播", got[1].Body)

	// 两种通道都没设时不发
	e.enable(func(s *Settings) { s.TodayEnabled = true })
	e.svc.today = todayState{}
	e.svc.Tick(e.ctx)
	assert.Len(t, e.noticesOf(NoticeCalendarToday), 2)
}

func TestTodaySettings(t *testing.T) {
	e := newEnv(t)
	set := e.enable(func(s *Settings) { s.TodayEnabled, s.TodayTime, s.TodayChannels = true, "9:05", []uint{3, 4} })
	assert.True(t, set.TodayEnabled)
	assert.Equal(t, "09:05", set.TodayTime)
	assert.Equal(t, []uint{3, 4}, set.TodayChannels)
	set, err := e.svc.Settings(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, "09:05", set.TodayTime, "读回来一样")

	for _, bad := range []Settings{
		{Enabled: true, TodayTime: "25:00"},
		{Enabled: true, TodayTime: "早上"},
		{Enabled: true, TodayChannels: make([]uint, 21)},
	} {
		_, err = e.svc.SaveSettings(e.ctx, bad)
		require.ErrorIs(t, err, ErrInvalid)
	}
	set, err = e.svc.Settings(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, "09:05", set.TodayTime, "保存失败时不变")
}

// 每日战报的追剧一行：那天播出的集（还没到播出时刻的不算）与其中入库的；订阅总开关关着、没有播出的集、TMDB 读不到时不写。
func TestFollowLine(t *testing.T) {
	e := newEnv(t)
	e.setNow(time.Date(2026, 10, 8, 20, 0, 0, 0, time.Local))
	nh := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 231620, Season: 4})
	e.airtimeAt(nh, "21:30", 0)
	assert.Empty(t, e.svc.FollowLine(e.ctx, "2026-10-02", "昨日"), "订阅总开关关着")
	e.enable(nil)
	assert.Equal(t, "📺 昨日追剧：播出 1 集，入库 0 集", e.svc.FollowLine(e.ctx, "2026-10-02", "昨日"))
	e.organized("h2", models.MediaKindTV, 231620, 4, 2, "/lib/nh/e02.mkv")
	assert.Equal(t, "📺 昨日追剧：播出 1 集，入库 1 集", e.svc.FollowLine(e.ctx, "2026-10-02", "昨日"))
	assert.Empty(t, e.svc.FollowLine(e.ctx, "2026-10-08", "今日"), "21:30 才播")
	e.setNow(time.Date(2026, 10, 8, 22, 0, 0, 0, time.Local))
	assert.Equal(t, "📺 今日追剧：播出 2 集，入库 0 集", e.svc.FollowLine(e.ctx, "2026-10-08", "今日"))
	assert.Empty(t, e.svc.FollowLine(e.ctx, "2026-10-05", "昨日"), "这天没有播出的集")
	e.svc.cfg.Recognizer = noKeyRecognizer{e.svc.cfg.Recognizer}
	assert.Empty(t, e.svc.FollowLine(e.ctx, "2026-10-02", "昨日"))
}

func TestEntryTitleAndLabels(t *testing.T) {
	at := time.Date(2026, 10, 8, 21, 30, 0, 0, time.Local)
	assert.Equal(t, "沙丘2 (2024) 上映", EntryTitle(CalendarEntry{MediaType: models.MediaKindMovie, Title: "沙丘2", Year: 2024}))
	assert.Equal(t, "剧 第 1 季 E02", EntryTitle(CalendarEntry{MediaType: models.MediaKindTV, Title: "剧", Season: 1, Episode: 2}))
	assert.Equal(t, "21:30", AirClock(CalendarEntry{AirAt: &at}))
	assert.Empty(t, AirClock(CalendarEntry{}))
	assert.Equal(t, "等版本", StateLabel(models.MediaEpisodeWaitingVersion))
	assert.Equal(t, "不追", StateLabel(models.MediaEpisodeSkipped))
	assert.Equal(t, "odd", StateLabel("odd"))
	assert.Equal(t, "今天上映 1 部电影：\n· 沙丘2 (2024) 上映 · 已入库",
		todayText([]CalendarEntry{{MediaType: models.MediaKindMovie, Title: "沙丘2", Year: 2024, State: models.MediaEpisodeLibrary}}))
	assert.Equal(t, "今天播出 1 集、上映 1 部电影：\n· 剧 第 1 季 E02 · 下载中\n· 沙丘2 上映 · 缺", todayText([]CalendarEntry{
		{MediaType: models.MediaKindTV, Title: "剧", Season: 1, Episode: 2, State: models.MediaEpisodeDownloading},
		{MediaType: models.MediaKindMovie, Title: "沙丘2", State: models.MediaEpisodeMissing},
	}))
}
