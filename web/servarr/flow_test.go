package servarr

import (
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/models"
)

// 照 Seerr 的真实调用顺序走一遍：设置里测试连接、请求一部电影与一部剧的第 2 季、扫描、下载同步、再请求第 1 季、删除。

func TestSeerrRadarrSettingsTest(t *testing.T) {
	e := newEnv(t)
	c := e.seerr("radarr")
	// routes/settings/radarr.ts 的 /test：系统状态的 urlBase、档案、根目录、标签
	st, err := c.getSystemStatus()
	require.NoError(t, err)
	assert.Equal(t, "/servarr/radarr", st["urlBase"])
	assert.Equal(t, RadarrVersion, st["version"])
	_, err = e.subs.SaveProfile(e.ctx, 0, subscribe.ProfileInput{Name: "1080p"})
	require.NoError(t, err)
	profiles, err := c.getProfiles()
	require.NoError(t, err)
	require.Len(t, profiles, 2)
	assert.Equal(t, map[string]any{"id": float64(defaultProfileID), "name": "默认档案"}, profiles[0])
	assert.Equal(t, "1080p", profiles[1].(map[string]any)["name"])
	folders, err := c.getRootFolders()
	require.NoError(t, err)
	require.Len(t, folders, 2, "auto 与启用的电影库（停用的与剧集库不列）")
	assert.Equal(t, autoRootPath, folders[0].(map[string]any)["path"])
	assert.Equal(t, float64(autoRootID), folders[0].(map[string]any)["id"])
	assert.Equal(t, e.movieLib.Path, folders[1].(map[string]any)["path"])
	assert.Equal(t, float64(100<<30), folders[1].(map[string]any)["freeSpace"])
	assert.Equal(t, float64(100<<30), folders[0].(map[string]any)["freeSpace"], "auto 写第一个电影库的空间")
	tags, err := c.getTags()
	require.NoError(t, err)
	assert.Empty(t, tags)
}

func TestSeerrSonarrSettingsTest(t *testing.T) {
	e := newEnv(t)
	c := e.seerr("sonarr")
	st, err := c.getSystemStatus()
	require.NoError(t, err)
	assert.Equal(t, "/servarr/sonarr", st["urlBase"])
	// routes/settings/sonarr.ts：主版本 <= 3 时才要语言档案
	assert.Equal(t, SonarrVersion, st["version"])
	folders, err := c.getRootFolders()
	require.NoError(t, err)
	require.Len(t, folders, 3)
	assert.Equal(t, []any{autoRootPath, e.tvLib.Path, e.animeLib.Path},
		[]any{folders[0].(map[string]any)["path"], folders[1].(map[string]any)["path"], folders[2].(map[string]any)["path"]})
	assert.Equal(t, false, folders[2].(map[string]any)["accessible"], "读不到空间的库照样列出")
	assert.Equal(t, float64(0), folders[2].(map[string]any)["freeSpace"])
	lp, err := c.getLanguageProfiles()
	require.NoError(t, err)
	assert.Equal(t, []any{map[string]any{"id": float64(1), "name": "Any"}}, lp)
}

