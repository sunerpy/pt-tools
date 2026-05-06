package douban

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

func TestSignFrodo_CaseInsensitiveMethod(t *testing.T) {
	t.Parallel()
	lower := signFrodo("get", "/api/v2/movie/1", 1700000000)
	upper := signFrodo("GET", "/api/v2/movie/1", 1700000000)
	require.Equal(t, lower, upper)
}

func TestSignFrodo_PathWithQueryString(t *testing.T) {
	t.Parallel()
	a := signFrodo("GET", "/api/v2/search/weixin?q=test", 1700000000)
	b := signFrodo("GET", "/api/v2/search/weixin", 1700000000)
	require.NotEqual(t, a, b)
}

func TestSignFrodo_TimestampZero(t *testing.T) {
	t.Parallel()
	sig := signFrodo("GET", "/api/v2/x", 0)
	require.NotEmpty(t, sig)
}

func TestToMovie_Nil(t *testing.T) {
	t.Parallel()
	require.Nil(t, toMovie(nil, nil, nil))
}

func TestToTVShow_Nil(t *testing.T) {
	t.Parallel()
	require.Nil(t, toTVShow(nil))
}

func TestHtmlToMovie_Nil(t *testing.T) {
	t.Parallel()
	require.Nil(t, htmlToMovie(nil))
}

func TestHtmlToTVShow_Nil(t *testing.T) {
	t.Parallel()
	require.Nil(t, htmlToTVShow(nil))
}

func TestToMovie_Full(t *testing.T) {
	t.Parallel()

	detail := &subjectDetailResponse{
		ID:            "1292052",
		Type:          "movie",
		Title:         "肖申克的救赎",
		OriginalTitle: "The Shawshank Redemption",
		Intro:         "希望让人自由",
		CardSubtitle:  "1994 / 美国 / 剧情 / TOP 1",
		Year:          "1994",
		Genres:        []string{"剧情"},
		Countries:     []string{"美国"},
		Languages:     []string{"英语"},
		Pubdate:       []string{"1994-09-10(多伦多电影节)"},
		Durations:     []string{"142分钟"},
		Rating:        rating{Value: 9.7, Count: 100},
		Pic:           image{Large: "poster-large.jpg", Normal: "poster.jpg", Small: "small.jpg"},
		IMDB:          "tt0111161",
		Directors:     []string{"Frank Darabont"},
		Actors:        []string{"Tim Robbins", "Morgan Freeman"},
	}

	celebs := &celebritiesResponse{Celebrities: []celebrity{
		{ID: "1", Name: "Tim Robbins", LatinName: "Tim", Character: "Andy", Role: "actor", Type: "actor", Avatar: image{Large: "ava-large.jpg"}, URL: "url1"},
		{ID: "2", Name: "Frank Darabont", Role: "导演", Type: "director", Avatar: image{Normal: "ava-normal.jpg"}, CoverURL: "cover.jpg"},
		{ID: "3", Name: "Stephen King", Role: "编剧", Type: "writer"},
	}}

	photos := &photosResponse{Photos: []photo{
		{ID: "p1", Image: image{Large: "photo-large.jpg"}, Cover: "cover.jpg"},
		{ID: "p2", Thumb: "thumb.jpg"},
	}}

	movie := toMovie(detail, celebs, photos)
	require.NotNil(t, movie)
	require.Equal(t, "肖申克的救赎", movie.Title)
	require.Equal(t, "The Shawshank Redemption", movie.OriginalTitle)
	require.Equal(t, 1994, movie.Year)
	require.Equal(t, "希望让人自由", movie.Plot)
	require.Equal(t, "tt0111161", movie.IDs["imdb"])
	require.Equal(t, "1292052", movie.IDs["douban"])
	require.Equal(t, 9.7, movie.Ratings["douban"].Value)
	require.Equal(t, []string{"剧情"}, movie.Genres)
	require.Equal(t, []string{"美国"}, movie.Countries)
	require.Equal(t, []string{"英语"}, movie.SpokenLanguages)
	require.Equal(t, 142, movie.Runtime)
	require.Equal(t, 1, movie.Top250)
	require.Equal(t, "douban", movie.Provider)
	require.Equal(t, "poster-large.jpg", movie.ArtworkURLs[core.ArtworkTypePoster])
	require.Equal(t, "photo-large.jpg", movie.ArtworkURLs[core.ArtworkTypeBackground])
	require.NotEmpty(t, movie.Actors)
	require.NotEmpty(t, movie.Directors)
	require.NotEmpty(t, movie.Writers)
}

