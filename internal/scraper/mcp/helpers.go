package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
	"github.com/sunerpy/pt-tools/internal/scraper/service"
	"github.com/sunerpy/pt-tools/internal/scraper/store"
)

var imdbIDPattern = regexp.MustCompile(`^tt\d{7,9}$`)

type SearchCandidate struct {
	ID        string  `json:"id" jsonschema:"candidate identifier from provider"`
	Title     string  `json:"title" jsonschema:"candidate title"`
	Year      int     `json:"year,omitempty" jsonschema:"release year if available"`
	MediaType string  `json:"media_type" jsonschema:"movie or tv"`
	Provider  string  `json:"provider" jsonschema:"provider name"`
	PosterURL string  `json:"poster_url,omitempty" jsonschema:"poster URL if available"`
	Overview  string  `json:"overview,omitempty" jsonschema:"overview if available"`
	Score     float64 `json:"score,omitempty" jsonschema:"provider score"`
}

type LibraryInfo struct {
	ID          uint   `json:"id" jsonschema:"library identifier"`
	Name        string `json:"name" jsonschema:"library name"`
	Type        string `json:"type" jsonschema:"library type"`
	Path        string `json:"path" jsonschema:"library path"`
	Enabled     bool   `json:"enabled" jsonschema:"whether library is enabled"`
	ProviderIDs string `json:"provider_ids,omitempty" jsonschema:"comma separated providers"`
	NfoDialect  string `json:"nfo_dialect,omitempty" jsonschema:"nfo dialect name"`
	ConnectorID *uint  `json:"connector_id,omitempty" jsonschema:"associated connector id"`
	ScanCron    string `json:"scan_cron,omitempty" jsonschema:"scan cron expression"`
	AutoScrape  bool   `json:"auto_scrape" jsonschema:"whether auto scrape is enabled"`
}

type TaskInfo struct {
	ID           uint       `json:"id" jsonschema:"task identifier"`
	LibraryID    *uint      `json:"library_id,omitempty" jsonschema:"associated library id"`
	TaskType     string     `json:"task_type" jsonschema:"task type"`
	MediaPath    string     `json:"media_path" jsonschema:"target media path"`
	State        string     `json:"state" jsonschema:"task state"`
	CurrentStage string     `json:"current_stage,omitempty" jsonschema:"current task stage"`
	Progress     float64    `json:"progress,omitempty" jsonschema:"progress percentage"`
	RetryCount   int        `json:"retry_count,omitempty" jsonschema:"current retry count"`
	MaxRetries   int        `json:"max_retries,omitempty" jsonschema:"configured max retries"`
	NextRetryAt  *time.Time `json:"next_retry_at,omitempty" jsonschema:"next retry time"`
	LastError    string     `json:"last_error,omitempty" jsonschema:"last error text"`
	StartedAt    *time.Time `json:"started_at,omitempty" jsonschema:"start timestamp"`
	CompletedAt  *time.Time `json:"completed_at,omitempty" jsonschema:"completion timestamp"`
	CreatedAt    time.Time  `json:"created_at" jsonschema:"creation timestamp"`
	UpdatedAt    time.Time  `json:"updated_at" jsonschema:"last update timestamp"`
}

type ProviderStatus struct {
	Name      string `json:"name" jsonschema:"provider name"`
	Kind      string `json:"kind" jsonschema:"provider kind"`
	Type      string `json:"type" jsonschema:"source or llm"`
	Active    bool   `json:"active" jsonschema:"provider active status"`
	Summary   string `json:"summary,omitempty" jsonschema:"provider summary"`
	Version   string `json:"version,omitempty" jsonschema:"provider version"`
	Connector string `json:"connector,omitempty" jsonschema:"connector type when applicable"`
}