func TestSeerrMovieRequest(t *testing.T) {
	e := newEnv(t)
	c := e.seerr("radarr")
	// MediaRequestSubscriber：tagRequests 打开时先找、没有时建「用户编号-名字」的标签
	tags, err := c.getTags()
	require.NoError(t, err)
	require.Empty(t, tags)
	tg, err := c.createTag("1-admin")
	require.NoError(t, err)
	tagID := num(tg["id"])

	m, err := c.addMovie(radarrMovieOptions{
		title: "Interstellar", qualityProfileID: defaultProfileID, profileID: defaultProfileID, minimumAvailability: "released",
		tags: []int{tagID}, year: 2014, rootFolderPath: e.movieLib.Path, tmdbID: 157336, monitored: true, searchNow: true,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"GET /tag", "POST /tag", "GET /movie/lookup", "POST /movie"}, c.calls)
	sub := e.sub(models.MediaKindMovie, 157336, 0)
	require.NotNil(t, sub)
	assert.Equal(t, float64(sub.ID), m["id"], "movie 的 id 是电影订阅的编号")
	assert.Equal(t, "157336", m["titleSlug"])
	assert.Equal(t, []any{float64(tagID)}, m["tags"])
	assert.Equal(t, true, m["monitored"])
	assert.Equal(t, false, m["hasFile"])
	assert.Equal(t, models.MediaSubFromServarr, sub.Source)
	assert.Equal(t, models.MediaSubActive, sub.Status)
	assert.Equal(t, uint(0), sub.ProfileID, "默认档案")
	assert.Equal(t, e.movieLib.ID, sub.LibraryID)
	assert.Equal(t, "星际穿越", sub.Title)
	require.NotNil(t, sub.NextSearchAt)
	assert.True(t, sub.NextSearchAt.Equal(e.Now()), "searchForMovie：马上搜")
	assert.Empty(t, sub.SavePath)
	assert.Empty(t, sub.Category)
	assert.Equal(t, uint(0), sub.DownloaderID)

	// 扫描（scanners/radarr）：在找、没有文件是处理中
	movies, err := c.getMovies()
	require.NoError(t, err)
	require.Len(t, movies, 1)
	mv := movies[0].(map[string]any)
	assert.Equal(t, float64(157336), mv["tmdbId"])
	assert.Equal(t, true, mv["monitored"])
	assert.Equal(t, false, mv["hasFile"])
	byTMDB, err := c.getLibraryMoviesByTmdbID(157336)
	require.NoError(t, err)
	require.Len(t, byTMDB, 1)
	none, err := c.getLibraryMoviesByTmdbID(27205)
	require.NoError(t, err)
	assert.Empty(t, none)

	// 下载同步（downloadtracker）：每分钟一次，不记审计，不报进度
	require.NoError(t, c.refreshMonitoredDownloads())
	q, err := c.getQueue()
	require.NoError(t, err)
	assert.Empty(t, q)

	// 整理入库以后：hasFile，可用性同步（availabilitySync 的 getMovie）也看得到
	e.organized(models.MediaKindMovie, 157336, 0, 0, filepath.Join(e.movieLib.Path, "星际穿越 (2014)", "星际穿越.mkv"), 30<<30)
	got, err := c.getMovie(int(sub.ID))
	require.NoError(t, err)
	assert.Equal(t, true, got["hasFile"])
	assert.Equal(t, float64(30<<30), got["sizeOnDisk"])
	assert.Equal(t, filepath.Join(e.movieLib.Path, "星际穿越 (2014)"), got["path"])
	assert.Equal(t, "星际穿越 (2014)", got["folderName"])

	// 再请求：库里已经有了，Seerr 直接返回（不发 POST）
	c.calls = nil
	_, err = c.addMovie(radarrMovieOptions{tmdbID: 157336, monitored: true, searchNow: true, rootFolderPath: autoRootPath, qualityProfileID: defaultProfileID})
	require.NoError(t, err)
	assert.Equal(t, []string{"GET /movie/lookup"}, c.calls)

	// 删除（routes/media.ts 的 removeMovie）：只删订阅，整理记录与文件不动
	require.NoError(t, c.removeMovie(157336))
	assert.Nil(t, e.sub(models.MediaKindMovie, 157336, 0))
	var n int64
	require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Where("tmdb_id = ?", 157336).Count(&n).Error)
	assert.Equal(t, int64(1), n)
	_, err = c.getMovie(int(sub.ID))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
	var links int64
	require.NoError(t, e.db.Model(&models.ServarrTagLink{}).Count(&links).Error)
	assert.Equal(t, int64(0), links)

	// 审计：写操作都记了（用户是令牌编号），RefreshMonitoredDownloads 没记
	tok, err := e.tokens.Verify(e.ctx, e.key)
	require.NoError(t, err)
	posts := e.audits("POST /servarr/radarr/api/v3/movie")
	require.Len(t, posts, 1)
	assert.Equal(t, "success", posts[0].Result)
	assert.Equal(t, ChannelType, posts[0].ChannelType)
	assert.Equal(t, formatID(tok.ID), posts[0].ChannelUserID)
	assert.Equal(t, 157336, posts[0].Args["tmdb_id"])
	assert.Equal(t, sub.ID, posts[0].Args["subscription_id"])
	assert.Equal(t, "seerr", posts[0].Args["name"])
	require.Len(t, e.audits("POST /servarr/radarr/api/v3/tag"), 1)
	require.Len(t, e.audits("DELETE /servarr/radarr/api/v3/movie/"+formatID(sub.ID)), 1)
	assert.Empty(t, e.audits("POST /servarr/radarr/api/v3/command"), "RefreshMonitoredDownloads 不记审计")
	for _, a := range e.audit.all() {
		assert.NotContains(t, a.Command, e.key)
	}
}

