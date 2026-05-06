package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
	"github.com/sunerpy/pt-tools/internal/scraper/service"
	"github.com/sunerpy/pt-tools/internal/scraper/store"
)

type searchMediaInput struct {
	MediaType    string   `json:"media_type" jsonschema:"movie or tv"`
	Query        string   `json:"query" jsonschema:"search query"`
	Year         int      `json:"year,omitempty" jsonschema:"release year hint"`
	Providers    []string `json:"providers,omitempty" jsonschema:"providers to query"`
	Language     string   `json:"language,omitempty" jsonschema:"search language"`
	Fallback     string   `json:"fallback_language,omitempty" jsonschema:"fallback language"`
	IMDBID       string   `json:"imdb_id,omitempty" jsonschema:"imdb identifier hint"`
	TMDBID       int      `json:"tmdb_id,omitempty" jsonschema:"tmdb identifier hint"`
	IncludeAdult bool     `json:"include_adult,omitempty" jsonschema:"include adult results"`
}

type searchMediaOutput struct {
	Results []SearchCandidate `json:"results" jsonschema:"search candidates"`
}

type getMetadataInput struct {
	MediaType string `json:"media_type" jsonschema:"movie, tv, or episode"`
	Provider  string `json:"provider" jsonschema:"provider name"`
	Query     string `json:"query,omitempty" jsonschema:"query text"`
	Year      int    `json:"year,omitempty" jsonschema:"release year"`
	Language  string `json:"language,omitempty" jsonschema:"metadata language"`
	Fallback  string `json:"fallback_language,omitempty" jsonschema:"fallback language"`
	IMDBID    string `json:"imdb_id,omitempty" jsonschema:"imdb identifier"`
	TMDBID    int    `json:"tmdb_id,omitempty" jsonschema:"tmdb identifier"`
	TVDBID    int    `json:"tvdb_id,omitempty" jsonschema:"tvdb identifier for shows"`
	TvShowID  int    `json:"tv_show_id,omitempty" jsonschema:"tmdb show id for episode lookup"`
	Season    int    `json:"season,omitempty" jsonschema:"season number"`
	Episode   int    `json:"episode,omitempty" jsonschema:"episode number"`
}

type getMetadataOutput struct {
	Provider string         `json:"provider" jsonschema:"provider name"`
	Metadata map[string]any `json:"metadata" jsonschema:"provider metadata object"`
}

type scrapeInput struct {
	MediaType    string   `json:"media_type" jsonschema:"movie, tv, or episode"`
	MediaPath    string   `json:"media_path" jsonschema:"file or directory path"`
	LibraryID    *uint    `json:"library_id,omitempty" jsonschema:"library identifier"`
	Title        string   `json:"title,omitempty" jsonschema:"explicit title override"`
	Year         int      `json:"year,omitempty" jsonschema:"explicit year override"`
	Season       int      `json:"season,omitempty" jsonschema:"season number"`
	Episode      int      `json:"episode,omitempty" jsonschema:"episode number"`
	Providers    []string `json:"providers,omitempty" jsonschema:"provider list"`
	Locale       string   `json:"locale,omitempty" jsonschema:"locale preference"`
	NfoDialect   string   `json:"nfo_dialect,omitempty" jsonschema:"nfo dialect"`
	ConnectorID  *uint    `json:"connector_id,omitempty" jsonschema:"connector identifier"`
	OverwriteNFO bool     `json:"overwrite_nfo,omitempty" jsonschema:"overwrite existing nfo files"`
}

type scrapeOutput struct {
	Success      bool     `json:"success" jsonschema:"whether scrape succeeded"`
	Type         string   `json:"type" jsonschema:"scraped media type"`
	MediaPath    string   `json:"media_path" jsonschema:"target media path"`
	NfoPath      string   `json:"nfo_path,omitempty" jsonschema:"written nfo path"`
	PosterPath   string   `json:"poster_path,omitempty" jsonschema:"written poster path"`
	Title        string   `json:"title,omitempty" jsonschema:"resolved title"`
	Year         int      `json:"year,omitempty" jsonschema:"resolved year"`
	Providers    []string `json:"providers,omitempty" jsonschema:"providers used"`
	CurrentStage string   `json:"current_stage,omitempty" jsonschema:"current or final stage"`
	FailedStage  string   `json:"failed_stage,omitempty" jsonschema:"failure stage"`
	ErrorMessage string   `json:"error_message,omitempty" jsonschema:"error message"`
}

type listLibrariesInput struct{}

type listLibrariesOutput struct {
	Libraries []LibraryInfo `json:"libraries" jsonschema:"configured libraries"`
}

type getTaskStatusInput struct {
	TaskID uint `json:"task_id" jsonschema:"task identifier"`
}

type getTaskStatusOutput struct {
	Task TaskInfo `json:"task" jsonschema:"task snapshot"`
}