func TestToTVShow_Full(t *testing.T) {
	t.Parallel()
	detail := &subjectDetailResponse{
		ID:      "26794435",
		Type:    "tv",
		Title:   "三体",
		Intro:   "科幻",
		Year:    "2023",
		Pubdate: []string{"2023-01-15"},
		Pic:     image{Large: "poster.jpg"},
		Rating:  rating{Value: 8.3, Count: 50},
	}
	show := toTVShow(detail)
	require.NotNil(t, show)
	require.Equal(t, "三体", show.Title)
	require.Equal(t, "douban", show.Provider)
	require.Equal(t, 2023, show.Year)
	require.False(t, show.FirstAired.IsZero())
}

func TestHtmlToMovie_Full(t *testing.T) {
	t.Parallel()
	detail := &htmlDetail{
		ID:        "1292052",
		Title:     "肖申克的救赎",
		Plot:      "希望",
		Rating:    9.7,
		IMDBID:    "tt0111161",
		Directors: []string{"Frank"},
		Actors:    []string{"Tim"},
		Year:      1994,
	}
	movie := htmlToMovie(detail)
	require.NotNil(t, movie)
	require.Equal(t, "肖申克的救赎", movie.Title)
	require.Equal(t, "tt0111161", movie.IDs["imdb"])
	require.Len(t, movie.Directors, 1)
	require.Len(t, movie.Actors, 1)
}

func TestHtmlToTVShow_Full(t *testing.T) {
	t.Parallel()
	detail := &htmlDetail{
		ID:        "26794435",
		Title:     "三体",
		Plot:      "科幻",
		Rating:    8.3,
		IMDBID:    "tt13016388",
		Directors: []string{"杨磊"},
		Actors:    []string{"张鲁一"},
		Year:      2023,
	}
	show := htmlToTVShow(detail)
	require.NotNil(t, show)
	require.Equal(t, "三体", show.Title)
	require.Equal(t, 2023, show.Year)
	require.Equal(t, core.EpisodeGroupAired, show.EpisodeGroupKind)
	require.NotNil(t, show.SeasonNames)
	require.NotNil(t, show.SeasonPlots)
}

func TestSearchCandidateFromItem_WithTarget(t *testing.T) {
	t.Parallel()
	item := searchItem{
		Target: &target{
			ID: "9876", Type: "movie", Title: "X",
			Year: "2020", Abstract: "abs", CardSubtitle: "2020",
			Pic: image{Large: "p.jpg"},
		},
		Rating: rating{Value: 85},
	}
	candidate := searchCandidateFromItem(item)
	require.Equal(t, "9876", candidate.ID)
	require.Equal(t, "X", candidate.Title)
	require.Equal(t, 2020, candidate.Year)
	require.Equal(t, core.MediaTypeMovie, candidate.MediaType)
	require.Equal(t, 8.5, candidate.Score)
}

func TestSearchCandidateFromItem_Direct(t *testing.T) {
	t.Parallel()
	item := searchItem{
		ID: "1", Type: "tv", Title: "Y", Year: "2023", Abstract: "a",
		Pic: image{Normal: "p.jpg"}, Rating: rating{Value: 8.5},
	}
	candidate := searchCandidateFromItem(item)
	require.Equal(t, core.MediaTypeTvShow, candidate.MediaType)
	require.Equal(t, 0.85, candidate.Score)
}

func TestMapMediaType(t *testing.T) {
	t.Parallel()
	require.Equal(t, core.MediaTypeMovie, mapMediaType("movie"))
	require.Equal(t, core.MediaTypeMovie, mapMediaType("MOVIE"))
	require.Equal(t, core.MediaTypeTvShow, mapMediaType("tv"))
	require.Equal(t, core.MediaTypeTvShow, mapMediaType("tv_show"))
	require.Equal(t, core.MediaTypeTvShow, mapMediaType("tvshow"))
	require.Equal(t, core.MediaTypeUnknown, mapMediaType("book"))
}

func TestNormalizeType(t *testing.T) {
	t.Parallel()
	require.Equal(t, "director", normalizeType("Director"))
	require.Equal(t, "director", normalizeType("导演"))
	require.Equal(t, "writer", normalizeType("Writer"))
	require.Equal(t, "writer", normalizeType("编剧"))
	require.Equal(t, "actor", normalizeType("Actor"))
	require.Equal(t, "actor", normalizeType("主演"))
	require.Equal(t, "actor", normalizeType("cast"))
	require.Equal(t, "actor", normalizeType("unknown"))
	require.Equal(t, "director", normalizeType("", "director"))
}

