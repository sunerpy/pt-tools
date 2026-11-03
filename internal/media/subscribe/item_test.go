package subscribe

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// tlouFull 是最后生还者的详情（详情、Extras 与季列表都从这个地址取）：两季、16 位演员、导演一位。
func tlouFull() string {
	var cast []string
	for i := 1; i <= 16; i++ {
		cast = append(cast, fmt.Sprintf(`{"name":"演员%d","character":"角色%d","order":%d,"profile_path":"/a%d.jpg"}`, i, i, i, i))
	}
	return `{"id":100088,"name":"最后生还者","original_name":"The Last of Us","first_air_date":"2023-01-15","number_of_seasons":2,
		"poster_path":"/tlou.jpg","backdrop_path":"/tlou-b.jpg","vote_average":8.6,"vote_count":1204,"status":"Returning Series",
		"origin_country":["US"],"episode_run_time":[55],"genres":[{"id":18,"name":"剧情"},{"id":10759,"name":"动作冒险"}],
		"networks":[{"name":"HBO"}],"external_ids":{"imdb_id":"tt3581920","tvdb_id":392256},
		"seasons":[{"season_number":0,"name":"特别篇","episode_count":2},{"season_number":1,"name":"第 1 季","episode_count":9,"air_date":"2023-01-15","poster_path":"/s1.jpg"},
			{"season_number":2,"name":"第 2 季","episode_count":3,"air_date":"2025-04-13","poster_path":"/s2.jpg"}],
		"credits":{"cast":[` + strings.Join(cast, ",") + `],"crew":[{"name":"克雷格·麦辛","job":"Director","department":"Directing"}]}}`
}

func (e *env) library(name, kind string) models.MediaLibrary {
	e.t.Helper()
	lib := models.MediaLibrary{Name: name, Kind: kind, Path: "/lib/" + name, Enabled: true}
	require.NoError(e.t, e.db.Create(&lib).Error)
	return lib
}

func ptrUint(v uint) *uint { return &v }

