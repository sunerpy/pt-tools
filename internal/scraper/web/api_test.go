package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
	"github.com/sunerpy/pt-tools/internal/scraper/service"
	"github.com/sunerpy/pt-tools/internal/scraper/source/llm"
	"github.com/sunerpy/pt-tools/internal/scraper/store"
)

type apiSearchScraper struct{}

func (apiSearchScraper) Info() core.ProviderInfo {
	return core.ProviderInfo{Name: "tmdb", DisplayName: "TMDB"}
}
func (apiSearchScraper) IsActive() bool { return true }
func (apiSearchScraper) SearchMovie(context.Context, core.MovieSearchOptions) ([]core.MediaSearchCandidate, error) {
	return []core.MediaSearchCandidate{{ID: "1", Title: "Inception", Year: 2010, Provider: "tmdb", MediaType: core.MediaTypeMovie}}, nil
}

func (apiSearchScraper) GetMovieMetadata(context.Context, core.MovieSearchOptions) (*core.Movie, error) {
	return &core.Movie{}, nil
}

func (apiSearchScraper) SearchTvShow(context.Context, core.TvShowSearchOptions) ([]core.MediaSearchCandidate, error) {
	return []core.MediaSearchCandidate{{ID: "2", Title: "Breaking Bad", Year: 2008, Provider: "tmdb", MediaType: core.MediaTypeTvShow}}, nil
}

func (apiSearchScraper) GetTvShowMetadata(context.Context, core.TvShowSearchOptions) (*core.TvShow, error) {
	return &core.TvShow{}, nil
}

func (apiSearchScraper) GetEpisodeList(context.Context, core.TvShowSearchOptions) ([]core.TvShowEpisode, error) {
	return nil, nil
}

func (apiSearchScraper) GetEpisodeMetadata(context.Context, core.TvShowEpisodeSearchOptions) (*core.TvShowEpisode, error) {
	return nil, nil
}

func (apiSearchScraper) GetArtwork(context.Context, core.ArtworkSearchOptions) ([]core.MediaArtwork, error) {
	return []core.MediaArtwork{{Type: core.ArtworkTypePoster, URL: "https://img/poster.jpg", Provider: "tmdb"}}, nil
}

type apiConnector struct{}

func (apiConnector) Name() string { return "jellyfin" }
func (apiConnector) Ping(context.Context) (*core.ServerInfo, error) {
	return &core.ServerInfo{Product: "Jellyfin", Version: "10.9.0", ServerID: "srv1", Name: "lab"}, nil
}
func (apiConnector) Authenticate(context.Context) (*core.ServerInfo, error) { return nil, nil }
func (apiConnector) ListLibraries(context.Context) ([]core.Library, error)  { return nil, nil }
func (apiConnector) RefreshLibrary(context.Context, string) error           { return nil }
func (apiConnector) ScanStatus(context.Context) (*core.ScanStatus, error)   { return nil, nil }

type apiTask struct{}

func (apiTask) ID() string                { return "task" }
func (apiTask) Type() string              { return "movie" }
func (apiTask) Run(context.Context) error { return errors.New("stop") }
func (apiTask) State() core.TaskState     { return core.TaskPending }
func (apiTask) RetryCount() int           { return 0 }
func (apiTask) MaxRetries() int           { return 0 }
func (apiTask) SetState(core.TaskState)   {}
func (apiTask) IncrementRetry()           {}
func (apiTask) LastError() error          { return nil }
func (apiTask) SetLastError(error)        {}

type apiLLMProvider struct{}

func (apiLLMProvider) Name() string           { return "mock-llm" }
func (apiLLMProvider) Kind() llm.ProviderKind { return llm.KindOpenAICompat }
func (apiLLMProvider) Close() error           { return nil }
func (apiLLMProvider) Extract(context.Context, llm.ExtractRequest) (*llm.NFOResult, error) {
	return &llm.NFOResult{Title: "Generated", Year: 2010, Type: "movie", TMDBID: 1}, nil
}

type apiFuser struct{}

func (apiFuser) Merge(context.Context, map[string]*core.RawMediaInfo) (*core.Movie, error) {
	return &core.Movie{}, nil
}

func (apiFuser) MergeTv(context.Context, map[string]*core.RawMediaInfo) (*core.TvShow, error) {
	return &core.TvShow{}, nil
}

func (apiFuser) MergeEpisode(context.Context, map[string]*core.RawMediaInfo) (*core.TvShowEpisode, error) {
	return &core.TvShowEpisode{}, nil
}