func TestNormalizedScore(t *testing.T) {
	t.Parallel()
	require.Equal(t, 0.0, normalizedScore(0))
	require.Equal(t, 0.0, normalizedScore(-1))
	require.Equal(t, 0.85, normalizedScore(8.5))
	require.Equal(t, 0.9, normalizedScore(0.9))
}

func TestParseTop250(t *testing.T) {
	t.Parallel()
	require.Equal(t, 1, parseTop250("1994 / 美国 / 剧情 / TOP 1"))
	require.Equal(t, 0, parseTop250("no top"))
	require.Equal(t, 250, parseTop250("top 250"))
}

func TestParseDate(t *testing.T) {
	t.Parallel()
	require.True(t, parseDate(nil).IsZero())
	require.True(t, parseDate([]string{""}).IsZero())
	require.True(t, parseDate([]string{"bad"}).IsZero())
	d := parseDate([]string{"1994-09-10(多伦多电影节)"})
	require.Equal(t, 1994, d.Year())
	d2 := parseDate([]string{"2023"})
	require.Equal(t, 2023, d2.Year())
	d3 := parseDate([]string{"2023/01/15"})
	require.Equal(t, 2023, d3.Year())
}

func TestFirstDateToken(t *testing.T) {
	t.Parallel()
	require.Equal(t, "1994-09-10", firstDateToken("1994-09-10(电影节)"))
	require.Equal(t, "1994", firstDateToken("1994"))
	require.Equal(t, "2023", firstDateToken("2023 美国"))
}

func TestParseRuntime(t *testing.T) {
	t.Parallel()
	require.Equal(t, 0, parseRuntime(nil))
	require.Equal(t, 0, parseRuntime([]string{""}))
	require.Equal(t, 142, parseRuntime([]string{"142分钟"}))
	require.Equal(t, 90, parseRuntime([]string{"abc", "90 min"}))
}

func TestParseYear(t *testing.T) {
	t.Parallel()
	require.Equal(t, 2023, parseYear("2023"))
	require.Equal(t, 2023, parseYear("", "2023 / 美国"))
	require.Equal(t, 0, parseYear(""))
	require.Equal(t, 0, parseYear("abc"))
}

func TestIsNumeric(t *testing.T) {
	t.Parallel()
	require.False(t, isNumeric(""))
	require.False(t, isNumeric("abc"))
	require.False(t, isNumeric("12a"))
	require.True(t, isNumeric("1292052"))
}

func TestBestImage(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", bestImage(image{}))
	require.Equal(t, "l", bestImage(image{Large: "l", Normal: "n", Small: "s"}))
	require.Equal(t, "n", bestImage(image{Normal: "n", Small: "s"}))
	require.Equal(t, "s", bestImage(image{Small: "s"}))
}

func TestBuildArtworkURLs(t *testing.T) {
	t.Parallel()
	require.Nil(t, buildArtworkURLs(image{}))
	a := buildArtworkURLs(image{Large: "L"})
	require.Equal(t, "L", a[core.ArtworkTypePoster])
}

func TestFirstNonEmpty(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", firstNonEmpty())
	require.Equal(t, "", firstNonEmpty("", "  "))
	require.Equal(t, "a", firstNonEmpty("", "a", "b"))
}

func TestPeopleFromNames(t *testing.T) {
	t.Parallel()
	require.Empty(t, peopleFromNames(nil, core.PersonTypeActor))
	p := peopleFromNames([]string{"  ", "A", "B"}, core.PersonTypeActor)
	require.Len(t, p, 2)
	require.Equal(t, "A", p[0].Name)
}

func TestBuildIDs_Douban(t *testing.T) {
	t.Parallel()
	require.Empty(t, buildIDs("", ""))
	ids := buildIDs("d1", "tt1")
	require.Equal(t, "d1", ids["douban"])
	require.Equal(t, "tt1", ids["imdb"])
}

func TestBuildRatings_Douban(t *testing.T) {
	t.Parallel()
	require.Nil(t, buildRatings(0, 0))
	r := buildRatings(8.5, 100)
	require.Equal(t, 8.5, r["douban"].Value)
}

func TestScraper_Info(t *testing.T) {
	t.Parallel()
	scraper := NewScraper(nil)
	info := scraper.Info()
	require.Equal(t, "douban", info.Name)
	require.Equal(t, 80, info.Priority)
	require.True(t, scraper.IsActive())
}

