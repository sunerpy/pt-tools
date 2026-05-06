package llm

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

func TestLLMSource_GetMovieMetadata_Offline(t *testing.T) {
	t.Parallel()

	src := mustNewLLMSource(t, SourceConfig{
		Provider: &stubSourceProvider{result: &NFOResult{Title: "Inception", Year: 2010, Type: "movie"}},
	})

	movie, err := src.GetMovieMetadata(context.Background(), core.MovieSearchOptions{Query: "Inception"})
	require.NoError(t, err)
	require.Equal(t, "Inception", movie.Title)
	require.Equal(t, "llm", movie.Provider)
	require.NotZero(t, movie.ScrapedAt)
	require.Empty(t, movie.ArtworkURLs)
}

func TestLLMSource_GetMovieMetadata_CrossValidate_Pass(t *testing.T) {
	t.Parallel()

	src := mustNewLLMSource(t, SourceConfig{
		Provider: &stubSourceProvider{result: &NFOResult{Title: "Inception", Year: 2010, Type: "movie", TMDBID: 27205}},
		TMDBValidator: stubTMDBValidator{
			movie: map[int]*ValidationInfo{27205: {Title: "Inception", Year: 2010, Type: "movie"}},
		},
	})

	movie, err := src.GetMovieMetadata(context.Background(), core.MovieSearchOptions{Query: "Inception"})
	require.NoError(t, err)
	require.Equal(t, "27205", movie.IDs["tmdb"])
}

func TestLLMSource_GetMovieMetadata_CrossValidate_Fail(t *testing.T) {
	t.Parallel()

	src := mustNewLLMSource(t, SourceConfig{
		Provider: &stubSourceProvider{result: &NFOResult{Title: "Inception", Year: 2010, Type: "movie", TMDBID: 12345}},
		TMDBValidator: stubTMDBValidator{
			movie: map[int]*ValidationInfo{12345: {Title: "Titanic", Year: 1997, Type: "movie"}},
		},
	})

	movie, err := src.GetMovieMetadata(context.Background(), core.MovieSearchOptions{Query: "Inception"})
	require.NoError(t, err)
	require.Empty(t, movie.IDs["tmdb"])
	_, exists := movie.IDs["tmdb"]
	require.False(t, exists)
}

func TestLLMSource_Info(t *testing.T) {
	t.Parallel()

	src := mustNewLLMSource(t, SourceConfig{Provider: &stubSourceProvider{result: &NFOResult{}}})
	info := src.Info()
	require.Equal(t, "llm", info.Name)
	require.Equal(t, "all", info.Kind)
	require.Equal(t, 10, info.Priority)
}

func TestLLMSource_SearchMovie_EmptyQuery(t *testing.T) {
	t.Parallel()

	src := mustNewLLMSource(t, SourceConfig{Provider: &stubSourceProvider{result: &NFOResult{}}})
	results, err := src.SearchMovie(context.Background(), core.MovieSearchOptions{})
	require.NoError(t, err)
	require.Nil(t, results)
}

func TestValidateFieldFormat_EdgeCases(t *testing.T) {
	t.Parallel()

	warnings := ValidateFieldFormat(&NFOResult{
		Year:   999999,
		TMDBID: 10000001,
		IMDBID: "bad-id",
	})
	require.Contains(t, warnings, "year too far in future")
	require.Contains(t, warnings, "tmdb_id out of range")
	require.Contains(t, warnings, "imdb_id invalid format")
}

func TestValidateAgainstTMDB_NilValidator(t *testing.T) {
	t.Parallel()

	res, warning := ValidateAgainstTMDB(context.Background(), &NFOResult{Title: "Inception", TMDBID: 1}, nil, "movie")
	require.NotNil(t, res)
	require.Equal(t, "TMDB 不可用，跳过 ID 验证", warning)
	require.Equal(t, 1, res.TMDBID)
}