// 有订阅的剧集：资料、各季、库里与做种的情况、订阅、关联的种子（合并三种来源、不在下载器里的不列）与推荐。
func TestItemTVWithSubscription(t *testing.T) {
	e := newEnv(t)
	e.tmdb.set("/3/tv/100088", tlouFull())
	e.tmdb.set("/3/tv/100088/recommendations", `{"results":[{"id":231620,"name":"现在就出发","first_air_date":"2023-08-27"},{"id":5,"name":"别的剧"}]}`)
	lib := e.library("剧集库", models.MediaKindTV)
	sub := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).Updates(map[string]any{
		"schedule_kind": models.MediaScheduleAirtime, "lock_version": true, "lock_group": "HHWEB", "lock_resolution": "1080p",
	}).Error)
	require.NoError(t, e.db.Create(&models.MediaSubscription{MediaType: models.MediaKindTV, TMDBID: 231620, Season: 4, Status: models.MediaSubActive}).Error)
	require.NoError(t, e.db.Create(&models.MediaTransferHistory{
		SourcePath: "/dl/other", MediaType: models.MediaKindTV, TMDBID: 5, Season: 1, Episode: 1, Status: models.MediaTransferDone,
	}).Error)

	// 第 2 季第 1 集入库了（整理记录带着下载器与 hash h1），推送记录也有它（H&R 72 小时）
	require.NoError(t, e.db.Create(&models.MediaTransferHistory{
		SourcePath: "/dl/h1/e01.mkv", TargetPath: "/lib/剧集库/tlou/S02E01.mkv", MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Episode: 1,
		Status: models.MediaTransferDone, InfoHash: "H1", DownloaderID: 1, DownloaderName: "qb", LibraryID: lib.ID, Size: 3 << 30,
		TorrentName: "The.Last.of.Us.S02E01.1080p.WEB-DL.H264-HHWEB",
	}).Error)
	pushed := true
	h1, h3 := "h1", "h3"
	require.NoError(t, e.db.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "11", TorrentHash: &h1, DownloaderID: ptrUint(1), Title: "The.Last.of.Us.S02E01.1080p.WEB-DL.H264-HHWEB",
		IsPushed: &pushed, IsCompleted: true, HasHR: true, HRSeedTimeH: 72, MediaType: models.MediaKindTV, TMDBID: 100088, MediaSeason: 2,
	}).Error)
	// 第 1 季的整季包：H&R 72 小时，做种才 1 天
	require.NoError(t, e.db.Create(&models.TorrentInfo{
		SiteName: "ourbits", TorrentID: "31", TorrentHash: &h3, DownloaderID: ptrUint(1), Title: "The.Last.of.Us.S01.1080p.WEB-DL.H264-YYWEB",
		IsPushed: &pushed, IsCompleted: true, HasHR: true, HRSeedTimeH: 72, TorrentSize: 30 << 30, MediaType: models.MediaKindTV, TMDBID: 100088, MediaSeason: 1,
	}).Error)
	// 订阅的种子：第 2 集在下载；还有一个已经不在下载器里的
	require.NoError(t, e.db.Create(&models.MediaSubscriptionTorrent{
		SubscriptionID: sub.ID, SiteName: "hdsky", TorrentID: "12", InfoHash: "h2", DownloaderID: 1, Title: "The.Last.of.Us.S02E02.1080p.WEB-DL.H264-HHWEB",
		Episode: 2, SizeBytes: 3 << 30, Status: models.MediaSubTorrentDownloading,
	}).Error)
	require.NoError(t, e.db.Create(&models.MediaSubscriptionTorrent{
		SubscriptionID: sub.ID, SiteName: "hdsky", TorrentID: "9", InfoHash: "h9", DownloaderID: 1, Title: "The.Last.of.Us.S02E01.720p-OLD",
		Episode: 1, Status: models.MediaSubTorrentReplaced,
	}).Error)
	e.dl.setTorrents(
		downloader.Torrent{InfoHash: "h1", Progress: 1, IsCompleted: true, Ratio: 1.31, SeedingTime: 18 * 86400, State: downloader.TorrentSeeding, TotalSize: 3 << 30},
		downloader.Torrent{InfoHash: "h2", Progress: 0.5, ETA: 720, State: downloader.TorrentDownloading, TotalSize: 3 << 30},
		downloader.Torrent{InfoHash: "H3", Progress: 1, IsCompleted: true, Ratio: 0.2, SeedingTime: 86400, State: downloader.TorrentSeeding},
	)

	d, err := e.svc.Item(e.ctx, models.MediaKindTV, 100088)
	require.NoError(t, err)
	assert.Empty(t, d.Errors)
	it := d.Item
	assert.Equal(t, "最后生还者", it.Title)
	assert.Equal(t, "The Last of Us", it.OriginalTitle)
	assert.Equal(t, 2023, it.Year)
	assert.Equal(t, []string{"剧情", "动作冒险"}, it.Genres)
	assert.Equal(t, []string{"US"}, it.Countries)
	assert.Equal(t, 55, it.Runtime)
	assert.Equal(t, "Returning Series", it.Status)
	assert.Equal(t, "/tlou-b.jpg", it.BackdropPath)
	assert.InDelta(t, 8.6, it.Rating, 0.001)
	assert.Equal(t, 1204, it.Votes)
	assert.Equal(t, "tt3581920", it.IMDbID)
	assert.Equal(t, 392256, it.TVDBID)
	assert.Equal(t, []string{"HBO"}, it.Studios)
	assert.Equal(t, []string{"克雷格·麦辛"}, it.Directors)
	require.Len(t, it.Cast, 15, "最多 15 位")
	assert.Equal(t, tmdb.CastMember{Name: "演员1", Character: "角色1", Order: 1, ProfilePath: "/a1.jpg"}, it.Cast[0])
	assert.Equal(t, 2, it.SeasonsCount)
	assert.Equal(t, 12, it.EpisodesCount, "特别篇不算")

	require.Len(t, d.Seasons, 3)
	assert.Equal(t, ItemSeason{Number: 1, Name: "第 1 季", EpisodeCount: 9, AirDate: "2023-01-15", PosterPath: "/s1.jpg"}, d.Seasons[0])
	assert.Equal(t, ItemSeason{Number: 2, Name: "第 2 季", EpisodeCount: 3, AirDate: "2025-04-13", PosterPath: "/s2.jpg", Have: 1}, d.Seasons[1])
	assert.Equal(t, 0, d.Seasons[2].Number, "特别篇排在最后")

	require.NotZero(t, d.NAS.Library.RecordID)
	assert.Equal(t, ItemLibrary{
		Seasons: []ItemSeasonHave{{Number: 2, Have: 1, Total: 3}}, Files: 1, Size: 3 << 30, Libraries: []string{"剧集库"}, RecordID: d.NAS.Library.RecordID,
	}, d.NAS.Library)
	assert.Equal(t, []ItemSubscription{{ID: sub.ID, Status: models.MediaSubActive, Season: 2, Follow: true, Lock: &MediaSearchLock{Group: "HHWEB", Resolution: "1080p"}}}, d.Subscriptions)

	require.Len(t, d.Torrents, 3, "不在下载器里的不列")
	pack, e01, e02 := d.Torrents[0], d.Torrents[1], d.Torrents[2]
	assert.Equal(t, ItemTorrent{
		Site: "ourbits", TorrentID: "31", Title: "The.Last.of.Us.S01.1080p.WEB-DL.H264-YYWEB", Season: 1, Episodes: "整季", Version: "1080p YYWEB",
		Size: 30 << 30, DownloaderID: 1, Downloader: "qb", Hash: "h3", State: ItemTorrentSeeding, Percent: 100, Ratio: 0.2, SeedingSeconds: 86400,
		HR: true, HRHours: 72, Reason: "H&R 还差 2 天",
	}, pack)
	assert.Equal(t, "hdsky", e01.Site, "推送记录给站点与编号")
	assert.Equal(t, "11", e01.TorrentID)
	assert.Equal(t, "E01", e01.Episodes)
	assert.Equal(t, int64(3<<30), e01.Size, "整理记录给体积")
	assert.True(t, e01.Deletable)
	assert.Equal(t, "H&R 已满足", e01.Reason)
	assert.Equal(t, "1080p HHWEB", e01.Version)
	assert.Equal(t, ItemTorrentDownloading, e02.State)
	assert.InDelta(t, 50, e02.Percent, 0.01)
	assert.Equal(t, int64(720), e02.ETASeconds)
	assert.Equal(t, "E02", e02.Episodes)
	assert.False(t, e02.Deletable)
	assert.Equal(t, "受保护：下载中", e02.Reason)

	require.NotNil(t, d.NAS.Seeding.LowestRatio)
	assert.InDelta(t, 0.2, *d.NAS.Seeding.LowestRatio, 0.001)
	assert.Equal(t, ItemSeeding{Count: 2, HRPending: 1, HRLeftSeconds: 48 * 3600, LowestRatio: d.NAS.Seeding.LowestRatio}, d.NAS.Seeding)

	require.Len(t, d.Recommendations, 2)
	assert.Equal(t, 231620, d.Recommendations[0].ID)
	assert.True(t, d.Recommendations[0].Subscribed)
	assert.False(t, d.Recommendations[0].InLibrary)
	assert.True(t, d.Recommendations[1].InLibrary)
	assert.False(t, d.Recommendations[1].Subscribed)

	// 只读：订阅、运行记录、台账都不变
	var runs int64
	require.NoError(t, e.db.Model(&models.MediaSubscriptionRun{}).Where("subscription_id = ?", sub.ID).Count(&runs).Error)
	before := runs
	_, err = e.svc.Item(e.ctx, models.MediaKindTV, 100088)
	require.NoError(t, err)
	require.NoError(t, e.db.Model(&models.MediaSubscriptionRun{}).Where("subscription_id = ?", sub.ID).Count(&runs).Error)
	assert.Equal(t, before, runs)
}