type listTasksInput struct {
	State string `json:"state,omitempty" jsonschema:"task state filter"`
	Limit int    `json:"limit,omitempty" jsonschema:"max tasks to return"`
}

type listTasksOutput struct {
	Tasks []TaskInfo `json:"tasks" jsonschema:"task list"`
}

type refreshJellyfinInput struct {
	ConnectorID uint   `json:"connector_id" jsonschema:"connector configuration id"`
	LibraryID   string `json:"library_id,omitempty" jsonschema:"remote media server library id"`
}

type refreshJellyfinOutput struct {
	ConnectorID uint   `json:"connector_id" jsonschema:"connector configuration id"`
	Connector   string `json:"connector" jsonschema:"connector type"`
	Triggered   bool   `json:"triggered" jsonschema:"whether refresh was triggered"`
	LibraryID   string `json:"library_id,omitempty" jsonschema:"remote library id"`
}

type getArtworksInput struct {
	Provider     string   `json:"provider" jsonschema:"provider name"`
	EntityID     string   `json:"entity_id" jsonschema:"provider entity id"`
	MediaType    string   `json:"media_type" jsonschema:"movie, tv, or episode"`
	ArtworkTypes []string `json:"artwork_types,omitempty" jsonschema:"artwork type filters"`
	Language     string   `json:"language,omitempty" jsonschema:"artwork language"`
}

type getArtworksOutput struct {
	Artworks []map[string]any `json:"artworks" jsonschema:"artwork candidates"`
}

type writeNFOInput struct {
	MediaType  string         `json:"media_type" jsonschema:"movie, tv, or episode"`
	MediaPath  string         `json:"media_path" jsonschema:"target media path"`
	NfoDialect string         `json:"nfo_dialect,omitempty" jsonschema:"writer dialect"`
	Metadata   MetadataResult `json:"metadata" jsonschema:"metadata used to write nfo"`
}

type writeNFOOutput struct {
	NfoPath    string `json:"nfo_path" jsonschema:"primary written nfo path"`
	Dialect    string `json:"dialect" jsonschema:"writer dialect used"`
	MediaType  string `json:"media_type" jsonschema:"media type written"`
	Successful bool   `json:"successful" jsonschema:"whether write succeeded"`
}

type parseFilenameInput struct {
	Path string `json:"path" jsonschema:"filename or full media path"`
}

type parseFilenameOutput struct {
	Title     string `json:"title,omitempty" jsonschema:"parsed title"`
	Year      int    `json:"year,omitempty" jsonschema:"parsed year"`
	Season    int    `json:"season,omitempty" jsonschema:"parsed season"`
	Episode   int    `json:"episode,omitempty" jsonschema:"parsed episode"`
	Quality   string `json:"quality,omitempty" jsonschema:"parsed quality"`
	Source    string `json:"source,omitempty" jsonschema:"parsed source"`
	Codec     string `json:"codec,omitempty" jsonschema:"parsed codec"`
	Group     string `json:"group,omitempty" jsonschema:"release group"`
	IsShow    bool   `json:"is_show" jsonschema:"whether parsed as show"`
	Extension string `json:"extension,omitempty" jsonschema:"file extension"`
}

type matchMediaInput struct {
	MediaType string   `json:"media_type" jsonschema:"movie or tv"`
	Query     string   `json:"query" jsonschema:"query text"`
	Year      int      `json:"year,omitempty" jsonschema:"release year hint"`
	Providers []string `json:"providers,omitempty" jsonschema:"providers to inspect"`
	Language  string   `json:"language,omitempty" jsonschema:"search language"`
}

type matchMediaOutput struct {
	Matched   bool            `json:"matched" jsonschema:"whether a candidate was matched"`
	Candidate SearchCandidate `json:"candidate" jsonschema:"best matched candidate"`
}

type scrapeWithLLMInput struct {
	Provider    string            `json:"provider,omitempty" jsonschema:"llm provider name"`
	RawTitle    string            `json:"raw_title,omitempty" jsonschema:"raw release title"`
	Filename    string            `json:"filename,omitempty" jsonschema:"filename hint"`
	FileSize    int64             `json:"file_size,omitempty" jsonschema:"file size in bytes"`
	SiteHints   map[string]string `json:"site_hints,omitempty" jsonschema:"extra site hints"`
	UserContext string            `json:"user_context,omitempty" jsonschema:"freeform extra context"`
	Language    string            `json:"language,omitempty" jsonschema:"target language"`
	MediaType   string            `json:"media_type,omitempty" jsonschema:"movie or tv"`
}

type scrapeWithLLMOutput struct {
	Provider string         `json:"provider" jsonschema:"llm provider name"`
	Result   MetadataResult `json:"result" jsonschema:"generated metadata"`
}

