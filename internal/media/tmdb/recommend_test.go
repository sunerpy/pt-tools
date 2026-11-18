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

func TestRecommendations(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/3/tv/1399/recommendations":
			assert.Equal(t, "zh-CN", r.URL.Query().Get("language"))
			assert.Equal(t, "1", r.URL.Query().Get("page"))
			_, _ = w.Write([]byte(`{"page":1,"results":[{"id":2,"name":"三体","first_air_date":"2023-01-15","poster_path":"/p.jpg","media_type":"tv"},
				{"id":3,"name":"漫长的季节","first_air_date":"2023-04-22"}]}`))
		case "/3/movie/129/recommendations":
			_, _ = w.Write([]byte(`{"page":1,"results":[{"id":4,"title":"龙猫","release_date":"1988-04-16"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := New(Options{APIKey: "k", BaseURL: srv.URL + "/3", RatePerSecond: 1000, Cache: NewDBCache(newDB(t))})
	require.NoError(t, err)
	ctx := context.Background()

	tv, err := c.Recommendations(ctx, KindTV, 1399)
	require.NoError(t, err)
	require.Len(t, tv, 2)
	assert.Equal(t, "三体", tv[0].Title)
	assert.Equal(t, 2023, tv[0].Year)
	assert.Equal(t, KindTV, tv[1].MediaType, "推荐和这个条目同类")
	_, err = c.Recommendations(ctx, KindTV, 1399)
	require.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load(), "第二次读缓存")

	mv, err := c.Recommendations(ctx, KindMovie, 129)
	require.NoError(t, err)
	require.Len(t, mv, 1)
	assert.Equal(t, "龙猫", mv[0].Title)

	_, err = c.Recommendations(ctx, KindTV, 5)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = c.Recommendations(ctx, "person", 1)
	require.Error(t, err)
	_, err = c.Recommendations(ctx, KindMovie, 0)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestSeasons(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/3/tv/1399" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		assert.Equal(t, "zh-CN", r.URL.Query().Get("language"))
		_, _ = w.Write([]byte(`{"id":1399,"name":"权力的游戏","number_of_seasons":2,"number_of_episodes":20,"seasons":[
			{"season_number":0,"name":"特别篇","episode_count":3,"air_date":"2010-12-05","poster_path":"/s0.jpg"},
			{"season_number":1,"name":"第 1 季","episode_count":10,"air_date":"2011-04-17","poster_path":"/s1.jpg"},
			{"season_number":2,"name":"第 2 季","episode_count":10,"air_date":"2012-04-01","poster_path":"bad path"}]}`))
	}))
	t.Cleanup(srv.Close)
	c, err := New(Options{APIKey: "k", BaseURL: srv.URL + "/3", RatePerSecond: 1000, Cache: NewDBCache(newDB(t))})
	require.NoError(t, err)
	ctx := context.Background()

	ss, err := c.Seasons(ctx, 1399)
	require.NoError(t, err)
	require.Len(t, ss, 3)
	assert.Equal(t, SeasonSummary{Number: 1, Name: "第 1 季", EpisodeCount: 10, AirDate: "2011-04-17", PosterPath: "/s1.jpg"}, ss[1])
	assert.Empty(t, ss[2].PosterPath, "格式不对的图片路径不要")
	_, err = c.Seasons(ctx, 1399)
	require.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load(), "第二次读缓存")

	_, err = c.Seasons(ctx, 7)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = c.Seasons(ctx, 0)
	require.ErrorIs(t, err, ErrNotFound)
}
