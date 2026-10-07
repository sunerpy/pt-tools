package tmdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

// fakeTMDB 按路径回 TMDB v3 的响应；记下每个请求的地址与鉴权头。
type fakeTMDB struct {
	srv      *httptest.Server
	hits     atomic.Int32
	lastURL  atomic.Value
	lastAuth atomic.Value
	status   int
}

func newFake(t *testing.T) *fakeTMDB {
	f := &fakeTMDB{status: http.StatusOK}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		f.lastURL.Store(r.URL.String())
		f.lastAuth.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		if f.status != http.StatusOK {
			w.WriteHeader(f.status)
			_, _ = w.Write([]byte(`{"status_code":7,"status_message":"Invalid API key: You must be granted a valid key.","success":false}`))
			return
		}
		lang := r.URL.Query().Get("language")
		switch r.URL.Path {
		case "/3/configuration":
			_, _ = w.Write([]byte(`{"images":{"secure_base_url":"https://image.tmdb.org/t/p/"}}`))
		case "/3/search/movie":
			_, _ = w.Write([]byte(`{"page":1,"results":[
				{"id":278,"title":"肖申克的救赎","original_title":"The Shawshank Redemption","release_date":"1994-09-23","overview":"银行家安迪……","poster_path":"/p.jpg","popularity":120.5,"vote_average":8.7,"original_language":"en"},
				{"id":1,"title":"Other","original_title":"Other","release_date":"","popularity":1}]}`))
		case "/3/search/tv":
			_, _ = w.Write([]byte(`{"page":1,"results":[{"id":1399,"name":"权力的游戏","original_name":"Game of Thrones","first_air_date":"2011-04-17","popularity":300}]}`))
		case "/3/find/tt0111161":
			_, _ = w.Write([]byte(`{"movie_results":[{"id":278,"title":"肖申克的救赎","original_title":"The Shawshank Redemption","release_date":"1994-09-23"}],"tv_results":[]}`))
		case "/3/movie/278":
			if lang == "en-US" {
				_, _ = w.Write([]byte(`{"id":278,"title":"The Shawshank Redemption","original_title":"The Shawshank Redemption","release_date":"1994-09-23","overview":"Imprisoned in the 1940s...","imdb_id":"tt0111161"}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":278,"title":"肖申克的救赎","original_title":"The Shawshank Redemption","release_date":"1994-09-23","overview":"","imdb_id":"tt0111161","poster_path":"/p.jpg"}`))
		case "/3/tv/1399":
			_, _ = w.Write([]byte(`{"id":1399,"name":"权力的游戏","original_name":"Game of Thrones","first_air_date":"2011-04-17","overview":"七大王国……","number_of_seasons":8,"external_ids":{"imdb_id":"tt0944947"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status_code":34,"status_message":"The resource you requested could not be found."}`))
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func newClient(t *testing.T, f *fakeTMDB, mod func(*Options)) *Client {
	t.Helper()
	o := Options{APIKey: "0123456789abcdef0123456789abcdef", BaseURL: f.srv.URL + "/3", RatePerSecond: 1000}
	if mod != nil {
		mod(&o)
	}
	c, err := New(o)
	require.NoError(t, err)
	return c
}

func TestNew(t *testing.T) {
	_, err := New(Options{})
	require.ErrorIs(t, err, ErrNoKey)
	_, err = New(Options{APIKey: "k", ProxyURL: "ftp://x"})
	assert.Error(t, err)
	c, err := New(Options{APIKey: "k"})
	require.NoError(t, err)
	assert.Equal(t, DefaultBaseURL, c.base)
	assert.Equal(t, "zh-CN", c.language)
}

func TestSearchAndAuth(t *testing.T) {
	f := newFake(t)
	c := newClient(t, f, nil)
	got, err := c.Search(context.Background(), KindMovie, "肖申克的救赎", 1994)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, Result{
		ID: 278, MediaType: KindMovie, Title: "肖申克的救赎", OriginalTitle: "The Shawshank Redemption", Year: 1994,
		Date: "1994-09-23", Overview: "银行家安迪……", PosterPath: "/p.jpg", Popularity: 120.5, VoteAverage: 8.7, OriginalLanguage: "en",
	}, got[0])
	u := f.lastURL.Load().(string)
	assert.Contains(t, u, "query=%E8%82%96")
	assert.Contains(t, u, "year=1994")
	assert.Contains(t, u, "language=zh-CN")
	assert.Contains(t, u, "include_adult=false")
	assert.Contains(t, u, "api_key=0123456789abcdef0123456789abcdef", "v3 的 API Key 放在查询参数里")

	tv, err := c.Search(context.Background(), KindTV, "Game of Thrones", 2019)
	require.NoError(t, err)
	require.Len(t, tv, 1)
	assert.Equal(t, KindTV, tv[0].MediaType)
	assert.Equal(t, "Game of Thrones", tv[0].OriginalTitle)
	assert.Equal(t, 2011, tv[0].Year)
	assert.NotContains(t, f.lastURL.Load().(string), "year", "剧集按首播年份过滤会漏掉后面几季，不带年份")

	// v4 的读取令牌（JWT）放在 Authorization 头里
	v4 := newClient(t, f, func(o *Options) { o.APIKey = "eyJhbGciOiJIUzI1NiJ9.payload.sig" })
	_, err = v4.Search(context.Background(), KindMovie, "x", 0)
	require.NoError(t, err)
	assert.Equal(t, "Bearer eyJhbGciOiJIUzI1NiJ9.payload.sig", f.lastAuth.Load())
	assert.NotContains(t, f.lastURL.Load().(string), "api_key")

	_, err = c.Search(context.Background(), "music", "x", 0)
	assert.Error(t, err)
}

func TestFindAndDetails(t *testing.T) {
	f := newFake(t)
	c := newClient(t, f, nil)
	found, err := c.Find(context.Background(), "https://www.imdb.com/title/tt0111161/")
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, 278, found[0].ID)
	assert.Equal(t, KindMovie, found[0].MediaType)
	assert.Contains(t, f.lastURL.Load().(string), "external_source=imdb_id")
	_, err = c.Find(context.Background(), "not-an-id")
	assert.Error(t, err)

	d, err := c.Details(context.Background(), KindMovie, 278)
	require.NoError(t, err)
	assert.Equal(t, "肖申克的救赎", d.Title)
	assert.Equal(t, "Imprisoned in the 1940s...", d.Overview, "中文简介为空时用英文的")
	assert.Equal(t, "tt0111161", d.IMDbID)

	tv, err := c.Details(context.Background(), KindTV, 1399)
	require.NoError(t, err)
	assert.Equal(t, 8, tv.Seasons)
	assert.Equal(t, "tt0944947", tv.IMDbID)
	assert.Contains(t, f.lastURL.Load().(string), "append_to_response=external_ids")

	_, err = c.Details(context.Background(), KindMovie, 999)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestErrorsHideKey(t *testing.T) {
	f := newFake(t)
	c := newClient(t, f, func(o *Options) { o.APIKey = "SECRETKEY0123456789" })
	f.status = http.StatusUnauthorized
	err := c.Validate(context.Background())
	require.ErrorIs(t, err, ErrUnauthorized)
	f.status = http.StatusTooManyRequests
	_, err = c.Search(context.Background(), KindMovie, "a", 0)
	require.ErrorIs(t, err, ErrRateLimited)
	f.status = http.StatusBadGateway
	_, err = c.Search(context.Background(), KindMovie, "b", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 502")
	assert.NotContains(t, err.Error(), "SECRETKEY")

	f.status = http.StatusOK
	require.NoError(t, c.Validate(context.Background()))

	f.srv.Close()
	_, err = c.Search(context.Background(), KindMovie, "c", 0)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "SECRETKEY", "连接失败的错误里不带地址")
}

func TestCache(t *testing.T) {
	f := newFake(t)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.MediaCache{}))
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	cache := NewDBCache(db)
	cache.now = func() time.Time { return now }
	c := newClient(t, f, func(o *Options) { o.Cache = cache })

	for range 3 {
		_, searchErr := c.Search(context.Background(), KindMovie, "肖申克的救赎", 1994)
		require.NoError(t, searchErr)
	}
	assert.EqualValues(t, 1, f.hits.Load(), "同样的搜索读缓存")
	_, err = c.Search(context.Background(), KindMovie, "肖申克的救赎", 1995)
	require.NoError(t, err)
	assert.EqualValues(t, 2, f.hits.Load(), "参数不同不共用缓存")

	now = now.Add(SearchTTL + time.Minute)
	_, err = c.Search(context.Background(), KindMovie, "肖申克的救赎", 1994)
	require.NoError(t, err)
	assert.EqualValues(t, 3, f.hits.Load(), "过期后重新请求")

	// 出错的响应不进缓存
	f.status = http.StatusBadGateway
	_, err = c.Search(context.Background(), KindMovie, "新的", 0)
	require.Error(t, err)
	f.status = http.StatusOK
	_, err = c.Search(context.Background(), KindMovie, "新的", 0)
	require.NoError(t, err)

	var n int64
	require.NoError(t, db.Model(&models.MediaCache{}).Count(&n).Error)
	assert.Positive(t, n)
	now = now.Add(30 * 24 * time.Hour)
	require.NoError(t, cache.Purge())
	require.NoError(t, db.Model(&models.MediaCache{}).Count(&n).Error)
	assert.Zero(t, n, "清掉过期的")
}

