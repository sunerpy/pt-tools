// Package imdb implements a key-less media scraper by parsing IMDb's public
// HTML pages. Inspired by tinyMediaManager's `ImdbMetadataProvider` which
// also avoids the non-existent IMDb public API by scraping `imdb.com/title/tt*`
// directly.
//
// Key properties:
//   - Zero authentication: no API key, no Bearer token, no OAuth
//   - Extracts metadata from <script type="application/ld+json"> + og:meta tags
//   - Complements (does NOT replace) TMDB/Douban — useful as fallback when
//     other providers are unavailable, or for English-language releases where
//     IMDb IDs are the de-facto identifier
//
// Search is limited: IMDb's public search returns a suggestion JSON endpoint
// (no HTML scraping needed). If a tt-ID is already known (common case:
// filename parsing extracted it), we skip search entirely and fetch the title
// page directly.
package imdb

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/scraper/bootstrap/buildkeys"
	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

const (
	defaultBaseURL = "https://www.imdb.com"
	defaultTimeout = 15 * time.Second
)

var providerInfo = core.ProviderInfo{
	Name:        "imdb",
	DisplayName: "IMDb",
	Version:     "html-scrape",
	Priority:    60,
	Kind:        "all",
}

// defaultUserAgents — 桌面浏览器 UA 轮换，降低 IMDb 反爬触发率。
// IMDb 对明显的 bot UA（如 Go-http-client/1.1）会返回 AWS WAF challenge 页。
var defaultUserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 13_6) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
}

// Config 允许调用方注入自定义 HTTP client（例如带代理）和 base URL（mirror）。
type Config struct {
	BaseURL string
	// HTTPClient 显式注入的 http.Client。非 nil 时直接使用，忽略 ProxyURL。
	HTTPClient *http.Client
	// ProxyURL 仅在 HTTPClient 为 nil 时生效，由 core.NewHTTPClient 构造。
	// 支持 http/https/socks5/socks5h；空串使用 http.ProxyFromEnvironment。
	ProxyURL string
}

// Scraper 实现 core.MediaScraper / MovieMetadataScraper / TvShowMetadataScraper。
type Scraper struct {
	baseURL    string
	httpClient *http.Client
	info       core.ProviderInfo
}

// NewScraper 构造 IMDb HTML 刮削器。BaseURL 优先级：
//
//	Config.BaseURL > buildkeys.ImdbBaseURL > defaultBaseURL
//
// ProxyURL 非法时 fallback 到默认 http.Client（与 douban.NewClient 行为保持一致）。
func NewScraper(cfg Config) *Scraper {
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = strings.TrimRight(buildkeys.ImdbBaseURL, "/")
	}
	if base == "" {
		base = defaultBaseURL
	}
	client := cfg.HTTPClient
	if client == nil && cfg.ProxyURL != "" {
		if built, err := core.NewHTTPClient(core.HTTPClientConfig{
			ProxyURL: cfg.ProxyURL,
			Timeout:  defaultTimeout,
		}); err == nil {
			client = built
		}
	}
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}
	return &Scraper{baseURL: base, httpClient: client, info: providerInfo}
}

// Register 把 Scraper 注册到给定 registry。
func Register(registry *core.ScraperRegistry, cfg Config) error {
	if registry == nil {
		return fmt.Errorf("register imdb scraper: %w", core.ErrInvalidID)
	}
	scraper := NewScraper(cfg)
	return registry.Register(providerInfo.Name, func() core.MediaScraper {
		return scraper
	})
}

func (s *Scraper) Info() core.ProviderInfo { return s.info }
func (s *Scraper) IsActive() bool          { return s != nil && s.httpClient != nil }

// SearchMovie 基于 IMDb 建议 API 搜索电影。如果 Query 本身是 tt-ID，则直接返回。
func (s *Scraper) SearchMovie(ctx context.Context, opts core.MovieSearchOptions) ([]core.MediaSearchCandidate, error) {
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		return nil, fmt.Errorf("imdb search: %w", core.ErrInvalidID)
	}
	if id := extractIMDbID(query); id != "" {
		return []core.MediaSearchCandidate{{
			ID:        id,
			Title:     query,
			Year:      opts.Year,
			MediaType: core.MediaTypeMovie,
			Provider:  providerInfo.Name,
		}}, nil
	}
	return s.searchSuggest(ctx, query, opts.Year, core.MediaTypeMovie)
}