// 有库没订阅的电影：版本写最新入库的那个文件；推荐没取到时写进 errors，其余照给。
// 刚推送的种子：台账一分钟里用缓存的下载器列表，条目详情只用 10 秒以内的，推完再打开详情就看得到。
func TestItemSeesFreshTorrents(t *testing.T) {
	e := newEnv(t)
	e.tmdb.set("/3/tv/100088", tlouFull())
	e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	// 推送以前台账列过一次下载器（那时还是空的）
	_, err := e.svc.downloaderTorrents(e.ctx, 1)
	require.NoError(t, err)
	lists := e.dl.listCount()

	// 刚推送就打开详情（没过 10 秒）：推送记录比那份列表晚记下，不用它
	pushTLOU(t, e, "h5", 3, time.Time{})
	e.dl.setTorrents(downloader.Torrent{InfoHash: "h5", Progress: 0.2, ETA: 600, State: downloader.TorrentDownloading, TotalSize: 3 << 30})
	d, err := e.svc.Item(e.ctx, models.MediaKindTV, 100088)
	require.NoError(t, err)
	require.Len(t, d.Torrents, 1, "刚推送的种子要列出来")
	assert.Equal(t, ItemTorrentDownloading, d.Torrents[0].State)
	assert.InDelta(t, 20, d.Torrents[0].Percent, 0.01)
	assert.Equal(t, lists+1, e.dl.listCount())

	// 一季的每一集：记录都比刚列的那份早，10 秒以内用同一份；在下载的那一集带上进度
	sd, err := e.svc.SeasonItem(e.ctx, 100088, 2)
	require.NoError(t, err)
	assert.Equal(t, lists+1, e.dl.listCount())
	assert.InDelta(t, 20, episodeOf(t, sd, 3).Percent, 0.01)

	// 进度不用 10 秒以前的
	e.dl.setTorrents(downloader.Torrent{InfoHash: "h5", Progress: 0.5, State: downloader.TorrentDownloading, TotalSize: 3 << 30})
	e.advance(11 * time.Second)
	sd, err = e.svc.SeasonItem(e.ctx, 100088, 2)
	require.NoError(t, err)
	assert.Equal(t, lists+2, e.dl.listCount(), "过了 10 秒再列一次")
	assert.InDelta(t, 50, episodeOf(t, sd, 3).Percent, 0.01)

	// 一季刚列过（10 秒以内）又推送了第 2 集：记录比那份列表晚，再列一次，第 2 集带上进度
	pushTLOU(t, e, "h7", 2, time.Time{})
	e.dl.setTorrents(
		downloader.Torrent{InfoHash: "h5", Progress: 0.5, State: downloader.TorrentDownloading, TotalSize: 3 << 30},
		downloader.Torrent{InfoHash: "h7", Progress: 0.3, State: downloader.TorrentDownloading, TotalSize: 3 << 30},
	)
	sd, err = e.svc.SeasonItem(e.ctx, 100088, 2)
	require.NoError(t, err)
	assert.Equal(t, lists+3, e.dl.listCount())
	assert.InDelta(t, 30, episodeOf(t, sd, 2).Percent, 0.01)

	// 台账照旧用一分钟的缓存，不看记录的时间
	_, err = e.svc.downloaderTorrents(e.ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, lists+3, e.dl.listCount())

	// 记录的时间比现在还晚（改过系统时间）也算刚记下的：列一次，下载器里没有就不再列
	pushTLOU(t, e, "h6", 3, time.Now().Add(time.Hour))
	_, err = e.svc.Item(e.ctx, models.MediaKindTV, 100088)
	require.NoError(t, err)
	assert.Equal(t, lists+4, e.dl.listCount())
}

