package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
	"github.com/sunerpy/pt-tools/internal/scraper/service"
	"github.com/sunerpy/pt-tools/internal/scraper/source/llm"
	"github.com/sunerpy/pt-tools/internal/scraper/store"
)

type Logger interface {
	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

type API struct {
	scrape       *service.ScrapeService
	library      *service.LibraryService
	db           *gorm.DB
	sourceReg    *core.Registry[core.MediaScraper]
	connectorReg *core.Registry[core.MediaServerConnector]
	llmProviders map[string]llm.Provider
	logger       Logger
}

type APIConfig struct {
	Scrape       *service.ScrapeService
	Library      *service.LibraryService
	DB           *gorm.DB
	SourceReg    *core.Registry[core.MediaScraper]
	ConnectorReg *core.Registry[core.MediaServerConnector]
	LLMProviders map[string]llm.Provider
	Logger       Logger
}

func NewAPI(cfg APIConfig) (*API, error) {
	if cfg.Scrape == nil {
		return nil, errors.New("nil scrape service")
	}
	if cfg.Library == nil {
		return nil, errors.New("nil library service")
	}
	if cfg.DB == nil {
		return nil, errors.New("nil db")
	}
	if cfg.SourceReg == nil {
		return nil, errors.New("nil source registry")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = noopLogger{}
	}
	providers := cfg.LLMProviders
	if providers == nil {
		providers = map[string]llm.Provider{}
	}
	return &API{
		scrape:       cfg.Scrape,
		library:      cfg.Library,
		db:           cfg.DB,
		sourceReg:    cfg.SourceReg,
		connectorReg: cfg.ConnectorReg,
		llmProviders: providers,
		logger:       logger,
	}, nil
}

func (a *API) HandleListLibraries(w http.ResponseWriter, r *http.Request) {
	libs, err := a.library.ListLibraries(r.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, libs)
}

func (a *API) HandleCreateLibrary(w http.ResponseWriter, r *http.Request) {
	var req createLibraryRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	lib, err := a.library.CreateLibrary(r.Context(), service.CreateLibraryRequest{
		Name:        req.Name,
		Type:        req.Type,
		Path:        req.Path,
		ProviderIDs: req.ProviderIDs,
		ConnectorID: req.ConnectorID,
		ScanCron:    req.ScanCron,
		AutoScrape:  req.AutoScrape,
		NfoDialect:  req.NfoDialect,
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, lib)
}

func (a *API) HandleGetLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUint(w, r, "id")
	if !ok {
		return
	}
	lib, err := a.library.GetLibrary(r.Context(), id)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lib)
}

func (a *API) HandleUpdateLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUint(w, r, "id")
	if !ok {
		return
	}
	var req updateLibraryRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	lib, err := a.library.UpdateLibrary(r.Context(), id, service.UpdateLibraryRequest{
		Name:        req.Name,
		Type:        req.Type,
		Path:        req.Path,
		Enabled:     req.Enabled,
		ProviderIDs: req.ProviderIDs,
		ConnectorID: req.ConnectorID,
		ScanCron:    req.ScanCron,
		AutoScrape:  req.AutoScrape,
		NfoDialect:  req.NfoDialect,
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lib)
}

func (a *API) HandleDeleteLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUint(w, r, "id")
	if !ok {
		return
	}
	if err := a.library.DeleteLibrary(r.Context(), id); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
}

func (a *API) HandleScrape(w http.ResponseWriter, r *http.Request) {
	var req scrapeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	mediaType := normalizeMediaType(req.Type)
	var (
		task *store.ScrapeTask
		err  error
	)
	switch mediaType {
	case "movie":
		task, err = a.scrape.EnqueueMovie(ctx, service.ScrapeMovieRequest{
			LibraryID:    req.LibraryID,
			MediaPath:    req.MediaPath,
			Title:        req.Title,
			Year:         req.Year,
			Providers:    req.Providers,
			Locale:       req.Locale,
			NfoDialect:   req.NfoDialect,
			ConnectorID:  req.ConnectorID,
			OverwriteNFO: req.OverwriteNFO,
		})
	case "tv":
		task, err = a.scrape.EnqueueTvShow(ctx, service.ScrapeTvShowRequest{
			LibraryID:    req.LibraryID,
			MediaPath:    req.MediaPath,
			Title:        req.Title,
			Year:         req.Year,
			Providers:    req.Providers,
			Locale:       req.Locale,
			NfoDialect:   req.NfoDialect,
			ConnectorID:  req.ConnectorID,
			OverwriteNFO: req.OverwriteNFO,
		})
	case "episode":
		task, err = a.scrape.EnqueueEpisode(ctx, service.ScrapeEpisodeRequest{
			LibraryID:    req.LibraryID,
			MediaPath:    req.MediaPath,
			Title:        req.Title,
			Year:         req.Year,
			Season:       req.Season,
			Episode:      req.Episode,
			Providers:    req.Providers,
			Locale:       req.Locale,
			NfoDialect:   req.NfoDialect,
			ConnectorID:  req.ConnectorID,
			OverwriteNFO: req.OverwriteNFO,
		})
	default:
		writeError(w, http.StatusBadRequest, "invalid scrape type")
		return
	}
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, task)
}

