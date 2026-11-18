package omdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/cachectl"
)

type memCache struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (c *memCache) Get(k string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[k]
	return v, ok
}

func (c *memCache) Set(k string, v []byte, _ time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[k] = v
}

func TestByIMDb(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		q := r.URL.Query()
		switch q.Get("apikey") {
		case "good":
		case "limited":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"Response":"False","Error":"Request limit reached!"}`))
			return
		default:
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"Response":"False","Error":"Invalid API key!"}`))
			return
		}
		switch q.Get("i") {
		case "tt15398776":
			_, _ = w.Write([]byte(`{"Title":"Oppenheimer","Rated":"R","imdbRating":"8.3","imdbVotes":"1,012,345","Metascore":"90","imdbID":"tt15398776",
				"Ratings":[{"Source":"Internet Movie Database","Value":"8.3/10"},{"Source":"Rotten Tomatoes","Value":"93%"},{"Source":"Metacritic","Value":"90/100"}],
				"Response":"True"}`))
		case "tt0000001":
			_, _ = w.Write([]byte(`{"Title":"Old","Rated":"N/A","imdbRating":"N/A","imdbVotes":"N/A","Metascore":"N/A","imdbID":"tt0000001","Response":"True"}`))
		case "tt0137523":
			_, _ = w.Write([]byte(`{"Title":"Fight Club","Response":"True"}`))
		default:
			_, _ = w.Write([]byte(`{"Response":"False","Error":"Incorrect IMDb ID."}`))
		}
	}))
	t.Cleanup(srv.Close)
	ctx := context.Background()
	c, err := New(Options{APIKey: "good", BaseURL: srv.URL + "/", Cache: &memCache{m: map[string][]byte{}}})
	require.NoError(t, err)

	got, err := c.ByIMDb(ctx, "tt15398776")
	require.NoError(t, err)
	assert.Equal(t, &Title{IMDbID: "tt15398776", Title: "Oppenheimer", Rated: "R", IMDbRating: 8.3, IMDbVotes: 1012345, RottenTomatoes: 93, Metacritic: 90}, got)

	old, err := c.ByIMDb(ctx, "tt0000001")
	require.NoError(t, err)
	assert.Equal(t, &Title{IMDbID: "tt0000001", Title: "Old"}, old, "N/A 都当作没有")

	none, err := c.ByIMDb(ctx, "tt9999999")
	require.NoError(t, err)
	assert.Nil(t, none)
	before := hits.Load()
	_, _ = c.ByIMDb(ctx, "tt9999999")
	_, _ = c.ByIMDb(ctx, "tt15398776")
	assert.Equal(t, before, hits.Load(), "没有的与有的都读缓存")
	_, err = c.ByIMDb(cachectl.Bypass(ctx), "tt15398776")
	require.NoError(t, err)
	assert.Equal(t, before+1, hits.Load(), "重新刮削时不读缓存")

	_, err = c.ByIMDb(ctx, "123")
	assert.Error(t, err)
	require.NoError(t, c.Validate(ctx))

	bad, err := New(Options{APIKey: "bad", BaseURL: srv.URL + "/"})
	require.NoError(t, err)
	_, err = bad.ByIMDb(ctx, "tt15398776")
	assert.ErrorIs(t, err, ErrUnauthorized)
	assert.NotContains(t, err.Error(), "apikey")
	assert.ErrorIs(t, bad.Validate(ctx), ErrUnauthorized)

	limited, err := New(Options{APIKey: "limited", BaseURL: srv.URL + "/"})
	require.NoError(t, err)
	_, err = limited.ByIMDb(ctx, "tt15398776")
	assert.ErrorIs(t, err, ErrRateLimited)

	_, err = New(Options{})
	assert.ErrorIs(t, err, ErrNoKey)
}

// Key 在地址里：跳到别的主机时不跟，Key 不会随跳转与 Referer 发到那里
func TestRedirectToOtherHost(t *testing.T) {
	var other atomic.Int32
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		other.Add(1)
	}))
	t.Cleanup(evil.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, evil.URL+"/?i=tt15398776", http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	c, err := New(Options{APIKey: "secretkey1", BaseURL: srv.URL + "/"})
	require.NoError(t, err)
	_, err = c.ByIMDb(context.Background(), "tt15398776")
	assert.ErrorContains(t, err, "跳转到了别的地址")
	assert.NotContains(t, err.Error(), "secretkey1")
	assert.Zero(t, other.Load(), "没有请求到跳转的地址")
}
