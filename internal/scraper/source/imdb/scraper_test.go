package imdb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return data
}

func TestExtractIMDbID(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"tt1375666", "tt1375666"},
		{"TT1375666", "tt1375666"},
		{"https://www.imdb.com/title/tt1375666/", "tt1375666"},
		{"Inception.2010.tt1375666.mkv", "tt1375666"},
		{"just a regular title", ""},
		{"", ""},
		{"tt12345", ""}, // 少于 7 位
		{"tt1234567890", "tt1234567890"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, extractIMDbID(tc.in))
		})
	}
}

func TestParseISODurationMinutes(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"PT2H28M", 148},
		{"PT1H", 60},
		{"PT45M", 45},
		{"PT0M", 0},
		{"", 0},
		{"invalid", 0},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, parseISODurationMinutes(tc.in))
		})
	}
}

func TestClassifyIMDbKind(t *testing.T) {
	assert.Equal(t, core.MediaTypeMovie, classifyIMDbKind("feature", ""))
	assert.Equal(t, core.MediaTypeMovie, classifyIMDbKind("TV movie", ""))
	assert.Equal(t, core.MediaTypeMovie, classifyIMDbKind("short", ""))
	assert.Equal(t, core.MediaTypeMovie, classifyIMDbKind("video", ""))
	assert.Equal(t, core.MediaTypeTvShow, classifyIMDbKind("TV series", ""))
	assert.Equal(t, core.MediaTypeTvShow, classifyIMDbKind("TV mini series", ""))
	assert.Equal(t, core.MediaTypeUnknown, classifyIMDbKind("video game", ""))
	assert.Equal(t, core.MediaTypeUnknown, classifyIMDbKind("podcast", ""))
}

func TestParseTitlePage_Inception(t *testing.T) {
	body := readFixture(t, "inception.html")
	detail, err := parseTitlePage("tt1375666", body)
	require.NoError(t, err)
	require.NotNil(t, detail)

	assert.Equal(t, "tt1375666", detail.ID)
	assert.Equal(t, "tt1375666", detail.IMDBID)
	assert.Equal(t, "Movie", detail.Type)
	assert.Equal(t, "Inception", detail.Title)
	assert.Equal(t, 2010, detail.Year)
	assert.Equal(t, 148, detail.Runtime)
	assert.InDelta(t, 8.8, detail.Rating, 0.001)
	assert.Equal(t, 2500000, detail.RatingCount)
	assert.ElementsMatch(t, []string{"Action", "Adventure", "Sci-Fi"}, detail.Genres)
	assert.Equal(t, []string{"Christopher Nolan"}, detail.Directors)
	assert.ElementsMatch(t,
		[]string{"Leonardo DiCaprio", "Joseph Gordon-Levitt", "Elliot Page"},
		detail.Actors)
	assert.Contains(t, detail.Plot, "dream-sharing")
	assert.Equal(t, "https://m.media-amazon.com/images/M/inception-poster.jpg", detail.PosterURL)
}

func TestParseTitlePage_WAFChallenge(t *testing.T) {
	body := readFixture(t, "waf_challenge.html")
	_, err := parseTitlePage("tt1375666", body)
	require.Error(t, err)
	require.True(t, errors.Is(err, core.ErrProviderDown),
		"WAF challenge page should return ErrProviderDown, got: %v", err)
}

func TestParseTitlePage_NoTitle(t *testing.T) {
	_, err := parseTitlePage("tt1", []byte(`<html><body></body></html>`))
	require.Error(t, err)
	require.True(t, errors.Is(err, core.ErrParseFailed))
}

func TestDetailToMovie_NilSafe(t *testing.T) {
	assert.Nil(t, detailToMovie(nil))
}

func TestDetailToMovie_Full(t *testing.T) {
	body := readFixture(t, "inception.html")
	detail, err := parseTitlePage("tt1375666", body)
	require.NoError(t, err)
	movie := detailToMovie(detail)
	require.NotNil(t, movie)

	assert.Equal(t, "Inception", movie.Title)
	assert.Equal(t, 2010, movie.Year)
	assert.Equal(t, 148, movie.Runtime)
	assert.Equal(t, "tt1375666", movie.IDs["imdb"])
	assert.Equal(t, 2010, movie.ReleaseDate.Year())
	assert.Equal(t, 8.8, movie.Ratings["imdb"].Value)
	assert.Equal(t, 2500000, movie.Ratings["imdb"].Votes)
	assert.Equal(t, 3, len(movie.Actors))
	assert.Equal(t, "Christopher Nolan", movie.Directors[0].Name)
}