// 早就记下、下载器里已经没有了的种子不再列（不然每次打开详情都要列一遍）；下载器读不到时 10 秒以内也不再列。
func TestItemOldRecordGone(t *testing.T) {
	e := newEnv(t)
	e.tmdb.set("/3/tv/100088", tlouFull())
	e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	pushTLOU(t, e, "gone", 3, time.Now().Add(-time.Hour))
	d, err := e.svc.Item(e.ctx, models.MediaKindTV, 100088)
	require.NoError(t, err)
	assert.Empty(t, d.Torrents)
	_, err = e.svc.Item(e.ctx, models.MediaKindTV, 100088)
	require.NoError(t, err)
	assert.Equal(t, 1, e.dl.listCount(), "10 秒以内用同一份")

	// 下载器读不到：刚记下的种子也不在 10 秒以内再列
	e.advance(11 * time.Second)
	e.dl.mu.Lock()
	e.dl.listErr = errors.New("连接被拒绝")
	e.dl.mu.Unlock()
	pushTLOU(t, e, "h5", 2, time.Time{})
	d, err = e.svc.Item(e.ctx, models.MediaKindTV, 100088)
	require.NoError(t, err)
	require.NotEmpty(t, d.Errors)
	_, err = e.svc.Item(e.ctx, models.MediaKindTV, 100088)
	require.NoError(t, err)
	assert.Equal(t, 2, e.dl.listCount())
}

