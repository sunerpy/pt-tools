package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

type recorded struct {
	Method, Path, Query, Body string
	Header                    http.Header
}

type fakeServer struct {
	*httptest.Server
	mu   sync.Mutex
	reqs []recorded
}

func (f *fakeServer) requests() []recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recorded(nil), f.reqs...)
}

func newFake(t *testing.T, h func(w http.ResponseWriter, r *http.Request)) *fakeServer {
	f := &fakeServer{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.reqs = append(f.reqs, recorded{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Body: string(b), Header: r.Header.Clone()})
		f.mu.Unlock()
		h(w, r)
	}))
	t.Cleanup(f.Close)
	return f
}

// fakeEmby 模拟 Emby / Jellyfin：checkAuth 判断鉴权头对不对。
func fakeEmby(t *testing.T, checkAuth func(http.Header) bool) *fakeServer {
	return newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(r.Header) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /System/Info":
			_, _ = w.Write([]byte(`{"ServerName":"家里的 Emby","Version":"4.8.10.0","Id":"x"}`))
		case "POST /Library/Media/Updated", "POST /Library/Refresh":
			w.WriteHeader(http.StatusNoContent)
		case "GET /Items":
			switch r.URL.Query().Get("SearchTerm") {
			case "奥本海默":
				_, _ = w.Write([]byte(`{"Items":[{"Name":"奥本海默","ProviderIds":{"Tmdb":"872585","Imdb":"tt15398776"}}],"TotalRecordCount":1}`))
			case "最后生还者":
				_, _ = w.Write([]byte(`{"Items":[{"Name":"最后生还者","ProviderIds":{"tmdb":"100088"}}],"TotalRecordCount":1}`))
			default:
				_, _ = w.Write([]byte(`{"Items":[],"TotalRecordCount":0}`))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func TestEmby(t *testing.T) {
	ctx := context.Background()
	f := fakeEmby(t, func(h http.Header) bool { return h.Get("X-Emby-Token") == "embykey" })
	c, err := New(Config{Kind: models.MediaServerEmby, URL: f.URL + "/", Token: "embykey"})
	require.NoError(t, err)

	info, err := c.Test(ctx)
	require.NoError(t, err)
	assert.Equal(t, Info{Name: "家里的 Emby", Version: "4.8.10.0"}, info)

	require.NoError(t, c.RefreshPaths(ctx, []string{"/data/movies/奥本海默 (2023)", "/data/tv/最后生还者 (2023)"}))
	require.NoError(t, c.RefreshPaths(ctx, nil), "没有目录时不发请求")
	require.NoError(t, c.RefreshAll(ctx))

	ok, err := c.Exists(ctx, Query{Kind: "movie", TMDBID: 872585, Title: "奥本海默"})
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = c.Exists(ctx, Query{Kind: "movie", IMDbID: "TT15398776", Title: "奥本海默"})
	require.NoError(t, err)
	assert.True(t, ok, "IMDb 编号不分大小写")
	ok, err = c.Exists(ctx, Query{Kind: "movie", TMDBID: 1, Title: "奥本海默"})
	require.NoError(t, err)
	assert.False(t, ok, "名字对上、编号对不上不算")
	ok, err = c.Exists(ctx, Query{Kind: "tv", TMDBID: 100088, Title: "最后生还者"})
	require.NoError(t, err)
	assert.True(t, ok)
	_, err = c.Exists(ctx, Query{Kind: "tv", TMDBID: 1})
	require.Error(t, err)

	reqs := f.requests()
	require.GreaterOrEqual(t, len(reqs), 4)
	upd := reqs[1]
	assert.Equal(t, "POST", upd.Method)
	assert.Equal(t, "/Library/Media/Updated", upd.Path)
	var body struct {
		Updates []embyUpdate `json:"Updates"`
	}
	require.NoError(t, json.Unmarshal([]byte(upd.Body), &body))
	assert.Equal(t, []embyUpdate{{Path: "/data/movies/奥本海默 (2023)", UpdateType: "Created"}, {Path: "/data/tv/最后生还者 (2023)", UpdateType: "Created"}}, body.Updates)
	assert.Equal(t, "application/json", upd.Header.Get("Content-Type"))
	assert.Equal(t, "/Library/Refresh", reqs[2].Path)
	assert.Contains(t, reqs[3].Query, "IncludeItemTypes=Movie")
	assert.Contains(t, reqs[3].Query, "Fields=ProviderIds")
	for _, r := range reqs {
		assert.NotContains(t, r.Query, "embykey", "Token 只放在请求头里")
	}

	bad, err := New(Config{Kind: models.MediaServerEmby, URL: f.URL, Token: "wrong"})
	require.NoError(t, err)
	_, err = bad.Test(ctx)
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestJellyfinAuthHeader(t *testing.T) {
	f := fakeEmby(t, func(h http.Header) bool {
		return h.Get("X-Emby-Token") == "" && strings.HasPrefix(h.Get("Authorization"), `MediaBrowser Token="jfkey"`)
	})
	c, err := New(Config{Kind: models.MediaServerJellyfin, URL: f.URL, Token: "jfkey"})
	require.NoError(t, err)
	_, err = c.Test(context.Background())
	require.NoError(t, err, "Jellyfin 用 Authorization: MediaBrowser Token")
	require.NoError(t, c.RefreshPaths(context.Background(), []string{"/media/tv/x"}))
}

func TestEmbyWrongService(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/System/Info" {
			_, _ = w.Write([]byte(`<html>login</html>`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})
	c, err := New(Config{Kind: models.MediaServerEmby, URL: f.URL, Token: "k"})
	require.NoError(t, err)
	_, err = c.Test(context.Background())
	require.ErrorIs(t, err, ErrUnavailable)
	assert.Contains(t, err.Error(), "不是预期的 JSON")

	c2, _ := New(Config{Kind: models.MediaServerJellyfin, URL: f.URL + "/x", Token: "k"})
	_, err = c2.Test(context.Background())
	require.ErrorIs(t, err, ErrUnavailable, "空的 JSON 也不算")
}

func TestRedirectNotFollowed(t *testing.T) {
	target := newFake(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"ServerName":"x"}`)) })
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusFound)
	})
	c, err := New(Config{Kind: models.MediaServerEmby, URL: f.URL, Token: "secret"})
	require.NoError(t, err)
	_, err = c.Test(context.Background())
	require.ErrorIs(t, err, ErrRedirect)
	assert.Empty(t, target.requests(), "Token 不会跟着跳转发出去")
}

func TestUnavailable(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadGateway) })
	c, _ := New(Config{Kind: models.MediaServerEmby, URL: f.URL, Token: "k"})
	_, err := c.Test(context.Background())
	require.ErrorIs(t, err, ErrUnavailable)
	assert.Contains(t, err.Error(), "502")

	f.Close()
	_, err = c.Test(context.Background())
	require.ErrorIs(t, err, ErrUnavailable)
}

func TestNewValidation(t *testing.T) {
	for _, tc := range []struct {
		cfg  Config
		want string
	}{
		{Config{Kind: "emby", URL: "ftp://x", Token: "k"}, "http://"},
		{Config{Kind: "emby", URL: "http://", Token: "k"}, "http://"},
		{Config{Kind: "emby", URL: "http://x?a=1", Token: "k"}, "查询参数"},
		{Config{Kind: "emby", URL: "http://u:p@x", Token: "k"}, "用户名"},
		{Config{Kind: "emby", URL: "http://x", Token: " "}, "没有填写"},
		{Config{Kind: "emby", URL: "http://x", Token: `a"b`}, "引号"},
		{Config{Kind: "kodi", URL: "http://x", Token: "k"}, "kodi"},
	} {
		_, err := New(tc.cfg)
		require.ErrorIs(t, err, ErrBadConfig, tc.cfg)
		assert.Contains(t, err.Error(), tc.want)
	}
	u, err := CheckURL(" https://emby.example.com:8920/emby/ ")
	require.NoError(t, err)
	assert.Equal(t, "https://emby.example.com:8920/emby", u)
}

func fakePlex(t *testing.T) *fakeServer {
	return newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != "plextoken" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`{"MediaContainer":{"friendlyName":"NAS Plex","version":"1.41.0.8992"}}`))
		case "/library/sections":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[
				{"key":"1","type":"movie","title":"电影","Location":[{"path":"/data/movies"}]},
				{"key":"2","type":"show","title":"剧集","Location":[{"path":"/data/tv"},{"path":"/data/tv/anime"}]},
				{"key":"3","type":"show","title":"动漫","Location":[{"path":"/data/tv/anime"}]}]}}`))
		case "/library/sections/1/refresh", "/library/sections/2/refresh", "/library/sections/3/refresh":
			w.WriteHeader(http.StatusOK)
		case "/library/sections/1/all":
			if r.URL.Query().Get("title") == "奥本海默" && r.URL.Query().Get("includeGuids") == "1" {
				_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"title":"奥本海默","Guid":[{"id":"imdb://tt15398776"},{"id":"tmdb://872585"}]}]}}`))
				return
			}
			_, _ = w.Write([]byte(`{"MediaContainer":{"size":0}}`))
		case "/library/sections/2/all", "/library/sections/3/all":
			_, _ = w.Write([]byte(`{"MediaContainer":{"size":0}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func TestPlex(t *testing.T) {
	ctx := context.Background()
	f := fakePlex(t)
	c, err := New(Config{Kind: models.MediaServerPlex, URL: f.URL, Token: "plextoken"})
	require.NoError(t, err)

	info, err := c.Test(ctx)
	require.NoError(t, err)
	assert.Equal(t, Info{Name: "NAS Plex", Version: "1.41.0.8992", Libraries: 3}, info)

	require.NoError(t, c.RefreshPaths(ctx, []string{"/data/movies/奥本海默 (2023)", "/data/tv/anime/葬送的芙莉莲 (2023)"}))
	var refreshes []string
	for _, r := range f.requests() {
		if strings.HasSuffix(r.Path, "/refresh") {
			refreshes = append(refreshes, r.Path+"?"+r.Query)
		}
	}
	assert.Equal(t, []string{
		"/library/sections/1/refresh?path=%2Fdata%2Fmovies%2F%E5%A5%A5%E6%9C%AC%E6%B5%B7%E9%BB%98+%282023%29",
		"/library/sections/2/refresh?path=%2Fdata%2Ftv%2Fanime%2F%E8%91%AC%E9%80%81%E7%9A%84%E8%8A%99%E8%8E%89%E8%8E%B2+%282023%29",
	}, refreshes, "取包含路径最长的那个分区；两个分区一样长时取先列出的")

	err = c.RefreshPaths(ctx, []string{"/elsewhere/x", "/data/movies2/y"})
	require.ErrorIs(t, err, ErrNoLibrary, "/data/movies2 不在 /data/movies 里")

	require.NoError(t, c.RefreshAll(ctx))

	ok, err := c.Exists(ctx, Query{Kind: "movie", TMDBID: 872585, Title: "奥本海默"})
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = c.Exists(ctx, Query{Kind: "movie", IMDbID: "tt15398776", Title: "奥本海默"})
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = c.Exists(ctx, Query{Kind: "tv", TMDBID: 872585, Title: "奥本海默"})
	require.NoError(t, err)
	assert.False(t, ok, "只在剧集分区里找")
	_, err = c.Exists(ctx, Query{Kind: "movie"})
	require.Error(t, err)

	bad, _ := New(Config{Kind: models.MediaServerPlex, URL: f.URL, Token: "nope"})
	_, err = bad.Test(ctx)
	require.ErrorIs(t, err, ErrUnauthorized)
	require.ErrorIs(t, bad.RefreshPaths(ctx, []string{"/data/movies/x"}), ErrUnauthorized)
	require.ErrorIs(t, bad.RefreshAll(ctx), ErrUnauthorized)
	_, err = bad.Exists(ctx, Query{Kind: "movie", Title: "x"})
	require.ErrorIs(t, err, ErrUnauthorized)
}

func TestWithin(t *testing.T) {
	assert.True(t, within("/data/tv", "/data/tv/x"))
	assert.True(t, within("/data/tv/", "/data/tv"))
	assert.False(t, within("/data/tv", "/data/tv2/x"))
	assert.True(t, within(`D:\Media\TV`, `d:/media/tv/Show`))
	assert.True(t, within("", "/x"))
	assert.False(t, within("", "x"))
}
