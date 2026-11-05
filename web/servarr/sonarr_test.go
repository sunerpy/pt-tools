package servarr

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/models"
)

func addSeriesBody(tvdbID int, root string, seasons map[int]bool, monitored, search bool) map[string]any {
	list := []map[string]any{}
	for _, n := range []int{0, 1, 2, 3, 4, 5} {
		if on, ok := seasons[n]; ok {
			list = append(list, map[string]any{"seasonNumber": n, "monitored": on})
		}
	}
	return map[string]any{
		"tvdbId": tvdbID, "title": "x", "qualityProfileId": defaultProfileID, "seasons": list, "tags": []int{},
		"seasonFolder": true, "monitored": monitored, "monitorNewItems": "none", "rootFolderPath": root, "seriesType": "standard",
		"addOptions": map[string]any{"ignoreEpisodesWithFiles": true, "searchForMissingEpisodes": search},
	}
}

func TestAddSeriesOptions(t *testing.T) {
	e := newEnv(t)
	// series 不监控：季订阅建成暂停；不马上搜：按播出时间的追剧排到一天以后
	r := e.sonarr(http.MethodPost, "/series", addSeriesBody(392256, autoRootPath, map[int]bool{1: true, 2: true}, false, false))
	requireStatus(t, http.StatusCreated, r)
	for _, n := range []int{1, 2} {
		sub := e.sub(models.MediaKindTV, 100088, n)
		require.NotNil(t, sub, "第 %d 季", n)
		assert.Equal(t, models.MediaSubPaused, sub.Status)
		assert.Equal(t, uint(0), sub.LibraryID)
		assert.True(t, sub.NextSearchAt.Equal(e.Now().Add(24*time.Hour)), "%v", sub.NextSearchAt)
	}
	assert.Equal(t, false, r.obj(t)["monitored"])
	// 已经有季订阅时 POST（同时发的两个请求）：已有的季只改状态；要马上搜时打开的季马上搜
	r = e.sonarr(http.MethodPost, "/series", addSeriesBody(392256, autoRootPath, map[int]bool{1: true, 2: false}, true, true))
	requireStatus(t, http.StatusCreated, r)
	s1 := e.sub(models.MediaKindTV, 100088, 1)
	assert.Equal(t, models.MediaSubActive, s1.Status)
	assert.True(t, s1.NextSearchAt.Equal(e.Now()), "%v", s1.NextSearchAt)
	assert.Equal(t, models.MediaSubPaused, e.sub(models.MediaKindTV, 100088, 2).Status)
	posts := e.audits("POST /servarr/sonarr/api/v3/series")
	require.Len(t, posts, 2)
	assert.Equal(t, []seasonPlan{{Season: 1, Action: "resumed"}, {Season: 2, Action: "unchanged"}}, posts[1].Args["seasons"])
	assert.Equal(t, 100088, posts[1].Args["tmdb_id"])
	assert.Equal(t, 392256, posts[1].Args["tvdb_id"])
}

// 请求的几季里有建不出的（TMDB 上没有这一季）：建得出的照建，201，审计写明哪一季为什么没建
func TestAddSeriesPartial(t *testing.T) {
	e := newEnv(t)
	r := e.sonarr(http.MethodPost, "/series", addSeriesBody(392256, autoRootPath, map[int]bool{1: true, 5: true}, true, true))
	requireStatus(t, http.StatusCreated, r)
	require.NotNil(t, e.sub(models.MediaKindTV, 100088, 1))
	assert.Nil(t, e.sub(models.MediaKindTV, 100088, 5))
	posts := e.audits("POST /servarr/sonarr/api/v3/series")
	require.Len(t, posts, 1)
	plans := posts[0].Args["seasons"].([]seasonPlan)
	require.Len(t, plans, 2)
	assert.Equal(t, seasonPlan{Season: 1, Action: "created"}, plans[0])
	assert.Equal(t, "skipped", plans[1].Action)
	assert.Contains(t, plans[1].Reason, "没有第 5 季")
}