// 关联记录（订阅的种子、先记下后推送的种子记录、整理记录）比缓存的列表晚记下时，详情与一季的每一集都再列一次；
// 同一个种子有几条记录时按最晚的那条算。
func TestItemListNewerThanRecords(t *testing.T) {
	old := time.Now().Add(-time.Hour)
	const title = "The.Last.of.Us.S02E03.1080p.WEB-DL.H264-HHWEB"
	subTorrent := func(e *env, sub *models.MediaSubscription, created time.Time) {
		require.NoError(t, e.db.Create(&models.MediaSubscriptionTorrent{
			SubscriptionID: sub.ID, SiteName: "hdsky", TorrentID: "15", InfoHash: "h5", DownloaderID: 1, Title: title,
			Episode: 3, SizeBytes: 3 << 30, Status: models.MediaSubTorrentDownloading, CreatedAt: created,
		}).Error)
	}
	cases := []struct {
		name   string
		season bool // 一季的每一集里第 3 集在下载（先问一季，再问详情）
		record func(e *env, sub *models.MediaSubscription)
	}{
		{"订阅的种子", true, func(e *env, sub *models.MediaSubscription) { subTorrent(e, sub, time.Time{}) }},
		{"订阅的种子（先问详情）", false, func(e *env, sub *models.MediaSubscription) { subTorrent(e, sub, time.Time{}) }},
		{"RSS 先记下、刚推送的种子记录", true, func(e *env, _ *models.MediaSubscription) {
			pushed, h, at := true, "h5", time.Now()
			require.NoError(t, e.db.Create(&models.TorrentInfo{
				SiteName: "hdsky", TorrentID: "15", TorrentHash: &h, DownloaderID: ptrUint(1), Title: title,
				IsPushed: &pushed, PushTime: &at, MediaType: models.MediaKindTV, TMDBID: 100088, MediaSeason: 2, CreatedAt: old,
			}).Error)
		}},
		{"整理记录（下载器里别处加的种子）", false, func(e *env, _ *models.MediaSubscription) {
			require.NoError(t, e.db.Create(&models.MediaTransferHistory{
				SourcePath: "/dl/h5/e03.mkv", MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Episode: 3, Status: models.MediaTransferDone,
				Size: 3 << 30, InfoHash: "h5", DownloaderID: 1, TorrentName: title,
			}).Error)
		}},
		{"订阅的种子早就记下、同一个种子的推送记录刚记下", false, func(e *env, sub *models.MediaSubscription) {
			subTorrent(e, sub, old)
			pushTLOU(t, e, "h5", 3, time.Time{})
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			e.tmdb.set("/3/tv/100088", tlouFull())
			sub := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
			_, err := e.svc.downloaderTorrents(e.ctx, 1)
			require.NoError(t, err)
			c.record(e, sub)
			e.dl.setTorrents(downloader.Torrent{InfoHash: "h5", Progress: 0.4, State: downloader.TorrentDownloading, TotalSize: 3 << 30})
			if c.season {
				sd, serr := e.svc.SeasonItem(e.ctx, 100088, 2)
				require.NoError(t, serr)
				assert.InDelta(t, 40, episodeOf(t, sd, 3).Percent, 0.01)
				assert.Equal(t, 2, e.dl.listCount())
			}
			d, err := e.svc.Item(e.ctx, models.MediaKindTV, 100088)
			require.NoError(t, err)
			require.Len(t, d.Torrents, 1)
			assert.Equal(t, ItemTorrentDownloading, d.Torrents[0].State)
			assert.Equal(t, 2, e.dl.listCount(), "一季刚列过的那份比记录晚，详情接着用")
		})
	}
}

