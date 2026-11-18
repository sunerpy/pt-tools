package subscribe

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/serverindex"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 首页的今天播出：同日历的今天，另加背景图、入库的版本与在媒体服务器里打开的链接、下载中的站点与进度、等版本锁住的版本；
// 明天起 6 天的列进这周还要播。
func TestHomeCalendar(t *testing.T) {
	e := newEnv(t)
	e.setNow(time.Date(2026, 10, 8, 20, 0, 0, 0, time.Local))
	e.tmdb.set("/3/tv/231620", `{"id":231620,"name":"现在就出发","original_name":"现在就出发","first_air_date":"2023-08-27","number_of_seasons":4,"poster_path":"/nh.jpg","backdrop_path":"/nh-bg.jpg"}`)
	e.tmdb.set("/3/tv/100088/season/2", `{"season_number":2,"episodes":[{"episode_number":1,"name":"未来","air_date":"2026-10-08"},
		{"episode_number":2,"name":"穿越","air_date":"2026-10-10"},{"episode_number":3,"name":"路","air_date":"2026-10-15"}]}`)
	e.tmdb.set("/3/movie/693134", `{"id":693134,"title":"沙丘2","original_title":"Dune: Part Two","release_date":"2026-10-08","imdb_id":"tt15239678","poster_path":"/dune2.jpg","backdrop_path":"/dune-bg.jpg"}`)
	nh := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 231620, Season: 4})
	tlou := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", tlou.ID).
		Updates(map[string]any{"lock_version": true, "lock_group": "HHWEB", "lock_resolution": "2160p"}).Error)

	// 第 3 集在下载（订阅的种子，下载器里 63%）；第 4 集入库了（媒体服务器里也有）；最后生还者第 1 集在等锁住的版本
	tor := models.MediaSubscriptionTorrent{SubscriptionID: nh.ID, SiteName: "hdsky", TorrentID: "9", InfoHash: "abcd", DownloaderID: 1}
	require.NoError(t, e.db.Create(&tor).Error)
	e.dl.setTorrents(downloader.Torrent{ID: "abcd", InfoHash: "ABCD", Progress: 0.634, ETA: 720, State: downloader.TorrentDownloading})
	lib := models.MediaTransferHistory{
		SourcePath: "/dl/nh/e04.mkv", TorrentName: "现在就出发.S04E04.2160p.WEB-DL.H265-HHWEB", MediaType: models.MediaKindTV, TMDBID: 231620,
		Season: 4, Episode: 4, Status: models.MediaTransferDone, TargetPath: "/lib/nh/E04.mkv",
	}
	movie := models.MediaTransferHistory{
		SourcePath: "/dl/Dune.Part.Two.2024.1080p.BluRay.x264-QA.mkv", MediaType: models.MediaKindMovie, TMDBID: 693134,
		Status: models.MediaTransferDone, TargetPath: "/lib/dune.mkv",
	}
	require.NoError(t, e.db.Create(&[]models.MediaTransferHistory{lib, movie}).Error)
	require.NoError(t, e.db.Where("subscription_id IN ?", []uint{nh.ID, tlou.ID}).Delete(&models.MediaSubscriptionEpisode{}).Error)
	require.NoError(t, e.db.Create(&[]models.MediaSubscriptionEpisode{
		{SubscriptionID: nh.ID, Episode: 3, State: models.MediaEpisodeDownloading, Source: models.MediaEpisodeFromSubscription, Ref: fmt.Sprint(tor.ID)},
		{SubscriptionID: nh.ID, Episode: 4, State: models.MediaEpisodeLibrary, Source: models.MediaEpisodeFromLibrary, Ref: "999"},
		{SubscriptionID: tlou.ID, Episode: 1, State: models.MediaEpisodeWaitingVersion},
	}).Error)
	jf := e.indexServer("jf", false, nil)
	e.indexItems(jf, episodeItem("e4", 231620, 4, 4), models.MediaServerItem{ItemID: "m1", Kind: models.MediaServerItemMovie, TMDBID: 693134})

	cal, err := e.svc.HomeCalendar(e.ctx)
	require.NoError(t, err)
	assert.Empty(t, cal.Errors)
	assert.Equal(t, int64(3), cal.Subscriptions)
	got := []string{}
	for _, it := range cal.Today {
		got = append(got, fmt.Sprintf("%s E%d %s", it.Title, it.Episode, it.State))
	}
	assert.Equal(t, []string{"最后生还者 E1 waiting_version", "沙丘2 E0 library", "现在就出发 E3 downloading", "现在就出发 E4 library"}, got)
	wait, film, dl, ep4 := cal.Today[0], cal.Today[1], cal.Today[2], cal.Today[3]
	assert.Equal(t, "HHWEB 2160p", wait.Lock)
	assert.Empty(t, wait.OpenURL)

	assert.Equal(t, "1080p QA", film.Version, "电影按最新的整理记录，没有种子名时按源文件名")
	assert.Equal(t, serverindex.OpenURL(jf, "m1"), film.OpenURL)
	assert.Equal(t, "/dune-bg.jpg", film.BackdropPath)

	assert.Equal(t, "hdsky", dl.Site)
	assert.InDelta(t, 63.4, dl.Percent, 0.01)
	assert.Equal(t, int64(720), dl.ETASeconds)
	assert.Empty(t, dl.Version)
	assert.Equal(t, "/nh-bg.jpg", dl.BackdropPath)

	assert.Equal(t, "2160p HHWEB", ep4.Version, "台账记的整理记录不在了时按这一集最新的")
	assert.Equal(t, serverindex.OpenURL(jf, "e4"), ep4.OpenURL)
	assert.NotEmpty(t, ep4.OpenURL)

	week := []string{}
	for _, en := range cal.Week {
		week = append(week, fmt.Sprintf("%s %s E%d", en.AirDate, en.Title, en.Episode))
	}
	assert.Equal(t, []string{"2026-10-10 最后生还者 E2"}, week, "明天起 6 天：10-15 的不算")

	// 认领来的种子（不是订阅自己推的）不写站点，进度照写
	require.NoError(t, e.db.Model(&tor).Update("site_name", models.MediaAdoptedSitePrefix+"qb").Error)
	cal, err = e.svc.HomeCalendar(e.ctx)
	require.NoError(t, err)
	assert.Empty(t, cal.Today[2].Site)
	assert.InDelta(t, 63.4, cal.Today[2].Percent, 0.01)

	// 算作已入库的集（开了「算作已入库」，台账记着服务器里的一项）：打开链接用它；服务器关了时没有
	// （两台服务器都有这一集时用台账记的那一台）
	require.NoError(t, e.db.Model(&models.MediaServer{}).Where("id = ?", jf.ID).Update("count_library", true).Error)
	e.indexItems(jf, episodeItem("e3", 231620, 4, 3))
	jf2 := e.indexServer("jf2", false, nil)
	e.indexItems(jf2, episodeItem("e3b", 231620, 4, 3))
	require.NoError(t, e.db.Model(&models.MediaSubscriptionEpisode{}).Where("subscription_id = ? AND episode = ?", nh.ID, 3).
		Updates(map[string]any{"state": models.MediaEpisodeLibrary, "source": models.MediaEpisodeFromServer, "ref": fmt.Sprintf("%d:e3b", jf2.ID)}).Error)
	cal, err = e.svc.HomeCalendar(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, serverindex.OpenURL(jf2, "e3b"), cal.Today[2].OpenURL)
	assert.Empty(t, cal.Today[2].Version, "整理记录里没有这一集")
	require.NoError(t, e.db.Model(&models.MediaServer{}).Where("id = ?", jf.ID).Update("enabled", false).Error)
	cal, err = e.svc.HomeCalendar(e.ctx)
	require.NoError(t, err)
	assert.Empty(t, cal.Today[2].OpenURL)
	assert.Empty(t, cal.Today[3].OpenURL)
	assert.Empty(t, cal.Today[1].OpenURL)

	// 订阅表读不到时报错
	require.NoError(t, e.db.Migrator().DropTable(&models.MediaSubscription{}))
	_, err = e.svc.HomeCalendar(e.ctx)
	require.ErrorContains(t, err, "读取订阅失败")
}
