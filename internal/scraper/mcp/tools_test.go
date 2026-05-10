package mcp_test

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	llmsource "github.com/sunerpy/pt-tools/internal/scraper/source/llm"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
	ptmcp "github.com/sunerpy/pt-tools/internal/scraper/mcp"
	"github.com/sunerpy/pt-tools/internal/scraper/service"
	"github.com/sunerpy/pt-tools/internal/scraper/store"
)

type mcpLLMAdapter struct{ provider llmsource.Provider }

func (a mcpLLMAdapter) Name() string { return a.provider.Name() }
func (a mcpLLMAdapter) Kind() string { return string(a.provider.Kind()) }
func (a mcpLLMAdapter) Close() error { return a.provider.Close() }
func (a mcpLLMAdapter) Extract(ctx context.Context, req ptmcp.LLMExtractRequest) (*ptmcp.MetadataResult, error) {
	result, err := a.provider.Extract(ctx, llmsource.ExtractRequest{
		RawTitle:    req.RawTitle,
		Filename:    req.Filename,
		FileSize:    req.FileSize,
		SiteHints:   req.SiteHints,
		UserContext: req.UserContext,
		Language:    req.Language,
		MediaType:   req.MediaType,
	})
	if err != nil {
		return nil, err
	}
	return &ptmcp.MetadataResult{
		Title:         result.Title,
		OriginalTitle: result.OriginalTitle,
		Year:          result.Year,
		Type:          result.Type,
		TMDBID:        result.TMDBID,
		IMDBID:        result.IMDBID,
		Season:        result.Season,
		Episode:       result.Episode,
		Genres:        append([]string(nil), result.Genres...),
		Language:      result.Language,
		Plot:          result.Plot,
		Directors:     append([]string(nil), result.Directors...),
		Cast:          append([]string(nil), result.Cast...),
		Runtime:       result.Runtime,
		GeneratedBy:   result.GeneratedBy,
	}, nil
}

func openMCPTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, store.Migrate(db))
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func newMCPTestServer(t *testing.T) *mcp.Server {
	t.Helper()
	db := openMCPTestDB(t)
	librarySvc, err := service.NewLibraryService(service.LibraryConfig{DB: db, PathValidator: func(string) error { return nil }})
	require.NoError(t, err)
	lib, err := librarySvc.CreateLibrary(context.Background(), service.CreateLibraryRequest{Name: "Movies", Path: t.TempDir(), Type: "movie", ProviderIDs: []string{"tmdb"}})
	require.NoError(t, err)
	require.NotZero(t, lib.ID)

	sourceReg := core.NewRegistry[core.MediaScraper]()
	require.NoError(t, sourceReg.Register("tmdb", func() core.MediaScraper { return apiSearchScraper{} }))

	provider := mcpLLMAdapter{provider: llmsource.NewSamplingProvider()}
	return ptmcp.NewMCPServer(ptmcp.Deps{
		Library:      librarySvc,
		DB:           db,
		SourceReg:    sourceReg,
		LLMProviders: map[string]ptmcp.LLMProvider{"sampling": provider},
	})
}

type apiSearchScraper struct{}

func (apiSearchScraper) Info() core.ProviderInfo {
	return core.ProviderInfo{Name: "tmdb", DisplayName: "TMDB", Kind: "all", Version: "test", Summary: "test provider"}
}
func (apiSearchScraper) IsActive() bool { return true }
func (apiSearchScraper) SearchMovie(context.Context, core.MovieSearchOptions) ([]core.MediaSearchCandidate, error) {
	return []core.MediaSearchCandidate{{ID: "1", Title: "Inception", Year: 2010, Provider: "tmdb", MediaType: core.MediaTypeMovie}}, nil
}

func (apiSearchScraper) GetMovieMetadata(context.Context, core.MovieSearchOptions) (*core.Movie, error) {
	return &core.Movie{MediaEntity: core.MediaEntity{Title: "Inception", Year: 2010, Provider: "tmdb"}}, nil
}

func (apiSearchScraper) SearchTvShow(context.Context, core.TvShowSearchOptions) ([]core.MediaSearchCandidate, error) {
	return []core.MediaSearchCandidate{{ID: "2", Title: "Breaking Bad", Year: 2008, Provider: "tmdb", MediaType: core.MediaTypeTvShow}}, nil
}