// 详情合进了一次推送以前就开始列的列表：那份里没有刚推送的种子，再列一次。
func TestItemDoesNotTakeAnEarlierList(t *testing.T) {
	e := newEnv(t)
	e.tmdb.set("/3/tv/100088", tlouFull())
	e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	release := make(chan struct{})
	e.dl.listHook = func(n int) {
		if n == 1 {
			<-release
		}
	}
	first := make(chan []downloader.Torrent, 1)
	go func() {
		list, _ := e.svc.downloaderTorrents(e.ctx, 1)
		first <- list
	}()
	require.Eventually(t, func() bool { return e.dl.listCount() == 1 }, 5*time.Second, time.Millisecond, "台账开始列（还是空的）")

	pushTLOU(t, e, "h5", 3, time.Time{})
	e.dl.setTorrents(downloader.Torrent{InfoHash: "h5", Progress: 0.2, State: downloader.TorrentDownloading, TotalSize: 3 << 30})
	got := make(chan *ItemDetail, 1)
	go func() {
		d, ierr := e.svc.Item(e.ctx, models.MediaKindTV, 100088)
		assert.NoError(t, ierr)
		got <- d
	}()
	// 详情等在台账那一次上（两个协程都在 singleflight 里）再放行
	require.Eventually(t, func() bool { return inSingleflight() >= 2 }, 5*time.Second, time.Millisecond)
	close(release)

	assert.Empty(t, <-first, "台账先开始列的那份里没有")
	d := <-got
	require.NotNil(t, d)
	require.Len(t, d.Torrents, 1, "合进去的那份比推送记录早：再列一次")
	assert.Equal(t, "h5", d.Torrents[0].Hash)
	assert.Equal(t, 2, e.dl.listCount())
}

// pushTLOU 记一条推送到下载器 1 的种子记录（第 2 季第 ep 集）；created 是零值时按现在记。
func pushTLOU(t *testing.T, e *env, hash string, ep int, created time.Time) {
	t.Helper()
	pushed, h := true, hash
	require.NoError(t, e.db.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: hash, TorrentHash: &h, DownloaderID: ptrUint(1), Title: fmt.Sprintf("The.Last.of.Us.S02E%02d.1080p.WEB-DL.H264-HHWEB", ep),
		IsPushed: &pushed, MediaType: models.MediaKindTV, TMDBID: 100088, MediaSeason: 2, CreatedAt: created,
	}).Error)
}

func episodeOf(t *testing.T, sd *SeasonDetail, n int) SeasonEpisode {
	t.Helper()
	for _, ep := range sd.Episodes {
		if ep.Episode == n {
			return ep
		}
	}
	require.Failf(t, "没有这一集", "第 %d 集", n)
	return SeasonEpisode{}
}

// inSingleflight 是现在在 singleflight.Group.Do 里的协程数（在列的那一个与等它的）。
func inSingleflight() int {
	buf := make([]byte, 1<<20)
	buf = buf[:runtime.Stack(buf, true)]
	n := 0
	for g := range strings.SplitSeq(string(buf), "\n\n") {
		if strings.Contains(g, "singleflight.(*Group).Do(") {
			n++
		}
	}
	return n
}

func TestItemMovieInLibrary(t *testing.T) {
	e := newEnv(t)
	lib := e.library("电影库", models.MediaKindMovie)
	require.NoError(t, e.db.Create(&models.MediaTransferHistory{
		SourcePath: "/dl/m0/old.mkv", MediaType: models.MediaKindMovie, TMDBID: 693134, Status: models.MediaTransferDone, LibraryID: lib.ID, Size: 10 << 30,
		TorrentName: "Dune.Part.Two.2024.1080p.WEB-DL-OLD",
	}).Error)
	newest := models.MediaTransferHistory{
		SourcePath: "/dl/m1/Dune.mkv", MediaType: models.MediaKindMovie, TMDBID: 693134, Status: models.MediaTransferDone, LibraryID: lib.ID, Size: 60 << 30,
		InfoHash: "m1", DownloaderID: 1, TorrentName: "Dune.Part.Two.2024.2160p.UHD.BluRay.x265-FRDS",
	}
	require.NoError(t, e.db.Create(&newest).Error)
	e.dl.setTorrents(downloader.Torrent{InfoHash: "m1", Progress: 1, IsCompleted: true, Ratio: 2, SeedingTime: 3600, State: downloader.TorrentPaused})

	d, err := e.svc.Item(e.ctx, models.MediaKindMovie, 693134)
	require.NoError(t, err)
	assert.Equal(t, "沙丘2", d.Item.Title)
	assert.Equal(t, "tt15239678", d.Item.IMDbID)
	assert.Empty(t, d.Seasons, "电影没有季")
	assert.Empty(t, d.Subscriptions)
	assert.Equal(t, "2160p FRDS", d.NAS.Library.Version, "最新入库的那个")
	assert.Equal(t, newest.ID, d.NAS.Library.RecordID, "重新刮削用最新的一条")
	assert.Equal(t, 2, d.NAS.Library.Files)
	assert.Equal(t, int64(70<<30), d.NAS.Library.Size)
	assert.Equal(t, []string{"电影库"}, d.NAS.Library.Libraries)
	require.Len(t, d.Torrents, 1)
	assert.Equal(t, ItemTorrentPaused, d.Torrents[0].State)
	assert.True(t, d.Torrents[0].Deletable)
	assert.Equal(t, "可以删", d.Torrents[0].Reason)
	assert.Equal(t, ItemSeeding{}, d.NAS.Seeding, "暂停的不算在做种")
	assert.Empty(t, d.Recommendations)
	require.Len(t, d.Errors, 1)
	assert.Contains(t, d.Errors[0], "推荐没取到")
}

