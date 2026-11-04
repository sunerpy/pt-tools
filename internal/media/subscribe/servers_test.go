package subscribe

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// indexServer 建一台同步过的媒体服务器；count 是「算作已入库」。
func (e *env) indexServer(name string, count bool, mod func(*models.MediaServer)) models.MediaServer {
	e.t.Helper()
	at := e.Now().Add(-time.Hour)
	srv := models.MediaServer{
		Name: name, Kind: models.MediaServerJellyfin, URL: "http://" + name + ":8096", Enabled: true, IndexedAt: &at, ServerUID: "uid-" + name,
		CountLibrary: count,
	}
	if mod != nil {
		mod(&srv)
	}
	require.NoError(e.t, e.db.Create(&srv).Error)
	return srv
}

// indexItems 往服务器的索引里放几项。
func (e *env) indexItems(srv models.MediaServer, rows ...models.MediaServerItem) {
	e.t.Helper()
	for i := range rows {
		rows[i].ServerID, rows[i].SyncedAt = srv.ID, e.Now()
	}
	require.NoError(e.t, e.db.Create(&rows).Error)
}

func episodeItem(id string, tmdbID, season, episode int) models.MediaServerItem {
	return models.MediaServerItem{ItemID: id, Kind: models.MediaServerItemEpisode, SeriesItemID: "show", TMDBID: tmdbID, Season: season, Episode: episode}
}

// 追剧：开了「算作已入库」的服务器里有的集算已入库（来源 server），不再算缺、不认领下载器里的种子；没开的服务器、删了的项不算。
func TestLedgerCountsServerEpisodes(t *testing.T) {
	e := newEnv(t)
	sub := e.follow(func(in *SubscriptionInput) { in.LockVersion = off() })
	e.organized("h1", models.MediaKindTV, 231620, 4, 1, "/lib/nh/E01.mkv")
	counting := e.indexServer("jf", true, nil)
	other := e.indexServer("emby", false, nil)
	gone := e.Now().Add(-time.Minute)
	e.indexItems(counting, episodeItem("e2", 231620, 4, 2), episodeItem("e1", 231620, 4, 1),
		models.MediaServerItem{ItemID: "e4", Kind: models.MediaServerItemEpisode, TMDBID: 231620, Season: 4, Episode: 4, RemovedAt: &gone})
	e.indexItems(other, episodeItem("x3", 231620, 4, 3))

	lg := e.ledger(sub)
	assert.Equal(t, []int{3, 4}, lg.missing, "第 2 集在服务器里；第 3 集只在没开选项的服务器里；第 4 集删了")
	r1, r2 := lg.row(1), lg.row(2)
	assert.Equal(t, [2]string{models.MediaEpisodeLibrary, models.MediaEpisodeFromLibrary}, [2]string{r1.State, r1.Source}, "整理记录优先")
	assert.Equal(t, [3]string{models.MediaEpisodeLibrary, models.MediaEpisodeFromServer, "1:e2"}, [3]string{r2.State, r2.Source, r2.Ref})

	// 关掉选项（或服务器关了）：又算缺
	require.NoError(t, e.db.Model(&models.MediaServer{}).Where("id = ?", counting.ID).Update("count_library", false).Error)
	lg = e.ledger(sub)
	assert.Equal(t, []int{2, 3, 4}, lg.missing)

	// 读不到索引时当作没有，不挡台账
	require.NoError(t, e.db.Migrator().DropTable(&models.MediaServerItem{}))
	lg = e.ledger(sub)
	assert.Equal(t, []int{2, 3, 4}, lg.missing)
}

// 服务器里有的集不认领下载器里的种子（不当成正在下）。
func TestLedgerServerEpisodesNotAdopted(t *testing.T) {
	e := newEnv(t)
	sub := e.follow(func(in *SubscriptionInput) { in.LockVersion = off() })
	jf := e.indexServer("jf", true, nil)
	e.indexItems(jf, episodeItem("e2", 231620, 4, 2))
	e.dl.setTorrents(downloader.Torrent{
		InfoHash: "AAAA0000000000000000000000000000000000E2", Name: "Natural.High.S04E02.2023.1080p.TX.WEB-DL.H.264.DDP2.0-HHWEB", TotalSize: 1 << 30,
	})
	lg := e.ledger(sub)
	assert.Empty(t, e.linked(sub.ID), "第 2 集在服务器里，不认领")
	assert.Equal(t, [2]string{models.MediaEpisodeLibrary, models.MediaEpisodeFromServer}, [2]string{lg.row(2).State, lg.row(2).Source})
}

