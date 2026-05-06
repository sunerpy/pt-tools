package mcp_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
	ptmcp "github.com/sunerpy/pt-tools/internal/scraper/mcp"
	"github.com/sunerpy/pt-tools/internal/scraper/service"
	"github.com/sunerpy/pt-tools/internal/scraper/store"
)

type fixtureLLM struct {
	name   string
	result *ptmcp.MetadataResult
	err    error
}

func (f *fixtureLLM) Name() string { return f.name }
func (f *fixtureLLM) Kind() string { return "stub" }
func (f *fixtureLLM) Close() error { return nil }

func (f *fixtureLLM) Extract(_ context.Context, _ ptmcp.LLMExtractRequest) (*ptmcp.MetadataResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.result == nil {
		return &ptmcp.MetadataResult{Title: "fixture-title", Type: "movie", TMDBID: 99}, nil
	}
	r := *f.result
	return &r, nil
}

type fixtureWriter struct {
	writeMovie   int
	writeShow    int
	writeEpisode int
	shouldErr    bool
}

func (w *fixtureWriter) Dialect() string { return "universal" }

func (w *fixtureWriter) WriteMovieNfo(_ context.Context, _ *core.Movie, paths []string) error {
	if w.shouldErr {
		return errors.New("write failed")
	}
	w.writeMovie++
	for _, p := range paths {
		if err := os.WriteFile(p, []byte("<movie/>"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (w *fixtureWriter) WriteTvShowNfo(_ context.Context, _ *core.TvShow, dir string) error {
	if w.shouldErr {
		return errors.New("write failed")
	}
	w.writeShow++
	return os.WriteFile(filepath.Join(dir, "tvshow.nfo"), []byte("<tvshow/>"), 0o644)
}

func (w *fixtureWriter) WriteSeasonNfo(_ context.Context, _ *core.TvShowSeason, _ string) error {
	return nil
}

func (w *fixtureWriter) WriteEpisodeNfo(_ context.Context, _ *core.TvShowEpisode, path string) error {
	if w.shouldErr {
		return errors.New("write failed")
	}
	w.writeEpisode++
	return os.WriteFile(path, []byte("<episodedetails/>"), 0o644)
}

type fixtureConnector struct {
	refreshed   bool
	refreshArg  string
	shouldError bool
}

func (c *fixtureConnector) Name() string { return "jellyfin" }

func (c *fixtureConnector) Ping(context.Context) (*core.ServerInfo, error) {
	return &core.ServerInfo{Product: "jellyfin"}, nil
}

func (c *fixtureConnector) Authenticate(context.Context) (*core.ServerInfo, error) {
	return &core.ServerInfo{Product: "jellyfin"}, nil
}

func (c *fixtureConnector) ListLibraries(context.Context) ([]core.Library, error) { return nil, nil }

func (c *fixtureConnector) RefreshLibrary(_ context.Context, libraryID string) error {
	if c.shouldError {
		return errors.New("refresh failed")
	}
	c.refreshed = true
	c.refreshArg = libraryID
	return nil
}

func (c *fixtureConnector) ScanStatus(context.Context) (*core.ScanStatus, error) {
	return &core.ScanStatus{}, nil
}

type failingScraper struct{ info core.ProviderInfo }

func (f failingScraper) Info() core.ProviderInfo { return f.info }
func (f failingScraper) IsActive() bool          { return true }
func (f failingScraper) SearchMovie(context.Context, core.MovieSearchOptions) ([]core.MediaSearchCandidate, error) {
	return nil, errors.New("search-err")
}

func (f failingScraper) GetMovieMetadata(context.Context, core.MovieSearchOptions) (*core.Movie, error) {
	return nil, errors.New("get-err")
}

type mcpTestEnv struct {
	t            *testing.T
	db           *gorm.DB
	librarySvc   *service.LibraryService
	sourceReg    *core.Registry[core.MediaScraper]
	writerReg    *core.Registry[core.NfoWriter]
	connectorReg *core.Registry[core.MediaServerConnector]
	llmProv      *fixtureLLM
	writer       *fixtureWriter
	connector    *fixtureConnector
	client       *mcp.ClientSession
	cancel       context.CancelFunc
	libraryID    uint
}

func (e *mcpTestEnv) call(name string, args map[string]any) (*mcp.CallToolResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return e.client.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
}

func setupMCPEnv(t *testing.T, opts ...func(*ptmcp.Deps)) *mcpTestEnv {
	t.Helper()
	db := openMCPTestDB(t)
	librarySvc, err := service.NewLibraryService(service.LibraryConfig{DB: db, PathValidator: func(string) error { return nil }})
	require.NoError(t, err)
	lib, err := librarySvc.CreateLibrary(context.Background(), service.CreateLibraryRequest{Name: "Movies", Path: t.TempDir(), Type: "movie", ProviderIDs: []string{"tmdb"}})
	require.NoError(t, err)

	sourceReg := core.NewRegistry[core.MediaScraper]()
	require.NoError(t, sourceReg.Register("tmdb", func() core.MediaScraper { return apiSearchScraper{} }))

	writer := &fixtureWriter{}
	writerReg := core.NewRegistry[core.NfoWriter]()
	require.NoError(t, writerReg.Register("universal", func() core.NfoWriter { return writer }))

	connector := &fixtureConnector{}
	connectorReg := core.NewRegistry[core.MediaServerConnector]()
	require.NoError(t, connectorReg.Register("jellyfin", func() core.MediaServerConnector { return connector }))

	llm := &fixtureLLM{name: "stub"}

	fuser := &stubFuser{movie: &core.Movie{MediaEntity: core.MediaEntity{Title: "Inception", Year: 2010}}}
	scrapeSvc, err := service.NewScrapeService(service.ServiceConfig{
		DB: db, SourceReg: sourceReg, WriterReg: writerReg, ConnectorReg: connectorReg, Fuser: fuser,
	})
	require.NoError(t, err)

	deps := ptmcp.Deps{
		Scrape:       scrapeSvc,
		Library:      librarySvc,
		DB:           db,
		SourceReg:    sourceReg,
		WriterReg:    writerReg,
		ConnectorReg: connectorReg,
		LLMProviders: map[string]ptmcp.LLMProvider{"stub": llm},
	}
	for _, opt := range opts {
		opt(&deps)
	}

	server := ptmcp.NewMCPServer(deps)

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, nil)
	t1, t2 := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	ss, err := server.Connect(ctx, t1, nil)
	require.NoError(t, err)
	cs, err := client.Connect(ctx, t2, nil)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = cs.Close()
		_ = ss.Close()
		cancel()
	})

	return &mcpTestEnv{
		t: t, db: db, librarySvc: librarySvc,
		sourceReg: sourceReg, writerReg: writerReg, connectorReg: connectorReg,
		llmProv: llm, writer: writer, connector: connector,
		client: cs, cancel: cancel, libraryID: lib.ID,
	}
}

type stubFuser struct {
	movie *core.Movie
	show  *core.TvShow
	ep    *core.TvShowEpisode
	err   error
}

func (s *stubFuser) Merge(context.Context, map[string]*core.RawMediaInfo) (*core.Movie, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.movie == nil {
		return &core.Movie{MediaEntity: core.MediaEntity{Title: "Stub", Year: 2024}}, nil
	}
	return s.movie, nil
}

func (s *stubFuser) MergeTv(context.Context, map[string]*core.RawMediaInfo) (*core.TvShow, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.show == nil {
		return &core.TvShow{MediaEntity: core.MediaEntity{Title: "StubShow", Year: 2024}}, nil
	}
	return s.show, nil
}

func (s *stubFuser) MergeEpisode(context.Context, map[string]*core.RawMediaInfo) (*core.TvShowEpisode, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.ep == nil {
		return &core.TvShowEpisode{MediaEntity: core.MediaEntity{Title: "StubEp"}, Season: 1, Episode: 1}, nil
	}
	return s.ep, nil
}

func TestMCPTools_SearchMedia_Movie(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	res, err := env.call("scraper_search_media", map[string]any{"media_type": "movie", "query": "Inception"})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Contains(t, res.Content[0].(*mcp.TextContent).Text, "Inception")
}

func TestMCPTools_SearchMedia_TV(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	res, err := env.call("scraper_search_media", map[string]any{"media_type": "tv", "query": "Breaking"})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Contains(t, res.Content[0].(*mcp.TextContent).Text, "Breaking Bad")
}

func TestMCPTools_SearchMedia_InvalidType(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	res, err := env.call("scraper_search_media", map[string]any{"media_type": "nonsense", "query": "x"})
	require.NoError(t, err)
	require.True(t, res.IsError)
}

func TestMCPTools_SearchMedia_NoSourceReg(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t, func(d *ptmcp.Deps) { d.SourceReg = nil })
	res, err := env.call("scraper_search_media", map[string]any{"media_type": "movie", "query": "x"})
	require.NoError(t, err)
	require.True(t, res.IsError)
}

