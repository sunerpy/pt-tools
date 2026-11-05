package servarr

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/models"
)

func addMovieBody(tmdbID int, root string, monitored, search bool) map[string]any {
	return map[string]any{
		"title": "x", "qualityProfileId": defaultProfileID, "profileId": defaultProfileID, "titleSlug": formatID(uint(tmdbID)),
		"minimumAvailability": "released", "tmdbId": tmdbID, "year": 2014, "rootFolderPath": root, "monitored": monitored,
		"tags": []int{}, "addOptions": map[string]any{"searchForMovie": search},
	}
}

func TestAddMovieOptions(t *testing.T) {
	e := newEnv(t)
	// 不监控：建成暂停
	r := e.radarr(http.MethodPost, "/movie", addMovieBody(157336, autoRootPath, false, true))
	requireStatus(t, http.StatusCreated, r)
	sub := e.sub(models.MediaKindMovie, 157336, 0)
	assert.Equal(t, models.MediaSubPaused, sub.Status)
	assert.Equal(t, uint(0), sub.LibraryID, "auto：按类型自动选库")
	assert.Equal(t, false, r.obj(t)["monitored"])
	assert.Equal(t, autoRootPath, r.obj(t)["rootFolderPath"])
	assert.Equal(t, autoRootPath, r.obj(t)["path"], "没整理过：path 写根目录")

	// 不马上搜（Seerr 的 preventSearch）：按订阅设置的间隔排
	r = e.radarr(http.MethodPost, "/movie", addMovieBody(27205, e.movieLib.Path, true, false))
	requireStatus(t, http.StatusCreated, r)
	sub = e.sub(models.MediaKindMovie, 27205, 0)
	assert.Equal(t, models.MediaSubActive, sub.Status)
	assert.True(t, sub.NextSearchAt.Equal(e.Now().Add(subscribe.DefaultSearchIntervalHours*time.Hour)), "%v", sub.NextSearchAt)

	// 已经订阅过：回已有的那个（201，带 id）
	r = e.radarr(http.MethodPost, "/movie", addMovieBody(27205, autoRootPath, true, true))
	requireStatus(t, http.StatusCreated, r)
	assert.Equal(t, float64(sub.ID), r.obj(t)["id"])
	posts := e.audits("POST /servarr/radarr/api/v3/movie")
	require.Len(t, posts, 3)
	assert.Equal(t, true, posts[2].Args["existing"])

	// 不带 monitored 与 addOptions（Seerr 都会带；别的客户端不带时按 Radarr 的默认）：建成在找、马上搜
	requireStatus(t, http.StatusOK, e.radarr(http.MethodDelete, "/movie/"+formatID(sub.ID), nil))
	requireStatus(t, http.StatusCreated, e.radarr(http.MethodPost, "/movie", map[string]any{"tmdbId": 27205, "qualityProfileId": defaultProfileID}))
	sub = e.sub(models.MediaKindMovie, 27205, 0)
	assert.Equal(t, models.MediaSubActive, sub.Status)
	assert.True(t, sub.NextSearchAt.Equal(e.Now()), "%v", sub.NextSearchAt)
}

