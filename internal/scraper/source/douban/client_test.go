package douban

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

func TestSignFrodo_Golden(t *testing.T) {
	t.Parallel()

	got := signFrodo(http.MethodGet, "/api/v2/movie/1292052", 1700000000)
	const want = "6Jvrc63BOBqIRrQO5XdiGmUWg+4="
	if got != want {
		t.Fatalf("signFrodo() = %q, want %q", got, want)
	}
}

func TestClient_GetMovie(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/movie/1292052" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		writeJSON(t, w, subjectDetailResponse{ID: "1292052", Type: "movie", Title: "肖申克的救赎", Year: "1994", Intro: "希望让人自由", Rating: rating{Value: 9.7, Count: 100}, Pic: image{Large: "poster.jpg"}})
	}))
	defer server.Close()

	client := newTestClient(server.URL+"/api/v2", "")
	got, err := client.GetMovie(context.Background(), "1292052")
	if err != nil {
		t.Fatalf("GetMovie() error = %v", err)
	}
	if got.Title != "肖申克的救赎" {
		t.Fatalf("GetMovie().Title = %q", got.Title)
	}
	if got.Rating.Value != 9.7 {
		t.Fatalf("GetMovie().Rating.Value = %v", got.Rating.Value)
	}
}

func TestClient_GetMovie_Retry429(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		current := attempts
		mu.Unlock()

		if current == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writeJSON(t, w, subjectDetailResponse{ID: "1292052", Type: "movie", Title: "重试成功"})
	}))
	defer server.Close()

	client := newTestClient(server.URL+"/api/v2", "")
	got, err := client.GetMovie(context.Background(), "1292052")
	if err != nil {
		t.Fatalf("GetMovie() error = %v", err)
	}
	if got.Title != "重试成功" {
		t.Fatalf("GetMovie().Title = %q", got.Title)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestClient_RandomUserAgent(t *testing.T) {
	t.Parallel()

	seen := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("User-Agent"))
		writeJSON(t, w, subjectDetailResponse{ID: "1292052", Type: "movie", Title: "UA"})
	}))
	defer server.Close()

	client := newTestClient(server.URL+"/api/v2", "")
	client.userAgents = []string{"ua-1", "ua-2"}
	seq := 0
	client.randIntn = func(n int) int {
		v := seq % n
		seq++
		return v
	}

	_, _ = client.GetMovie(context.Background(), "1292052")
	_, _ = client.GetMovie(context.Background(), "1292052")

	if len(seen) != 2 {
		t.Fatalf("requests = %d, want 2", len(seen))
	}
	allowed := map[string]bool{"ua-1": true, "ua-2": true}
	for _, ua := range seen {
		if !allowed[ua] {
			t.Fatalf("unexpected user agent %q", ua)
		}
	}
	if seen[0] == seen[1] {
		t.Fatalf("user agents should differ, got %v", seen)
	}
}

func TestClient_SignatureInQuery(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("apikey") != apiKey {
			t.Fatalf("apikey = %q", q.Get("apikey"))
		}
		ts := q.Get("_ts")
		if ts == "" {
			t.Fatal("missing _ts")
		}
		wantSig := signFrodo(http.MethodGet, r.URL.Path, mustAtoi64(t, ts))
		if q.Get("_sig") != wantSig {
			t.Fatalf("_sig = %q, want %q", q.Get("_sig"), wantSig)
		}
		writeJSON(t, w, subjectDetailResponse{ID: "1292052", Title: "签名校验"})
	}))
	defer server.Close()

	client := newTestClient(server.URL+"/api/v2", "")
	client.now = func() time.Time { return time.Unix(1700000000, 0) }

	if _, err := client.GetMovie(context.Background(), "1292052"); err != nil {
		t.Fatalf("GetMovie() error = %v", err)
	}
}

