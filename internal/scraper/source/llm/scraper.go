package llm

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

// LLMSource 使用 LLM Provider 生成元数据的数据源。
// 支持纯离线模式，或在提供 TMDBValidator 时进行 TMDB ID 交叉验证。
type LLMSource struct {
	provider      Provider
	tmdbValidator TMDBValidator
	priority      int
}

// TMDBValidator 抽象 TMDB 交叉验证能力，避免直接依赖 tmdb 包。
type TMDBValidator interface {
	ValidateMovie(ctx context.Context, tmdbID int) (*ValidationInfo, error)
	ValidateTvShow(ctx context.Context, tmdbID int) (*ValidationInfo, error)
}

// ValidationInfo 表示 TMDB 交叉验证返回的基础信息。
type ValidationInfo struct {
	Title string
	Year  int
	Type  string
}

// SourceConfig LLMSource 构造参数。
type SourceConfig struct {
	Provider      Provider
	TMDBValidator TMDBValidator
	Priority      int
}

func NewLLMSource(cfg SourceConfig) (*LLMSource, error) {
	if cfg.Provider == nil {
		return nil, fmt.Errorf("llm source: Provider required")
	}

	priority := cfg.Priority
	if priority <= 0 {
		priority = 10
	}

	return &LLMSource{
		provider:      cfg.Provider,
		tmdbValidator: cfg.TMDBValidator,
		priority:      priority,
	}, nil
}

func (s *LLMSource) Info() core.ProviderInfo {
	return core.ProviderInfo{
		Name:        "llm",
		DisplayName: "LLM-Native Scraper",
		Version:     "v1",
		Priority:    s.priority,
		Kind:        "all",
		Summary:     "Generate metadata from LLM knowledge (supports 10+ providers via adapter)",
	}
}

func (s *LLMSource) IsActive() bool {
	return s != nil && s.provider != nil
}

func (s *LLMSource) SearchMovie(_ context.Context, opts core.MovieSearchOptions) ([]core.MediaSearchCandidate, error) {
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		return nil, nil
	}

	return []core.MediaSearchCandidate{{
		ID:        "llm-pending",
		Title:     query,
		Year:      opts.Year,
		MediaType: core.MediaTypeMovie,
		Provider:  "llm",
		Score:     0.5,
	}}, nil
}

func (s *LLMSource) GetMovieMetadata(ctx context.Context, opts core.MovieSearchOptions) (*core.Movie, error) {
	res, err := s.extractAndValidate(ctx, ExtractRequest{
		RawTitle:  strings.TrimSpace(opts.Query),
		Language:  opts.Language,
		MediaType: "movie",
		SiteHints: buildMovieHints(opts),
	}, "movie")
	if err != nil {
		return nil, err
	}

	movie := &core.Movie{
		MediaEntity: core.MediaEntity{
			Title:         res.Title,
			OriginalTitle: res.OriginalTitle,
			Year:          res.Year,
			Plot:          res.Plot,
			Outline:       res.Plot,
			Genres:        cloneStrings(res.Genres),
			Provider:      "llm",
			ScrapedAt:     time.Now(),
			IDs:           buildIDs(res),
			Runtime:       res.Runtime,
		},
	}
	movie.Directors = buildPeople(res.Directors, core.PersonTypeDirector)
	movie.Actors = buildPeople(res.Cast, core.PersonTypeActor)

	return movie, nil
}

func (s *LLMSource) SearchTvShow(_ context.Context, opts core.TvShowSearchOptions) ([]core.MediaSearchCandidate, error) {
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		return nil, nil
	}

	year := opts.FirstAirYear
	if year == 0 {
		year = opts.Year
	}

	return []core.MediaSearchCandidate{{
		ID:        "llm-pending",
		Title:     query,
		Year:      year,
		MediaType: core.MediaTypeTvShow,
		Provider:  "llm",
		Score:     0.5,
	}}, nil
}

func (s *LLMSource) GetTvShowMetadata(ctx context.Context, opts core.TvShowSearchOptions) (*core.TvShow, error) {
	res, err := s.extractAndValidate(ctx, ExtractRequest{
		RawTitle:  strings.TrimSpace(opts.Query),
		Language:  opts.Language,
		MediaType: "tv",
		SiteHints: buildTVShowHints(opts),
	}, "tv")
	if err != nil {
		return nil, err
	}

	show := &core.TvShow{
		MediaEntity: core.MediaEntity{
			Title:         res.Title,
			OriginalTitle: res.OriginalTitle,
			Year:          res.Year,
			Plot:          res.Plot,
			Outline:       res.Plot,
			Genres:        cloneStrings(res.Genres),
			Provider:      "llm",
			ScrapedAt:     time.Now(),
			IDs:           buildIDs(res),
			Runtime:       res.Runtime,
		},
		SeasonNames: make(map[int]string),
		SeasonPlots: make(map[int]string),
	}
	show.Directors = buildPeople(res.Directors, core.PersonTypeDirector)
	show.Actors = buildPeople(res.Cast, core.PersonTypeActor)

	return show, nil
}