// GetMovieMetadata 抓取 IMDb /title/{ttID}/ 页面并解析 JSON-LD 块。
func (s *Scraper) GetMovieMetadata(ctx context.Context, opts core.MovieSearchOptions) (*core.Movie, error) {
	ttID, err := s.resolveID(ctx, opts.Query, opts.Year, core.MediaTypeMovie)
	if err != nil {
		return nil, err
	}
	detail, err := s.fetchTitle(ctx, ttID)
	if err != nil {
		return nil, err
	}
	if detail.Type != "" && !strings.EqualFold(detail.Type, "Movie") && !strings.EqualFold(detail.Type, "TVMovie") {
		return nil, fmt.Errorf("imdb %s is not a movie (type=%q): %w", ttID, detail.Type, core.ErrNotFound)
	}
	return detailToMovie(detail), nil
}

// SearchTvShow 搜索剧集；tt-ID 直接返回。
func (s *Scraper) SearchTvShow(ctx context.Context, opts core.TvShowSearchOptions) ([]core.MediaSearchCandidate, error) {
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		return nil, fmt.Errorf("imdb search tv: %w", core.ErrInvalidID)
	}
	if id := extractIMDbID(query); id != "" {
		return []core.MediaSearchCandidate{{
			ID:        id,
			Title:     query,
			Year:      opts.FirstAirYear,
			MediaType: core.MediaTypeTvShow,
			Provider:  providerInfo.Name,
		}}, nil
	}
	return s.searchSuggest(ctx, query, opts.FirstAirYear, core.MediaTypeTvShow)
}

// GetTvShowMetadata 抓取剧集信息（基本元数据，不含 episode 列表）。
func (s *Scraper) GetTvShowMetadata(ctx context.Context, opts core.TvShowSearchOptions) (*core.TvShow, error) {
	ttID, err := s.resolveID(ctx, opts.Query, opts.FirstAirYear, core.MediaTypeTvShow)
	if err != nil {
		return nil, err
	}
	detail, err := s.fetchTitle(ctx, ttID)
	if err != nil {
		return nil, err
	}
	return detailToTvShow(detail), nil
}

// GetEpisodeList / GetEpisodeMetadata 当前未实现（IMDb 每集页面结构不稳定）。
func (s *Scraper) GetEpisodeList(context.Context, core.TvShowSearchOptions) ([]core.TvShowEpisode, error) {
	return nil, fmt.Errorf("imdb episode list: %w", core.ErrUnsupported)
}

func (s *Scraper) GetEpisodeMetadata(context.Context, core.TvShowEpisodeSearchOptions) (*core.TvShowEpisode, error) {
	return nil, fmt.Errorf("imdb episode metadata: %w", core.ErrUnsupported)
}

func (s *Scraper) resolveID(ctx context.Context, query string, year int, kind core.MediaType) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("imdb resolve: %w", core.ErrInvalidID)
	}
	if id := extractIMDbID(query); id != "" {
		return id, nil
	}
	candidates, err := s.searchSuggest(ctx, query, year, kind)
	if err != nil {
		return "", err
	}
	for _, c := range candidates {
		if c.ID != "" {
			return c.ID, nil
		}
	}
	return "", fmt.Errorf("imdb id for %q: %w", query, core.ErrNotFound)
}

// imdbIDPattern 匹配 IMDb Title ID（tt + 7–10 位数字，IMDb 自 2020 年起发放 10 位 ID）。
var imdbIDPattern = regexp.MustCompile(`(?i)\btt\d{7,10}\b`)

// extractIMDbID 从任意字符串中提取首个 IMDb ID，没有则返回空串。
func extractIMDbID(s string) string {
	return strings.ToLower(imdbIDPattern.FindString(s))
}

// fetchTitle 抓取 /title/{ttID}/ 页面并解析元数据。失败时明确区分：
//   - AWS WAF challenge 页 → ErrProviderDown（生产用户通常正常，CI/数据中心 IP 常被挡）
//   - 404 → ErrNotFound
//   - 其他 → 原始错误
func (s *Scraper) fetchTitle(ctx context.Context, ttID string) (*titleDetail, error) {
	if !strings.HasPrefix(ttID, "tt") {
		return nil, fmt.Errorf("imdb fetchTitle invalid id %q: %w", ttID, core.ErrInvalidID)
	}
	url := s.baseURL + "/title/" + ttID + "/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("imdb build request: %w", err)
	}
	req.Header.Set("User-Agent", randomUserAgent())
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", s.baseURL+"/")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("imdb request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("imdb title %s: %w", ttID, core.ErrNotFound)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("imdb title %s: unexpected status %d", ttID, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("imdb read body: %w", err)
	}
	detail, err := parseTitlePage(ttID, body)
	if err != nil {
		return nil, err
	}
	return detail, nil
}