func TestMCPTools_GetMetadata_Movie(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	res, err := env.call("scraper_get_metadata", map[string]any{"media_type": "movie", "provider": "tmdb", "query": "Inception"})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Contains(t, res.Content[0].(*mcp.TextContent).Text, "Inception")
}

func TestMCPTools_GetMetadata_TV(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	res, err := env.call("scraper_get_metadata", map[string]any{"media_type": "tv", "provider": "tmdb", "query": "Breaking"})
	require.NoError(t, err)
	require.False(t, res.IsError)
}

func TestMCPTools_GetMetadata_Episode(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	res, err := env.call("scraper_get_metadata", map[string]any{"media_type": "episode", "provider": "tmdb", "tv_show_id": 1, "season": 1, "episode": 1})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Contains(t, res.Content[0].(*mcp.TextContent).Text, "Pilot")
}

func TestMCPTools_GetMetadata_InvalidProvider(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	_, err := env.call("scraper_get_metadata", map[string]any{"media_type": "movie", "provider": "missing", "query": "x"})
	require.Error(t, err)
}

func TestMCPTools_GetMetadata_InvalidType(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	_, err := env.call("scraper_get_metadata", map[string]any{"media_type": "nonsense", "provider": "tmdb"})
	require.Error(t, err)
}

func TestMCPTools_ScrapeFile_NoScrapeService(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t, func(d *ptmcp.Deps) { d.Scrape = nil })
	res, err := env.call("scraper_scrape_file", map[string]any{"media_type": "movie", "media_path": "/tmp/not-existing.mkv"})
	require.NoError(t, err)
	require.True(t, res.IsError)
}