type MetadataResult struct {
	Title         string   `json:"title,omitempty" jsonschema:"normalized title"`
	OriginalTitle string   `json:"original_title,omitempty" jsonschema:"original title"`
	Year          int      `json:"year,omitempty" jsonschema:"release year"`
	Type          string   `json:"type,omitempty" jsonschema:"movie or tv"`
	TMDBID        int      `json:"tmdb_id,omitempty" jsonschema:"TMDB id when known"`
	IMDBID        string   `json:"imdb_id,omitempty" jsonschema:"IMDb id when known"`
	Season        int      `json:"season,omitempty" jsonschema:"season number"`
	Episode       int      `json:"episode,omitempty" jsonschema:"episode number"`
	Genres        []string `json:"genres,omitempty" jsonschema:"detected genres"`
	Language      string   `json:"language,omitempty" jsonschema:"content language"`
	Plot          string   `json:"plot,omitempty" jsonschema:"plot summary"`
	Directors     []string `json:"directors,omitempty" jsonschema:"director names"`
	Cast          []string `json:"cast,omitempty" jsonschema:"cast names"`
	Runtime       int      `json:"runtime,omitempty" jsonschema:"runtime in minutes"`
	GeneratedBy   string   `json:"generated_by,omitempty" jsonschema:"provider that generated the metadata"`
}

type LLMExtractRequest struct {
	RawTitle    string            `json:"raw_title,omitempty" jsonschema:"raw title or release name"`
	Filename    string            `json:"filename,omitempty" jsonschema:"full filename if available"`
	FileSize    int64             `json:"file_size,omitempty" jsonschema:"file size in bytes"`
	SiteHints   map[string]string `json:"site_hints,omitempty" jsonschema:"extra site hints"`
	UserContext string            `json:"user_context,omitempty" jsonschema:"freeform extra context"`
	Language    string            `json:"language,omitempty" jsonschema:"target language"`
	MediaType   string            `json:"media_type,omitempty" jsonschema:"movie or tv"`
}

func toolError(err error) *mcpsdk.CallToolResult {
	return &mcpsdk.CallToolResult{
		IsError: true,
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: err.Error()}},
	}
}

func requireDB(db *gorm.DB) error {
	if db == nil {
		return errors.New("db not configured")
	}
	return nil
}

func requireSourceReg(reg *core.Registry[core.MediaScraper]) error {
	if reg == nil {
		return errors.New("source registry not configured")
	}
	return nil
}

func requireWriterReg(reg *core.Registry[core.NfoWriter]) error {
	if reg == nil {
		return errors.New("writer registry not configured")
	}
	return nil
}

func requireConnectorReg(reg *core.Registry[core.MediaServerConnector]) error {
	if reg == nil {
		return errors.New("connector registry not configured")
	}
	return nil
}

func requireLLMProvider(deps Deps, name string) (LLMProvider, error) {
	if len(deps.LLMProviders) == 0 {
		return nil, errors.New("llm providers not configured")
	}
	if name != "" {
		provider, ok := deps.LLMProviders[name]
		if !ok {
			return nil, fmt.Errorf("llm provider %q not found", name)
		}
		return provider, nil
	}
	names := make([]string, 0, len(deps.LLMProviders))
	for providerName := range deps.LLMProviders {
		names = append(names, providerName)
	}
	sort.Strings(names)
	return deps.LLMProviders[names[0]], nil
}

func normalizeMediaType(mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "movie":
		return "movie"
	case "tv", "tvshow", "tv_show", "show", "series":
		return "tv"
	case "episode":
		return "episode"
	default:
		return ""
	}
}

func toCoreMediaType(mediaType string) core.MediaType {
	switch normalizeMediaType(mediaType) {
	case "movie":
		return core.MediaTypeMovie
	case "tv":
		return core.MediaTypeTvShow
	case "episode":
		return core.MediaTypeEpisode
	default:
		return core.MediaTypeUnknown
	}
}

