package tmdb

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	tmdbsdk "github.com/cyruzin/golang-tmdb"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

const appendToResponse = "credits,images,external_ids,videos"

var providerInfo = core.ProviderInfo{
	Name:        "tmdb",
	DisplayName: "The Movie Database",
	Version:     "v3",
	Priority:    100,
	Kind:        "all",
}

type Config struct {
	BearerToken         string
	APIKey              string
	HTTPClient          *http.Client
	Language            string
	FallbackLanguage    string
	BaseURL             string
	UseAlternateBaseURL bool
}

type TMDBScraper struct {
	client       *tmdbsdk.Client
	info         core.ProviderInfo
	language     string
	fallbackLang string
}

func init() {}

func Register(registry *core.ScraperRegistry, cfg Config) error {
	if registry == nil {
		return fmt.Errorf("register tmdb scraper: %w", core.ErrInvalidID)
	}
	scraper, err := NewTMDBScraper(cfg)
	if err != nil {
		return err
	}
	return registry.Register(providerInfo.Name, func() core.MediaScraper {
		return scraper
	})
}

func NewTMDBScraper(cfg Config) (*TMDBScraper, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}

	language := strings.TrimSpace(cfg.Language)
	if language == "" {
		language = "zh-CN"
	}
	fallback := strings.TrimSpace(cfg.FallbackLanguage)
	if fallback == "" {
		fallback = "en-US"
	}

	s := &TMDBScraper{
		client:       client,
		info:         providerInfo,
		language:     language,
		fallbackLang: fallback,
	}
	s.configureImageBaseURL()
	return s, nil
}

func newClient(cfg Config) (*tmdbsdk.Client, error) {
	var (
		client *tmdbsdk.Client
		err    error
	)
	if strings.TrimSpace(cfg.BearerToken) != "" {
		client, err = tmdbsdk.InitV4(strings.TrimSpace(cfg.BearerToken))
	} else if strings.TrimSpace(cfg.APIKey) != "" {
		client, err = tmdbsdk.Init(strings.TrimSpace(cfg.APIKey))
	} else {
		return nil, core.Wrap(core.ErrUnauthorized, "tmdb 凭证未配置")
	}
	if err != nil {
		return nil, mapTMDBError(err, "初始化 tmdb client 失败")
	}

	if cfg.HTTPClient != nil {
		client.SetClientConfig(*cfg.HTTPClient)
	}
	client.SetClientAutoRetry()

	if strings.TrimSpace(cfg.BaseURL) != "" {
		client.SetCustomBaseURL(strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"))
	} else if cfg.UseAlternateBaseURL {
		client.SetAlternateBaseURL()
	}

	return client, nil
}

func (s *TMDBScraper) Info() core.ProviderInfo {
	return s.info
}

func (s *TMDBScraper) IsActive() bool {
	return s.client != nil
}

func (s *TMDBScraper) SearchMovie(ctx context.Context, opts core.MovieSearchOptions) ([]core.MediaSearchCandidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		return nil, fmt.Errorf("search movie: %w", core.ErrInvalidID)
	}

	primaryLang := s.pickLanguage(opts.Language, s.language)
	results, err := s.searchMovies(query, opts, primaryLang)
	if err != nil {
		return nil, err
	}
	fallbackLang := s.pickLanguage(opts.FallbackLanguage, s.fallbackLang)
	if needsSearchFallback(results) && fallbackLang != "" && fallbackLang != primaryLang {
		fallback, fallbackErr := s.searchMovies(query, opts, fallbackLang)
		if fallbackErr == nil {
			results = mergeCandidates(results, fallback)
		}
	}
	if len(results) == 0 {
		return nil, core.Wrap(core.ErrNotFound, "tmdb 未找到电影")
	}
	return results, nil
}