func TestSeerrMovieRequestExisting(t *testing.T) {
	e := newEnv(t)
	c := e.seerr("radarr")
	p, err := e.subs.SaveProfile(e.ctx, 0, subscribe.ProfileInput{Name: "4K"})
	require.NoError(t, err)
	// pt-tools 里已经有一个暂停的订阅（别的档案、自动选库、带一个 Servarr 标签）
	existing, err := e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 27205, ProfileID: p.ID, Paused: true}, models.MediaSubFromManual)
	require.NoError(t, err)
	old, err := c.createTag("old")
	require.NoError(t, err)
	_, err = e.srv.setTags(e.ctx, tagKindMovie, 27205, []uint{uint(num(old["id"]))})
	require.NoError(t, err)
	tg, err := c.createTag("2-bob")
	require.NoError(t, err)
	e.advance(time.Minute)

	c.calls = nil
	m, err := c.addMovie(radarrMovieOptions{
		title: "Inception", qualityProfileID: defaultProfileID, profileID: defaultProfileID, minimumAvailability: "released",
		tags: []int{num(tg["id"])}, year: 2010, rootFolderPath: e.movieLib.Path, tmdbID: 27205, monitored: true, searchNow: true,
	})
	require.NoError(t, err)
	// 有订阅、没在找：PUT 打开；在找、可用时再发 MoviesSearch
	assert.Equal(t, []string{"GET /movie/lookup", "PUT /movie", "POST /command"}, c.calls)
	assert.Equal(t, float64(existing.ID), m["id"])
	assert.Equal(t, true, m["monitored"])
	assert.Equal(t, []any{old["id"], tg["id"]}, m["tags"], "标签取并集")
	sub := e.sub(models.MediaKindMovie, 27205, 0)
	assert.Equal(t, models.MediaSubActive, sub.Status)
	assert.True(t, sub.NextSearchAt.Equal(e.Now()))
	assert.Equal(t, uint(0), sub.ProfileID, "PUT 按 qualityProfileId 改档案")
	assert.Equal(t, uint(0), sub.LibraryID, "PUT 不改媒体库")
	assert.Equal(t, models.MediaSubFromManual, sub.Source)

	// 已经在找：Seerr 只发搜索命令
	c.calls = nil
	_, err = c.addMovie(radarrMovieOptions{tmdbID: 27205, monitored: true, searchNow: true, qualityProfileID: defaultProfileID, rootFolderPath: autoRootPath})
	require.NoError(t, err)
	assert.Equal(t, []string{"GET /movie/lookup", "POST /command"}, c.calls)
	cmds := e.audits("POST /servarr/radarr/api/v3/command")
	require.Len(t, cmds, 2)
	assert.Equal(t, "MoviesSearch", cmds[1].Args["command"])
	assert.Equal(t, []int{int(existing.ID)}, cmds[1].Args["searched"])
}