func openWebTestDB(t *testing.T) *gorm.DB {
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

func newTestAPI(t *testing.T) (*API, *gorm.DB) {
	t.Helper()
	db := openWebTestDB(t)
	librarySvc, err := service.NewLibraryService(service.LibraryConfig{DB: db, PathValidator: func(string) error { return nil }})
	require.NoError(t, err)
	sourceReg := core.NewRegistry[core.MediaScraper]()
	writerReg := core.NewRegistry[core.NfoWriter]()
	connectorReg := core.NewRegistry[core.MediaServerConnector]()
	require.NoError(t, sourceReg.Register("tmdb", func() core.MediaScraper { return apiSearchScraper{} }))
	require.NoError(t, writerReg.Register("universal", func() core.NfoWriter { return &stubWriter{} }))
	require.NoError(t, connectorReg.Register("jellyfin", func() core.MediaServerConnector { return apiConnector{} }))
	pq, err := service.NewPersistentQueue(service.PersistentConfig{DB: db, BufferSize: 8, TaskBuilder: func(store.ScrapeTask) core.Task { return apiTask{} }, RetryCheckInterval: time.Hour})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); pq.Stop() })
	require.NoError(t, pq.Start(ctx, 1))
	scrapeSvc, err := service.NewScrapeService(service.ServiceConfig{DB: db, SourceReg: sourceReg, WriterReg: writerReg, ConnectorReg: connectorReg, Fuser: apiFuser{}, Queue: pq})
	require.NoError(t, err)
	api, err := NewAPI(APIConfig{Scrape: scrapeSvc, Library: librarySvc, DB: db, SourceReg: sourceReg, ConnectorReg: connectorReg, LLMProviders: map[string]llm.Provider{"mock": apiLLMProvider{}}})
	require.NoError(t, err)
	return api, db
}

type stubWriter struct{}

func (s *stubWriter) Dialect() string { return "universal" }

func (s *stubWriter) WriteMovieNfo(context.Context, *core.Movie, []string) error { return nil }

func (s *stubWriter) WriteTvShowNfo(context.Context, *core.TvShow, string) error { return nil }

func (s *stubWriter) WriteSeasonNfo(context.Context, *core.TvShowSeason, string) error {
	return nil
}

func (s *stubWriter) WriteEpisodeNfo(context.Context, *core.TvShowEpisode, string) error {
	return nil
}

func TestAPI_CreateAndListLibraries(t *testing.T) {
	api, _ := newTestAPI(t)
	body := mustJSON(t, map[string]any{"name": "Movies", "path": t.TempDir(), "provider_ids": []string{"tmdb"}})
	rr := httptest.NewRecorder()
	api.HandleCreateLibrary(rr, httptest.NewRequest(http.MethodPost, "/", body))
	require.Equal(t, http.StatusCreated, rr.Code)

	rr = httptest.NewRecorder()
	api.HandleListLibraries(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, rr.Code)
	var libs []store.MediaLibraryConfig
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &libs))
	require.Len(t, libs, 1)
	require.Equal(t, "Movies", libs[0].Name)
}