func TestScraper_IsActive_Nil(t *testing.T) {
	t.Parallel()
	var s *DoubanScraper
	require.False(t, s.IsActive())
}

func TestScraper_SearchTvShow(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, searchResponse{Items: []searchItem{
			{ID: "1", Type: "tv", Title: "三体", Year: "2023", Rating: rating{Value: 8.3}},
			{ID: "2", Type: "movie", Title: "Other", Year: "2020"},
		}})
	}))
	defer server.Close()

	scraper := NewScraper(newTestClient(server.URL+"/api/v2", ""))
	items, err := scraper.SearchTvShow(context.Background(), core.TvShowSearchOptions{Query: "三体", FirstAirYear: 2023})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "三体", items[0].Title)
}

func TestScraper_GetTvShowMetadata(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tv/") {
			writeJSON(t, w, subjectDetailResponse{ID: "26794435", Type: "tv", Title: "三体", Year: "2023", Rating: rating{Value: 8.3}})
			return
		}
		t.Fatalf("unexpected path: %s", r.URL.Path)
	}))
	defer server.Close()

	scraper := NewScraper(newTestClient(server.URL+"/api/v2", ""))
	show, err := scraper.GetTvShowMetadata(context.Background(), core.TvShowSearchOptions{Query: "26794435"})
	require.NoError(t, err)
	require.Equal(t, "三体", show.Title)
	require.Equal(t, "douban", show.Provider)
}

func TestScraper_GetTvShowMetadata_FallbackHTML(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/tv/"):
			w.WriteHeader(http.StatusForbidden)
		case strings.Contains(r.URL.Path, "/subject/"):
			_, _ = w.Write([]byte(`<html><body>
				<h1><span property="v:itemreviewed">三体</span><span class="year">(2023)</span></h1>
				<strong class="ll rating_num">8.3</strong>
				<span property="v:summary">科幻</span>
				<a rel="v:directedBy">杨磊</a>
				<a rel="v:starring">张鲁一</a>
				<a href="https://www.imdb.com/title/tt13016388/">IMDb</a>
				</body></html>`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	scraper := NewScraper(newTestClient(server.URL+"/api/v2", server.URL))
	show, err := scraper.GetTvShowMetadata(context.Background(), core.TvShowSearchOptions{Query: "26794435"})
	require.NoError(t, err)
	require.Equal(t, "三体", show.Title)
	require.Equal(t, "tt13016388", show.IDs["imdb"])
}

func TestScraper_GetEpisodeList_Unsupported(t *testing.T) {
	t.Parallel()
	scraper := NewScraper(nil)
	items, err := scraper.GetEpisodeList(context.Background(), core.TvShowSearchOptions{})
	require.Nil(t, items)
	require.Error(t, err)
}

func TestScraper_GetEpisodeMetadata_Unsupported(t *testing.T) {
	t.Parallel()
	scraper := NewScraper(nil)
	ep, err := scraper.GetEpisodeMetadata(context.Background(), core.TvShowEpisodeSearchOptions{})
	require.Nil(t, ep)
	require.Error(t, err)
}

func TestScraper_ResolveID_EmptyQuery(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, searchResponse{})
	}))
	defer server.Close()

	scraper := NewScraper(newTestClient(server.URL+"/api/v2", ""))
	_, err := scraper.GetMovieMetadata(context.Background(), core.MovieSearchOptions{Query: ""})
	require.Error(t, err)
}

func TestScraper_ResolveID_NotFound(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, searchResponse{Items: []searchItem{{ID: "1", Type: "tv", Title: "Show"}}})
	}))
	defer server.Close()

	scraper := NewScraper(newTestClient(server.URL+"/api/v2", ""))
	_, err := scraper.GetMovieMetadata(context.Background(), core.MovieSearchOptions{Query: "not found"})
	require.Error(t, err)
}

func TestClient_GetTV(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.URL.Path, "/tv/")
		writeJSON(t, w, subjectDetailResponse{ID: "1", Type: "tv", Title: "T"})
	}))
	defer server.Close()

	client := newTestClient(server.URL+"/api/v2", "")
	resp, err := client.GetTV(context.Background(), "1")
	require.NoError(t, err)
	require.Equal(t, "T", resp.Title)
}