func (s *TMDBScraper) GetMovieMetadata(ctx context.Context, opts core.MovieSearchOptions) (*core.Movie, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id, err := s.resolveMovieID(ctx, opts)
	if err != nil {
		return nil, err
	}

	primaryLang := s.pickLanguage(opts.Language, s.language)
	raw, err := s.getMovieDetails(id, primaryLang)
	if err != nil {
		return nil, err
	}
	fallbackLang := s.pickLanguage(opts.FallbackLanguage, s.fallbackLang)
	if shouldRetryWithFallback(raw.Title, raw.Overview) && fallbackLang != "" && fallbackLang != primaryLang {
		fallback, fallbackErr := s.getMovieDetails(id, fallbackLang)
		if fallbackErr == nil {
			overlayMovieText(raw, fallback)
		}
	}
	return tmdbToMovie(raw), nil
}

func (s *TMDBScraper) SearchTvShow(ctx context.Context, opts core.TvShowSearchOptions) ([]core.MediaSearchCandidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		return nil, fmt.Errorf("search tv show: %w", core.ErrInvalidID)
	}

	primaryLang := s.pickLanguage(opts.Language, s.language)
	results, err := s.searchTVShows(query, opts, primaryLang)
	if err != nil {
		return nil, err
	}
	fallbackLang := s.pickLanguage(opts.FallbackLanguage, s.fallbackLang)
	if needsSearchFallback(results) && fallbackLang != "" && fallbackLang != primaryLang {
		fallback, fallbackErr := s.searchTVShows(query, opts, fallbackLang)
		if fallbackErr == nil {
			results = mergeCandidates(results, fallback)
		}
	}
	if len(results) == 0 {
		return nil, core.Wrap(core.ErrNotFound, "tmdb 未找到剧集")
	}
	return results, nil
}

func (s *TMDBScraper) GetTvShowMetadata(ctx context.Context, opts core.TvShowSearchOptions) (*core.TvShow, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id, err := s.resolveTVShowID(ctx, opts)
	if err != nil {
		return nil, err
	}

	primaryLang := s.pickLanguage(opts.Language, s.language)
	raw, err := s.getTVDetails(id, primaryLang)
	if err != nil {
		return nil, err
	}
	fallbackLang := s.pickLanguage(opts.FallbackLanguage, s.fallbackLang)
	if shouldRetryWithFallback(raw.Name, raw.Overview) && fallbackLang != "" && fallbackLang != primaryLang {
		fallback, fallbackErr := s.getTVDetails(id, fallbackLang)
		if fallbackErr == nil {
			overlayTVText(raw, fallback)
		}
	}
	return tmdbToTvShow(raw), nil
}

func (s *TMDBScraper) GetEpisodeList(ctx context.Context, opts core.TvShowSearchOptions) ([]core.TvShowEpisode, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id, err := s.resolveTVShowID(ctx, opts)
	if err != nil {
		return nil, err
	}
	show, err := s.getTVDetails(id, s.pickLanguage(opts.Language, s.language))
	if err != nil {
		return nil, err
	}

	episodes := make([]core.TvShowEpisode, 0)
	for _, season := range show.Seasons {
		seasonDetails, seasonErr := s.client.GetTVSeasonDetails(id, season.SeasonNumber, map[string]string{
			"language": s.pickLanguage(opts.Language, s.language),
		})
		if seasonErr != nil {
			return nil, mapTMDBError(seasonErr, "获取 tmdb 季信息失败")
		}
		for _, item := range seasonDetails.Episodes {
			episode := tmdbToEpisode(&tmdbsdk.TVEpisodeDetails{
				AirDate:       item.AirDate,
				EpisodeNumber: item.EpisodeNumber,
				Name:          item.Name,
				Overview:      item.Overview,
				ID:            item.ID,
				Runtime:       item.Runtime,
				SeasonNumber:  item.SeasonNumber,
				StillPath:     item.StillPath,
				VoteMetrics:   item.VoteMetrics,
				Crew:          item.Crew,
				GuestStars:    item.GuestStars,
			})
			if episode != nil {
				episodes = append(episodes, *episode)
			}
		}
	}
	return episodes, nil
}