// 没订阅、库里没有的剧集：TMDB 没有季列表时按季数列出；下载器读不到时种子的状态是 unknown；条目不存在时 404。
func TestItemEmptyAndErrors(t *testing.T) {
	e := newEnv(t)
	d, err := e.svc.Item(e.ctx, models.MediaKindTV, 231620)
	require.NoError(t, err)
	assert.Equal(t, "现在就出发", d.Item.Title)
	require.Len(t, d.Seasons, 4, "没有季列表：按详情里的 4 季列出")
	assert.Equal(t, ItemSeason{Number: 1}, d.Seasons[0])
	assert.Empty(t, d.NAS.Library.Seasons)
	assert.Equal(t, 0, d.NAS.Library.Files)
	assert.Empty(t, d.Torrents)
	assert.Empty(t, d.Item.Cast)
	assert.NotNil(t, d.Item.Cast, "空的也是数组")

	pushed := true
	h := "hx"
	require.NoError(t, e.db.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "77", TorrentHash: &h, DownloaderID: ptrUint(1), Title: "Natural.High.S04E01.1080p-HHWEB", IsPushed: &pushed,
		MediaType: models.MediaKindTV, TMDBID: 231620, MediaSeason: 4,
	}).Error)
	e.dl.listErr = errors.New("连不上")
	d, err = e.svc.Item(e.ctx, models.MediaKindTV, 231620)
	require.NoError(t, err)
	require.Len(t, d.Torrents, 1)
	assert.Equal(t, ItemTorrentUnknown, d.Torrents[0].State)
	assert.False(t, d.Torrents[0].Deletable)
	assert.Contains(t, strings.Join(d.Errors, "；"), "下载器「qb」没读到")

	_, err = e.svc.Item(e.ctx, models.MediaKindTV, 424242)
	require.ErrorIs(t, err, tmdb.ErrNotFound)
	_, err = e.svc.Item(e.ctx, "person", 1)
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.Item(e.ctx, models.MediaKindMovie, 0)
	require.ErrorIs(t, err, ErrInvalid)
}

