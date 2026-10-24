package subscribe

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/schedule"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func (e *env) setNow(t time.Time) {
	e.mu.Lock()
	e.now = t
	e.mu.Unlock()
}

// follow 建一个追剧（现在就出发第 4 季），按追剧的默认设置；mod 可以改输入。
func (e *env) follow(mod func(*SubscriptionInput)) *models.MediaSubscription {
	e.t.Helper()
	in := SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 231620, Season: 4}
	if mod != nil {
		mod(&in)
	}
	FollowDefaults(&in)
	return e.sub(in)
}

func (e *env) ledger(sub *models.MediaSubscription) *ledger {
	e.t.Helper()
	row := e.subRow(sub.ID)
	lg, err := e.svc.rebuildLedger(e.ctx, &row, rebuildOpts{adopt: true})
	require.NoError(e.t, err)
	return lg
}

func (e *env) state(sub *models.MediaSubscription, n int) string {
	e.t.Helper()
	var row models.MediaSubscriptionEpisode
	require.NoError(e.t, e.db.Where("subscription_id = ? AND episode = ?", sub.ID, n).Limit(1).Find(&row).Error)
	return row.State
}

func (e *env) runs(id uint) []RunView {
	e.t.Helper()
	page, err := e.svc.Runs(e.ctx, id, 100, 0)
	require.NoError(e.t, err)
	return page.Items
}

func (e *env) noticesOf(kind string) []Notice {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []Notice
	for _, n := range e.notices {
		if n.Kind == kind {
			out = append(out, n)
		}
	}
	return out
}

func off() *bool { v := false; return &v }

// sameTime 按时刻比较（库里读出来的时间与测试里的时区不同）。
func sameTime(t *testing.T, want, got time.Time, msg ...any) {
	t.Helper()
	assert.True(t, want.Equal(got), "want %s, got %s %v", want, got, msg)
}

func TestFollowDefaults(t *testing.T) {
	in := SubscriptionInput{MediaType: models.MediaKindTV}
	FollowDefaults(&in)
	require.NotNil(t, in.Schedule)
	assert.Equal(t, models.MediaScheduleAirtime, in.Schedule.Kind)
	require.NotNil(t, in.LockVersion)
	assert.True(t, *in.LockVersion, "新建的追剧默认开着版本锁")

	keep := SubscriptionInput{MediaType: models.MediaKindTV, Schedule: &Schedule{Kind: models.MediaScheduleWeekly, Days: []int{6}}, LockVersion: off()}
	FollowDefaults(&keep)
	assert.Equal(t, models.MediaScheduleWeekly, keep.Schedule.Kind, "指定了的不改")
	assert.False(t, *keep.LockVersion)

	movie := SubscriptionInput{MediaType: models.MediaKindMovie}
	FollowDefaults(&movie)
	assert.Nil(t, movie.Schedule)
	assert.Nil(t, movie.LockVersion)
}

func TestScheduleNormalize(t *testing.T) {
	ok := []struct {
		in, want Schedule
		media    string
	}{
		{Schedule{}, Schedule{}, models.MediaKindMovie},
		{Schedule{Kind: models.MediaScheduleAirtime, At: " 8:05 ", AirWindowDays: 60}, Schedule{Kind: models.MediaScheduleAirtime, At: "08:05", AirWindowDays: 60}, models.MediaKindTV},
		{Schedule{Kind: models.MediaScheduleAirtime, Every: 3, Days: []int{1}}, Schedule{Kind: models.MediaScheduleAirtime}, models.MediaKindTV},
		{Schedule{Kind: models.MediaScheduleDaily}, Schedule{Kind: models.MediaScheduleDaily, At: "20:00"}, models.MediaKindMovie},
		{Schedule{Kind: models.MediaScheduleWeekly, At: "9:30", Days: []int{6, 0, 6}}, Schedule{Kind: models.MediaScheduleWeekly, At: "09:30", Days: []int{0, 6}}, models.MediaKindTV},
		{Schedule{Kind: models.MediaScheduleEveryDays, At: "23:59", Every: 30, Days: []int{1}}, Schedule{Kind: models.MediaScheduleEveryDays, At: "23:59", Every: 30}, models.MediaKindTV},
		// 按间隔：只留分钟数；电影也能用
		{Schedule{Kind: models.MediaScheduleInterval, IntervalMinutes: 90, At: "08:00", Cron: "x"}, Schedule{Kind: models.MediaScheduleInterval, IntervalMinutes: 90}, models.MediaKindMovie},
		// cron：存规整后的写法
		{Schedule{Kind: models.MediaScheduleCron, Cron: " 0  22 * * fri ;\n0 9,13 * * 6 ", Every: 3}, Schedule{Kind: models.MediaScheduleCron, Cron: "0 22 * * fri; 0 9,13 * * 6"}, models.MediaKindTV},
	}
	for _, c := range ok {
		got, err := c.in.normalize(c.media)
		require.NoError(t, err, "%+v", c.in)
		assert.Equal(t, c.want, got, "%+v", c.in)
	}
	for _, c := range []struct {
		in    Schedule
		media string
		why   string
	}{
		{Schedule{Kind: models.MediaScheduleAirtime}, models.MediaKindMovie, "只用于剧集"},
		{Schedule{Kind: models.MediaScheduleAirtime, At: "24:00"}, models.MediaKindTV, "HH:MM"},
		{Schedule{Kind: models.MediaScheduleAirtime, AirWindowDays: 61}, models.MediaKindTV, "集中重试"},
		{Schedule{Kind: models.MediaScheduleAirtime, AirDelayHours: -1}, models.MediaKindTV, "开始搜"},
		{Schedule{Kind: models.MediaScheduleEveryDays, Every: 31}, models.MediaKindTV, "每隔几天"},
		{Schedule{Kind: models.MediaScheduleWeekly, Days: []int{7}}, models.MediaKindTV, "星期几"},
		{Schedule{Kind: models.MediaScheduleMonthly}, models.MediaKindTV, "几号"},
		{Schedule{Kind: "hourly"}, models.MediaKindTV, "定时要选"},
		{Schedule{Kind: models.MediaScheduleInterval, IntervalMinutes: 29}, models.MediaKindTV, "30 分钟到 7 天"},
		{Schedule{Kind: models.MediaScheduleInterval, IntervalMinutes: 7*24*60 + 1}, models.MediaKindTV, "30 分钟到 7 天"},
		{Schedule{Kind: models.MediaScheduleCron}, models.MediaKindTV, "cron 表达式要填"},
		{Schedule{Kind: models.MediaScheduleCron, Cron: "0 25 * * *"}, models.MediaKindTV, "第 2 段（小时）"},
	} {
		_, err := c.in.normalize(c.media)
		require.ErrorIs(t, err, ErrInvalid, "%+v", c.in)
		assert.Contains(t, err.Error(), c.why)
	}
}

func TestFollowCreate(t *testing.T) {
	e := newEnv(t)
	sub := e.follow(nil)
	assert.Equal(t, models.MediaScheduleAirtime, sub.ScheduleKind)
	assert.True(t, sub.LockVersion)
	assert.Equal(t, 6, sub.TotalEpisodes)
	require.NotNil(t, sub.NextSearchAt)
	sameTime(t, e.Now(), *sub.NextSearchAt, "建好下一拍就搜")
	runs := e.runs(sub.ID)
	require.Len(t, runs, 1)
	assert.Equal(t, models.MediaRunCreate, runs[0].Trigger)
	assert.Contains(t, runs[0].Summary, "按播出时间搜索")
	assert.Contains(t, runs[0].Summary, "版本锁开")

	// 定时与媒体库都要对
	for _, in := range []SubscriptionInput{
		{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Schedule: &Schedule{Kind: "hourly"}},
		{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Schedule: &Schedule{Kind: models.MediaScheduleWeekly}},
		{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Schedule: &Schedule{Kind: models.MediaScheduleAirtime, At: "25:00"}},
		{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Schedule: &Schedule{Kind: models.MediaScheduleEveryDays, Every: 31}},
		{MediaType: models.MediaKindMovie, TMDBID: 693134, Schedule: &Schedule{Kind: models.MediaScheduleAirtime}},
		{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, LibraryID: func() *uint { v := uint(9); return &v }()},
		{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, StartEpisode: func() *int { v := -1; return &v }()},
	} {
		_, err := e.svc.CreateSubscription(e.ctx, in, models.MediaSubFromManual)
		assert.ErrorIs(t, err, ErrInvalid, "%+v", in)
	}

	// 每周：星期几去重排好；电影可以按每天搜索
	weekly, err := e.svc.CreateSubscription(e.ctx, SubscriptionInput{
		MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2,
		Schedule: &Schedule{Kind: models.MediaScheduleWeekly, Days: []int{6, 0, 6}, At: "21:30"},
	}, models.MediaSubFromManual)
	require.NoError(t, err)
	assert.Equal(t, Schedule{Kind: models.MediaScheduleWeekly, At: "21:30", Days: []int{0, 6}}, scheduleOf(weekly))
	movie, err := e.svc.CreateSubscription(e.ctx, SubscriptionInput{
		MediaType: models.MediaKindMovie, TMDBID: 693134,
		Schedule: &Schedule{Kind: models.MediaScheduleDaily},
	}, models.MediaSubFromManual)
	require.NoError(t, err)
	assert.Equal(t, Schedule{Kind: models.MediaScheduleDaily, At: "20:00", Days: []int{}}, scheduleOf(movie))
	assert.False(t, movie.LockVersion, "电影没有版本锁")
}