func TestAddMovieErrors(t *testing.T) {
	e := newEnv(t)
	badProfile := addMovieBody(157336, autoRootPath, true, true)
	badProfile["qualityProfileId"] = 42
	cases := []struct {
		name string
		body any
		msg  string
	}{
		{"没有 tmdbId", map[string]any{"title": "x"}, "tmdbId"},
		{"档案不存在", badProfile, "质量档案不存在"},
		{"根目录不是媒体库", addMovieBody(157336, "/data/somewhere", true, true), "不是 pt-tools 里启用的媒体库"},
		{"剧集库不能放电影", addMovieBody(157336, e.tvLib.Path, true, true), "不是 pt-tools 里启用的媒体库"},
		{"停用的库不认", addMovieBody(157336, filepath.Join(filepath.Dir(e.movieLib.Path), "off"), true, true), "不是 pt-tools 里启用的媒体库"},
		{"TMDB 上没有", addMovieBody(1, autoRootPath, true, true), "TMDB 上没有编号是 1 的电影"},
	}
	for _, c := range cases {
		r := e.radarr(http.MethodPost, "/movie", c.body)
		requireStatus(t, http.StatusBadRequest, r)
		assert.Contains(t, r.obj(t)["message"], c.msg, c.name)
	}
	assert.Nil(t, e.sub(models.MediaKindMovie, 157336, 0))
	posts := e.audits("POST /servarr/radarr/api/v3/movie")
	require.Len(t, posts, len(cases))
	assert.Equal(t, "error:http_400", posts[0].Result)

	// 根目录是空的：按 auto
	requireStatus(t, http.StatusCreated, e.radarr(http.MethodPost, "/movie", addMovieBody(157336, "", true, true)))
	assert.Equal(t, uint(0), e.sub(models.MediaKindMovie, 157336, 0).LibraryID)

	// TMDB 暂时不能访问：500（不建订阅）
	e.tmdb.setFail("/3/movie/27205", true)
	r := e.radarr(http.MethodPost, "/movie", addMovieBody(27205, autoRootPath, true, true))
	assert.GreaterOrEqual(t, r.status, 500)
	assert.Nil(t, e.sub(models.MediaKindMovie, 27205, 0))
}

func TestLookupMovie(t *testing.T) {
	e := newEnv(t)
	for _, term := range []string{"", "Interstellar", "tmdb:", "tmdb:abc", "tmdb:-1", "imdb:tt0816692"} {
		r := e.radarr(http.MethodGet, "/movie/lookup?term="+term, nil)
		requireStatus(t, http.StatusOK, r)
		assert.Equal(t, []any{}, r.list(t), term)
	}
	// TMDB 上没有：[]
	assert.Equal(t, []any{}, e.radarr(http.MethodGet, "/movie/lookup?term=tmdb:1", nil).list(t))

	// 没有订阅：按 TMDB 填，不带 id；库里有的 hasFile
	e.organized(models.MediaKindMovie, 157336, 0, 0, e.movieLib.Path+"/星际穿越 (2014)/星际穿越.mkv", 5<<30)
	l := e.radarr(http.MethodGet, "/movie/lookup?term=tmdb:157336", nil).list(t)
	require.Len(t, l, 1)
	m := l[0].(map[string]any)
	_, hasID := m["id"]
	assert.False(t, hasID, "没有订阅时不带 id")
	assert.Equal(t, "星际穿越", m["title"])
	assert.Equal(t, "Interstellar", m["originalTitle"])
	assert.Equal(t, float64(2014), m["year"])
	assert.Equal(t, "tt0816692", m["imdbId"])
	assert.Equal(t, true, m["hasFile"])
	assert.Equal(t, false, m["monitored"])
	assert.Equal(t, "一队探险家穿过虫洞。", m["overview"])
	assert.Equal(t, "https://image.tmdb.org/t/p/w342/interstellar.jpg", m["remotePoster"])

	// 有订阅：带 id 与订阅的状态
	sub, err := e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 27205, Paused: true}, models.MediaSubFromManual)
	require.NoError(t, err)
	m = e.radarr(http.MethodGet, "/movie/lookup?term=tmdb:27205", nil).list(t)[0].(map[string]any)
	assert.Equal(t, float64(sub.ID), m["id"])
	assert.Equal(t, false, m["monitored"])
	assert.Equal(t, false, m["hasFile"])

	// TMDB 出错：503
	e.tmdb.setFail("/3/movie/550", true)
	r := e.radarr(http.MethodGet, "/movie/lookup?term=tmdb:550", nil)
	requireStatus(t, http.StatusServiceUnavailable, r)
	assert.Contains(t, r.obj(t)["message"], "TMDB")
}

