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

func TestPersonChineseName(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/3/person/2037/translations":
			// 繁体在前也先取简体；英文的不要
			_, _ = w.Write([]byte(`{"translations":[{"iso_639_1":"zh","iso_3166_1":"TW","data":{"name":"席尼·墨菲"}},
				{"iso_639_1":"en","iso_3166_1":"US","data":{"name":"Cillian Murphy"}},
				{"iso_639_1":"zh","iso_3166_1":"cn","data":{"name":" 基里安·墨菲 "}},
				{"iso_639_1":"zh","iso_3166_1":"CN","data":{"name":"第二个简体名"}}]}`))
		case "/3/person/2/translations":
			_, _ = w.Write([]byte(`{"translations":[{"iso_639_1":"zh","iso_3166_1":"HK","data":{"name":"香港译名"}},
				{"iso_639_1":"zh","iso_3166_1":"SG","data":{"name":""}}]}`))
		case "/3/person/3/translations":
			_, _ = w.Write([]byte(`{"translations":[{"iso_639_1":"ja","iso_3166_1":"JP","data":{"name":"日本語"}}]}`))
		case "/3/person/5/translations":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := New(Options{APIKey: "k", BaseURL: srv.URL + "/3", RatePerSecond: 1000, Cache: NewDBCache(newDB(t))})
	require.NoError(t, err)
	ctx := context.Background()

	n, err := c.PersonChineseName(ctx, 2037)
	require.NoError(t, err)
	assert.Equal(t, "基里安·墨菲", n, "先取大陆的简体名（同一地区取第一个），去掉空白")
	n, err = c.PersonChineseName(ctx, 2037)
	require.NoError(t, err)
	assert.Equal(t, "基里安·墨菲", n)
	assert.Equal(t, int32(1), hits.Load(), "第二次读缓存")

	n, err = c.PersonChineseName(ctx, 2)
	require.NoError(t, err)
	assert.Equal(t, "香港译名", n, "没有简体时用繁体；空的名字不要")

	for _, id := range []int{3, 4, 0} {
		n, err = c.PersonChineseName(ctx, id)
		require.NoError(t, err, "没有中文名、TMDB 上没有这个人、编号无效都不是错误")
		assert.Empty(t, n)
	}
	before := hits.Load()
	_, err = c.PersonChineseName(ctx, 4)
	require.NoError(t, err)
	assert.Equal(t, before, hits.Load(), "TMDB 上没有的人也记进缓存")

	_, err = c.PersonChineseName(ctx, 5)
	assert.Error(t, err, "TMDB 出错时回错误")
}