func (a *API) HandleListTasks(w http.ResponseWriter, r *http.Request) {
	var tasks []store.ScrapeTask
	query := a.db.WithContext(r.Context()).Order("id DESC")
	if state := strings.TrimSpace(r.URL.Query().Get("state")); state != "" {
		query = query.Where("state = ?", state)
	}
	if err := query.Find(&tasks).Error; err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (a *API) HandleGetTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUint(w, r, "id")
	if !ok {
		return
	}
	var task store.ScrapeTask
	if err := a.db.WithContext(r.Context()).First(&task, id).Error; err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (a *API) HandleDeleteTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUint(w, r, "id")
	if !ok {
		return
	}
	if err := a.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("task_id = ?", id).Delete(&store.ScrapeResult{}).Error; err != nil {
			return err
		}
		res := tx.Delete(&store.ScrapeTask{}, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	}); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
}

func (a *API) HandleListProviders(w http.ResponseWriter, r *http.Request) {
	providers := make([]core.ProviderInfo, 0, len(a.sourceReg.List()))
	for _, name := range a.sourceReg.List() {
		src, err := a.sourceReg.Get(name)
		if err != nil || src == nil {
			continue
		}
		providers = append(providers, src.Info())
	}
	writeJSON(w, http.StatusOK, providers)
}

func (a *API) HandleSetProviderCredential(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		writeError(w, http.StatusBadRequest, "provider name required")
		return
	}
	var req providerCredentialRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	credential := store.ProviderCredential{Provider: name}
	err := a.db.WithContext(r.Context()).Where("provider = ?", name).First(&credential).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		writeInternalError(w, err)
		return
	}
	credential.DisplayName = req.DisplayName
	credential.APIKey = req.APIKey
	credential.BearerToken = req.BearerToken
	credential.BaseURL = req.BaseURL
	credential.ModelName = req.ModelName
	credential.ProxyURL = req.ProxyURL
	credential.ExtraConfig = req.ExtraConfig
	credential.Priority = req.Priority
	credential.Enabled = req.Enabled
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := a.db.WithContext(r.Context()).Create(&credential).Error; err != nil {
			writeInternalError(w, err)
			return
		}
	} else if err := a.db.WithContext(r.Context()).Save(&credential).Error; err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, credential)
}

func (a *API) HandleListConnectors(w http.ResponseWriter, r *http.Request) {
	var connectors []store.ConnectorConfig
	if err := a.db.WithContext(r.Context()).Order("id ASC").Find(&connectors).Error; err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, connectors)
}

func (a *API) HandleTestConnector(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUint(w, r, "id")
	if !ok {
		return
	}
	var cfg store.ConnectorConfig
	if err := a.db.WithContext(r.Context()).First(&cfg, id).Error; err != nil {
		writeDomainError(w, err)
		return
	}
	if a.connectorReg == nil {
		writeError(w, http.StatusBadRequest, "connector registry not configured")
		return
	}
	connector, err := a.connectorReg.Get(cfg.Type)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	info, err := connector.Ping(r.Context())
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (a *API) HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		var payload map[string]any
		if !decodeJSON(w, r, &payload) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"saved": true, "settings": payload})
		return
	}
	var (
		providerCredentials []store.ProviderCredential
		connectorConfigs    []store.ConnectorConfig
		libraryCount        int64
		taskCount           int64
	)
	ctx := r.Context()
	if err := a.db.WithContext(ctx).Order("provider ASC").Find(&providerCredentials).Error; err != nil {
		writeInternalError(w, err)
		return
	}
	if err := a.db.WithContext(ctx).Order("id ASC").Find(&connectorConfigs).Error; err != nil {
		writeInternalError(w, err)
		return
	}
	if err := a.db.WithContext(ctx).Model(&store.MediaLibraryConfig{}).Count(&libraryCount).Error; err != nil {
		writeInternalError(w, err)
		return
	}
	if err := a.db.WithContext(ctx).Model(&store.ScrapeTask{}).Count(&taskCount).Error; err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"provider_credentials": providerCredentials,
		"connectors":           connectorConfigs,
		"providers":            a.sourceReg.List(),
		"llm_providers":        a.sortedLLMProviderNames(),
		"library_count":        libraryCount,
		"task_count":           taskCount,
	})
}