func TestNextSearch(t *testing.T) {
	e := newEnv(t)
	set := Settings{SearchIntervalHours: 12}
	now := e.Now()
	legacy := &models.MediaSubscription{MediaType: models.MediaKindTV}
	sameTime(t, now.Add(12*time.Hour), e.svc.nextSearch(legacy, set, nil, now), "没有定时的沿用全局间隔")

	daily := &models.MediaSubscription{MediaType: models.MediaKindTV, ScheduleKind: models.MediaScheduleDaily, ScheduleAt: "21:00", CreatedAt: now}
	want, ok := schedule.Next(schedule.Spec{Kind: schedule.Daily, At: "21:00"}, now, time.Local)
	require.True(t, ok)
	got := e.svc.nextSearch(daily, set, nil, now)
	sameTime(t, want.Add(-scheduleJitter/2), got, "定时加前后 5 分钟的偏移（测试里偏移是 0，所以早 5 分钟）")
	if want.Sub(now) < minNextSearch+scheduleJitter {
		assert.False(t, got.Before(now.Add(minNextSearch)))
	}

	// 按间隔：从这次算起；偏移前后各最多 5 分钟（短间隔按十分之一）
	interval := &models.MediaSubscription{MediaType: models.MediaKindTV, ScheduleKind: models.MediaScheduleInterval, ScheduleIntervalMinutes: 180, CreatedAt: now}
	sameTime(t, now.Add(180*time.Minute-scheduleJitter/2), e.svc.nextSearch(interval, set, nil, now), "3 小时（测试里偏移是 0，所以早 5 分钟）")
	short := &models.MediaSubscription{MediaType: models.MediaKindTV, ScheduleKind: models.MediaScheduleInterval, ScheduleIntervalMinutes: 30, CreatedAt: now}
	sameTime(t, now.Add(minNextSearch), e.svc.nextSearch(short, set, nil, now), "最短 30 分钟一次")

	// cron：按表达式下一个到点的时间（加偏移），几条取最早的
	cron := &models.MediaSubscription{MediaType: models.MediaKindTV, ScheduleKind: models.MediaScheduleCron, ScheduleCron: "0 22 * * 5; 0 9 * * 6", CreatedAt: now}
	wantCron, ok := schedule.Next(schedule.Spec{Kind: schedule.Cron, Expr: "0 22 * * 5; 0 9 * * 6"}, now, time.Local)
	require.True(t, ok)
	gotCron := e.svc.nextSearch(cron, set, nil, now)
	if wantCron.Sub(now) > minNextSearch+scheduleJitter {
		sameTime(t, wantCron.Add(-scheduleJitter/2), gotCron)
	} else {
		assert.False(t, gotCron.Before(now.Add(minNextSearch)))
	}

	// 按播出时间：台账里每一集各算一个时间，取最早的
	sub := &models.MediaSubscription{MediaType: models.MediaKindTV, ScheduleKind: models.MediaScheduleAirtime}
	local := now.In(time.Local)
	day := func(d int) string { return local.AddDate(0, 0, d).Format(time.DateOnly) }
	lg := func(rows ...models.MediaSubscriptionEpisode) *ledger {
		l := &ledger{}
		for i := range rows {
			rows[i].Episode = i + 1
			l.required = append(l.required, i+1)
		}
		l.rows = rows
		return l
	}
	tomorrow := lg(models.MediaSubscriptionEpisode{State: models.MediaEpisodeUpcoming, AirDate: day(1)})
	sameTime(t, now.Add(24*time.Hour), airtimeNext(sub, tomorrow, now), "播出在一天以后：先按每天一次看 TMDB")
	soonAt := local.Add(3 * time.Hour)
	airing := &models.MediaSubscription{MediaType: models.MediaKindTV, ScheduleKind: models.MediaScheduleAirtime, ScheduleAt: soonAt.Format("15:04")}
	later := lg(models.MediaSubscriptionEpisode{State: models.MediaEpisodeUpcoming, AirDate: soonAt.Format(time.DateOnly)})
	ta, ok := airTime(airing, soonAt.Format(time.DateOnly))
	require.True(t, ok)
	sameTime(t, soonAt.Truncate(time.Minute), ta, "播出时刻按订阅的钟点")
	sameTime(t, now.Add(5*time.Hour), airtimeNext(airing, later, now), "还没播的：播出时刻加 2 小时")
	yesterday := lg(models.MediaSubscriptionEpisode{State: models.MediaEpisodeMissing, AirDate: day(-1)})
	sameTime(t, now.Add(6*time.Hour), airtimeNext(sub, yesterday, now), "播出以后 7 天里：隔 6 小时再搜")
	old := lg(models.MediaSubscriptionEpisode{State: models.MediaEpisodeMissing, AirDate: day(-10)})
	sameTime(t, now.Add(24*time.Hour), airtimeNext(sub, old, now), "更早的每天一次")
	undated := lg(models.MediaSubscriptionEpisode{State: models.MediaEpisodeUpcoming})
	sameTime(t, now.Add(24*time.Hour), airtimeNext(sub, undated, now))
	have := lg(models.MediaSubscriptionEpisode{State: models.MediaEpisodeLibrary, AirDate: day(-1)}, models.MediaSubscriptionEpisode{State: models.MediaEpisodeDownloading, AirDate: day(-1)})
	sameTime(t, now.Add(24*time.Hour), airtimeNext(sub, have, now), "都有了也每天看一次 TMDB")
	both := lg(models.MediaSubscriptionEpisode{State: models.MediaEpisodeMissing, AirDate: day(-10)}, models.MediaSubscriptionEpisode{State: models.MediaEpisodeUpcoming, AirDate: soonAt.Format(time.DateOnly)})
	sameTime(t, now.Add(5*time.Hour), airtimeNext(airing, both, now), "取最早的")

	custom := &models.MediaSubscription{MediaType: models.MediaKindTV, ScheduleKind: models.MediaScheduleAirtime, ScheduleAt: soonAt.Format("15:04"), AirDelayHours: 1, AirRetryHours: 3, AirWindowDays: 2}
	sameTime(t, now.Add(4*time.Hour), airtimeNext(custom, later, now))
	sameTime(t, now.Add(3*time.Hour), airtimeNext(custom, yesterday, now))
	threeDays := lg(models.MediaSubscriptionEpisode{State: models.MediaEpisodeMissing, AirDate: day(-3)})
	sameTime(t, now.Add(24*time.Hour), airtimeNext(custom, threeDays, now), "集中重试只有 2 天")

	// 今天的这一集再过 10 分钟就到「播出时刻加 2 小时」：下次搜索按 30 分钟算
	soon := &models.MediaSubscription{MediaType: models.MediaKindTV, ScheduleKind: models.MediaScheduleAirtime, ScheduleAt: local.Add(-110 * time.Minute).Format("15:04")}
	if local.Add(-110*time.Minute).Day() == local.Day() {
		todayRow := lg(models.MediaSubscriptionEpisode{State: models.MediaEpisodeUpcoming, AirDate: day(0)})
		sameTime(t, now.Add(minNextSearch), e.svc.nextSearch(soon, set, todayRow, now))
	}
}

