package tmdb

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	tmdbsdk "github.com/cyruzin/golang-tmdb"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

func mockTMDBServer(t *testing.T, handlers map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"images": map[string]any{
				"base_url":        "https://image.tmdb.org/t/p/",
				"secure_base_url": "https://image.tmdb.org/t/p/",
			},
		})
	})
	for path, handler := range handlers {
		mux.HandleFunc(path, handler)
	}
	return httptest.NewServer(mux)
}

func newTestScraper(t *testing.T, baseURL string, client *http.Client) *TMDBScraper {
	t.Helper()
	s, err := NewTMDBScraper(Config{
		BearerToken:      "bearer-token",
		Language:         "zh-CN",
		FallbackLanguage: "en-US",
		HTTPClient:       client,
		BaseURL:          baseURL,
	})
	require.NoError(t, err)
	return s
}

func TestTMDB_SearchMovie_Success(t *testing.T) {
	srv := mockTMDBServer(t, map[string]http.HandlerFunc{
		"/search/movie": func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Bearer bearer-token", r.Header.Get("Authorization"))
			require.Equal(t, "zh-CN", r.URL.Query().Get("language"))
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"page": 1, "total_pages": 1, "total_results": 1,
				"results": []map[string]any{{
					"id": 27205, "title": "盗梦空间", "original_title": "Inception",
					"release_date": "2010-07-15", "poster_path": "/path.jpg",
					"overview": "一部诺兰的科幻电影", "vote_average": 8.4,
				}},
			})
		},
	})
	defer srv.Close()

	s := newTestScraper(t, srv.URL, srv.Client())
	results, err := s.SearchMovie(context.Background(), core.MovieSearchOptions{Query: "Inception", Year: 2010})
	require.NoError(t, err)
	require.NotEmpty(t, results)
	require.Equal(t, "盗梦空间", results[0].Title)
	require.Equal(t, "https://image.tmdb.org/t/p/original/path.jpg", results[0].PosterURL)
}

func TestTMDB_GetMovieMetadata_WithAppends(t *testing.T) {
	srv := mockTMDBServer(t, map[string]http.HandlerFunc{
		"/movie/27205": func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, appendToResponse, r.URL.Query().Get("append_to_response"))
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":                   27205,
				"title":                "盗梦空间",
				"original_title":       "Inception",
				"overview":             "一部诺兰的科幻电影",
				"tagline":              "Your mind is the scene of the crime.",
				"release_date":         "2010-07-15",
				"runtime":              148,
				"poster_path":          "/poster.jpg",
				"backdrop_path":        "/backdrop.jpg",
				"vote_average":         8.4,
				"vote_count":           1000,
				"genres":               []map[string]any{{"id": 1, "name": "科幻"}},
				"production_companies": []map[string]any{{"id": 1, "name": "Warner Bros."}},
				"production_countries": []map[string]any{{"iso_3166_1": "US", "name": "United States"}},
				"spoken_languages":     []map[string]any{{"iso_639_1": "en", "name": "English"}},
				"credits": map[string]any{
					"cast": []map[string]any{{"id": 10, "name": "Leonardo DiCaprio", "character": "Cobb", "order": 0, "profile_path": "/actor.jpg"}},
					"crew": []map[string]any{{"id": 20, "name": "Christopher Nolan", "job": "Director", "department": "Directing", "profile_path": "/director.jpg"}},
				},
				"external_ids": map[string]any{"id": 27205, "imdb_id": "tt1375666"},
				"images": map[string]any{
					"posters":   []map[string]any{{"file_path": "/poster.jpg", "iso_639_1": "zh", "width": 1000, "height": 1500, "vote_count": 10}},
					"backdrops": []map[string]any{{"file_path": "/backdrop.jpg", "iso_639_1": "en", "width": 1920, "height": 1080, "vote_count": 8}},
					"logos":     []map[string]any{{"file_path": "/logo.png", "iso_639_1": "en", "width": 500, "height": 200, "vote_count": 5}},
				},
				"videos": map[string]any{
					"results": []map[string]any{{"id": "1", "key": "abc", "name": "Trailer", "site": "YouTube", "size": 1080, "type": "Trailer"}},
				},
			})
		},
	})
	defer srv.Close()

	s := newTestScraper(t, srv.URL, srv.Client())
	movie, err := s.GetMovieMetadata(context.Background(), core.MovieSearchOptions{TMDBID: 27205})
	require.NoError(t, err)
	require.Equal(t, "盗梦空间", movie.Title)
	require.Equal(t, "tt1375666", movie.IDs["imdb"])
	require.Equal(t, 148, movie.Runtime)
	require.Len(t, movie.Actors, 1)
	require.Len(t, movie.Directors, 1)
	require.Equal(t, "https://image.tmdb.org/t/p/original/poster.jpg", movie.ArtworkURLs[core.ArtworkTypePoster])
	require.NotEmpty(t, movie.Trailers)
}