func TestHTML_ParseDetailPage(t *testing.T) {
	t.Parallel()

	html := `<html><body>
	<h1><span property="v:itemreviewed">星际穿越</span><span class="year">(2014)</span></h1>
	<strong class="ll rating_num">9.4</strong>
	<span property="v:summary">  穿越宇宙，寻找家园。 </span>
	<a rel="v:directedBy">克里斯托弗·诺兰</a>
	<a rel="v:starring">马修·麦康纳</a>
	<a href="https://www.imdb.com/title/tt0816692/">IMDb</a>
	</body></html>`

	detail, err := parseHTMLDetail("1889243", strings.NewReader(html))
	if err != nil {
		t.Fatalf("parseHTMLDetail() error = %v", err)
	}
	if detail.IMDBID != "tt0816692" {
		t.Fatalf("IMDBID = %q", detail.IMDBID)
	}
	if detail.Title != "星际穿越" {
		t.Fatalf("Title = %q", detail.Title)
	}
	if detail.Year != 2014 {
		t.Fatalf("Year = %d", detail.Year)
	}
}

func TestScraper_SearchMovie(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/search/weixin" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		writeJSON(t, w, searchResponse{Items: []searchItem{{ID: "1", Type: "movie", Title: "流浪地球", Year: "2019", Pic: image{Large: "poster.jpg"}, Rating: rating{Value: 8.0}}, {ID: "2", Type: "tv", Title: "三体", Year: "2023"}}})
	}))
	defer server.Close()

	scraper := NewScraper(newTestClient(server.URL+"/api/v2", ""))
	items, err := scraper.SearchMovie(context.Background(), core.MovieSearchOptions{Query: "流浪地球"})
	if err != nil {
		t.Fatalf("SearchMovie() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].MediaType != core.MediaTypeMovie {
		t.Fatalf("MediaType = %v", items[0].MediaType)
	}
}

func TestScraper_GetMovieMetadata_FallbackToHTML(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/movie/1292052":
			w.WriteHeader(http.StatusForbidden)
		case "/subject/1292052/":
			_, _ = w.Write([]byte(`<html><body>
			<h1><span property="v:itemreviewed">肖申克的救赎</span><span class="year">(1994)</span></h1>
			<strong class="ll rating_num">9.7</strong>
			<span property="v:summary">  希望让人自由。 </span>
			<a rel="v:directedBy">弗兰克·德拉邦特</a>
			<a rel="v:starring">蒂姆·罗宾斯</a>
			<a href="https://www.imdb.com/title/tt0111161/">IMDb</a>
			</body></html>`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := newTestClient(server.URL+"/api/v2", server.URL)
	scraper := NewScraper(client)
	movie, err := scraper.GetMovieMetadata(context.Background(), core.MovieSearchOptions{Query: "1292052"})
	if err != nil {
		t.Fatalf("GetMovieMetadata() error = %v", err)
	}
	if movie.Title != "肖申克的救赎" {
		t.Fatalf("Title = %q", movie.Title)
	}
	if movie.IDs["imdb"] != "tt0111161" {
		t.Fatalf("IMDBID = %q", movie.IDs["imdb"])
	}
	if movie.Provider != "douban" {
		t.Fatalf("Provider = %q", movie.Provider)
	}
}

func newTestClient(baseURL, htmlURL string) *Client {
	client := NewClient(Config{
		BaseURL:    baseURL,
		HTMLURL:    htmlURL,
		HTTPClient: &http.Client{Timeout: 2 * time.Second},
	})
	client.rateLimit = 0
	client.sleeper = func(time.Duration) {}
	client.now = func() time.Time { return time.Unix(1700000000, 0) }
	client.randIntn = func(n int) int { return 0 }
	return client
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode json: %v", err)
	}
}

func mustAtoi64(t *testing.T, value string) int64 {
	t.Helper()
	parsed, err := url.QueryUnescape(value)
	if err != nil {
		t.Fatalf("QueryUnescape(%q): %v", value, err)
	}
	var out int64
	_, err = fmt.Sscanf(parsed, "%d", &out)
	if err != nil {
		t.Fatalf("parse int64 %q: %v", value, err)
	}
	return out
}