type generateMetadataFromTextInput struct {
	Provider  string `json:"provider,omitempty" jsonschema:"llm provider name"`
	Text      string `json:"text" jsonschema:"freeform user text"`
	MediaType string `json:"media_type,omitempty" jsonschema:"movie or tv"`
	Language  string `json:"language,omitempty" jsonschema:"target language"`
}

type generateMetadataFromTextOutput struct {
	Provider string         `json:"provider" jsonschema:"llm provider name"`
	Result   MetadataResult `json:"result" jsonschema:"generated metadata"`
}

type validateMetadataInput struct {
	Metadata MetadataResult `json:"metadata" jsonschema:"metadata to validate"`
}

type validateMetadataOutput struct {
	Valid    bool           `json:"valid" jsonschema:"whether metadata passed validation"`
	Warnings []string       `json:"warnings" jsonschema:"validation warnings"`
	Metadata MetadataResult `json:"metadata" jsonschema:"validated metadata"`
}

type enrichPartialInput struct {
	Provider    string         `json:"provider,omitempty" jsonschema:"llm provider name"`
	MediaPath   string         `json:"media_path,omitempty" jsonschema:"media path hint"`
	Metadata    MetadataResult `json:"metadata" jsonschema:"partial metadata"`
	UserContext string         `json:"user_context,omitempty" jsonschema:"extra context"`
}

type enrichPartialOutput struct {
	Provider string         `json:"provider" jsonschema:"llm provider name"`
	Result   MetadataResult `json:"result" jsonschema:"enriched metadata"`
}

type proposeMatchInput struct {
	Provider   string            `json:"provider,omitempty" jsonschema:"llm provider name"`
	MediaType  string            `json:"media_type" jsonschema:"movie or tv"`
	Query      string            `json:"query" jsonschema:"raw query or filename"`
	Year       int               `json:"year,omitempty" jsonschema:"release year hint"`
	Candidates []SearchCandidate `json:"candidates,omitempty" jsonschema:"candidate list to judge"`
	Providers  []string          `json:"providers,omitempty" jsonschema:"providers for fallback search"`
}

type proposeMatchOutput struct {
	Matched   bool            `json:"matched" jsonschema:"whether a candidate was proposed"`
	Candidate SearchCandidate `json:"candidate" jsonschema:"selected candidate"`
	Reason    string          `json:"reason,omitempty" jsonschema:"selection reasoning"`
}

