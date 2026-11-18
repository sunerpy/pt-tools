package csf

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeURL(t *testing.T) {
	for in, want := range map[string]string{
		"http://192.168.1.10:19035":        "http://192.168.1.10:19035/",
		" http://csf.lan:19035/ ":          "http://csf.lan:19035/",
		"https://nas.example.test/csf":     "https://nas.example.test/csf/",
		"https://nas.example.test/csf///":  "https://nas.example.test/csf/",
		"HTTP://NAS.example.test:19035/x/": "http://NAS.example.test:19035/x/",
	} {
		got, err := NormalizeURL(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, in := range []string{"", "192.168.1.10:19035", "ftp://csf.lan/", "http://", "http://u:p@csf.lan/", "http://csf.lan/?a=1", "http://csf.lan/#x", "http://csf.lan/\x00"} {
		_, err := NormalizeURL(in)
		require.ErrorIs(t, err, ErrBadConfig, in)
	}
}

func TestNew(t *testing.T) {
	_, err := New("http://csf.lan/", " ", nil)
	require.ErrorIs(t, err, ErrBadConfig)
	_, err = New("csf.lan", "k", nil)
	require.ErrorIs(t, err, ErrBadConfig)
	c, err := New("http://csf.lan", "k", nil)
	require.NoError(t, err)
	assert.Equal(t, "http://csf.lan/", c.base)
	assert.NotNil(t, c.hc)
}

// fakeCSF 是假的 ChineseSubFinder：认 Bearer secret-key，add-job 按 reply 回，记下收到的请求体。
func fakeCSF(t *testing.T, reply func(w http.ResponseWriter, body map[string]any)) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	var got []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f := strings.Fields(r.Header.Get("Authorization")); len(f) != 2 || f[0] != "Bearer" || f[1] != "secret-key" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"AccessToken Error"}`))
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/csf/api/v1/add-job":
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			b, _ := io.ReadAll(r.Body)
			var body map[string]any
			require.NoError(t, json.Unmarshal(b, &body))
			got = append(got, body)
			reply(w, body)
		case r.Method == http.MethodGet && r.URL.Path == "/csf/api/v1/job-status":
			assert.Empty(t, r.URL.Query().Get("job_id"))
			_, _ = w.Write([]byte(`{"job_id":"","job_status":0,"message":"job_id is empty"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func TestAddJob(t *testing.T) {
	srv, got := fakeCSF(t, func(w http.ResponseWriter, body map[string]any) {
		switch body["physical_video_file_full_path"] {
		case "/media/movies/A (2023)/A (2023).mkv":
			_, _ = w.Write([]byte(`{"job_id":"j1","job_status":0,"message":"ok"}`))
		case "/media/tv/B/Season 1/B - S01E01.mkv":
			_, _ = w.Write([]byte(`{"job_id":"j2","job_status":0,"message":"job is already in queue"}`))
		case "/missing.mkv":
			_, _ = w.Write([]byte(`{"job_id":"","job_status":0,"message":"physical video file not found"}`))
		case "/odd.mkv":
			_, _ = w.Write([]byte(`{"job_id":"","job_status":0,"message":"something else"}`))
		case "/nfo.mkv":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"GetVideoNfoInfo4OneSeriesEpisode: no nfo"}`))
		default:
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`<html>bad gateway</html>`))
		}
	})
	c, err := New(srv.URL+"/csf/", "secret-key", srv.Client())
	require.NoError(t, err)
	ctx := context.Background()

	id, err := c.AddJob(ctx, Job{VideoType: VideoMovie, Path: "/media/movies/A (2023)/A (2023).mkv"})
	require.NoError(t, err)
	assert.Equal(t, "j1", id)
	id, err = c.AddJob(ctx, Job{VideoType: VideoSeries, Path: "/media/tv/B/Season 1/B - S01E01.mkv"})
	require.NoError(t, err)
	assert.Equal(t, "j2", id)
	require.Len(t, *got, 2)
	assert.Equal(t, map[string]any{
		"video_type": float64(0), "physical_video_file_full_path": "/media/movies/A (2023)/A (2023).mkv",
		"task_priority_level": float64(3), "media_server_inside_video_id": "", "is_bluray": false,
	}, (*got)[0])
	assert.Equal(t, float64(1), (*got)[1]["video_type"])

	_, err = c.AddJob(ctx, Job{Path: "/missing.mkv"})
	require.ErrorIs(t, err, ErrFileNotFound)
	_, err = c.AddJob(ctx, Job{Path: "/odd.mkv"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "something else")
	_, err = c.AddJob(ctx, Job{Path: "/nfo.mkv", VideoType: VideoSeries})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no nfo")
	_, err = c.AddJob(ctx, Job{Path: "/gateway.mkv"})
	require.ErrorIs(t, err, ErrUnavailable)
	assert.Contains(t, err.Error(), "502")
	_, err = c.AddJob(ctx, Job{Path: ""})
	require.ErrorIs(t, err, ErrBadConfig)

	bad, err := New(srv.URL+"/csf/", "wrong-key", srv.Client())
	require.NoError(t, err)
	_, err = bad.AddJob(ctx, Job{Path: "/media/movies/A (2023)/A (2023).mkv"})
	require.ErrorIs(t, err, ErrUnauthorized)
	assert.NotContains(t, err.Error(), "wrong-key")
	assert.Len(t, *got, 6)
}

func TestTest(t *testing.T) {
	srv, got := fakeCSF(t, nil)
	c, err := New(srv.URL+"/csf", "secret-key", srv.Client())
	require.NoError(t, err)
	require.NoError(t, c.Test(context.Background()))
	assert.Empty(t, *got)

	bad, _ := New(srv.URL+"/csf", "nope", srv.Client())
	err = bad.Test(context.Background())
	require.ErrorIs(t, err, ErrUnauthorized)
	assert.NotContains(t, err.Error(), "nope")

	// 地址不对（不是 ChineseSubFinder）：404
	wrong, _ := New(srv.URL+"/other", "secret-key", srv.Client())
	err = wrong.Test(context.Background())
	require.ErrorIs(t, err, ErrUnavailable)
	assert.Contains(t, err.Error(), "404")

	// 连不上
	srv.Close()
	err = c.Test(context.Background())
	require.ErrorIs(t, err, ErrUnavailable)
}

func TestTestNotCSF(t *testing.T) {
	// 回 200 但不是 ChineseSubFinder 的 JSON（例如地址填成了别的网页）
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html>hello</html>`))
	}))
	t.Cleanup(srv.Close)
	c, _ := New(srv.URL, "k", srv.Client())
	err := c.Test(context.Background())
	require.ErrorIs(t, err, ErrUnavailable)
	_, err = c.AddJob(context.Background(), Job{Path: "/a.mkv"})
	require.ErrorIs(t, err, ErrUnavailable)
}