func TestAddSeriesErrors(t *testing.T) {
	e := newEnv(t)
	badProfile := addSeriesBody(392256, autoRootPath, map[int]bool{2: true}, true, true)
	badProfile["qualityProfileId"] = 42
	cases := []struct {
		name string
		body any
		msg  string
	}{
		{"没有 tvdbId", map[string]any{"title": "x"}, "tvdbId"},
		{"TVDB 换算不到", addSeriesBody(999, autoRootPath, map[int]bool{1: true}, true, true), "TVDB 编号是 999"},
		{"档案不存在", badProfile, "质量档案不存在"},
		{"电影库不能放剧集", addSeriesBody(392256, e.movieLib.Path, map[int]bool{1: true}, true, true), "不是 pt-tools 里启用的媒体库"},
		{"一季都没监控", addSeriesBody(392256, autoRootPath, map[int]bool{0: false, 1: false}, true, true), "没有可以追的季"},
		{"只监控了特别篇", addSeriesBody(392256, autoRootPath, map[int]bool{0: true}, true, true), "特别篇不建追剧"},
		{"TMDB 上没有这一季", addSeriesBody(392256, autoRootPath, map[int]bool{5: true}, true, true), "没有第 5 季"},
	}
	for _, c := range cases {
		r := e.sonarr(http.MethodPost, "/series", c.body)
		requireStatus(t, http.StatusBadRequest, r)
		assert.Contains(t, r.obj(t)["message"], c.msg, c.name)
	}
	var n int64
	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Count(&n).Error)
	assert.Equal(t, int64(0), n)

	// TMDB 暂时不能访问（换算结果缓存 30 天，换一个新环境）：503，不建订阅
	e = newEnv(t)
	e.tmdb.setFail("/3/find/392256", true)
	r := e.sonarr(http.MethodPost, "/series", addSeriesBody(392256, autoRootPath, map[int]bool{1: true}, true, true))
	requireStatus(t, http.StatusServiceUnavailable, r)
	assert.Nil(t, e.sub(models.MediaKindTV, 100088, 1))
}

func TestLookupSeries(t *testing.T) {
	e := newEnv(t)
	for _, term := range []string{"", "tvdb:", "tvdb:abc", "tvdb:0", "tvdb:999", "nothing"} {
		r := e.sonarr(http.MethodGet, "/series/lookup?term="+term, nil)
		requireStatus(t, http.StatusOK, r)
		assert.Equal(t, []any{}, r.list(t), term)
	}
	// 按名字：最多 10 个，TMDB 上没有 TVDB 编号的不列
	l := e.sonarr(http.MethodGet, "/series/lookup?term=Last", nil).list(t)
	require.Len(t, l, 1)
	s := l[0].(map[string]any)
	assert.Equal(t, float64(392256), s["tvdbId"])
	assert.Equal(t, "最后生还者", s["title"])
	assert.Equal(t, float64(2023), s["year"])
	assert.Equal(t, "末日之后。", s["overview"])
	assert.Equal(t, "https://image.tmdb.org/t/p/w342/tlou.jpg", s["remotePoster"])
	_, hasID := s["id"]
	assert.False(t, hasID)

	// 有季订阅：带 id；TMDB 出错时 503
	_, err := e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 1}, models.MediaSubFromManual)
	require.NoError(t, err)
	s = e.sonarr(http.MethodGet, "/series/lookup?term=tvdb:392256", nil).list(t)[0].(map[string]any)
	assert.Equal(t, float64(100088), s["id"])
	assert.Equal(t, true, seasonMap(t, s)[1]["monitored"])
	e.tmdb.setFail("/3/search/tv", true)
	requireStatus(t, http.StatusServiceUnavailable, e.sonarr(http.MethodGet, "/series/lookup?term=Lost", nil))
	e.tmdb.setFail("/3/find/81189", true)
	requireStatus(t, http.StatusServiceUnavailable, e.sonarr(http.MethodGet, "/series/lookup?term=tvdb:81189", nil))
}