func TestClient_GetMovieCelebrities(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.URL.Path, "/celebrities")
		writeJSON(t, w, celebritiesResponse{Celebrities: []celebrity{{ID: "1", Name: "A"}}})
	}))
	defer server.Close()

	client := newTestClient(server.URL+"/api/v2", "")
	resp, err := client.GetMovieCelebrities(context.Background(), "1")
	require.NoError(t, err)
	require.Len(t, resp.Celebrities, 1)
}

func TestClient_GetMoviePhotos(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.URL.Path, "/photos")
		writeJSON(t, w, photosResponse{Photos: []photo{{ID: "p1"}}})
	}))
	defer server.Close()

	client := newTestClient(server.URL+"/api/v2", "")
	resp, err := client.GetMoviePhotos(context.Background(), "1")
	require.NoError(t, err)
	require.Len(t, resp.Photos, 1)
}

func TestClient_GetHTMLDetail_NotFound(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(server.URL+"/api/v2", server.URL)
	_, err := client.GetHTMLDetail(context.Background(), "1")
	require.Error(t, err)
}

func TestClient_GetHTMLDetail_Error(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient(server.URL+"/api/v2", server.URL)
	_, err := client.GetHTMLDetail(context.Background(), "1")
	require.Error(t, err)
}

func TestClient_Search_DefaultCount(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "20", r.URL.Query().Get("count"))
		writeJSON(t, w, searchResponse{})
	}))
	defer server.Close()

	client := newTestClient(server.URL+"/api/v2", "")
	_, err := client.Search(context.Background(), "q", 0)
	require.NoError(t, err)
}

func TestClient_GetMovie_403(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := newTestClient(server.URL+"/api/v2", "")
	_, err := client.GetMovie(context.Background(), "1")
	require.Error(t, err)
}

func TestClient_GetMovie_404(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(server.URL+"/api/v2", "")
	_, err := client.GetMovie(context.Background(), "1")
	require.Error(t, err)
}

func TestClient_GetMovie_500(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient(server.URL+"/api/v2", "")
	_, err := client.GetMovie(context.Background(), "1")
	require.Error(t, err)
}

func TestClient_GetMovie_BadJSON(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := newTestClient(server.URL+"/api/v2", "")
	_, err := client.GetMovie(context.Background(), "1")
	require.Error(t, err)
}

func TestClient_Cloning_URLValuesInSignedURL(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NotEmpty(t, r.URL.Query().Get("apikey"))
		require.NotEmpty(t, r.URL.Query().Get("_sig"))
		writeJSON(t, w, subjectDetailResponse{ID: "1", Title: "T"})
	}))
	defer server.Close()

	client := newTestClient(server.URL+"/api/v2", "")
	_, err := client.GetMovie(context.Background(), "1")
	require.NoError(t, err)
}

func TestClient_RateLimitedTimer(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, subjectDetailResponse{ID: "1", Title: "T"})
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL + "/api/v2", HTMLURL: server.URL, HTTPClient: &http.Client{Timeout: 2 * time.Second}, RateLimit: 2 * time.Second})
	client.now = func() time.Time { return time.Unix(1700000000, 0) }
	sleepCount := 0
	client.sleeper = func(d time.Duration) { sleepCount++ }
	client.randIntn = func(n int) int { return 0 }
	_, err := client.GetMovie(context.Background(), "1")
	require.NoError(t, err)
	require.Greater(t, sleepCount, 0)
}

func TestClient_RandomUserAgent_SingleUA(t *testing.T) {
	t.Parallel()
	client := newTestClient("http://x/api/v2", "")
	client.userAgents = []string{"only-ua"}
	require.Equal(t, "only-ua", client.randomUserAgent())
}

func TestClient_RandomUserAgent_Empty(t *testing.T) {
	t.Parallel()
	client := newTestClient("http://x/api/v2", "")
	client.userAgents = nil
	require.Equal(t, "Mozilla/5.0", client.randomUserAgent())
}

func TestClient_JitterDelay(t *testing.T) {
	t.Parallel()
	client := NewClient(Config{BaseURL: "http://x", RateLimit: 2 * time.Second})
	client.randIntn = func(n int) int { return 0 }
	d := client.jitterDelay()
	require.GreaterOrEqual(t, d, time.Duration(0))

	client.rateLimit = 0
	require.Equal(t, time.Duration(0), client.jitterDelay())
}

func TestClient_ApplyCelebrities_NilInput(t *testing.T) {
	t.Parallel()
	applyCelebrities(nil, nil)
	applyCelebrities(&core.Movie{}, nil)
}