func parseArtworkTypes(values []string) []core.ArtworkType {
	if len(values) == 0 {
		return nil
	}
	out := make([]core.ArtworkType, 0, len(values))
	for _, value := range values {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "poster":
			out = append(out, core.ArtworkTypePoster)
		case "fanart", "background":
			out = append(out, core.ArtworkTypeBackground)
		case "banner":
			out = append(out, core.ArtworkTypeBanner)
		case "clearlogo":
			out = append(out, core.ArtworkTypeClearlogo)
		case "thumb", "landscape":
			out = append(out, core.ArtworkTypeThumb)
		}
	}
	return out
}

func dedupeStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func selectedProviders(reg *core.Registry[core.MediaScraper], requested []string) []string {
	if reg == nil {
		return nil
	}
	if len(requested) == 0 {
		return reg.List()
	}
	return dedupeStrings(requested)
}

func toSearchCandidate(candidate core.MediaSearchCandidate) SearchCandidate {
	return SearchCandidate{
		ID:        candidate.ID,
		Title:     candidate.Title,
		Year:      candidate.Year,
		MediaType: candidate.MediaType.String(),
		Provider:  candidate.Provider,
		PosterURL: candidate.PosterURL,
		Overview:  candidate.Overview,
		Score:     candidate.Score,
	}
}

func toLibraryInfo(lib store.MediaLibraryConfig) LibraryInfo {
	return LibraryInfo{
		ID:          lib.ID,
		Name:        lib.Name,
		Type:        lib.Type,
		Path:        lib.Path,
		Enabled:     lib.Enabled,
		ProviderIDs: lib.ProviderIDs,
		NfoDialect:  lib.NfoDialect,
		ConnectorID: lib.ConnectorID,
		ScanCron:    lib.ScanCron,
		AutoScrape:  lib.AutoScrape,
	}
}

func toTaskInfo(task store.ScrapeTask) TaskInfo {
	return TaskInfo{
		ID:           task.ID,
		LibraryID:    task.LibraryID,
		TaskType:     task.TaskType,
		MediaPath:    task.MediaPath,
		State:        task.State,
		CurrentStage: task.CurrentStage,
		Progress:     task.Progress,
		RetryCount:   task.RetryCount,
		MaxRetries:   task.MaxRetries,
		NextRetryAt:  task.NextRetryAt,
		LastError:    task.LastError,
		StartedAt:    task.StartedAt,
		CompletedAt:  task.CompletedAt,
		CreatedAt:    task.CreatedAt,
		UpdatedAt:    task.UpdatedAt,
	}
}

func metadataToMap(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return map[string]any{"raw": fmt.Sprintf("%v", value)}
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]any{"raw": string(data)}
	}
	return out
}

func validateMetadata(result MetadataResult) []string {
	warnings := make([]string, 0)
	if strings.TrimSpace(result.Title) == "" {
		warnings = append(warnings, "title is empty")
	}
	if result.Year != 0 && (result.Year < 1900 || result.Year > time.Now().Year()+5) {
		warnings = append(warnings, "year out of range")
	}
	if result.TMDBID < 0 || result.TMDBID > 10_000_000 {
		warnings = append(warnings, "tmdb_id out of range")
	}
	if result.IMDBID != "" && !imdbIDPattern.MatchString(strings.TrimSpace(result.IMDBID)) {
		warnings = append(warnings, "imdb_id format invalid")
	}
	switch result.Type {
	case "", "movie", "tv", "unknown":
	default:
		warnings = append(warnings, "type must be one of movie, tv, unknown")
	}
	if result.Season < 0 {
		warnings = append(warnings, "season must be >= 0")
	}
	if result.Episode < 0 {
		warnings = append(warnings, "episode must be >= 0")
	}
	return warnings
}