func TestDetailToTvShow_NilSafe(t *testing.T) {
	assert.Nil(t, detailToTvShow(nil))
}

func TestScraper_SearchMovie_DirectID(t *testing.T) {
	s := NewScraper(Config{})
	candidates, err := s.SearchMovie(context.Background(), core.MovieSearchOptions{
		Query: "tt1375666",
	})
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.Equal(t, "tt1375666", candidates[0].ID)
	assert.Equal(t, core.MediaTypeMovie, candidates[0].MediaType)
}

func TestScraper_SearchMovie_EmptyQuery(t *testing.T) {
	s := NewScraper(Config{})
	_, err := s.SearchMovie(context.Background(), core.MovieSearchOptions{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, core.ErrInvalidID))
}

// TestScraper_fetchTitle_WithFixture 用 httptest 伪造 IMDb 响应，验证
// HTTP 请求构造和 response 解析都正确，不依赖真实网络。
func TestScraper_fetchTitle_WithFixture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/title/tt1375666/", "should hit title page")
		assert.Contains(t, r.Header.Get("User-Agent"), "Mozilla", "should use browser UA")
		assert.Contains(t, r.Header.Get("Accept-Language"), "en", "should set lang header")
		w.WriteHeader(200)
		_, _ = w.Write(readFixture(t, "inception.html"))
	}))
	defer server.Close()

	s := NewScraper(Config{BaseURL: server.URL})
	detail, err := s.fetchTitle(context.Background(), "tt1375666")
	require.NoError(t, err)
	assert.Equal(t, "Inception", detail.Title)
	assert.Equal(t, 2010, detail.Year)
}

func TestScraper_fetchTitle_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer server.Close()

	s := NewScraper(Config{BaseURL: server.URL})
	_, err := s.fetchTitle(context.Background(), "tt9999999")
	require.Error(t, err)
	assert.True(t, errors.Is(err, core.ErrNotFound))
}

func TestScraper_fetchTitle_WAFPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write(readFixture(t, "waf_challenge.html"))
	}))
	defer server.Close()

	s := NewScraper(Config{BaseURL: server.URL})
	_, err := s.fetchTitle(context.Background(), "tt1375666")
	require.Error(t, err)
	assert.True(t, errors.Is(err, core.ErrProviderDown),
		"WAF challenge should be classified as ErrProviderDown for retry decisions")
}

func TestScraper_GetMovieMetadata_EndToEnd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/title/tt1375666/") {
			w.WriteHeader(200)
			_, _ = w.Write(readFixture(t, "inception.html"))
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	s := NewScraper(Config{BaseURL: server.URL})
	movie, err := s.GetMovieMetadata(context.Background(), core.MovieSearchOptions{
		Query: "tt1375666",
	})
	require.NoError(t, err)
	require.NotNil(t, movie)
	assert.Equal(t, "Inception", movie.Title)
	assert.Equal(t, 2010, movie.Year)
	assert.Equal(t, 148, movie.Runtime)
	assert.Equal(t, "tt1375666", movie.IDs["imdb"])
}

func TestScraper_Info(t *testing.T) {
	s := NewScraper(Config{})
	info := s.Info()
	assert.Equal(t, "imdb", info.Name)
	assert.Equal(t, "IMDb", info.DisplayName)
}

func TestScraper_IsActive(t *testing.T) {
	s := NewScraper(Config{})
	assert.True(t, s.IsActive())
}

func TestRegister(t *testing.T) {
	reg := core.NewRegistry[core.MediaScraper]()
	err := Register(reg, Config{})
	require.NoError(t, err)

	scraper, err := reg.Get("imdb")
	require.NoError(t, err)
	assert.True(t, scraper.IsActive())
}

func TestRegister_NilRegistry(t *testing.T) {
	err := Register(nil, Config{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, core.ErrInvalidID))
}