func TestListSeries(t *testing.T) {
	e := newEnv(t)
	assert.Equal(t, []any{}, e.sonarr(http.MethodGet, "/series", nil).list(t))
	_, err := e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2}, models.MediaSubFromManual)
	require.NoError(t, err)
	// TMDB 上没有 TVDB 编号的剧：Seerr 认不出，不列，写日志
	_, err = e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 231620, Season: 1}, models.MediaSubFromManual)
	require.NoError(t, err)
	// 电影订阅不算
	_, err = e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 157336}, models.MediaSubFromManual)
	require.NoError(t, err)
	l := e.sonarr(http.MethodGet, "/series", nil).list(t)
	require.Len(t, l, 1)
	assert.Equal(t, float64(100088), l[0].(map[string]any)["id"])
	assert.Equal(t, 1, e.logs.FilterMessageSnippet("这次没有列出 1 部剧").Len())
	assert.Equal(t, 1, e.logs.FilterMessageSnippet("现在就出发").Len())

	// 按 TVDB 编号
	assert.Len(t, e.sonarr(http.MethodGet, "/series?tvdbId=392256", nil).list(t), 1)
	assert.Equal(t, []any{}, e.sonarr(http.MethodGet, "/series?tvdbId=999", nil).list(t))
	assert.Equal(t, []any{}, e.sonarr(http.MethodGet, "/series?tvdbId=abc", nil).list(t))

	// 按编号；没有季订阅的 404
	requireStatus(t, http.StatusOK, e.sonarr(http.MethodGet, "/series/100088", nil))
	requireStatus(t, http.StatusNotFound, e.sonarr(http.MethodGet, "/series/1399", nil))
	requireStatus(t, http.StatusNotFound, e.sonarr(http.MethodGet, "/series/abc", nil))
}

func TestListSeriesTMDBDown(t *testing.T) {
	e := newEnv(t)
	_, err := e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2}, models.MediaSubFromManual)
	require.NoError(t, err)
	// 建订阅时取过详情与这一季；TVDB 编号与各季的列表还没取过，这时 TMDB 不能访问
	e.tmdb.setFail("/3/tv/100088", true)
	assert.Equal(t, []any{}, e.sonarr(http.MethodGet, "/series", nil).list(t), "取不到资料的剧这次不列")
	assert.Equal(t, 1, e.logs.FilterMessageSnippet("暂时取不到资料").Len())
	r := e.sonarr(http.MethodGet, "/series/100088", nil)
	requireStatus(t, http.StatusServiceUnavailable, r)
	assert.Contains(t, r.obj(t)["message"], "TMDB")
	// 恢复以后照常列出（结果有缓存）
	e.tmdb.setFail("/3/tv/100088", false)
	assert.Len(t, e.sonarr(http.MethodGet, "/series", nil).list(t), 1)
	hits := e.tmdb.hitCount("/3/tv/100088")
	assert.Len(t, e.sonarr(http.MethodGet, "/series", nil).list(t), 1)
	assert.Equal(t, hits, e.tmdb.hitCount("/3/tv/100088"), "第二次读缓存")
}

func TestSeriesStatistics(t *testing.T) {
	e := newEnv(t)
	manual := 4
	_, err := e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, TotalEpisodes: &manual}, models.MediaSubFromManual)
	require.NoError(t, err)
	e.organized(models.MediaKindTV, 100088, 2, 1, e.tvLib.Path+"/s2e1.mkv", 1<<30)
	// 没订阅的季：库里有的也算文件
	e.organized(models.MediaKindTV, 100088, 1, 9, e.tvLib.Path+"/s1e9.mkv", 2<<30)
	s := e.sonarr(http.MethodGet, "/series/100088", nil).obj(t)
	seasons := seasonMap(t, s)
	st2 := seasons[2]["statistics"].(map[string]any)
	assert.Equal(t, float64(4), st2["totalEpisodeCount"], "手动填了集数的季订阅用它")
	assert.Equal(t, float64(2), st2["episodeCount"])
	assert.Equal(t, float64(1), st2["episodeFileCount"])
	assert.Equal(t, float64(50), st2["percentOfEpisodes"])
	st1 := seasons[1]["statistics"].(map[string]any)
	assert.Equal(t, float64(1), st1["episodeFileCount"])
	assert.Equal(t, float64(9), st1["episodeCount"], "没订阅、已经开播的季算全部已播")
	assert.Equal(t, false, seasons[1]["monitored"])
	assert.Equal(t, float64(2023), s["year"], "年份是第 1 季开播的年份")
	assert.Equal(t, "none", s["monitorNewItems"])
	assert.Equal(t, "tt3581920", s["imdbId"])
	stats := s["statistics"].(map[string]any)
	assert.Equal(t, float64(2), stats["episodeFileCount"])
	assert.Equal(t, float64(3<<30), stats["sizeOnDisk"])
	assert.Equal(t, float64(13), stats["totalEpisodeCount"])
	assert.Equal(t, float64(11), stats["episodeCount"])
	assert.InDelta(t, 18.18, stats["percentOfEpisodes"], 0.01)

	// 跟下一季：monitorNewItems 写 all
	sub := e.sub(models.MediaKindTV, 100088, 2)
	require.NoError(t, e.db.Model(sub).Update("follow_next_season", true).Error)
	assert.Equal(t, "all", e.sonarr(http.MethodGet, "/series/100088", nil).obj(t)["monitorNewItems"])

	// TMDB 上没有的季（例如手动建的）也列出
	require.NoError(t, e.db.Create(&models.MediaSubscription{
		MediaType: models.MediaKindTV, TMDBID: 100088, Season: 3, Title: "最后生还者",
		TotalEpisodes: 8, Status: models.MediaSubActive, CreatedAt: e.Now(),
	}).Error)
	seasons = seasonMap(t, e.sonarr(http.MethodGet, "/series/100088", nil).obj(t))
	require.Contains(t, seasons, 3)
	assert.Equal(t, true, seasons[3]["monitored"])
	assert.Equal(t, float64(8), seasons[3]["statistics"].(map[string]any)["totalEpisodeCount"])
}