func TestPathFor(t *testing.T) {
	cases := []struct{ lib, prefix, target, want string }{
		{"/media/movies", "", "/media/movies/A (2023)/A.mkv", "/media/movies/A (2023)/A.mkv"},
		{"/media/movies", "/mnt/movies", "/media/movies/A (2023)/A.mkv", "/mnt/movies/A (2023)/A.mkv"},
		{"/media/movies", "/mnt/movies/", "/media/movies/A.mkv", "/mnt/movies/A.mkv"},
		{"/media/tv", `D:\Media\TV`, "/media/tv/B/Season 1/B - S01E01.mkv", `D:\Media\TV\B\Season 1\B - S01E01.mkv`},
		{"/media/tv", `\\nas\tv\`, "/media/tv/B/x.mkv", `\\nas\tv\B\x.mkv`},
	}
	for _, tc := range cases {
		got, ok := PathFor(tc.lib, tc.prefix, tc.target)
		require.True(t, ok, tc.target)
		assert.Equal(t, tc.want, got)
	}
	// 不在库目录里的不换
	_, ok := PathFor("/media/movies", "/mnt/movies", "/data/other/A.mkv")
	assert.False(t, ok)
	_, ok = PathFor("/media/movies", "/mnt/movies", "/media/movies-old/A.mkv")
	assert.False(t, ok)
}
