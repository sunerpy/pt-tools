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