func (a *API) HandleSearch(w http.ResponseWriter, r *http.Request) {
	var req searchRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	providerNames := req.Providers
	if len(providerNames) == 0 {
		if req.Provider != "" {
			providerNames = []string{req.Provider}
		} else {
			providerNames = a.sourceReg.List()
		}
	}
	mediaType := normalizeMediaType(req.Type)
	results := make([]core.MediaSearchCandidate, 0)
	var lastErr error
	for _, name := range dedupeStrings(providerNames) {
		src, err := a.sourceReg.Get(name)
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
				Query:            req.Query,
				Year:             req.Year,
				Language:         req.Language,
				FallbackLanguage: req.FallbackLanguage,
				IMDBID:           req.IMDBID,
				TMDBID:           req.TMDBID,
				IncludeAdult:     req.IncludeAdult,
			})
			if err != nil {
				lastErr = err
				continue
			}
			results = append(results, items...)
		case "tv":
			showScraper, ok := src.(core.TvShowMetadataScraper)
			if !ok {
				continue
			}
			items, err := showScraper.SearchTvShow(ctx, core.TvShowSearchOptions{
				Query:            req.Query,
				Year:             req.Year,
				FirstAirYear:     req.Year,
				Language:         req.Language,
				FallbackLanguage: req.FallbackLanguage,
				IMDBID:           req.IMDBID,
				TMDBID:           req.TMDBID,
				IncludeAdult:     req.IncludeAdult,
			})
			if err != nil {
				lastErr = err
				continue
			}
			results = append(results, items...)
		default:
			writeError(w, http.StatusBadRequest, "invalid search type")
			return
		}
	}
	if len(results) == 0 && lastErr != nil {
		writeDomainError(w, lastErr)
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func (a *API) HandleGetArtworks(w http.ResponseWriter, r *http.Request) {
	provider := strings.TrimSpace(r.URL.Query().Get("provider"))
	entityID := strings.TrimSpace(r.URL.Query().Get("entity_id"))
	if provider == "" || entityID == "" {
		writeError(w, http.StatusBadRequest, "provider and entity_id are required")
		return
	}
	src, err := a.sourceReg.Get(provider)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	artScraper, ok := src.(core.ArtworkScraper)
	if !ok {
		writeError(w, http.StatusBadRequest, "provider does not support artworks")
		return
	}
	items, err := artScraper.GetArtwork(r.Context(), core.ArtworkSearchOptions{
		EntityID: entityID,
		Type:     parseCoreMediaType(r.URL.Query().Get("media_type")),
		Language: strings.TrimSpace(r.URL.Query().Get("language")),
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *API) HandleListLLMProviders(w http.ResponseWriter, r *http.Request) {
	items := make([]map[string]any, 0, len(a.llmProviders))
	for _, name := range a.sortedLLMProviderNames() {
		provider := a.llmProviders[name]
		items = append(items, map[string]any{
			"name": name,
			"kind": provider.Kind(),
		})
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *API) HandleLLMGenerate(w http.ResponseWriter, r *http.Request) {
	var req llmGenerateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	provider, ok := a.llmProviders[strings.TrimSpace(req.Provider)]
	if !ok {
		writeError(w, http.StatusBadRequest, "llm provider not found")
		return
	}
	result, err := provider.Extract(r.Context(), llm.ExtractRequest{
		RawTitle:    req.RawTitle,
		Filename:    req.Filename,
		FileSize:    req.FileSize,
		SiteHints:   req.SiteHints,
		UserContext: req.UserContext,
		Language:    req.Language,
		MediaType:   req.MediaType,
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *API) HandleLLMValidate(w http.ResponseWriter, r *http.Request) {
	var req llmValidateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	warnings := llm.ValidateFieldFormat(&req.Result)
	writeJSON(w, http.StatusOK, map[string]any{
		"valid":    len(warnings) == 0,
		"warnings": warnings,
		"result":   req.Result,
	})
}

type createLibraryRequest struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Path        string   `json:"path"`
	ProviderIDs []string `json:"provider_ids"`
	ConnectorID *uint    `json:"connector_id"`
	ScanCron    string   `json:"scan_cron"`
	AutoScrape  bool     `json:"auto_scrape"`
	NfoDialect  string   `json:"nfo_dialect"`
}

type updateLibraryRequest struct {
	Name        *string  `json:"name"`
	Type        *string  `json:"type"`
	Path        *string  `json:"path"`
	Enabled     *bool    `json:"enabled"`
	ProviderIDs []string `json:"provider_ids"`
	ConnectorID *uint    `json:"connector_id"`
	ScanCron    *string  `json:"scan_cron"`
	AutoScrape  *bool    `json:"auto_scrape"`
	NfoDialect  *string  `json:"nfo_dialect"`
}

type scrapeRequest struct {
	Type         string   `json:"type"`
	LibraryID    *uint    `json:"library_id"`
	MediaPath    string   `json:"media_path"`
	Title        string   `json:"title"`
	Year         int      `json:"year"`
	Season       int      `json:"season"`
	Episode      int      `json:"episode"`
	Providers    []string `json:"providers"`
	Locale       string   `json:"locale"`
	NfoDialect   string   `json:"nfo_dialect"`
	ConnectorID  *uint    `json:"connector_id"`
	OverwriteNFO bool     `json:"overwrite_nfo"`
}

type providerCredentialRequest struct {
	DisplayName string `json:"display_name"`
	APIKey      string `json:"api_key"`
	BearerToken string `json:"bearer_token"`
	BaseURL     string `json:"base_url"`
	ModelName   string `json:"model_name"`
	ProxyURL    string `json:"proxy_url"`
	ExtraConfig string `json:"extra_config"`
	Priority    int    `json:"priority"`
	Enabled     bool   `json:"enabled"`
}

type searchRequest struct {
	Provider         string   `json:"provider"`
	Providers        []string `json:"providers"`
	Type             string   `json:"type"`
	Query            string   `json:"query"`
	Year             int      `json:"year"`
	Language         string   `json:"language"`
	FallbackLanguage string   `json:"fallback_language"`
	IMDBID           string   `json:"imdb_id"`
	TMDBID           int      `json:"tmdb_id"`
	IncludeAdult     bool     `json:"include_adult"`
}

type llmGenerateRequest struct {
	Provider    string            `json:"provider"`
	RawTitle    string            `json:"raw_title"`
	Filename    string            `json:"filename"`
	FileSize    int64             `json:"file_size"`
	SiteHints   map[string]string `json:"site_hints"`
	UserContext string            `json:"user_context"`
	Language    string            `json:"language"`
	MediaType   string            `json:"media_type"`
}

type llmValidateRequest struct {
	Result    llm.NFOResult `json:"result"`
	MediaType string        `json:"media_type"`
}

type noopLogger struct{}

func (noopLogger) Debugf(string, ...any) {}
func (noopLogger) Infof(string, ...any)  {}
func (noopLogger) Warnf(string, ...any)  {}
func (noopLogger) Errorf(string, ...any) {}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, `{"error":"encode response"}`, http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid json: %v", err))
		return false
	}
	return true
}

func pathUint(w http.ResponseWriter, r *http.Request, key string) (uint, bool) {
	id, err := strconv.ParseUint(strings.TrimSpace(r.PathValue(key)), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return uint(id), true
}

func writeDomainError(w http.ResponseWriter, err error) {
	if err == nil {
		writeError(w, http.StatusInternalServerError, "unknown error")
		return
	}
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, core.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, core.ErrInvalidID), strings.Contains(strings.ToLower(err.Error()), "required"):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
}

func writeInternalError(w http.ResponseWriter, err error) {
	writeError(w, http.StatusInternalServerError, err.Error())
}

func normalizeMediaType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
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

func parseCoreMediaType(v string) core.MediaType {
	switch normalizeMediaType(v) {
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

func dedupeStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func (a *API) sortedLLMProviderNames() []string {
	names := make([]string, 0, len(a.llmProviders))
	for name := range a.llmProviders {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
