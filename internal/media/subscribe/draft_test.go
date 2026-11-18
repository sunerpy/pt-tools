package subscribe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// 葬送的芙莉莲（剧集 209867）是动画：TMDB 类型 16
const (
	frierenDetails = `{"id":209867,"name":"葬送的芙莉莲","original_name":"葬送のフリーレン","first_air_date":"2023-09-29","number_of_seasons":2,"poster_path":"/frieren.jpg","genres":[{"id":16,"name":"动画"}]}`
	frierenSeason2 = `{"season_number":2,"air_date":"2026-01-16","episodes":[{"episode_number":1,"name":"一","air_date":"2026-01-16"},{"episode_number":2,"name":"二","air_date":"2026-01-23"}]}`
)

func (e *env) libraries(libs ...models.MediaLibrary) []models.MediaLibrary {
	e.t.Helper()
	for i := range libs {
		require.NoError(e.t, e.db.Create(&libs[i]).Error)
	}
	return libs
}

// Draft：新建订阅前读条目，不写库。0 换成最新一季；「自动」的库按整理的选法；这一季在追时给订阅编号
func TestDraft(t *testing.T) {
	e := newEnv(t)
	e.tmdb.set("/3/tv/209867", frierenDetails)
	e.tmdb.set("/3/tv/209867/season/2", frierenSeason2)
	e.tmdb.set("/3/tv/100088/season/1", `{"season_number":1,"air_date":"2023-01-15","episodes":[{"episode_number":1,"name":"一","air_date":"2023-01-15"}]}`)

	// 没有媒体库时「自动」选不到库
	d, err := e.svc.Draft(e.ctx, models.MediaKindTV, 100088, 0)
	require.NoError(t, err)
	assert.Equal(t, SubscriptionDraft{
		MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Seasons: 2, TotalEpisodes: 3,
		Title: "最后生还者", OriginalTitle: "The Last of Us", Year: 2025, PosterPath: "/tlou.jpg",
	}, *d, "季 0 换成最新一季，年份是这一季的")

	libs := e.libraries(
		models.MediaLibrary{Name: "停用的剧集", Kind: models.MediaKindTV, Path: "/lib/off"},
		models.MediaLibrary{Name: "电影", Kind: models.MediaKindMovie, Path: "/lib/movies", Enabled: true},
		models.MediaLibrary{Name: "剧集", Kind: models.MediaKindTV, Path: "/lib/tv", Enabled: true},
		models.MediaLibrary{Name: "动漫", Kind: models.MediaKindTV, Anime: true, Path: "/lib/anime", Enabled: true},
	)
	off, movies, tv, anime := libs[0], libs[1], libs[2], libs[3]
	require.False(t, off.Enabled)

	d, err = e.svc.Draft(e.ctx, models.MediaKindTV, 100088, 0)
	require.NoError(t, err)
	assert.Equal(t, tv.ID, d.AutoLibraryID, "普通剧跳过停用的库与只收动画的库")
	assert.False(t, d.Anime)

	d, err = e.svc.Draft(e.ctx, models.MediaKindTV, 209867, 0)
	require.NoError(t, err)
	assert.True(t, d.Anime)
	assert.Equal(t, anime.ID, d.AutoLibraryID, "动画进只收动画的库")
	assert.Equal(t, 2, d.Season)
	assert.Equal(t, 2, d.TotalEpisodes)
	assert.Equal(t, "葬送のフリーレン", d.OriginalTitle)

	d, err = e.svc.Draft(e.ctx, models.MediaKindMovie, 693134, 3)
	require.NoError(t, err)
	assert.Equal(t, SubscriptionDraft{
		MediaType: models.MediaKindMovie, TMDBID: 693134, Title: "沙丘2", OriginalTitle: "Dune: Part Two", Year: 2024,
		PosterPath: "/dune2.jpg", AutoLibraryID: movies.ID,
	}, *d, "电影没有季：季与集数都是 0")

	// 这一季已经在追：给订阅编号；别的季照旧是 0
	sub := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	d, err = e.svc.Draft(e.ctx, models.MediaKindTV, 100088, 0)
	require.NoError(t, err)
	assert.Equal(t, sub.ID, d.SubscriptionID)
	d, err = e.svc.Draft(e.ctx, models.MediaKindTV, 100088, 1)
	require.NoError(t, err)
	assert.Zero(t, d.SubscriptionID)
	assert.Equal(t, 1, d.Season)
	assert.Equal(t, 2023, d.Year)

	// 没有动画库时动画进普通剧集库
	require.NoError(t, e.db.Model(&anime).Update("enabled", false).Error)
	d, err = e.svc.Draft(e.ctx, models.MediaKindTV, 209867, 2)
	require.NoError(t, err)
	assert.Equal(t, tv.ID, d.AutoLibraryID)

	var n int64
	require.NoError(t, e.db.Model(&models.MediaSubscription{}).Count(&n).Error)
	assert.Equal(t, int64(1), n, "Draft 不写库")

	for name, c := range map[string]struct {
		kind       string
		id, season int
	}{
		"TMDB 上没有这一季": {models.MediaKindTV, 100088, 5},
		"类型不对":        {"book", 100088, 0},
		"没有 TMDB 编号":  {models.MediaKindTV, 0, 0},
		"季号不对":        {models.MediaKindTV, 100088, 201},
		"负的季号":        {models.MediaKindTV, 100088, -1},
	} {
		_, err = e.svc.Draft(e.ctx, c.kind, c.id, c.season)
		require.ErrorIs(t, err, ErrInvalid, name)
	}
}