func TestMCPTools_ScrapeFile_AutoDetectMedia(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	mediaDir := t.TempDir()
	mediaPath := filepath.Join(mediaDir, "Inception.2010.1080p.mkv")
	require.NoError(t, os.WriteFile(mediaPath, []byte("x"), 0o644))

	res, err := env.call("scraper_scrape_file", map[string]any{
		"media_path": mediaPath,
		"title":      "Inception",
		"year":       2010,
		"providers":  []string{"tmdb"},
	})
	require.NoError(t, err)
	_ = res
}

func TestMCPTools_ScrapeDirectory(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	mediaDir := t.TempDir()
	res, err := env.call("scraper_scrape_directory", map[string]any{
		"media_type": "tv",
		"media_path": mediaDir,
		"title":      "Breaking Bad",
		"providers":  []string{"tmdb"},
	})
	require.NoError(t, err)
	_ = res
}

func TestMCPTools_ListLibraries(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	res, err := env.call("scraper_list_libraries", map[string]any{})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Contains(t, res.Content[0].(*mcp.TextContent).Text, "Movies")
}

func TestMCPTools_GetTaskStatus(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	task := store.ScrapeTask{TaskType: "movie", MediaPath: "/a.mkv", State: "pending"}
	require.NoError(t, env.db.Create(&task).Error)

	res, err := env.call("scraper_get_task_status", map[string]any{"task_id": float64(task.ID)})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Contains(t, res.Content[0].(*mcp.TextContent).Text, "pending")
}