func registerTools(srv *mcpsdk.Server, deps Deps) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "scraper_search_media", Description: "Search media candidates across scraper providers."},
		func(ctx context.Context, req *mcpsdk.CallToolRequest, input searchMediaInput) (*mcpsdk.CallToolResult, searchMediaOutput, error) {
			ctx = WithSession(ctx, req.Session)
			if err := requireSourceReg(deps.SourceReg); err != nil {
				return toolError(err), searchMediaOutput{}, nil
			}
			mediaType := normalizeMediaType(input.MediaType)
			if mediaType == "" {
				return toolError(errors.New("invalid media_type")), searchMediaOutput{}, nil
			}
			results := make([]SearchCandidate, 0)
			var lastErr error
			for _, name := range selectedProviders(deps.SourceReg, input.Providers) {
				src, err := deps.SourceReg.Get(name)
				if err != nil {
					lastErr = err
					continue
				}
				switch mediaType {
				case "movie":
					movieScraper, ok := src.(core.MovieMetadataScraper)
					if !ok {
						continue
					}
					items, err := movieScraper.SearchMovie(ctx, core.MovieSearchOptions{
						Query:            input.Query,
						Year:             input.Year,
						Language:         input.Language,
						FallbackLanguage: input.Fallback,
						IMDBID:           input.IMDBID,
						TMDBID:           input.TMDBID,
						IncludeAdult:     input.IncludeAdult,
					})
					if err != nil {
						lastErr = err
						continue
					}
					for _, item := range items {
						results = append(results, toSearchCandidate(item))
					}
				case "tv":
					showScraper, ok := src.(core.TvShowMetadataScraper)
					if !ok {
						continue
					}
					items, err := showScraper.SearchTvShow(ctx, core.TvShowSearchOptions{
						Query:            input.Query,
						Year:             input.Year,
						FirstAirYear:     input.Year,
						Language:         input.Language,
						FallbackLanguage: input.Fallback,
						IMDBID:           input.IMDBID,
						TMDBID:           input.TMDBID,
						IncludeAdult:     input.IncludeAdult,
					})
					if err != nil {
						lastErr = err
						continue
					}
					for _, item := range items {
						results = append(results, toSearchCandidate(item))
					}
				}
			}
			if len(results) == 0 && lastErr != nil {
				return toolError(lastErr), searchMediaOutput{}, nil
			}
			return nil, searchMediaOutput{Results: results}, nil
		})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "scraper_get_metadata", Description: "Fetch full metadata from a specific provider."},
		func(ctx context.Context, req *mcpsdk.CallToolRequest, input getMetadataInput) (*mcpsdk.CallToolResult, getMetadataOutput, error) {
			ctx = WithSession(ctx, req.Session)
			if err := requireSourceReg(deps.SourceReg); err != nil {
				return toolError(err), getMetadataOutput{}, nil
			}
			src, err := deps.SourceReg.Get(strings.TrimSpace(input.Provider))
			if err != nil {
				return toolError(err), getMetadataOutput{}, nil
			}
			switch normalizeMediaType(input.MediaType) {
			case "movie":
				movieScraper, ok := src.(core.MovieMetadataScraper)
				if !ok {
					return toolError(errors.New("provider does not support movie metadata")), getMetadataOutput{}, nil
				}
				movie, err := movieScraper.GetMovieMetadata(ctx, core.MovieSearchOptions{
					Query:            input.Query,
					Year:             input.Year,
					Language:         input.Language,
					FallbackLanguage: input.Fallback,
					IMDBID:           input.IMDBID,
					TMDBID:           input.TMDBID,
				})
				if err != nil {
					return toolError(err), getMetadataOutput{}, nil
				}
				return nil, getMetadataOutput{Provider: input.Provider, Metadata: metadataToMap(movie)}, nil
			case "tv":
				showScraper, ok := src.(core.TvShowMetadataScraper)
				if !ok {
					return toolError(errors.New("provider does not support tv metadata")), getMetadataOutput{}, nil
				}
				show, err := showScraper.GetTvShowMetadata(ctx, core.TvShowSearchOptions{
					Query:            input.Query,
					Year:             input.Year,
					FirstAirYear:     input.Year,
					Language:         input.Language,
					FallbackLanguage: input.Fallback,
					IMDBID:           input.IMDBID,
					TMDBID:           input.TMDBID,
					TVDBID:           input.TVDBID,
				})
				if err != nil {
					return toolError(err), getMetadataOutput{}, nil
				}
				return nil, getMetadataOutput{Provider: input.Provider, Metadata: metadataToMap(show)}, nil
			case "episode":
				showScraper, ok := src.(core.TvShowMetadataScraper)
				if !ok {
					return toolError(errors.New("provider does not support episode metadata")), getMetadataOutput{}, nil
				}
				episode, err := showScraper.GetEpisodeMetadata(ctx, core.TvShowEpisodeSearchOptions{
					TvShowID: input.TvShowID,
					Season:   input.Season,
					Episode:  input.Episode,
					Language: input.Language,
				})
				if err != nil {
					return toolError(err), getMetadataOutput{}, nil
				}
				return nil, getMetadataOutput{Provider: input.Provider, Metadata: metadataToMap(episode)}, nil
			default:
				return toolError(errors.New("invalid media_type")), getMetadataOutput{}, nil
			}
		})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "scraper_scrape_file", Description: "Scrape a single file and write metadata."},
		func(ctx context.Context, req *mcpsdk.CallToolRequest, input scrapeInput) (*mcpsdk.CallToolResult, scrapeOutput, error) {
			ctx = WithSession(ctx, req.Session)
			if deps.Scrape == nil {
				return toolError(errors.New("scrape service not configured")), scrapeOutput{}, nil
			}
			mediaType := normalizeMediaType(input.MediaType)
			if mediaType == "" {
				if parsed, err := service.ParseFilename(input.MediaPath); err == nil && parsed.IsShow && input.Season == 0 && input.Episode == 0 {
					mediaType = "tv"
				} else {
					mediaType = "movie"
				}
			}
			var (
				result *service.ScrapeResult
				err    error
			)
			switch mediaType {
			case "movie":
				result, err = deps.Scrape.ScrapeMovie(ctx, service.ScrapeMovieRequest{
					LibraryID:    input.LibraryID,
					MediaPath:    input.MediaPath,
					Title:        input.Title,
					Year:         input.Year,
					Providers:    input.Providers,
					Locale:       input.Locale,
					NfoDialect:   input.NfoDialect,
					ConnectorID:  input.ConnectorID,
					OverwriteNFO: input.OverwriteNFO,
				})
			case "tv":
				result, err = deps.Scrape.ScrapeTvShow(ctx, service.ScrapeTvShowRequest{
					LibraryID:    input.LibraryID,
					MediaPath:    input.MediaPath,
					Title:        input.Title,
					Year:         input.Year,
					Providers:    input.Providers,
					Locale:       input.Locale,
					NfoDialect:   input.NfoDialect,
					ConnectorID:  input.ConnectorID,
					OverwriteNFO: input.OverwriteNFO,
				})
			case "episode":
				result, err = deps.Scrape.ScrapeEpisode(ctx, service.ScrapeEpisodeRequest{
					LibraryID:    input.LibraryID,
					MediaPath:    input.MediaPath,
					Title:        input.Title,
					Year:         input.Year,
					Season:       input.Season,
					Episode:      input.Episode,
					Providers:    input.Providers,
					Locale:       input.Locale,
					NfoDialect:   input.NfoDialect,
					ConnectorID:  input.ConnectorID,
					OverwriteNFO: input.OverwriteNFO,
				})
			}
			if err != nil {
				if result != nil {
					return nil, scrapeOutput{Success: result.Success, Type: result.Type, MediaPath: result.MediaPath, NfoPath: result.NfoPath, PosterPath: result.PosterPath, Title: result.Title, Year: result.Year, Providers: result.Providers, CurrentStage: result.CurrentStage, FailedStage: result.FailedStage, ErrorMessage: result.ErrorMessage}, nil
				}
				return toolError(err), scrapeOutput{}, nil
			}
			return nil, scrapeOutput{Success: result.Success, Type: result.Type, MediaPath: result.MediaPath, NfoPath: result.NfoPath, PosterPath: result.PosterPath, Title: result.Title, Year: result.Year, Providers: result.Providers, CurrentStage: result.CurrentStage, FailedStage: result.FailedStage, ErrorMessage: result.ErrorMessage}, nil
		})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "scraper_scrape_directory", Description: "Scrape a directory using movie or tv workflow."},
		func(ctx context.Context, req *mcpsdk.CallToolRequest, input scrapeInput) (*mcpsdk.CallToolResult, scrapeOutput, error) {
			ctx = WithSession(ctx, req.Session)
			if input.MediaType == "" {
				input.MediaType = "tv"
			}
			result, err := runScrape(ctx, deps, input)
			if err != nil {
				if result != nil {
					return nil, toScrapeOutput(result), nil
				}
				return toolError(err), scrapeOutput{}, nil
			}
			return nil, toScrapeOutput(result), nil
		})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "scraper_list_libraries", Description: "List configured scraper libraries."},
		func(ctx context.Context, req *mcpsdk.CallToolRequest, _ listLibrariesInput) (*mcpsdk.CallToolResult, listLibrariesOutput, error) {
			ctx = WithSession(ctx, req.Session)
			libraries, err := listLibraries(ctx, deps.Library)
			if err != nil {
				return toolError(err), listLibrariesOutput{}, nil
			}
			return nil, listLibrariesOutput{Libraries: libraries}, nil
		})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "scraper_get_task_status", Description: "Get detailed status for a scrape task."},
		func(ctx context.Context, req *mcpsdk.CallToolRequest, input getTaskStatusInput) (*mcpsdk.CallToolResult, getTaskStatusOutput, error) {
			ctx = WithSession(ctx, req.Session)
			task, err := loadTask(ctx, deps.DB, input.TaskID)
			if err != nil {
				return toolError(err), getTaskStatusOutput{}, nil
			}
			return nil, getTaskStatusOutput{Task: toTaskInfo(*task)}, nil
		})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "scraper_list_tasks", Description: "List recent scrape tasks from the queue."},
		func(ctx context.Context, req *mcpsdk.CallToolRequest, input listTasksInput) (*mcpsdk.CallToolResult, listTasksOutput, error) {
			ctx = WithSession(ctx, req.Session)
			tasks, err := listTasks(ctx, deps.DB, input.State, input.Limit)
			if err != nil {
				return toolError(err), listTasksOutput{}, nil
			}
			items := make([]TaskInfo, 0, len(tasks))
			for _, task := range tasks {
				items = append(items, toTaskInfo(task))
			}
			return nil, listTasksOutput{Tasks: items}, nil
		})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "scraper_refresh_jellyfin", Description: "Trigger a Jellyfin or Emby library refresh."},
		func(ctx context.Context, req *mcpsdk.CallToolRequest, input refreshJellyfinInput) (*mcpsdk.CallToolResult, refreshJellyfinOutput, error) {
			ctx = WithSession(ctx, req.Session)
			if err := requireDB(deps.DB); err != nil {
				return toolError(err), refreshJellyfinOutput{}, nil
			}
			if err := requireConnectorReg(deps.ConnectorReg); err != nil {
				return toolError(err), refreshJellyfinOutput{}, nil
			}
			var cfg store.ConnectorConfig
			if err := deps.DB.WithContext(ctx).First(&cfg, input.ConnectorID).Error; err != nil {
				return toolError(err), refreshJellyfinOutput{}, nil
			}
			connector, err := deps.ConnectorReg.Get(cfg.Type)
			if err != nil {
				return toolError(err), refreshJellyfinOutput{}, nil
			}
			if err := connector.RefreshLibrary(ctx, input.LibraryID); err != nil {
				return toolError(err), refreshJellyfinOutput{}, nil
			}
			return nil, refreshJellyfinOutput{ConnectorID: input.ConnectorID, Connector: cfg.Type, Triggered: true, LibraryID: input.LibraryID}, nil
		})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "scraper_get_artworks", Description: "Fetch candidate artworks for a provider entity."},
		func(ctx context.Context, req *mcpsdk.CallToolRequest, input getArtworksInput) (*mcpsdk.CallToolResult, getArtworksOutput, error) {
			ctx = WithSession(ctx, req.Session)
			if err := requireSourceReg(deps.SourceReg); err != nil {
				return toolError(err), getArtworksOutput{}, nil
			}
			src, err := deps.SourceReg.Get(strings.TrimSpace(input.Provider))
			if err != nil {
				return toolError(err), getArtworksOutput{}, nil
			}
			artScraper, ok := src.(core.ArtworkScraper)
			if !ok {
				return toolError(errors.New("provider does not support artworks")), getArtworksOutput{}, nil
			}
			artworks, err := artScraper.GetArtwork(ctx, core.ArtworkSearchOptions{
				EntityID:     input.EntityID,
				Type:         toCoreMediaType(input.MediaType),
				ArtworkTypes: parseArtworkTypes(input.ArtworkTypes),
				Language:     input.Language,
			})
			if err != nil {
				return toolError(err), getArtworksOutput{}, nil
			}
			items := make([]map[string]any, 0, len(artworks))
			for _, artwork := range artworks {
				items = append(items, metadataToMap(artwork))
			}
			return nil, getArtworksOutput{Artworks: items}, nil
		})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "scraper_write_nfo", Description: "Write NFO manually using supplied metadata."},
		func(ctx context.Context, req *mcpsdk.CallToolRequest, input writeNFOInput) (*mcpsdk.CallToolResult, writeNFOOutput, error) {
			ctx = WithSession(ctx, req.Session)
			if err := requireWriterReg(deps.WriterReg); err != nil {
				return toolError(err), writeNFOOutput{}, nil
			}
			dialect := strings.TrimSpace(input.NfoDialect)
			if dialect == "" {
				dialect = "universal"
			}
			writer, err := deps.WriterReg.Get(dialect)
			if err != nil {
				return toolError(err), writeNFOOutput{}, nil
			}
			switch normalizeMediaType(input.MediaType) {
			case "movie":
				nfoPath, movieNFOPath := buildMovieNFOPaths(input.MediaPath)
				if err := writer.WriteMovieNfo(ctx, coreMovieFromMetadata(input.Metadata, input.MediaPath), []string{nfoPath, movieNFOPath}); err != nil {
					return toolError(err), writeNFOOutput{}, nil
				}
				return nil, writeNFOOutput{NfoPath: nfoPath, Dialect: dialect, MediaType: "movie", Successful: true}, nil
			case "tv":
				showDir := filepath.Dir(input.MediaPath)
				if err := writer.WriteTvShowNfo(ctx, coreTvShowFromMetadata(input.Metadata, input.MediaPath), showDir); err != nil {
					return toolError(err), writeNFOOutput{}, nil
				}
				return nil, writeNFOOutput{NfoPath: buildTvShowNFOPath(input.MediaPath), Dialect: dialect, MediaType: "tv", Successful: true}, nil
			case "episode":
				nfoPath := buildEpisodeNFOPath(input.MediaPath)
				if err := writer.WriteEpisodeNfo(ctx, coreEpisodeFromMetadata(input.Metadata, input.MediaPath), nfoPath); err != nil {
					return toolError(err), writeNFOOutput{}, nil
				}
				return nil, writeNFOOutput{NfoPath: nfoPath, Dialect: dialect, MediaType: "episode", Successful: true}, nil
			default:
				return toolError(errors.New("invalid media_type")), writeNFOOutput{}, nil
			}
		})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "scraper_parse_filename", Description: "Parse a release filename into metadata hints."},
		func(ctx context.Context, req *mcpsdk.CallToolRequest, input parseFilenameInput) (*mcpsdk.CallToolResult, parseFilenameOutput, error) {
			ctx = WithSession(ctx, req.Session)
			_ = SessionFromContext(ctx)
			parsed, err := service.ParseFilename(input.Path)
			if err != nil {
				return toolError(err), parseFilenameOutput{}, nil
			}
			return nil, parseFilenameOutput{Title: parsed.Title, Year: parsed.Year, Season: parsed.Season, Episode: parsed.Episode, Quality: parsed.Quality, Source: parsed.Source, Codec: parsed.Codec, Group: parsed.Group, IsShow: parsed.IsShow, Extension: parsed.Extension}, nil
		})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "scraper_match_media", Description: "Pick the best media match from provider search results."},
		func(ctx context.Context, req *mcpsdk.CallToolRequest, input matchMediaInput) (*mcpsdk.CallToolResult, matchMediaOutput, error) {
			ctx = WithSession(ctx, req.Session)
			candidates, err := searchCandidates(ctx, deps, input.MediaType, input.Query, input.Year, input.Providers, input.Language)
			if err != nil {
				return toolError(err), matchMediaOutput{}, nil
			}
			if len(candidates) == 0 {
				return nil, matchMediaOutput{Matched: false}, nil
			}
			return nil, matchMediaOutput{Matched: true, Candidate: candidates[0]}, nil
		})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "scraper_scrape_with_llm", Description: "Generate structured metadata from release text using an LLM provider."},
		func(ctx context.Context, req *mcpsdk.CallToolRequest, input scrapeWithLLMInput) (*mcpsdk.CallToolResult, scrapeWithLLMOutput, error) {
			ctx = WithSession(ctx, req.Session)
			provider, err := requireLLMProvider(deps, strings.TrimSpace(input.Provider))
			if err != nil {
				return toolError(err), scrapeWithLLMOutput{}, nil
			}
			result, err := provider.Extract(ctx, LLMExtractRequest{RawTitle: input.RawTitle, Filename: input.Filename, FileSize: input.FileSize, SiteHints: input.SiteHints, UserContext: input.UserContext, Language: input.Language, MediaType: input.MediaType})
			if err != nil {
				return toolError(err), scrapeWithLLMOutput{}, nil
			}
			if result != nil && result.GeneratedBy == "" {
				result.GeneratedBy = provider.Name()
			}
			return nil, scrapeWithLLMOutput{Provider: provider.Name(), Result: *result}, nil
		})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "scraper_generate_metadata_from_text", Description: "Generate metadata from freeform user text with an LLM provider."},
		func(ctx context.Context, req *mcpsdk.CallToolRequest, input generateMetadataFromTextInput) (*mcpsdk.CallToolResult, generateMetadataFromTextOutput, error) {
			ctx = WithSession(ctx, req.Session)
			provider, err := requireLLMProvider(deps, strings.TrimSpace(input.Provider))
			if err != nil {
				return toolError(err), generateMetadataFromTextOutput{}, nil
			}
			result, err := provider.Extract(ctx, LLMExtractRequest{RawTitle: input.Text, UserContext: input.Text, Language: input.Language, MediaType: input.MediaType})
			if err != nil {
				return toolError(err), generateMetadataFromTextOutput{}, nil
			}
			if result != nil && result.GeneratedBy == "" {
				result.GeneratedBy = provider.Name()
			}
			return nil, generateMetadataFromTextOutput{Provider: provider.Name(), Result: *result}, nil
		})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "scraper_validate_metadata", Description: "Validate LLM-generated metadata fields."},
		func(ctx context.Context, req *mcpsdk.CallToolRequest, input validateMetadataInput) (*mcpsdk.CallToolResult, validateMetadataOutput, error) {
			ctx = WithSession(ctx, req.Session)
			_ = SessionFromContext(ctx)
			warnings := validateMetadata(input.Metadata)
			return nil, validateMetadataOutput{Valid: len(warnings) == 0, Warnings: warnings, Metadata: input.Metadata}, nil
		})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "scraper_enrich_partial", Description: "Use an LLM provider to fill missing metadata fields."},
		func(ctx context.Context, req *mcpsdk.CallToolRequest, input enrichPartialInput) (*mcpsdk.CallToolResult, enrichPartialOutput, error) {
			ctx = WithSession(ctx, req.Session)
			provider, err := requireLLMProvider(deps, strings.TrimSpace(input.Provider))
			if err != nil {
				return toolError(err), enrichPartialOutput{}, nil
			}
			patch, err := provider.Extract(ctx, llmRequestFromMetadata(input.Metadata, input.MediaPath, input.UserContext))
			if err != nil {
				return toolError(err), enrichPartialOutput{}, nil
			}
			merged := mergeMetadata(input.Metadata, patch)
			if merged.GeneratedBy == "" {
				merged.GeneratedBy = provider.Name()
			}
			return nil, enrichPartialOutput{Provider: provider.Name(), Result: merged}, nil
		})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "scraper_propose_match", Description: "Use search results and optional LLM context to propose a best match."},
		func(ctx context.Context, req *mcpsdk.CallToolRequest, input proposeMatchInput) (*mcpsdk.CallToolResult, proposeMatchOutput, error) {
			ctx = WithSession(ctx, req.Session)
			candidates := input.Candidates
			if len(candidates) == 0 {
				items, err := searchCandidates(ctx, deps, input.MediaType, input.Query, input.Year, input.Providers, "")
				if err != nil {
					return toolError(err), proposeMatchOutput{}, nil
				}
				candidates = items
			}
			if len(candidates) == 0 {
				return nil, proposeMatchOutput{Matched: false, Reason: "no candidates"}, nil
			}
			candidate := candidates[0]
			reason := "selected highest-ranked candidate from provider search"
			if strings.TrimSpace(input.Provider) != "" {
				provider, err := requireLLMProvider(deps, strings.TrimSpace(input.Provider))
				if err == nil {
					payload, _ := json.Marshal(candidates)
					_, _ = provider.Extract(ctx, LLMExtractRequest{RawTitle: input.Query, UserContext: "Choose the best candidate from this JSON array and explain briefly:\n" + string(payload), MediaType: input.MediaType})
					reason = "selected top search candidate with LLM context available"
				}
			}
			return nil, proposeMatchOutput{Matched: true, Candidate: candidate, Reason: reason}, nil
		})
}

