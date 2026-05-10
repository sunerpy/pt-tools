package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
	"github.com/sunerpy/pt-tools/internal/scraper/store"
)

type mockMovieScraper struct {
	info          core.ProviderInfo
	searchResults []core.MediaSearchCandidate
	movie         *core.Movie
	searchErr     error
	getErr        error
}

func (m *mockMovieScraper) Info() core.ProviderInfo { return m.info }
func (m *mockMovieScraper) IsActive() bool          { return true }

func (m *mockMovieScraper) SearchMovie(
	context.Context,
	core.MovieSearchOptions,
) ([]core.MediaSearchCandidate, error) {
	return m.searchResults, m.searchErr
}

func (m *mockMovieScraper) GetMovieMetadata(
	context.Context,
	core.MovieSearchOptions,
) (*core.Movie, error) {
	return m.movie, m.getErr
}

type mockWriter struct {
	moviePaths [][]string
	writeErr   error
}

func (m *mockWriter) Dialect() string { return defaultNfoDialect }

func (m *mockWriter) WriteMovieNfo(_ context.Context, movie *core.Movie, paths []string) error {
	if m.writeErr != nil {
		return m.writeErr
	}
	m.moviePaths = append(m.moviePaths, append([]string(nil), paths...))
	for _, path := range paths {
		content := []byte(movie.Title)
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockWriter) WriteTvShowNfo(context.Context, *core.TvShow, string) error { return nil }

func (m *mockWriter) WriteSeasonNfo(context.Context, *core.TvShowSeason, string) error { return nil }

func (m *mockWriter) WriteEpisodeNfo(context.Context, *core.TvShowEpisode, string) error { return nil }

type mockFuser struct {
	merged *core.Movie
	err    error
	seen   map[string]*core.RawMediaInfo
}

func (m *mockFuser) Merge(_ context.Context, sources map[string]*core.RawMediaInfo) (*core.Movie, error) {
	m.seen = sources
	if m.err != nil {
		return nil, m.err
	}
	if m.merged != nil {
		return m.merged, nil
	}
	for _, raw := range sources {
		if movie, ok := raw.Data.(*core.Movie); ok {
			return movie, nil
		}
	}
	return nil, errors.New("no movie to merge")
}

func (m *mockFuser) MergeTv(context.Context, map[string]*core.RawMediaInfo) (*core.TvShow, error) {
	return nil, errors.New("not implemented")
}

func (m *mockFuser) MergeEpisode(context.Context, map[string]*core.RawMediaInfo) (*core.TvShowEpisode, error) {
	return nil, errors.New("not implemented")
}

func TestScrape_Movie_EndToEnd_Mock(t *testing.T) {
	db := openTestDB(t)
	mediaDir := t.TempDir()
	mediaPath := filepath.Join(mediaDir, "Inception (2010).mkv")
	require.NoError(t, os.WriteFile(mediaPath, []byte("dummy"), 0o644))

	sourceReg := core.NewRegistry[core.MediaScraper]()
	writerReg := core.NewRegistry[core.NfoWriter]()
	movie := &core.Movie{MediaEntity: core.MediaEntity{Title: "Inception", Year: 2010}}
	require.NoError(t, sourceReg.Register("tmdb", func() core.MediaScraper {
		return &mockMovieScraper{
			info:          core.ProviderInfo{Name: "tmdb"},
			searchResults: []core.MediaSearchCandidate{{ID: "27205", Title: "Inception", Year: 2010}},
			movie:         movie,
		}
	}))
	writer := &mockWriter{}
	require.NoError(t, writerReg.Register(defaultNfoDialect, func() core.NfoWriter { return writer }))
	fuser := &mockFuser{merged: movie}

	svc, err := NewScrapeService(ServiceConfig{
		DB:        db,
		SourceReg: sourceReg,
		WriterReg: writerReg,
		Fuser:     fuser,
	})
	require.NoError(t, err)

	result, err := svc.ScrapeMovie(context.Background(), ScrapeMovieRequest{
		MediaPath: mediaPath,
		Title:     "Inception",
		Year:      2010,
		Providers: []string{"tmdb"},
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, stageDone, result.CurrentStage)
	require.Equal(t, []string{"tmdb"}, result.Providers)
	require.FileExists(t, result.NfoPath)
	require.FileExists(t, filepath.Join(mediaDir, "movie.nfo"))
	require.Len(t, writer.moviePaths, 1)

	var count int64
	require.NoError(t, db.Table("scrape_results").Count(&count).Error)
	require.Equal(t, int64(1), count)
	require.NotNil(t, fuser.seen["tmdb"])
}

func TestScrape_Movie_NoSource_Fails(t *testing.T) {
	db := openTestDB(t)
	svc, err := NewScrapeService(ServiceConfig{
		DB:        db,
		SourceReg: core.NewRegistry[core.MediaScraper](),
		WriterReg: core.NewRegistry[core.NfoWriter](),
		Fuser:     &mockFuser{},
	})
	require.NoError(t, err)

	result, err := svc.ScrapeMovie(context.Background(), ScrapeMovieRequest{
		MediaPath: "/tmp/Inception (2010).mkv",
		Title:     "Inception",
		Providers: []string{"tmdb"},
	})
	require.Error(t, err)
	require.False(t, result.Success)
	require.Equal(t, stageSearching, result.FailedStage)
	require.Contains(t, result.ErrorMessage, "no provider returned results")
	require.Contains(t, result.ErrorMessage, "未注册: [tmdb]")
	require.True(t, core.IsPermanent(err), "unregistered-only error should be permanent")
}

func TestScrape_Movie_SearchFails(t *testing.T) {
	db := openTestDB(t)
	sourceReg := core.NewRegistry[core.MediaScraper]()
	writerReg := core.NewRegistry[core.NfoWriter]()
	require.NoError(t, sourceReg.Register("tmdb", func() core.MediaScraper {
		return &mockMovieScraper{
			info:      core.ProviderInfo{Name: "tmdb"},
			searchErr: errors.New("boom"),
		}
	}))
	require.NoError(t, writerReg.Register(defaultNfoDialect, func() core.NfoWriter { return &mockWriter{} }))

	svc, err := NewScrapeService(ServiceConfig{
		DB:        db,
		SourceReg: sourceReg,
		WriterReg: writerReg,
		Fuser:     &mockFuser{},
	})
	require.NoError(t, err)

	result, err := svc.ScrapeMovie(context.Background(), ScrapeMovieRequest{
		MediaPath: "/tmp/Inception (2010).mkv",
		Title:     "Inception",
		Providers: []string{"tmdb"},
	})
	require.Error(t, err)
	require.False(t, result.Success)
	require.Equal(t, stageSearching, result.FailedStage)
	require.Contains(t, result.ErrorMessage, "no provider returned results")
	require.Contains(t, result.ErrorMessage, "空结果: [tmdb]")
	require.False(t, core.IsPermanent(err), "provider-called-but-empty should be transient")
}

func TestScrape_Movie_FilenameFallback(t *testing.T) {
	db := openTestDB(t)
	mediaDir := t.TempDir()
	mediaPath := filepath.Join(mediaDir, "Interstellar.2014.1080p.BluRay.mkv")
	require.NoError(t, os.WriteFile(mediaPath, []byte("dummy"), 0o644))

	sourceReg := core.NewRegistry[core.MediaScraper]()
	writerReg := core.NewRegistry[core.NfoWriter]()
	movie := &core.Movie{MediaEntity: core.MediaEntity{Title: "Interstellar", Year: 2014}}
	require.NoError(t, sourceReg.Register("tmdb", func() core.MediaScraper {
		return &mockMovieScraper{
			info:          core.ProviderInfo{Name: "tmdb"},
			searchResults: []core.MediaSearchCandidate{{ID: "157336", Title: "Interstellar", Year: 2014}},
			movie:         movie,
		}
	}))
	require.NoError(t, writerReg.Register(defaultNfoDialect, func() core.NfoWriter { return &mockWriter{} }))

	svc, err := NewScrapeService(ServiceConfig{
		DB:        db,
		SourceReg: sourceReg,
		WriterReg: writerReg,
		Fuser:     &mockFuser{merged: movie},
	})
	require.NoError(t, err)

	result, err := svc.ScrapeMovie(context.Background(), ScrapeMovieRequest{
		MediaPath: mediaPath,
		Providers: []string{"tmdb"},
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "Interstellar", result.Title)
	require.Equal(t, 2014, result.Year)
}

func TestScrape_Movie_NfoPathCorrect(t *testing.T) {
	db := openTestDB(t)
	mediaDir := t.TempDir()
	mediaPath := filepath.Join(mediaDir, "Inception (2010).mkv")
	require.NoError(t, os.WriteFile(mediaPath, []byte("dummy"), 0o644))

	sourceReg := core.NewRegistry[core.MediaScraper]()
	writerReg := core.NewRegistry[core.NfoWriter]()
	writer := &mockWriter{}
	movie := &core.Movie{MediaEntity: core.MediaEntity{Title: "Inception", Year: 2010}}
	require.NoError(t, sourceReg.Register("tmdb", func() core.MediaScraper {
		return &mockMovieScraper{
			info:          core.ProviderInfo{Name: "tmdb"},
			searchResults: []core.MediaSearchCandidate{{ID: "27205", Title: "Inception", Year: 2010}},
			movie:         movie,
		}
	}))
	require.NoError(t, writerReg.Register(defaultNfoDialect, func() core.NfoWriter { return writer }))

	svc, err := NewScrapeService(ServiceConfig{
		DB:        db,
		SourceReg: sourceReg,
		WriterReg: writerReg,
		Fuser:     &mockFuser{merged: movie},
	})
	require.NoError(t, err)

	result, err := svc.ScrapeMovie(context.Background(), ScrapeMovieRequest{
		MediaPath: mediaPath,
		Title:     "Inception",
		Year:      2010,
		Providers: []string{"tmdb"},
	})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(mediaDir, "Inception (2010).nfo"), result.NfoPath)
	require.Len(t, writer.moviePaths, 1)
	require.Equal(t, []string{
		filepath.Join(mediaDir, "Inception (2010).nfo"),
		filepath.Join(mediaDir, "movie.nfo"),
	}, writer.moviePaths[0])
}

func TestScrape_Movie_TaskBuilder_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	sourceReg := core.NewRegistry[core.MediaScraper]()
	writerReg := core.NewRegistry[core.NfoWriter]()
	movie := &core.Movie{MediaEntity: core.MediaEntity{Title: "Inception", Year: 2010, ScrapedAt: time.Now()}}
	require.NoError(t, sourceReg.Register("tmdb", func() core.MediaScraper {
		return &mockMovieScraper{
			info:          core.ProviderInfo{Name: "tmdb"},
			searchResults: []core.MediaSearchCandidate{{ID: "27205", Title: "Inception", Year: 2010}},
			movie:         movie,
		}
	}))
	require.NoError(t, writerReg.Register(defaultNfoDialect, func() core.NfoWriter { return &mockWriter{} }))

	svc, err := NewScrapeService(ServiceConfig{
		DB:        db,
		SourceReg: sourceReg,
		WriterReg: writerReg,
		Fuser:     &mockFuser{merged: movie},
	})
	require.NoError(t, err)

	record := store.ScrapeTask{
		ID:          42,
		TaskType:    "movie",
		MediaPath:   filepath.Join(t.TempDir(), "Inception (2010).mkv"),
		MaxRetries:  5,
		RequestData: `{"media_path":"/tmp/Inception (2010).mkv","title":"Inception","year":2010,"providers":["tmdb"]}`,
	}
	task := svc.TaskBuilder()(record)
	require.NotNil(t, task)
	require.Equal(t, "movie", task.Type())
	require.Equal(t, 5, task.MaxRetries())
	require.Equal(t, "scrape-movie-42", task.ID())
}