func TestAPI_GetLibrary_NotFound(t *testing.T) {
	api, _ := newTestAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/scraper/libraries/99", nil)
	req.SetPathValue("id", "99")
	rr := httptest.NewRecorder()
	api.HandleGetLibrary(rr, req)
	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestAPI_UpdateLibrary(t *testing.T) {
	api, db := newTestAPI(t)
	lib := store.MediaLibraryConfig{Name: "Old", Path: t.TempDir(), Type: "movie"}
	require.NoError(t, db.Create(&lib).Error)
	body := mustJSON(t, map[string]any{"name": "New"})
	req := httptest.NewRequest(http.MethodPut, "/api/v2/scraper/libraries/1", body)
	req.SetPathValue("id", strconvUint(lib.ID))
	rr := httptest.NewRecorder()
	api.HandleUpdateLibrary(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	var updated store.MediaLibraryConfig
	require.NoError(t, db.First(&updated, lib.ID).Error)
	require.Equal(t, "New", updated.Name)
}

func TestAPI_DeleteLibrary(t *testing.T) {
	api, db := newTestAPI(t)
	lib := store.MediaLibraryConfig{Name: "DeleteMe", Path: t.TempDir(), Type: "movie"}
	require.NoError(t, db.Create(&lib).Error)
	req := httptest.NewRequest(http.MethodDelete, "/api/v2/scraper/libraries/1", nil)
	req.SetPathValue("id", strconvUint(lib.ID))
	rr := httptest.NewRecorder()
	api.HandleDeleteLibrary(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	var count int64
	require.NoError(t, db.Model(&store.MediaLibraryConfig{}).Where("id = ?", lib.ID).Count(&count).Error)
	require.Zero(t, count)
}

func TestAPI_ScrapeEnqueueTask(t *testing.T) {
	api, db := newTestAPI(t)
	body := mustJSON(t, map[string]any{"type": "movie", "media_path": "/tmp/Inception (2010).mkv", "providers": []string{"tmdb"}})
	rr := httptest.NewRecorder()
	api.HandleScrape(rr, httptest.NewRequest(http.MethodPost, "/", body))
	require.Equal(t, http.StatusAccepted, rr.Code)
	var count int64
	require.NoError(t, db.Model(&store.ScrapeTask{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestAPI_ListAndGetTasks(t *testing.T) {
	api, db := newTestAPI(t)
	require.NoError(t, db.Create(&store.ScrapeTask{TaskType: "movie", MediaPath: "/tmp/a", State: "pending"}).Error)
	rr := httptest.NewRecorder()
	api.HandleListTasks(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, rr.Code)
	var tasks []store.ScrapeTask
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &tasks))
	require.Len(t, tasks, 1)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetPathValue("id", strconvUint(tasks[0].ID))
	rr = httptest.NewRecorder()
	api.HandleGetTask(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
}

func TestAPI_DeleteTask(t *testing.T) {
	api, db := newTestAPI(t)
	task := store.ScrapeTask{TaskType: "movie", MediaPath: "/tmp/a", State: "pending"}
	require.NoError(t, db.Create(&task).Error)
	require.NoError(t, db.Create(&store.ScrapeResult{TaskID: task.ID, MediaType: "movie", Title: "A", FilePath: "/tmp/a", ScrapedAt: time.Now()}).Error)
	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	req.SetPathValue("id", strconvUint(task.ID))
	rr := httptest.NewRecorder()
	api.HandleDeleteTask(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	var count int64
	require.NoError(t, db.Model(&store.ScrapeTask{}).Where("id = ?", task.ID).Count(&count).Error)
	require.Zero(t, count)
}

func TestAPI_ListProviders(t *testing.T) {
	api, _ := newTestAPI(t)
	rr := httptest.NewRecorder()
	api.HandleListProviders(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, rr.Code)
	var infos []core.ProviderInfo
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &infos))
	require.Len(t, infos, 1)
	require.Equal(t, "tmdb", infos[0].Name)
}

func TestAPI_SetProviderCredential(t *testing.T) {
	api, db := newTestAPI(t)
	body := mustJSON(t, map[string]any{"display_name": "TMDB", "api_key": "secret", "enabled": true})
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.SetPathValue("name", "tmdb")
	rr := httptest.NewRecorder()
	api.HandleSetProviderCredential(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	var cred store.ProviderCredential
	require.NoError(t, db.Where("provider = ?", "tmdb").First(&cred).Error)
	require.Equal(t, "TMDB", cred.DisplayName)
}

func TestAPI_ListConnectorsAndTestConnector(t *testing.T) {
	api, db := newTestAPI(t)
	conn := store.ConnectorConfig{Type: "jellyfin", Name: "jf", BaseURL: "http://x"}
	require.NoError(t, db.Create(&conn).Error)
	rr := httptest.NewRecorder()
	api.HandleListConnectors(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, rr.Code)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.SetPathValue("id", strconvUint(conn.ID))
	rr = httptest.NewRecorder()
	api.HandleTestConnector(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
}

func TestAPI_GetSettingsAndSearch(t *testing.T) {
	api, _ := newTestAPI(t)
	rr := httptest.NewRecorder()
	api.HandleGetSettings(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, rr.Code)

	body := mustJSON(t, map[string]any{"provider": "tmdb", "type": "movie", "query": "Inception"})
	rr = httptest.NewRecorder()
	api.HandleSearch(rr, httptest.NewRequest(http.MethodPost, "/", body))
	require.Equal(t, http.StatusOK, rr.Code)
	var items []core.MediaSearchCandidate
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &items))
	require.Len(t, items, 1)
}

func TestAPI_GetArtworks(t *testing.T) {
	api, _ := newTestAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/?provider=tmdb&entity_id=1&media_type=movie", nil)
	rr := httptest.NewRecorder()
	api.HandleGetArtworks(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	var items []core.MediaArtwork
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &items))
	require.Len(t, items, 1)
}

func TestAPI_LLMHandlersAndRouteRegistration(t *testing.T) {
	api, _ := newTestAPI(t)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux, NoAuthMiddleware())

	pages := []string{
		"/api/v2/scraper/llm/providers",
		"/api/v2/scraper/providers",
		"/api/v2/scraper/settings",
	}
	sort.Strings(pages)
	for _, path := range pages {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		require.NotEqual(t, http.StatusNotFound, rr.Code)
	}

	body := mustJSON(t, map[string]any{"provider": "mock", "raw_title": "Inception 2010", "media_type": "movie"})
	rr := httptest.NewRecorder()
	api.HandleLLMGenerate(rr, httptest.NewRequest(http.MethodPost, "/", body))
	require.Equal(t, http.StatusOK, rr.Code)

	body = mustJSON(t, map[string]any{"result": map[string]any{"title": "X", "year": 1890, "tmdb_id": 10000001}})
	rr = httptest.NewRecorder()
	api.HandleLLMValidate(rr, httptest.NewRequest(http.MethodPost, "/", body))
	require.Equal(t, http.StatusOK, rr.Code)
}

func mustJSON(t *testing.T, payload any) *bytes.Reader {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return bytes.NewReader(data)
}

func strconvUint(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
