package tmdb

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeAPIBaseURL(t *testing.T) {
	ok := map[string]string{
		"":                               "",
		"  ":                             "",
		"api.tmdb.org":                   "https://api.tmdb.org/3",
		"https://api.tmdb.org":           "https://api.tmdb.org/3",
		"https://api.tmdb.org/":          "https://api.tmdb.org/3",
		"https://api.themoviedb.org/3/":  "https://api.themoviedb.org/3",
		"http://192.168.1.2:8080/tmdb/3": "http://192.168.1.2:8080/tmdb/3",
		"https://tmdb.example.com/api/3": "https://tmdb.example.com/api/3",
	}
	for in, want := range ok {
		got, err := NormalizeAPIBaseURL(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, bad := range []string{"ftp://api.tmdb.org", "https://", "https://user:pw@api.tmdb.org", "https://api.tmdb.org/3?x=1", "https://api.tmdb.org/3#a", "api tmdb.org"} {
		_, err := NormalizeAPIBaseURL(bad)
		assert.Error(t, err, bad)
	}
}

func TestNormalizeImageBaseURL(t *testing.T) {
	ok := map[string]string{
		"":                                    "",
		"image.tmdb.org":                      "https://image.tmdb.org/t/p/",
		"https://image.tmdb.org":              "https://image.tmdb.org/t/p/",
		"https://image.tmdb.org/t/p":          "https://image.tmdb.org/t/p/",
		"https://static.example.com/tmdb/t/p": "https://static.example.com/tmdb/t/p/",
		"http://10.0.0.2:9000/img/":           "http://10.0.0.2:9000/img/",
	}
	for in, want := range ok {
		got, err := NormalizeImageBaseURL(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, bad := range []string{"ftp://image.tmdb.org", "https://image.tmdb.org/t/p/?a=1", "https://u:p@image.tmdb.org"} {
		_, err := NormalizeImageBaseURL(bad)
		assert.Error(t, err, bad)
	}
}

// 客户端的海报地址用自己的图片地址；包级的 PosterURL 仍用官方地址
func TestClientPosterURL(t *testing.T) {
	c, err := New(Options{APIKey: "0123456789abcdef0123", ImageBaseURL: "https://static.example.com/tmdb/t/p/"})
	require.NoError(t, err)
	assert.Equal(t, "https://static.example.com/tmdb/t/p/w500/abc.jpg", c.PosterURL("/abc.jpg", "w500"))
	assert.Equal(t, "https://static.example.com/tmdb/t/p/w342/abc.jpg", c.PosterURL("abc.jpg", ""))
	assert.Empty(t, c.PosterURL("", "w500"))
	assert.Equal(t, "https://image.tmdb.org/t/p/w500/abc.jpg", PosterURL("/abc.jpg", "w500"))
}