func TestLedgerRebuild(t *testing.T) {
	e := newEnv(t)
	sub := e.follow(func(in *SubscriptionInput) { in.LockVersion = off() })
	e.organized("h1", models.MediaKindTV, 231620, 4, 1, "/lib/nh/E01.mkv")
	require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Where("info_hash = ?", "h1").
		Update("torrent_name", "Natural.High.S04E01.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB").Error)
	lg := e.ledger(sub)
	assert.Equal(t, 6, lg.total)
	assert.Equal(t, []int{1, 2, 3, 4, 5, 6}, lg.required)
	assert.Equal(t, []int{2, 3, 4}, lg.missing, "已播没入库的是缺的")
	assert.Equal(t, models.MediaEpisodeLibrary, lg.row(1).State)
	assert.Equal(t, "HHWEB", lg.row(1).VersionGroup)
	assert.Equal(t, "2160p", lg.row(1).VersionResolution)
	assert.Equal(t, models.MediaEpisodeUpcoming, lg.row(5).State)
	assert.Equal(t, models.MediaEpisodeUpcoming, lg.row(6).State, "没有播出日期的算还没播")
	assert.False(t, lg.requiredAired())

	// 跳过一集：不算缺；从第 3 集开始追：前两集不追
	_, err := e.svc.SkipEpisode(e.ctx, sub.ID, 3, true)
	require.NoError(t, err)
	assert.Equal(t, models.MediaEpisodeSkipped, e.state(sub, 3))
	start := 2
	_, err = e.svc.UpdateSubscription(e.ctx, sub.ID, SubscriptionInput{StartEpisode: &start})
	require.NoError(t, err)
	lg = e.ledger(sub)
	assert.Equal(t, []int{2, 4, 5, 6}, lg.required)
	assert.Equal(t, []int{2, 4}, lg.missing)
	assert.Equal(t, models.MediaEpisodeLibrary, lg.row(1).State, "起始集以前的、已经入库的仍显示已入库")
	_, err = e.svc.SkipEpisode(e.ctx, sub.ID, 3, false)
	require.NoError(t, err)
	assert.Equal(t, models.MediaEpisodeMissing, e.state(sub, 3))
	_, err = e.svc.SkipEpisode(e.ctx, sub.ID, 9, true)
	assert.ErrorIs(t, err, ErrInvalid)

	// TMDB 多列了一集：有效总集数取大并写回；手动填的集数以手动的为准
	e.tmdb.set("/3/tv/231620/season/4", strings.Replace(varietySeason4, `{"episode_number":6,"name":"第 6 集"}]}`, `{"episode_number":6,"name":"第 6 集"},{"episode_number":7,"name":"第4期上"}]}`, 1))
	row := e.subRow(sub.ID)
	lg, err = e.svc.rebuildLedger(e.ctx, &row, rebuildOpts{fresh: true})
	require.NoError(t, err)
	assert.Equal(t, 7, lg.total)
	assert.Equal(t, 7, e.subRow(sub.ID).TotalEpisodes)
	total := 10
	_, err = e.svc.UpdateSubscription(e.ctx, sub.ID, SubscriptionInput{TotalEpisodes: &total})
	require.NoError(t, err)
	got := e.subRow(sub.ID)
	assert.True(t, got.TotalEpisodesManual)
	lg = e.ledger(sub)
	assert.Equal(t, 10, lg.total)
	assert.Len(t, lg.rows, 10, "TMDB 没列出的集也在台账里（还没播）")
	assert.False(t, lg.requiredAired(), "总集数比 TMDB 列出的多：没播完")
	total = 0
	_, err = e.svc.UpdateSubscription(e.ctx, sub.ID, SubscriptionInput{TotalEpisodes: &total})
	require.NoError(t, err)
	assert.False(t, e.subRow(sub.ID).TotalEpisodesManual)

	// 进度读台账
	p, err := e.svc.progress(e.ctx, &got, true)
	require.NoError(t, err)
	assert.Zero(t, p.Skipped)
	assert.Equal(t, 1, p.InLibrary)
	require.NotEmpty(t, p.Episodes)
	assert.Equal(t, "HHWEB 2160p", p.Episodes[0].Version)
}

func TestLedgerAdoptsDownloaderTorrent(t *testing.T) {
	e := newEnv(t)
	sub := e.follow(func(in *SubscriptionInput) { in.LockVersion = off() })
	e.organized("h1", models.MediaKindTV, 231620, 4, 1, "/lib/nh/E01.mkv")
	e.dl.setTorrents(
		downloader.Torrent{InfoHash: "AAAA0000000000000000000000000000000000E2", Name: "Natural.High.S04E02.2023.1080p.TX.WEB-DL.H.264.DDP2.0-HHWEB", TotalSize: 1 << 30},
		downloader.Torrent{InfoHash: "bbbb0000000000000000000000000000000000e1", Name: "Natural.High.S04E01.2023.1080p.TX.WEB-DL.H.264.DDP2.0-HHWEB"},
		downloader.Torrent{InfoHash: "cccc000000000000000000000000000000000000", Name: "Other.Show.S04E03.2023.1080p.WEB-DL-XX"},
	)
	lg := e.ledger(sub)
	assert.Equal(t, models.MediaEpisodeDownloading, lg.row(2).State)
	assert.Equal(t, models.MediaEpisodeFromDownloader, lg.row(2).Source)
	assert.Equal(t, []int{3, 4}, lg.missing)
	linked := e.linked(sub.ID)
	require.Len(t, linked, 1, "只认领有还缺的集的、对得上的种子（第 1 集已经入库）")
	assert.Equal(t, "downloader:1", linked[0].SiteName)
	assert.Equal(t, "aaaa0000000000000000000000000000000000e2", linked[0].TorrentID)
	assert.True(t, linked[0].Adopted)
	assert.Equal(t, models.MediaSubTorrentDownloading, linked[0].Status)
	e.ledger(sub)
	assert.Len(t, e.linked(sub.ID), 1, "认领过的不再认领")

	// 下载器里已经在下的集不再推
	e.search.set(e.item("mteam", "402", "Natural.High.S04E02.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "", 2, 10),
		e.item("mteam", "403", "Natural.High.S04E03.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "", 2, 10))
	e.enable(nil)
	msg, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	pushes := e.gotPushes()
	require.Len(t, pushes, 1, msg)
	assert.Equal(t, "403", pushes[0].TorrentID)

	// 读不到下载器：这一轮不推缺的集
	e.dl.mu.Lock()
	e.dl.listErr = errors.New("连接超时")
	e.dl.mu.Unlock()
	e.advance(2 * time.Minute) // 下载器的种子列表缓存 1 分钟
	e.search.set(e.item("mteam", "404", "Natural.High.S04E04.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "", 2, 10))
	msg, err = e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Contains(t, msg, "读不到下载器")
	assert.Len(t, e.gotPushes(), 1)
	runs := e.runs(sub.ID)
	assert.Equal(t, models.MediaRunWaiting, runs[0].Status)
}

// 种子名写的是整季、里面只有一集（M-Team 上 HHWEB 一集一个种子，种子名是季的目录名），季包也可能只选了几集：
// 按下载器里选中的文件认集；先加的先认领，版本锁跟着第一个下载的版本；读不到文件列表的整季种子不认领
func TestAdoptBySelectedFiles(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	sub := e.follow(nil)
	file := func(name string, prio int) downloader.TorrentFile {
		return downloader.TorrentFile{Name: name, Priority: prio, Size: 1 << 30}
	}
	e.dl.setTorrents(
		downloader.Torrent{ID: "h03", InfoHash: "H03", Name: "Natural.High.S04.2023.1080p.TX.WEB-DL.H.264.DDP2.0-HHWEB", DateAdded: 200},
		downloader.Torrent{ID: "h01", InfoHash: "H01", Name: "Natural.High.S04.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", DateAdded: 100},
		downloader.Torrent{ID: "hpk", InfoHash: "HPK", Name: "Natural.High.S04.2023.1080p.WEB-DL.H.264-PTerWEB", DateAdded: 300},
		downloader.Torrent{ID: "hno", InfoHash: "HNO", Name: "Natural.High.S04.2023.720p.WEB-DL.H.264-QAWEB", DateAdded: 50},
	)
	e.dl.setFiles("h03", file("Natural.High.S04.2023.1080p.TX.WEB-DL.H.264.DDP2.0-HHWEB/Natural.High.S04E03.2023.1080p.TX.WEB-DL.H.264.DDP2.0-HHWEB.mkv", 1))
	e.dl.setFiles("h01", file("Natural.High.S04.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB/Natural.High.S04E01.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB.mkv", 1))
	e.dl.setFiles("hpk",
		file("pk/Natural.High.S04E01.2023.1080p.WEB-DL.H.264-PTerWEB.mkv", 1),
		file("pk/Natural.High.S04E02.2023.1080p.WEB-DL.H.264-PTerWEB.mkv", 0),
		file("pk/Natural.High.S04E04.2023.1080p.WEB-DL.H.264-PTerWEB.mkv", 1),
		file("pk/readme.txt", 1),
	)
	e.dl.mu.Lock()
	e.dl.filesErr = map[string]error{"hno": errors.New("连接超时")}
	e.dl.mu.Unlock()

	lg := e.ledger(sub)
	assert.Equal(t, models.MediaEpisodeDownloading, lg.row(1).State)
	assert.Equal(t, "HHWEB", lg.row(1).VersionGroup, "第 1 集按先加的种子")
	assert.Equal(t, "2160p", lg.row(1).VersionResolution)
	assert.Equal(t, models.MediaEpisodeMissing, lg.row(2).State, "没选的文件不算")
	assert.Equal(t, models.MediaEpisodeDownloading, lg.row(3).State)
	assert.Equal(t, models.MediaEpisodeDownloading, lg.row(4).State)
	assert.Equal(t, models.MediaEpisodeUpcoming, lg.row(5).State, "种子名写整季也不把还没播的集算成在下载")
	linked := e.linked(sub.ID)
	ids := make([]string, 0, len(linked))
	for _, l := range linked {
		ids = append(ids, l.TorrentID)
	}
	assert.Equal(t, []string{"h01", "h03", "hpk"}, ids, "先加的先认领；读不到文件列表的整季种子不认领")
	require.Len(t, linked, 3)
	assert.Equal(t, `[1]`, linked[0].CoveredEpisodes)
	assert.Equal(t, `[1,4]`, linked[2].CoveredEpisodes, "季包里选中的集")
	assert.False(t, linked[2].Complete)

	// 搜一次：锁跟着第一个下载的版本（HHWEB 2160p），推第 2 集
	e.search.set(
		e.item("mteam", "702", "Natural.High.S04E02.2023.1080p.TX.WEB-DL.H.264.DDP2.0-HHWEB", "", 1, 10),
		e.item("mteam", "703", "Natural.High.S04E02.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "", 2, 10),
	)
	msg, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"703"}, pushedIDs(e), msg)
	got := e.subRow(sub.ID)
	assert.Equal(t, "HHWEB", got.LockGroup)
	assert.Equal(t, "2160p", got.LockResolution)
}

func TestDownloaderListCached(t *testing.T) {
	e := newEnv(t)
	sub := e.follow(nil)
	e.ledger(sub)
	e.ledger(sub)
	e.dl.mu.Lock()
	n := e.dl.lists
	e.dl.mu.Unlock()
	assert.Equal(t, 1, n, "每个下载器每分钟最多列一次")
	e.advance(61 * time.Second)
	e.ledger(sub)
	e.dl.mu.Lock()
	n = e.dl.lists
	e.dl.mu.Unlock()
	assert.Equal(t, 2, n)
}

// 版本锁的候选：HHWEB 的做种多，排在同分辨率的前面。
func lockItems(e *env) []v2.TorrentItem {
	return []v2.TorrentItem{
		e.item("mteam", "201", "Natural.High.S04E01.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "", 2, 90),
		e.item("mteam", "202", "Natural.High.S04E01.2023.1080p.WEB-DL.H.264-PTerWEB", "", 1, 99),
		e.item("mteam", "203", "Natural.High.S04E02.2023.2160p.WEB-DL.H.265-PTerWEB", "", 2, 70),
		e.item("mteam", "204", "Natural.High.S04E02.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "", 2, 80),
		e.item("mteam", "205", "Natural.High.S04E03.2023.2160p.WEB-DL.H.265-FRDS", "", 2, 60),
	}
}

func pushedIDs(e *env) []string {
	var ids []string
	for _, p := range e.gotPushes() {
		ids = append(ids, p.TorrentID)
	}
	return ids
}

func TestVersionLockRoundThenPersisted(t *testing.T) {
	e := newEnv(t)
	sub := e.follow(nil)
	e.enable(nil)
	e.search.set(lockItems(e)...)
	msg, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"201", "204"}, pushedIDs(e), msg)
	got := e.subRow(sub.ID)
	assert.Equal(t, "HHWEB", got.LockGroup)
	assert.Equal(t, "2160p", got.LockResolution)
	// 推送以后台账马上是新的：不用等下一次刷新
	assert.Equal(t, models.MediaEpisodeDownloading, e.state(sub, 1))
	assert.Equal(t, models.MediaEpisodeDownloading, e.state(sub, 2))
	runs := e.runs(sub.ID)
	require.NotEmpty(t, runs)
	var lockSteps []string
	for _, st := range runs[0].Steps {
		if st.Kind == stepLock {
			lockSteps = append(lockSteps, st.Text)
		}
	}
	assert.Contains(t, strings.Join(lockSteps, "\n"), "版本锁定为 HHWEB 2160p")
	assert.Contains(t, strings.Join(lockSteps, "\n"), "PTerWEB：版本锁，要 HHWEB 2160p")
	assert.Equal(t, models.MediaRunOK, runs[0].Status)

	// 第 3 集只有别的版本：接着等
	msg, err = e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Contains(t, msg, "找到的都不是锁住的版本（HHWEB 2160p）")
	assert.Len(t, e.gotPushes(), 2)
	assert.Equal(t, models.MediaRunWaiting, e.runs(sub.ID)[0].Status)
}

func TestVersionLockNotWrittenWithoutPush(t *testing.T) {
	e := newEnv(t)
	sub := e.follow(nil)
	e.enable(nil)
	e.search.set(lockItems(e)...)
	e.pushErr = errors.New("下载器断开")
	_, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	got := e.subRow(sub.ID)
	assert.Empty(t, got.LockGroup, "一个也没推成：不留下没有实际版本的锁")
	assert.Empty(t, got.LockResolution)
	assert.Equal(t, models.MediaRunFailed, e.runs(sub.ID)[0].Status)
}

func TestVersionLockChangedDuringRound(t *testing.T) {
	e := newEnv(t)
	sub := e.follow(nil)
	e.enable(nil)
	e.search.set(lockItems(e)...)
	first := true
	e.mu.Lock()
	e.onPush = func(ptinternal.PushTorrentRequest) {
		if first {
			first = false
			// 推第一个的时候用户把锁改成了 PTerWEB 2160p
			require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).
				Updates(map[string]any{"lock_group": "PTerWEB", "lock_resolution": "2160p"}).Error)
		}
	}
	e.mu.Unlock()
	_, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"201"}, pushedIDs(e), "改锁以后 HHWEB 的第 2 集不再推；PTerWEB 的不在这一轮的候选里")
	got := e.subRow(sub.ID)
	assert.Equal(t, "PTerWEB", got.LockGroup, "库里已经有锁：以用户的为准")
	steps := e.runs(sub.ID)[0].Steps
	var texts []string
	for _, st := range steps {
		texts = append(texts, st.Text)
	}
	assert.Contains(t, strings.Join(texts, "\n"), "版本锁改成了 PTerWEB 2160p")
}