// 一季整理好的集数多于已播出的集数（例如提前放出的）：百分比到 100 为止
func TestSeasonPercentCapped(t *testing.T) {
	e := newEnv(t)
	_, err := e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2}, models.MediaSubFromManual)
	require.NoError(t, err)
	for n := 1; n <= 3; n++ {
		e.organized(models.MediaKindTV, 100088, 2, n, e.tvLib.Path+"/s2e"+formatID(uint(n))+".mkv", 1<<30)
	}
	st := seasonMap(t, e.sonarr(http.MethodGet, "/series/100088", nil).obj(t))[2]["statistics"].(map[string]any)
	assert.Equal(t, float64(3), st["episodeFileCount"])
	assert.Equal(t, float64(2), st["episodeCount"], "第 3 集还没播")
	assert.Equal(t, float64(100), st["percentOfEpisodes"])
}

// 列出全部剧集时 TMDB 太慢：超出总时限的剧这次不列，回应不等 TMDB（Seerr 的请求 10 秒就超时）
func TestListSeriesBudget(t *testing.T) {
	e := newEnv(t)
	_, err := e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2}, models.MediaSubFromManual)
	require.NoError(t, err)
	saved := listBudget
	listBudget = 200 * time.Millisecond
	t.Cleanup(func() { listBudget = saved })
	e.tmdb.setSlow("/3/tv/100088", 3*time.Second)
	start := time.Now()
	assert.Equal(t, []any{}, e.sonarr(http.MethodGet, "/series", nil).list(t))
	assert.Less(t, time.Since(start), 2*time.Second)
	assert.Equal(t, 1, e.logs.FilterMessageSnippet("暂时取不到资料").Len())
}

