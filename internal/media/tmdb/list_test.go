package tmdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestList(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/3/trending/movie/week":
			assert.Equal(t, "2", r.URL.Query().Get("page"))
			assert.Equal(t, "zh-CN", r.URL.Query().Get("language"))
			_, _ = w.Write([]byte(`{"page":2,"total_pages":500,"results":[{"id":1,"title":"沙丘2","release_date":"2024-02-27","poster_path":"/p.jpg"}]}`))
		case "/3/tv/popular":
			_, _ = w.Write([]byte(`{"page":1,"total_pages":3,"results":[{"id":2,"name":"三体","first_air_date":"2023-01-15"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := New(Options{APIKey: "k", BaseURL: srv.URL + "/3", RatePerSecond: 1000, Cache: NewDBCache(newDB(t))})
	require.NoError(t, err)
	ctx := context.Background()

	p, err := c.List(ctx, KindMovie, ListTrending, 2)
	require.NoError(t, err)
	assert.Equal(t, 2, p.Page)
	assert.Equal(t, 20, p.TotalPages, "最多 20 页")
	require.Len(t, p.Results, 1)
	assert.Equal(t, "沙丘2", p.Results[0].Title)
	assert.Equal(t, 2024, p.Results[0].Year)
	assert.Equal(t, KindMovie, p.Results[0].MediaType)

	_, err = c.List(ctx, KindMovie, ListTrending, 2)
	require.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load(), "第二次读缓存")

	tv, err := c.List(ctx, KindTV, ListPopular, 0)
	require.NoError(t, err)
	require.Len(t, tv.Results, 1)
	assert.Equal(t, "三体", tv.Results[0].Title)
	assert.Equal(t, KindTV, tv.Results[0].MediaType)

	_, err = c.List(ctx, KindTV, "upcoming", 1)
	require.Error(t, err)
	_, err = c.List(ctx, "person", ListPopular, 1)
	require.Error(t, err)
}

// 新的列表：电影的正在上映、即将上映，剧集的正在播出、今天播出（按服务器的时区），两边都有高分；别的类型没有的列表报错。
func TestListMore(t *testing.T) {
	var paths []string
	var tz string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/3/tv/airing_today" {
			tz = r.URL.Query().Get("timezone")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"page":1,"total_pages":1,"results":[{"id":3,"name":"x","title":"x"}]}`))
	}))
	t.Cleanup(srv.Close)
	c, err := New(Options{APIKey: "k", BaseURL: srv.URL + "/3", RatePerSecond: 1000})
	require.NoError(t, err)
	ctx := context.Background()
	zone := localZone
	t.Cleanup(func() { localZone = zone })
	localZone = func() string { return "Asia/Shanghai" }
	for _, c2 := range []struct{ kind, list string }{
		{KindMovie, ListNowPlaying},
		{KindMovie, ListUpcoming},
		{KindMovie, ListTopRated},
		{KindTV, ListOnTheAir},
		{KindTV, ListAiringToday},
		{KindTV, ListTopRated},
	} {
		assert.True(t, HasList(c2.kind, c2.list), c2)
		_, err := c.List(ctx, c2.kind, c2.list, 1)
		require.NoError(t, err, c2)
	}
	assert.Equal(t, []string{
		"/3/movie/now_playing", "/3/movie/upcoming", "/3/movie/top_rated", "/3/tv/on_the_air", "/3/tv/airing_today", "/3/tv/top_rated",
	}, paths)
	assert.Equal(t, "Asia/Shanghai", tz, "今天播出按服务器的时区")

	localZone = func() string { return "Local" }
	_, err = c.List(ctx, KindTV, ListAiringToday, 2)
	require.NoError(t, err)
	assert.Empty(t, tz, "不知道时区名时不传")

	assert.False(t, HasList(KindTV, ListNowPlaying))
	assert.False(t, HasList(KindMovie, ListAiringToday))
	assert.False(t, HasList("person", ListTrending))
	_, err = c.List(ctx, KindMovie, ListOnTheAir, 1)
	require.Error(t, err)
}

// 按条件发现：按热度排、不含成人内容，出品国家与类型照传；写错的报错。
func TestDiscover(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.URL.Path+"?"+r.URL.Query().Encode())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"page":3,"total_pages":40,"results":[{"id":7,"name":"向往的生活","first_air_date":"2017-01-15","genre_ids":[10764]}]}`))
	}))
	t.Cleanup(srv.Close)
	c, err := New(Options{APIKey: "k", BaseURL: srv.URL + "/3", RatePerSecond: 1000})
	require.NoError(t, err)
	ctx := context.Background()
	p, err := c.Discover(ctx, KindTV, DiscoverFilter{Country: "CN", Genres: "10764|10767"}, 3)
	require.NoError(t, err)
	assert.Equal(t, 3, p.Page)
	assert.Equal(t, 20, p.TotalPages)
	require.Len(t, p.Results, 1)
	assert.Equal(t, "向往的生活", p.Results[0].Title)
	assert.Equal(t, KindTV, p.Results[0].MediaType)
	_, err = c.Discover(ctx, KindMovie, DiscoverFilter{}, 0)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "/3/discover/tv?api_key=k&include_adult=false&language=zh-CN&page=3&sort_by=popularity.desc&with_genres=10764%7C10767&with_origin_country=CN", got[0])
	assert.Equal(t, "/3/discover/movie?api_key=k&include_adult=false&include_video=false&language=zh-CN&page=1&sort_by=popularity.desc", got[1])

	for _, f := range []DiscoverFilter{{Country: "cn"}, {Country: "CHN"}, {Genres: "16,99"}, {Genres: "abc"}} {
		_, err = c.Discover(ctx, KindTV, f, 1)
		require.Error(t, err, f)
	}
	_, err = c.Discover(ctx, "person", DiscoverFilter{}, 1)
	require.Error(t, err)
}