func TestVersionLockFromLibrary(t *testing.T) {
	e := newEnv(t)
	sub := e.follow(nil)
	e.enable(nil)
	e.organized("h1", models.MediaKindTV, 231620, 4, 1, "/lib/nh/E01.mkv")
	require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Where("info_hash = ?", "h1").
		Update("torrent_name", "Natural.High.S04E01.2023.1080p.TX.WEB-DL.H.264.DDP2.0-HHWEB").Error)
	e.search.set(
		e.item("mteam", "301", "Natural.High.S04E02.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "", 2, 90),
		e.item("mteam", "302", "Natural.High.S04E02.2023.1080p.TX.WEB-DL.H.264.DDP2.0-HHWEB", "", 1, 10),
	)
	_, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"302"}, pushedIDs(e), "库里第 1 集是 HHWEB 1080p：锁跟着它")
	got := e.subRow(sub.ID)
	assert.Equal(t, "HHWEB", got.LockGroup)
	assert.Equal(t, "1080p", got.LockResolution)
}

// 原来的制作组不更新了：库里第 1 集是 MWeb 2160p，锁跟着它；站点上只有 HHWEB 的第 2 集。
func clearedLockEnv(t *testing.T) (*env, *models.MediaSubscription) {
	t.Helper()
	e := newEnv(t)
	sub := e.follow(nil)
	e.enable(nil)
	e.organized("h1", models.MediaKindTV, 231620, 4, 1, "/lib/nh/E01.mkv")
	require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Where("info_hash = ?", "h1").
		Update("torrent_name", "Natural.High.S04E01.2023.2160p.WEB-DL.H.265-MWeb").Error)
	e.search.set(e.item("mteam", "401", "Natural.High.S04E02.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "", 2, 90))
	msg, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	require.Empty(t, pushedIDs(e), msg)
	require.Equal(t, "MWeb", e.subRow(sub.ID).LockGroup, "锁跟着库里的第 1 集")

	empty := ""
	_, err = e.svc.UpdateSubscription(e.ctx, sub.ID, SubscriptionInput{LockGroup: &empty, LockResolution: &empty})
	require.NoError(t, err)
	got := e.subRow(sub.ID)
	require.Empty(t, got.LockGroup)
	require.Empty(t, got.LockResolution)
	require.True(t, got.LockFromNext)
	return e, sub
}

// 清掉锁以后不按库里前几集的版本锁回去：下一个推送成功的版本成为新锁。
func TestVersionLockClearedTakesNextDownload(t *testing.T) {
	e, sub := clearedLockEnv(t)
	msg, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"401"}, pushedIDs(e), msg)
	got := e.subRow(sub.ID)
	assert.Equal(t, "HHWEB", got.LockGroup, "下一个推送成功的版本成为新锁")
	assert.Equal(t, "2160p", got.LockResolution)
	assert.False(t, got.LockFromNext, "写进锁以后不再等下一次下载")
}

// 清掉以后这一轮一个也没推成：锁仍空着，下一轮还按那时推送成功的版本定，不按库里的 MWeb；改成具体版本时按用户填的。
func TestVersionLockClearedStaysOpenUntilPushed(t *testing.T) {
	e, sub := clearedLockEnv(t)
	e.pushErr = errors.New("下载器断开")
	_, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	got := e.subRow(sub.ID)
	assert.Empty(t, got.LockGroup, "没推成：不留下锁，也不锁回库里的版本")
	assert.True(t, got.LockFromNext)

	g, r := "ADWeb", "1080p"
	_, err = e.svc.UpdateSubscription(e.ctx, sub.ID, SubscriptionInput{LockGroup: &g, LockResolution: &r})
	require.NoError(t, err)
	got = e.subRow(sub.ID)
	assert.Equal(t, "ADWeb", got.LockGroup)
	assert.False(t, got.LockFromNext, "改成了具体的版本")
}