func TestTMDB_LanguageFallback(t *testing.T) {
	var zhCalls int
	srv := mockTMDBServer(t, map[string]http.HandlerFunc{
		"/movie/27205": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("language") == "zh-CN" {
				zhCalls++
				_ = json.NewEncoder(w).Encode(map[string]any{"id": 27205, "title": "", "overview": "", "release_date": "2010-07-15"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 27205, "title": "Inception", "overview": "Dream layers.", "release_date": "2010-07-15"})
		},
	})
	defer srv.Close()

	s := newTestScraper(t, srv.URL, srv.Client())
	movie, err := s.GetMovieMetadata(context.Background(), core.MovieSearchOptions{TMDBID: 27205})
	require.NoError(t, err)
	require.Equal(t, 1, zhCalls)
	require.Equal(t, "Inception", movie.Title)
	require.Equal(t, "Dream layers.", movie.Plot)
}

func TestTMDB_ProxyHTTPClient(t *testing.T) {
	target := mockTMDBServer(t, map[string]http.HandlerFunc{
		"/search/movie": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"page": 1, "results": []map[string]any{{"id": 1, "title": "Proxy Movie", "overview": "ok"}}})
		},
	})
	defer target.Close()

	proxyHits := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits++
		switch r.URL.Path {
		case "/configuration":
		case "/search/movie":
			require.Equal(t, target.URL+"/search/movie?api_key=&query=Proxy&language=zh-CN", r.URL.String())
		default:
			t.Fatalf("unexpected proxy path: %s", r.URL.String())
		}
		req, err := http.NewRequestWithContext(r.Context(), r.Method, r.URL.String(), nil)
		require.NoError(t, err)
		resp, err := target.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		for key, values := range resp.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	defer proxy.Close()

	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	s := newTestScraper(t, target.URL, client)

	results, err := s.SearchMovie(context.Background(), core.MovieSearchOptions{Query: "Proxy"})
	require.NoError(t, err)
	require.NotEmpty(t, results)
	require.Greater(t, proxyHits, 0)
}