func mergeMetadata(base MetadataResult, patch *MetadataResult) MetadataResult {
	if patch == nil {
		return base
	}
	if patch.Title != "" {
		base.Title = patch.Title
	}
	if patch.OriginalTitle != "" {
		base.OriginalTitle = patch.OriginalTitle
	}
	if patch.Year != 0 {
		base.Year = patch.Year
	}
	if patch.Type != "" {
		base.Type = patch.Type
	}
	if patch.TMDBID != 0 {
		base.TMDBID = patch.TMDBID
	}
	if patch.IMDBID != "" {
		base.IMDBID = patch.IMDBID
	}
	if patch.Season != 0 {
		base.Season = patch.Season
	}
	if patch.Episode != 0 {
		base.Episode = patch.Episode
	}
	if len(patch.Genres) > 0 {
		base.Genres = append([]string(nil), patch.Genres...)
	}
	if patch.Language != "" {
		base.Language = patch.Language
	}
	if patch.Plot != "" {
		base.Plot = patch.Plot
	}
	if len(patch.Directors) > 0 {
		base.Directors = append([]string(nil), patch.Directors...)
	}
	if len(patch.Cast) > 0 {
		base.Cast = append([]string(nil), patch.Cast...)
	}
	if patch.Runtime != 0 {
		base.Runtime = patch.Runtime
	}
	if patch.GeneratedBy != "" {
		base.GeneratedBy = patch.GeneratedBy
	}
	return base
}

func llmRequestFromMetadata(input MetadataResult, mediaPath, extraContext string) LLMExtractRequest {
	rawTitle := input.Title
	if rawTitle == "" && mediaPath != "" {
		rawTitle = filepath.Base(mediaPath)
	}
	payload, _ := json.Marshal(input)
	contextText := strings.TrimSpace(extraContext)
	if len(payload) > 2 {
		if contextText != "" {
			contextText += "\n\n"
		}
		contextText += "Partial metadata JSON:\n" + string(payload)
	}
	return LLMExtractRequest{
		RawTitle:    rawTitle,
		Filename:    filepath.Base(mediaPath),
		UserContext: contextText,
		Language:    input.Language,
		MediaType:   input.Type,
	}
}

