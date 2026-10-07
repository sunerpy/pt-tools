package tmdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

func TestDetailsGenresBackdropRuntime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/3/movie/129":
			_, _ = w.Write([]byte(`{"id":129,"title":"千与千寻","original_title":"千と千尋の神隠し","release_date":"2001-07-20","overview":"……",
				"backdrop_path":"/b.jpg","runtime":125,"genres":[{"id":16,"name":"动画"},{"id":10751,"name":"家庭"}]}`))
		case "/3/tv/1399":
			_, _ = w.Write([]byte(`{"id":1399,"name":"权力的游戏","first_air_date":"2011-04-17","overview":"……","episode_run_time":[60,55],"genres":[{"id":18,"name":"剧情"}]}`))
		case "/3/search/tv":
			_, _ = w.Write([]byte(`{"results":[{"id":1,"name":"A","genre_ids":[16,35],"backdrop_path":"/s.jpg"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := New(Options{APIKey: "k", BaseURL: srv.URL + "/3", RatePerSecond: 1000})
	require.NoError(t, err)
	ctx := context.Background()

	m, err := c.Details(ctx, KindMovie, 129)
	require.NoError(t, err)
	assert.Equal(t, "/b.jpg", m.BackdropPath)
	assert.Equal(t, 125, m.Runtime)
	assert.Equal(t, []int{16, 10751}, m.GenreIDs)
	assert.Equal(t, []string{"动画", "家庭"}, m.Genres)
	assert.True(t, m.IsAnimation())

	tv, err := c.Details(ctx, KindTV, 1399)
	require.NoError(t, err)
	assert.Equal(t, 60, tv.Runtime, "剧集取 episode_run_time 的第一个")
	assert.False(t, tv.IsAnimation())

	found, err := c.Search(ctx, KindTV, "A", 0)
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, []int{16, 35}, found[0].GenreIDs, "搜索结果带 genre_ids")
	assert.True(t, found[0].IsAnimation())
	assert.Equal(t, "/s.jpg", found[0].BackdropPath)
}

func TestSeason(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/3/tv/1399/season/1" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		assert.Equal(t, "zh-CN", r.URL.Query().Get("language"))
		_, _ = w.Write([]byte(`{"season_number":1,"name":"第 1 季","overview":"第一季","air_date":"2011-04-17","poster_path":"/s1.jpg",
			"episodes":[{"episode_number":1,"name":"凛冬将至","overview":"……","air_date":"2011-04-17","still_path":"/e1.jpg","runtime":62},
			{"episode_number":2,"name":"国王大道","air_date":"2011-04-24"}]}`))
	}))
	t.Cleanup(srv.Close)
	c, err := New(Options{APIKey: "k", BaseURL: srv.URL + "/3", RatePerSecond: 1000, Cache: NewDBCache(newDB(t))})
	require.NoError(t, err)
	ctx := context.Background()

	s, err := c.Season(ctx, 1399, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, s.Number)
	assert.Equal(t, "/s1.jpg", s.PosterPath)
	require.Len(t, s.Episodes, 2)
	e := s.Episode(1)
	require.NotNil(t, e)
	assert.Equal(t, "凛冬将至", e.Name)
	assert.Equal(t, "/e1.jpg", e.StillPath)
	assert.Equal(t, 62, e.Runtime)
	assert.Nil(t, s.Episode(9))
	assert.Nil(t, (*Season)(nil).Episode(1))

	_, err = c.Season(ctx, 1399, 1)
	require.NoError(t, err)
	assert.Equal(t, int32(1), hits.Load(), "第二次读缓存")

	_, err = c.Season(ctx, 1399, 7)
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = c.Season(ctx, 0, 1)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestImage(t *testing.T) {
	var lastPath atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastPath.Store(r.URL.Path)
		assert.Empty(t, r.URL.RawQuery, "图片请求不带 API Key")
		switch r.URL.Path {
		case "/t/p/w780/p.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("JPEG"))
		case "/t/p/w780/html.jpg":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>"))
		case "/t/p/original/big.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte(strings.Repeat("x", maxImage+1)))
		case "/t/p/w780/err.jpg":
			w.WriteHeader(http.StatusBadGateway)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := New(Options{APIKey: "k", ImageBaseURL: srv.URL + "/t/p/"})
	require.NoError(t, err)
	ctx := context.Background()

	b, err := c.Image(ctx, "/p.jpg", SizePoster)
	require.NoError(t, err)
	assert.Equal(t, "JPEG", string(b))
	assert.Equal(t, "/t/p/w780/p.jpg", lastPath.Load())

	_, err = c.Image(ctx, "/html.jpg", SizePoster)
	require.ErrorIs(t, err, ErrNotImage)
	_, err = c.Image(ctx, "/big.png", "original")
	require.ErrorContains(t, err, "10 MiB")
	_, err = c.Image(ctx, "/none.jpg", SizePoster)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = c.Image(ctx, "/err.jpg", SizePoster)
	require.ErrorIs(t, err, ErrUnavailable)

	for _, bad := range []string{"", "p.jpg", "/../x.jpg", "/a/b.jpg", "/p.jpg?x=1", "/p.exe"} {
		_, err = c.Image(ctx, bad, SizePoster)
		require.Error(t, err, bad)
	}
	_, err = c.Image(ctx, "/p.jpg", "w780/../x")
	require.Error(t, err)

	def, err := New(Options{APIKey: "k"})
	require.NoError(t, err)
	assert.Equal(t, ImageBaseURL, def.imageBase, "默认用 TMDB 的图片地址")
}

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.MediaCache{}))
	return db
}

func TestTitles(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		assert.Equal(t, "en-US", r.URL.Query().Get("language"))
		assert.Equal(t, "alternative_titles", r.URL.Query().Get("append_to_response"))
		switch r.URL.Path {
		case "/3/movie/372058":
			_, _ = w.Write([]byte(`{"id":372058,"title":"Your Name.","alternative_titles":{"titles":[{"iso_3166_1":"JP","title":"Kimi no Na wa."},{"title":"Your Name."},{"title":" "}]}}`))
		case "/3/tv/209867":
			_, _ = w.Write([]byte(`{"id":209867,"name":"Frieren: Beyond Journey's End","alternative_titles":{"results":[{"iso_3166_1":"JP","title":"Sousou no Frieren","type":"romaji"}]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := New(Options{APIKey: "k", BaseURL: srv.URL + "/3", RatePerSecond: 1000, Cache: NewDBCache(newDB(t))})
	require.NoError(t, err)
	ctx := context.Background()

	got, err := c.Titles(ctx, KindMovie, 372058)
	require.NoError(t, err)
	assert.Equal(t, []string{"Your Name.", "Kimi no Na wa."}, got, "电影的别名在 titles 里；去掉重复与空的")
	got, err = c.Titles(ctx, KindTV, 209867)
	require.NoError(t, err)
	assert.Equal(t, []string{"Frieren: Beyond Journey's End", "Sousou no Frieren"}, got, "剧集的别名在 results 里")
	_, err = c.Titles(ctx, KindTV, 209867)
	require.NoError(t, err)
	assert.Equal(t, int32(2), hits.Load(), "读缓存")

	_, err = c.Titles(ctx, KindMovie, 1)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = c.Titles(ctx, "music", 1)
	require.Error(t, err)
	_, err = c.Titles(ctx, KindMovie, 0)
	require.ErrorIs(t, err, ErrNotFound)
}