func (s *LLMSource) GetEpisodeList(context.Context, core.TvShowSearchOptions) ([]core.TvShowEpisode, error) {
	return nil, fmt.Errorf("llm: episode list not supported")
}

func (s *LLMSource) GetEpisodeMetadata(ctx context.Context, opts core.TvShowEpisodeSearchOptions) (*core.TvShowEpisode, error) {
	res, err := s.extractAndValidate(ctx, ExtractRequest{
		RawTitle:  fmt.Sprintf("tmdb:%d S%02dE%02d", opts.TvShowID, opts.Season, opts.Episode),
		Language:  opts.Language,
		MediaType: "tv",
		SiteHints: buildEpisodeHints(opts),
	}, "tv")
	if err != nil {
		return nil, err
	}

	season := res.Season
	if season == 0 {
		season = opts.Season
	}
	episode := res.Episode
	if episode == 0 {
		episode = opts.Episode
	}

	entity := core.MediaEntity{
		Title:         res.Title,
		OriginalTitle: res.OriginalTitle,
		Year:          res.Year,
		Plot:          res.Plot,
		Outline:       res.Plot,
		Genres:        cloneStrings(res.Genres),
		Provider:      "llm",
		ScrapedAt:     time.Now(),
		IDs:           buildIDs(res),
		Runtime:       res.Runtime,
	}

	episodeMeta := &core.TvShowEpisode{
		MediaEntity: entity,
		Season:      season,
		Episode:     episode,
	}
	episodeMeta.Directors = buildPeople(res.Directors, core.PersonTypeDirector)
	episodeMeta.Actors = buildPeople(res.Cast, core.PersonTypeActor)

	return episodeMeta, nil
}

func (s *LLMSource) extractAndValidate(ctx context.Context, req ExtractRequest, mediaType string) (*NFOResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	res, err := s.provider.Extract(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("llm extract: %w", err)
	}

	validated, _ := ValidateAgainstTMDB(ctx, res, s.tmdbValidator, mediaType)
	return validated, nil
}

func buildIDs(res *NFOResult) map[string]string {
	ids := make(map[string]string)
	if res == nil {
		return ids
	}
	if res.TMDBID > 0 {
		ids["tmdb"] = strconv.Itoa(res.TMDBID)
	}
	if res.IMDBID != "" {
		ids["imdb"] = res.IMDBID
	}
	return ids
}

func buildPeople(names []string, personType core.PersonType) []core.Person {
	if len(names) == 0 {
		return nil
	}

	people := make([]core.Person, 0, len(names))
	for idx, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		people = append(people, core.Person{
			Type:  personType,
			Name:  name,
			Order: idx,
		})
	}
	if len(people) == 0 {
		return nil
	}
	return people
}

func buildMovieHints(opts core.MovieSearchOptions) map[string]string {
	hints := make(map[string]string)
	if opts.TMDBID > 0 {
		hints["tmdb_id"] = strconv.Itoa(opts.TMDBID)
	}
	if opts.IMDBID != "" {
		hints["imdb_id"] = strings.TrimSpace(opts.IMDBID)
	}
	if opts.Year > 0 {
		hints["year"] = strconv.Itoa(opts.Year)
	}
	if len(hints) == 0 {
		return nil
	}
	return hints
}

func buildTVShowHints(opts core.TvShowSearchOptions) map[string]string {
	hints := make(map[string]string)
	if opts.TMDBID > 0 {
		hints["tmdb_id"] = strconv.Itoa(opts.TMDBID)
	}
	if opts.TVDBID > 0 {
		hints["tvdb_id"] = strconv.Itoa(opts.TVDBID)
	}
	if opts.IMDBID != "" {
		hints["imdb_id"] = strings.TrimSpace(opts.IMDBID)
	}
	if opts.FirstAirYear > 0 {
		hints["first_air_year"] = strconv.Itoa(opts.FirstAirYear)
	} else if opts.Year > 0 {
		hints["year"] = strconv.Itoa(opts.Year)
	}
	if len(hints) == 0 {
		return nil
	}
	return hints
}

func buildEpisodeHints(opts core.TvShowEpisodeSearchOptions) map[string]string {
	hints := map[string]string{
		"season":  strconv.Itoa(opts.Season),
		"episode": strconv.Itoa(opts.Episode),
	}
	if opts.TvShowID > 0 {
		hints["tmdb_id"] = strconv.Itoa(opts.TvShowID)
	}
	return hints
}

func cloneStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]string, len(items))
	copy(cloned, items)
	return cloned
}