// 订阅了的一季：按播出时间追剧时带播出时刻与开始搜的延迟；入库的带版本与体积，下载中的带进度。
func TestSeasonItemSubscribed(t *testing.T) {
	e := newEnv(t)
	e.tmdb.set("/3/tv/100088/season/2", `{"season_number":2,"name":"第 2 季","air_date":"2025-04-13","episodes":[
		{"episode_number":1,"name":"未来","air_date":"2025-04-13","still_path":"/e1.jpg","runtime":58},
		{"episode_number":2,"name":"穿越","air_date":"2025-04-20"},{"episode_number":3,"name":"路","air_date":"2026-12-01"}]}`)
	sub := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).Updates(map[string]any{
		"schedule_kind": models.MediaScheduleAirtime, "air_delay_hours": 5, "schedule_at": "21:30",
	}).Error)
	require.NoError(t, e.db.Create(&models.MediaTransferHistory{
		SourcePath: "/dl/h1/e01.mkv", MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Episode: 1, Status: models.MediaTransferDone,
		Size: 3 << 30, TorrentName: "The.Last.of.Us.S02E01.1080p.WEB-DL.H264-HHWEB",
	}).Error)
	require.NoError(t, e.db.Create(&models.MediaSubscriptionTorrent{
		SubscriptionID: sub.ID, SiteName: "hdsky", TorrentID: "12", InfoHash: "H2", DownloaderID: 1, Title: "The.Last.of.Us.S02E02.1080p-HHWEB",
		Episode: 2, Status: models.MediaSubTorrentDownloading,
	}).Error)
	e.dl.setTorrents(downloader.Torrent{InfoHash: "h2", Progress: 0.25, ETA: 600, State: downloader.TorrentDownloading})

	d, err := e.svc.SeasonItem(e.ctx, 100088, 2)
	require.NoError(t, err)
	assert.Equal(t, "第 2 季", d.Name)
	assert.Equal(t, &SeasonSubscription{ID: sub.ID, Status: models.MediaSubActive, Follow: true, Airtime: true, SearchDelayHours: 5}, d.Subscription)
	require.Len(t, d.Episodes, 3)
	at1 := time.Date(2025, 4, 13, 21, 30, 0, 0, time.Local)
	assert.Equal(t, SeasonEpisode{
		Episode: 1, Name: "未来", AirDate: "2025-04-13", AirAt: &at1, StillPath: "/e1.jpg", Runtime: 58, State: models.MediaEpisodeLibrary,
		Version: "1080p HHWEB", Size: 3 << 30,
	}, d.Episodes[0])
	assert.Equal(t, models.MediaEpisodeDownloading, d.Episodes[1].State)
	assert.Equal(t, "hdsky", d.Episodes[1].Site)
	assert.InDelta(t, 25, d.Episodes[1].Percent, 0.01)
	assert.Equal(t, int64(600), d.Episodes[1].ETASeconds)
	assert.Equal(t, models.MediaEpisodeUpcoming, d.Episodes[2].State)
	require.NotNil(t, d.Episodes[2].AirAt)
	assert.Equal(t, time.Date(2026, 12, 1, 21, 30, 0, 0, time.Local), *d.Episodes[2].AirAt)
	assert.Empty(t, d.Errors)

	// 延迟没设（0）是播出后 2 小时；不是按播出时间的追剧不带播出时刻
	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).Update("air_delay_hours", 0).Error)
	d, err = e.svc.SeasonItem(e.ctx, 100088, 2)
	require.NoError(t, err)
	assert.Equal(t, 2, d.Subscription.SearchDelayHours)
	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).Update("schedule_kind", models.MediaScheduleDaily).Error)
	d, err = e.svc.SeasonItem(e.ctx, 100088, 2)
	require.NoError(t, err)
	assert.False(t, d.Subscription.Airtime)
	assert.Zero(t, d.Subscription.SearchDelayHours)
	assert.Nil(t, d.Episodes[0].AirAt)
}

// 没订阅的一季：按 TMDB 的播出日期分成缺的与还没播的；季号不对、TMDB 上没有这一季时报错。
func TestSeasonItemNotSubscribed(t *testing.T) {
	e := newEnv(t)
	d, err := e.svc.SeasonItem(e.ctx, 231620, 4)
	require.NoError(t, err)
	assert.Nil(t, d.Subscription)
	require.Len(t, d.Episodes, 6)
	var states []string
	for _, ep := range d.Episodes {
		states = append(states, ep.State)
	}
	assert.Equal(t, []string{"missing", "missing", "missing", "missing", "upcoming", "upcoming"}, states)

	_, err = e.svc.SeasonItem(e.ctx, 231620, 9)
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "没有第 9 季")
	_, err = e.svc.SeasonItem(e.ctx, 231620, -1)
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.SeasonItem(e.ctx, 0, 1)
	require.ErrorIs(t, err, ErrInvalid)
}

func TestLeftTextAndSpanLabel(t *testing.T) {
	assert.Equal(t, "1 小时", leftText(60))
	assert.Equal(t, "23 小时", leftText(23*3600))
	assert.Equal(t, "1 天", leftText(25*3600))
	assert.Equal(t, "2 天", leftText(36*3600))
	assert.Equal(t, "", spanLabel(nil, false))
	assert.Equal(t, "整季", spanLabel([]int{1}, true))
	assert.Equal(t, "E01–E04、E07", spanLabel([]int{4, 2, 1, 3, 7, 7}, false))
	assert.Equal(t, "E03", spanLabel([]int{3}, false))
}