func searchCandidates(ctx context.Context, deps Deps, mediaType, query string, year int, providers []string, language string) ([]SearchCandidate, error) {
	if err := requireSourceReg(deps.SourceReg); err != nil {
		return nil, err
	}
	searchInput := searchMediaInput{MediaType: mediaType, Query: query, Year: year, Providers: providers, Language: language}
	items := make([]SearchCandidate, 0)
	var lastErr error
	for _, name := range selectedProviders(deps.SourceReg, searchInput.Providers) {
		src, err := deps.SourceReg.Get(name)
		if err != nil {
			lastErr = err
			continue
		}
		switch normalizeMediaType(searchInput.MediaType) {
		case "movie":
			movieScraper, ok := src.(core.MovieMetadataScraper)
			if !ok {
				continue
			}
			results, err := movieScraper.SearchMovie(ctx, core.MovieSearchOptions{Query: searchInput.Query, Year: searchInput.Year, Language: searchInput.Language})
			if err != nil {
				lastErr = err
				continue
			}
			for _, candidate := range results {
				items = append(items, toSearchCandidate(candidate))
			}
		case "tv":
			showScraper, ok := src.(core.TvShowMetadataScraper)
			if !ok {
				continue
			}
			results, err := showScraper.SearchTvShow(ctx, core.TvShowSearchOptions{Query: searchInput.Query, Year: searchInput.Year, FirstAirYear: searchInput.Year, Language: searchInput.Language})
			if err != nil {
				lastErr = err
				continue
			}
			for _, candidate := range results {
				items = append(items, toSearchCandidate(candidate))
			}
		}
	}
	if len(items) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return items, nil
}