// 季 tab 的已入库和集卡一样：开了「算作已入库」的服务器里有的集也算；「库里」那一行只算整理记录。
func TestItemSeasonTabCountsServerEpisodes(t *testing.T) {
	e := newEnv(t)
	e.tmdb.set("/3/tv/100088", tlouFull())
	require.NoError(t, e.db.Create(&models.MediaTransferHistory{
		SourcePath: "/dl/e01.mkv", TargetPath: "/lib/tlou/S02E01.mkv", MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Episode: 1,
		Status: models.MediaTransferDone,
	}).Error)
	counting := e.indexServer("jf", true, nil)
	other := e.indexServer("emby", false, nil)
	e.indexItems(counting, episodeItem("e1", 100088, 2, 1), episodeItem("e2", 100088, 2, 2))
	e.indexItems(other, episodeItem("x3", 100088, 2, 3))

	d, err := e.svc.Item(e.ctx, models.MediaKindTV, 100088)
	require.NoError(t, err)
	require.Len(t, d.Seasons, 3)
	assert.Equal(t, 2, d.Seasons[1].Have, "第 1 集整理过、第 2 集在算作已入库的服务器里；第 3 集只在没开选项的服务器里")
	assert.Equal(t, []ItemSeasonHave{{Number: 2, Have: 1, Total: 3}}, d.NAS.Library.Seasons, "「库里」只算整理记录")

	// 读不到索引时季 tab 只算整理记录
	require.NoError(t, e.db.Migrator().DropTable(&models.MediaServerItem{}))
	d, err = e.svc.Item(e.ctx, models.MediaKindTV, 100088)
	require.NoError(t, err)
	assert.Equal(t, 1, d.Seasons[1].Have)
}

// 详情：每台开着、同步过的服务器有没有这一部（剧集的集数与各季、打开链接、服务器里删了的）；关着的、没同步过的不列。
func TestItemServers(t *testing.T) {
	e := newEnv(t)
	jf := e.indexServer("jf", false, func(s *models.MediaServer) { s.PlayURL = "https://jf.example.com" })
	emby := e.indexServer("emby", false, func(s *models.MediaServer) { s.Kind = models.MediaServerEmby })
	e.indexServer("off", false, func(s *models.MediaServer) { s.Enabled = false })
	e.indexServer("never", false, func(s *models.MediaServer) { s.IndexedAt = nil })
	e.indexItems(jf,
		models.MediaServerItem{ItemID: "show", Kind: models.MediaServerItemSeries, TMDBID: 100088},
		episodeItem("e1", 100088, 1, 1), episodeItem("e2", 100088, 1, 2), episodeItem("s2e1", 100088, 2, 1),
	)
	require.NoError(t, e.db.Create(&models.MediaIssue{
		Kind: models.MediaIssueServerRemoved, Status: models.MediaIssueOpen, ServerID: jf.ID, ServerItemID: "s2e3",
		MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Episode: 3,
	}).Error)
	require.NoError(t, e.db.Create(&models.MediaIssue{
		Kind: models.MediaIssueServerRemoved, Status: models.MediaIssueDismissed, ServerID: jf.ID, ServerItemID: "s2e4",
		MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Episode: 4,
	}).Error)

	d, err := e.svc.Item(e.ctx, models.MediaKindTV, 100088)
	require.NoError(t, err)
	for _, msg := range d.Errors {
		assert.NotContains(t, msg, "媒体服务器")
	}
	require.Len(t, d.Servers, 2)
	s0 := d.Servers[0]
	assert.Equal(t, jf.ID, s0.ServerID)
	assert.Equal(t, "jf", s0.Name)
	assert.True(t, s0.Have)
	assert.Equal(t, 3, s0.Episodes)
	assert.Equal(t, []ItemSeasonHave{{Number: 1, Have: 2}, {Number: 2, Have: 1}}, s0.Seasons)
	assert.Equal(t, "https://jf.example.com/web/#/details?id=show&serverId=uid-jf", s0.OpenURL)
	require.Len(t, s0.Removed, 1, "忽略了的不列")
	assert.Equal(t, [2]int{2, 3}, [2]int{s0.Removed[0].Season, s0.Removed[0].Episode})
	require.NotNil(t, s0.SyncedAt)
	assert.Equal(t, emby.ID, d.Servers[1].ServerID)
	assert.False(t, d.Servers[1].Have)
	assert.Empty(t, d.Servers[1].OpenURL)
	assert.Equal(t, []ItemServerIssue{}, d.Servers[1].Removed)

	// 电影
	e.tmdb.set("/3/movie/693134", `{"id":693134,"title":"沙丘2","original_title":"Dune: Part Two","release_date":"2024-02-27"}`)
	e.indexItems(emby, models.MediaServerItem{ItemID: "m1", Kind: models.MediaServerItemMovie, TMDBID: 693134})
	d, err = e.svc.Item(e.ctx, models.MediaKindMovie, 693134)
	require.NoError(t, err)
	require.Len(t, d.Servers, 2)
	assert.False(t, d.Servers[0].Have)
	assert.True(t, d.Servers[1].Have)
	assert.Equal(t, "http://emby:8096/web/index.html#!/item?id=m1&serverId=uid-emby", d.Servers[1].OpenURL)
	assert.Zero(t, d.Servers[1].Episodes)

	// 读不到索引：写进 errors，别的照样给
	require.NoError(t, e.db.Migrator().DropTable(&models.MediaIssue{}))
	d, err = e.svc.Item(e.ctx, models.MediaKindTV, 100088)
	require.NoError(t, err)
	assert.Empty(t, d.Servers)
	assert.Contains(t, d.Errors, "媒体服务器的索引没读到：读取问题失败: SQL logic error: no such table: media_issues (1)")
}