func TestLLMSource_GetTvShowMetadata(t *testing.T) {
	t.Parallel()

	src := mustNewLLMSource(t, SourceConfig{
		Provider: &stubSourceProvider{result: &NFOResult{Title: "Breaking Bad", OriginalTitle: "Breaking Bad", Year: 2008, Type: "tv", Cast: []string{"Bryan Cranston"}, Directors: []string{"Vince Gilligan"}}},
	})

	show, err := src.GetTvShowMetadata(context.Background(), core.TvShowSearchOptions{Query: "Breaking Bad"})
	require.NoError(t, err)
	require.Equal(t, "Breaking Bad", show.Title)
	require.Equal(t, "llm", show.Provider)
	require.Len(t, show.Actors, 1)
	require.Len(t, show.Directors, 1)
	require.Empty(t, show.ArtworkURLs)
}

func TestLLMSource_GetEpisodeMetadata(t *testing.T) {
	t.Parallel()

	provider := &stubSourceProvider{result: &NFOResult{Title: "Pilot", Type: "tv", Season: 1, Episode: 1}}
	src := mustNewLLMSource(t, SourceConfig{Provider: provider})

	episode, err := src.GetEpisodeMetadata(context.Background(), core.TvShowEpisodeSearchOptions{TvShowID: 1396, Season: 1, Episode: 1, Language: "zh-CN"})
	require.NoError(t, err)
	require.Equal(t, "Pilot", episode.Title)
	require.Equal(t, 1, episode.Season)
	require.Equal(t, 1, episode.Episode)
	require.Equal(t, "tv", provider.lastReq.MediaType)
	require.Equal(t, "1396", provider.lastReq.SiteHints["tmdb_id"])
	require.Empty(t, episode.ArtworkURLs)
}

func TestLLMSource_GetEpisodeList_NotSupported(t *testing.T) {
	t.Parallel()

	src := mustNewLLMSource(t, SourceConfig{Provider: &stubSourceProvider{result: &NFOResult{}}})
	items, err := src.GetEpisodeList(context.Background(), core.TvShowSearchOptions{})
	require.Nil(t, items)
	require.EqualError(t, err, "llm: episode list not supported")
}

func TestNewLLMSource_RequiresProvider(t *testing.T) {
	t.Parallel()

	src, err := NewLLMSource(SourceConfig{})
	require.Nil(t, src)
	require.EqualError(t, err, "llm source: Provider required")
}

func TestValidateAgainstTMDB_ClearIDOnValidatorError(t *testing.T) {
	t.Parallel()

	res, warning := ValidateAgainstTMDB(context.Background(), &NFOResult{Title: "Inception", TMDBID: 27205}, stubTMDBValidator{movieErr: errors.New("boom")}, "movie")
	require.Equal(t, 0, res.TMDBID)
	require.Equal(t, "TMDB 验证失败，清空 ID", warning)
}

func mustNewLLMSource(t *testing.T, cfg SourceConfig) *LLMSource {
	t.Helper()
	src, err := NewLLMSource(cfg)
	require.NoError(t, err)
	return src
}

type stubSourceProvider struct {
	result  *NFOResult
	err     error
	lastReq ExtractRequest
}

func (s *stubSourceProvider) Name() string {
	return "stub"
}

func (s *stubSourceProvider) Kind() ProviderKind {
	return KindOpenAICompat
}

func (s *stubSourceProvider) Extract(ctx context.Context, req ExtractRequest) (*NFOResult, error) {
	_ = ctx
	s.lastReq = req
	if s.err != nil {
		return nil, s.err
	}
	if s.result == nil {
		return &NFOResult{}, nil
	}
	clone := *s.result
	return &clone, nil
}

func (s *stubSourceProvider) Close() error {
	return nil
}

type stubTMDBValidator struct {
	movie    map[int]*ValidationInfo
	tv       map[int]*ValidationInfo
	movieErr error
	tvErr    error
}

func (s stubTMDBValidator) ValidateMovie(_ context.Context, tmdbID int) (*ValidationInfo, error) {
	if s.movieErr != nil {
		return nil, s.movieErr
	}
	return s.movie[tmdbID], nil
}

func (s stubTMDBValidator) ValidateTvShow(_ context.Context, tmdbID int) (*ValidationInfo, error) {
	if s.tvErr != nil {
		return nil, s.tvErr
	}
	return s.tv[tmdbID], nil
}