// 洗版换成整季包以后改锁：清掉锁时留下的标记一起清掉。
func TestRelockClearsLockFromNext(t *testing.T) {
	e, sub := clearedLockEnv(t)
	row := e.subRow(sub.ID)
	e.svc.relock(e.ctx, &row, &models.MediaSubscriptionTorrent{Title: "Natural.High.S04.2023.2160p.WEB-DL.H.265-FRDS"})
	got := e.subRow(sub.ID)
	assert.Equal(t, "FRDS", got.LockGroup)
	assert.False(t, got.LockFromNext)
	assert.False(t, row.LockFromNext)
}

func TestWaitingVersionRemindAndUnlock(t *testing.T) {
	e := newEnv(t)
	group, res := "HHWEB", "2160p"
	sub := e.follow(func(in *SubscriptionInput) { in.LockGroup, in.LockResolution = &group, &res })
	e.enable(func(s *Settings) { s.NotifyChannels = []uint{1} })
	e1, ok := airTime(sub, "2026-10-01")
	require.True(t, ok)
	e.setNow(e1.Add(8 * 24 * time.Hour))
	e.svc.refresh(e.ctx)
	assert.Equal(t, models.MediaEpisodeWaitingVersion, e.state(sub, 1))
	assert.Equal(t, models.MediaEpisodeWaitingVersion, e.state(sub, 2), "第 2 集播出正好 7 天")
	assert.Equal(t, models.MediaEpisodeMissing, e.state(sub, 3))
	assert.Empty(t, e.noticesOf(NoticeFollowWaiting), "开始等以后还没搜过：不提醒")
	// 搜过一次还没有锁住的版本：提醒
	e.advance(time.Minute)
	_, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	e.svc.refresh(e.ctx)
	waits := e.noticesOf(NoticeFollowWaiting)
	require.Len(t, waits, 1)
	assert.Equal(t, "追剧等版本："+subTitle(sub), waits[0].Title)
	assert.Contains(t, waits[0].Body, fmt.Sprintf("/unlock %d 1", sub.ID))
	ev := waits[0].Event
	require.NotNil(t, ev)
	assert.Equal(t, notify.EventFollowWaiting, ev.Type)
	assert.Equal(t, "E01、E02", ev.Fields["episodes"])
	assert.Equal(t, "HHWEB 2160p", ev.Fields["version"])
	assert.Equal(t, int64(sub.ID), ev.Fields["subscription_id"])
	assert.Equal(t, int64(8), ev.Fields["days"])
	p, err := e.svc.progress(e.ctx, &models.MediaSubscription{ID: sub.ID, MediaType: models.MediaKindTV}, false)
	require.NoError(t, err)
	assert.Equal(t, 2, p.Waiting)
	assert.Contains(t, p.Missing, 1, "等版本的仍算缺")

	e.advance(time.Hour)
	e.svc.refresh(e.ctx)
	assert.Len(t, e.noticesOf(NoticeFollowWaiting), 1, "3 天里只提醒一次")
	e.advance(3 * 24 * time.Hour)
	e.svc.refresh(e.ctx)
	assert.Len(t, e.noticesOf(NoticeFollowWaiting), 2)

	// 这一集换版本：PTerWEB 的第 1 集可以下了，别的集仍受锁
	eps, err := e.svc.UnlockEpisode(e.ctx, sub.ID, 1, true)
	require.NoError(t, err)
	assert.Equal(t, models.MediaEpisodeMissing, eps[0].State)
	assert.True(t, eps[0].ManualUnlock)
	e.search.set(
		e.item("mteam", "501", "Natural.High.S04E01.2023.1080p.WEB-DL.H.264-PTerWEB", "", 1, 10),
		e.item("mteam", "502", "Natural.High.S04E02.2023.1080p.WEB-DL.H.264-PTerWEB", "", 1, 10),
	)
	_, err = e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"501"}, pushedIDs(e))
}

func TestFromTorrent(t *testing.T) {
	e := newEnv(t)
	e.enable(func(s *Settings) { s.NotifyChannels = []uint{1} })
	it := e.item("mteam", "601", "Natural.High.S04E03.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "", 2, 10)
	e.search.cache(it)
	sub := e.follow(func(in *SubscriptionInput) { in.FromTorrent = &FromTorrent{Site: "MTeam", TorrentID: "601"} })
	assert.Equal(t, 3, sub.StartEpisode, "起始集默认取这个种子的集号")
	assert.True(t, strings.HasPrefix(sub.Message, "从这个种子开始追：下载了"), sub.Message)
	assert.Equal(t, []string{"601"}, pushedIDs(e))
	got := e.subRow(sub.ID)
	assert.Equal(t, "HHWEB", got.LockGroup, "第一个下载的版本定下了锁")
	runs := e.runs(sub.ID)
	require.Len(t, runs, 1)
	assert.Equal(t, models.MediaRunCreate, runs[0].Trigger)
	assert.Equal(t, models.MediaRunOK, runs[0].Status)

	// 追剧更新的通知
	found := e.noticesOf(NoticeFollowFound)
	require.Len(t, found, 1)
	assert.Equal(t, "追剧更新："+subTitle(sub), found[0].Title)
	ev := found[0].Event
	require.NotNil(t, ev)
	assert.Equal(t, notify.EventFollowFound, ev.Type)
	assert.Equal(t, "第 3 集", ev.Fields["episodes"])
	assert.Equal(t, "mteam", ev.Fields["site"])
	assert.Equal(t, int64(4), ev.Fields["season"])
	assert.Empty(t, e.noticesOf(NoticeDownloaded), "追剧不再发订阅下载")
}

// 选中的种子没推成（种子文件没下载下来）：版本锁按它的版本定下，重新搜索时推的还是这个版本，不会换成分数更高的
// 别的版本（以前锁要等第一次推送成功才定，重新搜索的本轮锁按分数最高的候选定，用户选的版本就丢了）
func TestFromTorrentPushFailedKeepsChosenVersion(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	chosen := e.item("mteam", "611", "Natural.High.S04E01.2023.1080p.WEB-DL.H.264-HHWEB", "", 1, 10)
	file := e.site.files["611"]
	delete(e.site.files, "611")
	e.search.cache(chosen)
	sub := e.follow(func(in *SubscriptionInput) { in.FromTorrent = &FromTorrent{Site: "mteam", TorrentID: "611"} })
	assert.Empty(t, pushedIDs(e))
	got := e.subRow(sub.ID)
	assert.Equal(t, "HHWEB", got.LockGroup, "按选中的种子定了锁")
	assert.Equal(t, "1080p", got.LockResolution)
	runs := e.runs(sub.ID)
	require.Len(t, runs, 1)
	assert.Equal(t, models.MediaRunFailed, runs[0].Status)
	assert.Contains(t, stepsOf(runs[0]), "选中的种子没推成：版本锁按它的版本（HHWEB 1080p）定下，重新搜索时还要这个版本")

	// 种子文件能下载了：重新搜索推 HHWEB 1080p 的第 1、2 集；分数更高的 PTerWEB 2160p 是别的版本
	e.site.files["611"] = file
	e.search.set(
		chosen,
		e.item("mteam", "612", "Natural.High.S04E02.2023.1080p.WEB-DL.H.264-HHWEB", "", 1, 10),
		e.item("mteam", "613", "Natural.High.S04E01.2023.2160p.WEB-DL.H.265-PTerWEB", "", 3, 10),
	)
	_, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"611", "612"}, pushedIDs(e))

	// 没开版本锁：推送失败也不定锁
	e2 := newEnv(t)
	e2.enable(nil)
	it := e2.item("mteam", "621", "Natural.High.S04E01.2023.1080p.WEB-DL.H.264-HHWEB", "", 1, 10)
	delete(e2.site.files, "621")
	e2.search.cache(it)
	off := false
	sub2 := e2.follow(func(in *SubscriptionInput) {
		in.FromTorrent = &FromTorrent{Site: "mteam", TorrentID: "621"}
		in.LockVersion = &off
	})
	got = e2.subRow(sub2.ID)
	assert.Empty(t, got.LockGroup)
	assert.Empty(t, got.LockResolution)
}