func TestUpdateSeries(t *testing.T) {
	e := newEnv(t)
	p, err := e.subs.SaveProfile(e.ctx, 0, subscribe.ProfileInput{Name: "1080p"})
	require.NoError(t, err)
	lib := e.tvLib.ID
	s2, err := e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{
		MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, ProfileID: p.ID, LibraryID: &lib,
		SavePath: "/dl/tv", Category: "tv", Tags: "pt",
	}, models.MediaSubFromManual)
	require.NoError(t, err)
	tg := e.sonarr(http.MethodPost, "/tag", map[string]any{"label": "x"}).obj(t)

	// 路径带 id：新的季用请求体的档案与根目录（不马上搜），已有的季只改状态
	body := map[string]any{
		"tvdbId": 392256, "qualityProfileId": defaultProfileID, "rootFolderPath": e.animeLib.Path, "tags": []any{tg["id"]},
		"seasons": []map[string]any{{"seasonNumber": 0, "monitored": true}, {"seasonNumber": 1, "monitored": true}, {"seasonNumber": 2, "monitored": false}},
	}
	r := e.sonarr(http.MethodPut, "/series/100088", body)
	requireStatus(t, http.StatusOK, r)
	s1 := e.sub(models.MediaKindTV, 100088, 1)
	require.NotNil(t, s1)
	assert.Equal(t, uint(0), s1.ProfileID)
	assert.Equal(t, e.animeLib.ID, s1.LibraryID)
	assert.Equal(t, models.MediaSubActive, s1.Status)
	assert.True(t, s1.NextSearchAt.Equal(e.Now().Add(24*time.Hour)), "PUT 建的季不马上搜，等 MissingEpisodeSearch")
	row := e.sub(models.MediaKindTV, 100088, 2)
	assert.Equal(t, models.MediaSubPaused, row.Status)
	assert.Equal(t, p.ID, row.ProfileID, "已有的季不改档案")
	assert.Equal(t, e.tvLib.ID, row.LibraryID, "不改媒体库")
	assert.Equal(t, "/dl/tv", row.SavePath, "不改保存路径")
	assert.Equal(t, "tv", row.Category)
	assert.Equal(t, "pt", row.Tags, "不改下载标签")
	assert.Nil(t, e.sub(models.MediaKindTV, 100088, 0), "特别篇不建")
	assert.Equal(t, []any{tg["id"]}, r.obj(t)["tags"])
	puts := e.audits("PUT /servarr/sonarr/api/v3/series/100088")
	require.Len(t, puts, 1)
	assert.Equal(t, []seasonPlan{{Season: 0, Action: "skipped", Reason: "特别篇不建追剧"}, {Season: 1, Action: "created"}, {Season: 2, Action: "paused"}},
		puts[0].Args["seasons"])

	// 再打开：resumed，按定时排（Seerr 要搜时随后发 MissingEpisodeSearch）；不带 tags 时标签不变
	body = map[string]any{"id": 100088, "seasons": []map[string]any{{"seasonNumber": 2, "monitored": true}}}
	r = e.sonarr(http.MethodPut, "/series", body)
	requireStatus(t, http.StatusOK, r)
	row = e.sub(models.MediaKindTV, 100088, 2)
	assert.Equal(t, models.MediaSubActive, row.Status)
	assert.True(t, row.NextSearchAt.After(e.Now()), "%v", row.NextSearchAt)
	assert.Equal(t, []any{tg["id"]}, r.obj(t)["tags"])
	assert.Equal(t, s2.ID, e.sub(models.MediaKindTV, 100088, 2).ID)

	// 错误：没有这部剧、新季的档案或根目录不对、不是 JSON
	requireStatus(t, http.StatusNotFound, e.sonarr(http.MethodPut, "/series", map[string]any{"id": 1399}))
	requireStatus(t, http.StatusNotFound, e.sonarr(http.MethodPut, "/series", map[string]any{"seasons": []any{}}))
	requireStatus(t, http.StatusNotFound, e.sonarr(http.MethodPut, "/series/abc", map[string]any{}))
	bad := map[string]any{"id": 100088, "qualityProfileId": 42, "seasons": []map[string]any{{"seasonNumber": 3, "monitored": true}}}
	requireStatus(t, http.StatusBadRequest, e.sonarr(http.MethodPut, "/series", bad))
	bad = map[string]any{"id": 100088, "rootFolderPath": "/x", "seasons": []map[string]any{{"seasonNumber": 3, "monitored": true}}}
	requireStatus(t, http.StatusBadRequest, e.sonarr(http.MethodPut, "/series", bad))
	// 只改已有季时，请求体里的档案与根目录不检查（Seerr 发回的是查找到的 series）
	ok := map[string]any{"id": 100088, "qualityProfileId": 42, "rootFolderPath": "/x", "seasons": []map[string]any{{"seasonNumber": 1, "monitored": false}}}
	requireStatus(t, http.StatusOK, e.sonarr(http.MethodPut, "/series", ok))
	assert.Equal(t, models.MediaSubPaused, e.sub(models.MediaKindTV, 100088, 1).Status)
	requireStatus(t, http.StatusBadRequest, e.sonarr(http.MethodPut, "/series", "nope"))
}