func TestMovieGetAndList(t *testing.T) {
	e := newEnv(t)
	p, err := e.subs.SaveProfile(e.ctx, 0, subscribe.ProfileInput{Name: "1080p"})
	require.NoError(t, err)
	lib := e.movieLib.ID
	a, err := e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 157336, ProfileID: p.ID, LibraryID: &lib}, models.MediaSubFromManual)
	require.NoError(t, err)
	b, err := e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 27205, Pending: true}, models.MediaSubFromDouban)
	require.NoError(t, err)
	tv, err := e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2}, models.MediaSubFromManual)
	require.NoError(t, err)

	l := e.radarr(http.MethodGet, "/movie", nil).list(t)
	require.Len(t, l, 2, "只有电影订阅")
	first := l[0].(map[string]any)
	assert.Equal(t, float64(a.ID), first["id"])
	assert.Equal(t, float64(p.ID), first["qualityProfileId"])
	assert.Equal(t, float64(p.ID), first["profileId"])
	assert.Equal(t, e.movieLib.Path, first["rootFolderPath"])
	assert.Equal(t, true, first["monitored"])
	assert.Equal(t, true, first["isAvailable"])
	assert.Equal(t, timeText(a.CreatedAt), first["added"])
	second := l[1].(map[string]any)
	assert.Equal(t, false, second["monitored"], "待确认的不算在找")
	assert.Equal(t, float64(defaultProfileID), second["qualityProfileId"])

	assert.Equal(t, []any{}, e.radarr(http.MethodGet, "/movie?tmdbId=abc", nil).list(t))
	assert.Len(t, e.radarr(http.MethodGet, "/movie?tmdbId=27205", nil).list(t), 1)

	requireStatus(t, http.StatusOK, e.radarr(http.MethodGet, "/movie/"+formatID(b.ID), nil))
	requireStatus(t, http.StatusNotFound, e.radarr(http.MethodGet, "/movie/"+formatID(tv.ID), nil))
	requireStatus(t, http.StatusNotFound, e.radarr(http.MethodGet, "/movie/999", nil))
	requireStatus(t, http.StatusNotFound, e.radarr(http.MethodGet, "/movie/abc", nil))
	requireStatus(t, http.StatusNotFound, e.radarr(http.MethodGet, "/movie/0", nil))
}