func loadTask(ctx context.Context, db *gorm.DB, id uint) (*store.ScrapeTask, error) {
	if err := requireDB(db); err != nil {
		return nil, err
	}
	var task store.ScrapeTask
	if err := db.WithContext(ctx).First(&task, id).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func listTasks(ctx context.Context, db *gorm.DB, state string, limit int) ([]store.ScrapeTask, error) {
	if err := requireDB(db); err != nil {
		return nil, err
	}
	query := db.WithContext(ctx).Order("id DESC")
	if strings.TrimSpace(state) != "" {
		query = query.Where("state = ?", strings.TrimSpace(state))
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	var tasks []store.ScrapeTask
	if err := query.Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

func listLibraries(ctx context.Context, librarySvc *service.LibraryService) ([]LibraryInfo, error) {
	if librarySvc == nil {
		return nil, errors.New("library service not configured")
	}
	libs, err := librarySvc.ListLibraries(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]LibraryInfo, 0, len(libs))
	for _, lib := range libs {
		out = append(out, toLibraryInfo(lib))
	}
	return out, nil
}

func listProviderStatus(ctx context.Context, deps Deps) ([]ProviderStatus, error) {
	_ = ctx
	items := make([]ProviderStatus, 0)
	if deps.SourceReg != nil {
		for _, name := range deps.SourceReg.List() {
			src, err := deps.SourceReg.Get(name)
			if err != nil || src == nil {
				continue
			}
			info := src.Info()
			items = append(items, ProviderStatus{
				Name:    info.Name,
				Kind:    info.Kind,
				Type:    "source",
				Active:  src.IsActive(),
				Summary: info.Summary,
				Version: info.Version,
			})
		}
	}
	if len(deps.LLMProviders) > 0 {
		names := make([]string, 0, len(deps.LLMProviders))
		for name := range deps.LLMProviders {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			provider := deps.LLMProviders[name]
			if provider == nil {
				continue
			}
			items = append(items, ProviderStatus{
				Name:   provider.Name(),
				Kind:   provider.Kind(),
				Type:   "llm",
				Active: true,
			})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Type == items[j].Type {
			return items[i].Name < items[j].Name
		}
		return items[i].Type < items[j].Type
	})
	return items, nil
}

func writeJSONResource(uri string, payload any) (*mcpsdk.ReadResourceResult, error) {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, err
	}
	return &mcpsdk.ReadResourceResult{
		Contents: []*mcpsdk.ResourceContents{{
			URI:      uri,
			MIMEType: "application/json",
			Text:     string(data),
		}},
	}, nil
}

func buildMovieNFOPaths(mediaPath string) (string, string) {
	nfoPath := strings.TrimSuffix(mediaPath, filepath.Ext(mediaPath)) + ".nfo"
	movieNfoPath := filepath.Join(filepath.Dir(mediaPath), "movie.nfo")
	return nfoPath, movieNfoPath
}

func buildEpisodeNFOPath(mediaPath string) string {
	return strings.TrimSuffix(mediaPath, filepath.Ext(mediaPath)) + ".nfo"
}

func buildTvShowNFOPath(mediaPath string) string {
	return filepath.Join(filepath.Dir(mediaPath), "tvshow.nfo")
}

func coreMovieFromMetadata(result MetadataResult, mediaPath string) *core.Movie {
	ids := make(map[string]string)
	if result.IMDBID != "" {
		ids["imdb"] = result.IMDBID
	}
	if result.TMDBID > 0 {
		ids["tmdb"] = strconv.Itoa(result.TMDBID)
	}
	return &core.Movie{MediaEntity: core.MediaEntity{
		Title:         result.Title,
		OriginalTitle: result.OriginalTitle,
		Year:          result.Year,
		Plot:          result.Plot,
		Outline:       result.Plot,
		Genres:        append([]string(nil), result.Genres...),
		Path:          mediaPath,
		IDs:           ids,
		Runtime:       result.Runtime,
		Provider:      result.GeneratedBy,
		ScrapedAt:     time.Now(),
	}, Watched: false}
}

func coreTvShowFromMetadata(result MetadataResult, mediaPath string) *core.TvShow {
	ids := make(map[string]string)
	if result.IMDBID != "" {
		ids["imdb"] = result.IMDBID
	}
	if result.TMDBID > 0 {
		ids["tmdb"] = strconv.Itoa(result.TMDBID)
	}
	return &core.TvShow{MediaEntity: core.MediaEntity{
		Title:         result.Title,
		OriginalTitle: result.OriginalTitle,
		Year:          result.Year,
		Plot:          result.Plot,
		Outline:       result.Plot,
		Genres:        append([]string(nil), result.Genres...),
		Path:          filepath.Dir(mediaPath),
		IDs:           ids,
		Runtime:       result.Runtime,
		Provider:      result.GeneratedBy,
		ScrapedAt:     time.Now(),
	}, SeasonNames: map[int]string{}, SeasonPlots: map[int]string{}}
}

func coreEpisodeFromMetadata(result MetadataResult, mediaPath string) *core.TvShowEpisode {
	ids := make(map[string]string)
	if result.IMDBID != "" {
		ids["imdb"] = result.IMDBID
	}
	if result.TMDBID > 0 {
		ids["tmdb"] = strconv.Itoa(result.TMDBID)
	}
	return &core.TvShowEpisode{MediaEntity: core.MediaEntity{
		Title:         result.Title,
		OriginalTitle: result.OriginalTitle,
		Year:          result.Year,
		Plot:          result.Plot,
		Outline:       result.Plot,
		Genres:        append([]string(nil), result.Genres...),
		Path:          mediaPath,
		IDs:           ids,
		Runtime:       result.Runtime,
		Provider:      result.GeneratedBy,
		ScrapedAt:     time.Now(),
	}, Season: result.Season, Episode: result.Episode}
}