func (s *TMDBScraper) GetEpisodeMetadata(ctx context.Context, opts core.TvShowEpisodeSearchOptions) (*core.TvShowEpisode, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opts.TvShowID <= 0 || opts.Season < 0 || opts.Episode <= 0 {
		return nil, fmt.Errorf("get episode metadata: %w", core.ErrInvalidID)
	}

	primaryLang := s.pickLanguage(opts.Language, s.language)
	raw, err := s.getEpisodeDetails(opts.TvShowID, opts.Season, opts.Episode, primaryLang)
	if err != nil {
		return nil, err
	}
	if shouldRetryWithFallback(raw.Name, raw.Overview) && s.fallbackLang != "" && s.fallbackLang != primaryLang {
		fallback, fallbackErr := s.getEpisodeDetails(opts.TvShowID, opts.Season, opts.Episode, s.fallbackLang)
		if fallbackErr == nil {
			overlayEpisodeText(raw, fallback)
		}
	}
	return tmdbToEpisode(raw), nil
}

func (s *TMDBScraper) GetArtwork(ctx context.Context, opts core.ArtworkSearchOptions) ([]core.MediaArtwork, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch opts.Type {
	case core.MediaTypeMovie:
		id, err := strconv.Atoi(opts.EntityID)
		if err != nil {
			return nil, fmt.Errorf("parse movie artwork id: %w", core.ErrInvalidID)
		}
		movie, getErr := s.getMovieDetails(id, s.pickLanguage(opts.Language, s.language))
		if getErr != nil {
			return nil, getErr
		}
		return filterArtworkTypes(movieArtworks(movie), opts.ArtworkTypes), nil
	case core.MediaTypeTvShow:
		id, err := strconv.Atoi(opts.EntityID)
		if err != nil {
			return nil, fmt.Errorf("parse tv artwork id: %w", core.ErrInvalidID)
		}
		show, getErr := s.getTVDetails(id, s.pickLanguage(opts.Language, s.language))
		if getErr != nil {
			return nil, getErr
		}
		return filterArtworkTypes(tvArtworks(show), opts.ArtworkTypes), nil
	case core.MediaTypeEpisode:
		showID, season, episode, err := parseEpisodeEntityID(opts.EntityID)
		if err != nil {
			return nil, err
		}
		item, getErr := s.getEpisodeDetails(showID, season, episode, s.pickLanguage(opts.Language, s.language))
		if getErr != nil {
			return nil, getErr
		}
		return filterArtworkTypes(episodeArtworks(item), opts.ArtworkTypes), nil
	default:
		return nil, fmt.Errorf("get artwork: unsupported media type %q", opts.Type.String())
	}
}

func (s *TMDBScraper) searchMovies(query string, opts core.MovieSearchOptions, language string) ([]core.MediaSearchCandidate, error) {
	params := map[string]string{"language": language}
	if opts.Year > 0 {
		params["year"] = strconv.Itoa(opts.Year)
	}
	if opts.IncludeAdult {
		params["include_adult"] = "true"
	}
	res, err := s.client.GetSearchMovies(query, params)
	if err != nil {
		return nil, mapTMDBError(err, "搜索 tmdb 电影失败")
	}
	results := make([]core.MediaSearchCandidate, 0, len(res.Results))
	for _, item := range res.Results {
		results = append(results, core.MediaSearchCandidate{
			ID:        strconv.FormatInt(item.ID, 10),
			Title:     item.Title,
			Year:      parseYear(item.ReleaseDate),
			MediaType: core.MediaTypeMovie,
			Provider:  providerInfo.Name,
			PosterURL: fullImageURL(item.PosterPath),
			Overview:  item.Overview,
			Score:     float64(item.VoteAverage) / 10,
		})
	}
	return results, nil
}

func (s *TMDBScraper) searchTVShows(query string, opts core.TvShowSearchOptions, language string) ([]core.MediaSearchCandidate, error) {
	params := map[string]string{"language": language}
	if opts.FirstAirYear > 0 {
		params["first_air_date_year"] = strconv.Itoa(opts.FirstAirYear)
	}
	if opts.IncludeAdult {
		params["include_adult"] = "true"
	}
	res, err := s.client.GetSearchTVShow(query, params)
	if err != nil {
		return nil, mapTMDBError(err, "搜索 tmdb 剧集失败")
	}
	results := make([]core.MediaSearchCandidate, 0, len(res.Results))
	for _, item := range res.Results {
		results = append(results, core.MediaSearchCandidate{
			ID:        strconv.FormatInt(item.ID, 10),
			Title:     item.Name,
			Year:      parseYear(item.FirstAirDate),
			MediaType: core.MediaTypeTvShow,
			Provider:  providerInfo.Name,
			PosterURL: fullImageURL(item.PosterPath),
			Overview:  item.Overview,
			Score:     float64(item.VoteAverage) / 10,
		})
	}
	return results, nil
}