func TestMCPTools_GetTaskStatus_NotFound(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	res, err := env.call("scraper_get_task_status", map[string]any{"task_id": float64(9999)})
	require.NoError(t, err)
	require.True(t, res.IsError)
}

func TestMCPTools_ListTasks(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	for i := 0; i < 3; i++ {
		task := store.ScrapeTask{TaskType: "movie", MediaPath: "/a.mkv", State: "success"}
		require.NoError(t, env.db.Create(&task).Error)
	}
	res, err := env.call("scraper_list_tasks", map[string]any{"state": "success", "limit": float64(10)})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Contains(t, res.Content[0].(*mcp.TextContent).Text, "success")
}

func TestMCPTools_ParseFilename_Invalid(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	res, err := env.call("scraper_parse_filename", map[string]any{"path": ""})
	require.NoError(t, err)
	require.True(t, res.IsError)
}

func TestMCPTools_RefreshJellyfin(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	cfg := store.ConnectorConfig{Type: "jellyfin", Name: "test", BaseURL: "http://localhost"}
	require.NoError(t, env.db.Create(&cfg).Error)

	res, err := env.call("scraper_refresh_jellyfin", map[string]any{"connector_id": float64(cfg.ID), "library_id": "abc"})
	require.NoError(t, err)
	require.False(t, res.IsError)
	assert.True(t, env.connector.refreshed)
	assert.Equal(t, "abc", env.connector.refreshArg)
}

func TestMCPTools_RefreshJellyfin_NotFound(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	res, err := env.call("scraper_refresh_jellyfin", map[string]any{"connector_id": float64(9999)})
	require.NoError(t, err)
	require.True(t, res.IsError)
}

func TestMCPTools_RefreshJellyfin_NoDB(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t, func(d *ptmcp.Deps) { d.DB = nil })
	res, err := env.call("scraper_refresh_jellyfin", map[string]any{"connector_id": float64(1)})
	require.NoError(t, err)
	require.True(t, res.IsError)
}