func TestRateLimit(t *testing.T) {
	f := newFake(t)
	c := newClient(t, f, func(o *Options) { o.RatePerSecond = 0.5 })
	_, err := c.Search(context.Background(), KindMovie, "a", 0)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = c.Search(ctx, KindMovie, "b", 0)
	require.Error(t, err, "限速内等不到下一次请求的配额")
	assert.EqualValues(t, 1, f.hits.Load())
}

// 代理：设置里填了代理时经代理访问 TMDB。
func TestProxy(t *testing.T) {
	var host atomic.Value
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host.Store(r.URL.Host)
		_, _ = w.Write([]byte(`{"images":{}}`))
	}))
	defer proxy.Close()
	c, err := New(Options{APIKey: "k", BaseURL: "http://tmdb.invalid/3", ProxyURL: proxy.URL, RatePerSecond: 100})
	require.NoError(t, err)
	require.NoError(t, c.Validate(context.Background()))
	assert.Equal(t, "tmdb.invalid", host.Load())
}

func TestPosterURL(t *testing.T) {
	assert.Equal(t, "https://image.tmdb.org/t/p/w342/p.jpg", PosterURL("/p.jpg", "w342"))
	assert.Empty(t, PosterURL("", "w342"))
	assert.True(t, strings.HasSuffix(PosterURL("p.jpg", ""), "/w342/p.jpg"))
}