func runScrape(ctx context.Context, deps Deps, input scrapeInput) (*service.ScrapeResult, error) {
	if deps.Scrape == nil {
		return nil, errors.New("scrape service not configured")
	}
	mediaType := normalizeMediaType(input.MediaType)
	if mediaType == "" {
		mediaType = "movie"
	}
	switch mediaType {
	case "movie":
		return deps.Scrape.ScrapeMovie(ctx, service.ScrapeMovieRequest{
			LibraryID:    input.LibraryID,
			MediaPath:    input.MediaPath,
			Title:        input.Title,
			Year:         input.Year,
			Providers:    input.Providers,
			Locale:       input.Locale,
			NfoDialect:   input.NfoDialect,
			ConnectorID:  input.ConnectorID,
			OverwriteNFO: input.OverwriteNFO,
		})
	case "tv":
		return deps.Scrape.ScrapeTvShow(ctx, service.ScrapeTvShowRequest{
			LibraryID:    input.LibraryID,
			MediaPath:    input.MediaPath,
			Title:        input.Title,
			Year:         input.Year,
			Providers:    input.Providers,
			Locale:       input.Locale,
			NfoDialect:   input.NfoDialect,
			ConnectorID:  input.ConnectorID,
			OverwriteNFO: input.OverwriteNFO,
		})
	case "episode":
		return deps.Scrape.ScrapeEpisode(ctx, service.ScrapeEpisodeRequest{
			LibraryID:    input.LibraryID,
			MediaPath:    input.MediaPath,
			Title:        input.Title,
			Year:         input.Year,
			Season:       input.Season,
			Episode:      input.Episode,
			Providers:    input.Providers,
			Locale:       input.Locale,
			NfoDialect:   input.NfoDialect,
			ConnectorID:  input.ConnectorID,
			OverwriteNFO: input.OverwriteNFO,
		})
	default:
		return nil, errors.New("invalid media_type")
	}
}

func toScrapeOutput(result *service.ScrapeResult) scrapeOutput {
	if result == nil {
		return scrapeOutput{}
	}
	return scrapeOutput{
		Success:      result.Success,
		Type:         result.Type,
		MediaPath:    result.MediaPath,
		NfoPath:      result.NfoPath,
		PosterPath:   result.PosterPath,
		Title:        result.Title,
		Year:         result.Year,
		Providers:    result.Providers,
		CurrentStage: result.CurrentStage,
		FailedStage:  result.FailedStage,
		ErrorMessage: result.ErrorMessage,
	}
}