// 一季的集：已入库、服务器里有的带打开链接；已入库、服务器里删了的带问题；算作已入库的写来源与服务器。
func TestSeasonItemServers(t *testing.T) {
	e := newEnv(t)
	e.tmdb.set("/3/tv/100088/season/2", `{"season_number":2,"name":"第 2 季","episodes":[
		{"episode_number":1,"name":"未来","air_date":"2025-04-13"},{"episode_number":2,"name":"穿越","air_date":"2025-04-20"},
		{"episode_number":3,"name":"路","air_date":"2025-04-27"},{"episode_number":4,"name":"终","air_date":"2025-05-04"},
		{"episode_number":5,"name":"尾声","air_date":"2025-05-11"}]}`)
	for _, n := range []int{1, 2, 4} {
		require.NoError(t, e.db.Create(&models.MediaTransferHistory{
			SourcePath: "/dl/e0" + string(rune('0'+n)) + ".mkv", MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Episode: n,
			Status: models.MediaTransferDone, TorrentName: "The.Last.of.Us.S02.1080p-HHWEB",
		}).Error)
	}
	jf := e.indexServer("jf", true, nil)
	e.indexServer("emby", false, nil)
	e.indexItems(jf, episodeItem("e1", 100088, 2, 1), episodeItem("e3", 100088, 2, 3))
	require.NoError(t, e.db.Create(&models.MediaIssue{
		Kind: models.MediaIssueServerRemoved, Status: models.MediaIssueOpen, ServerID: jf.ID, ServerItemID: "e2",
		MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Episode: 2,
	}).Error)

	d, err := e.svc.SeasonItem(e.ctx, 100088, 2)
	require.NoError(t, err)
	assert.Empty(t, d.Errors)
	require.Len(t, d.Episodes, 5)
	e1, e2, e3, e4, e5 := d.Episodes[0], d.Episodes[1], d.Episodes[2], d.Episodes[3], d.Episodes[4]
	assert.Equal(t, "http://jf:8096/web/#/details?id=e1&serverId=uid-jf", e1.OpenURL)
	assert.Nil(t, e1.ServerIssue)
	assert.Empty(t, e1.Source, "整理记录里有的不写来源")
	assert.Empty(t, e2.OpenURL)
	require.NotNil(t, e2.ServerIssue)
	assert.Equal(t, EpisodeIssue{ID: e2.ServerIssue.ID, Server: "jf"}, *e2.ServerIssue)
	assert.Equal(t, models.MediaEpisodeLibrary, e3.State, "整理记录里没有、算作已入库的服务器里有")
	assert.Equal(t, [2]string{models.MediaEpisodeFromServer, "jf"}, [2]string{e3.Source, e3.Server})
	assert.NotEmpty(t, e3.OpenURL)
	assert.Equal(t, models.MediaEpisodeLibrary, e4.State)
	assert.Nil(t, e4.ServerIssue, "第 4 集服务器里没有，但没有它的问题：不借第 2 集的")
	assert.Empty(t, e4.OpenURL)
	assert.Equal(t, models.MediaEpisodeMissing, e5.State)
	assert.Empty(t, e5.OpenURL)

	// 读不到索引：写进 errors
	require.NoError(t, e.db.Migrator().DropTable(&models.MediaIssue{}))
	d, err = e.svc.SeasonItem(e.ctx, 100088, 2)
	require.NoError(t, err)
	require.Len(t, d.Errors, 1)
	assert.Contains(t, d.Errors[0], "媒体服务器的索引没读到")
}

// 日历：开了「算作已入库」的服务器里有的集算已入库。
func TestCalendarCountsServerEpisodes(t *testing.T) {
	e := newEnv(t)
	e.setNow(time.Date(2026, 10, 8, 20, 0, 0, 0, time.Local))
	e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 231620, Season: 4})
	jf := e.indexServer("jf", true, nil)
	e.indexItems(jf, episodeItem("e2", 231620, 4, 2))
	cal, err := e.svc.Calendar(e.ctx, "2026-10-01", "2026-10-03")
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-10-01 现在就出发 E1 missing", "2026-10-02 现在就出发 E2 library"}, calendarLines(cal))

	require.NoError(t, e.db.Migrator().DropTable(&models.MediaServerItem{}))
	cal, err = e.svc.Calendar(e.ctx, "2026-10-01", "2026-10-03")
	require.NoError(t, err, "读不到索引时不算服务器里的集，不挡日历")
	assert.Equal(t, []string{"2026-10-01 现在就出发 E1 missing", "2026-10-02 现在就出发 E2 missing"}, calendarLines(cal))
}