func TestMCPTools_GetArtworks(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	res, err := env.call("scraper_get_artworks", map[string]any{
		"provider": "tmdb", "entity_id": "1", "media_type": "movie", "artwork_types": []string{"poster"},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Contains(t, res.Content[0].(*mcp.TextContent).Text, "poster")
}

func TestMCPTools_GetArtworks_InvalidProvider(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	res, err := env.call("scraper_get_artworks", map[string]any{"provider": "ghost", "entity_id": "1", "media_type": "movie"})
	require.NoError(t, err)
	require.True(t, res.IsError)
}

func TestMCPTools_WriteNFO_Movie(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	dir := t.TempDir()
	mediaPath := filepath.Join(dir, "Inception.2010.mkv")
	require.NoError(t, os.WriteFile(mediaPath, []byte("x"), 0o644))

	res, err := env.call("scraper_write_nfo", map[string]any{
		"media_type": "movie",
		"media_path": mediaPath,
		"metadata":   map[string]any{"title": "Inception", "year": 2010, "imdb_id": "tt1375666"},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.FileExists(t, filepath.Join(dir, "Inception.2010.nfo"))
	require.FileExists(t, filepath.Join(dir, "movie.nfo"))
}

func TestMCPTools_WriteNFO_TV(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	dir := t.TempDir()
	mediaPath := filepath.Join(dir, "Breaking.Bad.mkv")
	require.NoError(t, os.WriteFile(mediaPath, []byte("x"), 0o644))

	res, err := env.call("scraper_write_nfo", map[string]any{
		"media_type": "tv",
		"media_path": mediaPath,
		"metadata":   map[string]any{"title": "Breaking Bad"},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.FileExists(t, filepath.Join(dir, "tvshow.nfo"))
}

func TestMCPTools_WriteNFO_Episode(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	dir := t.TempDir()
	mediaPath := filepath.Join(dir, "S01E01.mkv")
	require.NoError(t, os.WriteFile(mediaPath, []byte("x"), 0o644))

	res, err := env.call("scraper_write_nfo", map[string]any{
		"media_type": "episode",
		"media_path": mediaPath,
		"metadata":   map[string]any{"title": "Pilot", "season": 1, "episode": 1},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
}

func TestMCPTools_WriteNFO_InvalidType(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	res, err := env.call("scraper_write_nfo", map[string]any{
		"media_type": "unknown",
		"media_path": "/x",
		"metadata":   map[string]any{"title": "x"},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
}

func TestMCPTools_WriteNFO_MissingDialect(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	res, err := env.call("scraper_write_nfo", map[string]any{
		"media_type":  "movie",
		"media_path":  "/x",
		"nfo_dialect": "nonexistent",
		"metadata":    map[string]any{"title": "x"},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
}

func TestMCPTools_ValidateMetadata_Clean(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	res, err := env.call("scraper_validate_metadata", map[string]any{
		"metadata": map[string]any{"title": "Inception", "year": 2010, "type": "movie"},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Contains(t, res.Content[0].(*mcp.TextContent).Text, `"valid":true`)
}

func TestMCPTools_ValidateMetadata_Warnings(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	res, err := env.call("scraper_validate_metadata", map[string]any{
		"metadata": map[string]any{"title": "", "year": 1800},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Contains(t, res.Content[0].(*mcp.TextContent).Text, `"valid":false`)
}

func TestMCPTools_MatchMedia(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	res, err := env.call("scraper_match_media", map[string]any{"media_type": "movie", "query": "Inception"})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Contains(t, res.Content[0].(*mcp.TextContent).Text, "Inception")
}

func TestMCPTools_MatchMedia_NoResults(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t, func(d *ptmcp.Deps) {
		reg := core.NewRegistry[core.MediaScraper]()
		d.SourceReg = reg
	})
	res, err := env.call("scraper_match_media", map[string]any{"media_type": "movie", "query": "x"})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Contains(t, res.Content[0].(*mcp.TextContent).Text, `"matched":false`)
}

func TestMCPTools_ProposeMatch_WithCandidates(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	res, err := env.call("scraper_propose_match", map[string]any{
		"media_type": "movie",
		"query":      "Inception",
		"candidates": []map[string]any{{"id": "1", "title": "Inception", "media_type": "movie", "provider": "tmdb"}},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Contains(t, res.Content[0].(*mcp.TextContent).Text, `"matched":true`)
}

func TestMCPTools_ProposeMatch_FallbackSearch(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	res, err := env.call("scraper_propose_match", map[string]any{
		"media_type": "movie", "query": "Inception", "provider": "stub",
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
}

func TestMCPTools_ProposeMatch_NoCandidates(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t, func(d *ptmcp.Deps) {
		d.SourceReg = core.NewRegistry[core.MediaScraper]()
	})
	res, err := env.call("scraper_propose_match", map[string]any{"media_type": "movie", "query": "x"})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Contains(t, res.Content[0].(*mcp.TextContent).Text, "no candidates")
}

func TestMCPTools_GenerateFromText(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	res, err := env.call("scraper_generate_metadata_from_text", map[string]any{
		"provider": "stub", "text": "Inception 2010", "media_type": "movie",
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Contains(t, res.Content[0].(*mcp.TextContent).Text, "fixture-title")
}

func TestMCPTools_GenerateFromText_NoProvider(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t, func(d *ptmcp.Deps) { d.LLMProviders = nil })
	res, err := env.call("scraper_generate_metadata_from_text", map[string]any{"text": "x"})
	require.NoError(t, err)
	require.True(t, res.IsError)
}

func TestMCPTools_EnrichPartial(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	res, err := env.call("scraper_enrich_partial", map[string]any{
		"provider":   "stub",
		"media_path": "/a/b.mkv",
		"metadata":   map[string]any{"title": "Base"},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
}

func TestMCPTools_EnrichPartial_NoProvider(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t, func(d *ptmcp.Deps) { d.LLMProviders = nil })
	res, err := env.call("scraper_enrich_partial", map[string]any{"metadata": map[string]any{"title": "x"}})
	require.NoError(t, err)
	require.True(t, res.IsError)
}

func TestMCPTools_ScrapeWithLLM_ErrorPath(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t, func(d *ptmcp.Deps) {
		d.LLMProviders = map[string]ptmcp.LLMProvider{"broken": &fixtureLLM{name: "broken", err: errors.New("boom")}}
	})
	res, err := env.call("scraper_scrape_with_llm", map[string]any{"provider": "broken", "raw_title": "x"})
	require.NoError(t, err)
	require.True(t, res.IsError)
}

func TestMCPResources_Libraries(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := env.client.ReadResource(ctx, &mcp.ReadResourceParams{URI: "scraper://libraries"})
	require.NoError(t, err)
	require.Len(t, res.Contents, 1)
	require.Contains(t, res.Contents[0].Text, "Movies")
}

func TestMCPResources_RecentTasks(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	task := store.ScrapeTask{TaskType: "movie", MediaPath: "/a.mkv", State: "success"}
	require.NoError(t, env.db.Create(&task).Error)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := env.client.ReadResource(ctx, &mcp.ReadResourceParams{URI: "scraper://tasks/recent"})
	require.NoError(t, err)
	require.Contains(t, res.Contents[0].Text, "success")
}

func TestMCPResources_Providers(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := env.client.ReadResource(ctx, &mcp.ReadResourceParams{URI: "scraper://providers"})
	require.NoError(t, err)
	require.Contains(t, res.Contents[0].Text, "tmdb")
	require.Contains(t, res.Contents[0].Text, "stub")
}

func TestMCPPrompts_All(t *testing.T) {
	t.Parallel()
	env := setupMCPEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cases := []struct {
		name string
		args map[string]string
	}{
		{"scrape-movie-workflow", map[string]string{"media_path": "/m.mkv", "providers": "tmdb"}},
		{"bulk-scrape-workflow", map[string]string{"library_name": "Movies", "mode": "movie"}},
		{"llm-scrape-with-context", map[string]string{"raw_title": "Inception.2010", "hints": "pt-site"}},
	}
	for _, tc := range cases {
		res, err := env.client.GetPrompt(ctx, &mcp.GetPromptParams{Name: tc.name, Arguments: tc.args})
		require.NoError(t, err)
		require.NotEmpty(t, res.Messages, "prompt=%s", tc.name)
	}

	empty, err := env.client.GetPrompt(ctx, &mcp.GetPromptParams{Name: "scrape-movie-workflow"})
	require.NoError(t, err)
	require.NotEmpty(t, empty.Messages)
}

func TestNewMCPServer_PartialDeps(t *testing.T) {
	t.Parallel()
	srv := ptmcp.NewMCPServer(ptmcp.Deps{})
	require.NotNil(t, srv)
	srv2 := ptmcp.NewMCPServer(ptmcp.Deps{SourceReg: core.NewRegistry[core.MediaScraper]()})
	require.NotNil(t, srv2)
}

func TestServeStdio_NilServer(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := ptmcp.ServeStdio(ctx, nil)
	require.Error(t, err)
}

func TestServeHTTP_Validation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := ptmcp.ServeHTTP(ctx, nil, ":0")
	require.Error(t, err)

	srv := ptmcp.NewMCPServer(ptmcp.Deps{})
	err = ptmcp.ServeHTTP(ctx, srv, "")
	require.Error(t, err)
}

func TestServeHTTP_Shutdown(t *testing.T) {
	t.Parallel()
	srv := ptmcp.NewMCPServer(ptmcp.Deps{})
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	err := ptmcp.ServeHTTP(ctx, srv, "127.0.0.1:0")
	if err != nil {
		require.ErrorContains(t, err, "address")
	}
}