func TestFromTorrentFallbacks(t *testing.T) {
	e := newEnv(t)
	// 缓存里没有：按 title 在这个站点搜一次，编号要完全相同
	it := e.item("mteam", "701", "Natural.High.S04E02.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "", 2, 10)
	e.search.set(e.item("mteam", "7010", "Natural.High.S04E05.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "", 2, 10), it)
	start := 1
	sub := e.follow(func(in *SubscriptionInput) {
		in.FromTorrent = &FromTorrent{Site: "mteam", TorrentID: "701", Title: "Natural High S04E02"}
		in.StartEpisode = &start
	})
	assert.Equal(t, 1, sub.StartEpisode, "指定了起始集时不改")
	assert.Equal(t, []string{"701"}, pushedIDs(e))
	e.search.mu.Lock()
	q := e.search.queries[len(e.search.queries)-1]
	e.search.mu.Unlock()
	assert.Equal(t, []string{"mteam"}, q.Sites)
	assert.Equal(t, "Natural High S04E02", q.Keyword)
	require.NoError(t, e.svc.DeleteSubscription(e.ctx, sub.ID))

	// 取不到
	sub = e.follow(func(in *SubscriptionInput) { in.FromTorrent = &FromTorrent{Site: "mteam", TorrentID: "999"} })
	assert.Equal(t, "没能从站点取到这个种子，按追剧设置搜索", sub.Message)
	require.NoError(t, e.svc.DeleteSubscription(e.ctx, sub.ID))

	// 不是这一季
	other := e.item("mteam", "702", "Natural.High.S03E01.2022.1080p.WEB-DL-HHWEB", "", 1, 10)
	e.search.cache(other)
	sub = e.follow(func(in *SubscriptionInput) { in.FromTorrent = &FromTorrent{Site: "mteam", TorrentID: "702"} })
	assert.Contains(t, sub.Message, "不是这部剧的第 4 季")
	assert.Equal(t, 0, sub.StartEpisode)
	assert.Len(t, e.gotPushes(), 1)

	_, err := e.svc.CreateSubscription(e.ctx, SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, FromTorrent: &FromTorrent{Site: "mteam"}}, models.MediaSubFromManual)
	assert.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.UpdateSubscription(e.ctx, sub.ID, SubscriptionInput{FromTorrent: &FromTorrent{Site: "mteam", TorrentID: "1"}})
	assert.ErrorIs(t, err, ErrInvalid, "from_torrent 只在新建时用")
}

func TestPackVerifiedByFiles(t *testing.T) {
	e := newEnv(t)
	e.tmdb.set("/3/tv/231620/season/4", `{"season_number":4,"episodes":[
		{"episode_number":1,"name":"第1期上","air_date":"2026-10-01"},{"episode_number":2,"name":"第1期下","air_date":"2026-10-02"},
		{"episode_number":3,"name":"第2期上","air_date":"2026-10-03"},{"episode_number":4,"name":"第2期下","air_date":"2026-10-04"}]}`)
	sub := e.follow(nil)
	e.enable(nil)
	pack := e.item("mteam", "801", "Natural.High.S04.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "", 8, 10)
	e.site.files["801"] = packBytes("Natural.High.S04.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB",
		"Natural.High.S04E01.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB.mkv",
		"Natural.High.S04E02.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB.mkv",
		"Natural.High.S04E03.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB.mkv",
		"Natural.High.S04E03.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB.chs.srt")
	e.search.set(pack)
	_, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	linked := e.linked(sub.ID)
	require.Len(t, linked, 1)
	assert.True(t, linked[0].Complete)
	assert.Equal(t, "[1,2,3]", linked[0].CoveredEpisodes, "整季包按文件列表核对")
	lg := e.ledger(sub)
	assert.Equal(t, []int{4}, lg.missing, "包里没有的第 4 集仍算缺")
	e.search.set(e.item("mteam", "802", "Natural.High.S04E04.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "", 2, 10))
	_, err = e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"801", "802"}, pushedIDs(e))
}

func TestRequiredSetCompletion(t *testing.T) {
	e := newEnv(t)
	e.tmdb.set("/3/tv/231620/season/4", `{"season_number":4,"episodes":[
		{"episode_number":1,"name":"第1期上","air_date":"2026-10-01"},{"episode_number":2,"name":"第1期下","air_date":"2026-10-02"},
		{"episode_number":3,"name":"第2期上","air_date":"2026-10-03"},{"episode_number":4,"name":"第2期下","air_date":"2026-10-04"}]}`)
	start := 3
	sub := e.follow(func(in *SubscriptionInput) { in.StartEpisode = &start; in.LockVersion = off() })
	e.enable(nil)
	e.organized("h3", models.MediaKindTV, 231620, 4, 3, "/lib/nh/E03.mkv")
	e.svc.refresh(e.ctx)
	assert.Equal(t, models.MediaSubActive, e.subRow(sub.ID).Status)
	e.organized("h4", models.MediaKindTV, 231620, 4, 4, "/lib/nh/E04.mkv")
	e.svc.refresh(e.ctx)
	got := e.subRow(sub.ID)
	assert.Equal(t, models.MediaSubDone, got.Status, "必需集（第 3、4 集）都入库了就完成，不等前两集")
	require.NotNil(t, got.CompletedAt)
	sameTime(t, e.Now(), *got.CompletedAt)

	// 手动把集数改大：TMDB 还没列出那么多，不算完成
	sub2 := e.follow(func(in *SubscriptionInput) {
		in.TMDBID, in.Season = 100088, 2
		total := 3
		in.TotalEpisodes = &total
		in.LockVersion = off()
	})
	_ = sub2
	e.tmdb.set("/3/tv/100088/season/2", `{"season_number":2,"episodes":[{"episode_number":1,"air_date":"2025-04-13"},{"episode_number":2,"air_date":"2025-04-20"}]}`)
	for n := 1; n <= 2; n++ {
		e.organized(fmt.Sprintf("tl%d", n), models.MediaKindTV, 100088, 2, n, fmt.Sprintf("/lib/tlou/E%02d.mkv", n))
	}
	e.svc.refresh(e.ctx)
	assert.Equal(t, models.MediaSubActive, e.subRow(sub2.ID).Status)
}

func TestTargetReachedUsesRequired(t *testing.T) {
	e := newEnv(t)
	e.tmdb.set("/3/tv/231620/season/4", `{"season_number":4,"episodes":[
		{"episode_number":1,"name":"第1期上","air_date":"2026-10-01"},{"episode_number":2,"name":"第1期下","air_date":"2026-10-02"}]}`)
	p, err := e.svc.SaveProfile(e.ctx, 0, ProfileInput{Name: "4K", Resolutions: []string{"2160p"}})
	require.NoError(t, err)
	start := 2
	sub := e.follow(func(in *SubscriptionInput) {
		in.StartEpisode = &start
		in.Upgrade = true
		in.ProfileID = p.ID
		in.LockVersion = off()
	})
	e.enable(nil)
	row := e.organized("h2", models.MediaKindTV, 231620, 4, 2, "/lib/nh/E02.mkv")
	require.NoError(t, e.db.Model(&row).Update("torrent_name", "Natural.High.S04E02.2023.2160p.WEB-DL-HHWEB").Error)
	e.svc.refresh(e.ctx)
	assert.Equal(t, models.MediaSubDone, e.subRow(sub.ID).Status, "洗版的目标只看必需集（第 1 集不追）")
}

func TestDoneDueReopensAndFollowsNextSeason(t *testing.T) {
	e := newEnv(t)
	sub := e.follow(func(in *SubscriptionInput) { in.LockVersion = off() })
	e.enable(nil)
	for n := 1; n <= 4; n++ {
		e.organized(fmt.Sprintf("nh%d", n), models.MediaKindTV, 231620, 4, n, fmt.Sprintf("/lib/nh/E%02d.mkv", n))
	}
	done := e.Now().Add(-24 * time.Hour)
	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).
		Updates(map[string]any{"status": models.MediaSubDone, "completed_at": done}).Error)
	e.svc.doneDue(e.ctx, e.enable(nil))
	assert.Equal(t, models.MediaSubDone, e.subRow(sub.ID).Status, "还没有新的已播的集")

	// 第 5 集播出了：重新打开、马上搜
	e5, _ := airTime(sub, "2026-10-15")
	e.setNow(e5.Add(time.Hour))
	e.svc.doneDue(e.ctx, e.enable(nil))
	got := e.subRow(sub.ID)
	assert.Equal(t, models.MediaSubActive, got.Status)
	assert.Nil(t, got.CompletedAt)
	require.NotNil(t, got.NextSearchAt)
	sameTime(t, e.Now(), *got.NextSearchAt)
	assert.Contains(t, got.Message, "E05")

	// 跟下一季：完成 40 天以后，下一季有了播出日期就建它的追剧（只建一次）
	follow := true
	lib := models.MediaLibrary{Name: "剧集", Kind: models.MediaKindTV, Path: t.TempDir(), Enabled: true}

	require.NoError(t, e.db.Create(&lib).Error)
	_, err := e.svc.UpdateSubscription(e.ctx, sub.ID, SubscriptionInput{FollowNextSeason: &follow, LibraryID: &lib.ID})
	require.NoError(t, err)
	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).
		Updates(map[string]any{"status": models.MediaSubDone, "completed_at": e.Now().Add(-40 * 24 * time.Hour)}).Error)
	e.advance(25 * time.Hour)
	e.svc.doneDue(e.ctx, e.enable(nil))
	assert.Zero(t, e.subRow(sub.ID).NextSeasonID, "TMDB 上还没有第 5 季")
	e.tmdb.set("/3/tv/231620/season/5", `{"season_number":5,"air_date":"2027-08-01","episodes":[{"episode_number":1,"name":"第1期上","air_date":"2027-08-01"}]}`)
	e.svc.doneDue(e.ctx, e.enable(nil))
	assert.Zero(t, e.subRow(sub.ID).NextSeasonID, "一天只看一次")
	e.advance(25 * time.Hour)
	e.svc.doneDue(e.ctx, e.enable(nil))
	got = e.subRow(sub.ID)
	require.NotZero(t, got.NextSeasonID)
	next := e.subRow(got.NextSeasonID)
	assert.Equal(t, 5, next.Season)
	assert.Equal(t, models.MediaScheduleAirtime, next.ScheduleKind)
	assert.Equal(t, lib.ID, next.LibraryID)
	assert.True(t, next.FollowNextSeason)
	assert.Equal(t, 1, next.StartEpisode)
	e.advance(25 * time.Hour)
	e.svc.doneDue(e.ctx, e.enable(nil))
	var n int64
	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("tmdb_id = ? AND season = ?", 231620, 5).Count(&n).Error)
	assert.Equal(t, int64(1), n, "不重复建")
}

