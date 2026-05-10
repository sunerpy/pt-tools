package douban

import (
	"context"
	"fmt"
	"strings"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

type DoubanScraper struct {
	client *Client
	active bool
}

func NewScraper(client *Client) *DoubanScraper {
	if client == nil {
		client = NewClient(Config{})
	}
	return &DoubanScraper{client: client, active: true}
}

func (s *DoubanScraper) Info() core.ProviderInfo {
	return core.ProviderInfo{
		Name:        "douban",
		DisplayName: "豆瓣电影",
		Priority:    80,
		Kind:        "all",
	}
}

func (s *DoubanScraper) IsActive() bool {
	return s != nil && s.active
}

func (s *DoubanScraper) SearchMovie(ctx context.Context, opts core.MovieSearchOptions) ([]core.MediaSearchCandidate, error) {
	resp, err := s.client.Search(ctx, strings.TrimSpace(opts.Query), 20)
	if err != nil {
		return nil, err
	}
	items := make([]core.MediaSearchCandidate, 0)
	for _, item := range resp.Items {
		candidate := searchCandidateFromItem(item)
		if candidate.MediaType != core.MediaTypeMovie {
			continue
		}
		if opts.Year > 0 && candidate.Year > 0 && candidate.Year != opts.Year {
			continue
		}
		items = append(items, candidate)
	}
	return items, nil
}

func (s *DoubanScraper) GetMovieMetadata(ctx context.Context, opts core.MovieSearchOptions) (*core.Movie, error) {
	id, err := s.resolveMovieID(ctx, opts)
	if err != nil {
		return nil, err
	}

	detail, detailErr := s.client.GetMovie(ctx, id)
	if detailErr == nil {
		celebs, _ := s.client.GetMovieCelebrities(ctx, id)
		photos, _ := s.client.GetMoviePhotos(ctx, id)
		return toMovie(detail, celebs, photos), nil
	}

	htmlDetail, htmlErr := s.client.GetHTMLDetail(ctx, id)
	if htmlErr != nil {
		return nil, fmt.Errorf("douban movie frodo failed: %w; html fallback failed: %v", detailErr, htmlErr)
	}
	return htmlToMovie(htmlDetail), nil
}

func (s *DoubanScraper) SearchTvShow(ctx context.Context, opts core.TvShowSearchOptions) ([]core.MediaSearchCandidate, error) {
	resp, err := s.client.Search(ctx, strings.TrimSpace(opts.Query), 20)
	if err != nil {
		return nil, err
	}
	items := make([]core.MediaSearchCandidate, 0)
	for _, item := range resp.Items {
		candidate := searchCandidateFromItem(item)
		if candidate.MediaType != core.MediaTypeTvShow {
			continue
		}
		if opts.FirstAirYear > 0 && candidate.Year > 0 && candidate.Year != opts.FirstAirYear {
			continue
		}
		items = append(items, candidate)
	}
	return items, nil
}

func (s *DoubanScraper) GetTvShowMetadata(ctx context.Context, opts core.TvShowSearchOptions) (*core.TvShow, error) {
	id, err := s.resolveTVID(ctx, opts)
	if err != nil {
		return nil, err
	}

	detail, detailErr := s.client.GetTV(ctx, id)
	if detailErr == nil {
		return toTVShow(detail), nil
	}

	htmlDetail, htmlErr := s.client.GetHTMLDetail(ctx, id)
	if htmlErr != nil {
		return nil, fmt.Errorf("douban tv frodo failed: %w; html fallback failed: %v", detailErr, htmlErr)
	}
	return htmlToTVShow(htmlDetail), nil
}

func (s *DoubanScraper) GetEpisodeList(context.Context, core.TvShowSearchOptions) ([]core.TvShowEpisode, error) {
	return nil, fmt.Errorf("douban episode list: %w", core.ErrNotFound)
}

func (s *DoubanScraper) GetEpisodeMetadata(context.Context, core.TvShowEpisodeSearchOptions) (*core.TvShowEpisode, error) {
	return nil, fmt.Errorf("douban episode metadata: %w", core.ErrNotFound)
}

func (s *DoubanScraper) resolveMovieID(ctx context.Context, opts core.MovieSearchOptions) (string, error) {
	return s.resolveIDFromQuery(ctx, opts.Query, opts.Year, true)
}

func (s *DoubanScraper) resolveTVID(ctx context.Context, opts core.TvShowSearchOptions) (string, error) {
	year := opts.FirstAirYear
	if year == 0 {
		year = opts.Year
	}
	return s.resolveIDFromQuery(ctx, opts.Query, year, false)
}

func (s *DoubanScraper) resolveIDFromQuery(ctx context.Context, query string, year int, movie bool) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("douban query: %w", core.ErrInvalidID)
	}
	if isNumeric(query) {
		return query, nil
	}

	resp, err := s.client.Search(ctx, query, 20)
	if err != nil {
		return "", err
	}

	targetType := core.MediaTypeTvShow
	if movie {
		targetType = core.MediaTypeMovie
	}
	for _, item := range resp.Items {
		candidate := searchCandidateFromItem(item)
		if candidate.MediaType != targetType || candidate.ID == "" {
			continue
		}
		if year > 0 && candidate.Year > 0 && candidate.Year != year {
			continue
		}
		return candidate.ID, nil
	}
	return "", fmt.Errorf("douban search id for %q: %w", query, core.ErrNotFound)
}

func isNumeric(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
