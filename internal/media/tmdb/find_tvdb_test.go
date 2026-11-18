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

// 按 TVDB 编号找剧集：只看剧集的结果，缓存；找不到、编号不对时是 ErrNotFound（编号不对不发请求）。
func TestFindByTVDB(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		assert.Equal(t, "tvdb_id", r.URL.Query().Get("external_source"))
		switch r.URL.Path {
		case "/3/find/392256":
			_, _ = w.Write([]byte(`{"movie_results":[{"id":1,"title":"别的"}],"tv_results":[{"id":100088,"name":"最后生还者","original_name":"The Last of Us","first_air_date":"2023-01-15"}]}`))
		default:
			_, _ = w.Write([]byte(`{"movie_results":[],"tv_results":[]}`))
		}
	}))
	t.Cleanup(srv.Close)
	c, err := New(Options{APIKey: "k", BaseURL: srv.URL + "/3", RatePerSecond: 1000, Cache: NewDBCache(newDB(t))})
	require.NoError(t, err)
	ctx := context.Background()

	got, err := c.FindByTVDB(ctx, 392256)
	require.NoError(t, err)
	assert.Equal(t, 100088, got.ID)
	assert.Equal(t, KindTV, got.MediaType)
	assert.Equal(t, "最后生还者", got.Title)
	assert.Equal(t, 2023, got.Year)
	_, err = c.FindByTVDB(ctx, 392256)
	require.NoError(t, err)
	assert.EqualValues(t, 1, hits.Load(), "第二次读缓存")

	_, err = c.FindByTVDB(ctx, 1)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = c.FindByTVDB(ctx, 0)
	require.ErrorIs(t, err, ErrNotFound)
	assert.EqualValues(t, 2, hits.Load(), "编号是 0 时不发请求")
}