func (apiSearchScraper) GetTvShowMetadata(context.Context, core.TvShowSearchOptions) (*core.TvShow, error) {
	return &core.TvShow{MediaEntity: core.MediaEntity{Title: "Breaking Bad", Year: 2008, Provider: "tmdb"}}, nil
}

func (apiSearchScraper) GetEpisodeList(context.Context, core.TvShowSearchOptions) ([]core.TvShowEpisode, error) {
	return nil, nil
}

func (apiSearchScraper) GetEpisodeMetadata(context.Context, core.TvShowEpisodeSearchOptions) (*core.TvShowEpisode, error) {
	return &core.TvShowEpisode{MediaEntity: core.MediaEntity{Title: "Pilot", Provider: "tmdb"}, Season: 1, Episode: 1}, nil
}

func (apiSearchScraper) GetArtwork(context.Context, core.ArtworkSearchOptions) ([]core.MediaArtwork, error) {
	return []core.MediaArtwork{{Type: core.ArtworkTypePoster, URL: "https://img/poster.jpg", Provider: "tmdb"}}, nil
}

func TestMCPTools_ListAndCall(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server := newMCPTestServer(t)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, &mcp.ClientOptions{
		CreateMessageHandler: func(_ context.Context, _ *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
			payload := `{"title":"Inception","year":2010,"type":"movie","tmdb_id":27205}`
			return &mcp.CreateMessageResult{Model: "mock", Role: "assistant", Content: &mcp.TextContent{Text: payload}}, nil
		},
	})

	t1, t2 := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, t1, nil)
	require.NoError(t, err)
	defer ss.Close()
	cs, err := client.Connect(ctx, t2, nil)
	require.NoError(t, err)
	defer cs.Close()

	tools, err := cs.ListTools(ctx, nil)
	require.NoError(t, err)
	require.Len(t, tools.Tools, 17)

	parseResult, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "scraper_parse_filename", Arguments: map[string]any{"path": "Inception.2010.1080p.BluRay.mkv"}})
	require.NoError(t, err)
	require.False(t, parseResult.IsError)
	parseText := parseResult.Content[0].(*mcp.TextContent).Text
	require.Contains(t, parseText, "Inception")

	libraryResult, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "scraper_list_libraries", Arguments: map[string]any{}})
	require.NoError(t, err)
	require.False(t, libraryResult.IsError)
	libraryText := libraryResult.Content[0].(*mcp.TextContent).Text
	require.Contains(t, libraryText, "Movies")

	llmResult, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "scraper_scrape_with_llm", Arguments: map[string]any{"provider": "sampling", "raw_title": "Inception.2010.1080p.BluRay.mkv", "media_type": "movie"}})
	require.NoError(t, err)
	require.False(t, llmResult.IsError)
	llmText := llmResult.Content[0].(*mcp.TextContent).Text
	require.Contains(t, llmText, "27205")
}

func TestMCPTools_ResourcesAndPrompts(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server := newMCPTestServer(t)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	t1, t2 := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, t1, nil)
	require.NoError(t, err)
	defer ss.Close()
	cs, err := client.Connect(ctx, t2, nil)
	require.NoError(t, err)
	defer cs.Close()

	resources, err := cs.ListResources(ctx, nil)
	require.NoError(t, err)
	require.Len(t, resources.Resources, 3)
	readRes, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "scraper://providers"})
	require.NoError(t, err)
	require.Len(t, readRes.Contents, 1)
	require.Contains(t, readRes.Contents[0].Text, "sampling")

	prompts, err := cs.ListPrompts(ctx, nil)
	require.NoError(t, err)
	require.Len(t, prompts.Prompts, 3)
	getPrompt, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "scrape-movie-workflow", Arguments: map[string]string{"media_path": "movie.mkv"}})
	require.NoError(t, err)
	require.NotEmpty(t, getPrompt.Messages)
	require.Contains(t, getPrompt.Messages[0].Content.(*mcp.TextContent).Text, "scraper_parse_filename")
}