func TestApplyPhotos_Empty(t *testing.T) {
	t.Parallel()
	applyPhotos(nil, nil)
	applyPhotos(&core.MediaEntity{}, nil)
	applyPhotos(&core.MediaEntity{}, &photosResponse{})
	entity := &core.MediaEntity{ArtworkURLs: map[core.ArtworkType]string{core.ArtworkTypeBackground: "existing"}}
	applyPhotos(entity, &photosResponse{Photos: []photo{{Image: image{Large: "new"}}}})
	require.Equal(t, "existing", entity.ArtworkURLs[core.ArtworkTypeBackground])
}

func TestHTML_ParseDetail_EmptyTitle(t *testing.T) {
	t.Parallel()
	_, err := parseHTMLDetail("1", strings.NewReader(`<html><body></body></html>`))
	require.Error(t, err)
}

func TestHTML_ParseDetail_NoIMDB(t *testing.T) {
	t.Parallel()
	detail, err := parseHTMLDetail("1", strings.NewReader(`<html><body>
		<h1><span property="v:itemreviewed">Title</span></h1>
		</body></html>`))
	require.NoError(t, err)
	require.Equal(t, "Title", detail.Title)
	require.Empty(t, detail.IMDBID)
}

func TestHTML_ParseDetail_MetaOriginalTitle(t *testing.T) {
	t.Parallel()
	html := `<html><head><meta property="og:title" content="Original Name"/></head><body>
		<h1><span property="v:itemreviewed">中文名</span></h1>
		</body></html>`
	detail, err := parseHTMLDetail("1", strings.NewReader(html))
	require.NoError(t, err)
	require.Equal(t, "中文名", detail.Title)
	require.Equal(t, "Original Name", detail.OriginalTitle)
}

func TestWrapClientError_Timeout(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	err := wrapClientError("msg", ctx.Err())
	require.Error(t, err)
}

func TestCloneValues(t *testing.T) {
	t.Parallel()
	src := map[string][]string{"k": {"v1", "v2"}}
	dst := cloneValues(src)
	dst["k"][0] = "changed"
	require.Equal(t, "v1", src["k"][0])
}

func TestDecodeResponse_Default(t *testing.T) {
	t.Parallel()
	resp := &http.Response{StatusCode: http.StatusOK, Body: httpNopCloserString(`{"id":"1"}`)}
	var dest subjectDetailResponse
	require.NoError(t, decodeResponse(resp, &dest))
}

func httpNopCloserString(s string) *httpBody {
	return &httpBody{r: strings.NewReader(s)}
}

type httpBody struct {
	r *strings.Reader
}

func (b *httpBody) Read(p []byte) (int, error) { return b.r.Read(p) }
func (b *httpBody) Close() error               { return nil }

func TestDecodeResponse_Unauthorized(t *testing.T) {
	t.Parallel()
	resp := &http.Response{StatusCode: http.StatusForbidden, Body: httpNopCloserString(``)}
	err := decodeResponse(resp, nil)
	require.ErrorIs(t, err, core.ErrUnauthorized)
}

func TestDecodeResponse_RateLimited(t *testing.T) {
	t.Parallel()
	resp := &http.Response{StatusCode: http.StatusTooManyRequests, Body: httpNopCloserString(``)}
	err := decodeResponse(resp, nil)
	require.ErrorIs(t, err, core.ErrRateLimited)
}

func TestDecodeResponse_BadRequest(t *testing.T) {
	t.Parallel()
	resp := &http.Response{StatusCode: http.StatusBadRequest, Body: httpNopCloserString(``)}
	err := decodeResponse(resp, nil)
	require.Error(t, err)
}

func TestDecodeResponse_Redirect(t *testing.T) {
	t.Parallel()
	resp := &http.Response{StatusCode: http.StatusFound, Body: httpNopCloserString(``)}
	err := decodeResponse(resp, nil)
	require.NoError(t, err)
}

func TestNewClient_Defaults(t *testing.T) {
	t.Parallel()
	c := NewClient(Config{})
	require.Equal(t, defaultBaseURL, c.baseURL)
	require.Equal(t, defaultHTMLURL, c.htmlURL)
	require.Equal(t, defaultRateLimit, c.rateLimit)
}

func TestDecodeUnknownJSON(t *testing.T) {
	t.Parallel()
	var out searchResponse
	err := json.Unmarshal([]byte(`{"items":[]}`), &out)
	require.NoError(t, err)
}