func TestRunsRecordedAndPruned(t *testing.T) {
	e := newEnv(t)
	sub := e.follow(func(in *SubscriptionInput) { in.LockVersion = off() })
	e.enable(nil)
	it := e.item("mteam", "901", "Natural.High.S04E01.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "", 2, 10)
	e.search.set(it)
	_, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	runs := e.runs(sub.ID)
	require.Len(t, runs, 2)
	assert.Equal(t, models.MediaRunManual, runs[0].Trigger)
	assert.Equal(t, models.MediaRunOK, runs[0].Status)
	require.NotNil(t, runs[0].FinishedAt)
	var kinds []string
	for _, st := range runs[0].Steps {
		kinds = append(kinds, st.Kind)
	}
	assert.Contains(t, kinds, stepSearch)
	assert.Contains(t, kinds, stepPush)
	assert.Equal(t, runs[0].ID, e.subRow(sub.ID).LastRunID)

	// RSS 送来已经下过的：什么都没做，不留记录；送来新的：留一条
	e.svc.considerOffer(e.ctx, Candidate{Site: "mteam", Item: it})
	assert.Len(t, e.runs(sub.ID), 2)
	e.svc.considerOffer(e.ctx, Candidate{Site: "mteam", Item: e.item("mteam", "902", "Natural.High.S04E02.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "", 2, 10)})
	runs = e.runs(sub.ID)
	require.Len(t, runs, 3)
	assert.Equal(t, models.MediaRunRSS, runs[0].Trigger)

	// 清理：超过 90 天的、每个订阅 100 条以前的
	for i := range 110 {
		require.NoError(t, e.db.Create(&models.MediaSubscriptionRun{SubscriptionID: sub.ID, Trigger: models.MediaRunSchedule, StartedAt: e.Now().Add(-time.Duration(i) * time.Hour), Status: models.MediaRunNothing}).Error)
	}
	require.NoError(t, e.db.Create(&models.MediaSubscriptionRun{SubscriptionID: sub.ID, Trigger: models.MediaRunSchedule, StartedAt: e.Now().Add(-100 * 24 * time.Hour)}).Error)
	e.svc.pruneRuns(e.ctx)
	var n int64
	require.NoError(t, e.db.Model(&models.MediaSubscriptionRun{}).Where("subscription_id = ?", sub.ID).Count(&n).Error)
	assert.Equal(t, int64(keepRuns), n)
	page, err := e.svc.Runs(e.ctx, sub.ID, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(keepRuns), page.Total)
	assert.Len(t, page.Items, 10)

	// 删掉订阅时一起删
	require.NoError(t, e.svc.DeleteSubscription(e.ctx, sub.ID))
	require.NoError(t, e.db.Model(&models.MediaSubscriptionRun{}).Count(&n).Error)
	assert.Zero(t, n)
	require.NoError(t, e.db.Model(&models.MediaSubscriptionEpisode{}).Count(&n).Error)
	assert.Zero(t, n)
}

func TestFailStreakNotice(t *testing.T) {
	e := newEnv(t)
	sub := e.follow(func(in *SubscriptionInput) { in.LockVersion = off() })
	e.enable(func(s *Settings) { s.NotifyChannels = []uint{1} })
	e.search.set(e.item("mteam", "951", "Natural.High.S04E01.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "", 2, 10))
	e.pushErr = errors.New("下载器断开")
	for range 2 {
		_, err := e.svc.SearchNow(e.ctx, sub.ID)
		require.NoError(t, err)
	}
	assert.Empty(t, e.noticesOf(NoticeFollowFailed))
	// 中间手动跳过一集：是一次改动，不打断连续失败
	_, err := e.svc.SkipEpisode(e.ctx, sub.ID, 6, true)
	require.NoError(t, err)
	assert.Equal(t, models.MediaRunChanged, e.runs(sub.ID)[0].Status)
	_, err = e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	fails := e.noticesOf(NoticeFollowFailed)
	require.Len(t, fails, 1, "连续 3 次失败发一次")
	ev := fails[0].Event
	require.NotNil(t, ev)
	assert.Equal(t, notify.EventFollowFailed, ev.Type)
	assert.Equal(t, int64(3), ev.Fields["fails"])
	assert.Contains(t, ev.Fields["error"], "下载器断开")
	_, err = e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Len(t, e.noticesOf(NoticeFollowFailed), 1, "之后接着失败不再发")
}

func TestEpisodeRules(t *testing.T) {
	e := newEnv(t)
	sub := e.follow(func(in *SubscriptionInput) { in.LockVersion = off() })
	e.enable(nil)
	rules, err := e.svc.SaveEpisodeRule(e.ctx, sub.ID, EpisodeRuleInput{Group: "XX", Mode: models.MediaEpisodeRuleOffset, Offset: 1, Note: "XX 从 0 开始编"})
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, models.MediaRuleFromManual, rules[0].Source)
	rules, err = e.svc.SaveEpisodeRule(e.ctx, sub.ID, EpisodeRuleInput{Group: "XX", Mode: models.MediaEpisodeRuleOffset, Offset: 2})
	require.NoError(t, err)
	require.Len(t, rules, 1, "同一个组替换")
	assert.Equal(t, 2, rules[0].Offset)
	for _, bad := range []EpisodeRuleInput{{Mode: "guess"}, {Mode: models.MediaEpisodeRuleOffset}, {Mode: models.MediaEpisodeRuleTMDB, Source: "robot"}} {
		_, err = e.svc.SaveEpisodeRule(e.ctx, sub.ID, bad)
		assert.ErrorIs(t, err, ErrInvalid, "%+v", bad)
	}
	e.search.set(e.item("mteam", "961", "Natural.High.S04E01.1080p.WEB-DL-XX", "", 1, 10))
	_, err = e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	linked := e.linked(sub.ID)
	require.Len(t, linked, 1)
	assert.Equal(t, 3, linked[0].Episode, "XX 的 E01 按规则是第 3 集")
	rules, err = e.svc.DeleteEpisodeRule(e.ctx, sub.ID, "XX")
	require.NoError(t, err)
	assert.Empty(t, rules)
	_, err = e.svc.DeleteEpisodeRule(e.ctx, sub.ID, "XX")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestLibraryFor(t *testing.T) {
	e := newEnv(t)
	lib := models.MediaLibrary{Name: "综艺", Kind: models.MediaKindTV, Path: t.TempDir(), Enabled: true}
	require.NoError(t, e.db.Create(&lib).Error)
	sub := e.follow(func(in *SubscriptionInput) { in.LibraryID = &lib.ID; in.LockVersion = off() })
	assert.Equal(t, lib.ID, sub.LibraryID)
	e.enable(nil)
	e.search.set(e.item("mteam", "971", "Natural.High.S04E01.2023.2160p.TX.WEB-DL.H.265.DDP2.0-HHWEB", "", 2, 10))
	_, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	hash := e.hashOf("971")
	id, found, err := e.svc.LibraryFor(e.ctx, 1, strings.ToUpper(hash))
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, lib.ID, id)
	// 种子失败以后（重试整理时）照样有效
	require.NoError(t, e.db.Model(&models.MediaSubscriptionTorrent{}).Where("info_hash = ?", hash).Update("status", models.MediaSubTorrentFailed).Error)
	id, found, err = e.svc.LibraryFor(e.ctx, 1, hash)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, lib.ID, id)
	_, found, err = e.svc.LibraryFor(e.ctx, 1, "ffff")
	require.NoError(t, err)
	assert.False(t, found, "不是订阅的种子")

	// 没指定库的追剧：found 但编号是 0（整理按类型自动选）
	sub2 := e.follow(func(in *SubscriptionInput) { in.TMDBID, in.Season = 100088, 2; in.LockVersion = off() })
	e.search.set(e.item("mteam", "972", "The.Last.of.Us.S02E01.2160p.WEB-DL.H.265-HHWEB", "", 2, 10))
	_, err = e.svc.SearchNow(e.ctx, sub2.ID)
	require.NoError(t, err)
	id, found, err = e.svc.LibraryFor(e.ctx, 1, e.hashOf("972"))
	require.NoError(t, err)
	assert.True(t, found)
	assert.Zero(t, id)
}

// 只改定时：按新的定时排下次搜索，不马上搜；一样的再存一次什么都不变；改了版本锁马上搜
func TestUpdateReschedules(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	sub := e.follow(nil)
	now := e.Now()
	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).Update("next_search_at", now.Add(48*time.Hour)).Error)
	weekly := Schedule{Kind: models.MediaScheduleWeekly, At: "9:30", Days: []int{6}}
	_, err := e.svc.UpdateSubscription(e.ctx, sub.ID, SubscriptionInput{Schedule: &weekly})
	require.NoError(t, err)
	want, ok := schedule.Next(schedule.Spec{Kind: schedule.Weekly, At: "09:30", Days: []int{6}}, now, time.Local)
	require.True(t, ok)
	got := e.subRow(sub.ID)
	require.NotNil(t, got.NextSearchAt)
	sameTime(t, want.Add(-scheduleJitter/2), *got.NextSearchAt, "只改定时：按新的定时排")
	assert.Equal(t, "09:30", got.ScheduleAt)

	runs := len(e.runs(sub.ID))
	_, err = e.svc.UpdateSubscription(e.ctx, sub.ID, SubscriptionInput{Schedule: &weekly})
	require.NoError(t, err)
	assert.Len(t, e.runs(sub.ID), runs, "没改什么：不记运行")
	sameTime(t, want.Add(-scheduleJitter/2), *e.subRow(sub.ID).NextSearchAt)

	g := "PTerWEB"
	_, err = e.svc.UpdateSubscription(e.ctx, sub.ID, SubscriptionInput{Schedule: &weekly, LockGroup: &g})
	require.NoError(t, err)
	got = e.subRow(sub.ID)
	sameTime(t, now, *got.NextSearchAt, "改了版本锁：马上搜")
	require.Len(t, e.runs(sub.ID), runs+1)
	assert.Equal(t, models.MediaRunChanged, e.runs(sub.ID)[0].Status, "手动改动标成 changed")
}

// stepsOf 是一次运行的步骤，一步一行。
func stepsOf(run RunView) string {
	var b strings.Builder
	for _, st := range run.Steps {
		b.WriteString(st.Text + "\n")
	}
	return b.String()
}

// 种子文件没下载下来：同一站点连着两次没下载到，这一轮不再从它下载（别的站点照常）；下次搜索提前到 30 分钟以后，
// 接着没全成一次间隔翻一倍；都下载成了按平常的时间。
func TestFetchFailureRetriesSooner(t *testing.T) {
	e := newEnv(t)
	sub := e.follow(func(in *SubscriptionInput) { in.LockVersion = off() })
	e.enable(nil)
	a := e.item("mteam", "961", "Natural.High.S04E01.2023.2160p.WEB-DL.H.265-HHWEB", "", 2, 90)
	b := e.item("mteam", "962", "Natural.High.S04E01.2023.2160p.WEB-DL.H.265-PTerWEB", "", 2, 85)
	c := e.item("mteam", "963", "Natural.High.S04E02.2023.2160p.WEB-DL.H.265-HHWEB", "", 2, 80)
	d := e.item("hdsky", "964", "Natural.High.S04E02.2023.2160p.WEB-DL.H.265-PTerWEB", "", 2, 75)
	delete(e.site.files, "961")
	delete(e.site.files, "962")
	e.search.set(a, b, c, d)
	start := e.Now()

	msg, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"964"}, pushedIDs(e), "M-Team 连着两次没下载到，第 2 集从别的站点下：%s", msg)
	run := e.runs(sub.ID)[0]
	assert.Equal(t, models.MediaRunPartial, run.Status)
	steps := stepsOf(run)
	assert.Contains(t, steps, "mteam 连着 2 次没下载到种子文件，这一轮先不从它下载")
	assert.Contains(t, steps, "2 个种子文件没下载下来，")
	sameTime(t, start.Add(30*time.Minute), *e.subRow(sub.ID).NextSearchAt)

	// 接着没下载下来：间隔翻倍
	_, err = e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, models.MediaRunFailed, e.runs(sub.ID)[0].Status)
	sameTime(t, start.Add(time.Hour), *e.subRow(sub.ID).NextSearchAt)

	// 站点好了：推上，按平常的时间（第 3、4 集播出以后）
	e.site.files["961"] = torrentBytes(a.Title)
	_, err = e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"964", "961"}, pushedIDs(e))
	assert.Equal(t, models.MediaRunOK, e.runs(sub.ID)[0].Status)
	next := *e.subRow(sub.ID).NextSearchAt
	assert.True(t, next.After(start.Add(90*time.Minute)), "按平常的下次时间：%s", next)
}