func TestUpdateMovie(t *testing.T) {
	e := newEnv(t)
	p, err := e.subs.SaveProfile(e.ctx, 0, subscribe.ProfileInput{Name: "4K"})
	require.NoError(t, err)
	sub, err := e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 157336}, models.MediaSubFromManual)
	require.NoError(t, err)
	tg := e.radarr(http.MethodPost, "/tag", map[string]any{"label": "a"}).obj(t)

	// 路径里带 id；不监控：暂停；改档案；换标签
	r := e.radarr(http.MethodPut, "/movie/"+formatID(sub.ID), map[string]any{
		"monitored": false, "qualityProfileId": p.ID, "tags": []any{tg["id"]},
	})
	requireStatus(t, http.StatusOK, r)
	m := r.obj(t)
	assert.Equal(t, false, m["monitored"])
	assert.Equal(t, float64(p.ID), m["qualityProfileId"])
	assert.Equal(t, []any{tg["id"]}, m["tags"])
	row := e.sub(models.MediaKindMovie, 157336, 0)
	assert.Equal(t, models.MediaSubPaused, row.Status)
	assert.Equal(t, p.ID, row.ProfileID)
	paused := *row.NextSearchAt

	// 不带 monitored、不带 tags：状态与标签不变；searchForMovie 只对在找的生效
	e.advance(time.Minute)
	r = e.radarr(http.MethodPut, "/movie", map[string]any{"id": sub.ID, "addOptions": map[string]any{"searchForMovie": true}})
	requireStatus(t, http.StatusOK, r)
	assert.Equal(t, []any{tg["id"]}, r.obj(t)["tags"])
	row = e.sub(models.MediaKindMovie, 157336, 0)
	assert.Equal(t, models.MediaSubPaused, row.Status)
	assert.True(t, row.NextSearchAt.Equal(paused))

	// 打开、不马上搜
	e.advance(time.Minute)
	requireStatus(t, http.StatusOK, e.radarr(http.MethodPut, "/movie", map[string]any{"id": sub.ID, "monitored": true, "qualityProfileId": p.ID}))
	row = e.sub(models.MediaKindMovie, 157336, 0)
	assert.Equal(t, models.MediaSubActive, row.Status)
	assert.True(t, row.NextSearchAt.Equal(e.Now().Add(subscribe.DefaultSearchIntervalHours*time.Hour)))
	// 在找的、只要搜：提前到现在
	requireStatus(t, http.StatusOK, e.radarr(http.MethodPut, "/movie", map[string]any{"id": sub.ID, "addOptions": map[string]any{"searchForMovie": true}}))
	assert.True(t, e.sub(models.MediaKindMovie, 157336, 0).NextSearchAt.Equal(e.Now()))

	// 错误：没有 id、没有这个订阅、不是电影、档案不存在、不是 JSON
	requireStatus(t, http.StatusNotFound, e.radarr(http.MethodPut, "/movie", map[string]any{"monitored": true}))
	requireStatus(t, http.StatusNotFound, e.radarr(http.MethodPut, "/movie", map[string]any{"id": 999}))
	requireStatus(t, http.StatusNotFound, e.radarr(http.MethodPut, "/movie/abc", map[string]any{}))
	tv, err := e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2}, models.MediaSubFromManual)
	require.NoError(t, err)
	requireStatus(t, http.StatusNotFound, e.radarr(http.MethodPut, "/movie", map[string]any{"id": tv.ID, "monitored": false}))
	assert.Equal(t, models.MediaSubActive, e.sub(models.MediaKindTV, 100088, 2).Status)
	requireStatus(t, http.StatusBadRequest, e.radarr(http.MethodPut, "/movie", map[string]any{"id": sub.ID, "qualityProfileId": 77}))
	requireStatus(t, http.StatusBadRequest, e.radarr(http.MethodPut, "/movie", "nope"))
	puts := e.audits("PUT /servarr/radarr/api/v3/movie")
	require.NotEmpty(t, puts)
	assert.Equal(t, "success", puts[0].Result)
}

func TestDeleteMovie(t *testing.T) {
	e := newEnv(t)
	sub, err := e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 157336}, models.MediaSubFromManual)
	require.NoError(t, err)
	tv, err := e.subs.CreateSubscription(e.ctx, subscribe.SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2}, models.MediaSubFromManual)
	require.NoError(t, err)
	tg := e.radarr(http.MethodPost, "/tag", map[string]any{"label": "a"}).obj(t)
	_, err = e.srv.setTags(e.ctx, tagKindMovie, 157336, []uint{uint(num(tg["id"]))})
	require.NoError(t, err)
	requireStatus(t, http.StatusNotFound, e.radarr(http.MethodDelete, "/movie/"+formatID(tv.ID), nil))
	assert.NotNil(t, e.sub(models.MediaKindTV, 100088, 2), "Radarr 不能删剧集")
	r := e.radarr(http.MethodDelete, "/movie/"+formatID(sub.ID)+"?deleteFiles=true&addImportExclusion=false", nil)
	requireStatus(t, http.StatusOK, r)
	assert.Equal(t, map[string]any{}, r.obj(t))
	assert.Nil(t, e.sub(models.MediaKindMovie, 157336, 0))
	assert.Equal(t, []uint{}, e.srv.tagsOf(e.ctx, tagKindMovie, 157336), "标签一起删：再请求时不带上旧的标签")
	requireStatus(t, http.StatusNotFound, e.radarr(http.MethodDelete, "/movie/"+formatID(sub.ID), nil))
	dels := e.audits("DELETE /servarr/radarr/api/v3/movie/" + formatID(sub.ID))
	require.Len(t, dels, 2)
	assert.Equal(t, "success", dels[0].Result)
	assert.Equal(t, "error:http_404", dels[1].Result)
}