func (s *TMDBScraper) getMovieDetails(id int, language string) (*tmdbsdk.MovieDetails, error) {
	params := map[string]string{"append_to_response": appendToResponse}
	if language != "" {
		params["language"] = language
		params["include_image_language"] = strings.Join(compactStrings(language, s.fallbackLang, "null"), ",")
	}
	raw, err := s.client.GetMovieDetails(id, params)
	if err != nil {
		return nil, mapTMDBError(err, "获取 tmdb 电影详情失败")
	}
	return raw, nil
}

func (s *TMDBScraper) getTVDetails(id int, language string) (*tmdbsdk.TVDetails, error) {
	params := map[string]string{"append_to_response": appendToResponse}
	if language != "" {
		params["language"] = language
		params["include_image_language"] = strings.Join(compactStrings(language, s.fallbackLang, "null"), ",")
	}
	raw, err := s.client.GetTVDetails(id, params)
	if err != nil {
		return nil, mapTMDBError(err, "获取 tmdb 剧集详情失败")
	}
	return raw, nil
}

func (s *TMDBScraper) getEpisodeDetails(showID, season, episode int, language string) (*tmdbsdk.TVEpisodeDetails, error) {
	params := map[string]string{"append_to_response": appendToResponse}
	if language != "" {
		params["language"] = language
		params["include_image_language"] = strings.Join(compactStrings(language, s.fallbackLang, "null"), ",")
	}
	raw, err := s.client.GetTVEpisodeDetails(showID, season, episode, params)
	if err != nil {
		return nil, mapTMDBError(err, "获取 tmdb 分集详情失败")
	}
	return raw, nil
}

func (s *TMDBScraper) resolveMovieID(ctx context.Context, opts core.MovieSearchOptions) (int, error) {
	if opts.TMDBID > 0 {
		return opts.TMDBID, nil
	}
	results, err := s.SearchMovie(ctx, opts)
	if err != nil {
		return 0, err
	}
	id, convErr := strconv.Atoi(results[0].ID)
	if convErr != nil {
		return 0, fmt.Errorf("convert movie id: %w", core.ErrParseFailed)
	}
	return id, nil
}

func (s *TMDBScraper) resolveTVShowID(ctx context.Context, opts core.TvShowSearchOptions) (int, error) {
	if opts.TMDBID > 0 {
		return opts.TMDBID, nil
	}
	results, err := s.SearchTvShow(ctx, opts)
	if err != nil {
		return 0, err
	}
	id, convErr := strconv.Atoi(results[0].ID)
	if convErr != nil {
		return 0, fmt.Errorf("convert tv show id: %w", core.ErrParseFailed)
	}
	return id, nil
}

func (s *TMDBScraper) configureImageBaseURL() {
	config, err := s.client.GetConfigurationAPI()
	if err != nil {
		setImageBaseURL("")
		return
	}
	if config.Images.SecureBaseURL != "" {
		setImageBaseURL(config.Images.SecureBaseURL)
		return
	}
	setImageBaseURL(config.Images.BaseURL)
}

func (s *TMDBScraper) pickLanguage(preferred, fallback string) string {
	if strings.TrimSpace(preferred) != "" {
		return strings.TrimSpace(preferred)
	}
	return strings.TrimSpace(fallback)
}

func shouldRetryWithFallback(title, overview string) bool {
	return strings.TrimSpace(title) == "" || strings.TrimSpace(overview) == ""
}

func needsSearchFallback(results []core.MediaSearchCandidate) bool {
	for _, result := range results {
		if shouldRetryWithFallback(result.Title, result.Overview) {
			return true
		}
	}
	return false
}