// 推满了一轮的上限、还有缺的集：30 分钟以后接着推。
func TestRoundCapRetriesSooner(t *testing.T) {
	old := maxPushesPerRound
	maxPushesPerRound = 1
	t.Cleanup(func() { maxPushesPerRound = old })
	e := newEnv(t)
	sub := e.follow(func(in *SubscriptionInput) { in.LockVersion = off() })
	e.enable(nil)
	e.search.set(
		e.item("mteam", "971", "Natural.High.S04E01.2023.2160p.WEB-DL.H.265-HHWEB", "", 2, 90),
		e.item("mteam", "972", "Natural.High.S04E02.2023.2160p.WEB-DL.H.265-HHWEB", "", 2, 80),
	)
	start := e.Now()
	_, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"971"}, pushedIDs(e))
	run := e.runs(sub.ID)[0]
	assert.Equal(t, models.MediaRunOK, run.Status)
	assert.Contains(t, stepsOf(run), "这一轮推满了 1 个，")
	sameTime(t, start.Add(30*time.Minute), *e.subRow(sub.ID).NextSearchAt)
}

// RSS 送来的种子的种子文件没下载下来：下次主动搜索提前到 30 分钟以后；已经更早的不动。
func TestOfferFetchFailurePullsSearch(t *testing.T) {
	e := newEnv(t)
	sub := e.follow(func(in *SubscriptionInput) { in.LockVersion = off() })
	e.enable(nil)
	setNext := func(at time.Time) {
		require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).Update("next_search_at", at).Error)
	}
	setNext(e.Now().Add(6 * time.Hour))
	it := e.item("mteam", "981", "Natural.High.S04E01.2023.2160p.WEB-DL.H.265-HHWEB", "", 2, 90)
	delete(e.site.files, "981")
	e.svc.considerOffer(e.ctx, Candidate{Site: "mteam", Item: it, From: fromRSS, SeenAt: e.Now()})
	assert.Empty(t, e.gotPushes())
	sameTime(t, e.Now().Add(30*time.Minute), *e.subRow(sub.ID).NextSearchAt)
	run := e.runs(sub.ID)[0]
	assert.Equal(t, models.MediaRunRSS, run.Trigger)
	assert.Contains(t, stepsOf(run), "1 个种子文件没下载下来，")

	soon := e.Now().Add(10 * time.Minute)
	setNext(soon)
	e.svc.considerOffer(e.ctx, Candidate{Site: "mteam", Item: it, From: fromRSS, SeenAt: e.Now()})
	sameTime(t, soon, *e.subRow(sub.ID).NextSearchAt, "已经更早的不动")
	assert.NotContains(t, stepsOf(e.runs(sub.ID)[0]), "主动搜一次")
}

// 锁住的版本在站点上有、只是种子文件没下载下来：播出很久也不算等版本（不发「等版本」提醒），按缺的接着找；
// 版本锁改了以后重新算。
func TestVersionSeenIsNotWaiting(t *testing.T) {
	e := newEnv(t)
	group, res := "HHWEB", "2160p"
	sub := e.follow(func(in *SubscriptionInput) { in.LockGroup, in.LockResolution = &group, &res })
	e.enable(func(s *Settings) { s.NotifyChannels = []uint{1} })
	e1, ok := airTime(sub, "2026-10-01")
	require.True(t, ok)
	e.setNow(e1.Add(8 * 24 * time.Hour))
	it := e.item("mteam", "991", "Natural.High.S04E01.2023.2160p.WEB-DL.H.265-HHWEB", "", 2, 90)
	delete(e.site.files, "991")
	e.search.set(it)
	e.svc.refresh(e.ctx)
	require.Equal(t, models.MediaEpisodeWaitingVersion, e.state(sub, 1))

	e.advance(time.Minute)
	_, err := e.svc.SearchNow(e.ctx, sub.ID)
	require.NoError(t, err)
	e.svc.refresh(e.ctx)
	assert.Equal(t, models.MediaEpisodeMissing, e.state(sub, 1), "找到过锁住的版本：不是等版本")
	assert.Equal(t, models.MediaEpisodeWaitingVersion, e.state(sub, 2), "第 2 集没找到过，照样等")
	for _, n := range e.noticesOf(NoticeFollowWaiting) {
		assert.NotContains(t, n.Event.Fields["episodes"], "E01")
	}

	// 版本锁改了：以前找到的不算，重新等
	other := "PTerWEB"
	_, err = e.svc.UpdateSubscription(e.ctx, sub.ID, SubscriptionInput{LockGroup: &other, LockResolution: &res})
	require.NoError(t, err)
	e.svc.refresh(e.ctx)
	assert.Equal(t, models.MediaEpisodeWaitingVersion, e.state(sub, 1))
}