func TestTMDB_ConvertMovie(t *testing.T) {
	setImageBaseURL("https://image.tmdb.org/t/p/")
	movie := tmdbToMovie(&tmdbsdk.MovieDetails{
		ID:                     27205,
		Title:                  "盗梦空间",
		OriginalTitle:          "Inception",
		Overview:               "Dream movie",
		ReleaseDate:            "2010-07-15",
		Runtime:                148,
		Genres:                 []tmdbsdk.Genre{{ID: 1, Name: "科幻"}},
		MovieExternalIDsAppend: &tmdbsdk.MovieExternalIDsAppend{MovieExternalIDs: &tmdbsdk.MovieExternalIDs{IMDbID: "tt1375666"}},
		MovieCreditsAppend: &tmdbsdk.MovieCreditsAppend{Credits: struct{ *tmdbsdk.MovieCredits }{MovieCredits: &tmdbsdk.MovieCredits{Cast: []struct {
			Adult              bool    `json:"adult"`
			CastID             int64   `json:"cast_id"`
			Character          string  `json:"character"`
			CreditID           string  `json:"credit_id"`
			Gender             int     `json:"gender"`
			ID                 int64   `json:"id"`
			KnownForDepartment string  `json:"known_for_department"`
			Name               string  `json:"name"`
			Order              int     `json:"order"`
			OriginalName       string  `json:"original_name"`
			Popularity         float32 `json:"popularity"`
			ProfilePath        string  `json:"profile_path"`
		}{{ID: 1, Name: "Leonardo DiCaprio", Character: "Cobb", Order: 0, ProfilePath: "/actor.jpg"}}}}},
		MovieImagesAppend: &tmdbsdk.MovieImagesAppend{Images: &tmdbsdk.MovieImages{Posters: []tmdbsdk.MovieImage{{ImageBase: tmdbsdk.ImageBase{FilePath: "/poster.jpg", Width: 100, Height: 200}}}}},
	})
	require.NotNil(t, movie)
	require.Equal(t, "tt1375666", movie.IDs["imdb"])
	require.Equal(t, 2010, movie.Year)
	require.Equal(t, "https://image.tmdb.org/t/p/original/poster.jpg", movie.ArtworkURLs[core.ArtworkTypePoster])
}

func TestTMDB_Info(t *testing.T) {
	s := &TMDBScraper{info: providerInfo, client: &tmdbsdk.Client{}}
	info := s.Info()
	require.Equal(t, "tmdb", info.Name)
	require.Equal(t, "The Movie Database", info.DisplayName)
	require.Equal(t, "v3", info.Version)
	require.Equal(t, 100, info.Priority)
	require.Equal(t, "all", info.Kind)
}

func TestTMDB_SearchTvShow_Basic(t *testing.T) {
	srv := mockTMDBServer(t, map[string]http.HandlerFunc{
		"/search/tv": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"page": 1, "results": []map[string]any{{"id": 1396, "name": "绝命毒师", "overview": "Teacher.", "first_air_date": "2008-01-20", "vote_average": 9.5}}})
		},
	})
	defer srv.Close()

	s := newTestScraper(t, srv.URL, srv.Client())
	results, err := s.SearchTvShow(context.Background(), core.TvShowSearchOptions{Query: "Breaking Bad"})
	require.NoError(t, err)
	require.NotEmpty(t, results)
	require.Equal(t, "绝命毒师", results[0].Title)
}

func TestTMDB_GetEpisodeMetadata_Basic(t *testing.T) {
	srv := mockTMDBServer(t, map[string]http.HandlerFunc{
		"/tv/1396/season/1/episode/1": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":             62085,
				"name":           "Pilot",
				"overview":       "A chemistry teacher...",
				"air_date":       "2008-01-20",
				"season_number":  1,
				"episode_number": 1,
				"still_path":     "/still.jpg",
				"vote_average":   8.2,
				"vote_count":     20,
				"guest_stars":    []map[string]any{{"id": 1, "name": "Guest", "character": "Guest Role", "order": 0}},
				"crew":           []map[string]any{{"id": 2, "name": "Director", "job": "Director", "department": "Directing"}},
				"external_ids":   map[string]any{"imdb_id": "tt0959621", "tvdb_id": 349232},
				"images":         map[string]any{"stills": []map[string]any{{"file_path": "/still.jpg", "width": 1280, "height": 720, "vote_count": 1}}},
			})
		},
	})
	defer srv.Close()

	s := newTestScraper(t, srv.URL, srv.Client())
	episode, err := s.GetEpisodeMetadata(context.Background(), core.TvShowEpisodeSearchOptions{TvShowID: 1396, Season: 1, Episode: 1})
	require.NoError(t, err)
	require.Equal(t, "Pilot", episode.Title)
	require.Equal(t, 1, episode.Season)
	require.Equal(t, 1, episode.Episode)
	require.Equal(t, "tt0959621", episode.IDs["imdb"])
}