func mergeCandidates(primary, fallback []core.MediaSearchCandidate) []core.MediaSearchCandidate {
	fallbackByID := make(map[string]core.MediaSearchCandidate, len(fallback))
	for _, item := range fallback {
		fallbackByID[item.ID] = item
	}
	for i := range primary {
		item := primary[i]
		fallbackItem, ok := fallbackByID[item.ID]
		if !ok {
			continue
		}
		if strings.TrimSpace(item.Title) == "" {
			primary[i].Title = fallbackItem.Title
		}
		if strings.TrimSpace(item.Overview) == "" {
			primary[i].Overview = fallbackItem.Overview
		}
		if primary[i].PosterURL == "" {
			primary[i].PosterURL = fallbackItem.PosterURL
		}
	}
	return primary
}

func overlayMovieText(dst, src *tmdbsdk.MovieDetails) {
	if dst == nil || src == nil {
		return
	}
	if strings.TrimSpace(dst.Title) == "" {
		dst.Title = src.Title
	}
	if strings.TrimSpace(dst.Overview) == "" {
		dst.Overview = src.Overview
	}
	if strings.TrimSpace(dst.Tagline) == "" {
		dst.Tagline = src.Tagline
	}
}

func overlayTVText(dst, src *tmdbsdk.TVDetails) {
	if dst == nil || src == nil {
		return
	}
	if strings.TrimSpace(dst.Name) == "" {
		dst.Name = src.Name
	}
	if strings.TrimSpace(dst.Overview) == "" {
		dst.Overview = src.Overview
	}
	if strings.TrimSpace(dst.Tagline) == "" {
		dst.Tagline = src.Tagline
	}
	for i := range dst.Seasons {
		if i >= len(src.Seasons) {
			break
		}
		if strings.TrimSpace(dst.Seasons[i].Name) == "" {
			dst.Seasons[i].Name = src.Seasons[i].Name
		}
		if strings.TrimSpace(dst.Seasons[i].Overview) == "" {
			dst.Seasons[i].Overview = src.Seasons[i].Overview
		}
	}
}

func overlayEpisodeText(dst, src *tmdbsdk.TVEpisodeDetails) {
	if dst == nil || src == nil {
		return
	}
	if strings.TrimSpace(dst.Name) == "" {
		dst.Name = src.Name
	}
	if strings.TrimSpace(dst.Overview) == "" {
		dst.Overview = src.Overview
	}
}

func parseEpisodeEntityID(raw string) (int, int, int, error) {
	parts := strings.FieldsFunc(strings.TrimSpace(raw), func(r rune) bool {
		return r == ':' || r == '/'
	})
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("parse episode artwork id: %w", core.ErrInvalidID)
	}
	showID, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse show id: %w", core.ErrInvalidID)
	}
	season, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse season id: %w", core.ErrInvalidID)
	}
	episode, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse episode id: %w", core.ErrInvalidID)
	}
	return showID, season, episode, nil
}

func filterArtworkTypes(artworks []core.MediaArtwork, allowed []core.ArtworkType) []core.MediaArtwork {
	if len(allowed) == 0 {
		return artworks
	}
	allowedSet := make(map[core.ArtworkType]struct{}, len(allowed))
	for _, item := range allowed {
		allowedSet[item] = struct{}{}
	}
	filtered := make([]core.MediaArtwork, 0, len(artworks))
	for _, item := range artworks {
		if _, ok := allowedSet[item.Type]; ok {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func compactStrings(values ...string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func mapTMDBError(err error, msg string) error {
	if err == nil {
		return nil
	}
	var apiErr tmdbsdk.Error
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusUnauthorized:
			return core.Wrap(core.ErrUnauthorized, msg)
		case http.StatusNotFound:
			return core.Wrap(core.ErrNotFound, msg)
		case http.StatusTooManyRequests:
			return core.Wrap(core.ErrRateLimited, msg)
		default:
			if apiErr.StatusCode >= http.StatusInternalServerError {
				return core.Wrap(core.ErrProviderDisabled, msg)
			}
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return core.Wrap(core.ErrTimeout, msg)
	}
	return core.Wrap(err, msg)
}