func TestEpisodes(t *testing.T) {
	e := newEnv(t)
	requireStatus(t, http.StatusBadRequest, e.sonarr(http.MethodGet, "/episode", nil))
	requireStatus(t, http.StatusBadRequest, e.sonarr(http.MethodGet, "/episode?seriesId=abc", nil))
	requireStatus(t, http.StatusNotFound, e.sonarr(http.MethodGet, "/episode?seriesId=100088", nil))
	sub, err := e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2}, models.MediaSubFromManual)
	require.NoError(t, err)
	_, err = e.subs.SkipEpisode(e.ctx, sub.ID, 3, true)
	require.NoError(t, err)
	eps := e.sonarr(http.MethodGet, "/episode?seriesId=100088", nil).list(t)
	require.Len(t, eps, 3)
	last := eps[2].(map[string]any)
	assert.Equal(t, float64(20003), last["id"])
	assert.Equal(t, false, last["monitored"], "跳过的集不监控")
	assert.Equal(t, "路", last["title"])
	assert.Equal(t, "", last["overview"])
	assert.Equal(t, float64(0), last["episodeFileId"])
	assert.Equal(t, true, eps[0].(map[string]any)["monitored"])
	_, err = e.subs.SetStatus(e.ctx, sub.ID, models.MediaSubPaused)
	require.NoError(t, err)
	assert.Equal(t, false, e.sonarr(http.MethodGet, "/episode?seriesId=100088", nil).list(t)[0].(map[string]any)["monitored"], "暂停的季不监控")

	// 按集开关：不支持，回 202 {}，记审计、写日志
	r := e.sonarr(http.MethodPut, "/episode/monitor", map[string]any{"episodeIds": []int{20001, 20002}, "monitored": true})
	requireStatus(t, http.StatusAccepted, r)
	assert.Equal(t, map[string]any{}, r.obj(t))
	a := e.audits("PUT /servarr/sonarr/api/v3/episode/monitor")
	require.Len(t, a, 1)
	assert.Equal(t, 2, a[0].Args["episodes"])
	assert.Equal(t, true, a[0].Args["ignored"])
	assert.Equal(t, 1, e.logs.FilterMessageSnippet("不支持按集开关").Len())
	requireStatus(t, http.StatusBadRequest, e.sonarr(http.MethodPut, "/episode/monitor", "{"))

	// 台账读不到：500
	require.NoError(t, e.db.Exec("DROP TABLE media_subscription_episodes").Error)
	assert.GreaterOrEqual(t, e.sonarr(http.MethodGet, "/episode?seriesId=100088", nil).status, 500)
}

func TestDeleteSeries(t *testing.T) {
	e := newEnv(t)
	for _, n := range []int{1, 2} {
		_, err := e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: n}, models.MediaSubFromManual)
		require.NoError(t, err)
	}
	tg := e.sonarr(http.MethodPost, "/tag", map[string]any{"label": "a"}).obj(t)
	_, err := e.srv.setTags(e.ctx, tagKindSeries, 100088, []uint{uint(num(tg["id"]))})
	require.NoError(t, err)
	r := e.sonarr(http.MethodDelete, "/series/100088?deleteFiles=true&addImportExclusion=false", nil)
	requireStatus(t, http.StatusOK, r)
	assert.Equal(t, map[string]any{}, r.obj(t))
	assert.Nil(t, e.sub(models.MediaKindTV, 100088, 1))
	assert.Nil(t, e.sub(models.MediaKindTV, 100088, 2))
	assert.Equal(t, []uint{}, e.srv.tagsOf(e.ctx, tagKindSeries, 100088), "标签一起删")
	requireStatus(t, http.StatusNotFound, e.sonarr(http.MethodDelete, "/series/100088", nil))
	requireStatus(t, http.StatusNotFound, e.sonarr(http.MethodDelete, "/series/abc", nil))
	a := e.audits("DELETE /servarr/sonarr/api/v3/series/100088")
	require.Len(t, a, 2)
	assert.ElementsMatch(t, []int{1, 2}, a[0].Args["deleted_seasons"])
}

func TestRepresentative(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	subs := []models.MediaSubscription{
		{ID: 3, Status: models.MediaSubPaused, CreatedAt: at},
		{ID: 1, Status: models.MediaSubActive, CreatedAt: at},
		{ID: 2, Status: models.MediaSubActive, CreatedAt: at},
		{ID: 4, Status: models.MediaSubDone, CreatedAt: at},
	}
	assert.Equal(t, uint(2), representative(subs).ID, "在找的里编号最大的")
	assert.Equal(t, uint(4), representative([]models.MediaSubscription{subs[0], subs[3]}).ID, "没有在找的：编号最大的")
	assert.Equal(t, uint(1), representative(subs[1:2]).ID)
	assert.Equal(t, uint(0), representative(nil).ID)
}

func TestYearOf(t *testing.T) {
	assert.Equal(t, 2023, yearOf("2023-01-15"))
	assert.Equal(t, 0, yearOf(""))
	assert.Equal(t, 0, yearOf("abc"))
	assert.Equal(t, 0, yearOf("20"))
	assert.False(t, strings.Contains(timeText(time.Time{}), "+"))
}