func TestSeerrSeriesRequest(t *testing.T) {
	e := newEnv(t)
	c := e.seerr("sonarr")
	anime, err := e.subs.SaveProfile(e.ctx, 0, subscribe.ProfileInput{Name: "动画"})
	require.NoError(t, err)
	yes := true

	// 第一次请求第 2 季：查不到 id，POST，季列表来自查找（全部先是 false，请求的季改成 true）
	s, err := c.addSeries(addSeriesOptions{
		tvdbID: 392256, title: "The Last of Us", profileID: defaultProfileID, seasons: []int{2}, seasonFolder: true,
		rootFolderPath: e.tvLib.Path, tags: []int{}, seriesType: "standard", monitored: &yes, monitorNewItems: "all", searchNow: true,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"GET /series/lookup", "POST /series"}, c.calls)
	assert.Equal(t, float64(100088), s["id"], "series 的 id 是 TMDB 编号")
	assert.Equal(t, float64(392256), s["tvdbId"])
	assert.Nil(t, e.sub(models.MediaKindTV, 100088, 1), "没请求的季不建")
	s2 := e.sub(models.MediaKindTV, 100088, 2)
	require.NotNil(t, s2)
	assert.Equal(t, models.MediaSubFromServarr, s2.Source)
	assert.Equal(t, models.MediaSubActive, s2.Status)
	assert.Equal(t, e.tvLib.ID, s2.LibraryID)
	assert.Equal(t, uint(0), s2.ProfileID)
	assert.Equal(t, models.MediaScheduleAirtime, s2.ScheduleKind, "追剧的默认值：按播出时间")
	assert.True(t, s2.LockVersion, "追剧的默认值：版本锁")
	assert.False(t, s2.FollowNextSeason, "monitorNewItems 不改「跟下一季」")
	assert.True(t, s2.NextSearchAt.Equal(e.Now()), "searchForMissingEpisodes：马上搜")
	seasons := seasonMap(t, s)
	assert.Equal(t, false, seasons[0]["monitored"])
	assert.Equal(t, false, seasons[1]["monitored"])
	assert.Equal(t, true, seasons[2]["monitored"])
	st2 := seasons[2]["statistics"].(map[string]any)
	assert.Equal(t, float64(3), st2["totalEpisodeCount"])
	assert.Equal(t, float64(2), st2["episodeCount"], "第 3 集 2026-12-01 才播")
	assert.Equal(t, float64(0), st2["episodeFileCount"])

	// 扫描（scanners/sonarr）：第 2 季在找、没有文件是处理中
	all, err := c.getSeries()
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, float64(392256), all[0].(map[string]any)["tvdbId"])
	lib, err := c.getLibrarySeriesByTvdbID(392256)
	require.NoError(t, err)
	require.Len(t, lib, 1)

	// 再请求第 1 季（Seerr 这次选了别的档案与根目录）：查到 id，PUT；PUT 以后查分集、按需重新监控、发 MissingEpisodeSearch。
	// PUT 发回的是查找到的 series，只改了 monitored、tags 与 seasons：档案与根目录是 pt-tools 报的（代表季的），不是这次选的
	e.advance(time.Minute)
	c.calls = nil
	s, err = c.addSeries(addSeriesOptions{
		tvdbID: 392256, title: "The Last of Us", profileID: int(anime.ID), seasons: []int{1}, seasonFolder: true,
		rootFolderPath: e.animeLib.Path, tags: []int{}, seriesType: "anime", monitored: &yes, monitorNewItems: "all", searchNow: true,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"GET /series/lookup", "PUT /series", "GET /episode", "POST /command"}, c.calls, "季订阅在找、集都监控着：不调 /episode/monitor")
	s1 := e.sub(models.MediaKindTV, 100088, 1)
	require.NotNil(t, s1)
	assert.Equal(t, uint(0), s1.ProfileID, "新季用请求体里 series 级的档案（第 2 季的默认档案）")
	assert.Equal(t, e.tvLib.ID, s1.LibraryID, "新季用请求体里 series 级的根目录")
	assert.Equal(t, models.MediaSubFromServarr, s1.Source)
	assert.True(t, s1.NextSearchAt.Equal(e.Now()), "MissingEpisodeSearch 让它马上搜")
	s2 = e.sub(models.MediaKindTV, 100088, 2)
	assert.Equal(t, uint(0), s2.ProfileID, "已有的季不改档案")
	assert.Equal(t, e.tvLib.ID, s2.LibraryID, "已有的季不改媒体库")
	assert.True(t, s2.NextSearchAt.Equal(e.Now()))
	seasons = seasonMap(t, s)
	assert.Equal(t, true, seasons[1]["monitored"])
	assert.Equal(t, true, seasons[2]["monitored"])

	// 几季设置不同时 series 级的取哪一条：在找的里编号最大的（第 1 季），没有在找的时编号最大的
	_, err = e.subs.SetProfile(e.ctx, s1.ID, anime.ID)
	require.NoError(t, err)
	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Where("id = ?", s1.ID).Update("library_id", e.animeLib.ID).Error)
	got, err := c.getSeriesByID(100088)
	require.NoError(t, err)
	assert.Equal(t, float64(anime.ID), got["qualityProfileId"])
	assert.Equal(t, e.animeLib.Path, got["rootFolderPath"])
	_, err = e.subs.SetStatus(e.ctx, s1.ID, models.MediaSubPaused)
	require.NoError(t, err)
	got, err = c.getSeriesByID(100088)
	require.NoError(t, err)
	assert.Equal(t, float64(defaultProfileID), got["qualityProfileId"], "第 1 季暂停了：取在找的第 2 季")
	assert.Equal(t, e.tvLib.Path, got["rootFolderPath"])
	assert.Equal(t, false, seasonMap(t, got)[1]["monitored"])
	_, err = e.subs.SetStatus(e.ctx, s2.ID, models.MediaSubPaused)
	require.NoError(t, err)
	got, err = c.getSeriesByID(100088)
	require.NoError(t, err)
	assert.Equal(t, float64(anime.ID), got["qualityProfileId"], "都暂停了：取编号最大的第 1 季")
	assert.Equal(t, false, got["monitored"])
	for _, id := range []uint{s1.ID, s2.ID} {
		_, err = e.subs.SetStatus(e.ctx, id, models.MediaSubActive)
		require.NoError(t, err)
	}

	// 整理进库两集以后，扫描看到第 1 季有 2 个文件
	e.organized(models.MediaKindTV, 100088, 1, 1, filepath.Join(e.animeLib.Path, "s1e1.mkv"), 1<<30)
	e.organized(models.MediaKindTV, 100088, 1, 2, filepath.Join(e.animeLib.Path, "s1e2.mkv"), 2<<30)
	got, err = c.getSeriesByID(100088)
	require.NoError(t, err)
	st1 := seasonMap(t, got)[1]["statistics"].(map[string]any)
	assert.Equal(t, float64(2), st1["episodeFileCount"])
	assert.Equal(t, float64(9), st1["totalEpisodeCount"])
	assert.Equal(t, float64(9), st1["episodeCount"])
	assert.Equal(t, float64(3<<30), st1["sizeOnDisk"])
	assert.InDelta(t, 22.22, st1["percentOfEpisodes"], 0.01)
	stats := got["statistics"].(map[string]any)
	assert.Equal(t, float64(2), stats["seasonCount"], "特别篇不算")
	assert.Equal(t, float64(2), stats["episodeFileCount"])
	assert.Equal(t, float64(12), stats["totalEpisodeCount"])

	eps, err := c.getEpisodes(100088)
	require.NoError(t, err)
	require.Len(t, eps, 12)
	first := eps[0].(map[string]any)
	assert.Equal(t, float64(10001), first["id"])
	assert.Equal(t, float64(100088), first["seriesId"])
	assert.Equal(t, true, first["hasFile"])
	assert.Equal(t, true, first["monitored"])
	assert.Equal(t, "2023-01-15", first["airDate"])
	assert.Equal(t, "2023-01-15T00:00:00Z", first["airDateUtc"])
	assert.Equal(t, float64(20003), eps[11].(map[string]any)["id"])
	assert.Equal(t, false, eps[11].(map[string]any)["hasFile"])

	// 删除：这部剧的全部季订阅，不删文件
	require.NoError(t, c.removeSeries(392256))
	assert.Nil(t, e.sub(models.MediaKindTV, 100088, 1))
	assert.Nil(t, e.sub(models.MediaKindTV, 100088, 2))
	_, err = c.getSeriesByID(100088)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
	var n int64
	require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Where("tmdb_id = ?", 100088).Count(&n).Error)
	assert.Equal(t, int64(2), n)

	puts := e.audits("PUT /servarr/sonarr/api/v3/series")
	require.Len(t, puts, 1)
	assert.Equal(t, []seasonPlan{{Season: 1, Action: "created"}, {Season: 2, Action: "unchanged"}}, puts[0].Args["seasons"],
		"没有订阅、也不监控的季（特别篇、没请求的）不写")
	cmds := e.audits("POST /servarr/sonarr/api/v3/command")
	require.Len(t, cmds, 1)
	assert.ElementsMatch(t, []int{1, 2}, cmds[0].Args["searched_seasons"])
	require.Len(t, e.audits("DELETE /servarr/sonarr/api/v3/series/100088"), 1)
}

// seasonMap 是 series 的各季（季号 → 季）。
func seasonMap(t *testing.T, s map[string]any) map[int]map[string]any {
	t.Helper()
	out := map[int]map[string]any{}
	for _, v := range s["seasons"].([]any) {
		m := v.(map[string]any)
		out[num(m["seasonNumber"])] = m
	}
	return out
}

func formatID(id uint) string { return strconv.FormatUint(uint64(id), 10) }

// requireStatus 断言状态码，失败时带上回应。
func requireStatus(t *testing.T, want int, r response) {
	t.Helper()
	require.Equal(t, want, r.status, "%s", r.body)
}
